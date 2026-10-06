package diagram

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// Style 是绘制用的字体与颜色，由调用方按主题传入。留空的颜色取浅色纸面的默认值。
type Style struct {
	Font      ui.Font  // 节点与连线标签的字体
	Text      ui.Color // 文字
	NodeFill  ui.Color
	NodeLine  ui.Color
	Edge      ui.Color
	LabelFill ui.Color // 连线标签的底色，盖住连线；也当作图表所在纸面的颜色
	GroupFill ui.Color // subgraph 底色
	GroupLine ui.Color
}

// withDefaults 补全留空的字号与颜色。
func (s Style) withDefaults() Style {
	if s.Font.Size <= 0 {
		s.Font.Size = 16
	}
	if s.Font.Family == "" {
		s.Font.Family = "system-ui, sans-serif"
	}
	fill := func(c *ui.Color, hex string) {
		if *c == (ui.Color{}) {
			*c = ui.Hex(hex)
		}
	}
	fill(&s.Text, "#47423c")
	fill(&s.LabelFill, "#faf9f5")
	fill(&s.NodeFill, "#f3e8dd")
	fill(&s.NodeLine, "#d6b19a")
	fill(&s.Edge, "#9b8b7b")
	fill(&s.GroupFill, "#f4f0e8")
	fill(&s.GroupLine, "#e0d6c8")
	return s
}

// Layout 是排好的图表。
type Layout struct {
	Width, Height float32

	graph *Graph
	geo   geometry
	style Style
	font  ui.Font
	bold  ui.Font
	k     float32
	lineH float32
	asc   float32
}

func textWidth(s string, font ui.Font) float32 {
	w := float32(0)
	for _, gl := range ui.Shape(s, font) {
		w = max(w, gl.X+gl.Advance)
	}
	return w
}

// Layout 按字体测量文字并完成布局。maxWidth>0 时，图比它宽就整体等比缩小。
func (g *Graph) Layout(style Style, maxWidth float32) *Layout {
	style = style.withDefaults()
	build := func(k float32) *Layout {
		font := style.Font
		font.Size = style.Font.Size * k
		bold := font
		bold.Weight = 600
		fm := font.Metrics()
		lineH := (fm.Ascent + fm.Descent) * 1.25
		m := metrics{k: k, lineH: lineH, avail: max(0, maxWidth),
			width: func(s string) float32 { return textWidth(s, font) },
			bold:  func(s string) float32 { return textWidth(s, bold) },
		}
		geo := arrange(g, m)
		return &Layout{Width: geo.width, Height: geo.height, graph: g, geo: geo, style: style, font: font, bold: bold, k: k, lineH: lineH, asc: fm.Ascent + (lineH-fm.Ascent-fm.Descent)/2}
	}
	l := build(1)
	// 文字宽度随字号近似线性变化，缩放后复核一两次即可落到限宽以内。
	for try := 0; try < 4 && maxWidth > 0 && l.Width > maxWidth && l.k > .3; try++ {
		l = build(max(.3, l.k*maxWidth/l.Width*.985))
	}
	return l
}

// Paint 把图画在以 (x, y) 为左上角的位置。
func (l *Layout) Paint(p *ui.Painter, x, y float32) {
	l.draw(&painterCanvas{p: p, l: l}, x, y)
}

// painterCanvas 把绘制指令画到 MyGo 的 Painter 上。
type painterCanvas struct {
	p *ui.Painter
	l *Layout
}

func (pc *painterCanvas) group(string) func() { return func() {} }

func (pc *painterCanvas) rect(x, y, w, h, radius float32, fill, stroke ui.Color, width float32, dashed bool) {
	r := ui.Rect{X: x, Y: y, W: w, H: h}
	if fill.A > 0 {
		pc.p.Fill(r, fill, radius)
	}
	if stroke.A == 0 || width <= 0 {
		return
	}
	if dashed {
		pc.p.StrokeDashed(r, stroke, radius, width)
	} else {
		pc.p.Stroke(r, stroke, radius, width)
	}
}

func uiPath(o outline) *ui.Path {
	path := new(ui.Path)
	for _, s := range o {
		switch s.op {
		case 'M':
			path.MoveTo(s.x, s.y)
		case 'L':
			path.LineTo(s.x, s.y)
		case 'C':
			path.CubeTo(s.x1, s.y1, s.x2, s.y2, s.x, s.y)
		case 'Z':
			path.Close()
		}
	}
	return path
}

func (pc *painterCanvas) path(o outline, fill, stroke ui.Color, width float32, dashed bool) {
	if len(o) == 0 {
		return
	}
	if fill.A > 0 {
		pc.p.FillPath(uiPath(o), fill)
	}
	if stroke.A == 0 || width <= 0 {
		return
	}
	if !dashed {
		pc.p.StrokePath(uiPath(o), width, stroke)
		return
	}
	// 虚线：沿折线的弧长交替落笔与抬笔。
	dash, space := dashLength*pc.l.k, dashSpace*pc.l.k
	for _, poly := range o.flatten() {
		path := new(ui.Path).MoveTo(poly[0][0], poly[0][1])
		prev := poly[0]
		left, drawing := dash, true
		for _, pt := range poly[1:] {
			for {
				seg := float32(math.Hypot(float64(pt[0]-prev[0]), float64(pt[1]-prev[1])))
				if seg <= left {
					left -= seg
					if drawing {
						path.LineTo(pt[0], pt[1])
					}
					prev = pt
					break
				}
				t := left / seg
				prev = [2]float32{prev[0] + (pt[0]-prev[0])*t, prev[1] + (pt[1]-prev[1])*t}
				if drawing {
					path.LineTo(prev[0], prev[1])
					left = space
				} else {
					path.MoveTo(prev[0], prev[1])
					left = dash
				}
				drawing = !drawing
			}
		}
		pc.p.StrokePath(path, width, stroke)
	}
}

// 虚线的线段与间隔长度（未乘缩放），SVG 用同一组数值。
const (
	dashLength = 4
	dashSpace  = 3.5
)

func (pc *painterCanvas) text(s string, x, baseline float32, anchor int8, c ui.Color, bold bool) {
	font := pc.l.font
	if bold {
		font = pc.l.bold
	}
	glyphs := ui.Shape(s, font)
	if anchor >= 0 {
		w := float32(0)
		for _, gl := range glyphs {
			w = max(w, gl.X+gl.Advance)
		}
		if anchor == 0 {
			x -= w / 2
		} else {
			x -= w
		}
	}
	pc.p.Glyphs(glyphs, x, baseline, c)
}
