package richtext

import "testing"

func TestRoundTrip(t *testing.T) {
	samples := []string{
		"hello",
		"hello\n\nworld\n",
		"# 标题\n",
		"a **b** *c* ~~d~~ `e` [f](u)\n",
		"- item\n",
		"1. item\n",
		"- [x] done\n",
		"> hi\n",
		"```go\nfmt.Println()\n```\n",
		"---\n",
		"![alt](./a.png)\n",
		"![x](https://example.com/a.png)\n",
		"| a | b |\n| --- | --- |\n| 1 | 2 |\n",
		"<div>\n<b>x</b>\n</div>\n",
		"- a\n  - b\n",
		"a \\* b\n",
		"# T\n\npara **b**\n\n- [ ] t\n",
		"text ![a](u)\n",
		"",
		"hello\n",
	}
	for _, md := range samples {
		got := Parse(md).Markdown()
		if got != md {
			t.Errorf("\n in %q\nout %q", md, got)
		}
	}
}
