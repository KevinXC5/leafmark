package richtext

import (
	"strings"
	"testing"
)

func TestInsertBlockAfterAddsTableCodeAndRule(t *testing.T) {
	for _, item := range []struct {
		name, markdown string
		kind           Kind
	}{
		{"表格", "|  |  |\n| --- | --- |\n|  |  |\n", TableBlock},
		{"代码块", "```\n\n```\n", Code},
		{"分隔线", "---\n", Horizontal},
	} {
		t.Run(item.name, func(t *testing.T) {
			d := Parse("正文\n\n末尾\n")
			sel := d.InsertBlockAfter(1, item.markdown)
			blocks := d.Blocks()
			if len(blocks) != 3 || blocks[1].Kind != item.kind {
				t.Fatalf("插入块错误：%+v", blocks)
			}
			if start, _ := d.blockRange(1); sel.Start != start {
				t.Fatalf("光标不在新块开头：%v", sel)
			}
			again := Parse(d.Markdown()).Blocks()
			if len(again) != 3 || again[1].Kind != item.kind || !strings.HasSuffix(d.Markdown(), "末尾\n") {
				t.Fatalf("回写错误：%q", d.Markdown())
			}
			if _, ok := d.Undo(); !ok || d.Markdown() != "正文\n\n末尾\n" {
				t.Fatalf("撤销未恢复原文：%q", d.Markdown())
			}
		})
	}
	d := Parse("")
	d.InsertBlockAfter(0, "---\n")
	if blocks := d.Blocks(); len(blocks) != 1 || blocks[0].Kind != Horizontal {
		t.Fatalf("空段落未被替换：%+v", blocks)
	}
}

func TestShiftListLevelKeepsValidNesting(t *testing.T) {
	source := "- 一\n- 二\n- 三\n\n1. 甲\n2. 乙\n"
	d := Parse(source)
	if d.ShiftListLevel(Selection{0, 0}, 1) {
		t.Fatal("首项没有父项，不应缩进")
	}
	if !d.ShiftListLevel(Selection{2, 2}, 1) {
		t.Fatal("第二项应可缩进")
	}
	if d.ShiftListLevel(Selection{2, 2}, 1) {
		t.Fatal("层级不能超过上一项加一")
	}
	if got := d.Markdown(); got != "- 一\n  - 二\n- 三\n\n1. 甲\n2. 乙\n" {
		t.Fatalf("缩进回写错误：%q", got)
	}
	blocks := Parse(d.Markdown()).Blocks()
	if blocks[1].Level != 2 || blocks[2].Level != 1 {
		t.Fatalf("重新解析层级错误：%+v", blocks)
	}
	start, _ := d.blockRange(4)
	if !d.ShiftListLevel(Selection{start, start}, 1) {
		t.Fatal("有序项应可缩进")
	}
	again := Parse(d.Markdown()).Blocks()
	if again[4].Level != 2 || !again[4].Ordered || again[4].Start != 1 {
		t.Fatalf("有序嵌套错误：%q %+v", d.Markdown(), again[4])
	}
	d.ShiftListLevel(Selection{start, start}, -1)
	d.ShiftListLevel(Selection{2, 2}, -1)
	if got := Parse(d.Markdown()); got.Blocks()[1].Level != 1 || got.Blocks()[4].Level != 1 {
		t.Fatalf("提升层级错误：%q", d.Markdown())
	}
	for d.Changed() {
		if _, ok := d.Undo(); !ok {
			break
		}
	}
	if d.Markdown() != source {
		t.Fatalf("撤销未恢复原文：%q", d.Markdown())
	}
}

func TestTableRowAndColumnEditing(t *testing.T) {
	source := "| 甲 | 乙 |\n| :--- | ---: |\n| 1 | 2 |\n"
	d := Parse(source)
	block, row, col, ok := d.TableCell(4)
	if !ok || block != 0 || row != 1 || col != 0 {
		t.Fatalf("单元格定位错误：%d %d %d %v", block, row, col, ok)
	}
	if _, ok := d.TableInsertRow(0, 0, Selection{}); ok {
		t.Fatal("表头之前不能插入行")
	}
	sel, ok := d.TableInsertRow(0, 2, Selection{})
	if !ok || d.Text() != "甲\t乙\n1\t2\n\t" || sel.Start != 8 {
		t.Fatalf("追加行错误：%q %v", d.Text(), sel)
	}
	d.Insert(sel.Start, "3")
	if _, ok := d.TableInsertColumn(0, 1, Selection{}); !ok || d.Text() != "甲\t\t乙\n1\t\t2\n3\t\t" {
		t.Fatalf("插入列错误：%q", d.Text())
	}
	again := Parse(d.Markdown())
	tb := again.Blocks()[0].Table
	if again.Text() != d.Text() || len(tb.Aligns) != 3 || tb.Aligns[0] != AlignLeft || tb.Aligns[2] != AlignRight {
		t.Fatalf("表格回写错误：%q", d.Markdown())
	}
	if _, ok := d.TableDeleteRow(0, 0, Selection{}); ok {
		t.Fatal("表头不能删除")
	}
	d.TableDeleteColumn(0, 1, Selection{})
	d.TableDeleteRow(0, 2, Selection{})
	if d.Text() != "甲\t乙\n1\t2" {
		t.Fatalf("删除行列错误：%q", d.Text())
	}
	d.TableDeleteColumn(0, 0, Selection{})
	if _, ok := d.TableDeleteColumn(0, 0, Selection{}); ok {
		t.Fatal("至少保留一列")
	}
	if _, ok := d.TableSetAlign(0, 0, AlignCenter, Selection{}); !ok || !strings.Contains(d.Markdown(), ":---:") && !strings.Contains(d.Markdown(), ":-:") {
		t.Fatalf("对齐未写回：%q", d.Markdown())
	}
	for i := 0; i < 20; i++ {
		if _, ok := d.Undo(); !ok {
			break
		}
	}
	if d.Markdown() != source {
		t.Fatalf("撤销未恢复原文：%q", d.Markdown())
	}
}

func TestDeleteBlockRemovesTableAndKeepsNeighbours(t *testing.T) {
	source := "前\n\n| a |\n| --- |\n\n后\n"
	d := Parse(source)
	if _, ok := d.DeleteBlock(1, Selection{}); !ok || d.Markdown() != "前\n\n后\n" {
		t.Fatalf("删除块错误：%q", d.Markdown())
	}
	d.Undo()
	if d.Markdown() != source {
		t.Fatalf("撤销未恢复原文：%q", d.Markdown())
	}
	d = Parse("---\n")
	d.DeleteBlock(0, Selection{})
	if blocks := d.Blocks(); len(blocks) != 1 || blocks[0].Kind != Paragraph {
		t.Fatalf("文档应保留空段落：%+v", blocks)
	}
}
