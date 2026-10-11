package richtext

import (
	"strings"
	"testing"
)

func TestWikiImageBlockAndInline(t *testing.T) {
	source := "![[架构 图.drawio.svg]]\n\n前文 ![[assets/a.png|说明|300x200]] 后文\n\n![[b.PNG|120]]\n"
	d := Parse(source)
	blocks := d.Blocks()
	if len(blocks) != 3 {
		t.Fatalf("块数不对：%d", len(blocks))
	}
	first := blocks[0]
	if first.Kind != Image || !first.Wiki || first.URL != "%E6%9E%B6%E6%9E%84%20%E5%9B%BE.drawio.svg" || first.Alt != "" {
		t.Fatalf("独占一行的嵌入应是图片块：%+v", first)
	}
	if len(blocks[1].Runs) != 3 || blocks[1].Runs[1].Image == nil {
		t.Fatalf("行内嵌入应是行内图片：%+v", blocks[1].Runs)
	}
	inline := blocks[1].Runs[1].Image
	if !inline.Wiki || inline.URL != "assets/a.png" || inline.Alt != "说明" || inline.Size != "300x200" {
		t.Fatalf("行内嵌入字段不对：%+v", inline)
	}
	if blocks[2].Size != "120" || blocks[2].Alt != "" || ImageWidth(blocks[2].Size) != 120 || ImageWidth(inline.Size) != 300 {
		t.Fatalf("尺寸解析不对：%+v", blocks[2])
	}
	if d.Markdown() != source {
		t.Fatalf("未编辑的嵌入应原样写回：%q", d.Markdown())
	}
}

func TestWikiImageRewrittenAsEmbed(t *testing.T) {
	d := Parse("前文 ![[子目录/图 1.png|说明|300]] 后文\n")
	d.Insert(0, "新")
	if got := d.Markdown(); got != "新前文 ![[子目录/图 1.png|说明|300]] 后文\n" {
		t.Fatalf("编辑后应仍写成嵌入：%q", got)
	}
	d.Undo()
	if got := d.Markdown(); got != "前文 ![[子目录/图 1.png|说明|300]] 后文\n" {
		t.Fatalf("撤销后原文不对：%q", got)
	}
	// 说明形如尺寸时嵌入语法无法区分，改写成普通图片。
	if embed, ok := wikiEmbed("a.png", "300", ""); ok {
		t.Fatalf("形如尺寸的说明不能写成嵌入：%q", embed)
	}
	if embed, ok := wikiEmbed("https://example.com/a.png", "", ""); ok {
		t.Fatalf("远程地址不能写成嵌入：%q", embed)
	}
}

func TestWikiEmbedOfNotesStaysText(t *testing.T) {
	for _, source := range []string{"![[另一篇笔记]]\n", "![[文档.pdf]]\n", "![[]]\n", "![[a.png\n", "`![[a.png]]`\n", "![[a.png#标题]]\n"} {
		d := Parse(source)
		for _, b := range d.Blocks() {
			if b.Kind == Image {
				t.Fatalf("%q 不应解析成图片", source)
			}
			for _, r := range b.Runs {
				if r.Image != nil {
					t.Fatalf("%q 不应解析成行内图片", source)
				}
			}
		}
		if d.Markdown() != source {
			t.Fatalf("%q 应原样写回：%q", source, d.Markdown())
		}
	}
}

func TestWikiImageHTML(t *testing.T) {
	var asked []string
	out := Parse("![[图 1.png|说明|300x200]]\n\n甲 ![[b.svg|64]] 乙\n").HTMLWith(func(url string) string {
		asked = append(asked, url)
		return "data:image/png;base64,AAAA"
	})
	if len(asked) != 2 || asked[0] != "%E5%9B%BE%201.png" || asked[1] != "b.svg" {
		t.Fatalf("导出应按转义后的相对地址取图：%v", asked)
	}
	if !strings.Contains(out, `alt="说明" width="300" height="200">`) || !strings.Contains(out, `alt="" width="64">`) {
		t.Fatalf("导出缺少说明或尺寸：%s", out)
	}
}
