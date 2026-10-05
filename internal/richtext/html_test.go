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
		`<div class="x">原样</div>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("导出 HTML 缺少 %q：\n%s", want, got)
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
