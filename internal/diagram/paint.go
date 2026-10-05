package diagram

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// Style 是绘制用的字体与颜色，由调用方按主题传入。
type Style struct {
	Font      ui.Font  // 节点与连线标签的字体
	Text      ui.Color // 文字
	NodeFill  ui.Color
	NodeLine  ui.Color
	Edge      ui.Color
	LabelFill ui.Color // 连线标签的底色，盖住连线
	GroupFill ui.Color // subgraph 底色
	GroupLine ui.Color
}

// Layout 是排好的流程图。
type Layout struct {
	Width, Height float32

	geo   geometry
	style Style
	font  ui.Font
	k     float32
	lineH float32
	asc   float32
}

// Layout 按字体测量文字并完成布局。maxWidth>0 时，图比它宽就整体等比缩小。
func (g *Graph) Layout(style Style, maxWidth float32) *Layout {
	if style.Font.Size <= 0 {
		style.Font.Size = 16
	}
	build := func(k float32) *Layout {
		font := style.Font
		font.Size = style.Font.Size * k
		fm := font.Metrics()
		lineH := (fm.Ascent + fm.Descent) * 1.25
		m := metrics{k: k, lineH: lineH, width: func(s string) float32 {
			w := float32(0)
			for _, gl := range ui.Shape(s, font) {
				w = max(w, gl.X+gl.Advance)
			}
			return w
		}}
		geo := arrange(g, m)
		return &Layout{Width: geo.width, Height: geo.height, geo: geo, style: style, font: font, k: k, lineH: lineH, asc: fm.Ascent + (lineH-fm.Ascent-fm.Descent)/2}
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
	k, st := l.k, l.style
	line := max(1, 1.3*k)
	for _, f := range l.geo.frames {
		r := ui.Rect{X: x + f.x, Y: y + f.y, W: f.w, H: f.h}
		p.Fill(r, st.GroupFill, 8*k)
		p.Stroke(r, st.GroupLine, 8*k, 1)
		l.text(p, []string{f.title}, x+f.x+f.w/2, y+f.y+4*k+l.lineH/2, st.Text)
	}
	for i := range l.geo.routes {
		l.paintRoute(p, &l.geo.routes[i], x, y, line)
	}
	for i := range l.geo.nodes {
		n := &l.geo.nodes[i]
		l.paintNode(p, n, x+n.x, y+n.y, line)
		l.text(p, n.lines, x+n.x, y+n.y, st.Text)
	}
	// 标签最后画，底色盖住经过它的连线。
	for _, r := range l.geo.routes {
		if len(r.labelLines) == 0 {
			continue
		}
		p.Fill(ui.Rect{X: x + r.labelX - r.labelW/2, Y: y + r.labelY - r.labelH/2, W: r.labelW, H: r.labelH}, st.LabelFill, 4*k)
		l.text(p, r.labelLines, x+r.labelX, y+r.labelY, st.Text)
	}
}

// text 把若干行文字以 (cx, cy) 为中心居中绘制。
func (l *Layout) text(p *ui.Painter, lines []string, cx, cy float32, c ui.Color) {
	top := cy - float32(len(lines))*l.lineH/2
	for i, s := range lines {
		glyphs := ui.Shape(s, l.font)
		w := float32(0)
		for _, gl := range glyphs {
			w = max(w, gl.X+gl.Advance)
		}
		p.Glyphs(glyphs, cx-w/2, top+float32(i)*l.lineH+l.asc, c)
	}
}

func (l *Layout) paintNode(p *ui.Painter, n *placed, cx, cy, line float32) {
	k, st := l.k, l.style
	w, h := n.w, n.h
	left, top := cx-w/2, cy-h/2
	rect := ui.Rect{X: left, Y: top, W: w, H: h}
	poly := func(pts ...float32) {
		path := new(ui.Path).MoveTo(pts[0], pts[1])
		for i := 2; i < len(pts); i += 2 {
			path.LineTo(pts[i], pts[i+1])
		}
		path.Close()
		p.FillPath(path, st.NodeFill)
		p.StrokePath(path, line, st.NodeLine)
	}
	switch n.node.Shape {
	case ShapeRound:
		p.Fill(rect, st.NodeFill, 5*k)
		p.Stroke(rect, st.NodeLine, 5*k, 1)
	case ShapeStadium:
		p.Fill(rect, st.NodeFill, h/2)
		p.Stroke(rect, st.NodeLine, h/2, line)
	case ShapeCircle, ShapeDoubleCircle:
		circle := new(ui.Path).Circle(cx, cy, w/2)
		p.FillPath(circle, st.NodeFill)
		p.StrokePath(circle, line, st.NodeLine)
		if n.node.Shape == ShapeDoubleCircle {
			p.StrokePath(new(ui.Path).Circle(cx, cy, w/2-4*k), line, st.NodeLine)
		}
	case ShapeDiamond:
		poly(cx, top, left+w, cy, cx, top+h, left, cy)
	case ShapeHexagon:
		d := h / 4
		poly(left+d, top, left+w-d, top, left+w, cy, left+w-d, top+h, left+d, top+h, left, cy)
	case ShapeParallelogram:
		d := h * .3
		poly(left+d, top, left+w, top, left+w-d, top+h, left, top+h)
	case ShapeParallelogramAlt:
		d := h * .3
		poly(left, top, left+w-d, top, left+w, top+h, left+d, top+h)
	case ShapeTrapezoid:
		d := h * .3
		poly(left+d, top, left+w-d, top, left+w, top+h, left, top+h)
	case ShapeTrapezoidAlt:
		d := h * .3
		poly(left, top, left+w, top, left+w-d, top+h, left+d, top+h)
	case ShapeFlag:
		d := h * .35
		poly(left, top, left+w, top, left+w, top+h, left, top+h, left+d, cy)
	case ShapeCylinder:
		// 圆柱：上下各一段椭圆弧，中间是柱身。
		ry := 6 * k
		body := new(ui.Path).MoveTo(left, top+ry)
		body.CubeTo(left, top-ry/3, left+w, top-ry/3, left+w, top+ry)
		body.LineTo(left+w, top+h-ry)
		body.CubeTo(left+w, top+h+ry/3, left, top+h+ry/3, left, top+h-ry)
		body.Close()
		p.FillPath(body, st.NodeFill)
		p.StrokePath(body, line, st.NodeLine)
		rim := new(ui.Path).MoveTo(left, top+ry)
		rim.CubeTo(left, top+ry*2.3, left+w, top+ry*2.3, left+w, top+ry)
		p.StrokePath(rim, line, st.NodeLine)
	case ShapeSubroutine:
		p.Fill(rect, st.NodeFill, 2*k)
		p.Stroke(rect, st.NodeLine, 2*k, line)
		for _, bx := range []float32{left + 8*k, left + w - 8*k} {
			p.StrokePath(new(ui.Path).MoveTo(bx, top).LineTo(bx, top+h), line, st.NodeLine)
		}
	default:
		p.Fill(rect, st.NodeFill, 0)
		p.Stroke(rect, st.NodeLine, 0, 1)
	}
}

// paintRoute 画一条连线：相邻锚点之间先沿层方向走到中线，圆角拐弯后再走向下一个锚点。
func (l *Layout) paintRoute(p *ui.Painter, r *route, ox, oy, line float32) {
	if r.edge.Line == LineInvisible || len(r.pts) < 2 {
		return
	}
	k, c := l.k, l.style.Edge
	width := line
	if r.edge.Line == LineThick {
		width = line * 2.2
	}
	pts := make([][2]float32, len(r.pts))
	for i, pt := range r.pts {
		pts[i] = [2]float32{pt[0] + ox, pt[1] + oy}
	}
	// poly 是整条连线细分后的折线，实线与虚线都从它画出。
	var poly [][2]float32
	if r.loop {
		cv := [4][2]float32{pts[0], pts[1], pts[2], pts[3]}
		for step := 0; step <= 32; step++ {
			poly = append(poly, cubicAt(cv, float32(step)/32))
		}
		if r.edge.Head != ArrowNone {
			l.marker(p, r.edge.Head, [2]float32{pts[3][0] + 9*k, pts[3][1] + 4*k}, pts[3], c, width)
		}
	} else {
		// 箭头占掉末端一小段，线条停在箭头根部。
		trim := func(at, toward [2]float32, mark Arrow) [2]float32 {
			if mark == ArrowNone {
				return at
			}
			d := 9 * k
			if l.geo.horizontal {
				return [2]float32{at[0] + sign(toward[0]-at[0])*d, at[1]}
			}
			return [2]float32{at[0], at[1] + sign(toward[1]-at[1])*d}
		}
		last := len(pts) - 1
		tipTail, tipHead := pts[0], pts[last]
		pts[0] = trim(pts[0], pts[1], r.edge.Tail)
		pts[last] = trim(pts[last], pts[last-1], r.edge.Head)
		// main 是层方向的坐标轴，cross 是层内方向。
		main, cross := 1, 0
		if l.geo.horizontal {
			main, cross = 0, 1
		}
		poly = append(poly, pts[0])
		for i := 0; i+1 < len(pts); i++ {
			a, b := pts[i], pts[i+1]
			dm, dc := b[main]-a[main], b[cross]-a[cross]
			if abs32(dc) < .5 {
				poly = append(poly, b)
				continue
			}
			mid := (a[main] + b[main]) / 2
			rad := min(5*k, abs32(dc)/2, abs32(dm)/2)
			at := func(m, c float32) [2]float32 {
				var pt [2]float32
				pt[main], pt[cross] = m, c
				return pt
			}
			corner := func(from, ctrl, to [2]float32) {
				for step := 1; step <= 6; step++ {
					t := float32(step) / 6
					u := 1 - t
					poly = append(poly, [2]float32{u*u*from[0] + 2*u*t*ctrl[0] + t*t*to[0], u*u*from[1] + 2*u*t*ctrl[1] + t*t*to[1]})
				}
			}
			in := at(mid-sign(dm)*rad, a[cross])
			poly = append(poly, in)
			corner(in, at(mid, a[cross]), at(mid, a[cross]+sign(dc)*rad))
			out := at(mid, b[cross]-sign(dc)*rad)
			poly = append(poly, out)
			corner(out, at(mid, b[cross]), at(mid+sign(dm)*rad, b[cross]))
			poly = append(poly, b)
		}
		l.marker(p, r.edge.Tail, pts[0], tipTail, c, width)
		l.marker(p, r.edge.Head, pts[last], tipHead, c, width)
	}
	path := new(ui.Path).MoveTo(poly[0][0], poly[0][1])
	if r.edge.Line != LineDotted {
		for _, pt := range poly[1:] {
			path.LineTo(pt[0], pt[1])
		}
		p.StrokePath(path, width, c)
		return
	}
	// 虚线：沿折线的弧长交替落笔与抬笔。
	dash, space := 4*k, 3.5*k
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
	p.StrokePath(path, width, c)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// marker 在连线端点画箭头、圆点或叉，from 是根部，tip 是贴着节点的尖端。
func (l *Layout) marker(p *ui.Painter, mark Arrow, from, tip [2]float32, c ui.Color, width float32) {
	k := l.k
	dx, dy := tip[0]-from[0], tip[1]-from[1]
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if mark == ArrowNone || length == 0 {
		return
	}
	ux, uy := dx/length, dy/length
	nx, ny := -uy, ux
	switch mark {
	case ArrowNormal:
		half := 5 * k
		head := new(ui.Path).MoveTo(tip[0], tip[1]).LineTo(from[0]+nx*half, from[1]+ny*half).LineTo(from[0]-nx*half, from[1]-ny*half).Close()
		p.FillPath(head, c)
	case ArrowCircle:
		p.FillPath(new(ui.Path).Circle((from[0]+tip[0])/2, (from[1]+tip[1])/2, length/2), c)
	case ArrowCross:
		cx, cy, half := (from[0]+tip[0])/2, (from[1]+tip[1])/2, 3.2*k
		for _, s := range []float32{1, -1} {
			ax, ay := (ux+nx*s)*half, (uy+ny*s)*half
			p.StrokePath(new(ui.Path).MoveTo(cx-ax, cy-ay).LineTo(cx+ax, cy+ay), width, c)
		}
	}
}

func cubicAt(cv [4][2]float32, t float32) [2]float32 {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return [2]float32{a*cv[0][0] + b*cv[1][0] + c*cv[2][0] + d*cv[3][0], a*cv[0][1] + b*cv[1][1] + c*cv[2][1] + d*cv[3][1]}
}

func sign(v float32) float32 {
	if v < 0 {
		return -1
	}
	return 1
}
