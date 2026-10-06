package richtext

import (
	"os"
	"strings"
	"testing"
)

func TestHTMLCoversBlocksInlineMarksAndNestedLists(t *testing.T) {
	source := "# 标题 A&B\n\n正文 **粗** *斜* ~~删~~ `码` ==亮== H~2~O x^2^ $s = vt$ [链](https://example.com \"题\")\n\n> 引用\n\n> [!tip] 提示\n> 内容\n\n1. 一\n2. 二\n\n- 外\n  - 内\n- [x] 完成\n\n| 甲 | 乙 |\n| :--- | ---: |\n| 1 | 2 |\n\n```go\na < b\n```\n\n---\n\n<div class=\"x\">原样</div>\n"
	got := Parse(source).HTML()
	for _, want := range []string{
		"<h1>标题 A&amp;B</h1>",
		"<strong>粗</strong>", "<em>斜</em>", "<del>删</del>", "<code>码</code>", "<mark>亮</mark>", "<sub>2</sub>", "<sup>2</sup>",
		`<span class="math">s = vt</span>`,
		`<a href="https://example.com" title="题">链</a>`,
		"<blockquote><p>引用</p></blockquote>",
		`<div class="callout callout-tip"><p class="callout-title">提示</p><p>内容</p></div>`,
		"<ol>\n<li>一</li>\n<li>二</li></ol>",
		"<ul>\n<li>外<ul>\n<li>内</li></ul>\n</li>\n<li class=\"task\"><input type=\"checkbox\" disabled checked> 完成</li></ul>",
		`<th style="text-align:left">甲</th>`, `<td style="text-align:right">2</td>`,
		`<pre><code class="language-go">a &lt; b` + "\n</code></pre>",
		"<hr>",
		"<p>原样</p>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("导出 HTML 缺少 %q：\n%s", want, got)
		}
	}
}

func TestHTMLEmbedsRenderedMathAndDiagrams(t *testing.T) {
	source := "行内 $s = vt$ 继续\n\n$$\nQ_n = Q \\cdot r^n\n$$\n\n```mermaid\ngraph LR\nA-->B\n```\n\n<script>alert(1)</script>\n"
	render := Renderers{
		InlineMath: func(src string) (string, float32, bool) {
			if src != "s = vt" {
				t.Fatalf("行内公式原文错误：%q", src)
			}
			return `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="8"></svg>`, 2, true
		},
		DisplayMath: func(src string) (string, bool) {
			if src != "Q_n = Q \\cdot r^n" {
				t.Fatalf("块级公式原文错误：%q", src)
			}
			return `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"></svg>`, true
		},
		Diagram: func(src string) (string, bool) {
			if src != "graph LR\nA-->B" {
				t.Fatalf("流程图原文错误：%q", src)
			}
			return `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"></svg>`, true
		},
	}
	got := Parse(source).HTMLWithRenderers(nil, render)
	for _, want := range []string{
		`<span class="math" style="vertical-align:-2px"><svg xmlns="http://www.w3.org/2000/svg" width="10" height="8"></svg></span>`,
		`<div class="math-display"><svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"></svg></div>`,
		`<div class="diagram"><svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"></svg></div>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("导出缺少排版结果 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "s = vt") || strings.Contains(got, "graph LR") || strings.Contains(got, "<script") {
		t.Fatalf("排版后仍留下源码或未过滤脚本：\n%s", got)
	}

	// 排版失败或返回的不是 SVG 时保留源码，不把任意标记写进文档。
	fallback := Parse(source).HTMLWithRenderers(nil, Renderers{
		InlineMath:  func(string) (string, float32, bool) { return "<svg><script></script></svg>", 1, true },
		DisplayMath: func(string) (string, bool) { return "", false },
		Diagram:     func(string) (string, bool) { return "<img>", true },
	})
	for _, want := range []string{`<span class="math">s = vt</span>`, "Q_n = Q", "graph LR", "A--&gt;B"} {
		if !strings.Contains(fallback, want) {
			t.Fatalf("回退导出缺少 %q：\n%s", want, fallback)
		}
	}
	if strings.Contains(fallback, "<script") || strings.Contains(fallback, "<img>") {
		t.Fatalf("回退导出写入了非 SVG：\n%s", fallback)
	}
}

func TestHTMLAssemblesContainersAndFootnotes(t *testing.T) {
	doc := &Document{blocks: []editBlock{
		{Block: Block{Kind: Paragraph, Runs: []Run{
			{Text: "见", Link: nil},
			{Text: "1", Marks: MarkSup, Link: &Link{Footnote: "a"}},
			{Text: "又见", Marks: MarkUnderline},
			{Text: "回车", Marks: MarkKbd},
			{Text: "2", Marks: MarkSup, Link: &Link{Footnote: "a"}},
		}}},
		{Block: Block{
			Kind: Paragraph, Runs: []Run{{Text: "第一段"}},
			Containers: []Container{{Kind: Quote, ID: 1, Level: 1}, {Kind: List, ID: 2, Level: 2, Ordered: true, Start: 3}},
		}},
		{Block: Block{
			Kind: Code, Lang: "go", Code: "续段",
			Containers: []Container{{Kind: Quote, ID: 1, Level: 1}, {Kind: List, ID: 2, Level: 2, Ordered: true, Start: 3}},
		}},
		{Block: Block{
			Kind: Paragraph, Runs: []Run{{Text: "说明"}},
			Containers: []Container{{Kind: Callout, ID: 4, Callout: &CalloutData{Type: "tip", Title: "提示"}}},
		}},
		{Block: Block{Kind: Paragraph, Runs: []Run{{Text: "注一"}}, Footnote: &FootnoteData{Label: "a", Number: 1}}},
		{Block: Block{Kind: Paragraph, Runs: []Run{{Text: "注一续"}}, Footnote: &FootnoteData{Label: "a", Number: 1}}},
	}}
	got := doc.HTML()
	for _, want := range []string{
		`<a class="footnote-ref" href="#fn-a" id="fnref-a"><sup>1</sup></a>`,
		`<a class="footnote-ref" href="#fn-a" id="fnref-a-2"><sup>1</sup></a>`,
		"<u>又见</u>",
		"<kbd>回车</kbd>",
		"<blockquote>\n<ol start=\"3\">\n<li>第一段\n<pre><code class=\"language-go\">续段</code></pre>\n</li></ol>\n</blockquote>",
		`<div class="callout callout-tip"><p class="callout-title">提示</p>`,
		`<li id="fn-a">`,
		"<p>注一</p>",
		"<p>注一续</p>",
		`href="#fnref-a-1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("导出缺少 %q：\n%s", want, got)
		}
	}
	if strings.Count(got, "注一") != 2 || strings.Count(got, "<blockquote>") != 1 {
		t.Fatalf("脚注或容器重复写出：\n%s", got)
	}
}

func TestHTMLWritesDetailsContainer(t *testing.T) {
	doc := &Document{blocks: []editBlock{{Block: Block{
		Kind: Paragraph, Runs: []Run{{Text: "段"}},
		Containers: []Container{{Kind: Callout, ID: 1, Callout: &CalloutData{Type: "details", Title: "甲", Fold: "+"}, html: true}},
	}}}}
	got := doc.HTML()
	if !strings.Contains(got, "<details open>\n<summary>甲</summary>\n<p>段</p>\n</details>") {
		t.Fatalf("details 导出不对：\n%s", got)
	}
}

func TestHTMLSkipsReferenceDefsInsideLists(t *testing.T) {
	doc := &Document{blocks: []editBlock{
		{Block: Block{Kind: List, Level: 1, Runs: []Run{{Text: "甲"}, {Image: &InlineImage{Alt: "图", URL: "a.png"}}}}},
		{Block: Block{Kind: ReferenceDef, Raw: "[图]: a.png"}},
		{Block: Block{Kind: List, Level: 1, Runs: []Run{{Text: "乙"}}}},
	}}
	before := doc.blocks[1].Raw
	got := doc.HTML()
	if doc.blocks[1].Raw != before {
		t.Fatalf("导出改写了隐藏定义：%q", doc.blocks[1].Raw)
	}
	if strings.Contains(got, "[图]") || strings.Contains(got, "<p></p>") || strings.Count(got, "<ul>") != 1 {
		t.Fatalf("隐藏定义进入正文或截断了列表：\n%s", got)
	}
	if !strings.Contains(got, `<img src="a.png" alt="图">`) || !strings.Contains(got, "<li>甲") || !strings.Contains(got, "<li>乙") {
		t.Fatalf("行内图片或列表项丢失：\n%s", got)
	}
}

func TestHTMLReferenceImagesAndDetails(t *testing.T) {
	source := "见 [链][ref] 与 ![甲图][pic]。\n\n[ref]: https://example.com \"题\"\n\n[pic]: assets/a.png \"图题\"\n"
	got := Parse(source).HTMLWith(func(url string) string {
		if url == "assets/a.png" {
			return "data:image/png;base64,QQ"
		}
		return ""
	})
	for _, want := range []string{
		`<a href="https://example.com" title="题">链</a>`,
		`<img src="data:image/png;base64,QQ" alt="甲图" title="图题">`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("引用式导出缺少 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "[ref]:") || strings.Contains(got, "[pic]:") || strings.Contains(got, "assets/a.png") {
		t.Fatalf("定义原文或未改写地址进入了导出：\n%s", got)
	}

	open := Parse("<details open><summary>展开</summary><p>正文</p></details>\n")
	closed := Parse("<details><summary>标题</summary><p>收起</p></details>\n")
	if got := open.HTML(); !strings.Contains(got, "<details open>\n<summary>展开</summary>\n<p>正文</p>\n</details>") {
		t.Fatalf("展开的 details 不正确：\n%s", got)
	}
	if got := closed.HTML(); strings.Contains(got, " open") || !strings.Contains(got, "<details>\n<summary>标题</summary>") {
		t.Fatalf("收起的 details 不正确：\n%s", got)
	}
}

func TestHTMLWritesUnderlineAndKbd(t *testing.T) {
	doc := &Document{blocks: []editBlock{{Block: Block{Kind: Paragraph, Runs: []Run{
		{Text: "下", Marks: MarkUnderline},
		{Text: "键", Marks: MarkKbd},
	}}}}}
	got := doc.HTML()
	for _, want := range []string{"<u>下</u>", "<kbd>键</kbd>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺少 %q：\n%s", want, got)
		}
	}
}

func TestHTMLHandlesSampleDocuments(t *testing.T) {
	for _, name := range []string{"山中来信", "叶脉笔记"} {
		data, err := os.ReadFile("../../tests/fixtures/samples/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		got := Parse(string(data)).HTML()
		if !strings.Contains(got, "<h1>"+name+"</h1>") || strings.Count(got, "<ul>") != strings.Count(got, "</ul>") || strings.Count(got, "<li") != strings.Count(got, "</li>") {
			t.Fatalf("%s 导出结构不完整：\n%s", name, got)
		}
	}
}
