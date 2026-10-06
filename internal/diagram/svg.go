package diagram

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// SVG 把排好的图写成自包含的 SVG：只有矩形、路径与文字，不含脚本、样式表或外部资源，
// 坐标与配色和 Paint 相同。文字用 <text> 输出，字体沿用 Style.Font 的字体族。
func (l *Layout) SVG() string {
	if l == nil {
		return ""
	}
	sc := &svgCanvas{k: l.k}
	b := &sc.b
	w, h := num(max(1, l.Width)), num(max(1, l.Height))
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" role="img" width="` + w + `" height="` + h + `" viewBox="0 0 ` + w + ` ` + h + `"`)
	b.WriteString(` font-family="` + escape(l.font.Family) + `" font-size="` + num(l.font.Size) + `"`)
	if weight := l.font.Weight; weight > 0 && weight != 400 {
		b.WriteString(` font-weight="` + strconv.Itoa(weight) + `"`)
	}
	b.WriteString(` style="max-width:100%;height:auto">`)
	if l.graph != nil && l.graph.Title != "" {
		b.WriteString(`<title>` + escape(l.graph.Title) + `</title>`)
	}
	l.draw(sc, 0, 0)
	b.WriteString(`</svg>`)
	return b.String()
}

// SVG 按给定样式与限宽布局后输出 SVG，等价于 g.Layout(style, maxWidth).SVG()。
func (g *Graph) SVG(style Style, maxWidth float32) string {
	if g == nil {
		return ""
	}
	return g.Layout(style, maxWidth).SVG()
}

// svgCanvas 把绘制指令写成 SVG 标记。
type svgCanvas struct {
	b strings.Builder
	k float32
}

// num 把坐标写成最多两位小数，去掉多余的零。
func num(v float32) string {
	if v != v || v > 1e7 || v < -1e7 {
		return "0"
	}
	s := strconv.FormatFloat(float64(v), 'f', 2, 32)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

// escape 转义文字并去掉 XML 不允许的控制字符与无效编码。
func escape(s string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20, r == 0x7f, r == utf8.RuneError, r == 0xfffe, r == 0xffff:
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	return html.EscapeString(clean)
}

func (sc *svgCanvas) paint(name string, c ui.Color) {
	b := &sc.b
	if c.A == 0 {
		b.WriteString(` ` + name + `="none"`)
		return
	}
	const hex = "0123456789abcdef"
	b.WriteString(` ` + name + `="#`)
	for _, v := range [3]uint8{c.R, c.G, c.B} {
		b.WriteByte(hex[v>>4])
		b.WriteByte(hex[v&15])
	}
	b.WriteByte('"')
	if c.A < 255 {
		b.WriteString(` ` + name + `-opacity="` + num(float32(c.A)/255) + `"`)
	}
}

func (sc *svgCanvas) strokeAttrs(stroke ui.Color, width float32, dashed bool) {
	if stroke.A == 0 || width <= 0 {
		return
	}
	sc.paint("stroke", stroke)
	sc.b.WriteString(` stroke-width="` + num(width) + `"`)
	if dashed {
		sc.b.WriteString(` stroke-dasharray="` + num(dashLength*sc.k) + ` ` + num(dashSpace*sc.k) + `"`)
	}
}

func (sc *svgCanvas) group(tooltip string) func() {
	sc.b.WriteString(`<g><title>` + escape(tooltip) + `</title>`)
	return func() { sc.b.WriteString(`</g>`) }
}

func (sc *svgCanvas) rect(x, y, w, h, radius float32, fill, stroke ui.Color, width float32, dashed bool) {
	if w <= 0 || h <= 0 || (fill.A == 0 && (stroke.A == 0 || width <= 0)) {
		return
	}
	b := &sc.b
	b.WriteString(`<rect x="` + num(x) + `" y="` + num(y) + `" width="` + num(w) + `" height="` + num(h) + `"`)
	if radius > 0 {
		b.WriteString(` rx="` + num(min(radius, w/2, h/2)) + `"`)
	}
	sc.paint("fill", fill)
	sc.strokeAttrs(stroke, width, dashed)
	b.WriteString(`/>`)
}

func (sc *svgCanvas) path(o outline, fill, stroke ui.Color, width float32, dashed bool) {
	if len(o) == 0 || (fill.A == 0 && (stroke.A == 0 || width <= 0)) {
		return
	}
	b := &sc.b
	b.WriteString(`<path d="`)
	for i, s := range o {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch s.op {
		case 'M', 'L':
			b.WriteByte(s.op)
			b.WriteString(num(s.x) + ` ` + num(s.y))
		case 'C':
			b.WriteString(`C` + num(s.x1) + ` ` + num(s.y1) + ` ` + num(s.x2) + ` ` + num(s.y2) + ` ` + num(s.x) + ` ` + num(s.y))
		case 'Z':
			b.WriteByte('Z')
		}
	}
	b.WriteByte('"')
	sc.paint("fill", fill)
	sc.strokeAttrs(stroke, width, dashed)
	if stroke.A > 0 && width > 0 {
		b.WriteString(` stroke-linejoin="round"`)
	}
	b.WriteString(`/>`)
}

func (sc *svgCanvas) text(s string, x, baseline float32, anchor int8, c ui.Color, bold bool) {
	b := &sc.b
	b.WriteString(`<text x="` + num(x) + `" y="` + num(baseline) + `"`)
	switch {
	case anchor == 0:
		b.WriteString(` text-anchor="middle"`)
	case anchor > 0:
		b.WriteString(` text-anchor="end"`)
	}
	if bold {
		b.WriteString(` font-weight="600"`)
	}
	sc.paint("fill", c)
	b.WriteString(` xml:space="preserve">` + escape(s) + `</text>`)
}
