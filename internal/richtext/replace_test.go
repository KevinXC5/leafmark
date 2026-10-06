package richtext

import (
	"strings"
	"testing"
)

func TestNewPlainTextKeepsFencesAndTrailingBreaks(t *testing.T) {
	for _, text := range []string{
		"",
		"\n",
		"\n\n",
		"# 标题\n\n```go\nfmt.Println(\"甲\")\n```\n\n见 **甲**\n\n",
		"末尾没有换行",
		"\r\n",
	} {
		d := NewPlainText(text)
		if d.Text() != text || d.Markdown() != text {
			t.Fatalf("纯文本被改写：text=%q md=%q", d.Text(), d.Markdown())
		}
		blocks := d.Blocks()
		if len(blocks) != 1 || blocks[0].Kind != Code || blocks[0].Code != text {
			t.Fatalf("纯文本不是单个代码块：%+v", blocks)
		}
		if strings.Count(d.Text(), "```") != strings.Count(text, "```") {
			t.Fatalf("围栏被当成代码块边界：%q", d.Text())
		}
		// 中间插入、删除可撤销，撤销后回到原文。
		if text == "" {
			continue
		}
		d.Insert(0, "新")
		if d.Text() != "新"+text || d.Markdown() != "新"+text {
			t.Fatalf("插入后 = %q", d.Markdown())
		}
		if sel, ok := d.Undo(); !ok || sel != (Selection{0, 0}) || d.Markdown() != text {
			t.Fatalf("撤销后 = %q sel=%v", d.Markdown(), sel)
		}
		if sel, ok := d.Redo(); !ok || d.Markdown() != "新"+text || sel.Start != 1 {
			t.Fatalf("重做后 = %q sel=%v", d.Markdown(), sel)
		}
	}
}

func TestReplaceAllOneUndoKeepsMarksAndSkipsAtoms(t *testing.T) {
	source := "甲 **甲** [甲](https://example.com) `甲`\n\n```\n甲\n```\n\n| 甲 | 乙 |\n| --- | --- |\n\n甲见 $x^甲$ 与甲[^note]。\n\n![甲图](a.png)\n\n[^note]: 甲的定义\n"
	d := Parse(source)
	before := d.Markdown()
	// 先做一次无关编辑再撤销，确认替换不会清掉重做栈。
	d.Insert(0, "前")
	d.Undo()
	if _, ok := d.Redo(); !ok {
		t.Fatal("预备重做丢失")
	}
	d.Undo()

	dirty := d.Changed()
	n, sel := d.ReplaceAll("甲", "甲")
	if n != 0 || d.Markdown() != before || d.Changed() != dirty {
		t.Fatalf("相同文本不应替换：n=%d changed=%v md=%q", n, d.Changed(), d.Markdown())
	}
	if _, ok := d.Redo(); !ok {
		t.Fatal("相同替换清掉了重做")
	}
	d.Undo() // 回到插入之前，重做栈里仍是那一次插入

	n, sel = d.ReplaceAll("", "乙")
	if n != 0 || d.Markdown() != before {
		t.Fatalf("空查询不应替换：%d %q", n, d.Markdown())
	}

	n, sel = d.ReplaceAll("甲", "丙丁")
	if n == 0 || strings.Contains(d.Text(), "$x^丙丁$") || strings.Contains(d.Markdown(), "[^丙丁]") {
		t.Fatalf("公式或脚注被替换：%d %q", n, d.Markdown())
	}
	if strings.Contains(d.Markdown(), "![丙丁图]") {
		t.Fatalf("图片被替换：%q", d.Markdown())
	}
	again := Parse(d.Markdown())
	var bold, linked, coded, cell, fence bool
	for _, b := range again.Blocks() {
		switch b.Kind {
		case Code:
			if strings.Contains(b.Code, "丙丁") {
				fence = true
			}
		case TableBlock:
			for _, row := range b.Table.Rows {
				for _, c := range row {
					if plainRuns(c.Runs) == "丙丁" {
						cell = true
					}
				}
			}
		default:
			for _, r := range b.Runs {
				if !strings.Contains(r.Text, "丙丁") {
					continue
				}
				if r.Marks&MarkBold != 0 {
					bold = true
				}
				if r.Marks&MarkCode != 0 {
					coded = true
				}
				if r.Link != nil && r.Link.URL == "https://example.com" {
					linked = true
				}
			}
		}
	}
	if !bold || !linked || !coded || !cell || !fence {
		t.Fatalf("样式或位置丢失 bold=%v link=%v code=%v cell=%v fence=%v\n%q", bold, linked, coded, cell, fence, d.Markdown())
	}
	if sel.Start != sel.End || sel.Start == 0 {
		t.Fatalf("结束后选区应折叠在最后一处：%v", sel)
	}
	if back, ok := d.Undo(); !ok || back != (Selection{}) && d.Markdown() != before {
		if d.Markdown() != before {
			t.Fatalf("一步撤销未恢复：%q sel=%v", d.Markdown(), back)
		}
	}
	if _, ok := d.Undo(); ok {
		t.Fatal("全部替换被拆成多步")
	}
	if _, ok := d.Redo(); !ok || strings.Contains(d.Text(), "甲 **") {
		t.Fatalf("重做失败：%q", d.Markdown())
	}
}

func TestReplaceAllPartialRunKeepsSurroundingMarks(t *testing.T) {
	d := Parse("前**甲中甲**后\n")
	n, _ := d.ReplaceAll("甲", "乙丙")
	if n != 2 {
		t.Fatalf("处数 = %d", n)
	}
	again := Parse(d.Markdown()).Blocks()[0]
	if plainRuns(again.Runs) != "前乙丙中乙丙后" {
		t.Fatalf("正文 = %q md=%q", plainRuns(again.Runs), d.Markdown())
	}
	// 两处「甲」都在加粗里。替换继承命中起点的样式，普通的「前」「后」不动。
	if len(again.Runs) != 3 || again.Runs[0].Text != "前" || again.Runs[0].Marks != 0 || again.Runs[1].Text != "乙丙中乙丙" || again.Runs[1].Marks&MarkBold == 0 || again.Runs[2].Text != "后" || again.Runs[2].Marks != 0 {
		t.Fatalf("跨 run 替换丢失加粗：%+v", again.Runs)
	}
	if _, ok := d.Undo(); !ok || d.Markdown() != "前**甲中甲**后\n" {
		t.Fatalf("撤销后 = %q", d.Markdown())
	}
}

func TestHTMLQuoteAndEntity(t *testing.T) {
	source := "<div><details title=\"a &gt; b\" open><summary>甲&amp;lt;</summary><p>段&amp;amp;</p></details></div>\n"
	d := Parse(source)
	if d.Markdown() != source {
		t.Fatalf("HTML 原文变化：%q", d.Markdown())
	}
	blocks := d.Blocks()
	if len(blocks) != 1 || len(blocks[0].Containers) != 1 || !blocks[0].Containers[0].IsHTML() {
		t.Fatalf("details 未解析：%+v", blocks)
	}
	if blocks[0].Containers[0].Callout.Title != "甲&lt;" || plainRuns(blocks[0].Runs) != "段&amp;" {
		t.Fatalf("实体被二次解码：title=%q text=%q", blocks[0].Containers[0].Callout.Title, plainRuns(blocks[0].Runs))
	}
	if blocks[0].Containers[0].Callout.Fold != "+" {
		t.Fatalf("open 未识别：%q", blocks[0].Containers[0].Callout.Fold)
	}
}

func TestStructuredSyntaxRoundTrips(t *testing.T) {
	for _, source := range []string{
		"前 ![甲图](a.png) 后\n",
		"见 [链接][ref] 和文字\n\n![图][img]\n\n[ref]: https://example.com \"标题\"\n[img]: a.png\n",
		"正文\n\n    缩进代码\n    第二行\n",
		"> [!WARNING]- 标题\n> 内容\n>\n> 第二段\n",
		"- 甲项\n\n  续段正文\n\n  ```go\n  x := 1\n  ```\n- 乙项\n",
		"<u>线</u>与<kbd>键</kbd>\n",
		"<div><details open><summary>标题</summary><p>段1</p><p>段2</p></details></div>\n",
		"甲见 $x^甲$ 与甲[^note]。\n\n[^note]: 甲的定义\n",
	} {
		d := Parse(source)
		if d.Markdown() != source {
			t.Fatalf("未编辑原文变化：%q -> %q", source, d.Markdown())
		}
		for _, b := range d.Blocks() {
			if b.Kind == Raw {
				t.Fatalf("新语法退回 Raw：%q", source)
			}
		}
		d.Insert(0, "增")
		if !strings.Contains(d.Text(), "增") {
			t.Fatalf("编辑无效：%q", d.Markdown())
		}
		if _, ok := d.Undo(); !ok || d.Markdown() != source {
			t.Fatalf("撤销未恢复原文：%q", d.Markdown())
		}
	}
	img := Parse("前 ![甲图](a.png) 后\n").Blocks()[0]
	if img.Kind != Paragraph || len(img.Runs) != 3 || img.Runs[1].Image == nil || img.Runs[1].Image.URL != "a.png" {
		t.Fatalf("行内图片未结构化：%+v", img.Runs)
	}
	ref := Parse("见 [链接][ref]\n\n[ref]: https://example.com\n")
	if ref.Text() != "见 链接" || ref.Blocks()[1].Kind != ReferenceDef {
		t.Fatalf("引用定义未隐藏：%q %+v", ref.Text(), ref.Blocks())
	}
	code := Parse("正文\n\n    缩进代码\n")
	if code.Blocks()[1].Kind != Code || !strings.Contains(code.Blocks()[1].Code, "缩进代码") {
		t.Fatalf("缩进代码未识别：%+v", code.Blocks())
	}
	details := Parse("<div><details open><summary>标题</summary><p>段1</p><p>段2</p></details></div>\n")
	if len(details.Blocks()) != 2 || len(details.Blocks()[0].Containers) == 0 || !details.Blocks()[0].Containers[0].IsHTML() || details.Blocks()[0].Containers[0].Callout.Fold != "+" {
		t.Fatalf("details 结构错误：%+v", details.Blocks())
	}
}
