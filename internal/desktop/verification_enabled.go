//go:build verification

package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/nativeeditor"
	"leafmark/internal/workspace"
)

var verificationPath string

// 验收仅访问独立副本和独立设置，不能覆盖用户文档或恢复草稿。
func prepareVerification(files *Files) {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	data := filepath.Join(root, ".verification-data", "native")
	if err = os.MkdirAll(data, 0700); err != nil {
		panic(err)
	}
	if err = os.Setenv("LEAFMARK_NATIVE_DATA_DIR", data); err != nil {
		panic(err)
	}
	_ = os.Remove(filepath.Join(data, "native-recovery.json"))
	_ = os.Remove(filepath.Join(data, "native-settings.json"))
	backend, err := workspace.NewAt(data)
	if err != nil {
		panic(err)
	}
	files.workspace.once.Do(func() { files.workspace.store = backend })
	if err = os.MkdirAll(filepath.Join(root, "verification"), 0755); err != nil {
		panic(err)
	}
	if _, err = backend.SelectFolder(filepath.Join(root, "verification")); err != nil {
		panic(err)
	}
	verificationPath = filepath.Join(root, "verification", "本机读写验证.md")
	raw, err := os.ReadFile(filepath.Join(root, "verification", "输入验证.md"))
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(verificationPath, raw, 0600); err != nil {
		panic(err)
	}
	if mode := siteShotMode(); mode != "" {
		verificationPath, err = prepareSiteShot(root, mode)
		if err != nil {
			panic(err)
		}
	}
	if _, err = files.store.Load(verificationPath); err != nil {
		panic(err)
	}
	mygo.App.SetName("Leafmark 原生验证")
}

// 真实窗口由 ui.View 绘制；输入通过与 HandleInput 相同的入口处理，截图来自原生 Content。
func startNativeVerification(win *mygo.Window, app *nativeApp) {
	mygo.App.Focus()
	win.Show()
	win.Focus()
	// 独立超时守卫也覆盖主线程异常或初始化卡住的情况。
	go func() {
		time.Sleep(60 * time.Second)
		fmt.Fprintln(os.Stderr, "原生验证超时")
		mygo.App.Exit(1)
	}()
	go func() {
		defer func() {
			if p := recover(); p != nil {
				finishNativeVerification(map[string]any{}, fmt.Errorf("验收异常：%v", p))
			}
		}()
		if mode := siteShotMode(); mode != "" {
			if err := captureNativeSiteShots(win, app, mode); err != nil {
				finishNativeVerification(map[string]any{}, err)
				return
			}
			mygo.App.Exit(0)
			return
		}
		results := map[string]any{"platform": nativeVerificationPlatform(), "arch": runtime.GOARCH, "passed": false, "renderer": "MyGo Native UI", "inputMethod": "组合事件自动化；真实系统输入法需人工验收"}
		deadline := time.Now().Add(35 * time.Second)
		for {
			ready := false
			mygo.RunOnMain(func() {
				ready = app.active() != nil && app.active().editor != nil && app.active().id == app.files.Current().ID
			})
			if ready {
				break
			}
			if time.Now().After(deadline) {
				finishNativeVerification(results, fmt.Errorf("原生窗口初始化超时"))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		var err error
		mygo.RunOnMain(func() {
			tab := app.active()
			ed := tab.editor
			original := ed.Markdown()
			if !strings.Contains(ed.Text(), "最小功能验证") {
				err = fmt.Errorf("初始文档未正确加载")
				return
			}
			results["initialRendering"] = true
			// 在正文段落中输入，避免将格式断言落在文末的等宽代码块里。
			at := len([]rune(strings.SplitN(ed.Text(), "\n", 2)[0])) + 1
			ed.SetSelection(at, at)
			ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "中文输入验证 🌿 "})
			inserted := ed.Markdown()
			if !strings.Contains(inserted, "中文输入验证 🌿") {
				err = fmt.Errorf("中文与 emoji 插入失败")
				return
			}
			ed.Undo()
			if ed.Markdown() != original {
				err = fmt.Errorf("撤销未恢复原文")
				return
			}
			ed.Redo()
			if ed.Markdown() != inserted {
				err = fmt.Errorf("重做未恢复编辑")
				return
			}
			results["editing"] = true
			if !ed.Find("中文输入验证") {
				err = fmt.Errorf("查找失败")
				return
			}
			ed.Format("bold")
			if !strings.Contains(ed.Markdown(), "**中文输入验证**") {
				err = fmt.Errorf("加粗未生成正确 Markdown")
				return
			}
			results["formatting"] = true
			beforeCompose := ed.Markdown()
			ed.SetSelection(len([]rune(ed.Text())), len([]rune(ed.Text())))
			ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputCompose, Text: "中文组字", Caret: 2})
			if ed.Markdown() != beforeCompose {
				err = fmt.Errorf("未提交的组合输入写入了文档")
				return
			}
			ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputCompose, Text: ""})
			app.syncAll()
			results["composition"] = true
			app.settings.AutoSave = false
			app.command("save")
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		for {
			done := false
			mygo.RunOnMain(func() { done = !app.saving[app.files.Current().ID] && !app.files.Current().Dirty })
			if done {
				break
			}
			if time.Now().After(deadline) {
				finishNativeVerification(results, fmt.Errorf("真实磁盘保存超时"))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		raw, err := os.ReadFile(verificationPath)
		expected := ""
		mygo.RunOnMain(func() { expected = app.active().editor.Markdown() })
		originalRaw, readErr := os.ReadFile(filepath.Join("verification", "输入验证.md"))
		if readErr != nil {
			finishNativeVerification(results, readErr)
			return
		}
		if strings.Contains(string(originalRaw), "\r\n") {
			expected = strings.ReplaceAll(expected, "\n", "\r\n")
		}
		if strings.HasPrefix(string(originalRaw), string(rune(0xfeff))) {
			expected = string(rune(0xfeff)) + expected
		}
		if err != nil || string(raw) != expected || !strings.Contains(string(raw), "**中文输入验证**") || !strings.Contains(string(raw), "🌿") {
			finishNativeVerification(results, fmt.Errorf("磁盘内容不匹配：%v", err))
			return
		}
		results["diskSave"] = true
		mygo.RunOnMain(func() {
			firstID := app.active().id
			first := app.active().editor.Markdown()
			app.command("new")
			second := app.active()
			second.editor.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "独立草稿 🌿"})
			app.syncAll()
			for i, tab := range app.tabs {
				if tab.id == firstID {
					app.selectTab(i)
					break
				}
			}
			if app.active().editor.Markdown() != first {
				err = fmt.Errorf("切换标签丢失正文")
				return
			}
			for i, tab := range app.tabs {
				if tab.id == second.id {
					app.selectTab(i)
					break
				}
			}
			if !strings.Contains(app.active().editor.Text(), "独立草稿") {
				err = fmt.Errorf("切换标签丢失草稿")
				return
			}
			app.finishClose([]string{second.id})
			for i, tab := range app.tabs {
				if tab.id == firstID {
					app.selectTab(i)
					break
				}
			}
			results["tabs"] = true
			_, err = app.files.CheckExternal(firstID)
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		// 格式栏的每个按钮经同一条命令通道作用在真实编辑器上，逐个核对回写的 Markdown。
		mygo.RunOnMain(func() {
			const scratch = "甲乙丙丁\n"
			doc := app.files.store.New("格式验证.md", scratch)
			app.adopt(doc, doc.Content)
			tab := app.active()
			ed, ok := tab.editor.(*nativeeditor.Editor)
			if !ok {
				err = fmt.Errorf("格式验证没有使用原生编辑器")
				return
			}
			checked := 0
			for _, item := range nativeFormatBar {
				want := map[string]string{
					"bold": "甲**乙丙**丁", "italic": "甲*乙丙*丁", "strike": "甲~~乙丙~~丁", "code": "甲`乙丙`丁",
					"heading1": "# 甲乙丙丁", "heading2": "## 甲乙丙丁", "heading3": "### 甲乙丙丁",
					"bullet": "- 甲乙丙丁", "ordered": "1. 甲乙丙丁", "task": "- [ ] 甲乙丙丁", "quote": "> 甲乙丙丁",
					"table": "| --- | --- |", "codeblock": "```", "hr": "---",
				}[item.id]
				if want == "" {
					continue // 链接与图片会打开输入框或系统对话框，不在自动验收里。
				}
				ed.SetSelection(1, 3)
				app.command(item.id)
				if got := ed.Markdown(); !strings.Contains(got, want) {
					err = fmt.Errorf("格式栏“%s”未生效：%q", item.label, got)
					return
				}
				for i := 0; i < 3 && ed.Markdown() != scratch; i++ {
					ed.Undo()
				}
				if ed.Markdown() != scratch {
					err = fmt.Errorf("格式栏“%s”撤销后未恢复原文：%q", item.label, ed.Markdown())
					return
				}
				checked++
			}
			// 源码模式与原位编辑互换，正文逐字节不变。
			app.command("source")
			if !sourceMode(app.active()) || app.active().editor.Markdown() != scratch {
				err = fmt.Errorf("切换到源码模式后正文变化")
				return
			}
			app.command("source")
			if sourceMode(app.active()) || app.active().editor.Markdown() != scratch {
				err = fmt.Errorf("切回原位编辑后正文变化")
				return
			}
			results["formatBarCommands"] = checked
			results["sourceMode"] = true
			app.finishClose([]string{tab.id})
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		// 完整示例只读复制，用真实窗口核对标题、正文、表格与代码的视觉。
		sample, err := os.ReadFile(filepath.Join("tests", "fixtures", "samples", "山中来信.md"))
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		mygo.RunOnMain(func() {
			doc := app.files.store.New("山中来信.md", string(sample))
			app.adopt(doc, doc.Content)
			app.settings.Theme = "light"
			app.settings.FontSize = 16
			app.applySettings()
		})
		win.Invalidate()
		refreshVerificationWindow(win)
		time.Sleep(250 * time.Millisecond)
		// 截图会同步构建原生 Content 帧，也为后台窗口提供确定的布局观察点。
		if diagnostic, captureErr := win.CapturePage(); captureErr == nil {
			_ = os.WriteFile("verification/native-layout-diagnostic.png", diagnostic, 0644)
		}
		mygo.RunOnMain(func() {
			ed, ok := app.active().editor.(*nativeeditor.Editor)
			if !ok {
				err = fmt.Errorf("窗口没有使用原生编辑器")
				return
			}
			layout := ed.LayoutSnapshot()
			if layout.Width < 200 || layout.ViewHeight < 100 || len(layout.Lines) < 3 {
				err = fmt.Errorf("正文布局为空或尺寸异常：%+v", layout)
				return
			}
			if strings.HasPrefix(layout.Lines[0].Text, "#") {
				err = fmt.Errorf("标题仍显示 Markdown 标记")
				return
			}
			if layout.Lines[0].Height <= layout.Lines[1].Height {
				err = fmt.Errorf("标题与正文没有字号层次")
				return
			}
			results["layout"] = map[string]any{"width": layout.Width, "viewportHeight": layout.ViewHeight, "lines": len(layout.Lines), "headingHeight": layout.Lines[0].Height}
			ed.SetSelection(len([]rune(ed.Text())), len([]rune(ed.Text())))
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		win.Invalidate()
		refreshVerificationWindow(win)
		time.Sleep(150 * time.Millisecond)
		mygo.RunOnMain(func() {
			ed := app.active().editor.(*nativeeditor.Editor)
			layout := ed.LayoutSnapshot()
			// 光标在文末：它必须落在视口内，无论是否需要滚动。
			_, caretY, caretH := ed.PointForOffset(len([]rune(ed.Text())))
			if caretY-layout.ScrollY < 0 || caretY+caretH-layout.ScrollY > layout.ViewHeight {
				err = fmt.Errorf("长文档光标未滚动到可见位置：y=%v 滚动=%v 视口=%v", caretY, layout.ScrollY, layout.ViewHeight)
				return
			}
			results["scroll"] = true
			// 示例里的行内公式、嵌套列表与任务项都应排成正文，不落成原文占位。
			text := ed.Text()
			for _, want := range []string{"s = vt", "还有两张信笺、半包新茶", "把昨晚的想法誊到纸上"} {
				if !strings.Contains(text, want) {
					err = fmt.Errorf("示例内容未排成正文：%s", want)
					return
				}
			}
			if strings.Contains(text, "$") {
				err = fmt.Errorf("行内公式仍显示美元符号")
				return
			}
			results["inlineContent"] = true
			start := strings.Index(text, "把窗推开")
			from := len([]rune(text[:start]))
			ed.SetSelection(from, from+4)
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		win.Invalidate()
		refreshVerificationWindow(win)
		time.Sleep(150 * time.Millisecond)
		mygo.RunOnMain(func() {
			ed := app.active().editor.(*nativeeditor.Editor)
			if _, _, _, _, shown := ed.SelectionAnchor(); !shown {
				err = fmt.Errorf("选中文字后格式栏没有锚点")
				return
			}
			results["formatBar"] = true
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		if shot, captureErr := win.CapturePage(); captureErr == nil {
			_ = os.WriteFile("verification/native-format-bar.png", shot, 0644)
			results["formatBarScreenshot"] = "verification/native-format-bar.png"
		}
		mygo.RunOnMain(func() { app.active().editor.SetSelection(0, 0) })
		// 设置页逐个分类截图，核对与设计一致。
		for _, tab := range settingsTabs {
			mygo.RunOnMain(func() { app.settingsOn, app.settingsSection = true, tab.id })
			win.Invalidate()
			refreshVerificationWindow(win)
			time.Sleep(150 * time.Millisecond)
			if shot, captureErr := win.CapturePage(); captureErr == nil {
				_ = os.WriteFile("verification/native-settings-"+tab.id+".png", shot, 0644)
			}
		}
		mygo.RunOnMain(func() { app.settingsOn, app.settingsSection = false, "" })
		results["settingsPages"] = len(settingsTabs)
		if err := verifyNativeUpdateFlow(win, app); err != nil {
			finishNativeVerification(results, err)
			return
		}
		results["updateCallbacks"] = true
		// 更新窗口：用一条虚构的新版本截取下载中与安装完成两种状态，随后复原。
		for _, state := range []string{"downloading", "installed"} {
			mygo.RunOnMain(func() {
				app.updates.mu.Lock()
				app.updates.pending = &mygo.Update{Version: "9.9.9", Notes: "验收用的更新日志，概述一句。\n\n## 新增功能\n\n- 支持**公式排版**与流程图。\n- 新增源码模式。\n\n## 问题修复\n\n- 修复若干问题。\n"}
				app.updates.installed = state == "installed"
				app.updates.mu.Unlock()
				app.updateOpen, app.updateDownloading = true, state == "downloading"
				app.updateDownloaded, app.updateTotal = 45, 100
			})
			win.Invalidate()
			refreshVerificationWindow(win)
			time.Sleep(150 * time.Millisecond)
			if shot, captureErr := win.CapturePage(); captureErr == nil {
				_ = os.WriteFile("verification/native-update-"+state+".png", shot, 0644)
			}
		}
		mygo.RunOnMain(func() {
			app.updates.mu.Lock()
			app.updates.pending, app.updates.installed = nil, false
			app.updates.mu.Unlock()
			app.updateOpen, app.updateDownloading, app.updateDownloaded, app.updateTotal = false, false, 0, 0
		})
		results["updateDialog"] = true
		// 导出走与菜单相同的生成路径，只是把结果写进验证目录而不弹保存框。
		var exportName, exportBody string
		mygo.RunOnMain(func() { exportName, exportBody, _ = app.exportBody() })
		exported := exportDocument(exportName, exportBody)
		if !strings.Contains(exported, "<h1>山中来信</h1>") || !strings.Contains(exported, "<table>") {
			finishNativeVerification(results, fmt.Errorf("导出的 HTML 缺少标题或表格"))
			return
		}
		_ = os.WriteFile("verification/native-export.html", []byte(exported), 0644)
		pdf, pdfErr := renderPDF(exported)
		if pdfErr != nil || !strings.HasPrefix(string(pdf[:min(len(pdf), 5)]), "%PDF-") {
			finishNativeVerification(results, fmt.Errorf("导出 PDF 失败：%v", pdfErr))
			return
		}
		_ = os.WriteFile("verification/native-export.pdf", pdf, 0644)
		results["exportHTML"] = true
		results["exportPDFBytes"] = len(pdf)
		// 在真实窗口验证源码文本域贴右边，以及标题定位不修改原文。
		mygo.RunOnMain(func() { app.command("source") })
		// Windows 的窗口刷新是异步的，等待源码视图真正完成布局再读取边界。
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			refreshVerificationWindow(win)
			time.Sleep(40 * time.Millisecond)
			ready := false
			mygo.RunOnMain(func() { ready = app.active().editor.(*sourceEditor).bounds.W > 0 })
			if ready {
				break
			}
		}
		mygo.RunOnMain(func() {
			source := app.active().editor.(*sourceEditor)
			w, _ := win.ContentSize()
			if source.bounds.X+source.bounds.W < float32(w)-1 {
				err = fmt.Errorf("源码滚动区域未贴到窗口右侧：%+v，窗口宽度 %v", source.bounds, w)
				return
			}
			headings := nativeSourceOutline(source.text)
			if len(headings) < 2 {
				err = fmt.Errorf("示例文档缺少大纲标题")
				return
			}
			source.SetSelection(headings[len(headings)-1].at, headings[len(headings)-1].at)
		})
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			refreshVerificationWindow(win)
			time.Sleep(40 * time.Millisecond)
			ready := false
			mygo.RunOnMain(func() {
				source := app.active().editor.(*sourceEditor)
				ready = !source.jump && source.scroll.Y > 0
			})
			if ready {
				break
			}
		}
		mygo.RunOnMain(func() {
			source := app.active().editor.(*sourceEditor)
			if source.jump || source.scroll.Y <= 0 || source.Changed() {
				err = fmt.Errorf("源码大纲未完成滚动或修改了原文")
			}
			source.SetSelection(0, 0)
		})
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		results["sourceOutlineScroll"] = true
		results["sourceScrollRightEdge"] = true
		// 源码模式截图，随后切回原位编辑。
		win.Invalidate()
		refreshVerificationWindow(win)
		time.Sleep(200 * time.Millisecond)
		if shot, captureErr := win.CapturePage(); captureErr == nil {
			_ = os.WriteFile("verification/native-source-mode.png", shot, 0644)
		}
		mygo.RunOnMain(func() { app.command("source"); app.active().editor.SetSelection(0, 0) })
		win.Invalidate()
		refreshVerificationWindow(win)
		time.Sleep(150 * time.Millisecond)
		png, err := win.CapturePage()
		if err != nil {
			finishNativeVerification(results, err)
			return
		}
		if err = os.WriteFile("verification/native-window.png", png, 0644); err != nil {
			finishNativeVerification(results, err)
			return
		}
		results["screenshot"] = "verification/native-window.png"
		results["passed"] = true
		finishNativeVerification(results, nil)
	}()
}

func nativeVerificationPlatform() string {
	if runtime.GOOS == "darwin" {
		return "macOS Native UI"
	}
	if runtime.GOOS == "windows" {
		return "Windows Native UI"
	}
	return runtime.GOOS + " Native UI"
}

func finishNativeVerification(results map[string]any, err error) {
	results["platform"] = nativeVerificationPlatform()
	results["arch"] = runtime.GOARCH
	if err != nil {
		results["passed"] = false
		results["error"] = err.Error()
	}
	raw, marshalErr := json.MarshalIndent(results, "", "  ")
	if marshalErr == nil {
		marshalErr = os.WriteFile("verification/native-results.json", raw, 0644)
	}
	if err != nil || marshalErr != nil {
		fmt.Fprintln(os.Stderr, "原生验证失败：", err, marshalErr)
		mygo.App.Exit(1)
		return
	}
	fmt.Println("通过：MyGo Native UI 排版、中文与 emoji 编辑、撤销重做、组合输入、格式、保存和多标签")
	mygo.App.Exit(0)
}
