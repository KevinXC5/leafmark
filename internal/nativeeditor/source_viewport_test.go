package nativeeditor

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestSourceFindRemainsVisibleAfterViewportResize(t *testing.T) {
	const needle = "末尾匹配"
	source := strings.Repeat("源码长文。\n", 80) + needle + "\n"
	ed := NewPlainText(source)
	view := ui.NewTester(ed.View, 960, 900)
	view.Frame()
	if !ed.Find(needle) {
		t.Fatal("未找到匹配")
	}
	view.Frame()
	view.SetSize(764, 560)
	view.Frame()
	view.Frame()
	at, _ := ed.Selection()
	_, y, h := ed.PointForOffset(at)
	_, scroll := ed.Scroll()
	if y < scroll-1 || y+h > scroll+ed.Bounds().H+1 {
		t.Fatalf("缩小视口后匹配不可见：光标%v，滚动%v，视口%+v", y, scroll, ed.Bounds())
	}
	if ed.Markdown() != source || ed.Changed() {
		t.Fatal("窗口缩放改变了源码")
	}
}
