package richtext

import (
	"strconv"
	"strings"
	"unicode"
)

// Markdown 序列化。未改动的块回写源码，Raw 原样保留。
// 整文纯文本（NewPlainText）直接返回正文，不包代码围栏。
func (d *Document) Markdown() string {
	if len(d.blocks) == 1 && d.blocks[0].plain {
		return d.blocks[0].Code
	}
	var out strings.Builder
	out.WriteString(d.prefix)
	for i := 0; i < len(d.blocks); i++ {
		bl := &d.blocks[i]
		if bl.group != 0 {
			j := i + 1
			dirty := bl.dirty
			for j < len(d.blocks) && d.blocks[j].group == bl.group {
				dirty = dirty || d.blocks[j].dirty
				j++
			}
			piece := bl.source
			if dirty || piece == "" {
				piece = writeContainerMD(d.blocks[i:j], 0) + trailingLineBreak(bl.source)
				if j < len(d.blocks) && !strings.HasSuffix(piece+d.blocks[j-1].gap, "\n\n") {
					piece += "\n"
				}
			}
			out.WriteString(piece)
			out.WriteString(d.blocks[j-1].gap)
			if j < len(d.blocks) && d.blocks[j-1].gap == "" && !strings.HasSuffix(piece, "\n") {
				out.WriteString("\n\n")
			}
			i = j - 1
			continue
		}
		piece := bl.source
		if bl.dirty || piece == "" {
			var buf strings.Builder
			writeBlockMD(&buf, bl)
			piece = buf.String()
			if strings.HasSuffix(bl.source, "\r\n") {
				piece += "\r\n"
			} else if strings.HasSuffix(bl.source, "\n") {
				piece += "\n"
			}
		}
		out.WriteString(piece)
		out.WriteString(bl.gap)
		if i+1 < len(d.blocks) && bl.gap == "" && !strings.HasSuffix(piece, "\n") {
			if (bl.Kind == List || bl.Kind == Task) && (d.blocks[i+1].Kind == List || d.blocks[i+1].Kind == Task) {
				out.WriteByte('\n')
			} else {
				out.WriteString("\n\n")
			}
		}
	}
	return out.String()
}

func writeBlockMD(b *strings.Builder, bl *editBlock) {
	switch bl.Kind {
	case Heading:
		level := bl.Level
		if level < 1 {
			level = 1
		}
		for i := 0; i < level; i++ {
			b.WriteByte('#')
		}
		b.WriteByte(' ')
		writeRunsMD(b, bl.Runs)
	case Callout:
		data := bl.Callout
		if data == nil {
			data = &CalloutData{Type: "note"}
		}
		prefix := strings.Repeat("> ", max(1, bl.Level))
		b.WriteString(prefix + "[!" + data.Type + "]" + data.Fold)
		if data.Title != "" {
			b.WriteString(" " + data.Title)
		}
		if len(bl.Runs) > 0 {
			var body strings.Builder
			writeRunsMD(&body, bl.Runs)
			b.WriteString("\n" + prefix + strings.ReplaceAll(body.String(), "\n", "\n"+prefix))
		}
	case Quote:
		var body strings.Builder
		writeRunsMD(&body, bl.Runs)
		prefix := strings.Repeat("> ", max(1, bl.Level))
		b.WriteString(prefix + strings.ReplaceAll(body.String(), "\n", "\n"+prefix))
	case List:
		b.WriteString(listIndent(bl))
		if bl.Ordered {
			n := bl.Start
			if n <= 0 {
				n = 1
			}
			b.WriteString(itoaSmall(n) + ". ")
		} else {
			b.WriteString("- ")
		}
		writeListRuns(b, bl.Runs, listIndent(bl))
	case Task:
		b.WriteString(listIndent(bl))
		if bl.Ordered {
			b.WriteString(itoaSmall(max(1, bl.Start)) + ". ")
		} else {
			b.WriteString("- ")
		}
		if bl.Checked {
			b.WriteString("[x] ")
		} else {
			b.WriteString("[ ] ")
		}
		writeListRuns(b, bl.Runs, listIndent(bl))
	case Code:
		if bl.indented {
			lines := strings.Split(strings.TrimSuffix(bl.Code, "\n"), "\n")
			for i, line := range lines {
				if i > 0 {
					b.WriteByte('\n')
				}
				if line != "" {
					b.WriteString("    " + line)
				}
			}
			return
		}
		fence := strings.Repeat("`", max(3, maxBackticks(bl.Code)+1))
		b.WriteString(fence + bl.Lang + "\n" + bl.Code)
		if !strings.HasSuffix(bl.Code, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(fence)
	case Horizontal:
		b.WriteString("---")
	case TableBlock:
		writeTableMD(b, bl.Table)
	case Image:
		if embed, ok := wikiEmbed(bl.URL, bl.Alt, bl.Size); ok && bl.Wiki {
			b.WriteString(embed)
			return
		}
		b.WriteString("![" + escapeText(bl.Alt) + "](" + linkDestination(bl.URL) + linkTitle(bl.Title) + ")")
	case Raw:
		if bl.Raw != "" {
			b.WriteString(bl.Raw)
		} else {
			b.WriteString(bl.source)
		}
	default:
		writeRunsMD(b, bl.Runs)
	}
}

// writeRunsMD 以连续样式区间嵌套输出，避免相邻强调标记互相吞并。
func writeRunsMD(b *strings.Builder, runs []Run) { writeMarkedRuns(b, runs, 0) }

func writeMarkedRuns(b *strings.Builder, runs []Run, depth int) {
	marks := []Mark{MarkHighlight, MarkBold, MarkItalic, MarkStrike}
	if depth < len(marks) {
		mark := marks[depth]
		delimiter := map[Mark]string{MarkBold: "**", MarkItalic: "*", MarkStrike: "~~", MarkHighlight: "=="}[mark]
		for i := 0; i < len(runs); {
			j := i + 1
			marked := runs[i].Marks&mark != 0
			for j < len(runs) && (runs[j].Marks&mark != 0) == marked {
				j++
			}
			var inner strings.Builder
			writeMarkedRuns(&inner, runs[i:j], depth+1)
			text := inner.String()
			if marked {
				body := strings.TrimFunc(text, unicode.IsSpace)
				leading := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
				trailing := len(text) - len(strings.TrimRightFunc(text, unicode.IsSpace))
				if body != "" {
					b.WriteString(text[:leading])
					b.WriteString(delimiter + body + delimiter)
					b.WriteString(text[len(text)-trailing:])
				} else {
					b.WriteString(text)
				}
			} else {
				b.WriteString(text)
			}
			i = j
		}
		return
	}
	for _, r := range runs {
		if r.Link != nil && r.Link.Footnote != "" {
			b.WriteString("[^" + r.Link.Footnote + "]")
			continue
		}
		if r.Image != nil {
			img := r.Image
			if embed, ok := wikiEmbed(img.URL, img.Alt, img.Size); ok && img.Wiki {
				b.WriteString(embed)
				continue
			}
			alt := "![" + escapeText(img.Alt) + "]"
			if img.Ref != "" {
				b.WriteString(alt + "[" + img.Ref + "]")
			} else {
				b.WriteString(alt + "(" + linkDestination(img.URL) + linkTitle(img.Title) + ")")
			}
			continue
		}
		text := escapeText(r.Text)
		if r.Marks&MarkMath != 0 {
			// 公式内容是 TeX 原文，不做 Markdown 转义。
			text = "$" + r.Text + "$"
		} else if r.Marks&(MarkSub|MarkSup) != 0 {
			// 旧版上下标内部是纯文本；空格必须转义，否则会失去上下标语义。
			delimiter := "~"
			if r.Marks&MarkSup != 0 {
				delimiter = "^"
			}
			text = delimiter + strings.ReplaceAll(text, " ", "\\ ") + delimiter
		} else if r.Marks&MarkCode != 0 {
			fence := strings.Repeat("`", maxBackticks(r.Text)+1)
			content := r.Text
			if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") || (strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ") && strings.TrimSpace(content) != "") {
				content = " " + content + " "
			}
			text = fence + content + fence
		}
		if r.Link != nil {
			if r.Link.Ref != "" {
				text = "[" + text + "][" + r.Link.Ref + "]"
			} else {
				text = "[" + text + "](" + linkDestination(r.Link.URL) + linkTitle(r.Link.Title) + ")"
			}
		}
		b.WriteString(text)
	}
}

func escapeText(text string) string {
	var b strings.Builder
	for _, c := range text {
		if c < 128 && isASCIIPunct(byte(c)) {
			b.WriteByte('\\')
		}
		if c == '\n' {
			b.WriteString("  \n")
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func maxBackticks(text string) int {
	longest, count := 0, 0
	for _, c := range text {
		if c == '`' {
			count++
			longest = max(longest, count)
		} else {
			count = 0
		}
	}
	return longest
}

func linkDestination(url string) string {
	if !strings.ContainsAny(url, " \t\n\r<>") {
		return strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(url)
	}
	return "<" + strings.NewReplacer("\\", "\\\\", "<", "%3C", ">", "%3E", "\n", "%0A").Replace(url) + ">"
}

func linkTitle(title string) string {
	if title == "" {
		return ""
	}
	return " \"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(title) + "\""
}

func writeTableMD(b *strings.Builder, tb *TableData) {
	if tb == nil {
		return
	}
	cols := 0
	for _, row := range tb.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	// 单元格两侧各留一个空格，写出的表格在纯文本里同样易读。
	for r, row := range tb.Rows {
		b.WriteByte('|')
		for c := 0; c < cols; c++ {
			b.WriteByte(' ')
			if c < len(row) {
				var cell strings.Builder
				writeRunsMD(&cell, row[c].Runs)
				b.WriteString(strings.ReplaceAll(cell.String(), "\n", " "))
			}
			b.WriteString(" |")
		}
		if r == 0 {
			b.WriteString("\n|")
			for c := 0; c < cols; c++ {
				align := AlignNone
				if c < len(tb.Aligns) {
					align = tb.Aligns[c]
				}
				switch align {
				case AlignLeft:
					b.WriteString(" :--- |")
				case AlignCenter:
					b.WriteString(" :---: |")
				case AlignRight:
					b.WriteString(" ---: |")
				default:
					b.WriteString(" --- |")
				}
			}
		}
		if r+1 < len(tb.Rows) {
			b.WriteByte('\n')
		}
	}
}

func itoaSmall(n int) string { return strconv.Itoa(n) }

func writeListRuns(b *strings.Builder, runs []Run, indent string) {
	var body strings.Builder
	writeRunsMD(&body, runs)
	b.WriteString(strings.ReplaceAll(body.String(), "\n", "\n"+indent+"    "))
}

// listIndent 返回列表项行首缩进：优先沿用原文，新建的嵌套项每层四个空格。
func listIndent(bl *editBlock) string {
	if bl.indent != "" || bl.Level <= 1 {
		return bl.indent
	}
	return strings.Repeat("    ", bl.Level-1)
}
