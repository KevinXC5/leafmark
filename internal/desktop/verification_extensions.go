//go:build verification

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/diagram"
	"leafmark/internal/mathlayout"
	"leafmark/internal/nativeeditor"
	"leafmark/internal/richtext"
)

// verifyNativeExtensions 在真实窗口里核对本轮新增的语法、阅读模式、替换、导出与源码查找。
// 依赖的接口若尚未接入，结果里记为“未接入”并返回可读的失败，不静默跳过。
func verifyNativeExtensions(win *mygo.Window, app *nativeApp) (map[string]any, error) {
	out := map[string]any{}
	if err := verifyNewSyntax(win, app, out); err != nil {
		return out, err
	}
	if err := verifyReadingMode(app, out); err != nil {
		return out, err
	}
	if err := verifyFindReplace(app, out); err != nil {
		return out, err
	}
	if err := verifyExportGraphics(app, out); err != nil {
		return out, err
	}
	if err := verifySourceFind(win, app, out); err != nil {
		return out, err
	}
	return out, nil
}

// extensionMarkdown 覆盖脚注、含子段落与代码块的列表项、嵌套引用、多段提示块、
// 缩进代码块、引用式链接与图片，以及常用 HTML 子集。
const extensionMarkdown = "# 扩展语法\n\n" +
	"正文见脚注[^注]。\n\n" +
	"[^注]: 脚注说明。\n\n" +
	"- 列表首段\n\n" +
	"  列表子段落\n\n" +
	"      列表里的代码\n\n" +
	"> 外层引用\n>\n> > 内层引用\n\n" +
	"> [!note] 多段提示\n>\n> 提示第一段\n>\n> 提示第二段\n\n" +
	"    缩进代码块\n\n" +
	"引用式链接见[叶脉][leaf]，图片见 ![叶][leaf-img]。\n\n" +
	"[leaf]: https://example.com/leaf \"叶脉\"\n" +
	"[leaf-img]: leaf.png \"一片叶子\"\n\n" +
	"<div class=\"note\"><p>常用 <strong>HTML</strong> 子集</p></div>\n"

// verifyNewSyntax 打开一份独立副本，确认新语法不再落成 Raw 占位，
// 未编辑时 Markdown 与原文逐字节一致，并在列表项子段落里输入后撤销。
func verifyNewSyntax(win *mygo.Window, app *nativeApp, out map[string]any) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, ".verification-data", "native")
	path := filepath.Join(dir, "扩展语法验证.md")
	if err = os.WriteFile(path, []byte(extensionMarkdown), 0600); err != nil {
		return err
	}
	defer os.Remove(path)

	var tabID string
	var opened error
	mygo.RunOnMain(func() {
		doc, loadErr := app.files.store.Load(path)
		if loadErr != nil {
			opened = fmt.Errorf("扩展语法文档加载失败：%v", loadErr)
			return
		}
		app.adopt(doc, doc.Content)
		tabID = app.active().id
	})
	if opened != nil {
		return opened
	}
	defer func() {
		mygo.RunOnMain(func() { app.finishClose([]string{tabID}) })
	}()
	// 等待真实 GPU 帧显示扩展结构，截图用于核对容器和脚注的视觉。
	if win != nil {
		refreshVerificationWindow(win)
		time.Sleep(150 * time.Millisecond)
		refreshVerificationWindow(win)
		if shot, err := win.CapturePage(); err == nil {
			if err := os.WriteFile("verification/native-extensions.png", shot, 0644); err != nil {
				return fmt.Errorf("扩展语法截图写入失败：%w", err)
			}
		} else {
			return fmt.Errorf("扩展语法截图失败：%w", err)
		}
	}

	var check error
	mygo.RunOnMain(func() {
		ed, ok := app.active().editor.(*nativeeditor.Editor)
		if !ok {
			check = fmt.Errorf("扩展语法验收没有使用原生编辑器")
			return
		}
		got := ed.Markdown()
		if got != extensionMarkdown {
			out["syntaxRoundTrip"] = false
			check = fmt.Errorf("未编辑时 Markdown 与原文不一致")
			return
		}
		out["syntaxRoundTrip"] = true
		doc := richtext.Parse(got)
		for _, block := range doc.Blocks() {
			if block.Kind == richtext.Raw {
				out["syntaxStructured"] = false
				check = fmt.Errorf("新语法仍落成 Raw 占位：%q", truncate(block.Raw, 40))
				return
			}
		}
		if !syntaxPresent(doc) {
			out["syntaxStructured"] = false
			check = fmt.Errorf("新语法没有进入结构化块（脚注、容器列表、嵌套引用、提示块、缩进代码、引用式链接或 HTML）")
			return
		}
		out["syntaxStructured"] = true

		text := ed.Text()
		at := strings.Index(text, "列表子段落")
		if at < 0 {
			check = fmt.Errorf("列表项子段落没有排进正文")
			return
		}
		from := len([]rune(text[:at])) + len([]rune("列表子段落"))
		before := ed.Markdown()
		ed.SetSelection(from, from)
		ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "增补"})
		if ed.Markdown() == before || !strings.Contains(ed.Markdown(), "列表子段落增补") {
			out["syntaxNestedEdit"] = false
			check = fmt.Errorf("列表项子段落输入没有写入文档")
			return
		}
		ed.Undo()
		if ed.Markdown() != before {
			out["syntaxNestedEdit"] = false
			check = fmt.Errorf("撤销后列表项子段落没有恢复原文")
			return
		}
		out["syntaxNestedEdit"] = true
	})
	return check
}

// syntaxPresent 确认各扩展语法都进入了对应的结构化块，而不是被吞掉。
func syntaxPresent(doc *richtext.Document) bool {
	var footnoteRef, footnoteDef, nestedList, nestedQuote, callout, indented, refLink, html bool
	calloutParas := 0
	for _, block := range doc.Blocks() {
		for _, run := range block.Runs {
			if run.Link != nil && run.Link.Footnote != "" {
				footnoteRef = true
			}
			if run.Link != nil && run.Link.URL != "" {
				refLink = true
			}
		}
		if block.Footnote != nil {
			footnoteDef = true
		}
		if block.Kind == richtext.Code && strings.TrimSpace(block.Code) == "缩进代码块" {
			indented = true
		}
		if block.Kind == richtext.Raw && strings.Contains(block.Raw, "<div") {
			html = false
		}
		depth := map[richtext.Kind]int{}
		for _, c := range block.Containers {
			depth[c.Kind]++
			if c.Kind == richtext.Callout && block.Kind == richtext.Paragraph {
				callout = true
				calloutParas++
			}
		}
		if depth[richtext.List] >= 1 && block.Kind == richtext.Paragraph {
			nestedList = true
		}
		if depth[richtext.Quote] >= 2 {
			nestedQuote = true
		}
	}
	// HTML 子集被解析后不再以 Raw 原文出现，正文里应能看到它的文字。
	if strings.Contains(doc.Text(), "常用") && strings.Contains(doc.Text(), "HTML") && strings.Contains(doc.Text(), "子集") {
		html = true
	}
	return footnoteRef && footnoteDef && nestedList && nestedQuote && callout && calloutParas >= 2 && indented && refLink && html
}

// verifyReadingMode 进入阅读模式后，输入和格式化都不改变内容，查找仍能定位；退出后恢复可编辑。
func verifyReadingMode(app *nativeApp, out map[string]any) error {
	var check error
	var tabID string
	mygo.RunOnMain(func() {
		if app.reading {
			app.command("reading")
		}
		doc := app.files.store.New("阅读模式验证.md", extensionMarkdown)
		app.adopt(doc, doc.Content)
		tabID = app.active().id
		ed, ok := app.active().editor.(*nativeeditor.Editor)
		if !ok {
			check = fmt.Errorf("阅读模式验收没有使用原生编辑器")
			return
		}
		if _, ok := any(ed).(interface{ SetReadOnly(bool) }); !ok {
			out["readingMode"] = "未接入"
			check = fmt.Errorf("未接入：原生编辑器没有 SetReadOnly，阅读模式无法生效")
			return
		}
		before := ed.Markdown()
		text := ed.Text()
		needle := "列表子段落"
		at := strings.Index(text, needle)
		if at < 0 {
			check = fmt.Errorf("阅读模式验收找不到可定位的正文")
			return
		}
		from := len([]rune(text[:at]))
		ed.SetSelection(from, from)
		app.command("reading")
		if !app.reading {
			out["readingMode"] = false
			check = fmt.Errorf("阅读模式没有进入")
			return
		}
		ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "不应写入"})
		ed.Format("bold")
		app.command("bold")
		if ed.Markdown() != before {
			out["readingMode"] = false
			check = fmt.Errorf("阅读模式下输入或格式化改变了内容")
			return
		}
		if !ed.Find(needle) {
			out["readingFind"] = false
			check = fmt.Errorf("阅读模式下查找没有定位到“%s”", needle)
			return
		}
		start, end := ed.Selection()
		if end-start != len([]rune(needle)) {
			out["readingFind"] = false
			check = fmt.Errorf("阅读模式下查找没有选中匹配")
			return
		}
		out["readingFind"] = true
		app.command("reading")
		if app.reading {
			out["readingMode"] = false
			check = fmt.Errorf("阅读模式没有退出")
			return
		}
		ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "恢复"})
		if ed.Markdown() == before || !strings.Contains(ed.Text(), "恢复") {
			out["readingMode"] = false
			check = fmt.Errorf("退出阅读模式后没有恢复可编辑")
			return
		}
		ed.Undo()
		if ed.Markdown() != before {
			out["readingMode"] = false
			check = fmt.Errorf("退出阅读模式后的编辑无法撤销回原文")
			return
		}
		out["readingMode"] = true
	})
	if tabID != "" {
		mygo.RunOnMain(func() {
			if app.reading {
				app.command("reading")
			}
			app.finishClose([]string{tabID})
		})
	}
	return check
}

// verifyFindReplace 核对单处与全部替换的计数和结果，并确认一次撤销能恢复原文。
func verifyFindReplace(app *nativeApp, out map[string]any) error {
	const source = "甲处出现，甲处再现。\n\n甲处收尾。\n"
	var check error
	var tabID string
	mygo.RunOnMain(func() {
		if app.reading {
			app.command("reading")
		}
		doc := app.files.store.New("替换验证.md", source)
		app.adopt(doc, doc.Content)
		tabID = app.active().id
		ed, ok := app.active().editor.(*nativeeditor.Editor)
		if !ok {
			check = fmt.Errorf("查找替换验收没有使用原生编辑器")
			return
		}
		replacer, ok := any(ed).(interface {
			Replace(query, replacement string, all bool) int
		})
		if !ok {
			out["replaceOne"] = "未接入"
			out["replaceAll"] = "未接入"
			check = fmt.Errorf("未接入：原生编辑器没有 Replace，无法验收查找替换")
			return
		}
		app.findQuery, app.replaceText = "甲处", "乙处"
		ed.SetSelection(0, 0)
		if n := replacer.Replace(app.findQuery, app.replaceText, false); n != 1 {
			out["replaceOne"] = false
			check = fmt.Errorf("单处替换计数为 %d，期望 1", n)
			return
		}
		if strings.Count(ed.Markdown(), "乙处") != 1 || strings.Count(ed.Markdown(), "甲处") != 2 {
			out["replaceOne"] = false
			check = fmt.Errorf("单处替换结果不符合预期：%q", ed.Markdown())
			return
		}
		// 单处替换后选区应落在下一处匹配上，而不是停在刚替换的位置。
		start, end := ed.Selection()
		runes := []rune(ed.Text())
		if start < 0 || end > len(runes) || string(runes[start:end]) != "甲处" {
			out["replaceOne"] = false
			check = fmt.Errorf("单处替换后没有定位到下一处匹配")
			return
		}
		out["replaceOne"] = true
		ed.Undo()
		if ed.Markdown() != source {
			out["replaceUndo"] = false
			check = fmt.Errorf("撤销单处替换没有恢复原文")
			return
		}
		out["replaceUndo"] = true
		ed.SetSelection(0, 0)
		if n := replacer.Replace(app.findQuery, app.replaceText, true); n != 3 {
			out["replaceAll"] = false
			check = fmt.Errorf("全部替换计数为 %d，期望 3", n)
			return
		}
		if strings.Contains(ed.Markdown(), "甲处") || strings.Count(ed.Markdown(), "乙处") != 3 {
			out["replaceAll"] = false
			check = fmt.Errorf("全部替换结果不符合预期：%q", ed.Markdown())
			return
		}
		out["replaceAll"] = true
		// 全部替换应是一步撤销。文档模型还没有 ReplaceAll 时，编辑器退化为逐处替换，
		// 这里明确记为未接入，而不是把多次撤销当成通过。
		ed.Undo()
		if ed.Markdown() != source {
			if _, ok := any(ed).(interface{ ReadOnly() bool }); ok {
				out["replaceAllUndo"] = "未接入"
				check = fmt.Errorf("未接入：文档模型没有一步完成的全文替换，全部替换撤销一次没有恢复原文")
				return
			}
			out["replaceAllUndo"] = false
			check = fmt.Errorf("全部替换撤销一次没有恢复原文：%q", ed.Markdown())
			return
		}
		out["replaceAllUndo"] = true
		app.findQuery, app.replaceText = "", ""
	})
	if tabID != "" {
		mygo.RunOnMain(func() { app.finishClose([]string{tabID}) })
	}
	return check
}

// exportProbeMarkdown 含行内公式、块级公式、流程图和时序图，供导出断言使用。
const exportProbeMarkdown = "# 导出探针\n\n" +
	"路程 $s = vt$ 继续。\n\n" +
	"$$\nQ_n = Q \\cdot r^n\n$$\n\n" +
	"```mermaid\ngraph LR\n甲-->乙\n```\n\n" +
	"```mermaid\nsequenceDiagram\n甲->>乙: 问候\n```\n"

// verifyExportGraphics 导出含公式与图表的 HTML，要求内嵌 <svg，且不再以源码代码块出现。
func verifyExportGraphics(app *nativeApp, out map[string]any) error {
	var check error
	var tabID string
	var exported string
	mygo.RunOnMain(func() {
		doc := app.files.store.New("导出探针.md", exportProbeMarkdown)
		app.adopt(doc, doc.Content)
		tabID = app.active().id
		name, body, ok := app.exportBody()
		if !ok {
			check = fmt.Errorf("导出探针没有生成正文")
			return
		}
		exported = exportDocument(name, body)
	})
	if tabID != "" {
		mygo.RunOnMain(func() { app.finishClose([]string{tabID}) })
	}
	if check != nil {
		return check
	}
	missing := []string{}
	if !strings.Contains(exported, "<svg") {
		missing = append(missing, "没有内嵌 <svg")
	}
	// 公式与图表排成 SVG 后，源码不应再以代码块或公式原文出现。
	for _, leaked := range []string{"<code", "graph LR", "sequenceDiagram", "Q_n", "s = vt"} {
		if strings.Contains(exported, leaked) {
			missing = append(missing, "仍以源码出现："+leaked)
		}
	}
	if len(missing) > 0 {
		out["exportGraphics"] = false
		reason := exportUnavailableReason()
		if reason != "" {
			out["exportGraphics"] = "未接入"
			return fmt.Errorf("未接入：导出 HTML 未嵌入公式与图表（%s）；%s", reason, strings.Join(missing, "；"))
		}
		return fmt.Errorf("导出 HTML 不符合预期：%s", strings.Join(missing, "；"))
	}
	out["exportGraphics"] = true
	_ = os.WriteFile("verification/native-export-graphics.html", []byte(exported), 0644)
	return nil
}

// exportUnavailableReason 说明公式或图表的 SVG 导出为什么还不可用。
func exportUnavailableReason() string {
	var reasons []string
	if _, err := mathlayout.Layout("s = vt", 16, false); err != nil {
		reasons = append(reasons, "行内公式排版失败")
	}
	box, err := mathlayout.Layout("Q_n = Q \\cdot r^n", 16, true)
	if err != nil || box == nil {
		reasons = append(reasons, "块级公式排版失败")
	} else if _, svgErr := box.SVG(); svgErr != nil {
		reasons = append(reasons, "公式 SVG 导出失败："+svgErr.Error())
	}
	for _, src := range []string{"graph LR\n甲-->乙", "sequenceDiagram\n甲->>乙: 问候"} {
		g, parseErr := diagram.Parse(src)
		if parseErr != nil || g == nil {
			reasons = append(reasons, "图表解析失败")
			continue
		}
		lay := g.Layout(diagram.Style{}, 640)
		if lay == nil {
			reasons = append(reasons, "图表布局失败")
			continue
		}
		if svg, ok := any(lay).(interface{ SVG() string }); !ok {
			reasons = append(reasons, "diagram.Layout 没有 SVG 方法")
		} else if out := svg.SVG(); !strings.Contains(out, "<svg") {
			reasons = append(reasons, "图表 SVG 为空")
		}
	}
	return strings.Join(reasons, "，")
}

// verifySourceFind 切到源码模式后，Find 应选中匹配文本。
func verifySourceFind(win *mygo.Window, app *nativeApp, out map[string]any) error {
	source := "# 源码查找\n\n" + strings.Repeat("源码长文滚动验证。\n", 80) + "\n## 末尾章节\n\n这里有一个独特词组：叶脉定位。\n"
	var check error
	var tabID string
	mygo.RunOnMain(func() {
		if app.reading {
			app.command("reading")
		}
		doc := app.files.store.New("源码查找.md", source)
		app.adopt(doc, doc.Content)
		tabID = app.active().id
		app.command("source")
		if !sourceMode(app.active()) {
			check = fmt.Errorf("没有切到源码模式")
			return
		}
		finder := app.active().editor
		if !finder.Find("叶脉定位") {
			out["sourceFind"] = false
			check = fmt.Errorf("源码模式查找没有命中")
			return
		}
		start, end := finder.Selection()
		runes := []rune(finder.Text())
		if start < 0 || end > len(runes) || string(runes[start:end]) != "叶脉定位" {
			out["sourceFind"] = false
			check = fmt.Errorf("源码模式查找没有选中匹配：选区 %d-%d", start, end)
			return
		}
		out["sourceFind"] = true
	})
	if check == nil && win != nil {
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			refreshVerificationWindow(win)
			time.Sleep(40 * time.Millisecond)
			ready := false
			mygo.RunOnMain(func() {
				source := app.active().editor.(*sourceEditor)
				at, _ := source.Selection()
				_, caretY, lineH := source.ed.PointForOffset(at)
				_, y := source.Scroll()
				ready = !source.Jumping() && y > 0 && caretY >= y-1 && caretY+lineH <= y+source.Bounds().H+1
			})
			if ready {
				break
			}
		}
		mygo.RunOnMain(func() {
			source := app.active().editor.(*sourceEditor)
			at, _ := source.Selection()
			_, caretY, lineH := source.ed.PointForOffset(at)
			_, y := source.Scroll()
			if source.Jumping() || y <= 0 || caretY < y-1 || caretY+lineH > y+source.Bounds().H+1 {
				out["sourceFindScroll"] = false
				check = fmt.Errorf("源码长文查找未滚到可见匹配：滚动=%v，光标=%v", y, caretY)
				return
			}
			out["sourceFindScroll"] = true
		})
	}
	if tabID != "" {
		mygo.RunOnMain(func() {
			if sourceMode(app.active()) {
				app.command("source")
			}
			app.finishClose([]string{tabID})
		})
	}
	return check
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
