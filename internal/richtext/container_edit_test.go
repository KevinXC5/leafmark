package richtext

import (
	"strings"
	"testing"
)

func runeOffset(d *Document, text string) int {
	at := strings.Index(d.Text(), text)
	if at < 0 {
		return -1
	}
	return len([]rune(d.Text()[:at]))
}

func TestContainerEditedRoundTripAndEmptyLines(t *testing.T) {
	source := "- 甲项\n\n  续甲\n\n> 引用甲\n"
	d := Parse(source)
	at := runeOffset(d, "甲项")
	d.Replace(Selection{at, at + 1}, "乙")
	got := d.Markdown()
	if !strings.Contains(got, "续甲\n\n> 引用甲") {
		t.Fatalf("容器间空行丢失：%q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == "" && line != "" {
			t.Fatalf("空行含尾随空格：%q", got)
		}
	}
	parsed := Parse(got)
	if parsed.Text() != "乙项\n续甲\n引用甲" {
		t.Fatalf("保存后容器内容变化：%q", parsed.Text())
	}
	d.Undo()
	if d.Markdown() != source {
		t.Fatal("撤销没恢复原文")
	}
}

func TestContainerTaskAndIndentWriteBack(t *testing.T) {
	source := "- 父项\n\n  说明\n- [ ] 任务\n\n  任务说明\n"
	d := Parse(source)
	bi := d.BlockIndexAt(runeOffset(d, "任务"))
	d.SetChecked(bi, true)
	if !strings.Contains(d.Markdown(), "- [x] 任务") {
		t.Fatalf("任务没写回：%q", d.Markdown())
	}
	at := runeOffset(d, "任务")
	if !d.ShiftListLevel(Selection{at, at}, 1) {
		t.Fatal("任务缩进无效")
	}
	got := d.Markdown()
	p := Parse(got)
	bi = p.BlockIndexAt(runeOffset(p, "任务"))
	if len(p.Blocks()[bi].Containers) < 2 {
		t.Fatalf("未形成子列表：%q", got)
	}
	d.Undo()
	if !strings.Contains(d.Markdown(), "- [x] 任务") {
		t.Fatal("缩进撤销没恢复任务")
	}
	d.Undo()
	if d.Markdown() != source {
		t.Fatal("任务撤销没恢复原文")
	}
}

func TestContainerSplitAndLeave(t *testing.T) {
	source := "- 甲项\n\n  说明\n- 乙项\n"
	d := Parse(source)
	at := runeOffset(d, "甲项") + 2
	d.Split(at)
	if got := Parse(d.Markdown()); len(got.Blocks()) < 4 {
		t.Fatalf("未建立新列表项：%q", d.Markdown())
	}
	ids := map[int]bool{}
	for _, b := range d.Blocks() {
		if b.Kind == List {
			id := b.Containers[len(b.Containers)-1].ID
			if ids[id] {
				t.Fatal("新项复用了旧标识")
			}
			ids[id] = true
		}
	}
	d.Undo()
	if d.Markdown() != source {
		t.Fatal("拆项撤销没恢复原文")
	}
	quote := Parse("> 首段\n>\n> 次段\n")
	at = runeOffset(quote, "次段")
	quote.Delete(at-1, at)
	if len(quote.Blocks()[1].Containers) != 0 {
		t.Fatal("退格未退出引用")
	}
}

func TestInlineImageSurvivesAdjacentEdits(t *testing.T) {
	source := "前 ![图][pic] 后\n\n[pic]: a.png\n"
	d := Parse(source)
	d.Insert(0, "新增")
	d.Delete(0, 1)
	got := d.Markdown()
	if !strings.Contains(got, "![图][pic]") {
		t.Fatalf("编辑丢失图片：%q", got)
	}
	image := d.Blocks()[0].Runs[1].Image
	if image == nil || image.URL != "a.png" {
		t.Fatalf("图片数据丢失：%+v", d.Blocks()[0].Runs)
	}
	clone := d.Blocks()
	clone[0].Runs[1].Image.URL = "b.png"
	if d.Blocks()[0].Runs[1].Image.URL != "a.png" {
		t.Fatal("Blocks暴露可变图片")
	}
}

func TestFootnoteNumbersRefreshAfterDeleteAndUndo(t *testing.T) {
	d := Parse("甲[^a] 乙[^b]\n\n[^a]: 注甲\n\n[^b]: 注乙\n")
	at := runeOffset(d, "a")
	d.Delete(at, at+1)
	for _, b := range d.Blocks() {
		if b.Footnote != nil && b.Footnote.Label == "b" && b.Footnote.Number != 1 {
			t.Fatal("脚注编号未刷新")
		}
	}
	d.Undo()
	for _, b := range d.Blocks() {
		if b.Footnote != nil && b.Footnote.Label == "b" && b.Footnote.Number != 2 {
			t.Fatal("脚注撤销编号未恢复")
		}
	}
}
