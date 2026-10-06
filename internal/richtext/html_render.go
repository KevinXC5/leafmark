package richtext

import "strings"

// Renderers 把公式和流程图换成已排好的 SVG。
// 三个函数都可以为空：为空、返回空串或 ok 为 false 时，导出退回源码。
// richtext 不依赖界面库，排版由调用方完成。
type Renderers struct {
	// InlineMath 排版行内公式。src 是两个美元符号之间的原文，不含美元符号。
	// descent 是基线以下的深度，调用方用来把 SVG 对齐到周围文字的基线。
	InlineMath func(src string) (svg string, descent float32, ok bool)
	// DisplayMath 排版块级公式。src 是 $$ 围栏之间的正文。
	DisplayMath func(src string) (svg string, ok bool)
	// Diagram 排版 Mermaid 流程图。src 是围栏代码块的正文，不含围栏行。
	Diagram func(src string) (svg string, ok bool)
}

// HTMLWithRenderers 与 HTMLWith 相同，并按 r 嵌入公式与流程图。
func (d *Document) HTMLWithRenderers(image func(url string) string, r Renderers) string {
	return d.html(image, r)
}

// displayMathSource 取出独占一块的 $$…$$ 公式正文。
func displayMathSource(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if len(s) < 5 || !strings.HasPrefix(s, "$$") || !strings.HasSuffix(s, "$$") {
		return "", false
	}
	body := strings.TrimSpace(s[2 : len(s)-2])
	return body, body != "" && !strings.Contains(body, "$$")
}

// mermaidSource 取出 mermaid 围栏代码块的正文。
func mermaidSource(raw string) (string, bool) {
	lines := strings.Split(strings.TrimRight(raw, "\r\n"), "\n")
	if len(lines) < 3 {
		return "", false
	}
	open := strings.TrimSpace(lines[0])
	fence := strings.TrimRight(open, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ \t\r")
	if len(fence) < 3 || strings.Trim(fence, "`") != "" && strings.Trim(fence, "~") != "" {
		return "", false
	}
	if lang := strings.Fields(strings.TrimSpace(open[len(fence):])); len(lang) == 0 || !strings.EqualFold(lang[0], "mermaid") {
		return "", false
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), fence) {
		return "", false
	}
	return strings.Join(lines[1:len(lines)-1], "\n"), true
}
