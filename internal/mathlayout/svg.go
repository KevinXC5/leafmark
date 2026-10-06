package mathlayout

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-text/typesetting/di"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// svgFallbackFonts 是数学字体没有的字符（主要是 \text 里的中文）在 SVG 中使用的字体栈。
const svgFallbackFonts = `'PingFang SC','Hiragino Sans GB','Microsoft YaHei','Noto Sans CJK SC','Source Han Sans SC',sans-serif`

// SVG 导出矢量公式，尺寸单位为 DIP，颜色使用 currentColor（\color 指定的部分除外）。
// 基线在顶部下方 Ascent 处；行内嵌入的 vertical-align 应设为 -Descent px。
//
// 内嵌 STIX 字体里有的字形全部转成轮廓，不依赖浏览器字体或网络资源。
// 字体没有的字符（如 \text 里的中文）输出为 <text> 元素，由查看方按通用的中文字体栈绘制，
// 并用 textLength 固定为排版时占用的宽度。遇到无法显示的字符时返回错误，调用方应保留公式原文。
func (b *Box) SVG() (string, error) {
	if b == nil {
		return "", errors.New("不能导出空公式盒子")
	}
	f, err := loadFont()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" fill="currentColor">`, svgNumber(b.Width), svgNumber(b.Ascent+b.Descent), svgNumber(b.Width), svgNumber(b.Ascent+b.Descent))
	if err := b.writeSVG(&out, f, 0, b.Ascent); err != nil {
		return "", err
	}
	out.WriteString("</svg>")
	return out.String(), nil
}

func svgNumber(n float32) string { return strconv.FormatFloat(float64(n), 'f', 4, 32) }

func (b *Box) writeSVG(out *strings.Builder, f *mathFont, x, y float32) error {
	if b.tinted {
		fmt.Fprintf(out, `<g fill="#%02x%02x%02x">`, b.tint[0], b.tint[1], b.tint[2])
	}
	if err := b.writeSVGText(out, f, x, y); err != nil {
		return err
	}
	for _, r := range b.rules {
		fmt.Fprintf(out, `<rect x="%s" y="%s" width="%s" height="%s"/>`, svgNumber(x+r.x), svgNumber(y+r.y), svgNumber(r.w), svgNumber(r.h))
	}
	for _, p := range b.paths {
		writeSVGPath(out, p.segs, x, y)
	}
	for _, k := range b.kids {
		if err := k.box.writeSVG(out, f, x+k.x, y+k.y); err != nil {
			return err
		}
	}
	if b.tinted {
		out.WriteString("</g>")
	}
	return nil
}

// writeSVGText 输出盒子里的文字：按“字体里有没有”切成若干段，有的转轮廓，没有的输出 <text>。
func (b *Box) writeSVGText(out *strings.Builder, f *mathFont, x, y float32) error {
	runes := []rune(b.svgText)
	// nativeX 是 MyGo 排出的簇位置；文字引擎不可用时没有，改按字体度量累加。
	nativeX := func(cluster int) (float32, bool) {
		for _, g := range b.glyphs {
			if g.Cluster == cluster {
				return g.X, true
			}
		}
		return 0, false
	}
	var pen float32
	for start := 0; start < len(runes); {
		_, inFont := f.glyph(runes[start])
		end := start + 1
		for end < len(runes) {
			if _, ok := f.glyph(runes[end]); ok != inFont {
				break
			}
			end++
		}
		if nx, ok := nativeX(start); ok {
			pen = nx
		}
		run := runes[start:end]
		if inFont {
			pen = b.writeSVGGlyphs(out, f, run, start, pen, x, y, nativeX)
			start = end
			continue
		}
		var width float32
		for _, r := range run {
			if !unicode.IsPrint(r) {
				return fmt.Errorf("字符 %q 无法显示，不能导出", r)
			}
			width += fallbackAdvance(r) * b.svgSize
		}
		// 有文字引擎的结果时，以实际排出的宽度为准。
		if nx, ok := nativeX(end); ok && nx > pen {
			width = nx - pen
		} else if end == len(runes) && len(b.glyphs) > 0 && b.Width > pen {
			width = b.Width - pen
		}
		fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" font-family="%s" textLength="%s" lengthAdjust="spacing">`, svgNumber(x+pen), svgNumber(y), svgNumber(b.svgSize), svgFallbackFonts, svgNumber(width))
		xml.EscapeText(out, []byte(string(run)))
		out.WriteString("</text>")
		pen += width
		start = end
	}
	return nil
}

// writeSVGGlyphs 用内嵌字体排一段文字并输出轮廓，返回排完后的笔位置。
// 连字和重音不能逐字符拆开，所以整段一起排，再按 MyGo 的簇位置对齐。
func (b *Box) writeSVGGlyphs(out *strings.Builder, f *mathFont, run []rune, offset int, pen, x, y float32, nativeX func(int) (float32, bool)) float32 {
	var shaper shaping.HarfbuzzShaper
	f.mu.Lock()
	shaped := shaper.Shape(shaping.Input{Text: run, RunEnd: len(run), Face: f.face, Size: fixed.Int26_6(b.svgSize * 64), Direction: di.DirectionLTR, Script: language.Latin})
	f.mu.Unlock()
	clusterPen := pen
	cluster := -1
	for _, g := range shaped.Glyphs {
		if g.TextIndex() != cluster {
			cluster, clusterPen = g.TextIndex(), pen
		}
		gx := pen
		if nx, ok := nativeX(offset + cluster); ok {
			gx = nx + pen - clusterPen
		}
		segs := f.outline(g.GlyphID, b.svgSize, b.svgSize, x+gx+float32(g.XOffset)/64, y-float32(g.YOffset)/64)
		writeSVGPath(out, segs, 0, 0)
		pen += float32(g.Advance) / 64
	}
	return pen
}

func writeSVGPath(out *strings.Builder, segs []pathSeg, x, y float32) {
	if len(segs) == 0 {
		return
	}
	out.WriteString(`<path d="`)
	for _, s := range segs {
		var cmd byte
		var count int
		switch s.op {
		case ot.SegmentOpMoveTo:
			cmd, count = 'M', 1
		case ot.SegmentOpLineTo:
			cmd, count = 'L', 1
		case ot.SegmentOpQuadTo:
			cmd, count = 'Q', 2
		case ot.SegmentOpCubeTo:
			cmd, count = 'C', 3
		}
		out.WriteByte(cmd)
		for i := 0; i < count; i++ {
			out.WriteString(svgNumber(x + s.pts[i][0]))
			out.WriteByte(' ')
			out.WriteString(svgNumber(y + s.pts[i][1]))
			out.WriteByte(' ')
		}
		if s.end {
			out.WriteByte('Z')
		}
	}
	out.WriteString(`"/>`)
}
