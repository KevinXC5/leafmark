package nativeeditor

import (
	"bytes"
	"encoding/xml"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// SVG 文字由原生字体引擎排版，中文沿用系统字体回退；缓存最近一次显示比例的字形。
type svgText struct {
	text         string
	x, y, sx, sy float32
	font         ui.Font
	color        ui.Color
	anchor       string
	scale        float32
	glyphs       []ui.Glyph
}

func (t *svgText) paint(p *ui.Painter, fit ui.Rect, scale float32) {
	if t.scale != scale || t.glyphs == nil {
		font := t.font
		font.Size *= t.sy * scale
		t.glyphs = ui.Shape(t.text, font)
		ratio := t.sx / t.sy
		for i := range t.glyphs {
			t.glyphs[i].X *= ratio
			t.glyphs[i].Advance *= ratio
		}
		t.scale = scale
	}
	x := fit.X + t.x*scale
	switch t.anchor {
	case "middle":
		x -= advanceOf(t.glyphs) / 2
	case "end":
		x -= advanceOf(t.glyphs)
	}
	p.Glyphs(t.glyphs, x, fit.Y+t.y*scale, t.color)
}

type svgTextNode struct {
	name     string
	attrs    map[string]string
	text     string
	children []*svgTextNode
}

type svgTextRule struct {
	selector string
	values   map[string]string
	rank     int
}

func svgDeclarations(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		if k, v, ok := strings.Cut(part, ":"); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func svgTextRules(root *svgTextNode) []svgTextRule {
	var rules []svgTextRule
	var visit func(*svgTextNode)
	visit = func(n *svgTextNode) {
		if n.name == "style" {
			var css strings.Builder
			for _, child := range n.children {
				css.WriteString(child.text)
			}
			rest := css.String()
			for {
				selectors, tail, ok := strings.Cut(rest, "{")
				if !ok {
					break
				}
				body, next, ok := strings.Cut(tail, "}")
				if !ok {
					break
				}
				rest = next
				for _, selector := range strings.Split(selectors, ",") {
					selector = strings.TrimSpace(selector)
					if selector == "" || strings.ContainsAny(selector, " >+~[:") {
						continue
					}
					rank := 1
					if strings.Contains(selector, ".") {
						rank += 10
					}
					if strings.Contains(selector, "#") {
						rank += 100
					}
					rules = append(rules, svgTextRule{selector, svgDeclarations(body), rank})
				}
			}
		}
		for _, child := range n.children {
			visit(child)
		}
	}
	visit(root)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].rank < rules[j].rank })
	return rules
}

func svgSelectorMatches(selector string, n *svgTextNode) bool {
	if selector == "*" || selector == n.name {
		return true
	}
	if i := strings.IndexAny(selector, ".#"); i >= 0 {
		if i > 0 && selector[:i] != n.name {
			return false
		}
		value := selector[i+1:]
		if selector[i] == '#' {
			return n.attrs["id"] == value
		}
		for _, class := range strings.Fields(n.attrs["class"]) {
			if class == value {
				return true
			}
		}
	}
	return false
}

// svgTexts 支持 text/tspan、常用简单 CSS 选择器和轴对齐变换，不访问外部字体或资源。
func svgTexts(raw []byte, width, height float32) []svgText {
	root := &svgTextNode{}
	stack := []*svgTextNode{root}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		parent := stack[len(stack)-1]
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) > 128 {
				return nil
			}
			n := &svgTextNode{name: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[a.Name.Local] = a.Value
			}
			parent.children = append(parent.children, n)
			stack = append(stack, n)
		case xml.CharData:
			parent.children = append(parent.children, &svgTextNode{text: string(t)})
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	rules := svgTextRules(root)
	var out []svgText
	var walk func(*svgTextNode, map[string]string, affine, bool, *float32, *float32)
	walk = func(n *svgTextNode, inherited map[string]string, m affine, inside bool, x, y *float32) {
		if len(out) >= maxSVGOverlays {
			return
		}
		switch n.name {
		case "defs", "style", "script", "foreignObject", "title", "desc", "metadata", "clipPath", "mask", "symbol":
			return
		}
		style := map[string]string{}
		for k, v := range inherited {
			style[k] = v
		}
		for k, v := range n.attrs {
			style[k] = v
		}
		// CSS 的选择器样式优先于 presentation 属性，style 属性优先级最高。
		for _, rule := range rules {
			if svgSelectorMatches(rule.selector, n) {
				for k, v := range rule.values {
					style[k] = v
				}
			}
		}
		for k, v := range svgDeclarations(n.attrs["style"]) {
			style[k] = v
		}
		if style["display"] == "none" || style["visibility"] == "hidden" {
			return
		}
		if n.name == "svg" {
			m = m.mul(viewBoxAffine(n.attrs["viewBox"], float64(width), float64(height)))
		}
		if v := n.attrs["transform"]; v != "" {
			m = m.mul(parseAffine(v))
		}
		if n.name == "text" {
			inside = true
			xx, yy := float32(0), float32(0)
			x, y = &xx, &yy
		}
		if inside {
			if v, ok := n.attrs["x"]; ok {
				*x = float32(svgNumber(v))
			}
			if v, ok := n.attrs["y"]; ok {
				*y = float32(svgNumber(v))
			}
			*x += float32(svgNumber(n.attrs["dx"]))
			*y += float32(svgNumber(n.attrs["dy"]))
			if n.text != "" && strings.TrimSpace(n.text) != "" && math.Abs(m.b) < 1e-6 && math.Abs(m.c) < 1e-6 && m.a > 0 && m.d > 0 {
				text := strings.Join(strings.Fields(n.text), " ")
				size := float32(svgNumber(style["font-size"]))
				if size <= 0 {
					size = 16
				}
				weight, _ := strconv.Atoi(style["font-weight"])
				if style["font-weight"] == "bold" {
					weight = 700
				}
				if weight == 0 {
					weight = 400
				}
				family := strings.ReplaceAll(style["font-family"], "\"", "")
				font := ui.Font{Family: family, Size: size, Weight: weight, Italic: style["font-style"] == "italic"}
				color := svgTextColor(style["fill"])
				for _, key := range []string{"opacity", "fill-opacity"} {
					if v, ok := style[key]; ok {
						alpha, err := strconv.ParseFloat(v, 32)
						if err == nil {
							color = color.Alpha(float32(alpha) * float32(color.A) / 255)
						}
					}
				}
				out = append(out, svgText{text: text, x: float32(m.a*float64(*x) + m.e), y: float32(m.d*float64(*y) + m.f), sx: float32(m.a), sy: float32(m.d), font: font, color: color, anchor: style["text-anchor"]})
				*x += advanceOf(ui.Shape(text, font))
			}
		}
		// 几何属性不继承，避免父级 x/dx/transform 在子节点重复应用。
		for _, k := range []string{"x", "y", "dx", "dy", "transform", "viewBox", "id", "class", "style"} {
			delete(style, k)
		}
		for _, child := range n.children {
			if n.name == "switch" {
				// 与位图替身使用同一选择规则，避免 draw.io 的备用说明重复覆盖文字。
				if child.name == "" || child.name == "foreignObject" || child.attrs["requiredExtensions"] != "" {
					continue
				}
				walk(child, style, m, inside, x, y)
				break
			}
			walk(child, style, m, inside, x, y)
		}
	}
	x, y := float32(0), float32(0)
	walk(root, map[string]string{}, identityAffine, false, &x, &y)
	return out
}

func svgTextColor(value string) ui.Color {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		return ui.Hex(value)
	}
	switch value {
	case "none", "transparent":
		return ui.Transparent
	case "white":
		return ui.RGB(255, 255, 255)
	case "red":
		return ui.RGB(255, 0, 0)
	case "blue":
		return ui.RGB(0, 0, 255)
	case "green":
		return ui.RGB(0, 128, 0)
	}
	if strings.HasPrefix(value, "rgb(") {
		parts := strings.FieldsFunc(strings.TrimSuffix(strings.TrimPrefix(value, "rgb("), ")"), func(r rune) bool { return r == ',' || r == ' ' })
		if len(parts) == 3 {
			var rgb [3]uint8
			for i, v := range parts {
				rgb[i] = uint8(max(0, min(255, svgNumber(v))))
			}
			return ui.RGB(rgb[0], rgb[1], rgb[2])
		}
	}
	return ui.RGB(0, 0, 0)
}
