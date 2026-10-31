package nativeeditor

import (
	"runtime"
	"unicode"

	"github.com/egoist/mygo/ui"
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
			e.compose = ""
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
			e.cancelCompose()
			sel := e.doc.CreateParagraphAfter(e.focus)
			e.anchor, e.focus = sel.Start, sel.End
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
	x := ln.caretX(offset)
	j := li
	if down {
		if li+1 >= len(e.lay.lines) {
			return e.doc.Len()
		}
		j = li + 1
	} else {
		if li == 0 {
			return 0
		}
		j = li - 1
	}
	return e.lay.lines[j].hitOffset(x)
}

func (e *Editor) wordBound(offset, dir int) int {
	rs := []rune(e.doc.Text())
	if dir < 0 {
		i := offset
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
	i := offset
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

// clusterBound 按字形簇移动。ZWJ 序列和键帽 emoji 一次删掉，不拆开。
func (e *Editor) clusterBound(offset, dir int) int {
	bounds := clusterBounds([]rune(e.doc.Text()))
	if dir < 0 {
		for i := len(bounds) - 1; i >= 0; i-- {
			if bounds[i] < offset {
				return bounds[i]
			}
		}
		return 0
	}
	for _, at := range bounds {
		if at > offset {
			return at
		}
	}
	return e.doc.Len()
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
	before := e.doc.Markdown()
	out := e.doc.Replace(e.selection(), text)
	e.anchor, e.focus = out.Start, out.End
	e.pending = 0
	e.dirty = e.dirty || before != e.doc.Markdown()
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

func (e *Editor) blockBox(index int) blockBox {
	if index >= 0 && index < len(e.lay.blocks) && e.lay.blocks[index].index == index {
		return e.lay.blocks[index]
	}
	for _, b := range e.lay.blocks {
		if b.index == index {
			return b
		}
	}
	if len(e.lay.blocks) == 0 {
		return blockBox{}
	}
	return e.lay.blocks[0]
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
