package richtext

import "unicode/utf8"

// reindex 根据块重建纯文本和 rune 到块的映射。
func (d *Document) reindex() {
	d.text = d.text[:0]
	d.index = d.index[:0]
	for i := range d.blocks {
		b := &d.blocks[i]
		switch b.Kind {
		case Code:
			d.appendRunes(b.Code, i, -1, 0)
		case Horizontal, Image, Raw:
			d.appendRunes(objectReplacement, i, -1, -1)
		case TableBlock:
			d.appendTable(i, b.Table)
		default:
			d.appendRuns(i, -1, b.Runs)
		}
		if i != len(d.blocks)-1 {
			d.text = append(d.text, '\n')
			d.index = append(d.index, span{block: i, cell: -1, run: -1, gap: true})
		}
	}
}

func (d *Document) appendRuns(block, cell int, runs []Run) {
	if len(runs) == 0 {
		return
	}
	for ri := range runs {
		rs := []rune(runs[ri].Text)
		for off := range rs {
			d.text = append(d.text, rs[off])
			d.index = append(d.index, span{block: block, cell: cell, run: ri, off: off})
		}
	}
}

func (d *Document) appendRunes(s string, block, cell, run int) {
	rs := []rune(s)
	for off := range rs {
		d.text = append(d.text, rs[off])
		d.index = append(d.index, span{block: block, cell: cell, run: run, off: off})
	}
}

func (d *Document) appendTable(block int, tb *TableData) {
	if tb == nil {
		return
	}
	cell := 0
	for r, row := range tb.Rows {
		if r > 0 {
			d.text = append(d.text, '\n')
			d.index = append(d.index, span{block: block, cell: -1, run: -1, gap: true})
		}
		for c, col := range row {
			if c > 0 {
				d.text = append(d.text, '\t')
				d.index = append(d.index, span{block: block, cell: -1, run: -1, gap: true})
			}
			d.appendRuns(block, cell, col.Runs)
			cell++
		}
	}
}

// Text 返回可见正文。块之间一个换行，末块后不加。
func (d *Document) Text() string {
	return string(d.text)
}

// Len 返回纯文本的 rune 数。
func (d *Document) Len() int {
	return len(d.text)
}

// Blocks 返回块的副本。
func (d *Document) Blocks() []Block {
	out := make([]Block, len(d.blocks))
	for i := range d.blocks {
		out[i] = cloneBlock(d.blocks[i].Block)
	}
	return out
}

// Changed 报告相对 Parse 或最近一次 MarkClean 是否有编辑。
func (d *Document) Changed() bool {
	return !d.clean
}

// MarkClean 把当前内容记为已保存。
func (d *Document) MarkClean() {
	d.clean = true
}

// BlockIndexAt 返回 rune 偏移所在的块。越界时夹到最后一块。
func (d *Document) BlockIndexAt(at int) int {
	if len(d.blocks) == 0 {
		return 0
	}
	if at < 0 {
		return 0
	}
	if at >= len(d.index) {
		return len(d.blocks) - 1
	}
	return d.index[at].block
}

// MarksAt 返回该 rune 的行内样式。光标位置取待输入样式或左侧字符。
func (d *Document) MarksAt(at int) Mark {
	if d.pending != nil && d.pending.at == at {
		base := d.marksNear(at)
		return (base &^ d.pending.set) | (d.pending.marks & d.pending.set)
	}
	return d.marksNear(at)
}

func (d *Document) marksNear(at int) Mark {
	if at < 0 || at >= len(d.index) {
		if at > 0 {
			return d.marksOf(at - 1)
		}
		return 0
	}
	return d.marksOf(at)
}

func (d *Document) marksOf(at int) Mark {
	sp := d.index[at]
	if sp.gap || sp.run < 0 {
		return 0
	}
	runs := d.runsAt(sp)
	if runs == nil || sp.run >= len(runs) {
		return 0
	}
	return runs[sp.run].Marks
}

// LinkAt 返回该位置的链接，没有时为 nil。
func (d *Document) LinkAt(at int) *Link {
	if d.pending != nil && d.pending.at == at && d.pending.hasL {
		return cloneLink(d.pending.link)
	}
	if at < 0 || at >= len(d.index) {
		if at > 0 {
			at--
		} else {
			return nil
		}
	}
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

func (d *Document) runsAt(sp span) []Run {
	if sp.block < 0 || sp.block >= len(d.blocks) {
		return nil
	}
	b := &d.blocks[sp.block]
	if b.Kind == TableBlock {
		return d.cellRuns(b, sp.cell)
	}
	return b.Runs
}

func (d *Document) cellRuns(b *editBlock, cell int) []Run {
	if b.Table == nil || cell < 0 {
		return nil
	}
	n := 0
	for _, row := range b.Table.Rows {
		for _, c := range row {
			if n == cell {
				return c.Runs
			}
			n++
		}
	}
	return nil
}

func (d *Document) setCellRuns(b *editBlock, cell int, runs []Run) {
	if b.Table == nil {
		return
	}
	n := 0
	for ri := range b.Table.Rows {
		for ci := range b.Table.Rows[ri] {
			if n == cell {
				b.Table.Rows[ri][ci].Runs = runs
				return
			}
			n++
		}
	}
}

// blockRange 返回块在纯文本中的半开区间，不含后面的结构换行。
func (d *Document) blockRange(i int) (int, int) {
	start, end := -1, -1
	for at, sp := range d.index {
		if sp.block != i {
			continue
		}
		if sp.gap && sp.run < 0 && (at+1 >= len(d.index) || d.index[at+1].block != i) {
			continue
		}
		if start < 0 {
			start = at
		}
		end = at + 1
	}
	if start < 0 {
		// 空块：位置是前导结构换行之后。
		pos := 0
		for bi := 0; bi < i; bi++ {
			pos += d.blockRuneLen(bi) + 1
		}
		return pos, pos
	}
	return start, end
}

func (d *Document) blockRuneLen(i int) int {
	b := &d.blocks[i]
	switch b.Kind {
	case Code:
		return utf8.RuneCountInString(b.Code)
	case Horizontal, Image, Raw:
		return 1
	case TableBlock:
		return tableRuneLen(b.Table)
	default:
		n := 0
		for _, r := range b.Runs {
			n += utf8.RuneCountInString(r.Text)
		}
		return n
	}
}

func tableRuneLen(tb *TableData) int {
	if tb == nil || len(tb.Rows) == 0 {
		return 0
	}
	n := 0
	for r, row := range tb.Rows {
		if r > 0 {
			n++
		}
		for c, cell := range row {
			if c > 0 {
				n++
			}
			for _, run := range cell.Runs {
				n += utf8.RuneCountInString(run.Text)
			}
		}
	}
	return n
}

func cloneBlock(b Block) Block {
	out := b
	out.Runs = cloneRuns(b.Runs)
	if b.Callout != nil {
		callout := *b.Callout
		out.Callout = &callout
	}
	if b.Table != nil {
		tb := *b.Table
		tb.Aligns = append([]Align(nil), b.Table.Aligns...)
		tb.Rows = make([][]Cell, len(b.Table.Rows))
		for i, row := range b.Table.Rows {
			tb.Rows[i] = make([]Cell, len(row))
			for j, cell := range row {
				tb.Rows[i][j] = Cell{Runs: cloneRuns(cell.Runs)}
			}
		}
		out.Table = &tb
	}
	return out
}

func cloneRuns(runs []Run) []Run {
	if len(runs) == 0 {
		return nil
	}
	out := make([]Run, len(runs))
	for i, r := range runs {
		out[i] = r
		out[i].Link = cloneLink(r.Link)
	}
	return out
}
