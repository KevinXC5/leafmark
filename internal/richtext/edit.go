package richtext

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// snapshot 是撤销栈中的文档与选区。
type snapshot struct {
	blocks []editBlock
	sel    Selection
	// typing 为 true 时，紧接着的同点插入并入这一步。
	typing bool
	at     int
}

func (d *Document) remember(sel Selection) {
	d.rememberKind(sel, false)
}

// rememberType 记录一次插入。连续、相邻的插入合并成一步撤销。
func (d *Document) rememberType(sel Selection) {
	if d.coalesce == coalesceInsert && len(d.undo) > 0 {
		top := &d.undo[len(d.undo)-1]
		if top.typing && (top.at == sel.Start) {
			d.redo = nil
			d.clean = false
			d.pending = nil
			return
		}
	}
	d.rememberKind(sel, true)
}

func (d *Document) rememberKind(sel Selection, typing bool) {
	d.undo = append(d.undo, snapshot{blocks: cloneEdit(d.blocks), sel: sel, typing: typing, at: sel.Start})
	if len(d.undo) > 200 {
		d.undo = d.undo[1:]
	}
	d.redo = nil
	d.clean = false
	d.pending = nil
	d.after = sel
	if typing {
		d.coalesce = coalesceInsert
	} else {
		d.coalesce = coalesceNone
	}
}

// noteAfter 记下这次编辑完成后的光标，供重做恢复。
func (d *Document) noteAfter(sel Selection) Selection {
	d.after = sel
	if d.coalesce == coalesceInsert && len(d.undo) > 0 {
		d.undo[len(d.undo)-1].at = sel.Start
	}
	return sel
}

func cloneEdit(in []editBlock) []editBlock {
	out := make([]editBlock, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Block = cloneBlock(in[i].Block)
	}
	return out
}

func (d *Document) restore(s snapshot) {
	d.blocks = s.blocks
	d.reindex()
}

// Undo 回到上一步，并恢复操作前的选区。
func (d *Document) Undo() (Selection, bool) {
	if len(d.undo) == 0 {
		return Selection{}, false
	}
	d.redo = append(d.redo, snapshot{blocks: cloneEdit(d.blocks), sel: d.after})
	s := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.restore(s)
	d.clean = false
	d.pending = nil
	d.coalesce = coalesceNone
	d.after = s.sel
	return s.sel, true
}

// Redo 重做最近一次撤销，恢复该步完成后的选区。
func (d *Document) Redo() (Selection, bool) {
	if len(d.redo) == 0 {
		return Selection{}, false
	}
	d.undo = append(d.undo, snapshot{blocks: cloneEdit(d.blocks), sel: d.after})
	s := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.restore(s)
	d.clean = false
	d.pending = nil
	d.coalesce = coalesceNone
	d.after = s.sel
	return s.sel, true
}

func (d *Document) clampAt(at int) int {
	if at < 0 {
		return 0
	}
	if at > len(d.text) {
		return len(d.text)
	}
	return at
}

// rawCaret 报告光标是否正停在 Raw 占位符上。
func (d *Document) rawCaret(at int) bool {
	if at < 0 || at >= len(d.index) {
		return false
	}
	sp := d.index[at]
	return !sp.gap && sp.block >= 0 && sp.block < len(d.blocks) && d.blocks[sp.block].Kind == Raw
}

// Insert 在 at 插入文本。换行会拆成新块。落在 Raw 占位上时文档不变。
func (d *Document) Insert(at int, text string) Selection {
	at = d.clampAt(at)
	if text == "" || d.rawCaret(at) || (d.blocks[d.BlockIndexAt(at)].Kind == TableBlock && strings.ContainsAny(text, "\n\t\r")) {
		return Selection{Start: at, End: at}
	}
	marks, link := d.typingStyle(at)
	d.rememberType(Selection{Start: at, End: at})
	parts := strings.Split(text, "\n")
	p := at
	for i, part := range parts {
		if i > 0 {
			p = d.splitAt(p)
		}
		if part != "" {
			p = d.insertPlain(p, part, marks, link)
		}
	}
	d.reindex()
	return d.noteAfter(Selection{Start: p, End: p})
}

func (d *Document) insertPlain(at int, text string, marks Mark, link *Link) int {
	bi := d.BlockIndexAt(at)
	if bi < 0 || bi >= len(d.blocks) {
		return at
	}
	b := &d.blocks[bi]
	start, _ := d.blockRange(bi)
	local := at - start
	if local < 0 {
		local = 0
	}
	switch b.Kind {
	case Image, Horizontal, Raw:
		nb := editBlock{Block: Block{Kind: Paragraph, Runs: []Run{{Text: text}}}, dirty: true}
		d.blocks = insertEdit(d.blocks, bi+1, nb)
		d.reindex()
		ns, _ := d.blockRange(bi + 1)
		return ns + utf8.RuneCountInString(text)
	case Code:
		rs := []rune(b.Code)
		if local > len(rs) {
			local = len(rs)
		}
		b.Code = string(rs[:local]) + text + string(rs[local:])
		b.dirty = true
		return at + utf8.RuneCountInString(text)
	case TableBlock:
		return d.insertTableText(bi, at, text, marks, link)
	default:
		b.Runs = insertRuns(b.Runs, local, text, marks, link)
		b.dirty = true
		return at + utf8.RuneCountInString(text)
	}
}

func (d *Document) linkOf(at int) *Link {
	if at < 0 || at >= len(d.index) {
		return nil
	}
	sp := d.index[at]
	if sp.gap || sp.run < 0 {
		return nil
	}
	runs := d.runsAt(sp)
	if runs == nil || sp.run >= len(runs) {
		return nil
	}
	return cloneLink(runs[sp.run].Link)
}

// typingStyle 取插入点应继承的样式。待输入格式优先于左侧字符。
func (d *Document) typingStyle(at int) (Mark, *Link) {
	marks := Mark(0)
	var link *Link
	bi := d.BlockIndexAt(at)
	inherit := at > 0 && d.index[at-1].block == bi && !d.index[at-1].gap
	if d.blocks[bi].Kind == TableBlock {
		_, local := d.tableCaret(bi, at)
		inherit = inherit && local > 0
	}
	if inherit {
		marks = d.marksOf(at - 1)
		link = d.linkOf(at - 1)
		// 公式右侧继续输入的是普通正文，只有落在公式内部才延续。
		if marks&MarkMath != 0 && (at >= len(d.index) || d.index[at].gap || d.index[at].block != bi || d.marksOf(at)&MarkMath == 0) {
			marks &^= MarkMath
		}
	}
	if d.pending != nil && d.pending.at == at {
		marks = (marks &^ d.pending.set) | (d.pending.marks & d.pending.set)
		if d.pending.hasL {
			link = d.pending.link
		}
	}
	return marks, cloneLink(link)
}

// insertTableText 把文本写入 at 所在单元格。
// tableCaret 按整个单元格计算位置，包括空单元格和分隔符前的行尾。
func (d *Document) tableCaret(bi, at int) (cell, local int) {
	b := &d.blocks[bi]
	if b.Table == nil {
		return -1, 0
	}
	pos, _ := d.blockRange(bi)
	cell = 0
	for ri, row := range b.Table.Rows {
		if ri > 0 {
			pos++
		}
		for ci, c := range row {
			if ci > 0 {
				pos++
			}
			n := runCount(c.Runs)
			if at >= pos && at <= pos+n {
				return cell, at - pos
			}
			pos += n
			cell++
		}
	}
	return -1, 0
}

func (d *Document) insertTableText(bi, at int, text string, marks Mark, link *Link) int {
	b := &d.blocks[bi]
	cell, local := d.tableCaret(bi, at)
	if cell < 0 {
		return at
	}
	runs := d.cellRuns(b, cell)
	d.setCellRuns(b, cell, insertRuns(runs, local, text, marks, link))
	b.dirty = true
	return at + utf8.RuneCountInString(text)
}

func insertRuns(runs []Run, local int, text string, marks Mark, link *Link) []Run {
	if local < 0 {
		local = 0
	}
	if len(runs) == 0 {
		return []Run{{Text: text, Marks: marks, Link: cloneLink(link)}}
	}
	off := 0
	for i := range runs {
		rs := []rune(runs[i].Text)
		if local <= off+len(rs) {
			at := local - off
			head, tail := string(rs[:at]), string(rs[at:])
			// 调用方已结合当前位置与待输入格式，新文字必须使用解析后的样式。
			mid := Run{Text: text, Marks: marks, Link: cloneLink(link)}
			var next []Run
			next = append(next, runs[:i]...)
			if head != "" {
				next = append(next, Run{Text: head, Marks: runs[i].Marks, Link: cloneLink(runs[i].Link)})
			}
			next = append(next, mid)
			if tail != "" {
				next = append(next, Run{Text: tail, Marks: runs[i].Marks, Link: cloneLink(runs[i].Link)})
			}
			next = append(next, runs[i+1:]...)
			return mergeRuns(next)
		}
		off += len(rs)
	}
	return mergeRuns(append(runs, Run{Text: text, Marks: marks, Link: cloneLink(link)}))
}

func insertEdit(blocks []editBlock, at int, b editBlock) []editBlock {
	if at < 0 {
		at = 0
	}
	if at > len(blocks) {
		at = len(blocks)
	}
	blocks = append(blocks, editBlock{})
	copy(blocks[at+1:], blocks[at:])
	blocks[at] = b
	return blocks
}

func (d *Document) splitAt(at int) int {
	bi := d.BlockIndexAt(at)
	b := &d.blocks[bi]
	start, _ := d.blockRange(bi)
	if b.Kind == Raw || b.Kind == Image || b.Kind == Horizontal || b.Kind == TableBlock {
		nb := editBlock{Block: Block{Kind: Paragraph}, dirty: true}
		d.blocks = insertEdit(d.blocks, bi+1, nb)
		d.reindex()
		ns, _ := d.blockRange(bi + 1)
		return ns
	}
	if b.Kind == Callout {
		local := max(0, min(at-start, runCount(b.Runs)))
		marks, link := d.typingStyle(at)
		b.Runs = insertRuns(b.Runs, local, "\n", marks, link)
		b.dirty = true
		return at + 1
	}
	if b.Kind == Code {
		rs := []rune(b.Code)
		local := at - start
		if local < 0 {
			local = 0
		}
		if local > len(rs) {
			local = len(rs)
		}
		b.Code = string(rs[:local]) + "\n" + string(rs[local:])
		b.dirty = true
		return at + 1
	}
	rs := []rune(plainRuns(b.Runs))
	local := at - start
	if local < 0 {
		local = 0
	}
	if local > len(rs) {
		local = len(rs)
	}
	tail := sliceRuns(b.Runs, local, len(rs))
	b.Runs = sliceRuns(b.Runs, 0, local)
	b.dirty = true
	kind, level := b.Kind, b.Level
	if kind == Heading {
		kind, level = Paragraph, 0
	}
	nb := editBlock{Block: Block{Kind: kind, Level: level, Ordered: b.Ordered, Start: b.Start + 1, Runs: tail}, dirty: true, gap: b.gap, indent: b.indent}
	b.gap = ""
	nb.source = trailingLineBreak(b.source)
	b.source = ""
	d.blocks = insertEdit(d.blocks, bi+1, nb)
	d.reindex()
	ns, _ := d.blockRange(bi + 1)
	return ns
}

func plainRuns(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

func sliceRuns(runs []Run, from, to int) []Run {
	if from < 0 {
		from = 0
	}
	var out []Run
	off := 0
	for _, r := range runs {
		rs := []rune(r.Text)
		n := len(rs)
		a, z := off, off+n
		if z > from && a < to {
			if a < from {
				a = from
			}
			if z > to {
				z = to
			}
			out = append(out, Run{Text: string(rs[a-off : z-off]), Marks: r.Marks, Link: cloneLink(r.Link)})
		}
		off += n
	}
	return mergeRuns(out)
}

// Split 在 at 处换行。
func (d *Document) Split(at int) Selection {
	at = d.clampAt(at)
	if d.rawCaret(at) || d.blocks[d.BlockIndexAt(at)].Kind == TableBlock {
		return Selection{Start: at, End: at}
	}
	d.remember(Selection{Start: at, End: at})
	p := d.splitAt(at)
	return d.noteAfter(Selection{Start: p, End: p})
}

// Join 把 at 所在块并入前一块。
func (d *Document) Join(at int) Selection {
	at = d.clampAt(at)
	bi := d.BlockIndexAt(at)
	if bi <= 0 {
		return Selection{Start: at, End: at}
	}
	prev, cur := &d.blocks[bi-1], &d.blocks[bi]
	if !textKind(prev.Kind) || !textKind(cur.Kind) {
		return Selection{Start: at, End: at}
	}
	d.remember(Selection{Start: at, End: at})
	prev.Runs = mergeRuns(append(cloneRuns(prev.Runs), cur.Runs...))
	prev.dirty = true
	d.blocks = append(d.blocks[:bi], d.blocks[bi+1:]...)
	d.reindex()
	if at > 0 {
		at--
	}
	return d.noteAfter(Selection{Start: at, End: at})
}

func textKind(k Kind) bool {
	return k == Paragraph || k == Heading || k == Quote || k == List || k == Task || k == Callout
}

// Delete 删除半开区间；Raw 与表格结构边界保持只读。
func (d *Document) Delete(start, end int) Selection {
	start, end = d.clampAt(start), d.clampAt(end)
	if start > end {
		start, end = end, start
	}
	if start == end {
		return Selection{Start: start, End: start}
	}
	if !d.editableRange(start, end) {
		return Selection{Start: start, End: start}
	}
	d.remember(Selection{Start: start, End: end})
	bs, be := d.blockAt(start), d.blockAt(end-1)
	if bs == be {
		d.deleteInside(bs, start, end)
		d.reindex()
		return d.noteAfter(Selection{Start: start, End: start})
	}
	left, right := &d.blocks[bs], d.blocks[be]
	ls, _ := d.blockRange(bs)
	rs, _ := d.blockRange(be)
	if textKind(left.Kind) {
		tail := []Run{}
		if textKind(right.Kind) {
			tail = sliceRuns(right.Runs, end-rs, runCount(right.Runs))
		}
		left.Runs = mergeRuns(append(sliceRuns(left.Runs, 0, start-ls), tail...))
		left.dirty = true
	}
	d.blocks = append(d.blocks[:bs+1], d.blocks[be+1:]...)
	if len(d.blocks) == 0 {
		d.blocks = []editBlock{{Block: Block{Kind: Paragraph}, dirty: true}}
	}
	d.reindex()
	return d.noteAfter(Selection{Start: start, End: start})
}

func (d *Document) blockAt(at int) int {
	if at < 0 {
		return 0
	}
	if at >= len(d.index) {
		if len(d.blocks) == 0 {
			return 0
		}
		return len(d.blocks) - 1
	}
	return d.index[at].block
}

func runCount(runs []Run) int {
	n := 0
	for _, r := range runs {
		n += utf8.RuneCountInString(r.Text)
	}
	return n
}

func (d *Document) deleteInside(bi, start, end int) {
	if bi < 0 || bi >= len(d.blocks) {
		return
	}
	b := &d.blocks[bi]
	s0, _ := d.blockRange(bi)
	switch b.Kind {
	case Raw, Image, Horizontal:
		d.blocks = append(d.blocks[:bi], d.blocks[bi+1:]...)
		if len(d.blocks) == 0 {
			d.blocks = []editBlock{{Block: Block{Kind: Paragraph}, dirty: true}}
		}
	case TableBlock:
		cell, local := d.tableCaret(bi, start)
		if cell >= 0 {
			d.setCellRuns(b, cell, deleteRunRange(d.cellRuns(b, cell), local, local+end-start))
			b.dirty = true
		}
	case Code:
		rs := []rune(b.Code)
		a, z := start-s0, end-s0
		if a < 0 {
			a = 0
		}
		if z > len(rs) {
			z = len(rs)
		}
		b.Code = string(rs[:a]) + string(rs[z:])
		b.dirty = true
	default:
		b.Runs = deleteRunRange(b.Runs, start-s0, end-s0)
		b.dirty = true
	}
}

func deleteRunRange(runs []Run, from, to int) []Run {
	var out []Run
	off := 0
	for _, r := range runs {
		var keep []rune
		for _, ch := range []rune(r.Text) {
			if off < from || off >= to {
				keep = append(keep, ch)
			}
			off++
		}
		if len(keep) > 0 {
			out = append(out, Run{Text: string(keep), Marks: r.Marks, Link: cloneLink(r.Link)})
		}
	}
	return mergeRuns(out)
}

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
			out = append(out, Run{Text: string(rs[:from-off]), Marks: r.Marks, Link: cloneLink(r.Link)})
		}
		a, z := max(off, from), min(off+n, to)
		out = append(out, Run{Text: string(rs[a-off : z-off]), Marks: fn(r.Marks), Link: cloneLink(r.Link)})
		if off+n > to {
			out = append(out, Run{Text: string(rs[to-off:]), Marks: r.Marks, Link: cloneLink(r.Link)})
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
			out = append(out, Run{Text: string(rs[:from-off]), Marks: r.Marks, Link: cloneLink(r.Link)})
		}
		a, z := max(off, from), min(off+n, to)
		out = append(out, Run{Text: string(rs[a-off : z-off]), Marks: r.Marks, Link: cloneLink(link)})
		if off+n > to {
			out = append(out, Run{Text: string(rs[to-off:]), Marks: r.Marks, Link: cloneLink(r.Link)})
		}
		off += n
	}
	return mergeRuns(out)
}

// SetBlock 改变光标所在块的类型。
func (d *Document) SetBlock(sel Selection, kind Kind, level int) {
	bi := d.BlockIndexAt(d.clampAt(sel.Start))
	if bi < 0 || bi >= len(d.blocks) || (!textKind(d.blocks[bi].Kind) && d.blocks[bi].Kind != Code) || (!textKind(kind) && kind != Code) {
		return
	}
	if d.blocks[bi].Kind == kind && d.blocks[bi].Level == level {
		return
	}
	d.remember(sel)
	if kind == Code {
		d.blocks[bi].Code = plainRuns(d.blocks[bi].Runs)
		d.blocks[bi].Runs = nil
	} else if d.blocks[bi].Kind == Code {
		d.blocks[bi].Runs = []Run{{Text: d.blocks[bi].Code}}
		d.blocks[bi].Code = ""
	}
	d.blocks[bi].Kind = kind
	d.blocks[bi].Level = level
	if kind != List {
		d.blocks[bi].Ordered = false
	}
	d.blocks[bi].dirty = true
	d.reindex()
}

// SetOrdered 标记列表是否有序。
func (d *Document) SetOrdered(blockIndex int, ordered bool) {
	if blockIndex < 0 || blockIndex >= len(d.blocks) || d.blocks[blockIndex].Kind != List {
		return
	}
	d.remember(Selection{})
	d.blocks[blockIndex].Ordered = ordered
	d.blocks[blockIndex].dirty = true
}

// SetChecked 设置任务是否完成。
func (d *Document) SetChecked(blockIndex int, checked bool) {
	if blockIndex < 0 || blockIndex >= len(d.blocks) || d.blocks[blockIndex].Kind != Task {
		return
	}
	d.remember(Selection{})
	d.blocks[blockIndex].Checked = checked
	d.blocks[blockIndex].dirty = true
}

// InsertImage 在光标所在块之后插入图片块。
func (d *Document) InsertImage(at int, alt, url string) Selection {
	at = d.clampAt(at)
	d.remember(Selection{Start: at, End: at})
	bi := d.BlockIndexAt(at)
	nb := editBlock{Block: Block{Kind: Image, Alt: alt, URL: url}, dirty: true}
	d.blocks = insertEdit(d.blocks, bi+1, nb)
	d.reindex()
	ns, ne := d.blockRange(bi + 1)
	if ne <= ns {
		ne = ns + 1
	}
	return d.noteAfter(Selection{Start: ns, End: ne})
}

// SliceMarkdown 把选区写成可粘贴的 Markdown。
func (d *Document) SliceMarkdown(sel Selection) string {
	if sel.Start > sel.End {
		sel.Start, sel.End = sel.End, sel.Start
	}
	sel.Start, sel.End = d.clampAt(sel.Start), d.clampAt(sel.End)
	if sel.Start == sel.End {
		return ""
	}
	bs, be := d.blockAt(sel.Start), d.blockAt(sel.End-1)
	if bs == be {
		b := &d.blocks[bs]
		s0, _ := d.blockRange(bs)
		if textKind(b.Kind) {
			return plainRuns(sliceRuns(b.Runs, sel.Start-s0, sel.End-s0))
		}
		return string(d.text[sel.Start:sel.End])
	}
	var b strings.Builder
	for i := bs; i <= be && i < len(d.blocks); i++ {
		if i > bs {
			b.WriteString("\n\n")
		}
		writeBlockMD(&b, &d.blocks[i])
	}
	return b.String()
}

// Paste 在 at 粘贴 Markdown 片段。
func (d *Document) Paste(at int, markdown string) Selection {
	at = d.clampAt(at)
	if markdown == "" || d.rawCaret(at) {
		return Selection{at, at}
	}
	bi := d.BlockIndexAt(at)
	if d.blocks[bi].Kind == Code {
		return d.Insert(at, markdown)
	}
	frag := Parse(markdown)
	if d.blocks[bi].Kind == TableBlock {
		if strings.ContainsAny(markdown, "\n\r\t") {
			return Selection{at, at}
		}
		return d.Insert(at, frag.Text())
	}
	if !textKind(d.blocks[bi].Kind) {
		return Selection{at, at}
	}
	start, _ := d.blockRange(bi)
	local := at - start
	original := d.blocks[bi]
	head := sliceRuns(original.Runs, 0, local)
	tail := sliceRuns(original.Runs, local, runCount(original.Runs))
	d.remember(Selection{at, at})
	pieces := cloneEdit(frag.blocks)
	// 普通段落的首尾与插入位置相接；复杂块保留独立源码。
	if pieces[0].Kind == Paragraph {
		pieces[0].Runs = mergeRuns(append(head, pieces[0].Runs...))
		pieces[0].Kind = original.Kind
		pieces[0].Level = original.Level
		pieces[0].Ordered = original.Ordered
		pieces[0].Start = original.Start
		pieces[0].Checked = original.Checked
		pieces[0].Callout = cloneBlock(original.Block).Callout
		pieces[0].dirty = true
	} else if len(head) > 0 {
		prefix := original
		prefix.Runs = head
		prefix.source = ""
		prefix.gap = ""
		prefix.dirty = true
		pieces = append([]editBlock{prefix}, pieces...)
	}
	last := len(pieces) - 1
	caretLocal := d.blockTextLength(pieces[last])
	if pieces[last].Kind == Paragraph {
		pieces[last].Runs = mergeRuns(append(pieces[last].Runs, tail...))
		pieces[last].dirty = true
	} else if len(tail) > 0 {
		suffix := original
		suffix.Runs = tail
		suffix.source = ""
		suffix.gap = original.gap
		suffix.dirty = true
		pieces = append(pieces, suffix)
	}
	pieces[len(pieces)-1].gap = original.gap
	next := append([]editBlock(nil), d.blocks[:bi]...)
	next = append(next, pieces...)
	next = append(next, d.blocks[bi+1:]...)
	d.blocks = next
	d.reindex()
	pos, _ := d.blockRange(bi + last)
	return d.noteAfter(Selection{pos + caretLocal, pos + caretLocal})
}

func (d *Document) blockTextLength(b editBlock) int {
	switch b.Kind {
	case Raw, Image, Horizontal:
		return 1
	case Code:
		return utf8.RuneCountInString(b.Code)
	case TableBlock:
		return tableRuneLen(b.Table)
	default:
		return runCount(b.Runs)
	}
}

// Markdown 序列化。未改动的块回写源码，Raw 原样保留。
func (d *Document) Markdown() string {
	var out strings.Builder
	out.WriteString(d.prefix)
	for i := range d.blocks {
		bl := &d.blocks[i]
		piece := bl.source
		if bl.dirty || piece == "" {
			var buf strings.Builder
			writeBlockMD(&buf, bl)
			piece = buf.String()
			if strings.HasSuffix(bl.source, "\r\n") {
				piece += "\r\n"
			} else if strings.HasSuffix(bl.source, "\n") {
				piece += "\n"
			}
		}
		out.WriteString(piece)
		out.WriteString(bl.gap)
		if i+1 < len(d.blocks) && bl.gap == "" && !strings.HasSuffix(piece, "\n") {
			if (bl.Kind == List || bl.Kind == Task) && (d.blocks[i+1].Kind == List || d.blocks[i+1].Kind == Task) {
				out.WriteByte('\n')
			} else {
				out.WriteString("\n\n")
			}
		}
	}
	return out.String()
}

func writeBlockMD(b *strings.Builder, bl *editBlock) {
	switch bl.Kind {
	case Heading:
		level := bl.Level
		if level < 1 {
			level = 1
		}
		for i := 0; i < level; i++ {
			b.WriteByte('#')
		}
		b.WriteByte(' ')
		writeRunsMD(b, bl.Runs)
	case Callout:
		data := bl.Callout
		if data == nil {
			data = &CalloutData{Type: "note"}
		}
		prefix := strings.Repeat("> ", max(1, bl.Level))
		b.WriteString(prefix + "[!" + data.Type + "]" + data.Fold)
		if data.Title != "" {
			b.WriteString(" " + data.Title)
		}
		if len(bl.Runs) > 0 {
			var body strings.Builder
			writeRunsMD(&body, bl.Runs)
			b.WriteString("\n" + prefix + strings.ReplaceAll(body.String(), "\n", "\n"+prefix))
		}
	case Quote:
		var body strings.Builder
		writeRunsMD(&body, bl.Runs)
		prefix := strings.Repeat("> ", max(1, bl.Level))
		b.WriteString(prefix + strings.ReplaceAll(body.String(), "\n", "\n"+prefix))
	case List:
		b.WriteString(listIndent(bl))
		if bl.Ordered {
			n := bl.Start
			if n <= 0 {
				n = 1
			}
			b.WriteString(itoaSmall(n) + ". ")
		} else {
			b.WriteString("- ")
		}
		writeListRuns(b, bl.Runs, listIndent(bl))
	case Task:
		b.WriteString(listIndent(bl))
		if bl.Ordered {
			b.WriteString(itoaSmall(max(1, bl.Start)) + ". ")
		} else {
			b.WriteString("- ")
		}
		if bl.Checked {
			b.WriteString("[x] ")
		} else {
			b.WriteString("[ ] ")
		}
		writeListRuns(b, bl.Runs, listIndent(bl))
	case Code:
		fence := strings.Repeat("`", max(3, maxBackticks(bl.Code)+1))
		b.WriteString(fence + bl.Lang + "\n" + bl.Code)
		if !strings.HasSuffix(bl.Code, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(fence)
	case Horizontal:
		b.WriteString("---")
	case TableBlock:
		writeTableMD(b, bl.Table)
	case Image:
		b.WriteString("![" + escapeText(bl.Alt) + "](" + linkDestination(bl.URL) + linkTitle(bl.Title) + ")")
	case Raw:
		if bl.Raw != "" {
			b.WriteString(bl.Raw)
		} else {
			b.WriteString(bl.source)
		}
	default:
		writeRunsMD(b, bl.Runs)
	}
}

// writeRunsMD 以连续样式区间嵌套输出，避免相邻强调标记互相吞并。
func writeRunsMD(b *strings.Builder, runs []Run) { writeMarkedRuns(b, runs, 0) }

func writeMarkedRuns(b *strings.Builder, runs []Run, depth int) {
	marks := []Mark{MarkHighlight, MarkBold, MarkItalic, MarkStrike}
	if depth < len(marks) {
		mark := marks[depth]
		delimiter := map[Mark]string{MarkBold: "**", MarkItalic: "*", MarkStrike: "~~", MarkHighlight: "=="}[mark]
		for i := 0; i < len(runs); {
			j := i + 1
			marked := runs[i].Marks&mark != 0
			for j < len(runs) && (runs[j].Marks&mark != 0) == marked {
				j++
			}
			var inner strings.Builder
			writeMarkedRuns(&inner, runs[i:j], depth+1)
			text := inner.String()
			if marked {
				body := strings.TrimFunc(text, unicode.IsSpace)
				leading := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
				trailing := len(text) - len(strings.TrimRightFunc(text, unicode.IsSpace))
				if body != "" {
					b.WriteString(text[:leading])
					b.WriteString(delimiter + body + delimiter)
					b.WriteString(text[len(text)-trailing:])
				} else {
					b.WriteString(text)
				}
			} else {
				b.WriteString(text)
			}
			i = j
		}
		return
	}
	for _, r := range runs {
		text := escapeText(r.Text)
		if r.Marks&MarkMath != 0 {
			// 公式内容是 TeX 原文，不做 Markdown 转义。
			text = "$" + r.Text + "$"
		} else if r.Marks&(MarkSub|MarkSup) != 0 {
			// 旧版上下标内部是纯文本；空格必须转义，否则会失去上下标语义。
			delimiter := "~"
			if r.Marks&MarkSup != 0 {
				delimiter = "^"
			}
			text = delimiter + strings.ReplaceAll(text, " ", "\\ ") + delimiter
		} else if r.Marks&MarkCode != 0 {
			fence := strings.Repeat("`", maxBackticks(r.Text)+1)
			content := r.Text
			if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") || (strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ") && strings.TrimSpace(content) != "") {
				content = " " + content + " "
			}
			text = fence + content + fence
		}
		if r.Link != nil {
			text = "[" + text + "](" + linkDestination(r.Link.URL) + linkTitle(r.Link.Title) + ")"
		}
		b.WriteString(text)
	}
}

func escapeText(text string) string {
	var b strings.Builder
	for _, c := range text {
		if c < 128 && isASCIIPunct(byte(c)) {
			b.WriteByte('\\')
		}
		if c == '\n' {
			b.WriteString("  \n")
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func maxBackticks(text string) int {
	longest, count := 0, 0
	for _, c := range text {
		if c == '`' {
			count++
			longest = max(longest, count)
		} else {
			count = 0
		}
	}
	return longest
}

func linkDestination(url string) string {
	if !strings.ContainsAny(url, " \t\n\r<>") {
		return strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(url)
	}
	return "<" + strings.NewReplacer("\\", "\\\\", "<", "%3C", ">", "%3E", "\n", "%0A").Replace(url) + ">"
}

func linkTitle(title string) string {
	if title == "" {
		return ""
	}
	return " \"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(title) + "\""
}

func writeTableMD(b *strings.Builder, tb *TableData) {
	if tb == nil {
		return
	}
	cols := 0
	for _, row := range tb.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	// 单元格两侧各留一个空格，写出的表格在纯文本里同样易读。
	for r, row := range tb.Rows {
		b.WriteByte('|')
		for c := 0; c < cols; c++ {
			b.WriteByte(' ')
			if c < len(row) {
				var cell strings.Builder
				writeRunsMD(&cell, row[c].Runs)
				b.WriteString(strings.ReplaceAll(cell.String(), "\n", " "))
			}
			b.WriteString(" |")
		}
		if r == 0 {
			b.WriteString("\n|")
			for c := 0; c < cols; c++ {
				align := AlignNone
				if c < len(tb.Aligns) {
					align = tb.Aligns[c]
				}
				switch align {
				case AlignLeft:
					b.WriteString(" :--- |")
				case AlignCenter:
					b.WriteString(" :---: |")
				case AlignRight:
					b.WriteString(" ---: |")
				default:
					b.WriteString(" --- |")
				}
			}
		}
		if r+1 < len(tb.Rows) {
			b.WriteByte('\n')
		}
	}
}

func itoaSmall(n int) string { return strconv.Itoa(n) }

// ReplaceRaw 用 Markdown 替换一个 Raw 块。
func (d *Document) ReplaceRaw(blockIndex int, markdown string) error {
	if blockIndex < 0 || blockIndex >= len(d.blocks) || d.blocks[blockIndex].Kind != Raw {
		return ErrRaw
	}
	// 新内容沿用原块末尾的换行与空行，与后一块的间隔保持不变。
	if source := d.blocks[blockIndex].source; source != "" {
		markdown = strings.TrimRight(markdown, " \t\r\n") + source[len(strings.TrimRight(source, " \t\r\n")):]
	}
	frag := Parse(markdown)
	d.remember(Selection{})
	tail := append([]editBlock(nil), d.blocks[blockIndex+1:]...)
	frag.blocks[len(frag.blocks)-1].gap += d.blocks[blockIndex].gap
	d.blocks = append(append(d.blocks[:blockIndex], frag.blocks...), tail...)
	d.reindex()
	return nil
}

// editableRange 保证 Raw 与表格结构不会在普通编辑中被隐式删除。
func (d *Document) editableRange(start, end int) bool {
	for at := start; at < end; at++ {
		sp := d.index[at]
		if d.blocks[sp.block].Kind == Raw {
			return false
		}
		if d.blocks[sp.block].Kind == TableBlock {
			bi := sp.block
			if d.blockAt(start) != bi || d.blockAt(end-1) != bi {
				return false
			}
			cell, local := d.tableCaret(bi, start)
			return cell >= 0 && end-start <= runCount(d.cellRuns(&d.blocks[bi], cell))-local
		}
	}
	return true
}

// Replace 原子替换选区；结构边界与 Raw 上的替换保持原文。
func (d *Document) Replace(sel Selection, text string) Selection {
	if d.blocks[d.BlockIndexAt(d.clampAt(sel.Start))].Kind == TableBlock && strings.ContainsAny(text, "\n\r\t") {
		return sel
	}
	return d.atomicEdit(sel, func(at int) Selection { return d.Insert(at, text) })
}

// PasteSelection 原子替换选区并粘贴 Markdown。
func (d *Document) PasteSelection(sel Selection, markdown string) Selection {
	if d.blocks[d.BlockIndexAt(d.clampAt(sel.Start))].Kind == TableBlock && strings.ContainsAny(markdown, "\n\r\t") {
		return sel
	}
	return d.atomicEdit(sel, func(at int) Selection { return d.Paste(at, markdown) })
}

// InsertLinkSelection 原子替换选区并给新文本设置链接。
func (d *Document) InsertLinkSelection(sel Selection, label, url string) Selection {
	return d.atomicEdit(sel, func(at int) Selection {
		end := d.Insert(at, label)
		d.SetLink(Selection{Start: at, End: end.End}, &Link{URL: url})
		return end
	})
}

func (d *Document) atomicEdit(sel Selection, action func(int) Selection) Selection {
	sel.Start, sel.End = d.clampAt(sel.Start), d.clampAt(sel.End)
	if sel.Start > sel.End {
		sel.Start, sel.End = sel.End, sel.Start
	}
	if d.rawCaret(sel.Start) || !d.editableRange(sel.Start, sel.End) {
		return sel
	}
	before := cloneEdit(d.blocks)
	oldUndo := append([]snapshot(nil), d.undo...)
	oldRedo := append([]snapshot(nil), d.redo...)
	oldClean := d.clean
	d.coalesce = coalesceNone
	if sel.Start != sel.End {
		d.Delete(sel.Start, sel.End)
	}
	result := action(sel.Start)
	if len(d.undo) == len(oldUndo) {
		return result
	}
	d.undo = append(oldUndo, snapshot{blocks: before, sel: sel})
	d.redo = nil
	d.coalesce = coalesceNone
	d.after = result
	// 拒绝的操作不应留下撤销或脏标记。
	var beforeMD Document
	beforeMD.blocks = before
	beforeMD.prefix = d.prefix
	if d.Markdown() == beforeMD.Markdown() {
		d.undo = oldUndo
		d.redo = oldRedo
		d.clean = oldClean
	}
	return result
}

// InsertParagraphAfter 在指定块后插入空段落，供退出代码、表格与只读占位使用。
func (d *Document) InsertParagraphAfter(blockIndex int) Selection {
	if blockIndex < 0 || blockIndex >= len(d.blocks) {
		return d.after
	}
	_, end := d.blockRange(blockIndex)
	d.remember(Selection{end, end})
	// 新段落需要独立空行，围栏与 Raw 的原始片段保持不变。
	b := &d.blocks[blockIndex]
	if !strings.HasSuffix(b.gap, "\n\n") {
		if strings.HasSuffix(b.source, "\n") || strings.HasSuffix(b.gap, "\n") {
			b.gap += "\n"
		} else {
			b.gap += "\n\n"
		}
	}
	d.blocks = insertEdit(d.blocks, blockIndex+1, editBlock{Block: Block{Kind: Paragraph}, dirty: true})
	d.reindex()
	start, _ := d.blockRange(blockIndex + 1)
	return d.noteAfter(Selection{start, start})
}

// CreateParagraphAfter 使用光标位置定位要退出的块。
func (d *Document) CreateParagraphAfter(at int) Selection {
	return d.InsertParagraphAfter(d.BlockIndexAt(d.clampAt(at)))
}

func trailingLineBreak(source string) string {
	if strings.HasSuffix(source, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(source, "\n") {
		return "\n"
	}
	return ""
}

func writeListRuns(b *strings.Builder, runs []Run, indent string) {
	var body strings.Builder
	writeRunsMD(&body, runs)
	b.WriteString(strings.ReplaceAll(body.String(), "\n", "\n"+indent+"    "))
}

// listIndent 返回列表项行首缩进：优先沿用原文，新建的嵌套项每层四个空格。
func listIndent(bl *editBlock) string {
	if bl.indent != "" || bl.Level <= 1 {
		return bl.indent
	}
	return strings.Repeat("    ", bl.Level-1)
}
