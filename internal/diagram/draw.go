package diagram

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// canvas 是绘制后端：Paint 画到 MyGo 的 Painter，SVG 写成矢量标记。
// 颜色的 A 为 0 表示不填充或不描边。
type canvas interface {
	rect(x, y, w, h, radius float32, fill, stroke ui.Color, width float32, dashed bool)
	path(o outline, fill, stroke ui.Color, width float32, dashed bool)
	// text 画一行文字，anchor 为 -1、0、1 时 x 分别是左端、中点、右端。
	text(s string, x, baseline float32, anchor int8, c ui.Color, bold bool)
	// group 把随后的内容归成一组并附上提示文字，返回的函数结束这一组。
	group(tooltip string) func()
}

// seg 是轮廓的一段：M 移动、L 直线、C 三次曲线、Z 闭合。
type seg struct {
	op                   byte
	x, y, x1, y1, x2, y2 float32
}

// outline 是与设备无关的矢量轮廓。
type outline []seg

func (o *outline) move(x, y float32) *outline { *o = append(*o, seg{op: 'M', x: x, y: y}); return o }
func (o *outline) line(x, y float32) *outline { *o = append(*o, seg{op: 'L', x: x, y: y}); return o }
func (o *outline) cube(x1, y1, x2, y2, x, y float32) *outline {
	*o = append(*o, seg{op: 'C', x: x, y: y, x1: x1, y1: y1, x2: x2, y2: y2})
	return o
}
func (o *outline) close() *outline { *o = append(*o, seg{op: 'Z'}); return o }

// polygon 用成对的坐标组成闭合多边形。
func polygon(pts ...float32) outline {
	var o outline
	o.move(pts[0], pts[1])
	for i := 2; i+1 < len(pts); i += 2 {
		o.line(pts[i], pts[i+1])
	}
	o.close()
	return o
}

func polyline(pts [][2]float32) outline {
	var o outline
	for i, p := range pts {
		if i == 0 {
			o.move(p[0], p[1])
		} else {
			o.line(p[0], p[1])
		}
	}
	return o
}

// arcTo 从当前点沿圆心 (cx, cy)、半径 r 的圆弧走到角度 to（弧度），每段不超过四分之一圆。
func (o *outline) arcTo(cx, cy, r float32, from, to float64) *outline {
	steps := int(math.Ceil(math.Abs(to-from) / (math.Pi / 2)))
	if steps < 1 {
		steps = 1
	}
	step := (to - from) / float64(steps)
	t := float32(4.0 / 3 * math.Tan(step/4))
	for i := 0; i < steps; i++ {
		a, b := from+float64(i)*step, from+float64(i+1)*step
		ca, sa, cb, sb := float32(math.Cos(a)), float32(math.Sin(a)), float32(math.Cos(b)), float32(math.Sin(b))
		o.cube(cx+r*(ca-t*sa), cy+r*(sa+t*ca), cx+r*(cb+t*sb), cy+r*(sb-t*cb), cx+r*cb, cy+r*sb)
	}
	return o
}

func circle(cx, cy, r float32) outline {
	var o outline
	o.move(cx+r, cy)
	o.arcTo(cx, cy, r, 0, 2*math.Pi)
	o.close()
	return o
}

// roundRect 是四角半径各自指定的矩形，依次为左上、右上、右下、左下。
func roundRect(x, y, w, h, tl, tr, br, bl float32) outline {
	var o outline
	o.move(x+tl, y)
	o.line(x+w-tr, y)
	if tr > 0 {
		o.arcTo(x+w-tr, y+tr, tr, -math.Pi/2, 0)
	}
	o.line(x+w, y+h-br)
	if br > 0 {
		o.arcTo(x+w-br, y+h-br, br, 0, math.Pi/2)
	}
	o.line(x+bl, y+h)
	if bl > 0 {
		o.arcTo(x+bl, y+h-bl, bl, math.Pi/2, math.Pi)
	}
	o.line(x, y+tl)
	if tl > 0 {
		o.arcTo(x+tl, y+tl, tl, math.Pi, 3*math.Pi/2)
	}
	o.close()
	return o
}

func (o outline) shifted(dx, dy float32) outline {
	out := make(outline, len(o))
	for i, s := range o {
		s.x, s.y, s.x1, s.y1, s.x2, s.y2 = s.x+dx, s.y+dy, s.x1+dx, s.y1+dy, s.x2+dx, s.y2+dy
		out[i] = s
	}
	return out
}

// flatten 把轮廓细分成若干条折线，供后端自行画虚线，也用于测试里核对范围。
func (o outline) flatten() [][][2]float32 {
	var out [][][2]float32
	var cur [][2]float32
	for _, s := range o {
		switch s.op {
		case 'M':
			if len(cur) > 1 {
				out = append(out, cur)
			}
			cur = [][2]float32{{s.x, s.y}}
		case 'L':
			cur = append(cur, [2]float32{s.x, s.y})
		case 'C':
			if len(cur) == 0 {
				continue
			}
			cv := [4][2]float32{cur[len(cur)-1], {s.x1, s.y1}, {s.x2, s.y2}, {s.x, s.y}}
			for step := 1; step <= 12; step++ {
				cur = append(cur, cubicAt(cv, float32(step)/12))
			}
		case 'Z':
			if len(cur) > 0 {
				cur = append(cur, cur[0])
			}
		}
	}
	if len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

// inkRole 是绘制指令引用的颜色角色，绘制时按 Style 解析，布局因此不依赖具体配色。
type inkRole uint8

const (
	inkNone inkRole = iota
	inkText
	inkMuted // 次要文字
	inkNodeFill
	inkNodeLine
	inkEdge
	inkLabelFill
	inkGroupFill
	inkGroupLine
	inkNoteFill
	inkNoteLine
	inkActiveFill // 甘特图进行中的任务
	inkDoneFill   // 甘特图已完成的任务
	inkCritFill   // 甘特图关键任务
	inkCritLine
	inkSeries // 饼图的分类色，n 是序号
	inkCSS    // 源码里写明的颜色
)

type ink struct {
	role inkRole
	n    int
	css  string
}

// 分类色按固定顺序取用，浅色与深色纸面各一组；超出八类的部分用中性灰。
var (
	seriesLight = [...]string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#4a3aa7", "#e34948"}
	seriesDark  = [...]string{"#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#008300", "#9085e9", "#e66767"}
)

func luminance(c ui.Color) float32 {
	return (.2126*float32(c.R) + .7152*float32(c.G) + .0722*float32(c.B)) / 255
}

// paper 是图表所在纸面的颜色，空心标记用它填充。
func (l *Layout) paper() ui.Color {
	if l.style.LabelFill.A > 0 {
		return l.style.LabelFill
	}
	return ui.RGB(255, 255, 255)
}

func (l *Layout) dark() bool { return luminance(l.paper()) < .4 }

func (l *Layout) color(i ink) ui.Color {
	st := l.style
	switch i.role {
	case inkText:
		return st.Text
	case inkMuted:
		return st.Text.Mix(l.paper(), .38)
	case inkNodeFill:
		return st.NodeFill
	case inkNodeLine:
		return st.NodeLine
	case inkEdge:
		return st.Edge
	case inkLabelFill:
		return l.paper()
	case inkGroupFill:
		return st.GroupFill
	case inkGroupLine:
		return st.GroupLine
	case inkNoteFill:
		return l.paper().Mix(ui.Hex("#e9c46a"), .3)
	case inkNoteLine:
		return l.paper().Mix(ui.Hex("#c9a23f"), .75)
	case inkActiveFill:
		return st.NodeFill.Mix(st.NodeLine, .5)
	case inkDoneFill:
		return st.GroupFill
	case inkCritFill:
		return l.paper().Mix(l.color(ink{role: inkCritLine}), .24)
	case inkCritLine:
		if l.dark() {
			return ui.Hex("#e66767")
		}
		return ui.Hex("#d03b3b")
	case inkSeries:
		if i.n < len(seriesLight) {
			if l.dark() {
				return ui.Hex(seriesDark[i.n])
			}
			return ui.Hex(seriesLight[i.n])
		}
		return st.Edge.Mix(l.paper(), .25+.3*float32(i.n%2))
	case inkCSS:
		c, _ := cssColor(i.css)
		return c
	}
	return ui.Color{}
}

type opKind uint8

const (
	opRect opKind = iota
	opPath
	opText
)

// op 是一条绘制指令。时序图、甘特图与饼图的布局直接产出指令，
// 其余图种的节点、连线与外框在 draw 里按统一规则画出。
type op struct {
	kind         opKind
	x, y, w, h   float32 // 矩形的左上角与尺寸；文字的锚点与文字块的竖直中心
	r            float32 // 矩形圆角
	path         outline
	fill, stroke ink     // 文字颜色用 fill
	width        float32 // 线宽，0 表示默认线宽
	dashed       bool
	text         string // 可含换行
	anchor       int8
	bold         bool
}

// draw 把整张图画到后端上，(ox, oy) 是图的左上角。
func (l *Layout) draw(c canvas, ox, oy float32) {
	k, st := l.k, l.style
	line := max(1, 1.3*k)
	for _, f := range l.geo.frames {
		c.rect(ox+f.x, oy+f.y, f.w, f.h, 8*k, st.GroupFill, st.GroupLine, 1, false)
		l.block(c, []string{f.title}, ox+f.x+f.w/2, oy+f.y+4*k+l.lineH/2, 0, st.Text, false)
	}
	for i := range l.geo.routes {
		l.drawRoute(c, &l.geo.routes[i], ox, oy, line)
	}
	for i := range l.geo.nodes {
		n := &l.geo.nodes[i]
		end := func() {}
		if n.node.Tooltip != "" {
			end = c.group(n.node.Tooltip)
		}
		l.drawNode(c, n, ox+n.x, oy+n.y, line)
		end()
	}
	// 标签最后画，底色盖住经过它的连线。
	for _, r := range l.geo.routes {
		if len(r.labelLines) == 0 || r.edge.Line == LineInvisible {
			continue
		}
		c.rect(ox+r.labelX-r.labelW/2, oy+r.labelY-r.labelH/2, r.labelW, r.labelH, 4*k, st.LabelFill, ui.Color{}, 0, false)
		l.block(c, r.labelLines, ox+r.labelX, oy+r.labelY, 0, st.Text, false)
	}
	for _, o := range l.geo.ops {
		width := o.width
		if width == 0 {
			width = line
		}
		switch o.kind {
		case opRect:
			c.rect(ox+o.x, oy+o.y, o.w, o.h, o.r, l.color(o.fill), l.color(o.stroke), width, o.dashed)
		case opPath:
			c.path(o.path.shifted(ox, oy), l.color(o.fill), l.color(o.stroke), width, o.dashed)
		case opText:
			l.block(c, splitLines(o.text), ox+o.x, oy+o.y, o.anchor, l.color(o.fill), o.bold)
		}
	}
}

// block 把若干行文字以 cy 为竖直中心画出，x 的含义由 anchor 决定。
func (l *Layout) block(c canvas, lines []string, x, cy float32, anchor int8, col ui.Color, bold bool) {
	top := cy - float32(len(lines))*l.lineH/2
	for i, s := range lines {
		if s != "" {
			c.text(s, x, top+float32(i)*l.lineH+l.asc, anchor, col, bold)
		}
	}
}

func (l *Layout) drawNode(c canvas, n *placed, cx, cy, line float32) {
	k, st := l.k, l.style
	w, h := n.w, n.h
	left, top := cx-w/2, cy-h/2
	fill, stroke, ink := st.NodeFill, st.NodeLine, st.Text
	thin, dashed := float32(1), false
	if attr := l.graph.nodeAttributes(n.node); attr != (Attributes{}) {
		if col, ok := cssColor(attr.Fill); ok {
			fill = col
		}
		if col, ok := cssColor(attr.Stroke); ok {
			stroke = col
		}
		if col, ok := cssColor(attr.Color); ok && col.A > 0 {
			ink = col
		}
		if attr.StrokeWidth > 0 {
			line = attr.StrokeWidth * k
			thin = line
		}
		dashed = attr.Dashed
	}
	shape := func(o outline) { c.path(o, fill, stroke, line, dashed) }
	switch n.node.Shape {
	case ShapeRound:
		c.rect(left, top, w, h, 5*k, fill, stroke, thin, dashed)
	case ShapeState:
		c.rect(left, top, w, h, 7*k, fill, stroke, line, dashed)
	case ShapeStadium:
		c.rect(left, top, w, h, h/2, fill, stroke, line, dashed)
	case ShapeCircle, ShapeDoubleCircle:
		shape(circle(cx, cy, w/2))
		if n.node.Shape == ShapeDoubleCircle {
			c.path(circle(cx, cy, w/2-4*k), ui.Color{}, stroke, line, dashed)
		}
	case ShapeDiamond:
		shape(polygon(cx, top, left+w, cy, cx, top+h, left, cy))
	case ShapeHexagon:
		d := h / 4
		shape(polygon(left+d, top, left+w-d, top, left+w, cy, left+w-d, top+h, left+d, top+h, left, cy))
	case ShapeParallelogram:
		d := h * .3
		shape(polygon(left+d, top, left+w, top, left+w-d, top+h, left, top+h))
	case ShapeParallelogramAlt:
		d := h * .3
		shape(polygon(left, top, left+w-d, top, left+w, top+h, left+d, top+h))
	case ShapeTrapezoid:
		d := h * .3
		shape(polygon(left+d, top, left+w-d, top, left+w, top+h, left, top+h))
	case ShapeTrapezoidAlt:
		d := h * .3
		shape(polygon(left, top, left+w, top, left+w-d, top+h, left+d, top+h))
	case ShapeFlag:
		d := h * .35
		shape(polygon(left, top, left+w, top, left+w, top+h, left, top+h, left+d, cy))
	case ShapeCylinder:
		// 圆柱：上下各一段椭圆弧，中间是柱身。
		ry := 6 * k
		var body outline
		body.move(left, top+ry)
		body.cube(left, top-ry/3, left+w, top-ry/3, left+w, top+ry)
		body.line(left+w, top+h-ry)
		body.cube(left+w, top+h+ry/3, left, top+h+ry/3, left, top+h-ry)
		body.close()
		shape(body)
		var rim outline
		rim.move(left, top+ry)
		rim.cube(left, top+ry*2.3, left+w, top+ry*2.3, left+w, top+ry)
		c.path(rim, ui.Color{}, stroke, line, dashed)
	case ShapeSubroutine:
		c.rect(left, top, w, h, 2*k, fill, stroke, line, dashed)
		for _, bx := range []float32{left + 8*k, left + w - 8*k} {
			c.path(polyline([][2]float32{{bx, top}, {bx, top + h}}), ui.Color{}, stroke, line, false)
		}
	case ShapeNote:
		noteFill, noteLine := l.color(inkOf(inkNoteFill)), l.color(inkOf(inkNoteLine))
		if n.node.Style.Fill != "" || len(n.node.Classes) > 0 {
			noteFill, noteLine = fill, stroke
		}
		c.rect(left, top, w, h, 2*k, noteFill, noteLine, thin, dashed)
	case ShapeStart:
		c.path(circle(cx, cy, w/2), st.Edge, ui.Color{}, 0, false)
		return
	case ShapeEnd:
		c.path(circle(cx, cy, w/2-line/2), l.paper(), st.Edge, line, false)
		c.path(circle(cx, cy, w/2-4.5*k), st.Edge, ui.Color{}, 0, false)
		return
	case ShapeBar:
		c.rect(left, top, w, h, 2*k, st.Edge, ui.Color{}, 0, false)
		return
	case ShapeClass:
		l.drawClass(c, n, left, top, fill, stroke, ink, line, dashed)
		return
	case ShapeEntity:
		l.drawEntity(c, n, left, top, fill, stroke, ink, line, dashed)
		return
	default:
		c.rect(left, top, w, h, 0, fill, stroke, thin, dashed)
	}
	l.block(c, n.lines, cx, cy, 0, ink, false)
}

func inkOf(role inkRole) ink { return ink{role: role} }

// drawClass 画类图的类：名称栏居中，属性栏与方法栏左对齐，栏与栏之间一条横线。
func (l *Layout) drawClass(c canvas, n *placed, left, top float32, fill, stroke, ink ui.Color, line float32, dashed bool) {
	k := l.k
	c.rect(left, top, n.w, n.h, 3*k, fill, stroke, line, dashed)
	cx := left + n.w/2
	headTop := top + (n.headH-float32(len(n.lines))*l.lineH)/2
	for i, s := range n.lines {
		cy := headTop + (float32(i)+.5)*l.lineH
		if i == 0 && n.node.annotation != "" {
			l.block(c, []string{s}, cx, cy, 0, l.color(inkOf(inkMuted)), false)
			continue
		}
		l.block(c, []string{s}, cx, cy, 0, ink, true)
	}
	y := top + n.headH
	for i, sec := range n.secs {
		if n.secH[i] == 0 {
			continue
		}
		c.path(polyline([][2]float32{{left, y}, {left + n.w, y}}), ui.Color{}, stroke, line, false)
		for j, s := range sec {
			l.block(c, []string{s}, left+12*k, y+5*k+(float32(j)+.5)*l.lineH, -1, ink, false)
		}
		y += n.secH[i]
	}
}

// drawEntity 画 ER 图的实体：名称栏用节点底色，属性行用纸色，各列左对齐。
func (l *Layout) drawEntity(c canvas, n *placed, left, top float32, fill, stroke, ink ui.Color, line float32, dashed bool) {
	k := l.k
	radius := 3 * k
	c.rect(left, top, n.w, n.h, radius, fill, ui.Color{}, 0, false)
	rows := n.node.rows
	if len(rows) > 0 {
		c.path(roundRect(left, top+n.headH, n.w, n.h-n.headH, 0, 0, radius, radius), l.paper(), ui.Color{}, 0, false)
		rowH := (n.h - n.headH) / float32(len(rows))
		muted := l.color(inkOf(inkMuted))
		for i, row := range rows {
			y := top + n.headH + float32(i)*rowH
			if i > 0 {
				c.path(polyline([][2]float32{{left, y}, {left + n.w, y}}), ui.Color{}, l.style.GroupLine, 1, false)
			}
			x := left + 12*k
			for j, cell := range row {
				col := ink
				if j == 0 || j == 3 {
					col = muted
				}
				if cell != "" {
					l.block(c, []string{cell}, x, y+rowH/2, -1, col, j == 2)
				}
				if j < len(n.cols) {
					x += n.cols[j] + 14*k
				}
			}
		}
		c.path(polyline([][2]float32{{left, top + n.headH}, {left + n.w, top + n.headH}}), ui.Color{}, stroke, line, false)
	}
	c.rect(left, top, n.w, n.h, radius, ui.Color{}, stroke, line, dashed)
	l.block(c, n.lines, left+n.w/2, top+n.headH/2, 0, ink, true)
}

// markLength 是端点标记占掉的线长（未乘缩放），线条停在标记根部。
func markLength(mark Arrow) float32 {
	switch mark {
	case ArrowNormal, ArrowCircle, ArrowCross:
		return 9
	case ArrowTriangle:
		return 12
	case ArrowDiamond, ArrowDiamondFilled:
		return 16
	}
	return 0
}

// drawRoute 画一条连线：相邻锚点之间先沿层方向走到中线，圆角拐弯后再走向下一个锚点。
func (l *Layout) drawRoute(c canvas, r *route, ox, oy, line float32) {
	if r.edge.Line == LineInvisible || len(r.pts) < 2 {
		return
	}
	k, col := l.k, l.style.Edge
	width := line
	if r.edge.Line == LineThick {
		width = line * 2.2
	}
	dashed := r.edge.Line == LineDotted
	if attr := r.edge.Style; attr != (Attributes{}) {
		if cc, ok := cssColor(attr.Stroke); ok && cc.A > 0 {
			col = cc
		}
		if attr.StrokeWidth > 0 {
			width = attr.StrokeWidth * k
		}
		dashed = dashed || attr.Dashed
	}
	pts := make([][2]float32, len(r.pts))
	for i, pt := range r.pts {
		pts[i] = [2]float32{pt[0] + ox, pt[1] + oy}
	}
	// main 是层方向的坐标轴，cross 是层内方向。
	main, cross := 1, 0
	if r.horizontal {
		main, cross = 0, 1
	}
	// poly 是整条连线细分后的折线。
	var poly [][2]float32
	type end struct {
		mark   Arrow
		tip    [2]float32
		ux, uy float32
		text   string
	}
	var ends [2]end
	if r.loop {
		cv := [4][2]float32{pts[0], pts[1], pts[2], pts[3]}
		for step := 0; step <= 32; step++ {
			poly = append(poly, cubicAt(cv, float32(step)/32))
		}
		length := float32(math.Hypot(9, 4))
		ends[1] = end{mark: r.edge.Head, tip: pts[3], ux: -9 / length, uy: -4 / length}
	} else {
		last := len(pts) - 1
		// 标记占掉末端一小段，线条停在标记根部。
		trim := func(i, toward int, mark Arrow, text string) end {
			var u [2]float32
			u[main] = -sign(pts[toward][main] - pts[i][main])
			e := end{mark: mark, tip: pts[i], ux: u[0], uy: u[1], text: text}
			d := markLength(mark) * k
			pts[i][main] -= u[main] * d
			return e
		}
		ends[0] = trim(0, 1, r.edge.Tail, r.edge.CardinalityFrom)
		ends[1] = trim(last, last-1, r.edge.Head, r.edge.CardinalityTo)
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
	}
	c.path(polyline(poly), ui.Color{}, col, width, dashed)
	for _, e := range ends {
		l.endMark(c, e.mark, e.tip, e.ux, e.uy, col, max(1, width))
		if e.text != "" {
			// 基数贴在标记根部外侧：竖线时偏到右侧，横线时偏到上方，避免和标记重叠。
			back := (markLength(e.mark) + 8) * k
			x, y := e.tip[0]-e.ux*back, e.tip[1]-e.uy*back
			if abs32(e.uy) > abs32(e.ux) {
				x += 16 * k
			} else {
				y -= 14 * k
			}
			l.block(c, []string{e.text}, x, y, 0, l.style.Text, false)
		}
	}
}

// endMark 在连线端点画标记：tip 贴着节点，(ux, uy) 是指向节点的单位方向。
func (l *Layout) endMark(c canvas, mark Arrow, tip [2]float32, ux, uy float32, col ui.Color, width float32) {
	if mark == ArrowNone {
		return
	}
	k := l.k
	// at 取离端点 back、偏向一侧 side 的点。
	at := func(back, side float32) [2]float32 {
		return [2]float32{tip[0] - ux*back*k - uy*side*k, tip[1] - uy*back*k + ux*side*k}
	}
	poly := func(pts ...[2]float32) outline {
		o := polyline(pts)
		o.close()
		return o
	}
	stroke := func(pts ...[2]float32) { c.path(polyline(pts), ui.Color{}, col, width, false) }
	none := ui.Color{}
	switch mark {
	case ArrowNormal:
		c.path(poly(at(0, 0), at(9, 5), at(9, -5)), col, none, 0, false)
	case ArrowCircle:
		center := at(4.5, 0)
		c.path(circle(center[0], center[1], 4.5*k), col, none, 0, false)
	case ArrowCross:
		stroke(at(1.3, -3.2), at(7.7, 3.2))
		stroke(at(1.3, 3.2), at(7.7, -3.2))
	case ArrowOpen:
		stroke(at(9, 5), at(0, 0), at(9, -5))
	case ArrowTriangle:
		c.path(poly(at(.5, 0), at(12, 6.5), at(12, -6.5)), l.paper(), col, width, false)
	case ArrowDiamond:
		c.path(poly(at(.5, 0), at(8, 5), at(16, 0), at(8, -5)), l.paper(), col, width, false)
	case ArrowDiamondFilled:
		c.path(poly(at(.5, 0), at(8, 5), at(16, 0), at(8, -5)), col, col, width, false)
	case ArrowOne:
		stroke(at(7, 6), at(7, -6))
		stroke(at(11, 6), at(11, -6))
	case ArrowZeroOrOne:
		stroke(at(7, 6), at(7, -6))
		center := at(15, 0)
		c.path(circle(center[0], center[1], 4*k), l.paper(), col, width, false)
	case ArrowOneOrMany, ArrowZeroOrMany:
		stroke(at(0, 6), at(11, 0), at(0, -6))
		if mark == ArrowOneOrMany {
			stroke(at(13, 6), at(13, -6))
		} else {
			center := at(15, 0)
			c.path(circle(center[0], center[1], 4*k), l.paper(), col, width, false)
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
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
