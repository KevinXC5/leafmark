// Package mathlayout 把常用的 TeX 数学公式排成可以用 ui.Painter 绘制的盒子。
//
// 字形来自内嵌的 STIX Two Math：普通字符交给 MyGo 的文字引擎绘制，
// 根号、可伸缩括号和块级大型运算符读取字体 MATH 表里的尺寸变体与拼接部件，
// 以轮廓路径绘制。
package mathlayout

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Family 是内嵌数学字体注册给 MyGo 的字体族名。
const Family = "Leafmark Math"

//go:embed fonts/STIXTwoMath-Regular.otf
var fontData []byte

// FontLicense 是内嵌字体 STIX Two Math 的许可证全文（SIL OFL 1.1）。
//
//go:embed fonts/STIXTwoMath-LICENSE.txt
var FontLicense string

var (
	registerOnce sync.Once
	registerErr  error
)

// Register 把内嵌的数学字体注册给 MyGo。可重复调用，只有第一次真正注册。
func Register() error {
	registerOnce.Do(func() {
		if err := ui.RegisterFont(fontData, Family); err != nil {
			registerErr = fmt.Errorf("注册数学字体：%w", err)
		}
	})
	return registerErr
}

// MATH 表 MathConstants 中 MathValueRecord 的下标，顺序与 OpenType 规范一致。
const (
	cMathLeading = iota
	cAxisHeight
	cAccentBaseHeight
	cFlattenedAccentBaseHeight
	cSubscriptShiftDown
	cSubscriptTopMax
	cSubscriptBaselineDropMin
	cSuperscriptShiftUp
	cSuperscriptShiftUpCramped
	cSuperscriptBottomMin
	cSuperscriptBaselineDropMax
	cSubSuperscriptGapMin
	cSuperscriptBottomMaxWithSubscript
	cSpaceAfterScript
	cUpperLimitGapMin
	cUpperLimitBaselineRiseMin
	cLowerLimitGapMin
	cLowerLimitBaselineDropMin
	cStackTopShiftUp
	cStackTopDisplayStyleShiftUp
	cStackBottomShiftDown
	cStackBottomDisplayStyleShiftDown
	cStackGapMin
	cStackDisplayStyleGapMin
	cStretchStackTopShiftUp
	cStretchStackBottomShiftDown
	cStretchStackGapAboveMin
	cStretchStackGapBelowMin
	cFractionNumeratorShiftUp
	cFractionNumeratorDisplayStyleShiftUp
	cFractionDenominatorShiftDown
	cFractionDenominatorDisplayStyleShiftDown
	cFractionNumeratorGapMin
	cFractionNumDisplayStyleGapMin
	cFractionRuleThickness
	cFractionDenominatorGapMin
	cFractionDenomDisplayStyleGapMin
	cSkewedFractionHorizontalGap
	cSkewedFractionVerticalGap
	cOverbarVerticalGap
	cOverbarRuleThickness
	cOverbarExtraAscender
	cUnderbarVerticalGap
	cUnderbarRuleThickness
	cUnderbarExtraDescender
	cRadicalVerticalGap
	cRadicalDisplayStyleVerticalGap
	cRadicalRuleThickness
	cRadicalExtraAscender
	cRadicalKernBeforeDegree
	cRadicalKernAfterDegree
	numConstants
)

// glyphMetrics 是字形的墨迹范围，单位 em，Y 向上为正。
type glyphMetrics struct {
	advance                float32
	xMin, xMax, yMin, yMax float32
	ok                     bool
}

type variant struct {
	gid     font.GID
	advance float32 // 沿伸缩方向的尺寸，em
}

type part struct {
	gid              font.GID
	start, end, full float32 // 首尾可重叠长度与完整长度，em
	extender         bool
}

// construction 是一个字形在某个方向上的伸缩方案：先试现成的尺寸变体，不够再拼接。
type construction struct {
	variants []variant
	parts    []part
}

type mathFont struct {
	consts            [numConstants]float32 // em
	scriptScale       float32
	scriptScriptScale float32
	displayOpMin      float32 // 块级大型运算符的最小高度，em
	minOverlap        float32 // 拼接部件的最小重叠，em
	degreeRaise       float32 // 根指数底部相对根号高度的抬升比例
	italic            map[font.GID]float32
	topAccent         map[font.GID]float32
	vert, horiz       map[font.GID]*construction

	mu      sync.Mutex // 保护 face 与缓存；face 自带的缓存不是并发安全的
	face    *font.Face
	upem    float32
	metrics map[font.GID]glyphMetrics
}

var (
	fontOnce   sync.Once
	loadedFont *mathFont
	fontErr    error
)

// loadFont 解析内嵌字体，只做一次。
func loadFont() (*mathFont, error) {
	fontOnce.Do(func() { loadedFont, fontErr = parseMathFont(fontData) })
	return loadedFont, fontErr
}

func parseMathFont(data []byte) (*mathFont, error) {
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解析数学字体：%w", err)
	}
	upem := float32(face.Upem())
	if upem <= 0 {
		return nil, errors.New("数学字体缺少 unitsPerEm")
	}
	f := &mathFont{
		face: face, upem: upem,
		scriptScale: 0.7, scriptScriptScale: 0.5,
		italic:    map[font.GID]float32{},
		topAccent: map[font.GID]float32{},
		vert:      map[font.GID]*construction{},
		horiz:     map[font.GID]*construction{},
		metrics:   map[font.GID]glyphMetrics{},
	}
	math := findTable(data, "MATH")
	if math == nil {
		return nil, errors.New("数学字体缺少 MATH 表")
	}
	f.parseMath(math)
	return f, nil
}

// table 是越界读取返回 0 的大端字节视图。
type table []byte

func (t table) u16(off int) int {
	if off < 0 || off+2 > len(t) {
		return 0
	}
	return int(t[off])<<8 | int(t[off+1])
}

func (t table) i16(off int) int { return int(int16(t.u16(off))) }

func (t table) u32(off int) int { return t.u16(off)<<16 | t.u16(off+2) }

// sub 返回从 off 开始的子表；偏移为 0 表示子表不存在。
func (t table) sub(off int) table {
	if off <= 0 || off >= len(t) {
		return nil
	}
	return t[off:]
}

func findTable(data []byte, tag string) table {
	t := table(data)
	n := t.u16(4)
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if rec+16 > len(data) {
			return nil
		}
		if string(data[rec:rec+4]) != tag {
			continue
		}
		off, length := t.u32(rec+8), t.u32(rec+12)
		if off < 0 || length < 0 || off+length > len(data) {
			return nil
		}
		return table(data[off : off+length])
	}
	return nil
}

// coverage 按覆盖下标顺序列出字形。
func (t table) coverage() []font.GID {
	var out []font.GID
	switch t.u16(0) {
	case 1:
		n := t.u16(2)
		for i := 0; i < n; i++ {
			out = append(out, font.GID(t.u16(4+2*i)))
		}
	case 2:
		n := t.u16(2)
		for i := 0; i < n; i++ {
			start, end := t.u16(4+6*i), t.u16(6+6*i)
			for g := start; g <= end; g++ {
				out = append(out, font.GID(g))
			}
		}
	}
	return out
}

func (f *mathFont) parseMath(math table) {
	em := func(v int) float32 { return float32(v) / f.upem }

	if c := math.sub(math.u16(4)); c != nil {
		if v := c.i16(0); v > 0 {
			f.scriptScale = float32(v) / 100
		}
		if v := c.i16(2); v > 0 {
			f.scriptScriptScale = float32(v) / 100
		}
		f.displayOpMin = em(c.u16(6))
		for i := range f.consts {
			f.consts[i] = em(c.i16(8 + 4*i))
		}
		f.degreeRaise = float32(c.i16(8+4*numConstants)) / 100
	}

	if info := math.sub(math.u16(6)); info != nil {
		f.valueMap(info.sub(info.u16(0)), f.italic)
		f.valueMap(info.sub(info.u16(2)), f.topAccent)
	}

	if v := math.sub(math.u16(8)); v != nil {
		f.minOverlap = em(v.u16(0))
		vertCount, horizCount := v.u16(6), v.u16(8)
		for i, gid := range v.sub(v.u16(2)).coverage() {
			if i < vertCount {
				f.vert[gid] = f.construction(v.sub(v.u16(10 + 2*i)))
			}
		}
		for i, gid := range v.sub(v.u16(4)).coverage() {
			if i < horizCount {
				f.horiz[gid] = f.construction(v.sub(v.u16(10 + 2*vertCount + 2*i)))
			}
		}
	}
}

// valueMap 读取“覆盖表 + MathValueRecord 数组”结构（斜体修正、重音附着点）。
func (f *mathFont) valueMap(t table, into map[font.GID]float32) {
	if t == nil {
		return
	}
	n := t.u16(2)
	for i, gid := range t.sub(t.u16(0)).coverage() {
		if i >= n {
			break
		}
		into[gid] = float32(t.i16(4+4*i)) / f.upem
	}
}

func (f *mathFont) construction(t table) *construction {
	c := &construction{}
	if t == nil {
		return c
	}
	n := t.u16(2)
	for i := 0; i < n; i++ {
		c.variants = append(c.variants, variant{
			gid:     font.GID(t.u16(4 + 4*i)),
			advance: float32(t.u16(6+4*i)) / f.upem,
		})
	}
	if a := t.sub(t.u16(0)); a != nil {
		n := a.u16(4)
		for i := 0; i < n; i++ {
			rec := 6 + 10*i
			c.parts = append(c.parts, part{
				gid:      font.GID(a.u16(rec)),
				start:    float32(a.u16(rec+2)) / f.upem,
				end:      float32(a.u16(rec+4)) / f.upem,
				full:     float32(a.u16(rec+6)) / f.upem,
				extender: a.u16(rec+8)&1 != 0,
			})
		}
	}
	return c
}

// glyph 返回字符在数学字体中的字形；字体没有这个字符时 ok 为 false。
func (f *mathFont) glyph(r rune) (font.GID, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.face.NominalGlyph(r)
}

func (f *mathFont) glyphMetrics(gid font.GID) glyphMetrics {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.metrics[gid]; ok {
		return m
	}
	m := glyphMetrics{advance: f.face.HorizontalAdvance(gid) / f.upem}
	if ext, ok := f.face.GlyphExtents(gid); ok {
		m.ok = true
		m.xMin = ext.XBearing / f.upem
		m.xMax = (ext.XBearing + ext.Width) / f.upem
		m.yMax = ext.YBearing / f.upem
		m.yMin = (ext.YBearing + ext.Height) / f.upem
	}
	f.metrics[gid] = m
	return m
}

// pathSeg 是轮廓路径的一段，坐标相对盒子原点，单位 DIP，Y 向下为正。
type pathSeg struct {
	op  ot.SegmentOp
	pts [3][2]float32
	end bool // 这一段之后闭合子路径
}

// outline 取字形轮廓：按 sx、sy（DIP/em）缩放，再平移到 (dx, dy)。
func (f *mathFont) outline(gid font.GID, sx, sy, dx, dy float32) []pathSeg {
	f.mu.Lock()
	o, ok := f.face.GlyphDataOutline(gid)
	f.mu.Unlock()
	if !ok {
		return nil
	}
	segs := make([]pathSeg, 0, len(o.Segments))
	for _, s := range o.Segments {
		if s.Op == ot.SegmentOpMoveTo && len(segs) > 0 {
			segs[len(segs)-1].end = true
		}
		seg := pathSeg{op: s.Op}
		for i, pt := range s.ArgsSlice() {
			seg.pts[i] = [2]float32{dx + pt.X/f.upem*sx, dy - pt.Y/f.upem*sy}
		}
		segs = append(segs, seg)
	}
	if len(segs) > 0 {
		segs[len(segs)-1].end = true
	}
	return segs
}

// placedPart 是拼接结果中的一个部件，offset 是它沿伸缩方向的起点（em）。
type placedPart struct {
	gid    font.GID
	offset float32
}

// assemble 用拼接部件凑出不小于 target（em）的长度，返回各部件位置与实际总长。
// 重复延伸部件直到够长，再把多出来的长度均摊到各接缝的重叠里。
func (f *mathFont) assemble(parts []part, target float32) ([]placedPart, float32) {
	if len(parts) == 0 {
		return nil, 0
	}
	const maxRepeat = 400
	var seq []part
	for repeat := 0; ; repeat++ {
		seq = seq[:0]
		var total float32
		for _, p := range parts {
			n := 1
			if p.extender {
				n = repeat
			}
			for i := 0; i < n; i++ {
				seq = append(seq, p)
				total += p.full
			}
		}
		if len(seq) == 0 {
			continue
		}
		longest := total - float32(len(seq)-1)*f.minOverlap
		if longest >= target || repeat >= maxRepeat {
			break
		}
		extends := false
		for _, p := range parts {
			extends = extends || (p.extender && p.full > f.minOverlap)
		}
		if !extends {
			break
		}
	}
	var full float32
	for _, p := range seq {
		full += p.full
	}
	joints := float32(len(seq) - 1)
	overlap := f.minOverlap
	if joints > 0 {
		overlap = max(f.minOverlap, (full-target)/joints)
	}
	out := make([]placedPart, len(seq))
	var at float32
	for i, p := range seq {
		if i > 0 {
			// 重叠不能超过相邻两端各自允许的长度。
			at -= min(overlap, seq[i-1].end, p.start)
		}
		out[i] = placedPart{gid: p.gid, offset: at}
		at += p.full
	}
	return out, at
}
