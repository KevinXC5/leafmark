package nativeeditor

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/diagram"
	"leafmark/internal/mathlayout"
)

// 块级公式比正文略大，与常见的数学排版一致。
const displayMathScale = 1.2

type mathKey struct {
	src     string
	size    int
	display bool
}

type diagramKey struct {
	src   string
	width int
	theme string
}

// math 排版一条公式并缓存结果。源码不在支持范围内时返回 nil，调用方退回显示原文。
func (c *shapeCache) math(src string, size float32, display bool) *mathlayout.Box {
	key := mathKey{src: src, size: int(size * 100), display: display}
	if box, ok := c.maths[key]; ok {
		return box
	}
	box, err := mathlayout.Layout(src, size, display)
	if err != nil {
		box = nil
	}
	if len(c.maths) >= 512 {
		clear(c.maths)
	}
	c.maths[key] = box
	return box
}

// diagram 解析并布局一张流程图。不是受支持的图类型时返回 nil。
func (c *shapeCache) diagram(src string, width float32) *diagram.Layout {
	key := diagramKey{src: src, width: int(width), theme: fmt.Sprint(c.diagramStyle.Text, c.diagramStyle.NodeFill, c.diagramStyle.NodeLine)}
	if lay, ok := c.diagrams[key]; ok {
		return lay
	}
	var lay *diagram.Layout
	if graph, err := diagram.Parse(src); err == nil {
		lay = graph.Layout(c.diagramStyle, width)
	}
	if len(c.diagrams) >= 64 {
		clear(c.diagrams)
	}
	c.diagrams[key] = lay
	return lay
}

// diagramStyle 按当前主题给出流程图的配色：节点、连线与分组取纸色和墨色的同一色调。
func diagramStyle(theme *ui.Theme) diagram.Style {
	st := diagram.Style{
		Font: ui.Font{Family: "system-ui, sans-serif", Size: 16}, Text: theme.Text, LabelFill: theme.Background,
		NodeFill: ui.Hex("#f3e8dd"), NodeLine: ui.Hex("#d6b19a"), Edge: ui.Hex("#9b8b7b"), GroupFill: ui.Hex("#f4f0e8"), GroupLine: ui.Hex("#e0d6c8"),
	}
	if theme.Dark {
		st.NodeFill, st.NodeLine, st.Edge, st.GroupFill, st.GroupLine = ui.Hex("#343b44"), ui.Hex("#6c7c8d"), ui.Hex("#9da7b3"), ui.Hex("#2a2c30"), ui.Hex("#43464d")
	}
	return st
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

// placeRendered 尝试把原文块排成块级公式或流程图，成功时返回高度。
func placeRendered(box *blockBox, raw string, fontSize float32, cache *shapeCache, lay *layout) (float32, bool) {
	h := float32(0)
	if src, ok := displayMathSource(raw); ok {
		if box.math = cache.math(src, fontSize*displayMathScale, true); box.math != nil {
			h = box.math.Ascent + box.math.Descent + 16
		}
	} else if src, ok := mermaidSource(raw); ok {
		if box.diagram = cache.diagram(src, box.w); box.diagram != nil {
			h = box.diagram.Height + 16
		}
	}
	if h == 0 {
		return 0, false
	}
	// 一条不占文字的行，供光标与命中使用。
	lay.lines = append(lay.lines, line{block: box.index, y: box.y, ascent: h, height: h, origin: box.origin})
	box.lines = append(box.lines, len(lay.lines)-1)
	return h, true
}
