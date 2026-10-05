package richtext

import (
	"bytes"
	"html"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

// rawHTML 把未进入模型的原文交给通用渲染器，脚注、原始 HTML 等仍能导出。
var rawHTML = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote),
	goldmark.WithRendererOptions(ghtml.WithUnsafe()),
)

// HTML 把文档写成 HTML 片段，供导出与打印使用。
func (d *Document) HTML() string { return d.HTMLWith(nil) }

// HTMLWith 与 HTML 相同，图片地址先经 image 改写（例如换成内嵌数据），返回空串时保留原地址。
func (d *Document) HTMLWith(image func(url string) string) string {
	var out strings.Builder
	list := []string{} // 当前打开的列表标签栈
	closeTo := func(depth int) {
		for len(list) > depth {
			out.WriteString("</li></" + list[len(list)-1] + ">\n")
			list = list[:len(list)-1]
		}
	}
	for i := range d.blocks {
		b := &d.blocks[i]
		if b.Kind == List || b.Kind == Task {
			level := max(1, b.Level)
			tag := "ul"
			if b.Ordered {
				tag = "ol"
			}
			// 顶层有序与无序交替时是两个列表。
			if level == 1 && len(list) > 0 && list[0] != tag {
				closeTo(0)
			}
			if len(list) >= level {
				closeTo(level)
				out.WriteString("</li>\n")
			}
			for len(list) < level {
				open := "<" + tag
				if tag == "ol" && b.Start > 1 {
					open += ` start="` + strconv.Itoa(b.Start) + `"`
				}
				out.WriteString(open + ">\n")
				list = append(list, tag)
				if len(list) < level {
					out.WriteString("<li>")
				}
			}
			if b.Kind == Task {
				checked := ""
				if b.Checked {
					checked = " checked"
				}
				out.WriteString(`<li class="task"><input type="checkbox" disabled` + checked + "> ")
			} else {
				out.WriteString("<li>")
			}
			writeRunsHTML(&out, b.Runs)
			continue
		}
		closeTo(0)
		switch b.Kind {
		case Heading:
			tag := "h" + strconv.Itoa(max(1, min(b.Level, 6)))
			out.WriteString("<" + tag + ">")
			writeRunsHTML(&out, b.Runs)
			out.WriteString("</" + tag + ">\n")
		case Quote:
			out.WriteString("<blockquote><p>")
			writeRunsHTML(&out, b.Runs)
			out.WriteString("</p></blockquote>\n")
		case Callout:
			kind, title := "note", ""
			if b.Callout != nil {
				kind, title = strings.ToLower(b.Callout.Type), b.Callout.Title
			}
			out.WriteString(`<div class="callout callout-` + html.EscapeString(kind) + `">`)
			if title != "" {
				out.WriteString(`<p class="callout-title">` + html.EscapeString(title) + "</p>")
			}
			if len(b.Runs) > 0 {
				out.WriteString("<p>")
				writeRunsHTML(&out, b.Runs)
				out.WriteString("</p>")
			}
			out.WriteString("</div>\n")
		case Code:
			class := ""
			if b.Lang != "" {
				class = ` class="language-` + html.EscapeString(strings.Fields(b.Lang)[0]) + `"`
			}
			out.WriteString("<pre><code" + class + ">" + html.EscapeString(b.Code) + "</code></pre>\n")
		case Horizontal:
			out.WriteString("<hr>\n")
		case TableBlock:
			writeTableHTML(&out, b.Table)
		case Image:
			title := ""
			if b.Title != "" {
				title = ` title="` + html.EscapeString(b.Title) + `"`
			}
			src := b.URL
			if image != nil {
				if mapped := image(src); mapped != "" {
					src = mapped
				}
			}
			out.WriteString(`<p><img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(b.Alt) + `"` + title + "></p>\n")
		case Raw:
			source := b.Raw
			if source == "" {
				source = b.source
			}
			var buf bytes.Buffer
			if err := rawHTML.Convert([]byte(source), &buf); err != nil {
				out.WriteString("<pre>" + html.EscapeString(source) + "</pre>\n")
			} else {
				out.Write(buf.Bytes())
			}
		default:
			out.WriteString("<p>")
			writeRunsHTML(&out, b.Runs)
			out.WriteString("</p>\n")
		}
	}
	closeTo(0)
	return out.String()
}

func writeRunsHTML(out *strings.Builder, runs []Run) {
	for _, r := range runs {
		text := strings.ReplaceAll(html.EscapeString(r.Text), "\n", "<br>\n")
		// 标签由内到外套，顺序固定，输出稳定。
		for _, m := range []struct {
			mark Mark
			tag  string
		}{{MarkCode, "code"}, {MarkMath, `span class="math"`}, {MarkSub, "sub"}, {MarkSup, "sup"}, {MarkStrike, "del"}, {MarkItalic, "em"}, {MarkBold, "strong"}, {MarkHighlight, "mark"}} {
			if r.Marks&m.mark != 0 {
				text = "<" + m.tag + ">" + text + "</" + strings.Fields(m.tag)[0] + ">"
			}
		}
		if r.Link != nil {
			title := ""
			if r.Link.Title != "" {
				title = ` title="` + html.EscapeString(r.Link.Title) + `"`
			}
			text = `<a href="` + html.EscapeString(r.Link.URL) + `"` + title + ">" + text + "</a>"
		}
		out.WriteString(text)
	}
}

func writeTableHTML(out *strings.Builder, tb *TableData) {
	if tb == nil {
		return
	}
	out.WriteString("<table>\n")
	for ri, row := range tb.Rows {
		cell := "td"
		if tb.Header && ri == 0 {
			cell = "th"
			out.WriteString("<thead>")
		} else if ri == 0 || (tb.Header && ri == 1) {
			out.WriteString("<tbody>")
		}
		out.WriteString("<tr>")
		for ci, c := range row {
			align := ""
			if ci < len(tb.Aligns) {
				switch tb.Aligns[ci] {
				case AlignLeft:
					align = ` style="text-align:left"`
				case AlignCenter:
					align = ` style="text-align:center"`
				case AlignRight:
					align = ` style="text-align:right"`
				}
			}
			out.WriteString("<" + cell + align + ">")
			writeRunsHTML(out, c.Runs)
			out.WriteString("</" + cell + ">")
		}
		out.WriteString("</tr>")
		if tb.Header && ri == 0 {
			out.WriteString("</thead>")
		}
		out.WriteString("\n")
	}
	if len(tb.Rows) > 1 || !tb.Header {
		out.WriteString("</tbody>")
	}
	out.WriteString("</table>\n")
}
