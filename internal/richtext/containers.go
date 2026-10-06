package richtext

import (
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"html"
	"strings"
)

// expandContainer 只展开旧模型不能表示的容器，简单列表与引用沿用原有坐标行为。
func expandContainer(n ast.Node, src []byte, nextID *int) ([]editBlock, bool) {
	switch v := n.(type) {
	case *ast.List:
		if _, ok := convertListItems(v, src); ok {
			return nil, false
		}
	case *ast.Blockquote:
		if convertQuote(v, src, 1).Kind != Raw {
			return nil, false
		}
	case *extast.Footnote:
	default:
		return nil, false
	}
	var out []editBlock
	var visit func(ast.Node, []Container)
	visit = func(node ast.Node, parents []Container) {
		switch v := node.(type) {
		case *ast.List:
			number := max(1, v.Start)
			for item := v.FirstChild(); item != nil; item = item.NextSibling() {
				*nextID++
				id := *nextID
				checked, task := taskState(item.FirstChild())
				kind := List
				if task {
					kind = Task
				}
				ctx := Container{Kind: kind, ID: id, Level: len(parents) + 1, Ordered: v.IsOrdered(), Start: number, Checked: checked}
				before := len(out)
				for child := item.FirstChild(); child != nil; child = child.NextSibling() {
					visit(child, appendContainer(parents, ctx))
				}
				if len(out) == before {
					out = append(out, editBlock{Block: Block{Kind: Paragraph, Containers: appendContainer(parents, ctx)}})
				}
				if len(out) > before && out[before].Kind == Paragraph {
					out[before].Kind = kind
					out[before].Level = ctx.Level
					out[before].Ordered = ctx.Ordered
					out[before].Start = ctx.Start
					out[before].Checked = checked
				}
				number++
			}
		case *ast.Blockquote:
			*nextID++
			ctx := Container{Kind: Quote, ID: *nextID, Level: len(parents) + 1}
			first := v.FirstChild()
			if p, ok := first.(*ast.Paragraph); ok && p.Lines().Len() > 0 {
				line := p.Lines().At(0)
				m := calloutMarker.FindStringSubmatch(strings.TrimSpace(string(line.Value(src))))
				if m != nil {
					ctx.Kind = Callout
					ctx.Callout = &CalloutData{Type: m[1], Fold: m[2], Title: m[3]}
				}
			}
			before := len(out)
			for child := v.FirstChild(); child != nil; child = child.NextSibling() {
				if ctx.Kind == Callout && child == first {
					p := child.(*ast.Paragraph)
					var body strings.Builder
					for i := 1; i < p.Lines().Len(); i++ {
						line := p.Lines().At(i)
						body.Write(line.Value(src))
					}
					if body.Len() > 0 {
						d := parseBody(body.String())
						for _, b := range d.blocks {
							b.Containers = appendContainer(parents, ctx)
							out = append(out, b)
						}
					}
				} else {
					visit(child, appendContainer(parents, ctx))
				}
			}
			if len(out) == before {
				out = append(out, editBlock{Block: Block{Kind: Paragraph, Containers: appendContainer(parents, ctx)}})
			}
		case *extast.Footnote:
			*nextID++
			ctx := Container{Kind: FootnoteDef, ID: *nextID, Level: len(parents) + 1, Footnote: string(v.Ref)}
			before := len(out)
			for child := v.FirstChild(); child != nil; child = child.NextSibling() {
				visit(child, appendContainer(parents, ctx))
			}
			if len(out) == before {
				out = append(out, editBlock{Block: Block{Kind: Paragraph, Containers: appendContainer(parents, ctx)}})
			}
			for i := before; i < len(out); i++ {
				out[i].Footnote = &FootnoteData{Label: string(v.Ref), Number: v.Index}
			}
		default:
			b := convertBlock(node, src, 1)
			if tb, ok := node.(*ast.TextBlock); ok {
				runs, supported := inlineRuns(tb, src)
				if supported {
					b = editBlock{Block: Block{Kind: Paragraph, Runs: runs}}
				}
			}
			b.Containers = append([]Container(nil), parents...)
			b.source = ""
			b.gap = ""
			out = append(out, b)
		}
	}
	visit(n, nil)
	return out, len(out) > 0
}

func appendContainer(parents []Container, ctx Container) []Container {
	out := append([]Container(nil), parents...)
	return append(out, ctx)
}

// writeContainerMD 把线性叶块恢复为容器结构，空白行不附加尾随空格。
func writeContainerMD(blocks []editBlock, depth int) string {
	var out strings.Builder
	for i := 0; i < len(blocks); {
		b := blocks[i]
		j := i + 1
		piece := ""
		list := false
		if depth >= len(b.Containers) {
			if b.Kind == List || b.Kind == Task {
				b.Kind = Paragraph
			}
			b.Containers = nil
			var text strings.Builder
			writeBlockMD(&text, &b)
			piece = text.String()
		} else {
			ctx := b.Containers[depth]
			for j < len(blocks) && len(blocks[j].Containers) > depth && blocks[j].Containers[depth].ID == ctx.ID {
				j++
			}
			body := writeContainerMD(blocks[i:j], depth+1)
			switch {
			case ctx.IsHTML():
				open := "<details>"
				if ctx.Callout != nil && ctx.Callout.Fold == "+" {
					open = "<details open>"
				}
				title := ""
				if ctx.Callout != nil {
					title = ctx.Callout.Title
				}
				// HTML 容器内部使用 HTML 叶块，Markdown 文本不会在 details 内自动解析。
				var content strings.Builder
				for _, leaf := range blocks[i:j] {
					leaf.Containers = nil
					leaf.Footnote = nil
					doc := &Document{blocks: []editBlock{leaf}}
					content.WriteString(doc.HTML())
				}
				piece = open + "<summary>" + html.EscapeString(title) + "</summary>\n" + content.String() + "</details>"
			case ctx.Footnote != "":
				piece = "[^" + ctx.Footnote + "]: " + indentBody(body, "    ")
			case ctx.Kind == Quote || ctx.Kind == Callout:
				if ctx.Kind == Callout && ctx.Callout != nil {
					header := "[!" + ctx.Callout.Type + "]" + ctx.Callout.Fold
					if ctx.Callout.Title != "" {
						header += " " + ctx.Callout.Title
					}
					body = header + "\n" + body
				}
				lines := strings.Split(body, "\n")
				for k, line := range lines {
					if line == "" {
						lines[k] = ">"
					} else {
						lines[k] = "> " + line
					}
				}
				piece = strings.Join(lines, "\n")
			case ctx.Kind == List || ctx.Kind == Task:
				list = true
				marker := "- "
				if ctx.Ordered {
					marker = itoaSmall(max(1, ctx.Start)) + ". "
				}
				indent := strings.Repeat(" ", len(marker))
				if ctx.Kind == Task {
					if ctx.Checked {
						marker += "[x] "
					} else {
						marker += "[ ] "
					}
				}
				piece = marker + indentBody(body, indent)
			default:
				piece = body
			}
		}
		if out.Len() > 0 {
			sep := "\n\n"
			if list && i > 0 && len(blocks[i-1].Containers) > depth {
				prev := blocks[i-1].Containers[depth]
				if prev.Kind == List || prev.Kind == Task {
					sep = "\n"
				}
			}
			out.WriteString(sep)
		}
		out.WriteString(strings.TrimRight(piece, "\r\n"))
		i = j
	}
	return out.String()
}

func indentBody(body, indent string) string {
	lines := strings.Split(body, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
