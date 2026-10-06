package richtext

import (
	"strconv"
	"strings"
)

// InsertBlockAfter 把一段 Markdown 解析成块，插在 at 所在块之后。
// 当前块是空段落时直接替换它。返回落在首个新块开头的光标。
func (d *Document) InsertBlockAfter(at int, markdown string) Selection {
	at = d.clampAt(at)
	pieces := cloneEdit(Parse(markdown).blocks)
	if len(pieces) == 0 {
		return Selection{at, at}
	}
	d.remember(Selection{at, at})
	bi := d.BlockIndexAt(at)
	cur := d.blocks[bi]
	replace := cur.Kind == Paragraph && runCount(cur.Runs) == 0
	for i := range pieces {
		// 新块没有原文可回写，全部按模型重新生成；各自带行尾换行，块间再留一个空行。
		if pieces[i].Kind != Raw {
			pieces[i].dirty = true
			pieces[i].source = "\n"
		}
		pieces[i].gap = "\n"
	}
	last := &pieces[len(pieces)-1]
	last.gap = cur.gap
	if bi+1 < len(d.blocks) && cur.gap == "" {
		last.gap = "\n"
	}
	first := bi + 1
	keep := bi + 1
	if replace {
		first, keep = bi, bi
	} else if strings.HasSuffix(cur.source, "\n") {
		d.blocks[bi].gap = "\n"
	} else {
		d.blocks[bi].gap = ""
	}
	next := append([]editBlock(nil), d.blocks[:keep]...)
	next = append(next, pieces...)
	d.blocks = append(next, d.blocks[bi+1:]...)
	d.reindex()
	pos, _ := d.blockRange(first)
	return d.noteAfter(Selection{pos, pos})
}

// ShiftListLevel 把选区覆盖的列表项整体缩进或提升一层，返回是否有变化。
// 层级不会超过上一项加一，也不会小于一。
func (d *Document) ShiftListLevel(sel Selection, delta int) bool {
	if sel.Start > sel.End {
		sel.Start, sel.End = sel.End, sel.Start
	}
	from, to := d.BlockIndexAt(d.clampAt(sel.Start)), d.BlockIndexAt(d.clampAt(sel.End))
	if from >= 0 && from < len(d.blocks) && listContext(d.blocks[from]) >= 0 {
		before := cloneEdit(d.blocks)
		changed := false
		for i := from; i <= to && i < len(d.blocks); i++ {
			if d.shiftContainerItem(i, delta) {
				changed = true
			}
		}
		if changed {
			d.undo = append(d.undo, snapshot{blocks: before, sel: sel})
			d.redo = nil
			d.clean = false
			d.coalesce = coalesceNone
			d.reindex()
			d.noteAfter(sel)
		}
		return changed
	}
	levels := map[int]int{}
	for i := from; i <= to && i < len(d.blocks); i++ {
		b := d.blocks[i]
		if b.Kind != List && b.Kind != Task {
			continue
		}
		level := max(1, b.Level) + delta
		limit := 1
		if i > 0 && (d.blocks[i-1].Kind == List || d.blocks[i-1].Kind == Task) {
			prev := max(1, d.blocks[i-1].Level)
			if v, ok := levels[i-1]; ok {
				prev = v
			}
			limit = prev + 1
		}
		level = max(1, min(level, min(limit, 6)))
		if level != max(1, b.Level) {
			levels[i] = level
		}
	}
	if len(levels) == 0 {
		return false
	}
	d.remember(sel)
	for i := from; i <= to && i < len(d.blocks); i++ {
		level, ok := levels[i]
		if !ok {
			continue
		}
		b := &d.blocks[i]
		b.Level = level
		for ci := range b.Containers {
			if b.Containers[ci].Kind == List || b.Containers[ci].Kind == Task {
				b.Containers[ci].Level = level
			}
		}
		b.indent = d.listIndentFor(i, level)
		if b.Ordered {
			b.Start = d.listNumberFor(i, level)
		}
		b.dirty = true
	}
	d.reindex()
	d.noteAfter(sel)
	return true
}

// listIndentFor 推出第 i 块在 level 层应有的行首缩进：沿用同层兄弟，否则落在父项正文起点。
func (d *Document) listIndentFor(i, level int) string {
	if level <= 1 {
		return ""
	}
	for j := i - 1; j >= 0; j-- {
		p := d.blocks[j]
		if p.Kind != List && p.Kind != Task {
			break
		}
		pl := max(1, p.Level)
		if pl == level && (p.indent != "" || !p.dirty) {
			return p.indent
		}
		if pl == level-1 {
			marker := 2
			if p.Ordered {
				marker = len(strconv.Itoa(max(1, p.Start))) + 2
			}
			indent := p.indent
			for k := 0; k < marker; k++ {
				indent += " "
			}
			return indent
		}
		if pl < level-1 {
			break
		}
	}
	return ""
}

// listNumberFor 给移到 level 层的有序项接上同层前一项的编号。
func (d *Document) listNumberFor(i, level int) int {
	for j := i - 1; j >= 0; j-- {
		p := d.blocks[j]
		if p.Kind != List && p.Kind != Task {
			break
		}
		pl := max(1, p.Level)
		if pl < level {
			break
		}
		if pl == level && p.Ordered {
			return max(1, p.Start) + 1
		}
	}
	return 1
}

// TableCell 返回 at 所在的表格块、行、列。不在表格里时 ok 为 false。
func (d *Document) TableCell(at int) (block, row, col int, ok bool) {
	at = d.clampAt(at)
	bi := d.BlockIndexAt(at)
	if bi < 0 || bi >= len(d.blocks) || d.blocks[bi].Kind != TableBlock || d.blocks[bi].Table == nil {
		return 0, 0, 0, false
	}
	cell, _ := d.tableCaret(bi, at)
	if cell < 0 {
		return 0, 0, 0, false
	}
	for ri, r := range d.blocks[bi].Table.Rows {
		if cell < len(r) {
			return bi, ri, cell, true
		}
		cell -= len(r)
	}
	return 0, 0, 0, false
}

func (d *Document) tableOf(block int) *TableData {
	if block < 0 || block >= len(d.blocks) || d.blocks[block].Kind != TableBlock {
		return nil
	}
	return d.blocks[block].Table
}

// squareTable 把各行补齐到同样的列数，行列操作之后下标才一致。
func squareTable(tb *TableData) int {
	cols := len(tb.Aligns)
	for _, r := range tb.Rows {
		cols = max(cols, len(r))
	}
	for len(tb.Aligns) < cols {
		tb.Aligns = append(tb.Aligns, AlignNone)
	}
	for i := range tb.Rows {
		for len(tb.Rows[i]) < cols {
			tb.Rows[i] = append(tb.Rows[i], Cell{})
		}
	}
	return cols
}

// cellStart 返回表格某个单元格开头的文档偏移。
func (d *Document) cellStart(block, row, col int) int {
	pos, _ := d.blockRange(block)
	for ri, r := range d.blocks[block].Table.Rows {
		if ri > 0 {
			pos++
		}
		for ci, c := range r {
			if ci > 0 {
				pos++
			}
			if ri == row && ci == col {
				return pos
			}
			pos += runCount(c.Runs)
		}
	}
	return pos
}

func (d *Document) tableEdit(block int, sel Selection, edit func(tb *TableData, cols int) (row, col int, ok bool)) (Selection, bool) {
	tb := d.tableOf(block)
	if tb == nil {
		return sel, false
	}
	before := cloneEdit(d.blocks)
	cols := squareTable(tb)
	row, col, ok := edit(tb, cols)
	if !ok {
		d.blocks = before
		return sel, false
	}
	// 撤销快照取修改前的块，补齐列数也一并可撤销。
	after := d.blocks
	d.blocks = before
	d.remember(sel)
	d.blocks = after
	d.blocks[block].dirty = true
	d.reindex()
	at := d.cellStart(block, row, col)
	return d.noteAfter(Selection{at, at}), true
}

// TableInsertRow 在第 row 行之前插入空行；row 等于行数时追加在末尾。表头之前不能插入。
func (d *Document) TableInsertRow(block, row int, sel Selection) (Selection, bool) {
	return d.tableEdit(block, sel, func(tb *TableData, cols int) (int, int, bool) {
		first := 0
		if tb.Header {
			first = 1
		}
		if row < first || row > len(tb.Rows) {
			return 0, 0, false
		}
		tb.Rows = append(tb.Rows, nil)
		copy(tb.Rows[row+1:], tb.Rows[row:])
		tb.Rows[row] = make([]Cell, cols)
		return row, 0, true
	})
}

// TableDeleteRow 删除第 row 行。表头不能删除。
func (d *Document) TableDeleteRow(block, row int, sel Selection) (Selection, bool) {
	return d.tableEdit(block, sel, func(tb *TableData, cols int) (int, int, bool) {
		if row < 0 || row >= len(tb.Rows) || (tb.Header && row == 0) || len(tb.Rows) <= 1 {
			return 0, 0, false
		}
		tb.Rows = append(tb.Rows[:row], tb.Rows[row+1:]...)
		return min(row, len(tb.Rows)-1), 0, true
	})
}

// TableInsertColumn 在第 col 列之前插入空列；col 等于列数时追加在末尾。
func (d *Document) TableInsertColumn(block, col int, sel Selection) (Selection, bool) {
	return d.tableEdit(block, sel, func(tb *TableData, cols int) (int, int, bool) {
		if col < 0 || col > cols {
			return 0, 0, false
		}
		tb.Aligns = append(tb.Aligns, AlignNone)
		copy(tb.Aligns[col+1:], tb.Aligns[col:])
		tb.Aligns[col] = AlignNone
		for i := range tb.Rows {
			tb.Rows[i] = append(tb.Rows[i], Cell{})
			copy(tb.Rows[i][col+1:], tb.Rows[i][col:])
			tb.Rows[i][col] = Cell{}
		}
		return 0, col, true
	})
}

// TableDeleteColumn 删除第 col 列，至少保留一列。
func (d *Document) TableDeleteColumn(block, col int, sel Selection) (Selection, bool) {
	return d.tableEdit(block, sel, func(tb *TableData, cols int) (int, int, bool) {
		if col < 0 || col >= cols || cols <= 1 {
			return 0, 0, false
		}
		tb.Aligns = append(tb.Aligns[:col], tb.Aligns[col+1:]...)
		for i := range tb.Rows {
			tb.Rows[i] = append(tb.Rows[i][:col], tb.Rows[i][col+1:]...)
		}
		return 0, min(col, cols-2), true
	})
}

// TableSetAlign 设置第 col 列的对齐方式。
func (d *Document) TableSetAlign(block, col int, align Align, sel Selection) (Selection, bool) {
	return d.tableEdit(block, sel, func(tb *TableData, cols int) (int, int, bool) {
		if col < 0 || col >= cols || tb.Aligns[col] == align {
			return 0, 0, false
		}
		tb.Aligns[col] = align
		return 0, col, true
	})
}

// DeleteBlock 删除整个块，用于移除表格、图片等不能逐字删空的块。文档至少保留一个空段落。
func (d *Document) DeleteBlock(block int, sel Selection) (Selection, bool) {
	if block < 0 || block >= len(d.blocks) {
		return sel, false
	}
	d.remember(sel)
	gap := d.blocks[block].gap
	d.blocks = append(d.blocks[:block:block], d.blocks[block+1:]...)
	if len(d.blocks) == 0 {
		d.blocks = []editBlock{{Block: Block{Kind: Paragraph}, dirty: true}}
	} else if block == len(d.blocks) && block > 0 {
		d.blocks[block-1].gap = gap
	}
	d.reindex()
	at, _ := d.blockRange(min(block, len(d.blocks)-1))
	return d.noteAfter(Selection{at, at}), true
}
