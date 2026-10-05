package mathlayout

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/font"
)

// Layout 把 TeX 数学源码（不含两侧的 $ 或 $$）排成盒子。size 是正文字号（DIP）。
// display 为 true 时用块级样式：大号求和与积分、上下限放在正上正下、分式不缩小。
// 遇到不认识的命令或语法错误返回 error，调用方可以退回显示原文。
func Layout(src string, size float32, display bool) (b *Box, err error) {
	// 排版过程不应 panic；万一出现，也只让这一条公式退回原文，而不是拖垮界面。
	defer func() {
		if r := recover(); r != nil {
			b, err = nil, fmt.Errorf("公式排版失败：%v", r)
		}
	}()
	return layout(src, size, display)
}

func layout(src string, size float32, display bool) (*Box, error) {
	if !(size > 0) || math.IsInf(float64(size), 0) {
		return nil, errors.New("字号必须是正数")
	}
	root, err := parse(src)
	if err != nil {
		return nil, err
	}
	f, err := loadFont()
	if err != nil {
		return nil, err
	}
	if err := Register(); err != nil {
		return nil, err
	}
	l := &layouter{f: f, base: size}
	st := style{level: 1}
	if display {
		st.level = 0
	}
	return l.node(root, st), nil
}

// style 是 TeX 的数学样式：level 0 块级、1 行内、2 上下标、3 二级上下标。
type style struct {
	level   int
	cramped bool // 压缩样式：上标抬得低一些（分母、根号内、下标内）
}

func (s style) sup() style {
	if s.level >= 2 {
		return style{3, s.cramped}
	}
	return style{2, s.cramped}
}

func (s style) sub() style   { return style{s.sup().level, true} }
func (s style) num() style   { return style{min(s.level+1, 3), s.cramped} }
func (s style) den() style   { return style{min(s.level+1, 3), true} }
func (s style) cramp() style { return style{s.level, true} }

type layouter struct {
	f    *mathFont
	base float32
}

func (l *layouter) size(s style) float32 {
	switch s.level {
	case 2:
		return l.base * l.f.scriptScale
	case 3:
		return l.base * l.f.scriptScriptScale
	}
	return l.base
}

// c 返回 MATH 表常量在当前样式字号下的长度。
func (l *layouter) c(i int, s style) float32 { return l.f.consts[i] * l.size(s) }

// pick 按是否块级样式在两个常量之间选择。
func (l *layouter) pick(text, display int, s style) float32 {
	if s.level == 0 {
		return l.c(display, s)
	}
	return l.c(text, s)
}

func (n *node) class() class {
	if n.kind == nScript && n.cls == clsOrd && n.a != nil {
		return n.a.class()
	}
	return n.cls
}

// 原子间距，行是左原子、列是右原子：0 无，1 细，2 细（上下标内省略），
// 3 中（上下标内省略），4 粗（上下标内省略）。出自 The TeXbook 第 18 章。
var spacing = [8][8]uint8{
	clsOrd:   {0, 1, 3, 4, 0, 0, 0, 2},
	clsOp:    {1, 1, 0, 4, 0, 0, 0, 2},
	clsBin:   {3, 3, 0, 0, 3, 0, 0, 3},
	clsRel:   {4, 4, 0, 0, 4, 0, 0, 4},
	clsOpen:  {0, 0, 0, 0, 0, 0, 0, 0},
	clsClose: {0, 1, 3, 4, 0, 0, 0, 2},
	clsPunct: {2, 2, 0, 2, 2, 2, 2, 2},
	clsInner: {2, 1, 3, 4, 2, 0, 2, 2},
}

func (l *layouter) gap(left, right class, s style) float32 {
	if left >= clsNone || right >= clsNone {
		return 0
	}
	mu := l.size(s) / 18
	code := spacing[left][right]
	if code >= 2 && s.level >= 2 {
		return 0
	}
	switch code {
	case 1, 2:
		return 3 * mu
	case 3:
		return 4 * mu
	case 4:
		return 5 * mu
	}
	return 0
}

// hlist 排一行原子。before 是这一行左侧的原子类别（clsNone 表示没有），
// 用来决定行首的间距，以及行首的二元运算符是否退化成普通符号。
func (l *layouter) hlist(nodes []*node, st style, before class) *Box {
	cls := make([]class, len(nodes))
	prev, prevAt, atoms := before, -1, 0
	for i, n := range nodes {
		c := n.class()
		cls[i] = c
		if c == clsNone {
			continue
		}
		atoms++
		// 二元运算符前面没有操作数时当作普通符号（如负号）。
		if c == clsBin {
			switch prev {
			case clsNone, clsBin, clsOp, clsRel, clsOpen, clsPunct:
				c = clsOrd
				cls[i] = c
			}
		}
		if (c == clsRel || c == clsClose || c == clsPunct) && prevAt >= 0 && cls[prevAt] == clsBin {
			cls[prevAt] = clsOrd
		}
		prev, prevAt = c, i
	}
	if prevAt >= 0 && cls[prevAt] == clsBin {
		cls[prevAt] = clsOrd
	}

	// 只有一个原子时直接返回它的盒子，保留单字符信息供外层的上下标与重音使用。
	if atoms == 1 && len(nodes) == 1 && before == clsNone {
		return l.node(nodes[0], st)
	}

	b := &Box{}
	var x float32
	prev = before
	cur := st
	for i, n := range nodes {
		switch n.kind {
		case nStyle:
			cur = style{n.level, cur.cramped}
			continue
		case nSpace:
			x += n.size / 18 * l.size(cur)
			continue
		}
		if prev != clsNone {
			x += l.gap(prev, cls[i], cur)
		}
		k := l.node(n, cur)
		b.add(k, x, 0)
		x += k.Width
		prev = cls[i]
	}
	b.Width = max(x, 0)
	return b
}

func (l *layouter) node(n *node, st style) *Box {
	if n == nil {
		return &Box{}
	}
	switch n.kind {
	case nSymbol, nText:
		return l.glyphs(n.text, st)
	case nGroup:
		return l.hlist(n.list, st, clsNone)
	case nFrac:
		return l.frac(n, st)
	case nSqrt:
		return l.sqrt(n, st)
	case nScript:
		return l.scripts(n, st)
	case nOp:
		return l.op(n, st)
	case nLeftRight:
		return l.fenced(l.hlist(n.list, st, clsOpen), n.left, n.right, st)
	case nDelim:
		return l.delimiter(n.left, n.size*l.size(st), st)
	case nAccent:
		return l.accent(n, st)
	case nEnv:
		return l.table(n.table, st)
	}
	return &Box{}
}

// glyphs 用文字引擎排一段字符，高度与深度取自字体里各字形的墨迹范围。
func (l *layouter) glyphs(text string, st style) *Box {
	b := &Box{}
	if text == "" {
		return b
	}
	size := l.size(st)
	b.glyphs = ui.Shape(text, ui.Font{Family: Family, Size: size})
	for _, g := range b.glyphs {
		b.Width = max(b.Width, g.X+g.Advance)
	}
	var count int
	var advance float32
	var first font.GID
	for _, r := range text {
		count++
		gid, ok := l.f.glyph(r)
		if !ok {
			// 数学字体没有的字符（如汉字）由系统字体兜底，按常见的全角字形估计。
			if r != ' ' {
				b.Ascent = max(b.Ascent, 0.86*size)
				b.Descent = max(b.Descent, 0.14*size)
			}
			advance += size
			continue
		}
		if count == 1 {
			first = gid
			b.single = true
		}
		m := l.f.glyphMetrics(gid)
		advance += m.advance * size
		b.Ascent = max(b.Ascent, m.yMax*size)
		b.Descent = max(b.Descent, -m.yMin*size)
	}
	if len(b.glyphs) == 0 {
		// 文字引擎不可用时仍按字体度量占位，保证排版结果有尺寸。
		b.Width = advance
	}
	if count != 1 {
		b.single = false
	}
	if b.single {
		b.italic = l.f.italic[first] * size
		if x, ok := l.f.topAccent[first]; ok {
			b.accentX, b.hasAccentX = x*size, true
		}
	}
	return b
}

// outlineBox 把一个字形以轮廓放进盒子，原点在字形原点。
func (l *layouter) outlineBox(gid font.GID, size float32) *Box {
	m := l.f.glyphMetrics(gid)
	b := &Box{Width: m.advance * size, Ascent: m.yMax * size, Descent: -m.yMin * size}
	b.italic = l.f.italic[gid] * size
	if segs := l.f.outline(gid, size, size, 0, 0); len(segs) > 0 {
		b.paths = append(b.paths, &outlinePath{segs: segs})
	}
	return b
}

// onAxis 把盒子竖直居中到数学轴上。
func (l *layouter) onAxis(b *Box, st style) *Box {
	shift := (b.Ascent-b.Descent)/2 - l.c(cAxisHeight, st)
	out := &Box{Width: b.Width, Ascent: b.Ascent - shift, Descent: b.Descent + shift, italic: b.italic}
	out.kids = []kid{{y: shift, box: b}}
	return out
}

// stretch 竖直伸缩字形到不小于 target（DIP）：先找够高的尺寸变体，再用部件拼接，
// 都没有时把轮廓纵向拉长。返回的盒子原点在字形基线上。
func (l *layouter) stretch(gid font.GID, target, size float32) *Box {
	m := l.f.glyphMetrics(gid)
	cons := l.f.vert[gid]
	if cons != nil {
		for _, v := range cons.variants {
			if v.advance*size >= target {
				return l.outlineBox(v.gid, size)
			}
		}
		if parts, total := l.f.assemble(cons.parts, target/size); len(parts) > 0 {
			b := &Box{Ascent: total * size}
			for _, p := range parts {
				pm := l.f.glyphMetrics(p.gid)
				b.Width = max(b.Width, pm.advance*size)
				// 部件的墨迹底边落在 offset 处，自下而上堆叠。
				segs := l.f.outline(p.gid, size, size, 0, -(p.offset-pm.yMin)*size)
				if len(segs) > 0 {
					b.paths = append(b.paths, &outlinePath{segs: segs})
				}
			}
			return b
		}
		if n := len(cons.variants); n > 0 {
			return l.outlineBox(cons.variants[n-1].gid, size)
		}
	}
	height := (m.yMax - m.yMin) * size
	if !m.ok || height <= 0 {
		return l.outlineBox(gid, size)
	}
	k := max(target/height, 1)
	b := &Box{Width: m.advance * size, Ascent: m.yMax * size * k, Descent: -m.yMin * size * k}
	if segs := l.f.outline(gid, size, size*k, 0, 0); len(segs) > 0 {
		b.paths = append(b.paths, &outlinePath{segs: segs})
	}
	return b
}

// delimiter 排一个总高度不小于 target 的定界符，竖直居中在数学轴上。
func (l *layouter) delimiter(r rune, target float32, st style) *Box {
	size := l.size(st)
	if r == 0 {
		return &Box{Width: 0.12 * size}
	}
	gid, ok := l.f.glyph(r)
	if !ok {
		return l.glyphs(string(r), st)
	}
	m := l.f.glyphMetrics(gid)
	if (m.yMax-m.yMin)*size >= target {
		// 原始大小就够：交给文字引擎绘制，与正文笔画粗细一致。
		return l.onAxis(l.glyphs(string(r), st), st)
	}
	return l.onAxis(l.stretch(gid, target, size), st)
}

// fenced 给内容两侧加上随高度伸缩的定界符。
func (l *layouter) fenced(body *Box, left, right rune, st style) *Box {
	axis := l.c(cAxisHeight, st)
	// TeX 的规则：定界符至少盖住内容离轴较远一侧的 90.1%，且缺口不超过 5pt。
	delta := max(body.Ascent-axis, body.Descent+axis)
	target := max(delta*2*0.901, 2*delta-0.5*l.base)
	lb := l.delimiter(left, target, st)
	rb := l.delimiter(right, target, st)
	out := &Box{}
	out.add(lb, 0, 0)
	out.add(body, lb.Width, 0)
	out.add(rb, lb.Width+body.Width, 0)
	out.Width = lb.Width + body.Width + rb.Width
	return out
}

func (l *layouter) frac(n *node, st style) *Box {
	switch n.fracStyle {
	case 1:
		st.level = 0
	case 2:
		st.level = 1
	}
	num := l.node(n.a, st.num())
	den := l.node(n.b, st.den())
	size := l.size(st)
	axis := l.c(cAxisHeight, st)
	theta := l.c(cFractionRuleThickness, st)

	var up, down float32
	if n.binom {
		theta = 0
		up = l.pick(cStackTopShiftUp, cStackTopDisplayStyleShiftUp, st)
		down = l.pick(cStackBottomShiftDown, cStackBottomDisplayStyleShiftDown, st)
		gap := l.pick(cStackGapMin, cStackDisplayStyleGapMin, st)
		if d := gap - ((up - num.Descent) - (den.Ascent - down)); d > 0 {
			up += d / 2
			down += d / 2
		}
	} else {
		up = l.pick(cFractionNumeratorShiftUp, cFractionNumeratorDisplayStyleShiftUp, st)
		down = l.pick(cFractionDenominatorShiftDown, cFractionDenominatorDisplayStyleShiftDown, st)
		// 分子、分母与分数线之间至少留出规定的空隙。
		gapUp := l.pick(cFractionNumeratorGapMin, cFractionNumDisplayStyleGapMin, st)
		if d := gapUp - ((up - num.Descent) - (axis + theta/2)); d > 0 {
			up += d
		}
		gapDown := l.pick(cFractionDenominatorGapMin, cFractionDenomDisplayStyleGapMin, st)
		if d := gapDown - ((axis - theta/2) - (den.Ascent - down)); d > 0 {
			down += d
		}
	}

	w := max(num.Width, den.Width)
	pad := 0.12 * size
	if n.binom {
		pad = 0
	}
	b := &Box{Width: w + 2*pad}
	b.add(num, pad+(w-num.Width)/2, -up)
	b.add(den, pad+(w-den.Width)/2, down)
	if theta > 0 {
		b.rules = append(b.rules, rule{x: pad, y: -(axis + theta/2), w: w, h: theta})
	}
	if n.binom {
		return l.fenced(b, '(', ')', st)
	}
	return b
}

func (l *layouter) sqrt(n *node, st style) *Box {
	body := l.node(n.a, st.cramp())
	size := l.size(st)
	theta := l.c(cRadicalRuleThickness, st)
	gap := l.pick(cRadicalVerticalGap, cRadicalDisplayStyleVerticalGap, st)
	target := body.Ascent + body.Descent + gap + theta

	gid, ok := l.f.glyph(0x221A)
	if !ok {
		return body
	}
	sign := l.stretch(gid, target, size)
	total := sign.Ascent + sign.Descent
	if total > target {
		// 根号比需要的高：多出的部分一半留给横线下方的空隙。
		gap += (total - target) / 2
	}
	top := body.Ascent + gap + theta // 横线上沿到基线的距离
	drop := sign.Ascent - top        // 根号下移到顶端与横线上沿对齐

	var x float32
	out := &Box{snapPaths: true}
	if n.b != nil {
		index := l.node(n.b, style{3, st.cramped})
		before := l.c(cRadicalKernBeforeDegree, st)
		after := l.c(cRadicalKernAfterDegree, st)
		// 根指数的底边位于根号总高度的固定比例处。
		bottom := (sign.Descent + drop) - l.f.degreeRaise*total
		out.add(index, max(before, 0), bottom-index.Descent)
		x = max(before+index.Width+after, 0)
	}
	// 根号轮廓直接并入本盒子，才能与横线一起对齐像素。
	for _, p := range sign.paths {
		for i := range p.segs {
			for j := range p.segs[i].pts {
				p.segs[i].pts[j][0] += x
				p.segs[i].pts[j][1] += drop
			}
		}
		out.paths = append(out.paths, p)
	}
	out.rules = append(out.rules, rule{x: x + sign.Width, y: -top, w: body.Width, h: theta})
	out.add(body, x+sign.Width, 0)
	out.Width = x + sign.Width + body.Width
	out.Ascent = max(out.Ascent, top+l.c(cRadicalExtraAscender, st))
	out.Descent = max(out.Descent, sign.Descent+drop)
	return out
}

func (l *layouter) op(n *node, st style) *Box {
	if !n.bigSym {
		b := l.glyphs(n.text, st)
		b.single = false
		return b
	}
	r := []rune(n.text)[0]
	gid, ok := l.f.glyph(r)
	if !ok {
		return l.glyphs(n.text, st)
	}
	if st.level == 0 {
		// 块级样式用字体提供的大号变体，以轮廓绘制。
		if cons := l.f.vert[gid]; cons != nil && len(cons.variants) > 0 {
			pick := cons.variants[len(cons.variants)-1]
			for _, v := range cons.variants {
				if v.advance >= l.f.displayOpMin {
					pick = v
					break
				}
			}
			if pick.gid != gid {
				return l.onAxis(l.outlineBox(pick.gid, l.size(st)), st)
			}
		}
	}
	b := l.glyphs(n.text, st)
	out := l.onAxis(b, st)
	out.italic = b.italic
	return out
}

func (l *layouter) scripts(n *node, st style) *Box {
	base := l.node(n.a, st)
	limits := n.a != nil && n.a.kind == nOp && n.a.opLimits && st.level == 0
	switch n.limits {
	case 1:
		limits = true
	case -1:
		limits = false
	}
	if limits && n.primes == 0 {
		return l.limits(base, n, st)
	}

	var sup, sub *Box
	if n.sup != nil {
		sup = l.node(n.sup, st.sup())
	}
	if n.sub != nil {
		sub = l.node(n.sub, st.sub())
	}

	// 上标抬高 u、下标降低 v，规则出自 The TeXbook 附录 G 第 18 条。
	var u, v float32
	if !base.single {
		u = base.Ascent - l.c(cSuperscriptBaselineDropMax, st)
		v = base.Descent + l.c(cSubscriptBaselineDropMin, st)
	}
	out := &Box{}
	out.add(base, 0, 0)
	supX := base.Width + base.italic
	width := base.Width
	if n.primes > 0 {
		// 字体里的撇号已经是抬高的小字形，按基字号直接接在后面。
		primes := l.glyphs(strings.Repeat("′", n.primes), st)
		out.add(primes, supX, 0)
		supX += primes.Width
		width = supX
	}
	if sup != nil {
		shift := l.c(cSuperscriptShiftUp, st)
		if st.cramped {
			shift = l.c(cSuperscriptShiftUpCramped, st)
		}
		u = max(u, shift, sup.Descent+l.c(cSuperscriptBottomMin, st))
	}
	if sub != nil {
		v = max(v, l.c(cSubscriptShiftDown, st))
		if sup == nil {
			v = max(v, sub.Ascent-l.c(cSubscriptTopMax, st))
		} else {
			// 上下标之间留出最小空隙，必要时再整体上移。
			gapMin := l.c(cSubSuperscriptGapMin, st)
			if gap := (u - sup.Descent) - (sub.Ascent - v); gap < gapMin {
				v += gapMin - gap
				if psi := l.c(cSuperscriptBottomMaxWithSubscript, st) - (u - sup.Descent); psi > 0 {
					u += psi
					v -= psi
				}
			}
		}
	}
	space := l.c(cSpaceAfterScript, st)
	if sup != nil {
		out.add(sup, supX, -u)
		width = max(width, supX+sup.Width+space)
	}
	if sub != nil {
		out.add(sub, base.Width, v)
		width = max(width, base.Width+sub.Width+space)
	}
	out.Width = width
	return out
}

// limits 把上下限放在运算符的正上方与正下方。
func (l *layouter) limits(base *Box, n *node, st style) *Box {
	var sup, sub *Box
	width := base.Width
	if n.sup != nil {
		sup = l.node(n.sup, st.sup())
		width = max(width, sup.Width)
	}
	if n.sub != nil {
		sub = l.node(n.sub, st.sub())
		width = max(width, sub.Width)
	}
	out := &Box{Width: width}
	out.add(base, (width-base.Width)/2, 0)
	// 运算符倾斜时（积分号），上限右移、下限左移半个斜体修正。
	if sup != nil {
		gap := max(l.c(cUpperLimitGapMin, st), l.c(cUpperLimitBaselineRiseMin, st)-sup.Descent)
		out.add(sup, (width-sup.Width)/2+base.italic/2, -(base.Ascent + gap + sup.Descent))
	}
	if sub != nil {
		gap := max(l.c(cLowerLimitGapMin, st), l.c(cLowerLimitBaselineDropMin, st)-sub.Ascent)
		out.add(sub, (width-sub.Width)/2-base.italic/2, base.Descent+gap+sub.Ascent)
	}
	return out
}

func (l *layouter) accent(n *node, st style) *Box {
	body := l.node(n.a, st.cramp())
	out := &Box{Width: body.Width, italic: body.italic}
	out.add(body, 0, 0)

	if n.acc.r == 0 {
		if n.acc.under {
			gap := l.c(cUnderbarVerticalGap, st)
			theta := l.c(cUnderbarRuleThickness, st)
			out.rules = append(out.rules, rule{y: body.Descent + gap, w: body.Width, h: theta})
			out.Descent = body.Descent + gap + theta + l.c(cUnderbarExtraDescender, st)
			return out
		}
		gap := l.c(cOverbarVerticalGap, st)
		theta := l.c(cOverbarRuleThickness, st)
		out.rules = append(out.rules, rule{y: -(body.Ascent + gap + theta), w: body.Width, h: theta})
		out.Ascent = body.Ascent + gap + theta + l.c(cOverbarExtraAscender, st)
		return out
	}

	size := l.size(st)
	gid, ok := l.f.glyph(n.acc.r)
	if !ok {
		return out
	}
	// 重音字形按 x 高度设计；内容更高时整体上移。
	lift := body.Ascent - min(body.Ascent, l.c(cAccentBaseHeight, st))

	if n.acc.wide {
		cons := l.f.horiz[gid]
		if cons != nil && len(cons.parts) > 0 && len(cons.variants) > 0 &&
			body.Width > cons.variants[len(cons.variants)-1].advance*size {
			// 比最宽的变体还宽（长箭头）：横向拼接到与内容等宽。
			parts, total := l.f.assemble(cons.parts, body.Width/size)
			start := (body.Width - total*size) / 2
			for _, p := range parts {
				pm := l.f.glyphMetrics(p.gid)
				segs := l.f.outline(p.gid, size, size, start+(p.offset-pm.xMin)*size, -lift)
				if len(segs) > 0 {
					out.paths = append(out.paths, &outlinePath{segs: segs})
				}
				out.Ascent = max(out.Ascent, lift+pm.yMax*size)
			}
			return out
		}
		if cons != nil {
			// 取不超过内容宽度的最宽变体。
			for _, v := range cons.variants {
				if v.advance*size <= body.Width {
					gid = v.gid
				}
			}
		}
	}

	m := l.f.glyphMetrics(gid)
	attach := (m.xMin + m.xMax) / 2
	at := body.Width / 2
	if body.hasAccentX && !n.acc.wide {
		at = body.accentX
		if x, ok := l.f.topAccent[gid]; ok {
			attach = x
		}
	}
	if segs := l.f.outline(gid, size, size, at-attach*size, -lift); len(segs) > 0 {
		out.paths = append(out.paths, &outlinePath{segs: segs})
	}
	out.Ascent = max(out.Ascent, lift+m.yMax*size)
	return out
}

// table 排矩阵、cases、aligned 等按行列对齐的内容，整体竖直居中在数学轴上。
func (l *layouter) table(t *tableNode, st style) *Box {
	spec := t.spec
	cellStyle := st
	if !spec.display && cellStyle.level == 0 {
		cellStyle.level = 1
	}
	size := l.size(cellStyle)

	cols := 0
	for _, row := range t.rows {
		cols = max(cols, len(row))
	}
	cells := make([][]*Box, len(t.rows))
	widths := make([]float32, cols)
	ascents := make([]float32, len(t.rows))
	descents := make([]float32, len(t.rows))
	var height float32
	for r, row := range t.rows {
		// 每行至少有一个支柱的高度，行距才均匀。
		ascents[r], descents[r] = 0.84*size, 0.36*size
		cells[r] = make([]*Box, len(row))
		for c, cell := range row {
			before := clsNone
			if spec.pairs && c%2 == 1 {
				// aligned 的左对齐列以空原子开头，= 前才有关系符的间距。
				before = clsOrd
			}
			b := l.hlist(cell, cellStyle, before)
			cells[r][c] = b
			widths[c] = max(widths[c], b.Width)
			ascents[r] = max(ascents[r], b.Ascent)
			descents[r] = max(descents[r], b.Descent)
		}
		height += ascents[r] + descents[r]
		if r > 0 {
			height += spec.rowGap * size
		}
	}

	xs := make([]float32, cols)
	var width float32
	for c := range widths {
		if c > 0 {
			if !spec.pairs || c%2 == 0 {
				width += spec.colSep * size
			}
		}
		xs[c] = width
		width += widths[c]
	}

	axis := l.c(cAxisHeight, st)
	body := &Box{Width: width, Ascent: height/2 + axis, Descent: height/2 - axis}
	y := -body.Ascent
	for r, row := range cells {
		if r > 0 {
			y += spec.rowGap * size
		}
		y += ascents[r]
		for c, b := range row {
			x := xs[c]
			switch spec.aligns[c%len(spec.aligns)] {
			case 'c':
				x += (widths[c] - b.Width) / 2
			case 'r':
				x += widths[c] - b.Width
			}
			body.kids = append(body.kids, kid{x: x, y: y, box: b})
		}
		y += descents[r]
	}
	if !spec.fenced {
		return body
	}
	if spec.pad > 0 {
		padded := &Box{Width: body.Width + spec.pad*size, Ascent: body.Ascent, Descent: body.Descent}
		padded.kids = []kid{{x: spec.pad * size, box: body}}
		body = padded
	}
	return l.fenced(body, spec.left, spec.right, st)
}
