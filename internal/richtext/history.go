package richtext

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
