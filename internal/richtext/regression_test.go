package richtext

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func TestSamplesRepeatedConcurrentParsingPreservesSource(t *testing.T) {
	var samples []string
	for _, name := range []string{"山中来信", "叶脉笔记"} {
		data, err := os.ReadFile("../../tests/fixtures/samples/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, string(data))
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repeat := 0; repeat < 30; repeat++ {
				for _, source := range samples {
					d := Parse(source)
					if d.Markdown() != source {
						t.Errorf("样例原文回写变化：%q", d.Markdown())
						return
					}
					if !strings.Contains(d.Text(), "落笔成章") && !strings.Contains(d.Text(), "光合产物") {
						t.Errorf("样例正常正文丢失：%q", d.Text())
						return
					}
				}
			}
		}()
	}
	wg.Wait()
}

func TestUnsupportedDisplayMathAndDiagramRemainReadOnly(t *testing.T) {
	for _, source := range []string{"$$\nQ_n = Q \\cdot r^n\n$$\n", "```mermaid\ngraph LR\nA-->B\n```\n"} {
		d := Parse(source)
		if d.Blocks()[0].Kind != Raw {
			t.Fatalf("复杂语法不应显示为正文源码：%q", source)
		}
		d.Insert(0, "误输入")
		if d.Markdown() != source {
			t.Fatalf("复杂原文被改写：%q", d.Markdown())
		}
	}
}

func TestSourcePreservedAroundEditedBlock(t *testing.T) {
	for _, source := range []string{
		"\n\n标题\n===\n\n正文\n\n<div>复杂</div>\n",
		"开头\r\n\r\n- 外层\r\n  - 内层\r\n\r\n末尾\r\n",
		"正文\n\n[引用]: https://example.com \"标题\"\n",
		"正文\n\ntext ![图片](image.png) tail\n",
		"正文\n\n```mermaid\ngraph TD\nA-->B\n```\n",
	} {
		d := Parse(source)
		if got := d.Markdown(); got != source {
			t.Fatalf("未编辑内容变化：%q != %q", got, source)
		}
		d.Insert(0, "增")
		// 只允许第一块变化，其他块及空白逐字节保留。
		split := strings.Index(source, "\n\n")
		if strings.HasPrefix(source, "\n") {
			split = strings.Index(source[2:], "\n\n") + 2
		}
		if split < 0 {
			split = strings.Index(source, "\r\n\r\n")
		}
		if split >= 0 && !strings.HasSuffix(d.Markdown(), source[split:]) {
			t.Fatalf("相邻源码丢失：%q", d.Markdown())
		}
	}
}

func TestWhitespaceAndRepeatedBlocksRoundTrip(t *testing.T) {
	for _, s := range []string{"\n\n", "  \n\nhello\n\n", "a\n\na\n\na\n", "\r\n\r\nx\r\n", "```\n```\n\ntext\n"} {
		if got := Parse(s).Markdown(); got != s {
			t.Fatalf("%q -> %q", s, got)
		}
	}
}

func TestSimpleListsRemainEditableAndPreserveOtherItems(t *testing.T) {
	for _, source := range []string{"- 一\n- 二\n- 三\n", "3) 一\n4) 二\n5) 三\n", "- [ ] 一\n- [x] 二\n- [ ] 三\n", "* 一\n\n* 二\n\n* 三\n"} {
		d := Parse(source)
		if d.Text() != "一\n二\n三" || len(d.Blocks()) != 3 {
			t.Fatalf("列表不可编辑：%q %+v", d.Text(), d.Blocks())
		}
		if d.Markdown() != source {
			t.Fatalf("列表源码变化：%q", d.Markdown())
		}
		d.Insert(3, "新")
		if got := Parse(d.Markdown()).Text(); got != "一\n二新\n三" {
			t.Fatalf("编辑后列表变化：%q -> %q", d.Markdown(), got)
		}
		lines := strings.Split(source, "\n")
		if !strings.HasPrefix(d.Markdown(), lines[0]+"\n") || !strings.HasSuffix(d.Markdown(), lines[len(lines)-2]+"\n") {
			t.Fatalf("其他列表项变化：%q", d.Markdown())
		}
		if _, ok := d.Undo(); !ok || d.Markdown() != source {
			t.Fatalf("列表撤销未恢复原文：%q", d.Markdown())
		}
	}
}

func TestPendingFormatsContinueAndCanBeDisabled(t *testing.T) {
	d := Parse("ab\n")
	d.ToggleMark(Selection{1, 1}, MarkBold)
	d.ToggleMark(Selection{1, 1}, MarkItalic)
	at := d.Insert(1, "甲").End
	at = d.Insert(at, "乙").End
	if d.Text() != "a甲乙b" || d.Blocks()[0].Runs[1].Marks != MarkBold|MarkItalic {
		t.Fatalf("连续格式输入失败：%+v", d.Blocks()[0].Runs)
	}
	d.ToggleMark(Selection{at, at}, MarkBold)
	d.Insert(at, "丙")
	b := Parse(d.Markdown()).Blocks()[0]
	if plainRuns(b.Runs) != "a甲乙丙b" || b.Runs[1].Marks != MarkBold|MarkItalic || b.Runs[2].Marks != MarkItalic {
		t.Fatalf("格式序列化失败：%q %+v", d.Markdown(), b.Runs)
	}
}

func TestRawRemainsReadOnlyForOrdinaryEditing(t *testing.T) {
	for _, source := range []string{"text ![图片](image.png) end\n", "<div>内容</div>\n", "[引用][id]\n\n[id]: /url\n", "- 一\n\n  ```\n  代码\n  ```\n"} {
		d := Parse(source)
		if d.Blocks()[0].Kind != Raw {
			t.Fatalf("预期 Raw：%+v", d.Blocks())
		}
		d.Insert(0, "新")
		d.Split(0)
		d.Delete(0, 1)
		d.SetBlock(Selection{}, Paragraph, 0)
		if d.Markdown() != source || d.Changed() {
			t.Fatalf("只读内容被修改：%q", d.Markdown())
		}
	}
}

func TestTableCellEditingBoundaries(t *testing.T) {
	d := Parse("| **ab**cd |  |\n| :--- | ---: |\n| x | y |\n")
	d.Insert(3, "新")
	if d.Text() != "abc新d\t\nx\ty" {
		t.Fatalf("run 偏移误作单元格偏移：%q", d.Text())
	}
	d.Insert(6, "空")
	if d.Text() != "abc新d\t空\nx\ty" {
		t.Fatalf("空单元格插入失败：%q", d.Text())
	}
	d.Delete(1, 3)
	if d.Text() != "a新d\t空\nx\ty" {
		t.Fatalf("单元格删除失败：%q", d.Text())
	}
	before := d.Markdown()
	d.Delete(2, 6)
	d.Insert(1, "a\nb")
	d.Split(1)
	if d.Markdown() != before {
		t.Fatalf("跨单元格结构被修改：%q", d.Markdown())
	}
	again := Parse(before)
	if again.Text() != d.Text() || again.Blocks()[0].Table.Aligns[0] != AlignLeft || again.Blocks()[0].Table.Aligns[1] != AlignRight {
		t.Fatalf("表格回写损坏：%q", before)
	}
}

func TestEditedMarkdownEscapesAndRetainsFormats(t *testing.T) {
	for _, source := range []string{"a **粗 *斜* 粗** z\n", "[**链接**](https://example.com \"标题\") z\n", "`a``b` z\n", "a &amp; b\n", ">> 引用\n"} {
		d := Parse(source)
		before := d.Text()
		d.Insert(d.Len(), " *[x]|\\!")
		got := Parse(d.Markdown())
		if got.Text() != before+" *[x]|\\!" {
			t.Fatalf("内容变化：%q -> %q", d.Markdown(), got.Text())
		}
	}
}

func TestAtomicSelectionReplacementUndoRedo(t *testing.T) {
	for _, edit := range []struct {
		name string
		run  func(*Document) Selection
		want string
	}{
		{"替换", func(d *Document) Selection { return d.Replace(Selection{1, 3}, "XY") }, "aXYd"},
		{"换行", func(d *Document) Selection { return d.Replace(Selection{1, 3}, "\n") }, "a\nd"},
		{"链接", func(d *Document) Selection {
			return d.InsertLinkSelection(Selection{1, 3}, "链接", "https://example.com")
		}, "a链接d"},
	} {
		t.Run(edit.name, func(t *testing.T) {
			d := Parse("abcd\n")
			after := edit.run(d)
			if d.Text() != edit.want {
				t.Fatalf("替换结果：%q", d.Text())
			}
			sel, ok := d.Undo()
			if !ok || sel != (Selection{1, 3}) || d.Markdown() != "abcd\n" {
				t.Fatalf("撤销失败：%v %q", sel, d.Markdown())
			}
			if _, ok := d.Undo(); ok {
				t.Fatal("替换被拆成多个撤销步骤")
			}
			sel, ok = d.Redo()
			if !ok || sel != after || d.Text() != edit.want {
				t.Fatalf("重做失败：%v %q", sel, d.Text())
			}
		})
	}
}

func TestPasteSelectionMarkdownKeepsCaretAndUndo(t *testing.T) {
	d := Parse("abcd\n")
	after := d.PasteSelection(Selection{1, 3}, "**甲**\n\n乙")
	if d.Text() != "a甲\n乙d" || after != (Selection{4, 4}) {
		t.Fatalf("粘贴位置错误：%q %v", d.Text(), after)
	}
	if got := Parse(d.Markdown()).Text(); got != d.Text() {
		t.Fatalf("粘贴回写错误：%q -> %q", d.Markdown(), got)
	}
	if _, ok := d.Undo(); !ok || d.Markdown() != "abcd\n" {
		t.Fatalf("粘贴撤销失败：%q", d.Markdown())
	}
	if _, ok := d.Undo(); ok {
		t.Fatal("粘贴产生多步撤销")
	}
	d.PasteSelection(Selection{1, 3}, "**甲**")
	if d.Text() != "a甲d" || d.Blocks()[0].Runs[1].Marks != MarkBold {
		t.Fatalf("单行Markdown未解析：%+v", d.Blocks())
	}
}

func TestExitSpecialBlockKeepsOriginalAndUndo(t *testing.T) {
	for _, source := range []string{"```go\nx\n```\n", "| a | b |\n| --- | --- |\n", "![图](a.png)\n", "<div>内容</div>\n"} {
		d := Parse(source)
		sel := d.InsertParagraphAfter(0)
		if sel != (Selection{d.Len(), d.Len()}) || len(d.Blocks()) != 2 || d.Blocks()[1].Kind != Paragraph {
			t.Fatalf("退出块失败：%v %+v", sel, d.Blocks())
		}
		d.Insert(sel.End, "正文")
		if !strings.HasPrefix(d.Markdown(), source) || Parse(d.Markdown()).Blocks()[1].Kind != Paragraph {
			t.Fatalf("原文或新段落损坏：%q", d.Markdown())
		}
		d.Undo()
		d.Undo()
		if d.Markdown() != source {
			t.Fatalf("退出块撤销未恢复原文：%q", d.Markdown())
		}
	}
}

func TestTableRejectedReplacementDoesNotDeleteSelection(t *testing.T) {
	d := Parse("| abc | def |\n| --- | --- |\n")
	original := d.Markdown()
	d.Replace(Selection{0, 2}, "\n")
	d.PasteSelection(Selection{0, 2}, "x\ny")
	if d.Markdown() != original || d.Changed() {
		t.Fatalf("拒绝替换删除了选区：%q", d.Markdown())
	}
}

func TestFormatToggleBreaksTypingUndoGroup(t *testing.T) {
	d := Parse("ab\n")
	at := d.Insert(1, "甲").End
	d.ToggleMark(Selection{at, at}, MarkBold)
	d.Insert(at, "乙")
	d.Undo()
	if d.Text() != "a甲b" {
		t.Fatalf("格式切换前输入被合并撤销：%q", d.Text())
	}
	d.Undo()
	if d.Markdown() != "ab\n" {
		t.Fatalf("撤销未恢复原文：%q", d.Markdown())
	}
}

func TestCodeBlockConversionPreservesBody(t *testing.T) {
	d := Parse("正文\n")
	d.SetBlock(Selection{}, Code, 0)
	if d.Blocks()[0].Code != "正文" || Parse(d.Markdown()).Blocks()[0].Kind != Code {
		t.Fatalf("转代码丢正文：%q", d.Markdown())
	}
	d.SetBlock(Selection{}, Paragraph, 0)
	if d.Text() != "正文" {
		t.Fatalf("转段落丢正文：%q", d.Text())
	}
}

func TestMarkdownDisplaySemantics(t *testing.T) {
	for _, item := range []struct{ source, text string }{
		{"a &amp; &#x4e2d;\n", "a & 中"},
		{"一\n二\n", "一\n二"},
		{"` a `\n", "a"},
		{"```\ncode\n```\n\n末尾\n", "code\n\n末尾"},
	} {
		d := Parse(item.source)
		if d.Text() != item.text {
			t.Fatalf("显示文本：%q -> %q，预期%q", item.source, d.Text(), item.text)
		}
	}
}

func TestReplaceRawRetainsFollowingBlocks(t *testing.T) {
	d := Parse("<div>原文</div>\n\n第一\n\n第二\n")
	if err := d.ReplaceRaw(0, "甲\n\n乙\n"); err != nil {
		t.Fatal(err)
	}
	if d.Text() != "甲\n乙\n第一\n第二" || !strings.HasSuffix(d.Markdown(), "第一\n\n第二\n") {
		t.Fatalf("替换占位覆盖相邻块：%q", d.Markdown())
	}
}

func TestUnsupportedMathAndComplexCalloutRemainRaw(t *testing.T) {
	for _, source := range []string{"跨节点 $a *b* c$。\n", "> [!tip] 提示\n> $$\n", "> [!WARNING]- 标题\n> 内容\n>\n> 第二段\n"} {
		d := Parse(source)
		if d.Blocks()[0].Kind != Raw || d.Text() != objectReplacement {
			t.Fatalf("未支持语法露出源码：%q %+v", source, d.Blocks())
		}
		d.Insert(0, "新")
		if d.Markdown() != source {
			t.Fatalf("未支持语法丢失：%q", d.Markdown())
		}
	}
	for _, source := range []string{"价格 $5\n", "价格 $5 和 $10\n", "转义 \\$x\\$\n", "代码 `$s = vt$`\n"} {
		if d := Parse(source); d.Blocks()[0].Kind == Raw {
			t.Fatalf("普通美元误判公式：%q", source)
		}
	}
}

func TestCalloutBodyEditingRetainsHeaderAndSource(t *testing.T) {
	for _, source := range []string{"> [!tip] 提示\n> 正文 **粗体**\n", "> [!WARNING]- 标题\r\n> 第一行\r\n> 第二行\r\n", "> [!note]+\n"} {
		d := Parse(source)
		b := d.Blocks()[0]
		if b.Kind != Callout || b.Callout == nil || d.Markdown() != source {
			t.Fatalf("callout 模型或原文错误：%q %+v", source, b)
		}
		before := d.Text()
		d.Insert(0, "增")
		again := Parse(d.Markdown())
		if again.Text() != "增"+before || again.Blocks()[0].Kind != Callout || *again.Blocks()[0].Callout != *b.Callout {
			t.Fatalf("callout 编辑丢失头部或正文：%q %+v", d.Markdown(), again.Blocks())
		}
		d.Undo()
		if d.Markdown() != source {
			t.Fatalf("callout 撤销丢原文：%q", d.Markdown())
		}
		b.Callout.Type = "外部修改"
		if d.Blocks()[0].Callout.Type == b.Callout.Type {
			t.Fatal("callout 元数据副本泄漏")
		}
	}
}

func TestCalloutNewlineKeepsSingleBodyAndHeader(t *testing.T) {
	d := Parse("> [!tip]- 标题\n> 正文\n")
	d.Split(1)
	if len(d.Blocks()) != 1 || Parse(d.Markdown()).Text() != "正\n文" || Parse(d.Markdown()).Blocks()[0].Callout.Fold != "-" {
		t.Fatalf("callout 换行丢失结构：%q", d.Markdown())
	}
}

func TestHighlightAndScriptsRetainTextAndMarks(t *testing.T) {
	for _, item := range []struct {
		source, text string
		mark         Mark
	}{
		{"==高亮 **粗体**==", "高亮 粗体", MarkHighlight},
		{"H~2~O", "H2O", MarkSub},
		{"x^2^", "x2", MarkSup},
		{"~a\\ b~", "a b", MarkSub},
		{"^a*b*^", "a*b*", MarkSup},
	} {
		d := Parse(item.source)
		if d.Text() != item.text || d.Markdown() != item.source {
			t.Fatalf("行内解析错误：%q %q", item.source, d.Text())
		}
		found := false
		for _, r := range d.Blocks()[0].Runs {
			found = found || r.Marks&item.mark != 0
		}
		if !found {
			t.Fatalf("缺少标记：%+v", d.Blocks())
		}
		d.Insert(d.Len(), "尾")
		again := Parse(d.Markdown())
		if again.Text() != item.text+"尾" {
			t.Fatalf("行内回写丢正文：%q -> %q", d.Markdown(), again.Text())
		}
		for i, r := range d.Blocks()[0].Runs {
			if again.Blocks()[0].Runs[i].Marks != r.Marks {
				t.Fatalf("标记回写变化：%q %+v", d.Markdown(), again.Blocks())
			}
		}
	}
	for _, source := range []string{"~a b~", "^a b^", "\\==普通\\==", "`==代码== ~2~ ^2^`"} {
		d := Parse(source)
		for _, r := range d.Blocks()[0].Runs {
			if r.Marks&(MarkHighlight|MarkSub|MarkSup) != 0 {
				t.Fatalf("普通文本误判标记：%q %+v", source, d.Blocks())
			}
		}
	}
}

func TestUnsupportedListInlineOnlyProtectsItsItem(t *testing.T) {
	for _, source := range []string{"3) 正常\n1) 公式 $a *b* c$\n1) 末项\n\n尾段\n", "* 正常\n\n* text ![图](a.png) end\n\n* 末项\n"} {
		d := Parse(source)
		blocks := d.Blocks()
		if len(blocks) < 3 || blocks[0].Kind != List || blocks[1].Kind != Raw || blocks[2].Kind != List || d.Markdown() != source {
			t.Fatalf("逐项 Raw 或原文错误：%q %+v", d.Markdown(), blocks)
		}
		if blocks[0].Ordered && (blocks[1].Start != 4 || blocks[2].Start != 5) {
			t.Fatalf("Raw 影响编号：%+v", blocks)
		}
		d.Insert(0, "增")
		start, _ := d.blockRange(2)
		d.Insert(start, "增")
		if !strings.Contains(d.Markdown(), d.blocks[1].source) || Parse(d.Markdown()).Blocks()[2].Kind != List {
			t.Fatalf("相邻编辑损坏 Raw 项：%q", d.Markdown())
		}
		d.Undo()
		d.Undo()
		if d.Markdown() != source {
			t.Fatalf("逐项撤销丢原文：%q", d.Markdown())
		}
	}
}

func TestInlineMathIsEditableRunAndRoundTrips(t *testing.T) {
	source := "1. 沿溪走了 $s = vt$ 约三里，价格 $5\n"
	d := Parse(source)
	b := d.Blocks()[0]
	if b.Kind != List || d.Text() != "沿溪走了 s = vt 约三里，价格 $5" || d.Markdown() != source {
		t.Fatalf("行内公式解析错误：%q %+v", d.Text(), b)
	}
	if len(b.Runs) != 3 || b.Runs[1].Marks != MarkMath || b.Runs[1].Text != "s = vt" {
		t.Fatalf("公式未落在独立片段：%+v", b.Runs)
	}
	// 公式内部继续输入仍属于公式，公式右侧输入的是普通正文。
	d.Insert(6, "_0")
	end := 5 + len([]rune("s_0 = vt"))
	d.Insert(end, "！")
	want := "1. 沿溪走了 $s_0 = vt$！ 约三里，价格 \\$5\n"
	if d.Markdown() != want {
		t.Fatalf("公式回写错误：%q", d.Markdown())
	}
	again := Parse(d.Markdown()).Blocks()[0]
	if again.Runs[1].Marks != MarkMath || again.Runs[1].Text != "s_0 = vt" {
		t.Fatalf("公式重新解析错误：%+v", again.Runs)
	}
}

func TestNestedListsFlattenWithLevelsAndKeepIndent(t *testing.T) {
	source := "- 外层\n  - 内层\n    1. 三层\n- [x] 完成\n- [ ] 待办\n\n尾段\n"
	d := Parse(source)
	blocks := d.Blocks()
	if len(blocks) != 6 || d.Markdown() != source {
		t.Fatalf("嵌套列表展开或原文错误：%q %+v", d.Markdown(), blocks)
	}
	for i, want := range []struct {
		kind  Kind
		level int
	}{{List, 1}, {List, 2}, {List, 3}, {Task, 1}, {Task, 1}, {Paragraph, 0}} {
		if blocks[i].Kind != want.kind || blocks[i].Level != want.level {
			t.Fatalf("第%d块层级错误：%+v", i, blocks[i])
		}
	}
	if !blocks[2].Ordered || !blocks[3].Checked || blocks[4].Checked {
		t.Fatalf("列表属性错误：%+v", blocks)
	}
	// 编辑内层项只重写该行，并沿用原缩进，重新解析后层级不变。
	start, _ := d.blockRange(1)
	d.Insert(start, "新")
	if d.Markdown() != "- 外层\n  - 新内层\n    1. 三层\n- [x] 完成\n- [ ] 待办\n\n尾段\n" {
		t.Fatalf("嵌套项回写错误：%q", d.Markdown())
	}
	// 在内层项中换行得到同级新项。
	d.Split(start + 1)
	again := Parse(d.Markdown()).Blocks()
	if len(again) != 7 || again[1].Level != 2 || again[2].Level != 2 || again[3].Level != 3 {
		t.Fatalf("嵌套项换行层级错误：%q %+v", d.Markdown(), again)
	}
	d.Undo()
	d.Undo()
	if d.Markdown() != source {
		t.Fatalf("嵌套列表撤销丢原文：%q", d.Markdown())
	}
}

func TestOrderedListItemNumbersPersistAfterEditing(t *testing.T) {
	d := Parse("3. 一\n1. 二\n1. 三\n")
	for i, b := range d.Blocks() {
		if b.Start != 3+i {
			t.Fatalf("第%d项编号错误：%d", i, b.Start)
		}
	}
	d.Insert(3, "新")
	if !strings.Contains(d.Markdown(), "4. 二新") {
		t.Fatalf("编辑未使用真实编号：%q", d.Markdown())
	}
	for i, b := range Parse(d.Markdown()).Blocks() {
		if b.Start != 3+i {
			t.Fatalf("回写后第%d项编号错误：%d", i, b.Start)
		}
	}
}
