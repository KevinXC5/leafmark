package diagram

import (
	"sort"
	"strings"
)

// metrics 是布局需要的文字度量，由调用方注入，布局本身不依赖界面库。
type metrics struct {
	width func(string) float32 // 单行文字的宽度
	lineH float32              // 行高
	k     float32              // 整体缩放，所有间距随它变化
}

// placed 是布局后的节点，x、y 是中心。
type placed struct {
	node       *Node
	x, y, w, h float32
	lines      []string
}

// route 是一条连线：pts 从 From 走到 To，相邻两点之间画一段沿层方向出入的曲线。
type route struct {
	edge           *Edge
	pts            [][2]float32
	loop           bool // 自环，pts 是节点右侧的四个控制点
	labelLines     []string
	labelX, labelY float32 // 标签中心
	labelW, labelH float32
}

// frame 是子图的外框。
type frame struct {
	title      string
	x, y, w, h float32 // 左上角与尺寸
	depth      int
}

type geometry struct {
	width, height float32
	horizontal    bool // 层沿水平方向排开（LR / RL）
	nodes         []placed
	routes        []route
	frames        []frame
}

// vnode 是分层图里的一个位置：真实节点或长边上的虚拟节点。坐标用抽象的“自上而下”坐标系。
type vnode struct {
	real     int // 真实节点下标，虚拟节点为 -1
	rank     int
	w, h     float32 // 抽象坐标系里的宽（层内方向）与高（层方向）
	x, y     float32
	group    int
	up, down []int
	pos      int
}

// arrange 完成分层布局。
func arrange(g *Graph, m metrics) geometry {
	k := m.k
	horizontal := g.Direction == LeftRight || g.Direction == RightLeft
	geo := geometry{horizontal: horizontal}
	n := len(g.Nodes)
	if n == 0 {
		return geo
	}
	geo.nodes = make([]placed, n)
	vs := make([]vnode, n, n+len(g.Edges))
	for i, node := range g.Nodes {
		p := sizeNode(node, m)
		geo.nodes[i] = p
		vs[i] = vnode{real: i, w: p.w, h: p.h, group: node.group}
		if horizontal {
			vs[i].w, vs[i].h = p.h, p.w
		}
	}

	// 一、去环：深度优先遍历中指向栈上节点的边视为回边，布局时反向。
	out := make([][]int, n)
	for i, e := range g.Edges {
		if e.from != e.to {
			out[e.from] = append(out[e.from], i)
		}
	}
	reversed := make([]bool, len(g.Edges))
	state := make([]uint8, n)
	var visit func(u int)
	visit = func(u int) {
		state[u] = 1
		for _, ei := range out[u] {
			switch v := g.Edges[ei].to; state[v] {
			case 0:
				visit(v)
			case 1:
				reversed[ei] = true
			}
		}
		state[u] = 2
	}
	for i := range g.Nodes {
		if state[i] == 0 {
			visit(i)
		}
	}
	ends := func(i int) (int, int) {
		e := g.Edges[i]
		if reversed[i] {
			return e.to, e.from
		}
		return e.from, e.to
	}

	// 二、分层：按拓扑序做最长路径。
	indeg := make([]int, n)
	succ := make([][]int, n)
	for i, e := range g.Edges {
		if e.from == e.to {
			continue
		}
		u, v := ends(i)
		succ[u] = append(succ[u], i)
		indeg[v]++
	}
	queue := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	maxRank := 0
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, ei := range succ[u] {
			_, v := ends(ei)
			if r := vs[u].rank + max(1, g.Edges[ei].Length); r > vs[v].rank {
				vs[v].rank = r
			}
			maxRank = max(maxRank, vs[v].rank)
			if indeg[v]--; indeg[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	// 三、跨多层的边拆成虚拟节点链。
	chains := make([][]int, len(g.Edges))
	for i, e := range g.Edges {
		if e.from == e.to {
			continue
		}
		u, v := ends(i)
		chain := []int{u}
		for r := vs[u].rank + 1; r < vs[v].rank; r++ {
			vs = append(vs, vnode{real: -1, rank: r, w: 14 * k, group: -1})
			chain = append(chain, len(vs)-1)
		}
		chain = append(chain, v)
		for j := 0; j+1 < len(chain); j++ {
			a, b := chain[j], chain[j+1]
			vs[a].down = append(vs[a].down, b)
			vs[b].up = append(vs[b].up, a)
		}
		chains[i] = chain
	}

	// 四、层内排序：先按遍历顺序落位，再用重心法上下扫描，保留交叉最少的一次。
	layers := make([][]int, maxRank+1)
	seen := make([]bool, len(vs))
	var seed func(v int)
	seed = func(v int) {
		if seen[v] {
			return
		}
		seen[v] = true
		layers[vs[v].rank] = append(layers[vs[v].rank], v)
		for _, d := range vs[v].down {
			seed(d)
		}
	}
	for i := 0; i < n; i++ {
		if len(vs[i].up) == 0 {
			seed(i)
		}
	}
	for i := range vs {
		seed(i)
	}
	index := func() {
		for _, layer := range layers {
			for p, v := range layer {
				vs[v].pos = p
			}
		}
	}
	index()
	segments := 0
	for i := range vs {
		segments += len(vs[i].down)
	}
	crossings := func() int {
		if segments > 3000 {
			return 0 // 规模过大时不计数，直接采用最后一轮结果
		}
		total := 0
		for _, layer := range layers {
			var segs [][2]int
			for _, u := range layer {
				for _, d := range vs[u].down {
					segs = append(segs, [2]int{vs[u].pos, vs[d].pos})
				}
			}
			for a := 0; a < len(segs); a++ {
				for b := a + 1; b < len(segs); b++ {
					if (segs[a][0]-segs[b][0])*(segs[a][1]-segs[b][1]) < 0 {
						total++
					}
				}
			}
		}
		return total
	}
	snapshot := func() [][]int {
		c := make([][]int, len(layers))
		for i, layer := range layers {
			c[i] = append([]int(nil), layer...)
		}
		return c
	}
	best, bestCross := snapshot(), crossings()
	sortLayer := func(r int, upward bool) {
		layer := layers[r]
		keys := make(map[int]float32, len(layer))
		for _, v := range layer {
			near := vs[v].up
			if upward {
				near = vs[v].down
			}
			key := float32(vs[v].pos)
			if len(near) > 0 {
				key = 0
				for _, o := range near {
					key += float32(vs[o].pos)
				}
				key /= float32(len(near))
			}
			keys[v] = key
		}
		sort.SliceStable(layer, func(a, b int) bool { return keys[layer[a]] < keys[layer[b]] })
		for p, v := range layer {
			vs[v].pos = p
		}
	}
	for sweep := 0; sweep < 16 && bestCross > 0; sweep++ {
		if sweep%2 == 0 {
			for r := 1; r <= maxRank; r++ {
				sortLayer(r, false)
			}
		} else {
			for r := maxRank - 1; r >= 0; r-- {
				sortLayer(r, true)
			}
		}
		if c := crossings(); c < bestCross {
			best, bestCross = snapshot(), c
		}
	}
	layers = best
	index()

	// 同一子图的节点在层内排在一起，外框才不会夹住别的节点。
	top := func(group int) int {
		for group >= 0 && g.Subgraphs[group].Parent >= 0 {
			group = g.Subgraphs[group].Parent
		}
		return group
	}
	if len(g.Subgraphs) > 0 {
		for _, layer := range layers {
			mean := func(of func(v int) int) map[int]float32 {
				sum, count := map[int]float32{}, map[int]float32{}
				for _, v := range layer {
					if key := of(v); key >= 0 {
						sum[key] += float32(vs[v].pos)
						count[key]++
					}
				}
				for key := range sum {
					sum[key] /= count[key]
				}
				return sum
			}
			outer := mean(func(v int) int { return top(vs[v].group) })
			inner := mean(func(v int) int { return vs[v].group })
			key := func(v int) (float32, float32) {
				p := float32(vs[v].pos)
				if vs[v].group < 0 {
					return p, p
				}
				return outer[top(vs[v].group)], inner[vs[v].group]
			}
			sort.SliceStable(layer, func(a, b int) bool {
				a1, a2 := key(layer[a])
				b1, b2 := key(layer[b])
				if a1 != b1 {
					return a1 < b1
				}
				return a2 < b2
			})
		}
		index()
	}

	// 五、坐标。层方向：每层取最高的节点，层与层之间留出连线与标签的空间。
	rankSep := 40 * k
	labelLines := make([][]string, len(g.Edges))
	var labelW, labelH []float32
	labelW, labelH = make([]float32, len(g.Edges)), make([]float32, len(g.Edges))
	for i, e := range g.Edges {
		if e.Label == "" {
			continue
		}
		labelLines[i] = strings.Split(e.Label, "\n")
		for _, line := range labelLines[i] {
			labelW[i] = max(labelW[i], m.width(line))
		}
		labelW[i] += 12 * k
		labelH[i] = float32(len(labelLines[i]))*m.lineH + 6*k
		if horizontal {
			rankSep = max(rankSep, labelW[i]+36*k)
		} else {
			rankSep = max(rankSep, labelH[i]+36*k)
		}
	}
	if len(g.Subgraphs) > 0 {
		rankSep += m.lineH + 14*k
	}
	y := float32(0)
	for _, layer := range layers {
		tall := float32(0)
		for _, v := range layer {
			tall = max(tall, vs[v].h)
		}
		for _, v := range layer {
			vs[v].y = y + tall/2
		}
		y += tall + rankSep
	}

	// 层内方向：先紧凑排开，再反复向相邻层邻居的平均位置靠拢。
	// 每次分别从左、从右贪心放置并取平均，两个都满足间距约束，平均后仍然满足。
	gap := func(a, b int) float32 {
		sep := 40 * k
		if vs[a].real < 0 && vs[b].real < 0 {
			sep = 14 * k
		}
		if vs[a].group != vs[b].group {
			sep += 30 * k
		}
		return (vs[a].w+vs[b].w)/2 + sep
	}
	for _, layer := range layers {
		for p, v := range layer {
			if p == 0 {
				vs[v].x = vs[v].w / 2
			} else {
				vs[v].x = vs[layer[p-1]].x + gap(layer[p-1], v)
			}
		}
	}
	align := func(layer []int, mode int) {
		want := make([]float32, len(layer))
		for p, v := range layer {
			var near []int
			if mode <= 0 {
				near = append(near, vs[v].up...)
			}
			if mode >= 0 {
				near = append(near, vs[v].down...)
			}
			want[p] = vs[v].x
			if len(near) > 0 {
				sum := float32(0)
				for _, o := range near {
					sum += vs[o].x
				}
				want[p] = sum / float32(len(near))
			}
		}
		left, right := make([]float32, len(layer)), make([]float32, len(layer))
		for p := range layer {
			left[p] = want[p]
			if p > 0 {
				left[p] = max(left[p], left[p-1]+gap(layer[p-1], layer[p]))
			}
		}
		for p := len(layer) - 1; p >= 0; p-- {
			right[p] = want[p]
			if p+1 < len(layer) {
				right[p] = min(right[p], right[p+1]-gap(layer[p], layer[p+1]))
			}
		}
		for p, v := range layer {
			vs[v].x = (left[p] + right[p]) / 2
		}
	}
	for pass := 0; pass < 24; pass++ {
		if pass%2 == 0 {
			for r := 1; r <= maxRank; r++ {
				align(layers[r], -1)
			}
		} else {
			for r := maxRank - 1; r >= 0; r-- {
				align(layers[r], 1)
			}
		}
	}
	for _, layer := range layers {
		align(layer, 0)
	}

	// 六、换回真实坐标系。
	extent := y - rankSep
	real := func(ax, ay float32) (float32, float32) {
		switch g.Direction {
		case BottomUp:
			return ax, extent - ay
		case LeftRight:
			return ay, ax
		case RightLeft:
			return extent - ay, ax
		}
		return ax, ay
	}
	for i := 0; i < n; i++ {
		geo.nodes[i].x, geo.nodes[i].y = real(vs[i].x, vs[i].y)
	}
	// 同一侧有多条连线时，接点沿节点边错开，按对端位置排序以免交叉。
	port := func(v, other int, down bool) float32 {
		near := vs[v].up
		if down {
			near = vs[v].down
		}
		if len(near) < 2 || vs[v].real < 0 || g.Nodes[vs[v].real].Shape == ShapeDiamond || g.Nodes[vs[v].real].Shape == ShapeCircle || g.Nodes[vs[v].real].Shape == ShapeDoubleCircle {
			return 0
		}
		before := 0
		for _, o := range near {
			if vs[o].x < vs[other].x {
				before++
			}
		}
		step := min(13*k, vs[v].w*.6/float32(len(near)))
		return (float32(before) - float32(len(near)-1)/2) * step
	}
	for i, e := range g.Edges {
		r := route{edge: e, labelLines: labelLines[i], labelW: labelW[i], labelH: labelH[i]}
		if e.from == e.to {
			p := geo.nodes[e.from]
			right, reach := p.x+p.w/2, 30*k
			r.loop = true
			r.pts = [][2]float32{{right, p.y - 7*k}, {right + reach, p.y - 22*k}, {right + reach, p.y + 22*k}, {right, p.y + 7*k}}
			r.labelX, r.labelY = right+reach*.75+r.labelW/2, p.y
			geo.routes = append(geo.routes, r)
			continue
		}
		chain := chains[i]
		first, last := chain[0], chain[len(chain)-1]
		abstract := [][2]float32{{vs[first].x + port(first, chain[1], true), vs[first].y + vs[first].h/2}}
		for _, v := range chain[1 : len(chain)-1] {
			abstract = append(abstract, [2]float32{vs[v].x, vs[v].y})
		}
		abstract = append(abstract, [2]float32{vs[last].x + port(last, chain[len(chain)-2], false), vs[last].y - vs[last].h/2})
		for _, p := range abstract {
			x, y := real(p[0], p[1])
			r.pts = append(r.pts, [2]float32{x, y})
		}
		// 标签放在连线正中：段数为奇数取中间一段的中点，为偶数取中间的锚点。
		if s := len(r.pts) - 1; s%2 == 1 {
			a, b := r.pts[s/2], r.pts[s/2+1]
			r.labelX, r.labelY = (a[0]+b[0])/2, (a[1]+b[1])/2
		} else {
			r.labelX, r.labelY = r.pts[s/2][0], r.pts[s/2][1]
		}
		if reversed[i] {
			for a, b := 0, len(r.pts)-1; a < b; a, b = a+1, b-1 {
				r.pts[a], r.pts[b] = r.pts[b], r.pts[a]
			}
		}
		geo.routes = append(geo.routes, r)
	}

	// 同一对节点之间的多条连线在层内方向错开，避免完全重合。
	pairs := map[[2]int][]int{}
	for i, r := range geo.routes {
		if e := r.edge; !r.loop && len(r.pts) == 2 {
			key := [2]int{min(e.from, e.to), max(e.from, e.to)}
			pairs[key] = append(pairs[key], i)
		}
	}
	for _, list := range pairs {
		// 错开的步长要容得下标签，否则标签会互相压住。
		step := 16 * k
		for _, ri := range list {
			if r := geo.routes[ri]; horizontal {
				step = max(step, r.labelH+4*k)
			} else {
				step = max(step, r.labelW+6*k)
			}
		}
		for j, ri := range list {
			shift := (float32(j) - float32(len(list)-1)/2) * step
			r := &geo.routes[ri]
			axis := 0
			if horizontal {
				axis = 1
			}
			for pi := range r.pts {
				r.pts[pi][axis] += shift
			}
			if axis == 0 {
				r.labelX += shift
			} else {
				r.labelY += shift
			}
		}
	}

	// 七、子图外框：由内向外，包住直接成员与内层外框。
	depth := make([]int, len(g.Subgraphs))
	for i := range g.Subgraphs {
		for p := g.Subgraphs[i].Parent; p >= 0; p = g.Subgraphs[p].Parent {
			depth[i]++
		}
	}
	order := make([]int, len(g.Subgraphs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return depth[order[a]] > depth[order[b]] })
	boxes := make([]*frame, len(g.Subgraphs))
	pad, titleH := 14*k, m.lineH+8*k
	for _, si := range order {
		var x0, y0, x1, y1 float32
		any := false
		grow := func(l, t, r, b float32) {
			if !any {
				x0, y0, x1, y1, any = l, t, r, b, true
				return
			}
			x0, y0, x1, y1 = min(x0, l), min(y0, t), max(x1, r), max(y1, b)
		}
		for i, node := range g.Nodes {
			if node.group == si {
				p := geo.nodes[i]
				grow(p.x-p.w/2, p.y-p.h/2, p.x+p.w/2, p.y+p.h/2)
			}
		}
		for ci, sub := range g.Subgraphs {
			if sub.Parent == si && boxes[ci] != nil {
				c := boxes[ci]
				grow(c.x, c.y, c.x+c.w, c.y+c.h)
			}
		}
		if !any {
			continue
		}
		title := g.Subgraphs[si].Title
		if title == "" {
			title = g.Subgraphs[si].ID
		}
		w := max(x1-x0+2*pad, m.width(title)+2*pad)
		cx := (x0 + x1) / 2
		boxes[si] = &frame{title: title, x: cx - w/2, y: y0 - pad - titleH, w: w, h: y1 - y0 + 2*pad + titleH, depth: depth[si]}
	}
	for i := len(order) - 1; i >= 0; i-- { // 外层先画
		if b := boxes[order[i]]; b != nil {
			geo.frames = append(geo.frames, *b)
		}
	}

	// 八、整体平移到留白之内，并算出画布大小。
	margin := 8 * k
	var x0, y0, x1, y1 float32
	first := true
	grow := func(l, t, r, b float32) {
		if first {
			x0, y0, x1, y1, first = l, t, r, b, false
			return
		}
		x0, y0, x1, y1 = min(x0, l), min(y0, t), max(x1, r), max(y1, b)
	}
	for _, p := range geo.nodes {
		grow(p.x-p.w/2, p.y-p.h/2, p.x+p.w/2, p.y+p.h/2)
	}
	for _, f := range geo.frames {
		grow(f.x, f.y, f.x+f.w, f.y+f.h)
	}
	for _, r := range geo.routes {
		for _, p := range r.pts {
			grow(p[0], p[1], p[0], p[1])
		}
		if len(r.labelLines) > 0 {
			grow(r.labelX-r.labelW/2, r.labelY-r.labelH/2, r.labelX+r.labelW/2, r.labelY+r.labelH/2)
		}
	}
	dx, dy := margin-x0, margin-y0
	for i := range geo.nodes {
		geo.nodes[i].x += dx
		geo.nodes[i].y += dy
	}
	for i := range geo.frames {
		geo.frames[i].x += dx
		geo.frames[i].y += dy
	}
	for i := range geo.routes {
		r := &geo.routes[i]
		for j := range r.pts {
			r.pts[j][0] += dx
			r.pts[j][1] += dy
		}
		r.labelX += dx
		r.labelY += dy
	}
	geo.width, geo.height = x1-x0+2*margin, y1-y0+2*margin
	return geo
}

// sizeNode 按文字与外形算出节点尺寸，保证文字落在外形之内。
func sizeNode(node *Node, m metrics) placed {
	k := m.k
	lines := strings.Split(node.Text, "\n")
	tw := float32(0)
	for _, line := range lines {
		tw = max(tw, m.width(line))
	}
	th := float32(len(lines)) * m.lineH
	padX, padY := 60*k, 10*k
	if node.Shape != ShapeRect && node.Shape != ShapeRound && node.Shape != ShapeSubroutine {
		padX = 20 * k
	}
	w, h := tw+2*padX, th+2*padY
	switch node.Shape {
	case ShapeStadium:
		w += h * .4
	case ShapeSubroutine:
		w += 16 * k
	case ShapeCircle, ShapeDoubleCircle:
		d := max(tw, th) + 2*padY + 4*k
		if node.Shape == ShapeDoubleCircle {
			d += 8 * k
		}
		w, h = d, d
	case ShapeDiamond:
		// 菱形内接矩形满足 tw/w + th/h ≤ 1。
		h = 2*th + 2*padY
		w = tw*h/(h-th) + padX
	case ShapeHexagon:
		w += h * .5
	case ShapeParallelogram, ShapeParallelogramAlt, ShapeTrapezoid, ShapeTrapezoidAlt:
		w += h * .6
	case ShapeFlag:
		w += h * .4
	case ShapeCylinder:
		h += 14 * k
	}
	w = max(w, 40*k)
	return placed{node: node, w: w, h: h, lines: lines}
}
