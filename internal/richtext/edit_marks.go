package richtext

import (
	"unicode/utf8"
)

// ToggleMark 切换一种行内样式。折叠光标只记录待输入格式。
func (d *Document) ToggleMark(sel Selection, mark Mark) {
	if sel.Start > sel.End {
		sel.Start, sel.End = sel.End, sel.Start
	}
	sel.Start, sel.End = d.clampAt(sel.Start), d.clampAt(sel.End)
	if sel.Start == sel.End {
		base, _ := d.typingStyle(sel.Start)
		next := base ^ mark
		// 只记录被切换的那一位，其余样式留给插入时从左侧继承。
		if d.pending == nil || d.pending.at != sel.Start {
			d.pending = &pendingMark{at: sel.Start}
		}
		d.pending.marks = next
		d.pending.set |= mark
		d.coalesce = coalesceNone
		return
	}
	bi := d.blockAt(sel.Start)
	if bi != d.blockAt(sel.End-1) {
		return
	}
	b := &d.blocks[bi]
	if !textKind(b.Kind) {
		return
	}
	d.remember(sel)
	s0, _ := d.blockRange(bi)
	from, to := sel.Start-s0, sel.End-s0
	all := true
	off := 0
	for _, r := range b.Runs {
		n := utf8.RuneCountInString(r.Text)
		if off < to && off+n > from && r.Marks&mark == 0 {
			all = false
		}
		off += n
	}
	b.Runs = mapMarks(b.Runs, from, to, func(m Mark) Mark {
		if all {
			return m &^ mark
		}
		return m | mark
	})
	b.dirty = true
	d.reindex()
}

func mapMarks(runs []Run, from, to int, fn func(Mark) Mark) []Run {
	var out []Run
	off := 0
	for _, r := range runs {
		rs := []rune(r.Text)
		n := len(rs)
		if off >= to || off+n <= from {
			out = append(out, r)
			off += n
			continue
		}
		if off < from {
			out = append(out, Run{Text: string(rs[:from-off]), Marks: r.Marks, Link: cloneLink(r.Link), Image: cloneInlineImage(r.Image)})
		}
		a, z := max(off, from), min(off+n, to)
		out = append(out, Run{Text: string(rs[a-off : z-off]), Marks: fn(r.Marks), Link: cloneLink(r.Link), Image: cloneInlineImage(r.Image)})
		if off+n > to {
			out = append(out, Run{Text: string(rs[to-off:]), Marks: r.Marks, Link: cloneLink(r.Link), Image: cloneInlineImage(r.Image)})
		}
		off += n
	}
	return mergeRuns(out)
}

// SetLink 设置或取消选区链接。
func (d *Document) SetLink(sel Selection, link *Link) {
	if sel.Start > sel.End {
		sel.Start, sel.End = sel.End, sel.Start
	}
	sel.Start, sel.End = d.clampAt(sel.Start), d.clampAt(sel.End)
	if sel.Start == sel.End {
		return
	}
	bi := d.blockAt(sel.Start)
	if bi != d.blockAt(sel.End-1) {
		return
	}
	b := &d.blocks[bi]
	if !textKind(b.Kind) {
		return
	}
	d.remember(sel)
	s0, _ := d.blockRange(bi)
	b.Runs = mapLink(b.Runs, sel.Start-s0, sel.End-s0, link)
	b.dirty = true
	d.reindex()
}

func mapLink(runs []Run, from, to int, link *Link) []Run {
	var out []Run
	off := 0
	for _, r := range runs {
		rs := []rune(r.Text)
		n := len(rs)
		if off >= to || off+n <= from {
			out = append(out, r)
			off += n
			continue
		}
		if off < from {
			out = append(out, Run{Text: string(rs[:from-off]), Marks: r.Marks, Link: cloneLink(r.Link), Image: cloneInlineImage(r.Image)})
		}
		a, z := max(off, from), min(off+n, to)
		out = append(out, Run{Text: string(rs[a-off : z-off]), Marks: r.Marks, Link: cloneLink(link), Image: cloneInlineImage(r.Image)})
		if off+n > to {
			out = append(out, Run{Text: string(rs[to-off:]), Marks: r.Marks, Link: cloneLink(r.Link), Image: cloneInlineImage(r.Image)})
		}
		off += n
	}
	return mergeRuns(out)
}
