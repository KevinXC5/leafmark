package mathlayout

import (
	"math"

	"github.com/egoist/mygo/ui"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Box 是排好的公式。坐标原点在左端基线上，Y 向下为正。
//
// Ascent 与 Descent 是公式内容实际占用的高度与深度，不含行距；
// 行内公式比正文矮时，调用方应与正文行的度量取较大值。
type Box struct {
	Width, Ascent, Descent float32

	glyphs []ui.Glyph
	rules  []rule
	paths  []*outlinePath
	kids   []kid

	// snapPaths 让轮廓跟着第一条横线一起对齐像素，根号与上横线才不会错位。
	snapPaths bool

	single     bool    // 只含一个字形，上下标按单字符规则定位
	italic     float32 // 斜体修正：上标右移的距离
	accentX    float32 // 重音附着点的横坐标
	hasAccentX bool
}

type kid struct {
	x, y float32
	box  *Box
}

// rule 是实心矩形（分数线、根号横线、上划线），坐标相对盒子原点。
type rule struct{ x, y, w, h float32 }

// outlinePath 是以轮廓绘制的字形；built 缓存上一次绘制位置的路径。
type outlinePath struct {
	segs   []pathSeg
	built  *ui.Path
	bx, by float32
}

func (o *outlinePath) path(x, y float32) *ui.Path {
	if o.built != nil && o.bx == x && o.by == y {
		return o.built
	}
	p := &ui.Path{}
	for _, s := range o.segs {
		a, b, c := s.pts[0], s.pts[1], s.pts[2]
		switch s.op {
		case ot.SegmentOpMoveTo:
			p.MoveTo(x+a[0], y+a[1])
		case ot.SegmentOpLineTo:
			p.LineTo(x+a[0], y+a[1])
		case ot.SegmentOpQuadTo:
			p.QuadTo(x+a[0], y+a[1], x+b[0], y+b[1])
		case ot.SegmentOpCubeTo:
			p.CubeTo(x+a[0], y+a[1], x+b[0], y+b[1], x+c[0], y+c[1])
		}
		if s.end {
			p.Close()
		}
	}
	o.built, o.bx, o.by = p, x, y
	return p
}

// add 把子盒子放在 (x, y) 处并扩大自身的高度与深度；宽度由调用方决定。
func (b *Box) add(k *Box, x, y float32) {
	b.kids = append(b.kids, kid{x: x, y: y, box: k})
	b.Ascent = max(b.Ascent, k.Ascent-y)
	b.Descent = max(b.Descent, k.Descent+y)
}

// Paint 把公式画在 (x, baseline) 处，全部使用颜色 c。
func (b *Box) Paint(p *ui.Painter, x, baseline float32, c ui.Color) {
	if b == nil || p == nil {
		return
	}
	if len(b.glyphs) > 0 {
		p.Glyphs(b.glyphs, x, baseline, c)
	}
	scale := p.Scale()
	if scale <= 0 {
		scale = 1
	}
	var snap float32
	for i, r := range b.rules {
		// 细线对齐到设备像素，并且至少一个像素粗，否则会被舍入成零高度。
		top := round(float64((baseline+r.y)*scale)) / scale
		height := max(round(float64(r.h*scale)), 1) / scale
		if i == 0 {
			snap = top - (baseline + r.y)
		}
		p.Fill(ui.Rect{X: x + r.x, Y: top, W: r.w, H: height}, c, 0)
	}
	if !b.snapPaths {
		snap = 0
	}
	for _, o := range b.paths {
		p.FillPath(o.path(x, baseline+snap), c)
	}
	for _, k := range b.kids {
		k.box.Paint(p, x+k.x, baseline+k.y, c)
	}
}

func round(v float64) float32 { return float32(math.Round(v)) }
