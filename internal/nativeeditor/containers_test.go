package nativeeditor

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// offsetOf 返回 needle 在可见文本中的 rune 偏移。
func offsetOf(t *testing.T, ed *Editor, needle string) int {
	t.Helper()
	at := strings.Index(ed.Text(), needle)
	if at < 0 {
		t.Fatalf("文本 %q 中找不到 %q", ed.Text(), needle)
	}
	return len([]rune(ed.Text()[:at]))
}

// blockOf 返回 needle 所在块的排版盒。
func blockOf(t *testing.T, ed *Editor, needle string) blockBox {
	t.Helper()
	return ed.lay.blocks[ed.doc.BlockIndexAt(offsetOf(t, ed, needle))]
}

func framesOf(ed *Editor, kind richtext.Kind) []frame {
	var out []frame
	for _, f := range ed.lay.frames {
		if f.kind == kind && f.foot == "" {
			out = append(out, f)
		}
	}
	return out
}

func TestListItemBodyAlignsUnderItemText(t *testing.T) {
	source := "- 甲项\n\n  续段正文\n\n  ```go\n  x := 1\n  ```\n- 乙项\n  1. 子一\n  2. 子二\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	first, body, code := blockOf(t, ed, "甲项"), blockOf(t, ed, "续段正文"), blockOf(t, ed, "x := 1")
	if first.kind != richtext.List || !first.bullet {
		t.Fatalf("列表项首块应带圆点：%+v", first)
	}
	textX, _, _ := ed.PointForOffset(offsetOf(t, ed, "甲项"))
	bodyX, _, _ := ed.PointForOffset(offsetOf(t, ed, "续段正文"))
	if abs32(textX-bodyX) > .01 || abs32(first.x+12-textX) > .01 {
		t.Fatalf("续段应对齐到项内正文左缘：首段 %v 续段 %v 块 %v", textX, bodyX, first.x)
	}
	if body.kind != richtext.Paragraph || code.kind != richtext.Code || abs32(code.x-bodyX) > .01 {
		t.Fatalf("项内代码块应与续段左对齐：%+v %+v", body, code)
	}
	if code.x+code.w > first.x+first.w+.01 {
		t.Fatalf("项内代码块超出正文栏：%+v", code)
	}
	second, child := blockOf(t, ed, "乙项"), blockOf(t, ed, "子一")
	if got := second.y - (code.y + code.h); abs32(got-looseGap) > .01 {
		t.Fatalf("多段列表项与下一项的间距 = %v", got)
	}
	if child.y != second.y+second.h || abs32(child.x-second.x-nestIndent) > .01 {
		t.Fatalf("子列表应紧挨父项并缩进一层：父 %+v 子 %+v", second, child)
	}
	if got := ed.lay.lines[child.lines[0]].markers; len(got) == 0 {
		t.Fatal("有序子项缺少编号")
	}
	if ed.Markdown() != source {
		t.Fatalf("排版改变了原文：%q", ed.Markdown())
	}
}

func TestListItemWithoutLeadingTextDrawsMarker(t *testing.T) {
	ed, tt := newTest(t, "- ```sh\n  ls\n  ```\n- [ ] 任务\n\n  说明\n")
	tt.Frame()
	markers := framesOf(ed, richtext.List)
	if len(markers) != 1 || !markers[0].marker || !markers[0].bullet {
		t.Fatalf("以代码块开头的列表项应自己画圆点：%+v", ed.lay.frames)
	}
	code := blockOf(t, ed, "ls")
	if abs32(code.x-markers[0].x-12) > .01 || markers[0].lineY < code.y {
		t.Fatalf("圆点应位于代码块左侧首行：%+v %+v", markers[0], code)
	}
	task, note := blockOf(t, ed, "任务"), blockOf(t, ed, "说明")
	if task.kind != richtext.Task || abs32(note.x-task.x-28) > .01 {
		t.Fatalf("任务说明应对齐到任务文字：%+v %+v", task, note)
	}
}

func TestNestedQuoteAndCalloutFrames(t *testing.T) {
	source := "> 外层\n>\n> > 内层\n>\n> 收尾\n\n> [!note] 标题\n> 第一段\n>\n> 第二段\n>\n> - 项"
	ed, tt := newTest(t, source)
	tt.Frame()
	quotes := framesOf(ed, richtext.Quote)
	if len(quotes) != 2 {
		t.Fatalf("应有内外两层引用装饰：%+v", ed.lay.frames)
	}
	outer, inner := quotes[0], quotes[1]
	head, nested, tail := blockOf(t, ed, "外层"), blockOf(t, ed, "内层"), blockOf(t, ed, "收尾")
	if abs32(inner.x-outer.x-quoteInset) > .01 || abs32(head.x-outer.x-quoteInset) > .01 || abs32(nested.x-inner.x-quoteInset) > .01 {
		t.Fatalf("引用缩进不符：外 %v 内 %v 正文 %v %v", outer.x, inner.x, head.x, nested.x)
	}
	if head.y != outer.y+10 || abs32(outer.y+outer.h-(tail.y+tail.h)-10) > .01 {
		t.Fatalf("外层引用上下留白应为 10：%+v 首 %+v 尾 %+v", outer, head, tail)
	}
	if inner.y <= head.y+head.h || inner.y+inner.h >= tail.y || nested.y != inner.y+10 {
		t.Fatalf("内层引用应夹在外层两段之间：%+v", inner)
	}
	if !head.muted || !nested.muted {
		t.Fatal("引用里的正文应使用次要色")
	}
	callouts := framesOf(ed, richtext.Callout)
	if len(callouts) != 1 || len(callouts[0].title) == 0 {
		t.Fatalf("提示块缺少底色或标题：%+v", ed.lay.frames)
	}
	box := callouts[0]
	p1, p2, item := blockOf(t, ed, "第一段"), blockOf(t, ed, "第二段"), blockOf(t, ed, "项")
	if abs32(p1.x-box.x-37) > .01 || p1.y != box.y+35 || p2.y != p1.y+p1.h+innerGap {
		t.Fatalf("提示块正文位置不符：框 %+v 段 %+v %+v", box, p1, p2)
	}
	if item.kind != richtext.List || abs32(item.x-p1.x) > .01 || abs32(box.y+box.h-(item.y+item.h)-12) > .01 {
		t.Fatalf("提示块里的列表项位置不符：%+v 框 %+v", item, box)
	}
	if got := box.y - (outer.y + outer.h); got != blockGap {
		t.Fatalf("引用与提示块之间应保留块间距：%v", got)
	}
	if ed.Markdown() != source {
		t.Fatalf("排版改变了原文：%q", ed.Markdown())
	}
}

func TestContainerOffsetsHitAndVerticalMove(t *testing.T) {
	ed, tt := newTest(t, "- 甲项\n\n  续段\n- 乙项\n  1. 子一\n\n> 外层\n>\n> > 内层\n\n> [!tip] 标题\n> 提示\n>\n> 再提示\n\n尾注[^a]\n\n[^a]: 定义\n\n    再定义\n")
	tt.SetSize(640, 1200)
	tt.Frame()
	text := []rune(ed.Text())
	footnote := offsetOf(t, ed, "a")
	for i, r := range text {
		if r == '\n' || i == footnote {
			continue
		}
		x, y, h := ed.PointForOffset(i)
		tt.ClickAt(x, y+h/4)
		if a, b := ed.Selection(); a != i || b != i {
			t.Fatalf("容器内偏移 %d（%q）命中 %d", i, string(r), a)
		}
	}
	// 上下移动逐行经过每个容器，不跳块也不卡住。
	ed.SetSelection(0, 0)
	tt.Frame()
	seen := map[int]bool{}
	for range len(ed.lay.lines) + 2 {
		at, _ := ed.Selection()
		seen[ed.doc.BlockIndexAt(at)] = true
		tt.Key(0, ui.KeyDown)
	}
	if at, _ := ed.Selection(); at != len(text) || len(seen) != len(ed.lay.blocks) {
		t.Fatalf("向下移动应经过全部 %d 块并到达文末：经过 %d 块，停在 %d/%d", len(ed.lay.blocks), len(seen), at, len(text))
	}
	// 跨容器选区画在各自的行上，删除后其余内容仍在。
	ed.SetSelection(offsetOf(t, ed, "续段"), offsetOf(t, ed, "乙项")+1)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if got := ed.Text(); strings.Contains(got, "续段") || !strings.Contains(got, "甲项") || !strings.Contains(got, "项\n子一") {
		t.Fatalf("跨容器删除后文本 = %q", got)
	}
	ed.Undo()
	if !strings.Contains(ed.Text(), "续段\n乙项") {
		t.Fatalf("撤销后文本 = %q", ed.Text())
	}
}

func TestContainerSplitAndUndo(t *testing.T) {
	source := "> 外层\n>\n> > 内层引用\n\n- 甲项\n\n  续段正文\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	for _, needle := range []string{"层引用", "段正文"} {
		at := offsetOf(t, ed, needle)
		before := ed.doc.Blocks()[ed.doc.BlockIndexAt(at)]
		ed.SetSelection(at, at)
		tt.Frame()
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		blocks := ed.doc.Blocks()
		bi := ed.doc.BlockIndexAt(offsetOf(t, ed, needle))
		if got, want := len(blocks[bi].Containers), len(before.Containers); got != want {
			t.Fatalf("在 %q 前回车后容器层数 %d，原为 %d", needle, got, want)
		}
		head, tail := ed.lay.blocks[bi-1], ed.lay.blocks[bi]
		if abs32(head.x-tail.x) > .01 || tail.y <= head.y {
			t.Fatalf("拆出的段落应留在同一容器并左对齐：%+v %+v", head, tail)
		}
		ed.Undo()
		tt.Frame()
		if ed.Markdown() != source {
			t.Fatalf("撤销拆分后 Markdown = %q", ed.Markdown())
		}
		ed.Redo()
		ed.Undo()
		if ed.Markdown() != source {
			t.Fatalf("重做再撤销后 Markdown = %q", ed.Markdown())
		}
	}
}

func TestFootnoteReferenceAndDefinitionJump(t *testing.T) {
	source := "正文[^note]和另一处[^b]。\n\n[^note]: 定义一\n\n    第二段\n[^b]: 定义二\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	var refs []paintSpan
	for _, ln := range ed.lay.lines {
		for _, span := range ln.spans {
			if span.footnote != "" {
				refs = append(refs, span)
			}
		}
	}
	if len(refs) != 2 || refs[0].dy >= 0 || len(refs[0].display) != 1 || len(refs[1].display) != 1 {
		t.Fatalf("脚注引用应排成上标编号：%+v", refs)
	}
	var defs []frame
	for _, f := range ed.lay.frames {
		if f.foot != "" {
			defs = append(defs, f)
		}
	}
	if len(defs) != 2 || defs[0].foot != "note" || !defs[0].rule || defs[1].rule || len(defs[0].label) == 0 || defs[0].backRect.W <= 0 {
		t.Fatalf("脚注定义区应有编号、分隔线和回跳：%+v", defs)
	}
	first, second := blockOf(t, ed, "定义一"), blockOf(t, ed, "第二段")
	if first.x <= defs[0].x || abs32(first.x-second.x) > .01 {
		t.Fatalf("定义正文应排在编号右侧并对齐：%+v %+v", first, second)
	}
	// 单击引用跳到定义，指针在引用上是小手。
	ref := offsetOf(t, ed, "note")
	x, y, h := ed.PointForOffset(ref)
	tt.Move(x+2, y+h/2)
	if tt.Cursor() != ui.CursorPointer {
		t.Fatalf("脚注引用上的指针 = %v", tt.Cursor())
	}
	tt.ClickAt(x+2, y+h/2)
	if at, _ := ed.Selection(); at != offsetOf(t, ed, "定义一") {
		t.Fatalf("点击引用后光标 = %d", at)
	}
	// 回跳箭头和编号都跳回引用。
	back := defs[0].backRect
	tt.Move(back.X+back.W/2, back.Y+back.H/2)
	if tt.Cursor() != ui.CursorPointer {
		t.Fatalf("回跳箭头上的指针 = %v", tt.Cursor())
	}
	tt.ClickAt(back.X+back.W/2, back.Y+back.H/2)
	if at, _ := ed.Selection(); at != ref {
		t.Fatalf("点击回跳后光标 = %d，引用在 %d", at, ref)
	}
	tt.ClickAt(defs[1].x+defs[1].labelW/2, defs[1].lineY+defs[1].lineH/2)
	if at, _ := ed.Selection(); at != offsetOf(t, ed, "b。") {
		t.Fatalf("点击编号后光标 = %d", at)
	}
	if ed.Markdown() != source || ed.Changed() {
		t.Fatal("脚注跳转不应改动文档")
	}
}

func TestPointerShapeFollowsHotContent(t *testing.T) {
	ed, tt := newTest(t, "正文 [链接](https://example.com) 结尾\n\n- [ ] 任务\n\n$$x^2$$\n")
	ed.SetEditRaw(func(int, string) {})
	tt.Frame()
	at := func(needle string) (float32, float32) {
		x, y, h := ed.PointForOffset(offsetOf(t, ed, needle))
		return x + 3, y + h/2
	}
	tx, ty := at("正文")
	lx, ly := at("链接")
	task := blockOf(t, ed, "任务")
	cx, cy := task.x+10, task.y+task.h/2
	math := ed.lay.blocks[len(ed.lay.blocks)-1]
	mx, my := math.x+math.w/2, math.y+math.h/2
	for _, tc := range []struct {
		name string
		x, y float32
		want ui.Cursor
	}{{"正文", tx, ty, ui.CursorText}, {"编辑模式的链接", lx, ly, ui.CursorText}, {"任务复选框", cx, cy, ui.CursorPointer}, {"任务文字", cx + 40, cy, ui.CursorText}, {"公式块", mx, my, ui.CursorPointer}} {
		tt.Move(tc.x, tc.y)
		if got := tt.Cursor(); got != tc.want {
			t.Fatalf("%s上的指针 = %v，应为 %v", tc.name, got, tc.want)
		}
	}
	// 按住 Mod 时链接才显示小手；松开后恢复文本指针。
	tt.Move(tx, ty)
	if !ed.updatePointer(lx, ly, ui.Cmd) || !ed.hand {
		t.Fatal("按住 Mod 悬停链接应显示小手")
	}
	if !ed.updatePointer(lx, ly, 0) || ed.hand {
		t.Fatal("松开 Mod 后链接应恢复文本指针")
	}
	ed.SetReadOnly(true)
	for _, tc := range []struct {
		name string
		x, y float32
		want ui.Cursor
	}{{"阅读模式的链接", lx, ly, ui.CursorPointer}, {"阅读模式的正文", tx, ty, ui.CursorText}, {"阅读模式的复选框", cx, cy, ui.CursorText}, {"阅读模式的公式块", mx, my, ui.CursorText}} {
		tt.Move(tc.x, tc.y)
		if got := tt.Cursor(); got != tc.want {
			t.Fatalf("%s上的指针 = %v，应为 %v", tc.name, got, tc.want)
		}
	}
	// 从链接上按下并拖选时不保留小手。
	tt.Press(lx, ly)
	tt.Move(lx+60, ly)
	if a, b := ed.Selection(); a == b || ed.hand {
		t.Fatalf("拖选应产生选区并恢复文本指针：%d,%d hand=%v", a, b, ed.hand)
	}
	tt.Release(lx+60, ly)
}

func TestLinkOpensByModeAndModifier(t *testing.T) {
	ed, tt := newTest(t, "正文 [链接](https://example.com/a) 和 [邮件](mailto:a@example.com) 和 [本地](notes/a.md) 和 [电话](tel:10086)")
	var opened []string
	ed.SetOpenLink(func(url string) { opened = append(opened, url) })
	tt.Frame()
	at := func(needle string) (float32, float32) {
		x, y, h := ed.PointForOffset(offsetOf(t, ed, needle))
		return x + 3, y + h/2
	}
	lx, ly := at("链接")
	tt.ClickAt(lx, ly)
	if a, b := ed.Selection(); len(opened) != 0 || a != b || a != offsetOf(t, ed, "链接") {
		t.Fatalf("编辑模式普通单击应只放置光标：opened=%v sel=%d,%d", opened, a, b)
	}
	tt.ClickAtWith(ui.Cmd, lx, ly)
	if len(opened) != 1 || opened[0] != "https://example.com/a" {
		t.Fatalf("Mod+单击应打开链接：%v", opened)
	}
	ed.SetReadOnly(true)
	mx, my := at("邮件")
	tt.ClickAt(mx, my)
	if len(opened) != 2 || opened[1] != "mailto:a@example.com" {
		t.Fatalf("阅读模式单击应打开链接：%v", opened)
	}
	for _, needle := range []string{"本地", "电话", "正文"} {
		x, y := at(needle)
		tt.ClickAt(x, y)
	}
	if len(opened) != 2 {
		t.Fatalf("只应打开 http、https、mailto：%v", opened)
	}
	// 从链接上拖出选区不算单击。
	tt.Press(lx, ly)
	tt.Move(lx+80, ly)
	tt.Release(lx+80, ly)
	if a, b := ed.Selection(); len(opened) != 2 || a == b {
		t.Fatalf("拖选不应打开链接：opened=%v sel=%d,%d", opened, a, b)
	}
	// 没有设置回调时交给界面库打开。
	ed.SetOpenLink(nil)
	tt.ClickAt(lx, ly)
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://example.com/a" {
		t.Fatalf("默认应由界面库打开：%v", got)
	}
}

func TestDiagramNodeLinkIsClickable(t *testing.T) {
	ed, tt := newTest(t, "```mermaid\nflowchart LR\nA[官网] --> B[其他]\nclick A \"https://example.com\"\n```")
	var opened []string
	ed.SetOpenLink(func(url string) { opened = append(opened, url) })
	tt.Frame()
	box := ed.lay.blocks[0]
	if box.diagram == nil {
		t.Fatal("流程图未排版")
	}
	dx, dy := box.x+max(0, (box.w-box.diagram.Width)/2), box.y+8
	var hx, hy float32
	found := false
	for y := float32(2); y < box.diagram.Height && !found; y += 4 {
		for x := float32(2); x < box.diagram.Width; x += 4 {
			if h, ok := box.diagram.HitTest(x, y); ok && h.URL != "" {
				hx, hy, found = dx+x, dy+y, true
				break
			}
		}
	}
	if !found {
		t.Fatal("流程图没有可点击节点")
	}
	if h := ed.hotAt(hx, hy); h.kind != hotLink || h.value != "https://example.com" {
		t.Fatalf("节点命中 = %+v", h)
	}
	tt.ClickAt(hx, hy)
	if len(opened) != 0 {
		t.Fatalf("编辑模式普通单击不应打开节点链接：%v", opened)
	}
	tt.ClickAtWith(ui.Cmd, hx, hy)
	// 指针不动，切到阅读模式后下一帧就换成小手。
	ed.SetReadOnly(true)
	tt.Frame()
	if tt.Cursor() != ui.CursorPointer {
		t.Fatalf("阅读模式下可点击节点的指针 = %v", tt.Cursor())
	}
	tt.ClickAt(hx, hy)
	if len(opened) != 2 || opened[0] != "https://example.com" || opened[1] != opened[0] {
		t.Fatalf("节点链接打开记录 = %v", opened)
	}
}

func TestEveryDiagramKindRendersInPlace(t *testing.T) {
	for _, body := range []string{
		"sequenceDiagram\nA->>B: 你好\nB-->>A: 收到",
		"pie title 占比\n\"甲\" : 40\n\"乙\" : 60",
		"gantt\ntitle 计划\ndateFormat YYYY-MM-DD\nsection 阶段\n任务一 :a1, 2026-01-01, 3d",
		"classDiagram\nclass Animal\nAnimal <|-- Dog",
		"stateDiagram-v2\n[*] --> 运行\n运行 --> [*]",
		"erDiagram\nCUSTOMER ||--o{ ORDER : places",
	} {
		source := "```mermaid\n" + body + "\n```"
		ed, tt := newTest(t, source)
		tt.Frame()
		box := ed.lay.blocks[0]
		if box.diagram == nil || box.h != box.diagram.Height+16 || ed.lay.lines[0].text != "" {
			t.Fatalf("图表应按排版结果绘制而不是占位卡片：%q %+v", body, box)
		}
		if ed.Markdown() != source {
			t.Fatalf("图表改变了原文：%q", ed.Markdown())
		}
	}
}

func TestReadOnlyBlocksEditsButKeepsSelectionAndCopy(t *testing.T) {
	source := "甲乙丙丁\n\n- [ ] 任务\n\n$$x^2$$\n"
	ed, tt := newTest(t, source)
	edits := 0
	ed.SetEditRaw(func(int, string) { edits++ })
	ed.SetReadOnly(true)
	tt.Frame()
	if !ed.ReadOnly() {
		t.Fatal("应处于阅读模式")
	}
	ed.SetSelection(1, 3)
	tt.Frame()
	tt.SetClipboard("粘贴内容")
	tt.Type("字")
	tt.Compose("ni", 2)
	tt.Key(0, ui.KeyBackspace)
	tt.Key(0, ui.KeyDelete)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyTab)
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Key(ui.Cmd, ui.KeyX)
	tt.Key(ui.Cmd, ui.KeyB)
	tt.Command("paste")
	tt.Command("cut")
	tt.Command("delete")
	ed.Format("bold")
	ed.Format("heading1")
	ed.Format("table")
	ed.InsertLink("链接", "https://example.com")
	ed.InsertImage("图", "a.png")
	ed.Undo()
	ed.Redo()
	if n := ed.Replace("甲", "乙", true); n != 0 {
		t.Fatalf("阅读模式不应替换：%d", n)
	}
	if ed.ReplaceRaw(len(ed.doc.Blocks())-1, "正文") {
		t.Fatal("阅读模式不应替换原文块")
	}
	task := blockOf(t, ed, "任务")
	tt.ClickAt(task.x+10, task.y+task.h/2)
	math := ed.lay.blocks[len(ed.lay.blocks)-1]
	tt.ClickAt(math.x+math.w/2, math.y+math.h/2)
	tt.ClickAt(math.x+math.w/2, math.y+math.h/2)
	if ed.Markdown() != source || ed.Changed() || edits != 0 || ed.compose != "" {
		t.Fatalf("阅读模式下文档被改动：%q changed edits=%d compose=%q", ed.Markdown(), edits, ed.compose)
	}
	// 选择、复制、查找和移动仍然可用。
	ed.SetSelection(1, 3)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "乙丙" {
		t.Fatalf("阅读模式复制 = %q", tt.Clipboard())
	}
	tt.Key(ui.Shift, ui.KeyRight)
	if a, b := ed.Selection(); a != 1 || b != 4 {
		t.Fatalf("阅读模式扩展选区 = %d,%d", a, b)
	}
	if !ed.Find("任务") {
		t.Fatal("阅读模式应能查找")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	if a, b := ed.Selection(); a != 0 || b != ed.doc.Len() {
		t.Fatalf("阅读模式全选 = %d,%d", a, b)
	}
	x, y, h := ed.PointForOffset(0)
	tt.Press(x, y+h/2)
	tt.Move(x+40, y+h/2)
	tt.Release(x+40, y+h/2)
	if a, b := ed.Selection(); a != 0 || b <= a {
		t.Fatalf("阅读模式拖选 = %d,%d", a, b)
	}
	tt.RightClickAt(x+10, y+h/2)
	if menu := tt.Menu(); len(menu) != 1 || menu[0] != "复制" {
		t.Fatalf("阅读模式右键菜单 = %v", menu)
	}
	tt.CloseMenu()
	// 退出阅读模式后恢复编辑。
	ed.SetReadOnly(false)
	ed.SetSelection(0, 0)
	tt.Frame()
	tt.Type("新")
	if !strings.HasPrefix(ed.Text(), "新甲") || !ed.Changed() {
		t.Fatalf("退出阅读模式后应能输入：%q", ed.Text())
	}
}

func TestReplaceOneAndAll(t *testing.T) {
	source := "甲乙甲\n\n- 甲项\n\n  续甲\n\n> 引用甲\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	if n := ed.Replace("", "丙", true); n != 0 || ed.Changed() {
		t.Fatalf("空查询不应替换：%d", n)
	}
	if n := ed.Replace("没有", "丙", true); n != 0 || ed.Markdown() != source {
		t.Fatalf("找不到时不应改动：%d %q", n, ed.Markdown())
	}
	// 单处替换：光标在文首时替换第一处，并选中下一处。
	ed.SetSelection(0, 0)
	if n := ed.Replace("甲", "丙丁", false); n != 1 || !strings.HasPrefix(ed.Text(), "丙丁乙甲") {
		t.Fatalf("单处替换 = %d %q", n, ed.Text())
	}
	if a, b := ed.Selection(); a != 3 || b != 4 {
		t.Fatalf("单处替换后应选中下一处：%d,%d", a, b)
	}
	if n := ed.Replace("甲", "丙丁", false); n != 1 || !strings.HasPrefix(ed.Text(), "丙丁乙丙丁\n") {
		t.Fatalf("替换选中的匹配 = %d %q", n, ed.Text())
	}
	ed.Undo()
	ed.Undo()
	if ed.Markdown() != source {
		t.Fatalf("两次撤销后 = %q", ed.Markdown())
	}
	// 全部替换覆盖各容器，并且是一步撤销。
	ed.Changed()
	if n := ed.Replace("甲", "丙丁", true); n != 5 {
		t.Fatalf("全部替换处数 = %d，文本 %q", n, ed.Text())
	}
	if got := ed.Text(); strings.Contains(got, "甲") || strings.Count(got, "丙丁") != 5 || !ed.Changed() {
		t.Fatalf("全部替换后文本 = %q", got)
	}
	if md := ed.Markdown(); !strings.Contains(md, "- 丙丁项") || !strings.Contains(md, "> 引用丙丁") {
		t.Fatalf("全部替换后 Markdown = %q", md)
	}
	ed.Undo()
	if ed.Markdown() != source {
		t.Fatalf("全部替换应一步撤销：%q", ed.Markdown())
	}
	ed.Redo()
	if strings.Contains(ed.Text(), "甲") {
		t.Fatalf("重做后文本 = %q", ed.Text())
	}
}

func TestReferenceLinkAndImageRenderAsOrdinaryContent(t *testing.T) {
	source := "见 [链接][ref] 和文字\n\n![图][img]\n\n[ref]: https://example.com \"标题\"\n[img]: a.png\n"
	ed, tt := newTest(t, source)
	var opened []string
	ed.SetOpenLink(func(url string) { opened = append(opened, url) })
	tt.Frame()
	if first := ed.lay.blocks[0]; first.raw || ed.lay.blocks[1].kind != richtext.Image {
		t.Fatalf("引用式链接与图片应正常排版：%+v", ed.lay.blocks)
	}
	x, y, h := ed.PointForOffset(offsetOf(t, ed, "链接"))
	if got := ed.hotAt(x+3, y+h/2); got.kind != hotLink || got.value != "https://example.com" {
		t.Fatalf("引用式链接命中 = %+v", got)
	}
	tt.ClickAtWith(ui.Cmd, x+3, y+h/2)
	if len(opened) != 1 {
		t.Fatalf("引用式链接应可打开：%v", opened)
	}
	for _, b := range ed.lay.blocks[2:] {
		if !b.raw || !strings.Contains(ed.lay.lines[b.lines[0]].text, "链接定义") {
			t.Fatalf("链接定义应标明类型：%+v", b)
		}
	}
	ed.SetSelection(offsetOf(t, ed, "和文字"), offsetOf(t, ed, "和文字"))
	tt.Frame()
	tt.Type("新")
	if md := ed.Markdown(); !strings.Contains(md, "新和文字") || !strings.Contains(md, "[链接]") || !strings.Contains(md, "[ref]: https://example.com \"标题\"") {
		t.Fatalf("编辑后链接与定义应仍在：%q", md)
	}
}

func TestIndentedCodeBlockIsEditable(t *testing.T) {
	ed, tt := newTest(t, "正文\n\n    缩进代码\n    第二行\n")
	tt.Frame()
	code := ed.lay.blocks[1]
	if code.kind != richtext.Code || code.raw || len(code.lines) != 2 {
		t.Fatalf("缩进代码块应排成代码块：%+v", code)
	}
	at := offsetOf(t, ed, "第二行")
	ed.SetSelection(at, at)
	tt.Frame()
	tt.Type("新")
	if !strings.Contains(ed.Text(), "新第二行") || !strings.Contains(ed.Markdown(), "新第二行") {
		t.Fatalf("缩进代码块应可编辑：%q", ed.Markdown())
	}
	ed.Undo()
	if ed.Markdown() != "正文\n\n    缩进代码\n    第二行\n" {
		t.Fatalf("撤销后 = %q", ed.Markdown())
	}
}

func TestUnderlineAndKbdStyles(t *testing.T) {
	styles := runsToStyled([]richtext.Run{{Text: "线", Marks: richtext.MarkUnderline}, {Text: "Ctrl", Marks: richtext.MarkKbd}, {Text: "文"}})
	if len(styles) != 3 || !styles[0].underline || styles[0].kbd || !styles[1].kbd || !styles[1].code || styles[2].underline || styles[2].kbd || styles[2].code {
		t.Fatalf("下划线与按键样式 = %+v", styles)
	}
}

func TestFindSkipsAtomicStructures(t *testing.T) {
	source := "甲见 $x^甲$ 尾[^note]。\n\n![甲图](a.png)\n\n[^note]: 甲的定义\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	ed.SetSelection(0, 0)
	if !ed.Find("甲") {
		t.Fatal("应找到正文里的甲")
	}
	if a, b := ed.Selection(); a != 0 || b != 1 {
		t.Fatalf("第一处应是正文：%d,%d 文本 %q", a, b, ed.Text())
	}
	// 公式里的甲、脚注标签和图片占位都跳过，下一处只能是定义正文。
	if !ed.Find("甲") {
		t.Fatal("应继续找到定义里的甲")
	}
	// 公式源码里的美元符号不进纯文本，定义紧接在图片占位之后。
	def := offsetOf(t, ed, "甲的定义")
	if a, b := ed.Selection(); a != def || b != def+1 {
		t.Fatalf("第二处应是定义：%d,%d，定义在 %d，文本 %q", a, b, def, ed.Text())
	}
	if ed.Find("甲") {
		a, b := ed.Selection()
		if a != 0 || b != 1 {
			t.Fatalf("绕回后应回到正文：%d,%d", a, b)
		}
	}
	// 单处替换同样跳过原子结构，只改定义里的甲。
	ed.SetSelection(0, 0)
	ed.Replace("甲", "乙", false)
	ed.Replace("甲", "乙", false)
	if got := ed.Text(); !strings.HasPrefix(got, "乙") || !strings.Contains(got, "乙的定义") || strings.Contains(got, "$x^乙$") {
		t.Fatalf("只应替换可编辑的甲：%q", got)
	}
	if strings.Contains(ed.Markdown(), "$x^乙$") || strings.Contains(ed.Markdown(), "[^乙]") {
		t.Fatalf("原子结构被改写：%q", ed.Markdown())
	}
}

func TestSourceModeKeepsMarkersAndColors(t *testing.T) {
	source := "# 标题\n\n```go\nfmt.Println(\"甲\")\n```\n\n见 **甲** 与 `代码`。\n\n"
	ed := NewPlainText(source)
	tt := ui.NewTester(ed.View, 640, 480)
	tt.Frame()
	if ed.Markdown() != source || ed.Text() != source {
		t.Fatalf("源码模式应原样返回全文：md=%q text=%q", ed.Markdown(), ed.Text())
	}
	if strings.Count(ed.Text(), "```") != 2 {
		t.Fatalf("正文里的围栏不应被当成代码块边界：%q", ed.Text())
	}
	if ed.lay.blocks[0].kind != richtext.Code || len(ed.lay.blocks) != 1 {
		t.Fatalf("源码应排成一块等宽文本：%d %+v", len(ed.lay.blocks), ed.lay.blocks[0].kind)
	}
	// 视口矩形来自滚动元素，宽度与测试窗口一致。
	if b := ed.Bounds(); b.W < 600 || b.H < 400 {
		t.Fatalf("Bounds 应是视口矩形：%+v", b)
	}
	var colors [6]ui.Color
	colors[1] = ui.RGB(10, 20, 30)
	colors[2] = ui.RGB(40, 50, 60)
	ed.SetSourcePalette(colors)
	var seen []string
	ed.SetSourceSyntax(func(text string) []SourceSpan {
		seen = append(seen, text)
		mark := utf8.RuneCountInString(text[:strings.Index(text, "**")])
		return []SourceSpan{{Start: 0, End: 1, Kind: 1}, {Start: mark, End: mark + 2, Kind: 2}}
	})
	tt.Frame()
	if len(seen) == 0 || seen[len(seen)-1] != source {
		t.Fatalf("着色回调应收到当前全文：%q", seen)
	}
	var heading, emphasis bool
	for _, ln := range ed.lay.lines {
		for _, sp := range ln.spans {
			if len(sp.glyphs) == 0 {
				continue
			}
			at := ln.origin + sp.glyphs[0].Cluster
			if sp.source == 1 && at == 0 {
				heading = true
			}
			if sp.source == 2 && at == utf8.RuneCountInString(source[:strings.Index(source, "**")]) {
				emphasis = true
			}
		}
	}
	if !heading || !emphasis {
		t.Fatalf("源码着色未落到对应字形：heading=%v emphasis=%v", heading, emphasis)
	}
	// 中间插入、删除后全文仍是纯源码，撤销回到原文。
	at := offsetOf(t, ed, "**甲**")
	x, y, h := ed.PointForOffset(at)
	tt.ClickAt(x+1, y+h/2)
	ed.SetSelection(at, at)
	before := len(seen)
	tt.Type("新")
	ed.Changed()
	tt.Frame()
	if len(seen) <= before || !strings.Contains(seen[len(seen)-1], "新**甲**") {
		t.Fatalf("编辑后着色回调应收到新全文：%q", seen)
	}
	if !strings.Contains(ed.Markdown(), "新**甲**") || strings.Count(ed.Markdown(), "```") != 2 {
		t.Fatalf("中间插入后 = %q", ed.Markdown())
	}
	tt.Key(0, ui.KeyBackspace)
	ed.Changed()
	if ed.Markdown() != source {
		t.Fatalf("删除后 = %q", ed.Markdown())
	}
	ed.Undo()
	ed.Changed()
	if !strings.Contains(ed.Markdown(), "新**甲**") {
		t.Fatalf("撤销删除后 = %q", ed.Markdown())
	}
	ed.Undo()
	ed.Changed()
	if ed.Markdown() != source {
		t.Fatalf("撤销插入后 = %q", ed.Markdown())
	}
	// 空文档与只有换行的文档保持原样。
	for _, text := range []string{"", "\n", "\n\n"} {
		blank := NewPlainText(text)
		if blank.Markdown() != text {
			t.Fatalf("纯文本 %q 被改成 %q", text, blank.Markdown())
		}
	}
	// 阅读模式不改源码，查找和选区仍可用。
	ed.SetReadOnly(true)
	ed.SetSelection(0, 0)
	tt.Frame()
	tt.Type("字")
	ed.Undo()
	if ed.Markdown() != source {
		t.Fatalf("阅读模式改了源码：%q", ed.Markdown())
	}
	if !ed.Find("代码") {
		t.Fatal("阅读模式应能查找源码")
	}
}

func TestSourceWrapsLongLine(t *testing.T) {
	line := "甲乙丙丁戊己庚辛壬癸"
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString(line)
	}
	b.WriteString("\n尾\n")
	source := b.String()
	ed := NewPlainText(source)
	ed.FontSize(16)
	ed.SetLineHeight(1.7)
	tt := ui.NewTester(ed.View, 320, 480)
	tt.Frame()
	if ed.Markdown() != source {
		t.Fatal("折行不应改纯文本")
	}
	var got strings.Builder
	lines := 0
	for _, ln := range ed.lay.lines {
		if ln.text == "尾" {
			break
		}
		lines++
		got.WriteString(ln.text)
	}
	if lines < 2 || got.String() != strings.Repeat(line, 40) {
		t.Fatalf("长行应折行且原文不丢字：行 %d 得到 %d rune", lines, len([]rune(got.String())))
	}
	// 字号与行距作用于源码行。
	h := ed.lay.lines[0].height
	if h < 16*1.7-1 || h > 16*1.7+8 {
		t.Fatalf("源码行高应跟随字号与行距：%v", h)
	}
	// 折行后的着色仍按全文偏移。
	ed.SetSourceSyntax(func(text string) []SourceSpan {
		return []SourceSpan{{Start: 0, End: 2, Kind: 1}}
	})
	tt.Frame()
	if len(ed.lay.lines) == 0 || len(ed.lay.lines[0].spans) == 0 || ed.lay.lines[0].spans[0].source != 1 {
		t.Fatal("折行后首段着色丢失")
	}
}

func TestInlineImageIsAtomicAndDefinitionHidden(t *testing.T) {
	source := "前 ![甲图](a.png) 后见 [链][ref]。\n\n[ref]: https://example.com\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	text := ed.Text()
	if strings.Contains(text, "ref") || strings.Contains(text, "https://") {
		t.Fatalf("链接定义不应进入正文：%q", text)
	}
	for _, b := range ed.lay.blocks {
		if b.kind == richtext.ReferenceDef {
			t.Fatal("隐藏定义不应占排版块")
		}
	}
	if !strings.Contains(ed.Markdown(), "[ref]: https://example.com") {
		t.Fatalf("定义应从 Markdown 回写：%q", ed.Markdown())
	}
	var image bool
	for _, ln := range ed.lay.lines {
		for _, sp := range ln.spans {
			if sp.image == "a.png" && len(sp.display) > 0 {
				image = true
			}
		}
	}
	if !image {
		t.Fatal("行内图片应以替代文字画出")
	}
	// 替代文字只用于绘制，正文里是一个占位符，查找和替换都进不去。
	ed.SetSelection(0, 0)
	if ed.Find("甲") || strings.Contains(ed.Text(), "甲") {
		t.Fatalf("替代文字不应进入可查找正文：%q", ed.Text())
	}
	if n := ed.Replace("甲", "乙", false); n != 0 || strings.Contains(ed.Markdown(), "乙图") {
		t.Fatalf("行内图片内的文字不可替换：%d %q", n, ed.Markdown())
	}
	if !strings.Contains(ed.Markdown(), "![甲图](a.png)") {
		t.Fatalf("图片原文应保留：%q", ed.Markdown())
	}
	if !strings.Contains(ed.Text(), "后见") {
		t.Fatalf("图片之后的正文应还在：%q", ed.Text())
	}
}

func TestInlineImageBitmapAndCaret(t *testing.T) {
	source := "前 ![图][pic] 后\n\n[pic]: assets/a.png\n"
	var raw bytes.Buffer
	_ = png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 20, 10)))
	reads := 0
	ed := New(source)
	ed.FontSize(16)
	ed.SetReadImage(func(path string) *ui.Bitmap {
		reads++
		if path != "assets/a.png" {
			t.Fatalf("不应读取未引用的路径：%s", path)
		}
		bm, err := ui.DecodeBitmap(raw.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		return bm
	})
	tt := ui.NewTester(ed.View, 640, 480)
	tt.Frame()
	tt.Frame()
	if reads != 1 || ed.images["assets/a.png"] == nil {
		t.Fatalf("授权位图应只读一次：reads=%d", reads)
	}
	var sp paintSpan
	var ln line
	found := false
	for _, row := range ed.lay.lines {
		for _, span := range row.spans {
			if span.image == "assets/a.png" {
				sp, ln, found = span, row, true
			}
		}
	}
	if !found || len(sp.glyphs) != 1 || sp.glyphs[0].Runes != 1 || len(sp.display) != 0 {
		t.Fatalf("有位图时应占一个 rune 且不画替代文字：%+v", sp)
	}
	h := ed.fontSize * 1.15
	want := h * 2
	if abs32(sp.glyphs[0].Advance-want) > 1 {
		t.Fatalf("行内位图宽度 = %v，按行高缩放应为 %v", sp.glyphs[0].Advance, want)
	}
	at := ln.origin + sp.glyphs[0].Cluster
	x0, y0, h0 := ed.PointForOffset(at)
	tt.ClickAt(x0+1, y0+h0/2)
	if a, b := ed.Selection(); a != at || b != at {
		t.Fatalf("图片左侧光标 = %d,%d，占位在 %d", a, b, at)
	}
	x1, y1, h1 := ed.PointForOffset(at + 1)
	tt.ClickAt(x1-1, y1+h1/2)
	if a, b := ed.Selection(); a != at+1 || b != at+1 {
		t.Fatalf("图片右侧光标 = %d,%d", a, b)
	}
	ed.SetSelection(at, at+1)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	if strings.Contains(ed.Text(), "￼") || !strings.Contains(ed.Text(), "前") || !strings.Contains(ed.Text(), "后") || strings.Contains(ed.Markdown(), "![图]") {
		t.Fatalf("删除图片后 = %q / %q", ed.Text(), ed.Markdown())
	}
	ed.Undo()
	if !strings.Contains(ed.Markdown(), "![图][pic]") || !strings.Contains(ed.Markdown(), "[pic]: assets/a.png") || !strings.Contains(ed.Text(), "前") || !strings.Contains(ed.Text(), "后") {
		t.Fatalf("撤销应恢复图片和前后正文：%q", ed.Markdown())
	}
	ed.SetReadOnly(true)
	ed.SetSelection(0, 0)
	tt.Frame()
	tt.Compose("ni", 1)
	if ed.compose != "" || ed.Markdown() != source && !strings.Contains(ed.Markdown(), "![图][pic]") {
		t.Fatalf("只读组合输入不应留下预编辑：compose=%q md=%q", ed.compose, ed.Markdown())
	}
	if strings.Contains(ed.Text(), "ni") {
		t.Fatalf("只读组合输入改了正文：%q", ed.Text())
	}
}

func TestDetailsFoldKeepsSource(t *testing.T) {
	source := "<details><summary>标题</summary><p>隐藏正文</p></details>\n\n> [!note]- 普通\n> 提示正文\n"
	ed := New(source)
	ed.FontSize(16)
	tt := ui.NewTester(ed.View, 640, 480)
	tt.Frame()
	var box frame
	found := false
	for _, f := range ed.lay.frames {
		if f.details {
			box, found = f, true
		}
	}
	if !found || !box.folded {
		t.Fatalf("未打开的 details 应默认收起：folded=%v map=%v", box.folded, ed.collapsed)
	}
	hidden := blockOf(t, ed, "隐藏正文")
	if hidden.x < 1000 {
		t.Fatalf("收起时正文应移出视口：%+v", hidden)
	}
	note := blockOf(t, ed, "提示正文")
	if note.kind != richtext.Callout || note.x > 1000 {
		t.Fatalf("普通提示块应保持展开：%+v", note)
	}
	before := ed.Markdown()
	tt.Move(box.x+20, box.y+16)
	if tt.Cursor() != ui.CursorPointer {
		t.Fatalf("折叠标题上的指针 = %v", tt.Cursor())
	}
	tt.ClickAt(box.x+20, box.y+16)
	tt.Frame()
	var open frame
	for _, f := range ed.lay.frames {
		if f.details {
			open = f
		}
	}
	if open.folded {
		t.Fatalf("点击标题应展开：hot=%+v map=%v", ed.hotAt(box.x+20, box.y+16), ed.collapsed)
	}
	shown := blockOf(t, ed, "隐藏正文")
	if shown.x > 1000 || shown.y < open.y {
		t.Fatalf("展开后正文应回到标题下：%+v", shown)
	}
	if ed.Markdown() != before || ed.Changed() {
		t.Fatalf("折叠不应改文档：%q", ed.Markdown())
	}
	tt.ClickAt(open.x+20, open.y+16)
	tt.Frame()
	ed.SetSelection(offsetOf(t, ed, "隐藏正文"), offsetOf(t, ed, "隐藏正文"))
	tt.Frame()
	for _, f := range ed.lay.frames {
		if f.details && f.folded {
			t.Fatal("光标进入隐藏正文时应展开祖先")
		}
	}
	ed.SetReadOnly(true)
	var title frame
	for _, f := range ed.lay.frames {
		if f.details {
			title = f
		}
	}
	tt.ClickAt(title.x+20, title.y+16)
	tt.Frame()
	folded := false
	for _, f := range ed.lay.frames {
		if f.details {
			folded = f.folded
		}
	}
	if !folded || ed.Markdown() != before {
		t.Fatalf("只读时标题仍应能收起且不改文档：folded=%v md=%q", folded, ed.Markdown())
	}
}
