package richtext

import (
	"strings"
	"unicode/utf8"
)

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

// ReplaceAll 把全文里的 query 换成 replacement，整个操作是一步撤销。
// 空 query 返回 0 且选区不动。Raw、图片、行内公式和脚注引用里的匹配跳过；
// 普通代码块允许替换。替换继承命中处的样式。
// 返回替换处数，以及最后一处替换终点的折叠选区；没有替换时选区不动。
func (d *Document) ReplaceAll(query, replacement string) (int, Selection) {
	// 相同文本不会改文档。直接返回，保留原有的脏标记、撤销栈和选区。
	if query == "" || query == replacement || len(d.text) == 0 {
		return 0, d.after
	}
	q := []rune(query)
	type hit struct{ start, end int }
	var hits []hit
	for i := 0; i+len(q) <= len(d.text); {
		if !runesEqual(d.text[i:i+len(q)], q) || !d.replaceableSpan(i, i+len(q)) {
			i++
			continue
		}
		hits = append(hits, hit{i, i + len(q)})
		i += len(q)
	}
	if len(hits) == 0 {
		return 0, d.after
	}
	// 与 atomicEdit 一样：先记下原文和撤销栈，各处替换走现有 Replace，最后收成一步。
	before := cloneEdit(d.blocks)
	oldUndo := append([]snapshot(nil), d.undo...)
	oldRedo := append([]snapshot(nil), d.redo...)
	oldClean := d.clean
	oldAfter := d.after
	d.coalesce = coalesceNone
	sel := oldAfter
	shift := 0
	for _, h := range hits {
		at := h.start + shift
		// 删除会把命中处的样式一起删掉。整段替换继承命中起点的样式。
		marks, link := d.styleAt(at)
		out := d.atomicEdit(Selection{at, h.end + shift}, func(pos int) Selection {
			d.pending = &pendingMark{at: pos, marks: marks, set: ^Mark(0), link: link, hasL: true}
			return d.Insert(pos, replacement)
		})
		shift = out.End - h.end
		sel = Selection{out.End, out.End}
	}
	if len(d.undo) == len(oldUndo) {
		// 每一处都被拒绝，文档没动。
		d.redo = oldRedo
		d.clean = oldClean
		d.after = oldAfter
		return 0, oldAfter
	}
	d.undo = append(oldUndo, snapshot{blocks: before, sel: oldAfter})
	d.redo = nil
	d.coalesce = coalesceNone
	d.after = sel
	d.clean = false
	return len(hits), sel
}

// replaceableSpan 报告半开区间是否整段落在可替换文本里。
// 跨块、跨单元格、Raw、图片、公式和脚注引用都不算。
func (d *Document) replaceableSpan(start, end int) bool {
	if start < 0 || end > len(d.index) || start >= end {
		return false
	}
	first := d.index[start]
	if first.gap || first.block < 0 {
		return false
	}
	b := &d.blocks[first.block]
	switch b.Kind {
	case Raw, Image, Horizontal:
		return false
	case Code:
		for at := start; at < end; at++ {
			sp := d.index[at]
			if sp.block != first.block || sp.gap {
				return false
			}
		}
		return true
	case TableBlock:
		cell, local := d.tableCaret(first.block, start)
		if cell < 0 {
			return false
		}
		runs := d.cellRuns(b, cell)
		if end-start > runCount(runs)-local {
			return false
		}
		return spanMarksOK(runs, local, local+end-start)
	default:
		if !textKind(b.Kind) {
			return false
		}
		s0, _ := d.blockRange(first.block)
		for at := start; at < end; at++ {
			sp := d.index[at]
			if sp.block != first.block || sp.gap {
				return false
			}
		}
		return spanMarksOK(b.Runs, start-s0, end-s0)
	}
}

// spanMarksOK 拒绝落在公式或脚注引用上的区间。
func spanMarksOK(runs []Run, from, to int) bool {
	off := 0
	for _, r := range runs {
		n := utf8.RuneCountInString(r.Text)
		if off < to && off+n > from {
			if r.Marks&MarkMath != 0 {
				return false
			}
			if r.Image != nil || r.Link != nil && r.Link.Footnote != "" {
				return false
			}
		}
		off += n
	}
	return true
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
