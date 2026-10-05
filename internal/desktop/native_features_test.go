//go:build desktoptest

package desktop

import (
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
	source.text += "追加\n"
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
	if source.bounds.X+source.bounds.W < 1499 {
		t.Fatalf("源码文本域未贴到编辑区右侧：%+v", source.bounds)
	}
	// 通过大纲按钮验证实际滚动，而不只检查定位请求。
	if err := view.Click("大纲：目标章节"); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		view.Frame()
	}
	if source.scroll.Y <= 1000 || source.jump {
		t.Fatalf("大纲未滚动到目标章节：滚动=%v，待跳转=%v", source.scroll.Y, source.jump)
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
	if source.scroll.Y != 0 {
		t.Fatalf("首个标题未回到顶部：%v", source.scroll.Y)
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
	name, body, ok := app.exportBody()
	if !ok {
		t.Fatal("没有可导出的文档")
	}
	html := exportDocument("山中<来信>.md", body)
	for _, want := range []string{"<!doctype html>", "<title>山中&lt;来信&gt;</title>", "<mark>高亮</mark>", "@media print"} {
		if !strings.Contains(html, want) {
			t.Fatalf("导出文档缺少 %q", want)
		}
	}
	if exportName(name, ".pdf") != "欢迎.pdf" || exportName("a/b\\笔记.markdown", ".html") != "笔记.html" || exportName("", ".html") != "未命名.html" {
		t.Fatalf("导出文件名建议错误：%q", exportName(name, ".pdf"))
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
