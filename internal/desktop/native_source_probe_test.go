//go:build desktoptest

package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSourceReadOnlyRejectsTyping(t *testing.T) {
	s := newSourceEditor("# 只读\n\n正文\n", 14)
	s.SetReadOnly(true)
	if s.Replace("正文", "改写", false) != 0 || s.Markdown() != "# 只读\n\n正文\n" {
		t.Fatalf("只读替换应被拒绝：%q", s.Markdown())
	}
	if s.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "键入"}) {
		t.Fatal("只读不应接收键入")
	}
	if s.Markdown() != "# 只读\n\n正文\n" {
		t.Fatalf("只读后原文变化：%q", s.Markdown())
	}
	if !s.Find("正文") {
		t.Fatal("只读仍应能查找")
	}
	start, end := s.Selection()
	if string([]rune(s.Text())[start:end]) != "正文" {
		t.Fatalf("只读查找未选中：%d-%d", start, end)
	}
}

func TestSourceSyntaxKeepsMarkers(t *testing.T) {
	text := "# 标题\n\n**强调** 与 [链接](https://example.com) `代码` $x$。\n\n```\n# 不是标题\n```\n"
	s := newSourceEditor(text, 14)
	view := ui.NewTester(s.View, 800, 500)
	view.Frame()
	if s.Markdown() != text {
		t.Fatalf("源码应原样保留标记和末尾换行：%q", s.Markdown())
	}
	spans := sourceSyntax(s.Markdown())
	got := map[uint8]bool{}
	for _, sp := range spans {
		got[sp.Kind] = true
		if sp.End <= sp.Start {
			t.Fatalf("空着色区间：%+v", sp)
		}
	}
	for _, kind := range []sourceKind{sourceHeading, sourceMark, sourceLink, sourceCode, sourceMath} {
		if !got[uint8(kind)] {
			t.Fatalf("着色回调缺少种类 %d：%+v", kind, spans)
		}
	}
}

func TestSourceDefaultFontSize(t *testing.T) {
	s := newSourceEditor("", 0)
	if s.size != 14 {
		t.Fatalf("空字号应回落到 14，实际 %v", s.size)
	}
	s.FontSize(0)
	if s.size != 14 {
		t.Fatalf("再次设置空字号应保持 14，实际 %v", s.size)
	}
	s.FontSize(16)
	if s.size != 16 {
		t.Fatalf("正文字号应记下 16，实际 %v", s.size)
	}
}
