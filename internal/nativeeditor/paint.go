package nativeeditor

import (
	"math"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// containerRadius 是引用、提示块、代码块、表格表头、原始块与图片占位的统一圆角。
// 行内元素（行内代码、键位、高亮）的圆角较小，不使用这个值。
const containerRadius = 10

func (e *Editor) paint(p *ui.Painter, r ui.Rect, theme *ui.Theme, now time.Time) {
	sel := e.selection()
	// 视口外的元素跳过绘制，上下各留半屏余量容纳本帧与滚动偏移的差；viewH <= 0 时不做裁剪。
	clip := e.viewH > 0
	margin := e.viewH * 0.5
	top, bottom := e.scroll.Y-margin, e.scroll.Y+e.viewH+margin
	for _, f := range e.lay.frames {
		if clip && (f.y+f.h < top || f.y > bottom) {
			continue
		}
		e.paintFrame(p, r, f, theme)
	}
	blocks := e.lay.blocks
	firstB, endB := 0, len(blocks)
	if clip && len(blocks) > 0 {
		firstB = sort.Search(len(blocks), func(i int) bool {
			return blocks[i].y+blocks[i].h >= top
		})
		endB = firstB
		for endB < len(blocks) && blocks[endB].y <= bottom {
			endB++
		}
	}
	for i := firstB; i < endB; i++ {
		e.paintBlock(p, r, blocks[i], top, bottom, clip, theme)
	}
	for i := firstB; i < endB; i++ {
		b := blocks[i]
		if len(b.lines) == 0 {
			continue
		}
		if clip && b.kind != richtext.TableBlock {
			// 非表格块内的行按 y 单调递增，二分定位起始可见行并在超出底边时提前结束。
			start := sort.Search(len(b.lines), func(k int) bool {
				ln := e.lay.lines[b.lines[k]]
				return ln.y+ln.height >= top
			})
			for k := start; k < len(b.lines); k++ {
				ln := e.lay.lines[b.lines[k]]
				if ln.y > bottom {
					break
				}
				e.paintSelection(p, r, b, ln, sel, theme)
				e.paintLine(p, r, b, ln, theme)
			}
			continue
		}
		for _, idx := range b.lines {
			ln := e.lay.lines[idx]
			if clip && (ln.y+ln.height < top || ln.y > bottom) {
				continue
			}
			e.paintSelection(p, r, b, ln, sel, theme)
			e.paintLine(p, r, b, ln, theme)
		}
	}
	if e.focusMode {
		// 用纸色盖住光标所在块之外的内容，装饰与文字一并淡化。
		active := e.doc.BlockIndexAt(e.focus)
		veil := theme.Background.Alpha(.58)
		for i := firstB; i < endB; i++ {
			b := blocks[i]
			if b.index != active {
				p.Fill(ui.Rect{X: r.X + b.x - 6, Y: r.Y + b.y - 2, W: b.w + 12, H: b.h + 4}, veil, 0)
			}
		}
	}
	if e.compose != "" {
		e.paintCompose(p, r, theme)
	}
	if int(now.UnixMilli()/530)%2 == 0 && e.anchor == e.focus && e.compose == "" && !e.readOnly {
		c := e.caretRect()
		p.Fill(ui.Rect{X: r.X + c[0], Y: r.Y + c[1], W: 1.5, H: c[3]}, theme.Accent, 0)
	}
}

// paintFrame 画容器的装饰：引用竖线与底色、提示块底色与标题、脚注编号与回跳箭头，
// 以及没有首段正文的列表项标记。数值与单块的引用、提示块和列表项一致。
func (e *Editor) paintFrame(p *ui.Painter, r ui.Rect, f frame, theme *ui.Theme) {
	x, y := r.X+f.x, r.Y+f.y
	switch {
	case f.foot != "":
		if f.rule {
			p.Fill(ui.Rect{X: x, Y: y - blockGap/2, W: min(f.w, 160), H: 1}, theme.Border, 0)
		}
		p.Glyphs(f.label, x, r.Y+f.lineY+f.ascent, theme.Accent)
		if len(f.back) > 0 {
			p.Glyphs(f.back, r.X+f.backX, r.Y+f.backBase, theme.Accent)
		}
	case f.kind == richtext.Quote:
		p.Fill(ui.Rect{X: x, Y: y, W: f.w, H: f.h}, theme.Surface, containerRadius)
		paintQuoteBar(p, x, y, f.h, theme)
	case f.kind == richtext.Callout && f.details:
		p.Fill(ui.Rect{X: x, Y: y, W: f.w, H: f.h}, theme.Surface, containerRadius)
		paintDisclosure(p, x+14, y+18, !f.folded, theme.TextMuted)
		p.Glyphs(f.title, x+32, y+12+f.titleAscent, theme.Text)
	case f.kind == richtext.Callout:
		p.Fill(ui.Rect{X: x, Y: y, W: f.w, H: f.h}, CalloutBackground(theme), containerRadius)
		p.Glyphs(f.title, x+37, y+12+f.titleAscent, theme.Text)
		paintBulb(p, x+13, y+12, theme.TextMuted)
	case f.marker && f.kind == richtext.Task:
		paintCheck(p, taskCheckRect(x+2.5, r.Y+f.lineY+f.ascent, e.fontSize), f.checked, theme)
	case f.marker && f.bullet:
		p.FillPath(new(ui.Path).Circle(x+4, r.Y+f.lineY+f.lineH/2+1, 1.7), theme.TextMuted.Alpha(.75))
	case f.marker:
		p.Glyphs(f.label, x, r.Y+f.lineY+f.ascent, theme.Text)
	}
}

// QuoteRule 是引用竖线的颜色：用弱墨色而不是强调色，强调色只留给链接、选中与勾选。
func QuoteRule(theme *ui.Theme) ui.Color { return theme.TextMuted.Alpha(.35) }

// paintQuoteBar 画引用竖线，竖线的外侧沿容器的圆角弧线收拢，与引用块的底色轮廓保持一致，
// 不会在四角露出直角。弧线按 containerRadius 取圆心，高度不足时圆角随之缩小。
func paintQuoteBar(p *ui.Painter, x, y, h float32, theme *ui.Theme) {
	r := min(containerRadius, h/2)
	if r <= quoteBar {
		// 容器太矮、圆角盖不住竖线时直接画直角竖线。
		p.Fill(ui.Rect{X: x, Y: y, W: quoteBar, H: h}, QuoteRule(theme), 0)
		return
	}
	// cut 是圆角弧与竖线右边缘相交处的角度（上半圆在 (π/2, π) 之间）。
	cut := math.Acos(float64(quoteBar-r) / float64(r))
	// arc 返回圆心为 (x+r, cy) 的圆角上某个角度的点，上角向上凸，下角向下凸。
	arc := func(cy float32, a float64) (float32, float32) {
		return x + r + r*float32(math.Cos(a)), cy - r*float32(math.Sin(a))
	}
	const steps = 6
	top, bottom := y+r, y+h-r
	path := new(ui.Path).MoveTo(x, top)
	for i := 0; i <= steps; i++ {
		px, py := arc(top, math.Pi-(math.Pi-cut)*float64(i)/steps)
		path.LineTo(px, py)
	}
	for i := 0; i <= steps; i++ {
		px, py := arc(bottom, (2*math.Pi-cut)-(math.Pi-cut)*float64(i)/steps)
		path.LineTo(px, py)
	}
	p.FillPath(path.Close(), QuoteRule(theme))
}

// CodeBackground 与导出共用代码块底色；深色下比纸面稍暗。
func CodeBackground(theme *ui.Theme) ui.Color {
	if theme.Dark {
		return theme.Background.Mix(ui.RGB(0, 0, 0), .14)
	}
	return theme.Surface
}

// CalloutBackground 与导出共用提示块的轻微暖色填充。
func CalloutBackground(theme *ui.Theme) ui.Color {
	return theme.Accent.Alpha(.04).Over(theme.Surface)
}

// paintDisclosure 画折叠块标题前的三角。open 时朝下，收起时朝右。
func paintDisclosure(p *ui.Painter, x, y float32, open bool, c ui.Color) {
	path := new(ui.Path)
	if open {
		path.MoveTo(x-4, y-2).LineTo(x+4, y-2).LineTo(x, y+3)
	} else {
		path.MoveTo(x-2, y-4).LineTo(x-2, y+4).LineTo(x+3, y)
	}
	p.FillPath(path, c)
}

// taskCheckRect 把复选框的中心放在文字的视觉中线上。行盒中线受字体上下伸部比例影响，
// 衬线体会比文字低一截；中英文字面的中心大约在基线上方 0.35 个字号处。
func taskCheckRect(x, baseline, fontSize float32) ui.Rect {
	const size = 15
	center := baseline - fontSize*.35
	return ui.Rect{X: x, Y: center - size/2, W: size, H: size}
}

// paintCheck 画任务复选框。
func paintCheck(p *ui.Painter, check ui.Rect, checked bool, theme *ui.Theme) {
	if !checked {
		p.Stroke(check, theme.TextMuted.Alpha(.8), 4, 1.5)
		return
	}
	p.Fill(check, theme.Accent, 4)
	tick := new(ui.Path).MoveTo(check.X+3.8, check.Y+7.8).LineTo(check.X+6.5, check.Y+10.4).LineTo(check.X+11.3, check.Y+4.9)
	p.StrokePath(tick, 1.7, theme.Background)
}

func (e *Editor) paintBlock(p *ui.Painter, r ui.Rect, b blockBox, top, bottom float32, clip bool, theme *ui.Theme) {
	x, y := r.X+b.x, r.Y+b.y
	switch b.kind {
	case richtext.Callout:
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, CalloutBackground(theme), containerRadius)
		p.Glyphs(b.title, x+37, y+12+b.titleAscent, theme.Text)
		// 提示图标是灯泡轮廓，不进入正文坐标。
		paintBulb(p, x+13, y+12, theme.TextMuted)
	case richtext.List:
		if b.bullet && len(b.lines) > 0 {
			ln := e.lay.lines[b.lines[0]]
			dot := new(ui.Path).Circle(x+b.indent+4, r.Y+ln.y+ln.height/2+1, 1.7)
			p.FillPath(dot, theme.TextMuted.Alpha(.75))
		}
	case richtext.Task:
		if len(b.lines) > 0 {
			ln := e.lay.lines[b.lines[0]]
			paintCheck(p, taskCheckRect(x+b.indent+2.5, r.Y+ln.y+ln.ascent, e.fontSize), b.checked, theme)
		}
	case richtext.Quote:
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Surface, containerRadius)
		paintQuoteBar(p, x, y, b.h, theme)
	case richtext.Code:
		if e.sourceMode {
			break
		}
		bg := CodeBackground(theme)
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, bg, containerRadius)
	case richtext.Raw:
		if b.math != nil {
			b.math.Paint(p, x+max(0, (b.w-b.math.Width)/2), y+8+b.math.Ascent, theme.Text)
			return
		}
		if b.diagram != nil {
			b.diagram.Paint(p, x+max(0, (b.w-b.diagram.Width)/2), y+8)
			return
		}
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Surface, containerRadius)
	case richtext.Horizontal:
		p.Fill(ui.Rect{X: x, Y: y + b.h/2, W: b.w, H: 1}, theme.Border, 0)
	case richtext.Image:
		if b.image != nil {
			ih := b.h - 8 // 块末尾的留白不参与图片缩放。
			if b.alt != "" {
				ih -= e.fontSize + 6
			}
			b.image.paint(p, ui.Rect{X: x, Y: y, W: b.w, H: ih}, theme.Dark)
		} else {
			p.Fill(ui.Rect{X: x, Y: y, W: min(b.w, 240), H: 64}, theme.Surface, containerRadius)
		}
	case richtext.TableBlock:
		e.paintTableGrid(p, x, y, b, top, bottom, clip, theme)
	}
}

// paintBulb 在 16×16 的范围内画灯泡轮廓，坐标取自 24 单位的图标网格。
func paintBulb(p *ui.Painter, x, y float32, c ui.Color) {
	const k = float32(16) / 24
	at := func(px, py float32) (float32, float32) { return x + px*k, y + py*k }
	cube := func(path *ui.Path, pts ...float32) {
		c1x, c1y := at(pts[0], pts[1])
		c2x, c2y := at(pts[2], pts[3])
		ex, ey := at(pts[4], pts[5])
		path.CubeTo(c1x, c1y, c2x, c2y, ex, ey)
	}
	bulb := new(ui.Path)
	bulb.MoveTo(at(15, 14))
	cube(bulb, 15.2, 13, 15.7, 12.3, 16.5, 11.5)
	cube(bulb, 17.5, 10.6, 18, 9.3, 18, 8)
	cube(bulb, 18, 4.69, 15.31, 2, 12, 2)
	cube(bulb, 8.69, 2, 6, 4.69, 6, 8)
	cube(bulb, 6, 9, 6.2, 10.2, 7.5, 11.5)
	cube(bulb, 8.2, 12.2, 8.8, 13, 9, 14)
	p.StrokePath(bulb, 1.4, c)
	for _, seg := range [][4]float32{{9, 18, 15, 18}, {10, 22, 14, 22}} {
		x0, y0 := at(seg[0], seg[1])
		x1, y1 := at(seg[2], seg[3])
		p.Line(x0, y0, x1, y1, 1.4, c)
	}
}

// paintTableGrid 只画表头底色与可见行间横线：不描外框、不画列线，列靠对齐区分。
func (e *Editor) paintTableGrid(p *ui.Painter, x, y float32, b blockBox, top, bottom float32, clip bool, theme *ui.Theme) {
	first := 1
	if b.tableHeader && len(b.rows) > 1 {
		if !clip || (b.y+b.rows[1] >= top && b.y <= bottom) {
			p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.rows[1]}, theme.Surface, containerRadius)
		}
		first = 2 // 表头底色已经与正文分开，第一条横线省去
	}
	for i := first; i+1 < len(b.rows); i++ {
		rowY := b.y + b.rows[i]
		if clip {
			if rowY+1 < top {
				continue
			}
			if rowY > bottom {
				break
			}
		}
		p.Fill(ui.Rect{X: x, Y: y + b.rows[i], W: b.w, H: 1}, theme.Border, 0)
	}
}

func (e *Editor) paintLine(p *ui.Painter, r ui.Rect, b blockBox, ln line, theme *ui.Theme) {
	baseX := r.X + b.x
	baseY := r.Y + ln.y + ln.ascent
	if len(ln.glyphs) == 0 {
		return
	}
	color := theme.Text
	if b.raw || b.kind == richtext.Quote || b.muted {
		color = theme.TextMuted
	}
	p.Glyphs(ln.markers, baseX, baseY, color)
	if len(ln.spans) == 0 {
		p.Glyphs(ln.glyphs, baseX, baseY, color)
		return
	}
	for _, span := range ln.spans {
		if len(span.glyphs) == 0 {
			continue
		}
		start := span.glyphs[0].X
		end := span.endX
		if span.highlight {
			p.Fill(ui.Rect{X: baseX + start, Y: r.Y + ln.y + 2, W: end - start, H: ln.height - 4}, ui.RGBA(233, 185, 73, .35), 2)
		}
		if span.kbd {
			cap := ui.Rect{X: baseX + start - 3, Y: r.Y + ln.y + 2, W: end - start + 6, H: ln.height - 4}
			p.Fill(cap, theme.Surface, 4)
		} else if span.code {
			p.Fill(ui.Rect{X: baseX + start - 2, Y: r.Y + ln.y + 2, W: end - start + 4, H: ln.height - 4}, theme.Surface, 3)
		}
		ink := color
		switch span.ink {
		case inkAccent:
			ink = theme.Accent
		case inkMuted:
			ink = theme.TextMuted
		}
		if e.sourceMode && span.source > 0 && int(span.source) < len(e.sourcePalette) {
			ink = e.sourcePalette[span.source]
		}
		if span.linkSafe {
			ink = theme.Accent
		}
		if span.footnote != "" {
			p.Glyphs(span.display, baseX+start, baseY+span.dy, theme.Accent)
		} else if span.image != "" {
			if bm := e.images[span.image]; bm != nil {
				bm.paint(p, ui.Rect{X: baseX + start, Y: r.Y + ln.y + 1, W: end - start, H: ln.height - 2}, theme.Dark)
			} else {
				cap := ui.Rect{X: baseX + start, Y: r.Y + ln.y + 1, W: end - start, H: ln.height - 2}
				p.Fill(cap, theme.Surface, 3)
				p.Glyphs(span.display, baseX, baseY, theme.TextMuted)
			}
		} else if span.math != nil {
			span.math.Paint(p, baseX+start, baseY, ink)
		} else {
			p.Glyphs(span.glyphs, baseX, baseY+span.dy, ink)
		}
		if span.strike {
			p.Fill(ui.Rect{X: baseX + start, Y: baseY - ln.ascent*.35, W: end - start, H: 1}, theme.Text, 0)
		}
		if span.linkSafe {
			p.Fill(ui.Rect{X: baseX + start, Y: baseY + 2, W: end - start, H: 1}, theme.Accent, 0)
		} else if span.underline {
			p.Fill(ui.Rect{X: baseX + start, Y: baseY + 2, W: end - start, H: 1}, ink, 0)
		}
	}
}

func (e *Editor) blockOrigin(index int) int {
	return e.blockBox(index).origin
}

func (e *Editor) paintSelection(p *ui.Painter, r ui.Rect, b blockBox, ln line, sel richtext.Selection, theme *ui.Theme) {
	if sel.Start == sel.End || ln.count == 0 && !ln.newline {
		return
	}
	a := ln.origin
	z := ln.origin + ln.count
	if sel.End <= a || sel.Start >= z && !(ln.newline && sel.Start <= z && sel.End > z) {
		return
	}
	from := max(sel.Start, a)
	to := min(sel.End, z)
	if from >= to && !ln.newline {
		return
	}
	x0 := b.x + ln.caretX(from)
	x1 := b.x + ln.caretX(to)
	if to == z && ln.newline && sel.End > z {
		x1 += e.fontSize * 0.5
	}
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	p.Fill(ui.Rect{X: r.X + x0, Y: r.Y + ln.y, W: max(x1-x0, 1), H: ln.height}, theme.Selection, 2)
}

func (e *Editor) paintCompose(p *ui.Painter, r ui.Rect, theme *ui.Theme) {
	c := e.caretRect()
	// caretRect 已含组合光标偏移，绘制时回到组合起点，基线对齐当前行。
	width := e.composeWidth()
	x := r.X + c[0] - e.composeAdvance
	y := r.Y + c[1]
	p.Fill(ui.Rect{X: x, Y: y, W: width + 2, H: c[3]}, theme.Surface, 2)
	ascent := e.composeAscent
	if ascent <= 0 {
		ascent = e.composeMetrics.Ascent
	}
	p.Glyphs(e.composeGlyphs, x, y+ascent, theme.Text)
	p.Fill(ui.Rect{X: x, Y: y + c[3] - 1, W: width, H: 1}, theme.Accent, 0)
}
