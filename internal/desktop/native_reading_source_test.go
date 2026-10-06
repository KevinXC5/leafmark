//go:build desktoptest

package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSourceToggleInheritsReadOnly(t *testing.T) {
	app, _ := newTestApp(t)
	const markdown = "# 只读切换\n\n正文\n"
	app.adopt(app.files.Current(), markdown)
	app.reading = true
	app.toggleSource()
	source, ok := app.active().editor.(*sourceEditor)
	if !ok {
		t.Fatal("未切换到源码模式")
	}
	source.SetSelection(0, 0)
	source.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "不能写入"})
	if source.Markdown() != markdown || source.Replace("正文", "修改", true) != 0 {
		t.Fatal("源码切换没有继承阅读模式")
	}
	app.command("reading")
	source.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "恢复"})
	if source.Markdown() == markdown {
		t.Fatal("退出阅读模式后仍不能编辑源码")
	}
	source.Undo()
	if source.Markdown() != markdown {
		t.Fatal("源码编辑撤销未恢复原文")
	}
}
