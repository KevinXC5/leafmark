package nativeeditor

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// hotKind 是指针下可点击内容的种类。
type hotKind uint8

const (
	hotNone     hotKind = iota
	hotTask             // 任务复选框
	hotFootRef          // 脚注引用，跳到定义
	hotFootBack         // 脚注定义的编号或回跳箭头，跳回引用
	hotLink             // 行内链接或图表节点的链接
	hotRaw              // 可双击编辑源码的占位卡片、公式与图表
	hotDetails          // 折叠块标题
)

// hot 是一次命中的结果。value 是脚注标签或链接地址，block 是所在块。
type hot struct {
	kind  hotKind
	value string
	block int
}

// SetReadOnly 开关阅读模式：禁止输入、格式化、任务框切换、源码编辑与剪切粘贴，
// 仍可选择、复制、查找、滚动和点击链接。
func (e *Editor) SetReadOnly(on bool) {
	if e.readOnly == on {
		return
	}
	e.readOnly = on
	e.cancelCompose()
	e.pending = 0
	e.pressHot = hot{}
}

// ReadOnly 报告是否处于阅读模式。
func (e *Editor) ReadOnly() bool { return e.readOnly }

// SetOpenLink 设置打开链接的回调。只会收到 http、https 与 mailto 地址。
func (e *Editor) SetOpenLink(fn func(url string)) { e.openLink = fn }

// openable 只放行交给系统打开的链接协议。
func openable(url string) bool {
	low := strings.ToLower(strings.TrimSpace(url))
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") && !strings.HasPrefix(low, "mailto:") {
		return false
	}
	return safeURL(url, false)
}

// hotAt 返回内容坐标下的可点击内容，不考虑当前模式是否允许触发。
func (e *Editor) hotAt(x, y float32) hot {
	if bi, ok := e.taskAt(x, y); ok {
		return hot{kind: hotTask, block: bi}
	}
	for _, f := range e.lay.frames {
		if f.details && (ui.Rect{X: f.x, Y: f.y, W: min(f.w, 280), H: 32}).Contains(x, y) {
			return hot{kind: hotDetails, block: f.foldID}
		}
		if f.foot == "" {
			continue
		}
		if f.backRect.W > 0 && f.backRect.Contains(x, y) {
			return hot{kind: hotFootBack, value: f.foot}
		}
		if (ui.Rect{X: f.x - 2, Y: f.lineY, W: f.labelW + 4, H: f.lineH}).Contains(x, y) {
			return hot{kind: hotFootBack, value: f.foot}
		}
	}
	for _, b := range e.lay.blocks {
		if y < b.y || y >= b.y+b.h || x < b.x || x >= b.x+b.w {
			continue
		}
		if b.raw {
			if b.diagram != nil {
				dx, dy := b.x+max(0, (b.w-b.diagram.Width)/2), b.y+8
				if h, ok := b.diagram.HitTest(x-dx, y-dy); ok && openable(h.URL) {
					return hot{kind: hotLink, value: h.URL, block: b.index}
				}
			}
			return hot{kind: hotRaw, block: b.index}
		}
		break
	}
	for _, ln := range e.lay.lines {
		if y < ln.y || y >= ln.y+ln.height || len(ln.spans) == 0 {
			continue
		}
		bx := e.blockBox(ln.block).x
		for _, span := range ln.spans {
			if len(span.glyphs) == 0 || span.footnote == "" && span.link == "" {
				continue
			}
			if x < bx+span.glyphs[0].X || x >= bx+advanceOf(span.glyphs) {
				continue
			}
			if span.footnote != "" {
				return hot{kind: hotFootRef, value: span.footnote, block: ln.block}
			}
			if openable(span.link) {
				return hot{kind: hotLink, value: span.link, block: ln.block}
			}
		}
	}
	return hot{}
}

// clickable 报告在当前模式和修饰键下，单击 h 是否触发动作。
// 链接在阅读模式下单击打开，编辑模式下要按住 Mod，普通单击仍用于放置光标。
func (e *Editor) clickable(h hot, mods ui.Modifiers) bool {
	switch h.kind {
	case hotFootRef, hotFootBack, hotDetails:
		return true
	case hotLink:
		return e.readOnly || mods&ui.Cmd != 0
	default:
		return false
	}
}

// updatePointer 按指针位置决定是否显示小手，返回是否有变化。
func (e *Editor) updatePointer(x, y float32, mods ui.Modifiers) bool {
	hand := false
	if !e.dragging {
		switch h := e.hotAt(x, y); h.kind {
		case hotTask:
			hand = !e.readOnly
		case hotRaw:
			hand = !e.readOnly && e.editRaw != nil
		default:
			hand = e.clickable(h, mods)
		}
	}
	changed := hand != e.hand
	e.hand = hand
	return changed
}

// activate 执行单击动作：脚注往返跳转或打开链接。
func (e *Editor) activate(c *ui.Context, h hot) {
	switch h.kind {
	case hotDetails:
		if e.collapsed == nil {
			e.collapsed = map[int]bool{}
		}
		folded := true
		for _, f := range e.lay.frames {
			if f.details && f.foldID == h.block {
				folded = f.folded
			}
		}
		if folded {
			e.collapsed[h.block] = false
		} else {
			e.collapsed[h.block] = true
		}
	case hotFootRef, hotFootBack:
		if at, ok := e.doc.FootnoteTarget(h.value, h.kind == hotFootBack); ok {
			e.cancelCompose()
			e.anchor, e.focus = at, at
			e.pending = 0
			e.reveal = true
		}
	case hotLink:
		if !openable(h.value) {
			return
		}
		if e.openLink != nil {
			e.openLink(h.value)
		} else if c != nil {
			c.OpenURL(h.value)
		}
	}
}

// taskAt 返回坐标下的任务复选框所在块，只有首行的复选框算数。
func (e *Editor) taskAt(x, y float32) (int, bool) {
	for _, b := range e.lay.blocks {
		if b.kind != richtext.Task || len(b.lines) == 0 {
			continue
		}
		ln := e.lay.lines[b.lines[0]]
		if x < b.x+b.indent || x > b.x+b.indent+20 || y < ln.y || y > ln.y+ln.height {
			continue
		}
		return b.index, true
	}
	return 0, false
}

// Replace 把 query 替换成 replacement，返回替换的处数。query 为空或处于阅读模式时不做任何事。
// 公式、脚注引用、原文占位和图片里的匹配不算可替换的命中：单处替换会继续找下一处可编辑的匹配，
// 全文确实没有可编辑匹配时返回 0。
// all 为 false 时替换当前选中的匹配（没有选中匹配时替换光标后的下一处），再选中其后的一处；
// all 为 true 时替换全文。全部替换只走文档模型的 ReplaceAll，整个操作是一步撤销。
func (e *Editor) Replace(query, replacement string, all bool) int {
	if query == "" || e.readOnly {
		return 0
	}
	e.cancelCompose()
	if all {
		n, sel := e.doc.ReplaceAll(query, replacement)
		if n > 0 {
			e.anchor, e.focus = sel.Start, sel.End
			e.pending = 0
			e.dirty, e.reveal = true, true
		}
		return n
	}
	q := []rune(query)
	sel := e.selection()
	if !e.matchAt(sel.Start, q) || sel.End-sel.Start != len(q) {
		at := e.nextEditable(query, sel.Start, false)
		if at < 0 {
			return 0
		}
		sel = richtext.Selection{Start: at, End: at + len(q)}
	}
	before := e.doc.Markdown()
	out := e.doc.Replace(sel, replacement)
	if before == e.doc.Markdown() {
		// 选区落在原子结构上：不改文档，从这里继续找下一处可编辑匹配。
		if at := e.nextEditable(query, sel.End, false); at >= 0 {
			sel = richtext.Selection{Start: at, End: at + len(q)}
			before = e.doc.Markdown()
			out = e.doc.Replace(sel, replacement)
		}
		if before == e.doc.Markdown() {
			return 0
		}
	}
	e.anchor, e.focus = out.End, out.End
	e.pending = 0
	e.dirty, e.reveal = true, true
	e.Find(query)
	return 1
}

// nextEditable 返回 from 起下一处完全落在可编辑文本里的匹配。
// 当前有选区时从选区终点继续，避免重复命中正在看的这一处。
func (e *Editor) nextEditable(query string, from int, selected bool) int {
	q := []rune(query)
	text := []rune(e.doc.Text())
	start := from
	if selected {
		start = max(e.anchor, e.focus)
	}
	if at := e.editableFrom(text, q, start); at >= 0 {
		return at
	}
	if start > 0 {
		if at := e.editableFrom(text, q, 0); at >= 0 && at < start {
			return at
		}
	}
	return -1
}

// editableFrom 从 from 起找第一处不与原子结构相交的匹配。
func (e *Editor) editableFrom(text, q []rune, from int) int {
	for at := indexFrom(text, q, from); at >= 0; at = indexFrom(text, q, at+1) {
		if e.editableSpan(at, at+len(q)) {
			return at
		}
	}
	return -1
}

// matchAt 报告 at 处是否正好是一段可编辑的匹配。
func (e *Editor) matchAt(at int, q []rune) bool {
	text := []rune(e.doc.Text())
	return indexFrom(text, q, at) == at && e.editableSpan(at, at+len(q))
}

// editableSpan 报告半开区间是否完全落在可直接编辑的文本上。
// 行内公式、脚注引用、原文占位、图片和分隔线都是原子结构，区间与它们相交即不可编辑。
func (e *Editor) editableSpan(start, end int) bool {
	if start < 0 || end < start || end > e.doc.Len() {
		return false
	}
	blocks := e.doc.Blocks()
	for at := start; at < end; at++ {
		bi := e.doc.BlockIndexAt(at)
		if bi < 0 || bi >= len(blocks) {
			return false
		}
		b := blocks[bi]
		switch b.Kind {
		case richtext.Raw, richtext.Image, richtext.Horizontal:
			return false
		case richtext.Code, richtext.TableBlock:
			continue
		}
		if runAt(b, at-blockOrigin(blocks, bi)).atomic {
			return false
		}
	}
	return true
}

// blockOrigin 返回块在纯文本中的起点。块与块之间恰好隔一个换行。
func blockOrigin(blocks []richtext.Block, index int) int {
	at := 0
	for i := 0; i < index && i < len(blocks); i++ {
		at += blockRunes(blocks[i]) + 1
	}
	return at
}

func blockRunes(b richtext.Block) int {
	switch b.Kind {
	case richtext.Raw, richtext.Image, richtext.Horizontal:
		return 1
	case richtext.Code:
		return len([]rune(b.Code))
	default:
		return len([]rune(visibleRuns(b.Runs)))
	}
}

// runPiece 是块内某个偏移所在的 run。atomic 表示公式或脚注引用，整段不可拆开编辑。
type runPiece struct{ atomic bool }

func runAt(b richtext.Block, local int) runPiece {
	at := 0
	for _, r := range b.Runs {
		n := len([]rune(r.Text))
		if local < at+n {
			atomic := r.Marks&richtext.MarkMath != 0 || r.Image != nil || r.Link != nil && r.Link.Footnote != ""
			return runPiece{atomic: atomic}
		}
		at += n
	}
	return runPiece{}
}

// expandToCaret 在光标落在收起的 details 正文里时展开祖先。返回是否展开了容器。
func (e *Editor) expandToCaret() bool {
	blocks := e.doc.Blocks()
	bi := e.doc.BlockIndexAt(e.focus)
	if bi < 0 || bi >= len(blocks) {
		return false
	}
	opened := false
	for _, c := range blocks[bi].Containers {
		if !detailsContainer(c) {
			continue
		}
		folded := c.Callout != nil && c.Callout.Fold != "+"
		if e.collapsed != nil {
			if on, ok := e.collapsed[c.ID]; ok {
				folded = on
			}
		}
		if !folded {
			continue
		}
		if e.collapsed == nil {
			e.collapsed = map[int]bool{}
		}
		e.collapsed[c.ID] = false
		opened = true
	}
	if opened {
		e.cache.collapsed = e.collapsed
	}
	return opened
}
