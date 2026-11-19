//go:build desktoptest

package desktop

import (
	"testing"
	"unicode/utf8"

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

func TestSourceReplaceAndRuneOffsets(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		query       string
		replacement string
		all         bool
		wantCount   int
		wantText    string
	}{
		{"空查找词不替换", "正文", "", "新", true, 0, "正文"},
		{"未命中原样返回", "山中来信", "叶脉", "笔记", true, 0, "山中来信"},
		{"单次替换只改首处", "中文🌿测试中文🌿", "中文🌿", "甲", false, 1, "甲测试中文🌿"},
		{"全部替换含多字节与Emoji", "中文🌿测试中文🌿尾", "中文🌿", "乙✨", true, 2, "乙✨测试乙✨尾"},
		{"重叠模式非重叠计数偶数", "aaaa", "aa", "b", true, 2, "bb"},
		{"重叠模式非重叠计数奇数", "aaa", "aa", "b", true, 1, "ba"},
		{"替换词包含查找词不死循环", "a-a", "a", "aa", true, 2, "aa-aa"},
		{"替换为空串相当于删除", "前缀[删]中段[删]后缀", "[删]", "", true, 2, "前缀中段后缀"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count, got := sourceReplace(tc.text, tc.query, tc.replacement, tc.all)
			if count != tc.wantCount || got != tc.wantText {
				t.Fatalf("got (%d, %q), want (%d, %q)", count, got, tc.wantCount, tc.wantText)
			}
		})
	}

	// 验证源码编辑器在含中文与 Emoji 时查找选区的 rune 起止偏移，以及全部替换后可一步撤销。
	initial := "标题🌿\n\n第一句：目标词。\n第二句：目标词。\n"
	s := newSourceEditor(initial, 14)
	if !s.Find("目标词") {
		t.Fatal("应查找到第一处目标词")
	}
	start, end := s.Selection()
	wantStart := utf8.RuneCountInString("标题🌿\n\n第一句：")
	wantEnd := wantStart + utf8.RuneCountInString("目标词")
	if start != wantStart || end != wantEnd {
		t.Fatalf("首个匹配 rune 偏移不符：got %d-%d, want %d-%d", start, end, wantStart, wantEnd)
	}
	if n := s.Replace("目标词", "新词✨", true); n != 2 {
		t.Fatalf("全文替换次数不符：%d", n)
	}
	if want := "标题🌿\n\n第一句：新词✨。\n第二句：新词✨。\n"; s.Markdown() != want {
		t.Fatalf("全文替换结果不符：%q", s.Markdown())
	}
	s.Undo()
	if s.Markdown() != initial {
		t.Fatalf("全文替换应一步撤销回原文：%q", s.Markdown())
	}
}
