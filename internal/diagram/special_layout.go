package diagram

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// mark 是图种专用的绘制元素；原生与 SVG 共用同一组坐标。
type mark struct {
	kind         string
	x, y, w, h   float32
	points       [][2]float32
	text         string
	anchor       int8 // -1 左对齐，0 居中
	fill, stroke string
	dashed       bool
	open         bool // 时序图的开口箭头
	value        float64
}

func titleMark(g *Graph, m metrics, geo *geometry) float32 {
	if g.Title == "" {
		return 20 * m.k
	}
	geo.marks = append(geo.marks, mark{kind: "text", x: geo.width / 2, y: 20 * m.k, text: g.Title})
	return 48 * m.k
}
func arrangeSequence(g *Graph, m metrics) geometry {
	k := m.k
	geo := geometry{}
	gap := 160 * k
	for _, ev := range g.sequence {
		gap = max(gap, m.width(ev.text)+36*k)
	}
	widths := make([]float32, len(g.Nodes))
	xs := map[string]float32{}
	x := 24 * k
	for i, n := range g.Nodes {
		p := sizeNode(n, m)
		widths[i] = max(100*k, p.w)
		x += widths[i] / 2
		xs[n.ID] = x
		x += widths[i]/2 + gap
	}
	geo.width = max(200*k, x-gap+24*k)
	top := titleMark(g, m, &geo)
	y := top + 30*k
	for i, n := range g.Nodes {
		p := sizeNode(n, m)
		p.x, p.y, p.w, p.h = xs[n.ID], y, widths[i], 44*k
		geo.nodes = append(geo.nodes, p)
	}
	y += 45 * k
	lifeStart := y
	auto := false
	number := 0
	var stack []int
	active := map[string][]float32{}
	for _, ev := range g.sequence {
		a, b := xs[ev.from], xs[ev.to]
		switch ev.kind {
		case "autonumber":
			auto = true
		case "activate":
			active[ev.from] = append(active[ev.from], y)
		case "deactivate":
			list := active[ev.from]
			if len(list) > 0 {
				start := list[len(list)-1]
				geo.marks = append(geo.marks, mark{kind: "activation", x: a - 5*k, y: start, w: 10 * k, h: max(k, y-start)})
				active[ev.from] = list[:len(list)-1]
			}
		case "message":
			number++
			text := ev.text
			if auto {
				text = fmt.Sprintf("%d. %s", number, text)
			}
			h := max(56*k, float32(len(strings.Split(text, "\n")))*m.lineH+28*k)
			pts := [][2]float32{{a, y + 24*k}, {b, y + 24*k}}
			tx := (a + b) / 2
			if a == b {
				pts = [][2]float32{{a, y + 20*k}, {a + gap*.45, y + 20*k}, {a + gap*.45, y + 44*k}, {a, y + 44*k}}
				tx = a + gap*.25
				h += 24 * k
			}
			geo.marks = append(geo.marks, mark{kind: "arrow", points: pts, dashed: ev.dashed, open: ev.open}, mark{kind: "text", x: tx, y: y + 8*k, text: text})
			y += h
		case "frame":
			geo.marks = append(geo.marks, mark{kind: "frame", x: 12 * k, y: y, w: geo.width - 24*k, text: ev.text})
			stack = append(stack, len(geo.marks)-1)
			y += 40 * k
		case "divider":
			geo.marks = append(geo.marks, mark{kind: "line", points: [][2]float32{{12 * k, y}, {geo.width - 12*k, y}}, dashed: true}, mark{kind: "text", x: geo.width / 2, y: y + 16*k, text: ev.text})
			y += 40 * k
		case "end":
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			geo.marks[i].h = y - geo.marks[i].y + 12*k
			y += 24 * k
		default:
			if strings.HasPrefix(ev.kind, "note ") {
				lines := strings.Split(ev.text, "\n")
				w := 80 * k
				for _, l := range lines {
					w = max(w, m.width(l)+24*k)
				}
				cx := (a + b) / 2
				if ev.kind == "note left" {
					cx = a - w/2 - 16*k
				}
				if ev.kind == "note right" {
					cx = a + w/2 + 16*k
				}
				left := max(4*k, cx-w/2)
				if left+w > geo.width {
					geo.width = left + w + 16*k
				}
				h := float32(len(lines))*m.lineH + 20*k
				geo.marks = append(geo.marks, mark{kind: "note", x: left, y: y, w: w, h: h, text: ev.text})
				y += h + 16*k
			}
		}
	}
	for id, starts := range active {
		for _, start := range starts {
			geo.marks = append(geo.marks, mark{kind: "activation", x: xs[id] - 5*k, y: start, w: 10 * k, h: y - start})
		}
	}
	// 生命线先绘制，消息与激活条覆盖其上。
	lifelines := []mark{}
	for _, n := range g.Nodes {
		lifelines = append(lifelines, mark{kind: "line", points: [][2]float32{{xs[n.ID], lifeStart}, {xs[n.ID], y + 12*k}}, dashed: true})
	}
	geo.marks = append(lifelines, geo.marks...)
	geo.height = y + 32*k
	emitMarks(&geo, m)
	return geo
}
func arrangeGantt(g *Graph, m metrics) geometry {
	k := m.k
	geo := geometry{}
	left := 100 * k
	start, end := g.tasks[0].start, g.tasks[0].end
	for _, t := range g.tasks {
		left = max(left, m.width(t.text)+24*k)
		if t.start.Before(start) {
			start = t.start
		}
		if t.end.After(end) {
			end = t.end
		}
	}
	span := end.Sub(start).Seconds()
	if span <= 0 {
		span = 86400
	}
	plot := max(520*k, min(1000*k, float32(span/86400)*24*k))
	geo.width = left + plot + 36*k
	y := titleMark(g, m, &geo) + 30*k
	section := ""
	axisY := y
	for tick := 0; tick <= 4; tick++ {
		x := left + plot*float32(tick)/4
		date := start.Add(time.Duration(span*float64(tick)/4) * time.Second)
		geo.marks = append(geo.marks, mark{kind: "text", x: x, y: y, text: date.Format("2006-01-02")})
	}
	y += 28 * k
	for _, t := range g.tasks {
		if t.section != section {
			section = t.section
			if section != "" {
				geo.marks = append(geo.marks, mark{kind: "text", x: left / 2, y: y + 12*k, text: section})
				y += 32 * k
			}
		}
		x := left + plot*float32(t.start.Sub(start).Seconds()/span)
		w := max(4*k, plot*float32(t.end.Sub(t.start).Seconds()/span))
		geo.marks = append(geo.marks, mark{kind: "text", x: left / 2, y: y + 14*k, text: t.text}, mark{kind: "bar", x: x, y: y, w: w, h: 28 * k, text: t.status})
		y += 44 * k
	}
	for tick := 0; tick <= 4; tick++ {
		x := left + plot*float32(tick)/4
		geo.marks = append([]mark{{kind: "line", points: [][2]float32{{x, axisY + 16*k}, {x, y}}, dashed: true}}, geo.marks...)
	}
	geo.height = y + 20*k
	emitMarks(&geo, m)
	return geo
}
func arrangePie(g *Graph, m metrics) geometry {
	k := m.k
	geo := geometry{}
	top := titleMark(g, m, &geo)
	r := 130 * k
	cx, cy := 160*k, top+r+16*k
	sum := 0.
	for _, s := range g.slices {
		sum += s.value
	}
	// 色块在饼图右侧，文字左对齐贴在色块右边，画布宽按最长标签计算。
	box := 16 * k
	gap := 12 * k
	legendX := cx + r + 28*k
	textX := legendX + box + gap
	labels := make([]string, len(g.slices))
	textW := float32(0)
	for i, s := range g.slices {
		labels[i] = fmt.Sprintf("%s：%.2g（%.1f%%）", s.text, s.value, 100*s.value/sum)
		textW = max(textW, m.width(labels[i]))
	}
	angle := -math.Pi / 2
	for i, s := range g.slices {
		end := angle + 2*math.Pi*s.value/sum
		points := [][2]float32{{cx, cy}}
		steps := max(2, int((end-angle)*24))
		for j := 0; j <= steps; j++ {
			a := angle + (end-angle)*float64(j)/float64(steps)
			points = append(points, [2]float32{cx + r*float32(math.Cos(a)), cy + r*float32(math.Sin(a))})
		}
		rowY := top + float32(i)*32*k
		geo.marks = append(geo.marks,
			mark{kind: "slice", points: points, value: float64(i), text: s.text},
			mark{kind: "legend", x: legendX, y: rowY, w: box, h: box, value: float64(i)},
			mark{kind: "text", x: textX, y: rowY + box/2, text: labels[i], anchor: -1},
		)
		angle = end
	}
	geo.width = textX + textW + 16*k
	geo.height = max(cy+r+24*k, top+float32(len(g.slices))*32*k+24*k)
	// 标题在最终宽度上居中；titleMark 写入时宽度还是 0。
	for i := range geo.marks {
		if geo.marks[i].kind == "text" && geo.marks[i].text == g.Title {
			geo.marks[i].x = geo.width / 2
		}
	}
	emitMarks(&geo, m)
	return geo
}

// emitMarks 把图种专用标记收成绘制指令，原生绘制与 SVG 共用。
func emitMarks(geo *geometry, m metrics) {
	k := m.k
	var ops []op
	add := func(o op) { ops = append(ops, o) }
	line := func(pts [][2]float32, role inkRole, dashed bool) {
		if len(pts) < 2 {
			return
		}
		add(op{kind: opPath, path: polyline(pts), stroke: ink{role: role}, dashed: dashed})
	}
	for _, mk := range geo.marks {
		switch mk.kind {
		case "text":
			add(op{kind: opText, x: mk.x, y: mk.y, text: mk.text, anchor: mk.anchor, fill: inkOf(inkText)})
		case "line":
			line(mk.points, inkEdge, mk.dashed)
		case "activation":
			add(op{kind: opRect, x: mk.x, y: mk.y, w: mk.w, h: mk.h, fill: inkOf(inkNodeFill), stroke: inkOf(inkNodeLine)})
		case "arrow":
			if len(mk.points) < 2 {
				continue
			}
			line(mk.points, inkEdge, mk.dashed)
			a, b := mk.points[len(mk.points)-2], mk.points[len(mk.points)-1]
			dx, dy := b[0]-a[0], b[1]-a[1]
			length := float32(math.Hypot(float64(dx), float64(dy)))
			if length < 1 {
				continue
			}
			ux, uy := dx/length, dy/length
			tip := b
			if mk.open {
				left := [2]float32{tip[0] - ux*10*k - uy*4.5*k, tip[1] - uy*10*k + ux*4.5*k}
				right := [2]float32{tip[0] - ux*10*k + uy*4.5*k, tip[1] - uy*10*k - ux*4.5*k}
				add(op{kind: opPath, path: polyline([][2]float32{left, tip, right}), stroke: inkOf(inkEdge)})
			} else {
				root := [2]float32{tip[0] - ux*10*k, tip[1] - uy*10*k}
				left := [2]float32{root[0] - uy*4.5*k, root[1] + ux*4.5*k}
				right := [2]float32{root[0] + uy*4.5*k, root[1] - ux*4.5*k}
				add(op{kind: opPath, path: polygon(tip[0], tip[1], left[0], left[1], right[0], right[1]), fill: inkOf(inkEdge)})
			}
		case "frame":
			h := mk.h
			if h <= 0 {
				h = 28 * k
			}
			labelW := min(m.width(mk.text)+16*k, mk.w)
			add(op{kind: opRect, x: mk.x, y: mk.y, w: mk.w, h: h, r: 6 * k, stroke: inkOf(inkGroupLine)})
			add(op{kind: opRect, x: mk.x, y: mk.y, w: labelW, h: m.lineH + 8*k, r: 4 * k, fill: inkOf(inkGroupFill), stroke: inkOf(inkGroupLine)})
			add(op{kind: opText, x: mk.x + labelW/2, y: mk.y + (m.lineH+8*k)/2, text: mk.text, fill: inkOf(inkText)})
		case "note":
			add(op{kind: opRect, x: mk.x, y: mk.y, w: mk.w, h: mk.h, r: 3 * k, fill: inkOf(inkNoteFill), stroke: inkOf(inkNoteLine)})
			add(op{kind: opText, x: mk.x + mk.w/2, y: mk.y + mk.h/2, text: mk.text, fill: inkOf(inkText)})
		case "bar":
			fill, stroke := inkOf(inkNodeFill), inkOf(inkNodeLine)
			if strings.Contains(mk.text, "done") {
				fill = inkOf(inkDoneFill)
			}
			if strings.Contains(mk.text, "active") {
				fill = inkOf(inkActiveFill)
			}
			if strings.Contains(mk.text, "crit") {
				fill, stroke = inkOf(inkCritFill), inkOf(inkCritLine)
			}
			add(op{kind: opRect, x: mk.x, y: mk.y, w: mk.w, h: mk.h, r: 4 * k, fill: fill, stroke: stroke})
		case "slice":
			add(op{kind: opPath, path: polygonOf(mk.points), fill: ink{role: inkSeries, n: int(mk.value)}, stroke: inkOf(inkLabelFill)})
		case "legend":
			add(op{kind: opRect, x: mk.x, y: mk.y, w: mk.w, h: mk.h, r: 3 * k, fill: ink{role: inkSeries, n: int(mk.value)}})
		}
	}
	geo.ops = ops
}

func polygonOf(pts [][2]float32) outline {
	flat := make([]float32, 0, len(pts)*2)
	for _, pt := range pts {
		flat = append(flat, pt[0], pt[1])
	}
	if len(flat) < 6 {
		return nil
	}
	return polygon(flat...)
}
