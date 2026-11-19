package nativeeditor

import (
	"runtime"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

func wordModifier() ui.Modifiers {
	if runtime.GOOS == "darwin" {
		return ui.Alt
	}
	return ui.Ctrl
}

func (e *Editor) onInput(services ui.Services) func(ui.InputEvent) bool {
	return func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp, ui.InputScroll:
			e.hoverX, e.hoverY, e.hoverMods, e.hovered = ev.X, ev.Y, ev.Mods, true
		case ui.InputKeyDown, ui.InputKeyUp:
			e.hoverMods = ev.Mods
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			x, y := ev.X+e.scroll.X, ev.Y+e.scroll.Y
			if ev.Button != 0 {
				// 右键落在选区之外时先把光标移过去，菜单才作用于点到的位置。
				if off, sel := e.lay.hit(x, y), e.selection(); off < sel.Start || off > sel.End {
					e.anchor, e.focus = off, off
					e.hasGoalX = false
				}
				return false
			}
			if e.toggleTaskAt(x, y) {
				return true
			}
			e.pressHot = hot{}
			if h := e.hotAt(x, y); ev.Clicks == 1 && e.clickable(h, ev.Mods) {
				e.pressHot = h
			}
			off := e.lay.hit(x, y)
			if ev.Clicks == 2 && e.requestRawEdit(off) {
				return true
			}
			if ev.Clicks >= 3 {
				e.selectLine(off)
			} else if ev.Clicks == 2 {
				e.selectWord(off)
			} else {
				e.anchor, e.focus = off, off
			}
			e.dragging = true
			e.hasGoalX = false
			e.cancelCompose()
			if e.pressHot.kind != hotDetails {
				e.reveal = true
			}
			return true
		case ui.InputPointerMove:
			if !e.dragging || ev.Button < 0 {
				// 悬停：只在指针形状变化时要一帧。
				return e.updatePointer(ev.X+e.scroll.X, ev.Y+e.scroll.Y, ev.Mods)
			}
			e.focus = e.lay.hit(ev.X+e.scroll.X, ev.Y+e.scroll.Y)
			e.hasGoalX = false
			e.reveal = true
			e.hand = e.hand && e.anchor == e.focus // 拖出选区后恢复文本指针
			return true
		case ui.InputPointerUp:
			x, y := ev.X+e.scroll.X, ev.Y+e.scroll.Y
			h := e.pressHot
			e.dragging, e.pressHot = false, hot{}
			if h.kind != hotNone && ev.Button == 0 && e.anchor == e.focus && e.hotAt(x, y) == h {
				e.activate(services, h)
			}
			e.updatePointer(x, y, ev.Mods)
			return true
		case ui.InputKeyUp:
			// 松开 Mod 后链接不再可点，指针形状跟着变。
			if e.updatePointer(ev.X+e.scroll.X, ev.Y+e.scroll.Y, ev.Mods) {
				services.Invalidate()
			}
			return false
		case ui.InputScroll:
			// 内容在指针下滚过，下一帧按新位置重算指针形状。
			services.Invalidate()
			return false
		case ui.InputKeyDown:
			if e.updatePointer(ev.X+e.scroll.X, ev.Y+e.scroll.Y, ev.Mods) {
				services.Invalidate()
			}
			return e.onKey(services, ev)
		case ui.InputText:
			if e.readOnly {
				return false
			}
			e.insertText(ev.Text)
			return true
		case ui.InputCompose:
			if e.readOnly {
				return false
			}
			e.compose = ev.Text
			e.comCaret = ev.Caret
			if ev.Text == "" {
				e.comCaret = 0
			}
			e.reveal = true
			return true
		case ui.InputCommand:
			return e.onCommand(services, ev.Text)
		default:
			return false
		}
	}
}

func (e *Editor) onKey(services ui.Services, ev ui.InputEvent) bool {
	mods := ev.Mods &^ ui.Shift
	word := mods == e.wordMod
	if e.readOnly {
		// 阅读模式只保留移动、全选和复制，其余按键交还给窗口的快捷键。
		switch ev.Key {
		case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd:
		case ui.KeyA, ui.KeyC:
			if mods != ui.Cmd {
				return false
			}
		default:
			return false
		}
	}
	switch ev.Key {
	case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd:
		e.move(ev.Key, ev.Mods&ui.Shift != 0, word || mods == ui.Cmd)
		return true
	case ui.KeyBackspace:
		e.backspace(word)
		return true
	case ui.KeyDelete:
		e.deleteForward(word)
		return true
	case ui.KeyEnter:
		if mods == ui.Cmd || mods == ui.Ctrl {
			// 源码模式始终保持单块纯文本不变式，禁止拆出新段落块。
			if e.sourceMode {
				return false
			}
			e.cancelCompose()
			sel := e.doc.CreateParagraphAfter(e.focus)
			e.anchor, e.focus = sel.Start, sel.End
			e.hasGoalX = false
			e.dirty, e.reveal = true, true
			return true
		}
		if mods != 0 {
			return false
		}
		e.split()
		return true
	case ui.KeyTab:
		if mods != 0 {
			return false
		}
		return e.tabKey(ev.Mods&ui.Shift != 0)
	case ui.KeyEscape:
		return false
	case ui.KeyA:
		if mods == ui.Cmd {
			e.anchor, e.focus = 0, e.doc.Len()
			e.hasGoalX = false
			return true
		}
	case ui.KeyC:
		if mods == ui.Cmd {
			e.copy(services, false)
			return true
		}
	case ui.KeyX:
		if mods == ui.Cmd {
			e.copy(services, true)
			return true
		}
	case ui.KeyV:
		if mods == ui.Cmd {
			e.paste(services)
			return true
		}
	case ui.KeyZ:
		if ev.Mods&ui.Shift != 0 && ev.Mods&(ui.Cmd|ui.Ctrl) != 0 {
			e.Redo()
			return true
		}
		if mods == ui.Cmd || mods == ui.Ctrl {
			e.Undo()
			return true
		}
	case ui.KeyY:
		if mods == ui.Cmd || mods == ui.Ctrl {
			e.Redo()
			return true
		}
	case ui.KeyB:
		if mods == ui.Cmd {
			e.Format("bold")
			return true
		}
	}
	// 可打印字符由随后的 InputText 插入。
	return mods == 0 || (runtime.GOOS == "darwin" && mods == ui.Alt)
}

func (e *Editor) onCommand(services ui.Services, name string) bool {
	if e.readOnly && name != "copy" && name != "selectAll" {
		return false
	}
	switch name {
	case "copy":
		e.copy(services, false)
	case "cut":
		e.copy(services, true)
	case "paste":
		e.paste(services)
	case "selectAll":
		e.anchor, e.focus = 0, e.doc.Len()
		e.hasGoalX = false
	case "undo":
		e.Undo()
	case "redo":
		e.Redo()
	case "delete":
		e.replaceSelection("")
	default:
		return false
	}
	return true
}

func (e *Editor) move(key ui.Key, extend, extra bool) {
	e.cancelCompose()
	if key != ui.KeyUp && key != ui.KeyDown {
		e.hasGoalX = false
	}
	n := e.doc.Len()
	next := e.focus
	switch key {
	case ui.KeyLeft:
		if !extend && e.anchor != e.focus {
			next = min(e.anchor, e.focus)
			break
		}
		if extra {
			next = e.wordBound(e.focus, -1)
		} else {
			next = e.clusterBound(e.focus, -1)
		}
	case ui.KeyRight:
		if !extend && e.anchor != e.focus {
			next = max(e.anchor, e.focus)
			break
		}
		if extra {
			next = e.wordBound(e.focus, 1)
		} else {
			next = e.clusterBound(e.focus, 1)
		}
	case ui.KeyHome:
		if extra {
			next = 0
		} else {
			next = e.lineEdge(e.focus, true)
		}
	case ui.KeyEnd:
		if extra {
			next = n
		} else {
			next = e.lineEdge(e.focus, false)
		}
	case ui.KeyUp, ui.KeyDown:
		next = e.vertical(e.focus, key == ui.KeyDown)
	}
	if !extend {
		e.anchor = next
	}
	e.focus = next
	e.pending = 0
	e.reveal = true
}

func (e *Editor) lineEdge(offset int, home bool) int {
	li := e.lay.lineAt(offset)
	if li < 0 {
		return offset
	}
	ln := e.lay.lines[li]
	if home {
		return ln.origin
	}
	return ln.origin + ln.count
}

func (e *Editor) vertical(offset int, down bool) int {
	li := e.lay.lineAt(offset)
	if li < 0 {
		return offset
	}
	ln := e.lay.lines[li]
	curBox := e.blockBox(ln.block)
	if !e.hasGoalX {
		e.goalX = curBox.x + ln.caretX(offset)
		e.hasGoalX = true
	}
	targetX := e.goalX

	// 表格内上下方向键优先在同一单元格折行间移动，再跨表格行保持同列。
	if curBox.kind == richtext.TableBlock {
		if down {
			if li+1 < len(e.lay.lines) {
				next := e.lay.lines[li+1]
				if next.block == ln.block && next.tableRow == ln.tableRow && next.tableCol == ln.tableCol {
					return next.hitOffset(targetX - curBox.x)
				}
			}
			if j, ok := e.tableLineAt(curBox, ln.tableRow+1, ln.tableCol, true); ok {
				return e.lay.lines[j].hitOffset(targetX - curBox.x)
			}
			if len(curBox.lines) > 0 {
				after := curBox.lines[len(curBox.lines)-1] + 1
				if after < len(e.lay.lines) {
					return e.hitVerticalTarget(after, targetX, true)
				}
			}
			return e.doc.Len()
		}
		if li > 0 {
			prev := e.lay.lines[li-1]
			if prev.block == ln.block && prev.tableRow == ln.tableRow && prev.tableCol == ln.tableCol {
				return prev.hitOffset(targetX - curBox.x)
			}
		}
		if ln.tableRow > 0 {
			if j, ok := e.tableLineAt(curBox, ln.tableRow-1, ln.tableCol, false); ok {
				return e.lay.lines[j].hitOffset(targetX - curBox.x)
			}
		}
		if len(curBox.lines) > 0 {
			before := curBox.lines[0] - 1
			if before >= 0 {
				return e.hitVerticalTarget(before, targetX, false)
			}
		}
		return 0
	}

	if down {
		if li+1 >= len(e.lay.lines) {
			return e.doc.Len()
		}
		return e.hitVerticalTarget(li+1, targetX, true)
	}
	if li == 0 {
		return 0
	}
	return e.hitVerticalTarget(li-1, targetX, false)
}

// tableLineAt 返回表格中指定行列的首行（fromTop=true）或末行（fromTop=false）在 lay.lines 中的下标。
func (e *Editor) tableLineAt(b blockBox, row, col int, fromTop bool) (int, bool) {
	chosen := -1
	fallback := -1
	for _, idx := range b.lines {
		ln := e.lay.lines[idx]
		if ln.tableRow != row {
			if ln.tableRow > row {
				break
			}
			continue
		}
		if ln.tableCol == col {
			if fromTop {
				return idx, true
			}
			chosen = idx
		} else if ln.tableCol < col {
			if fallback < 0 || ln.tableCol > e.lay.lines[fallback].tableCol || (!fromTop && ln.tableCol == e.lay.lines[fallback].tableCol) {
				fallback = idx
			}
		}
	}
	if chosen >= 0 {
		return chosen, true
	}
	if fallback >= 0 {
		return fallback, true
	}
	return -1, false
}

// hitVerticalTarget 把屏幕横坐标落到目标行上；目标若是表格则先按列边界匹配到同横坐标所在列。
func (e *Editor) hitVerticalTarget(j int, targetX float32, fromTop bool) int {
	targetLn := e.lay.lines[j]
	targetBox := e.blockBox(targetLn.block)
	relX := targetX - targetBox.x
	if targetBox.kind == richtext.TableBlock && len(targetBox.cols) > 1 {
		col := sort.Search(len(targetBox.cols)-1, func(c int) bool {
			return targetBox.cols[c] > relX
		}) - 1
		if col < 0 {
			col = 0
		}
		if col >= len(targetBox.cols)-1 {
			col = len(targetBox.cols) - 2
		}
		if tj, ok := e.tableLineAt(targetBox, targetLn.tableRow, col, fromTop); ok {
			return e.lay.lines[tj].hitOffset(relX)
		}
	}
	return targetLn.hitOffset(relX)
}

// localTextWindow 流式解码文档局部窗口 [max(0, offset-before), offset+after) 的 rune，避免把整篇文档转成 []rune。
func localTextWindow(text string, offset, before, after int) (rs []rune, base int, reachedEnd bool) {
	start := max(0, offset-before)
	end := offset + after
	rs = make([]rune, 0, min(end-start, 512))
	i := 0
	for len(text) > 0 {
		if i >= end {
			return rs, start, false
		}
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if i >= start {
			rs = append(rs, r)
		}
		i++
	}
	return rs, start, true
}

func wordBoundLocal(rs []rune, local, dir int) int {
	if dir < 0 {
		i := min(max(local, 0), len(rs))
		for i > 0 && unicode.IsSpace(rs[i-1]) {
			i--
		}
		if i == 0 {
			return 0
		}
		kind := runeKind(rs[i-1])
		for i > 0 && runeKind(rs[i-1]) == kind {
			i--
		}
		return i
	}
	i := min(max(local, 0), len(rs))
	for i < len(rs) && unicode.IsSpace(rs[i]) {
		i++
	}
	if i >= len(rs) {
		return len(rs)
	}
	kind := runeKind(rs[i])
	for i < len(rs) && runeKind(rs[i]) == kind {
		i++
	}
	return i
}

func (e *Editor) wordBound(offset, dir int) int {
	n := e.doc.Len()
	if dir < 0 && offset <= 0 {
		return 0
	}
	if dir > 0 && offset >= n {
		return n
	}
	// 布局与文档版本一致时，先在当前行局部窗口内查找，命中内部边界时无需读取全文。
	if e.lay.docRev == e.doc.Revision() && len(e.lay.lines) > 0 {
		if li := e.lay.lineAt(offset); li >= 0 {
			ln := e.lay.lines[li]
			local := offset - ln.origin
			if dir < 0 && local > 0 && local <= ln.count {
				rs, base, _ := localTextWindow(ln.text, local, 256, 0)
				res := wordBoundLocal(rs, local-base, -1)
				if res > 0 {
					return ln.origin + base + res
				}
			} else if dir > 0 && local >= 0 && local < ln.count {
				rs, base, _ := localTextWindow(ln.text, local, 0, 256)
				res := wordBoundLocal(rs, local-base, 1)
				if res < len(rs) {
					return ln.origin + base + res
				}
			}
		}
	}
	text := e.doc.Text()
	for win := 256; ; win *= 2 {
		before, after := 0, win
		if dir < 0 {
			before, after = win, 0
		}
		rs, base, reachedEnd := localTextWindow(text, offset, before, after)
		res := wordBoundLocal(rs, offset-base, dir)
		if dir < 0 {
			if res > 0 || base == 0 {
				return base + res
			}
		} else {
			if res < len(rs) || reachedEnd {
				return base + res
			}
		}
	}
}

// clusterBound 按字形簇移动。ZWJ 序列和键帽 emoji 一次删掉，不拆开。
func (e *Editor) clusterBound(offset, dir int) int {
	n := e.doc.Len()
	if dir < 0 && offset <= 0 {
		return 0
	}
	if dir > 0 && offset >= n {
		return n
	}
	// 软换行从不拆簇，光标严格落在当前行内部时只需扫描当前行局部窗口。
	if e.lay.docRev == e.doc.Revision() && len(e.lay.lines) > 0 {
		if li := e.lay.lineAt(offset); li >= 0 {
			ln := e.lay.lines[li]
			local := offset - ln.origin
			if (dir < 0 && local > 0 && local <= ln.count) || (dir > 0 && local >= 0 && local < ln.count) {
				rs, base, _ := localTextWindow(ln.text, local, 64, 64)
				if at, ok := clusterStepInWindow(rs, local-base, dir); ok {
					return ln.origin + base + at
				}
			}
		}
	}
	rs, base, _ := localTextWindow(e.doc.Text(), offset, 64, 64)
	if at, ok := clusterStepInWindow(rs, offset-base, dir); ok {
		return base + at
	}
	if dir < 0 {
		return 0
	}
	return n
}

func clusterStepInWindow(rs []rune, local, dir int) (int, bool) {
	bounds := clusterBounds(rs)
	if dir < 0 {
		for i := len(bounds) - 1; i >= 0; i-- {
			if bounds[i] < local {
				return bounds[i], true
			}
		}
		return 0, false
	}
	for _, at := range bounds {
		if at > local {
			return at, true
		}
	}
	return len(rs), false
}

// clusterBounds 不把组合符后面的普通文字并进簇；ZWJ 和区域旗帜成对处理。
func clusterBounds(rs []rune) []int {
	out := []int{0}
	regional := 0
	for i := 1; i < len(rs); i++ {
		prev, cur := rs[i-1], rs[i]
		if prev >= 0x1f1e6 && prev <= 0x1f1ff {
			regional++
		} else {
			regional = 0
		}
		join := isClusterContinue(cur) || prev == 0x200d || prev == '\r' && cur == '\n'
		if cur >= 0x1f1e6 && cur <= 0x1f1ff && regional%2 == 1 {
			join = true
		}
		if !join {
			out = append(out, i)
		}
	}
	if len(rs) > 0 {
		out = append(out, len(rs))
	}
	return out
}

func isClusterContinue(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r) || r == 0x200d || r >= 0x1f3fb && r <= 0x1f3ff || r >= 0xe0020 && r <= 0xe007f
}

func runeKind(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	default:
		return 2
	}
}

func (e *Editor) selectWord(offset int) {
	a := e.wordBound(offset, -1)
	b := e.wordBound(a, 1)
	if b < offset {
		b = e.wordBound(offset, 1)
	}
	e.anchor, e.focus = a, b
}

func (e *Editor) selectLine(offset int) {
	e.anchor = e.lineEdge(offset, true)
	e.focus = e.lineEdge(offset, false)
}

func (e *Editor) backspace(word bool) {
	e.cancelCompose()
	if e.anchor != e.focus {
		e.replaceSelection("")
		return
	}
	if e.focus == 0 {
		return
	}
	from := e.clusterBound(e.focus, -1)
	if word {
		from = e.wordBound(e.focus, -1)
	}
	sel := e.doc.Delete(from, e.focus)
	e.anchor, e.focus = sel.Start, sel.End
	e.hasGoalX = false
	e.dirty = true
	e.reveal = true
}

func (e *Editor) deleteForward(word bool) {
	e.cancelCompose()
	if e.anchor != e.focus {
		e.replaceSelection("")
		return
	}
	if e.focus >= e.doc.Len() {
		return
	}
	to := e.clusterBound(e.focus, 1)
	if word {
		to = e.wordBound(e.focus, 1)
	}
	sel := e.doc.Delete(e.focus, to)
	e.anchor, e.focus = sel.Start, sel.End
	e.hasGoalX = false
	e.dirty = true
	e.reveal = true
}

func (e *Editor) split() { e.cancelCompose(); e.replaceSelection("\n") }

func (e *Editor) insertText(text string) {
	if text == "" {
		return
	}
	e.cancelCompose()
	e.replaceSelection(text)
}

func (e *Editor) replaceSelection(text string) {
	if e.readOnly {
		return
	}
	rev := e.doc.Revision()
	out := e.doc.Replace(e.selection(), text)
	e.anchor, e.focus = out.Start, out.End
	e.pending = 0
	e.hasGoalX = false
	if e.doc.Revision() != rev {
		e.dirty = true
	}
	e.reveal = true
}

// 只有 InputText 能提交输入法内容；移动和删除仅取消预编辑串。
func (e *Editor) cancelCompose() { e.compose = ""; e.comCaret = 0 }

func (e *Editor) copy(services ui.Services, cut bool) {
	if services == (ui.Services{}) {
		return
	}
	cut = cut && !e.readOnly
	sel := e.selection()
	if sel.Start == sel.End {
		return
	}
	services.WriteClipboard(e.doc.SliceMarkdown(sel))
	if cut {
		out := e.doc.Delete(sel.Start, sel.End)
		e.anchor, e.focus = out.Start, out.End
		e.hasGoalX = false
		e.dirty = true
		e.reveal = true
	}
}

func (e *Editor) paste(services ui.Services) {
	if services == (ui.Services{}) || e.readOnly {
		return
	}
	text := services.ReadClipboard()
	if text == "" {
		return
	}
	e.cancelCompose()
	sel := e.doc.PasteSelection(e.selection(), text)
	e.anchor, e.focus = sel.Start, sel.End
	e.hasGoalX = false
	e.dirty = true
	e.reveal = true
}

func (e *Editor) caretRect() [4]float32 {
	off := e.focus
	if e.compose != "" {
		off = e.focus
	}
	li := e.lay.lineAt(off)
	if li < 0 || len(e.lay.blocks) == 0 {
		return [4]float32{padX, padY, 1, e.fontSize}
	}
	ln := e.lay.lines[li]
	b := e.blockBox(ln.block)
	x := b.x + ln.caretX(off)
	if e.compose != "" {
		x += e.composeAdvance
	}
	return [4]float32{x, ln.y, 1, ln.height}
}

func (e *Editor) composeWidth() float32 {
	if e.compose == "" {
		return 0
	}
	return advanceOf(e.composeGlyphs)
}

// findBlockBox 按文档块索引二分查找对应排版盒，兼容跳过 ReferenceDef 或收起 details 后的稀疏下标。
func findBlockBox(lay layout, index int) (blockBox, bool) {
	blocks := lay.blocks
	if index >= 0 && index < len(blocks) && blocks[index].index == index {
		return blocks[index], true
	}
	i := sort.Search(len(blocks), func(i int) bool {
		return blocks[i].index >= index
	})
	if i < len(blocks) && blocks[i].index == index {
		return blocks[i], true
	}
	return blockBox{}, false
}

func blockBoxIn(lay layout, index int) blockBox {
	if b, ok := findBlockBox(lay, index); ok {
		return b
	}
	if len(lay.blocks) == 0 {
		return blockBox{}
	}
	return lay.blocks[0]
}

func (e *Editor) blockBox(index int) blockBox {
	return blockBoxIn(e.lay, index)
}

func (e *Editor) keepCaretVisible() {
	r := e.caretRect()
	if e.typewriter {
		e.scroll.Y = r[1] + r[3]/2 - e.viewH/2
	}
	if r[1] < e.scroll.Y {
		e.scroll.Y = max(0, r[1]-24)
	}
	if r[1]+r[3] > e.scroll.Y+e.viewH {
		e.scroll.Y = max(0, r[1]+r[3]-e.viewH+24)
	}
	e.scroll.Y = max(0, min(e.scroll.Y, max(0, e.lay.height-e.viewH)))
	if r[0] < e.scroll.X {
		e.scroll.X = max(0, r[0]-24)
	}
	if r[0]+1 > e.scroll.X+e.width {
		e.scroll.X = max(0, r[0]+25-e.width)
	}
}
