package richtext

import (
	"encoding/xml"
	"io"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// convertHTML 把受支持的 HTML 块收成可编辑块。不认识的标签整块保留。
func convertHTML(n *ast.HTMLBlock, src []byte, nextID *int) ([]editBlock, bool) {
	raw := n.Text(src)
	blocks, ok := parseNativeHTML(string(raw), nextID)
	if !ok {
		return nil, false
	}
	return blocks, true
}

// normalizeBareAttrs 给没有值的 HTML 属性补上空值，标准库解码器只接受带等号的属性。
func normalizeBareAttrs(source string) string {
	var b strings.Builder
	for i := 0; i < len(source); {
		if source[i] != '<' {
			b.WriteByte(source[i])
			i++
			continue
		}
		end := tagClose(source, i)
		if end < 0 {
			b.WriteString(source[i:])
			break
		}
		tag := source[i:end]
		b.WriteString(quoteBare(tag))
		b.WriteByte('>')
		i = end + 1
	}
	return b.String()
}

// tagClose 返回从 i 开始的标签里 '>' 的位置。引号中的 '>' 不算标签结束。
func tagClose(source string, i int) int {
	quote := byte(0)
	for j := i + 1; j < len(source); j++ {
		c := source[j]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '>' {
			return j
		}
	}
	return -1
}

// quoteBare 把标签名之后的 open 这类裸属性写成 open=""。引号里的内容原样保留。
func quoteBare(tag string) string {
	var b strings.Builder
	quote := byte(0)
	i := 0
	if len(tag) > 1 && tag[1] == '/' {
		i = 2
	} else if len(tag) > 0 {
		i = 1
	}
	for i < len(tag) && tag[i] != ' ' && tag[i] != '\t' && tag[i] != '\n' && tag[i] != '\r' && tag[i] != '>' && tag[i] != '/' {
		i++
	}
	b.WriteString(tag[:i])
	for ; i < len(tag); i++ {
		c := tag[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			j := i + 1
			for j < len(tag) && (tag[j] == ' ' || tag[j] == '\t' || tag[j] == '\n' || tag[j] == '\r') {
				j++
			}
			if j >= len(tag) || tag[j] == '/' || tag[j] == '>' {
				b.WriteString(tag[i:])
				break
			}
			k := j
			for k < len(tag) && tag[k] != ' ' && tag[k] != '\t' && tag[k] != '=' && tag[k] != '/' && tag[k] != '>' && tag[k] != '"' && tag[k] != '\'' {
				k++
			}
			m := k
			for m < len(tag) && (tag[m] == ' ' || tag[m] == '\t') {
				m++
			}
			b.WriteString(tag[i:j])
			b.WriteString(tag[j:k])
			if m >= len(tag) || tag[m] != '=' {
				b.WriteString(`=""`)
			}
			i = k - 1
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// parseNativeHTML 只解释文档语义标签，不执行脚本、样式或事件属性。
// 不认识的标签保留为 Raw；受支持内容编辑后按等价 Markdown 写回。
func parseNativeHTML(source string, nextID *int) ([]editBlock, bool) {
	decoder := xml.NewDecoder(strings.NewReader("<leafmark>" + normalizeBareAttrs(source) + "</leafmark>"))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity
	type state struct {
		tag   string
		marks Mark
		link  *Link
	}
	stack := []state{{tag: "leafmark"}}
	var out []editBlock
	var parents []Container
	current := Block{Kind: Paragraph}
	sealed := 0
	flush := func() {
		keep := current.Kind == Code || current.Kind == Image || current.Kind == Horizontal || len(current.Runs) > 0
		if keep {
			out = append(out, editBlock{Block: current})
		}
		current = Block{Kind: Paragraph}
	}
	// 容器标题在子节点里才补全，所以等到容器结束再复制。
	seal := func() {
		for i := sealed; i < len(out); i++ {
			out[i].Containers = append([]Container(nil), parents...)
		}
		sealed = len(out)
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false
		}
		switch t := token.(type) {
		case xml.StartElement:
			tag := strings.ToLower(t.Name.Local)
			next := stack[len(stack)-1]
			next.tag = tag
			attrs := map[string]string{}
			for _, a := range t.Attr {
				attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			switch tag {
			case "leafmark", "span":
			case "b", "strong":
				next.marks |= MarkBold
			case "i", "em":
				next.marks |= MarkItalic
			case "s", "del", "strike":
				next.marks |= MarkStrike
			case "mark":
				next.marks |= MarkHighlight
			case "sub":
				next.marks |= MarkSub
			case "sup":
				next.marks |= MarkSup
			case "code":
				if current.Kind != Code {
					next.marks |= MarkCode
				}
			case "u", "ins":
				next.marks |= MarkUnderline
			case "kbd":
				next.marks |= MarkKbd
			case "a":
				next.link = &Link{URL: attrs["href"], Title: attrs["title"]}
			case "br":
				appendRun(&current.Runs, "\n", next.marks, next.link)
			case "p", "div", "section", "article":
				flush()
			case "h1", "h2", "h3", "h4", "h5", "h6":
				flush()
				current.Kind = Heading
				current.Level = int(tag[1] - '0')
			case "blockquote":
				flush()
				current.Kind = Quote
				current.Level = 1
			case "pre":
				flush()
				current.Kind = Code
			case "ul", "ol":
				flush()
			case "li":
				flush()
				current.Kind = List
				current.Level = 1
				for _, s := range stack {
					if s.tag == "ol" {
						current.Ordered = true
					}
				}
			case "hr":
				flush()
				current.Kind = Horizontal
				flush()
			case "details":
				flush()
				fold := "-"
				if _, open := attrs["open"]; open {
					fold = "+"
				}
				parents = append(parents, Container{
					Kind: Callout, ID: *nextID, Level: len(parents) + 1, html: true,
					Callout: &CalloutData{Type: "details", Fold: fold},
				})
				(*nextID)++
			case "summary":
			case "img":
				flush()
				current = Block{Kind: Image, URL: attrs["src"], Alt: attrs["alt"], Title: attrs["title"]}
				flush()
			default:
				return nil, false
			}
			stack = append(stack, next)
		case xml.EndElement:
			tag := strings.ToLower(t.Name.Local)
			switch tag {
			case "p", "div", "section", "article", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "li", "details":
				flush()
				if tag == "details" && len(parents) > 0 {
					seal()
					parents = parents[:len(parents)-1]
				}
			}
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			value := string(t)
			s := stack[len(stack)-1]
			if s.tag == "summary" && len(parents) > 0 && parents[len(parents)-1].Callout != nil {
				// 解码器已经还原实体，这里不再解一次。
				parents[len(parents)-1].Callout.Title += strings.TrimSpace(string(value))
			} else if current.Kind == Code {
				current.Code += value
			} else if strings.TrimSpace(value) != "" || len(current.Runs) > 0 {
				appendRun(&current.Runs, string(value), s.marks, s.link)
			}
		case xml.Comment:
		default:
			return nil, false
		}
	}
	flush()
	seal()
	return out, len(out) > 0
}
