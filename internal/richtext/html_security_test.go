package richtext

import (
	"strings"
	"testing"
)

func TestHTMLRendersNestedQuotesIndentedCodeAndReferenceImages(t *testing.T) {
	source := strings.Join([]string{
		"> 外层",
		">",
		"> > 内层",
		"",
		"    缩进代码 <a>",
		"",
		"![图][pic]",
		"",
		"[pic]: assets/pic.png \"题\"",
		"",
	}, "\n")
	got := Parse(source).HTML()
	for _, want := range []string{
		"<blockquote>",
		"外层",
		"内层",
		"<pre><code>缩进代码 &lt;a&gt;",
		`<img src="assets/pic.png" alt="图" title="题"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("导出缺少 %q：\n%s", want, got)
		}
	}
	if strings.Count(got, "<blockquote>") < 2 {
		t.Fatalf("嵌套引用没有两层：\n%s", got)
	}
}

func TestHTMLStripsScriptsFromRawHTML(t *testing.T) {
	source := "<div onclick=\"alert(1)\">正文<script>alert(2)</script>结尾</div>\n"
	got := Parse(source).HTML()
	lower := strings.ToLower(got)
	if strings.Contains(lower, "<script") || strings.Contains(lower, "onclick") {
		t.Fatalf("原始 HTML 的脚本没有去掉：\n%s", got)
	}
	if !strings.Contains(got, "正文") || !strings.Contains(got, "结尾") {
		t.Fatalf("去掉脚本时丢了正文：\n%s", got)
	}
}

func TestHTMLDropsUnquotedEventsButKeepsLiteralText(t *testing.T) {
	source := "<p onclick=alert(1) title=\"onclick=&#34;文字&#34;\">正文里写了 onclick=\"文字\" 不是属性</p>\n"
	got := Parse(source).HTML()
	lower := strings.ToLower(got)
	if strings.Contains(lower, "<p onclick") || strings.Contains(lower, "alert(1)") {
		t.Fatalf("无引号事件属性没有去掉：\n%s", got)
	}
	if !strings.Contains(got, `onclick="文字"`) && !strings.Contains(got, `onclick=&#34;文字&#34;`) {
		t.Fatalf("正文或属性值里的普通文字被误删：\n%s", got)
	}
	if !strings.Contains(got, "不是属性") {
		t.Fatalf("去掉事件时丢了正文：\n%s", got)
	}
}

func TestHTMLCrossBlockFootnoteDefinitions(t *testing.T) {
	source := "见[^a]与[^b]。\n\n<script>x</script>\n\n[^b]: 后定义\n\n中间提到[^a]。\n\n[^a]: 前定义\n"
	got := Parse(source).HTML()
	for _, want := range []string{`href="#fn-a"`, `href="#fn-b"`, "前定义", "后定义", `id="fnref-a-2"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("交叉脚注丢失 %q：\n%s", want, got)
		}
	}
	if strings.Count(got, "前定义") != 1 || strings.Count(got, "后定义") != 1 {
		t.Fatalf("脚注定义被重复渲染：\n%s", got)
	}
	if strings.Contains(strings.ToLower(got), "<script") {
		t.Fatalf("原始 HTML 的脚本没有去掉：\n%s", got)
	}
}

func TestHTMLKeepsRawSourceBytes(t *testing.T) {
	source := "正文 onclick=\"文字\" 仍在\n"
	doc := Parse(source)
	before := doc.Markdown()
	_ = doc.HTML()
	if doc.Markdown() != before {
		t.Fatalf("导出改写了原文：\n前：%q\n后：%q", before, doc.Markdown())
	}
}

func TestIsSafeSVGRejectsActiveContent(t *testing.T) {
	safe := []string{
		`  <svg></svg>  `,
		`<svg xmlns="http://www.w3.org/2000/svg"><g fill="#112233"><path d="M0 0"/><rect width="1" height="1"/><line x1="0" y1="0" x2="1" y2="1"/><text font-family="Songti SC">中文</text></g></svg>`,
	}
	unsafe := []string{
		`<svg><script>alert(1)</script></svg>`,
		`<svg><foreignObject><iframe></iframe></foreignObject></svg>`,
		`<div><svg></svg></div>`,
		`<svg onclick="alert(1)"></svg>`,
		`<svg><image href="https://example.com/a.png"/></svg>`,
		`<svg></svg><svg></svg>`,
		`<svg><text>onclick="文字"</text><script></script></svg>`,
	}
	for _, svg := range safe {
		if !isSafeSVG(svg) {
			t.Fatalf("合法 SVG 被拒绝：%q", svg)
		}
	}
	for _, svg := range unsafe {
		if isSafeSVG(svg) {
			t.Fatalf("不安全的 SVG 被接受：%q", svg)
		}
	}
}
