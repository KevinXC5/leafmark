//go:build desktoptest

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/workspace"
)

func TestWordCountMatchesMarkdownConvention(t *testing.T) {
	for source, want := range map[string]int{
		"":             0,
		"山中来信":         4,
		"hello world":  2,
		"第3稿 draft.md": 5, // 第、3、稿、draft、md
		"# 标题\n\n**把窗推开**，café 2024": 8,
	} {
		if got := nativeWordCount(source); got != want {
			t.Fatalf("%q 字数=%d，预期 %d", source, got, want)
		}
	}
}

func TestSourceModeTogglesWithoutChangingDocument(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "# 标题\n\n正文\n")
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	if !view.HasText("• 原位编辑") {
		t.Fatal("状态栏应显示原位编辑")
	}
	if err := view.Click("切换原位编辑与源码"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	tab := app.active()
	source, ok := tab.editor.(*sourceEditor)
	if !ok || source.Markdown() != "# 标题\n\n正文\n" || !view.HasText("• 源码模式") {
		t.Fatalf("未进入源码模式：%T", tab.editor)
	}
	if app.pendingEdit(tab) {
		t.Fatal("切换模式不应产生未保存修改")
	}
	app.command("bold")
	if app.notice == "" {
		t.Fatal("源码模式下的排版命令应给出提示")
	}
	source.SetSelection(len([]rune(source.Text())), len([]rune(source.Text())))
	source.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "追加\n"})
	app.command("source")
	if _, ok := app.active().editor.(*stubEditor); !ok || app.active().editor.Markdown() != "# 标题\n\n正文\n追加\n" {
		t.Fatalf("回到原位编辑后正文错误：%q", app.active().editor.Markdown())
	}
	if !app.pendingEdit(app.active()) {
		t.Fatal("源码模式里的修改应保持为未保存")
	}
}

func TestSourceOutlineScrollsFullWidthEditor(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	markdown := "# 开头\n\n" + strings.Repeat("中文正文 abc\n\n", 100) + "## 目标章节\n\n正文\n"
	app.adopt(app.files.Current(), markdown)
	app.toggleSource()
	source := app.active().editor.(*sourceEditor)
	view := ui.NewTester(app.View, 1500, 760)
	for range 3 {
		view.Frame()
	}
	if box := source.Bounds(); box.X+box.W < 1499 {
		t.Fatalf("源码编辑区未贴到右侧：%+v", box)
	}
	// 通过大纲按钮验证实际滚动，而不只检查定位请求。
	if err := view.Click("大纲：目标章节"); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		view.Frame()
	}
	if _, y := source.Scroll(); y <= 1000 || source.Jumping() {
		t.Fatalf("大纲未滚动到目标章节：滚动=%v，待跳转=%v", y, source.Jumping())
	}
	if source.Markdown() != markdown || source.Changed() {
		t.Fatal("大纲跳转不应修改原文")
	}
	if err := view.Click("大纲：开头"); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		view.Frame()
	}
	if _, y := source.Scroll(); y > 24 {
		t.Fatalf("首个标题未回到顶部：%v", y)
	}
}

func TestSourceOutlineUsesSourceOffsets(t *testing.T) {
	markdown := "---\ntitle: 标题\n---\n\n```md\n# 假标题\n```\n\n# 同名\n\n正文😀\n\n# 同名\n\nSetext 标题\n===\n"
	headings := nativeSourceOutline(markdown)
	if len(headings) != 3 {
		t.Fatalf("源码标题数错误：%+v", headings)
	}
	for _, h := range headings {
		tail := string([]rune(markdown)[h.at:])
		if !strings.HasPrefix(tail, "# 同名") && !strings.HasPrefix(tail, "Setext 标题") {
			t.Fatalf("标题偏移不在源码行首：%+v，%q", h, tail)
		}
	}
	if headings[0].at == headings[1].at {
		t.Fatal("同名标题应保留独立源码位置")
	}
}

func TestFormatMenuReachesBlockCommands(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	for _, name := range []string{"heading2", "ordered", "table", "codeblock", "hr"} {
		app.command(name)
	}
	got := app.active().editor.(*stubEditor).formats
	if strings.Join(got, ",") != "heading2,ordered,table,codeblock,hr" {
		t.Fatalf("块级命令未到达编辑器：%v", got)
	}
}

func TestExportDocumentIsStandaloneHTML(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "# 山中 <来信>\n\n正文 ==高亮==\n")
	name, body, ok := app.exportBody(false)
	if !ok {
		t.Fatal("没有可导出的文档")
	}
	html := exportDocument("山中<来信>.md", body, false)
	for _, want := range []string{"<!doctype html>", "<title>山中&lt;来信&gt;</title>", "<mark>高亮</mark>", "@media print"} {
		if !strings.Contains(html, want) {
			t.Fatalf("导出文档缺少 %q", want)
		}
	}
	if exportName(name, ".pdf") != "欢迎.pdf" || exportName("a/b\\笔记.markdown", ".html") != "笔记.html" || exportName("", ".html") != "未命名.html" {
		t.Fatalf("导出文件名建议错误：%q", exportName(name, ".pdf"))
	}
}

func TestSourceFindSelectsAndReplaceUndoes(t *testing.T) {
	s := newSourceEditor("# 标题\n\n叶脉定位，再次叶脉定位。\n", 14)
	if !s.Find("叶脉定位") {
		t.Fatal("第一次查找未命中")
	}
	start, end := s.Selection()
	if string([]rune(s.Text())[start:end]) != "叶脉定位" {
		t.Fatalf("查找没有选中：%d-%d", start, end)
	}
	if !s.Find("叶脉定位") {
		t.Fatal("第二次查找未命中下一处")
	}
	start, _ = s.Selection()
	if start == 0 {
		t.Fatal("连续查找应离开第一处")
	}
	if s.Find("没有这个词") {
		t.Fatal("不存在的词不应命中")
	}

	s = newSourceEditor("甲甲甲\n", 14)
	if n := s.Replace("甲", "乙", false); n != 1 || s.Text() != "乙甲甲\n" {
		t.Fatalf("单次替换错误：%d %q", n, s.Text())
	}
	if n := s.Replace("甲", "乙", true); n != 2 || s.Text() != "乙乙乙\n" {
		t.Fatalf("全部替换错误：%d %q", n, s.Text())
	}
	s.Undo()
	if s.Text() != "乙甲甲\n" {
		t.Fatalf("撤销全部替换失败：%q", s.Text())
	}
	s.Undo()
	if s.Text() != "甲甲甲\n" {
		t.Fatalf("撤销单次替换失败：%q", s.Text())
	}
	s.Redo()
	if s.Text() != "乙甲甲\n" {
		t.Fatalf("重做失败：%q", s.Text())
	}

	fenced := "```\n甲\n甲\n```\n"
	s = newSourceEditor(fenced, 14)
	if n := s.Replace("甲", "乙", true); n != 2 || s.Markdown() != "```\n乙\n乙\n```\n" {
		t.Fatalf("围栏内全部替换错误：%d %q", n, s.Markdown())
	}
	s.Undo()
	if s.Markdown() != fenced {
		t.Fatalf("撤销未精确恢复围栏与末尾换行：%q", s.Markdown())
	}

	s = newSourceEditor("正文", 14)
	s.InsertLink("名", "https://example.com")
	s.InsertImage("图", "a.png")
	if s.Markdown() != "正文\n[名](https://example.com)\n![图](a.png)\n" {
		t.Fatalf("无尾换行时插入粘连了正文：%q", s.Markdown())
	}

	kept := s.Markdown()
	s.SetReadOnly(true)
	if n := s.Replace("名", "丙", true); n != 0 || s.Markdown() != kept {
		t.Fatalf("只读仍被替换：%d %q", n, s.Markdown())
	}
	s.InsertLink("另一个", "https://example.com")
	if s.Markdown() != kept {
		t.Fatalf("只读仍被插入：%q", s.Markdown())
	}
	s.Undo()
	if s.Markdown() != kept {
		t.Fatalf("只读仍被撤销：%q", s.Markdown())
	}
}

func TestSourceHighlightKinds(t *testing.T) {
	text := "# 标题\n\n**强调** 与 [链接](https://example.com) `代码` $x$。\n\n```\n# 不是标题\n```\n"
	spans := sourceHighlight(text)
	got := map[sourceKind]bool{}
	for _, sp := range spans {
		got[sp.kind] = true
		if sp.end <= sp.start {
			t.Fatalf("空区间：%+v", sp)
		}
	}
	for _, kind := range []sourceKind{sourceHeading, sourceMark, sourceLink, sourceCode, sourceMath} {
		if !got[kind] {
			t.Fatalf("缺少着色种类 %d：%+v", kind, spans)
		}
	}
	runes := []rune(text)
	for _, sp := range spans {
		if sp.kind == sourceCode && string(runes[sp.start:sp.end]) == "# 不是标题" {
			t.Fatal("围栏代码里的标题标记不应单独着色")
		}
	}
	// 不同标记不能互相关闭；四个空格不是围栏，后面的标题仍要着色。
	mixed := "```\n# 仍是代码\n~~~\n# 代码外\n"
	code := sourceHighlight(mixed)
	if len(code) == 0 || code[0].kind != sourceCode || code[0].end != len([]rune(mixed)) {
		t.Fatalf("~~~ 不应关闭 ```：%+v", code)
	}
	indented := "    ```\n# 不是围栏\n"
	sawHeading := false
	for _, sp := range sourceHighlight(indented) {
		if sp.kind == sourceCode && sp.start == 0 {
			t.Fatal("四个空格后不应成为围栏")
		}
		if sp.kind == sourceHeading {
			sawHeading = true
		}
	}
	if !sawHeading {
		t.Fatal("缩进过深的记号之后，标题应照常着色")
	}
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时：%s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWorkspaceCreateAndRenameThroughPrompts(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	root := filepath.Join(dir, "笔记")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	backend, err := app.workspace.backend()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.SelectFolder(root); err != nil {
		t.Fatal(err)
	}
	app.folderName = "笔记"

	app.askWorkspacePath("folder", "")
	if !app.promptOpen || app.promptTitle != "新建文件夹" {
		t.Fatalf("未弹出新建文件夹输入框：%q", app.promptTitle)
	}
	app.promptOK("草稿")
	waitFor(t, "创建文件夹", func() bool { _, err := os.Stat(filepath.Join(root, "草稿")); return err == nil })

	app.askWorkspacePath("file", "草稿")
	if app.promptText != "草稿/" {
		t.Fatalf("上级目录未带入输入框：%q", app.promptText)
	}
	app.promptOK("草稿/第一篇")
	created := filepath.Join(root, "草稿", "第一篇.md")
	waitFor(t, "创建文档", func() bool { _, err := os.Stat(created); return err == nil })

	// 没有窗口时后台结果不会回到界面，这里直接核对重命名的守卫与结果。
	node := workspace.Node{Name: "第一篇.md", Path: "草稿/第一篇.md"}
	app.promptOpen = false
	app.askRename(node)
	if !app.promptOpen || app.promptText != "第一篇.md" {
		t.Fatalf("未弹出重命名输入框：%q", app.promptText)
	}
	app.promptOK("a/b.md")
	if app.notice != "名称不能包含路径分隔符" {
		t.Fatalf("应拒绝带路径的名称：%q", app.notice)
	}
	app.promptOK("第二篇.md")
	waitFor(t, "重命名", func() bool { _, err := os.Stat(filepath.Join(root, "草稿", "第二篇.md")); return err == nil })
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatal("旧文件名仍然存在")
	}
}

func TestRenameRefusedWhileDocumentIsOpen(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	root := filepath.Join(dir, "笔记")
	if err := os.MkdirAll(filepath.Join(root, "夹"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "夹", "已打开.md")
	if err := os.WriteFile(path, []byte("正文\n"), 0644); err != nil {
		t.Fatal(err)
	}
	backend, _ := app.workspace.backend()
	if _, err := backend.SelectFolder(root); err != nil {
		t.Fatal(err)
	}
	opened, err := backend.OpenDocument("夹/已打开.md")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := app.files.store.LoadSnapshot(opened.Path, opened.Raw)
	if err != nil {
		t.Fatal(err)
	}
	app.adopt(doc, doc.Content)
	for _, node := range []workspace.Node{{Name: "已打开.md", Path: "夹/已打开.md"}, {Name: "夹", Path: "夹", Directory: true}} {
		app.notice, app.promptOpen = "", false
		app.askRename(node)
		if app.promptOpen || app.notice != "请先关闭相关标签再重命名" {
			t.Fatalf("已打开的文档或其文件夹不应可重命名：%+v", node)
		}
	}
}

func TestExportEmbedsRenderedMathAndDiagrams(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	source := "行内 $s = vt$ 继续\n\n$$\nQ_n = Q \\cdot r^n\n$$\n\n```mermaid\ngraph LR\nA-->B\n```\n\n见脚注[^a]\n\n[^a]: 注一\n"
	app.adopt(app.files.Current(), source)
	_, body, ok := app.exportBody(false)
	if !ok {
		t.Fatal("没有可导出的文档")
	}
	html := exportDocument("笔记.md", body, false)
	for _, want := range []string{
		`<span class="math" style="vertical-align:`,
		`<div class="math-display"><svg`,
		`<div class="diagram"><svg`,
		`class="footnotes"`,
		`<sup>1</sup>`,
		`href="#fn-a"`,
		`id="fn-a"`,
		`href="#fnref-a"`,
		`break-inside: avoid`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("导出缺少 %q：\n%s", want, html)
		}
	}
	// 排版结果替换源码，且导出文件不依赖外部资源。
	if strings.Contains(html, "s = vt") || strings.Contains(html, "<script") {
		t.Fatalf("导出仍含源码或脚本：\n%s", html)
	}
	if strings.Contains(html, "graph LR") {
		t.Fatalf("图表排版后仍留下源码：\n%s", html)
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		// SVG 命名空间是内嵌标记，不是外部资源。
		rest := strings.ReplaceAll(html, "http://www.w3.org/2000/svg", "")
		if strings.Contains(rest, "http://") || strings.Contains(rest, "https://") {
			t.Fatalf("导出引用了外部资源：\n%s", html)
		}
	}
}

func TestExportFollowsEffectiveTheme(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文 $s = vt$\n\n```mermaid\ngraph LR\nA-->B\n```\n")
	for _, tc := range []struct {
		theme                string
		systemDark, wantDark bool
	}{
		{"light", true, false}, {"dark", false, true},
		{"system", false, false}, {"system", true, true},
	} {
		t.Run(fmt.Sprintf("%s-%v", tc.theme, tc.systemDark), func(t *testing.T) {
			app.settings.Theme, app.effectiveDark = tc.theme, tc.systemDark
			dark := app.exportDark()
			if dark != tc.wantDark {
				t.Fatalf("导出主题错误：得到 %v，期望 %v", dark, tc.wantDark)
			}
			name, body, ok := app.exportBody(dark)
			if !ok {
				t.Fatal("没有可导出的正文")
			}
			html := exportDocument(name, body, dark)
			if strings.Contains(html, `<html lang="zh-CN" class="dark">`) != tc.wantDark {
				t.Fatal("文档主题未随当前外观变化")
			}
			fill := `fill="#f3e8dd"`
			if dark {
				fill = `fill="#343b44"`
			}
			if !strings.Contains(body, fill) || !strings.Contains(body, "currentColor") {
				t.Fatal("图表或公式未使用正文主题配色")
			}
			if strings.Contains(html, "background: #fff") || !strings.Contains(html, "print-color-adjust: exact") {
				t.Fatal("打印样式必须保留主题配色")
			}
		})
	}
}

func TestExportKeepsSourceWhenLayoutFails(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	// \unknown 不是受支持的公式命令，排版失败时应退回源码。
	app.adopt(app.files.Current(), "行内 $\\unknown$ 结束\n")
	_, body, ok := app.exportBody(false)
	if !ok {
		t.Fatal("没有可导出的文档")
	}
	if !strings.Contains(body, `<span class="math">\unknown</span>`) || strings.Contains(body, "<svg") {
		t.Fatalf("失败的公式应保留源码：\n%s", body)
	}
}
