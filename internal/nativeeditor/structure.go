package nativeeditor

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// insertable 是格式栏可插入的整块内容，光标落在新块开头。
var insertable = map[string]string{
	"table":     "|  |  |\n| --- | --- |\n|  |  |\n",
	"codeblock": "```\n\n```\n",
	"hr":        "---\n",
}

// insertBlock 在当前块之后插入表格、代码块或分隔线。
func (e *Editor) insertBlock(name string) bool {
	markdown, ok := insertable[name]
	if !ok {
		return false
	}
	e.cancelCompose()
	sel := e.doc.InsertBlockAfter(e.selection().End, markdown)
	if name == "hr" {
		// 分隔线之后接着写正文。
		sel = e.doc.CreateParagraphAfter(sel.Start)
	}
	e.anchor, e.focus = sel.Start, sel.End
	return true
}

// tabKey 处理 Tab 与 Shift+Tab：列表项增减层级，表格里在单元格间移动。
func (e *Editor) tabKey(back bool) bool {
	sel := e.selection()
	if block, row, col, ok := e.doc.TableCell(e.focus); ok {
		e.cancelCompose()
		e.moveCell(block, row, col, back)
		return true
	}
	blocks := e.doc.Blocks()
	bi := e.doc.BlockIndexAt(sel.Start)
	if bi < 0 || bi >= len(blocks) || !isListItem(blocks[bi]) {
		return false
	}
	delta := 1
	if back {
		delta = -1
	}
	e.cancelCompose()
	if e.doc.ShiftListLevel(sel, delta) {
		e.dirty = true
	}
	return true
}

// moveCell 把光标移到下一个或上一个单元格；在最后一格按 Tab 追加一行。
func (e *Editor) moveCell(block, row, col int, back bool) {
	table := e.doc.Blocks()[block].Table
	if table == nil {
		return
	}
	cells := 0
	index := 0
	for ri, r := range table.Rows {
		if ri < row {
			index += len(r)
		}
		cells += len(r)
	}
	index += col
	if back {
		index--
	} else {
		index++
	}
	if index < 0 {
		return
	}
	if index >= cells {
		if sel, ok := e.doc.TableInsertRow(block, len(table.Rows), e.selection()); ok {
			e.anchor, e.focus = sel.Start, sel.End
			e.dirty, e.reveal = true, true
		}
		return
	}
	start, _, _, _ := e.cellSpan(block, index)
	e.anchor, e.focus = start, start
	e.reveal = true
}

// cellSpan 返回表格里第 index 个单元格（按行展开）的文档区间。
func (e *Editor) cellSpan(block, index int) (start, end, row, col int) {
	origin := e.blockOrigin(block)
	table := e.doc.Blocks()[block].Table
	pos, n := origin, 0
	for ri, r := range table.Rows {
		if ri > 0 {
			pos++
		}
		for ci, cell := range r {
			if ci > 0 {
				pos++
			}
			size := len([]rune(visibleRuns(cell.Runs)))
			if n == index {
				return pos, pos + size, ri, ci
			}
			pos += size
			n++
		}
	}
	return pos, pos, 0, 0
}

// contextMenu 是正文的右键菜单：剪贴板命令，以及表格里的行列操作。
func (e *Editor) contextMenu(c *ui.Context, m *ui.Menu) {
	sel := e.selection()
	empty := sel.Start == sel.End
	if m.Item("剪切").Disabled(empty).Chosen() {
		e.copy(c, true)
	}
	if m.Item("复制").Disabled(empty).Chosen() {
		e.copy(c, false)
	}
	if m.Item("粘贴").Chosen() {
		e.paste(c)
	}
	block, row, col, ok := e.doc.TableCell(e.focus)
	if !ok {
		return
	}
	table := e.doc.Blocks()[block].Table
	header := table != nil && table.Header && row == 0
	apply := func(next richtext.Selection, changed bool) {
		if changed {
			e.cancelCompose()
			e.anchor, e.focus = next.Start, next.End
			e.dirty, e.reveal = true, true
		}
	}
	m.Separator()
	if m.Item("在上方插入行").Disabled(header).Chosen() {
		apply(e.doc.TableInsertRow(block, row, sel))
	}
	if m.Item("在下方插入行").Chosen() {
		apply(e.doc.TableInsertRow(block, row+1, sel))
	}
	if m.Item("在左侧插入列").Chosen() {
		apply(e.doc.TableInsertColumn(block, col, sel))
	}
	if m.Item("在右侧插入列").Chosen() {
		apply(e.doc.TableInsertColumn(block, col+1, sel))
	}
	m.Separator()
	m.Submenu("对齐", func(m *ui.Menu) {
		for _, item := range []struct {
			label string
			align richtext.Align
		}{{"左对齐", richtext.AlignLeft}, {"居中", richtext.AlignCenter}, {"右对齐", richtext.AlignRight}, {"默认", richtext.AlignNone}} {
			if m.Item(item.label).Chosen() {
				apply(e.doc.TableSetAlign(block, col, item.align, sel))
			}
		}
	})
	m.Separator()
	if m.Item("删除行").Disabled(header).Chosen() {
		apply(e.doc.TableDeleteRow(block, row, sel))
	}
	if m.Item("删除列").Disabled(table == nil || len(table.Aligns) <= 1).Chosen() {
		apply(e.doc.TableDeleteColumn(block, col, sel))
	}
	if m.Item("删除表格").Chosen() {
		apply(e.doc.DeleteBlock(block, sel))
	}
}

// SetEditRaw 设置双击原文占位块（公式、图表、HTML 等）时的回调，由宿主弹出源码编辑框。
func (e *Editor) SetEditRaw(fn func(index int, source string)) { e.editRaw = fn }

// requestRawEdit 在 off 落在原文占位块上时请求宿主编辑它的源码。
func (e *Editor) requestRawEdit(off int) bool {
	if e.editRaw == nil {
		return false
	}
	blocks := e.doc.Blocks()
	bi := e.doc.BlockIndexAt(off)
	if bi < 0 || bi >= len(blocks) || blocks[bi].Kind != richtext.Raw {
		return false
	}
	e.cancelCompose()
	e.dragging = false
	e.anchor, e.focus = off, off
	e.editRaw(bi, strings.TrimRight(blocks[bi].Raw, " \t\r\n"))
	return true
}

// ReplaceRaw 用新的 Markdown 替换第 index 个原文占位块，返回是否成功。
// 新内容重新解析：改成受支持的语法后即成为可直接编辑的块。
func (e *Editor) ReplaceRaw(index int, markdown string) bool {
	blocks := e.doc.Blocks()
	if index < 0 || index >= len(blocks) || blocks[index].Kind != richtext.Raw || strings.TrimSpace(blocks[index].Raw) == strings.TrimSpace(markdown) {
		return false
	}
	if strings.TrimSpace(markdown) == "" {
		sel, ok := e.doc.DeleteBlock(index, e.selection())
		if ok {
			e.anchor, e.focus = sel.Start, sel.End
			e.dirty, e.reveal = true, true
		}
		return ok
	}
	if e.doc.ReplaceRaw(index, markdown) != nil {
		return false
	}
	at := e.blockOrigin(index)
	e.anchor, e.focus = at, at
	e.dirty, e.reveal = true, true
	return true
}
