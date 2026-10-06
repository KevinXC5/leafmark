package nativeeditor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 源码行的文字，按排版顺序。
func sourceLineTexts(e *Editor) []string {
	out := make([]string, 0, len(e.lay.lines))
	for _, ln := range e.lay.lines {
		out = append(out, ln.text)
	}
	return out
}

func TestSourceKindTableKeepsFirstMatch(t *testing.T) {
	spans := []SourceSpan{{Start: 2, End: 6, Kind: 1}, {Start: 4, End: 9, Kind: 3}, {Start: -3, End: 1, Kind: 2}, {Start: 11, End: 40, Kind: 4}}
	const length = 12
	kinds := sourceKindTable(spans, length)
	if len(kinds) != length {
		t.Fatalf("着色表长度 %d，应为 %d", len(kinds), length)
	}
	for at := 0; at < length; at++ {
		want := uint8(0)
		for _, sp := range spans {
			if at >= sp.Start && at < sp.End {
				want = sp.Kind
				break
			}
		}
		if kinds[at] != want {
			t.Fatalf("偏移 %d 的种类 %d，应为 %d", at, kinds[at], want)
		}
	}
	if sourceKindTable(nil, length) != nil || sourceKindTable(spans, 0) != nil {
		t.Fatal("没有区间或空文档不应建表")
	}
}

func TestSourceLayoutReusedUntilInputsChange(t *testing.T) {
	source := "# 标题\n\n" + strings.Repeat("一行很长的源码 ", 40) + "\n末行\n"
	ed := NewPlainText(source)
	calls := 0
	ed.SetSourceSyntax(func(text string) []SourceSpan {
		calls++
		return []SourceSpan{{Start: 0, End: 4, Kind: 1}}
	})
	view := ui.NewTester(ed.View, 960, 700)
	view.Frame()
	first := sourceLineTexts(ed)
	if strings.Join(first, "") != strings.ReplaceAll(source, "\n", "") {
		t.Fatalf("折行丢失或重复了正文：%q", first)
	}
	if len(first) <= strings.Count(source, "\n")+1 {
		t.Fatalf("超宽的行应被折开，实际 %d 行", len(first))
	}
	laid := calls
	if laid == 0 {
		t.Fatal("首帧应调用着色回调")
	}
	for range 3 {
		view.Frame()
	}
	if calls != laid {
		t.Fatalf("正文未变的帧不应重新着色排版：%d → %d", laid, calls)
	}

	// 编辑后必须按新正文重排。
	ed.SetSelection(0, 0)
	ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "新"})
	view.Frame()
	if calls == laid {
		t.Fatal("编辑后没有重新排版")
	}
	if got := sourceLineTexts(ed); got[0] != "新# 标题" {
		t.Fatalf("编辑后首行未更新：%q", got[0])
	}
	if ed.Markdown() != "新"+source {
		t.Fatalf("源码回写不一致：%q", ed.Markdown())
	}

	// 栏宽变化要重新折行，字号变化也是。
	laid, wide := calls, len(ed.lay.lines)
	view.SetSize(420, 700)
	view.Frame()
	if calls == laid || len(ed.lay.lines) <= wide {
		t.Fatalf("变窄后应折出更多行：%d → %d", wide, len(ed.lay.lines))
	}
	laid = calls
	ed.FontSize(20)
	view.Frame()
	if calls == laid {
		t.Fatal("字号变化后没有重新排版")
	}
	laid = calls
	ed.SetSourceSyntax(func(string) []SourceSpan { calls++; return nil })
	view.Frame()
	if calls == laid {
		t.Fatal("更换着色回调后没有重新排版")
	}
}

func TestFitRunesWholeAndWrapped(t *testing.T) {
	cache := newShapeCache()
	rs := []rune("alpha beta gamma delta epsilon zeta eta theta")
	seg := styled{text: string(rs), code: true}
	full := measure(cache, string(rs), 14, false, false, true)
	if got := fitRunes(cache, seg, rs, full, 14, false); got != len(rs) {
		t.Fatalf("刚好放得下应整段返回，实际 %d", got)
	}
	got := fitRunes(cache, seg, rs, full/2, 14, false)
	if got <= 0 || got >= len(rs) {
		t.Fatalf("半宽应折在中间，实际 %d", got)
	}
	// 行尾空白悬在栏外，不计入宽度。
	if measure(cache, strings.TrimRight(string(rs[:got]), " "), 14, false, false, true) > full/2 {
		t.Fatalf("折出的前缀超宽：%q", string(rs[:got]))
	}
	if rs[got-1] != ' ' {
		t.Fatalf("应在空白处断开：%q", string(rs[:got]))
	}
}

func TestShapeCacheSurvivesGenerationTurnover(t *testing.T) {
	cache := newShapeCache()
	want := cache.shape("保留", 14, false, false, true)
	cache.aged, cache.text = cache.text, map[shapeKey][]ui.Glyph{}
	got := cache.shape("保留", 14, false, false, true)
	if len(got) != len(want) || len(cache.text) != 1 {
		t.Fatalf("上一代的条目应搬回当前代：%d 个字形，当前代 %d 条", len(got), len(cache.text))
	}
	got[0].X += 100
	if again := cache.shape("保留", 14, false, false, true); again[0].X != want[0].X {
		t.Fatal("返回的字形应是副本，不能改到缓存")
	}
	cache.reset()
	if len(cache.text)+len(cache.aged) != 0 {
		t.Fatal("reset 应清空两代")
	}
}

// 视口外的行不提交字形，滚到文末后视口里的行仍要画出来。
func TestSourcePaintsVisibleLinesAfterScroll(t *testing.T) {
	const needle = "末尾匹配"
	ed := NewPlainText(strings.Repeat("源码长文，用来撑出很多屏。\n", 400) + needle + "\n")
	view := ui.NewTester(ed.View, 900, 600)
	view.Frame()
	inked := func() int {
		img := view.Image()
		bg := img.RGBAAt(img.Bounds().Max.X-2, img.Bounds().Max.Y/2)
		n := 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				if img.RGBAAt(x, y) != bg {
					n++
				}
			}
		}
		return n
	}
	top := inked()
	if top == 0 {
		t.Fatal("文首没有画出文字")
	}
	if !ed.Find(needle) {
		t.Fatal("未找到匹配")
	}
	view.Frame()
	view.Frame()
	if _, y := ed.Scroll(); y < 1000 {
		t.Fatalf("查找后应滚到文末，实际 %v", y)
	}
	if end := inked(); end < top/2 {
		t.Fatalf("滚到文末后视口里的文字没有画出来：文首 %d 像素，文末 %d 像素", top, end)
	}
}
