//go:build desktoptest

package desktop

import (
	"strings"
	"testing"
)

type readingEditor struct {
	stubEditor
	readOnly bool
}

func (e *readingEditor) SetReadOnly(value bool) { e.readOnly = value }
func (e *readingEditor) Replace(query, replacement string, all bool) int {
	count := strings.Count(e.text, query)
	if !all && count > 1 {
		count = 1
	}
	e.text = strings.Replace(e.text, query, replacement, count)
	e.changed = count > 0
	return count
}

func TestReadingModeBlocksEditsAndKeepsFind(t *testing.T) {
	app, _ := newTestApp(t)
	app.adopt(app.files.Current(), "正文 正文")
	editor := &readingEditor{stubEditor: stubEditor{text: "正文 正文", findHit: true}}
	app.active().editor = editor
	app.command("reading")
	if !app.reading || !editor.readOnly {
		t.Fatal("阅读模式未同步至编辑器")
	}
	for _, command := range []string{"bold", "undo", "redo", "link", "image", "task"} {
		app.command(command)
	}
	if len(editor.formats) != 0 || editor.undone != 0 || editor.redone != 0 || app.promptOpen {
		t.Fatal("阅读模式执行了内容修改动作")
	}
	app.findQuery, app.replaceText = "正文", "替换"
	app.runFind()
	if len(editor.finds) != 1 || app.findNote != "已定位" {
		t.Fatal("阅读模式应保留查找")
	}
	app.runReplace(true)
	if editor.text != "正文 正文" {
		t.Fatal("阅读模式执行了替换")
	}
	app.command("reading")
	app.runReplace(false)
	if editor.text != "替换 正文" || app.findNote != "已替换 1 处" {
		t.Fatalf("单处替换失败：%q %q", editor.text, app.findNote)
	}
	app.runReplace(true)
	if editor.text != "替换 替换" {
		t.Fatalf("全部替换失败：%q", editor.text)
	}
}

func TestNewEditorInheritsReadingMode(t *testing.T) {
	app, _ := newTestApp(t)
	app.reading = true
	editor := &readingEditor{}
	app.configureEditor(editor)
	if !editor.readOnly {
		t.Fatal("新标签必须继承阅读模式")
	}
}
