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

const svgTextScaleCacheSize = 4

// svgTextScaled 缓存单个量化缩放比例下的字形与总宽度，避免同一 SVG 多尺寸显示时每帧重复塑形。
type svgTextScaled struct {
	key     int32
	glyphs  []ui.Glyph
	advance float32
}

// SVG 文字由原生字体引擎排版，中文沿用系统字体回退；按量化缩放比例缓存最近使用过的字形。
type svgText struct {
	text         string
	x, y, sx, sy float32
	font         ui.Font
	color        ui.Color
	anchor       string
	cache        [svgTextScaleCacheSize]svgTextScaled
	cacheLen     uint8
}

func svgTextScaleKey(scale float32) int32 {
	q := math.Round(float64(scale) * 1000)
	if q < 1 {
		return 1
	}
	if q > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(q)
}

func (t *svgText) shaped(scale float32) ([]ui.Glyph, float32) {
	if !(scale > 0) || !(t.sy > 0) {
		return nil, 0
	}
	key := svgTextScaleKey(scale)
	n := int(t.cacheLen)
	for i := 0; i < n; i++ {
		if t.cache[i].key == key {
			entry := t.cache[i]
			if i > 0 {
				copy(t.cache[1:i+1], t.cache[0:i])
				t.cache[0] = entry
			}
			return entry.glyphs, entry.advance
		}
	}
	font := t.font
	font.Size *= t.sy * scale
	glyphs := ui.Shape(t.text, font)
	if ratio := t.sx / t.sy; ratio != 1 {
		for i := range glyphs {
			glyphs[i].X *= ratio
			glyphs[i].Advance *= ratio
		}
	}
	adv := advanceOf(glyphs)
	if n < svgTextScaleCacheSize {
		n++
		t.cacheLen = uint8(n)
	}
	if n > 1 {
		copy(t.cache[1:n], t.cache[0:n-1])
	}
	t.cache[0] = svgTextScaled{key: key, glyphs: glyphs, advance: adv}
	return glyphs, adv
}

func (t *svgText) paint(p *ui.Painter, fit ui.Rect, scale float32) {
	glyphs, adv := t.shaped(scale)
	if len(glyphs) == 0 {
		return
	}
	x := fit.X + t.x*scale
	switch t.anchor {
	case "middle":
		x -= adv / 2
	case "end":
		x -= adv
	}
	p.Glyphs(glyphs, x, fit.Y+t.y*scale, t.color)
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

// isSVGTextStyleAttr 判断属性是否影响文字排版、配色或节点可见性。
func isSVGTextStyleAttr(k string) bool {
	switch k {
	case "display", "visibility",
		"font-size", "font-weight", "font-family", "font-style",
		"fill", "opacity", "fill-opacity", "text-anchor":
		return true
	}
	return false
}

func visitSVGDeclarations(s string, fn func(k, v string)) {
	for s != "" {
		part, rest, ok := strings.Cut(s, ";")
		if k, v, has := strings.Cut(part, ":"); has {
			fn(strings.TrimSpace(k), strings.TrimSpace(v))
		}
		if !ok {
			break
		}
		s = rest
	}
}

func svgDeclarations(s string) map[string]string {
	out := map[string]string{}
	visitSVGDeclarations(s, func(k, v string) {
		out[k] = v
	})
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
				var values map[string]string
				visitSVGDeclarations(body, func(k, v string) {
					if !isSVGTextStyleAttr(k) {
						return
					}
					if values == nil {
						values = map[string]string{}
					}
					values[k] = v
				})
				if len(values) == 0 {
					continue
				}
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
					rules = append(rules, svgTextRule{selector, values, rank})
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
	dec.Strict = false
	dec.CharsetReader = nil
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
			n := &svgTextNode{name: t.Name.Local}
			if len(t.Attr) > 0 {
				n.attrs = make(map[string]string, len(t.Attr))
				for _, a := range t.Attr {
					n.attrs[a.Name.Local] = a.Value
				}
			}
			parent.children = append(parent.children, n)
			stack = append(stack, n)
		case xml.CharData:
			parent.children = append(parent.children, &svgTextNode{text: string(t)})
		case xml.EndElement:
			for i := len(stack) - 1; i >= 1; i-- {
				if stack[i].name == t.Name.Local {
					stack = stack[:i]
					break
				}
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
		case "defs", "style", "script", "foreignObject", "title", "desc", "metadata", "clipPath", "mask", "symbol", "pattern", "marker":
			return
		}
		// 不在 text 内的非文本叶子元素（如大量 path/rect）不会产出文字，也无子层可继承。
		if !inside && n.name != "text" && len(n.children) == 0 {
			return
		}
		// 仅当节点实际带有会改变文字样式或可见性的属性/规则/内联样式时才写时复制 inherited。
		style := inherited
		if n.name != "" {
			copied := false
			setStyle := func(k, v string) {
				if !isSVGTextStyleAttr(k) {
					return
				}
				if cur, ok := style[k]; ok && cur == v {
					return
				}
				if !copied {
					next := make(map[string]string, len(inherited)+4)
					for ik, iv := range inherited {
						next[ik] = iv
					}
					style = next
					copied = true
				}
				style[k] = v
			}
			for k, v := range n.attrs {
				setStyle(k, v)
			}
			// CSS 的选择器样式优先于 presentation 属性，style 属性优先级最高。
			for _, rule := range rules {
				if svgSelectorMatches(rule.selector, n) {
					for k, v := range rule.values {
						setStyle(k, v)
					}
				}
			}
			if inline := n.attrs["style"]; inline != "" {
				visitSVGDeclarations(inline, setStyle)
			}
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
	walk(root, nil, identityAffine, false, &x, &y)
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
