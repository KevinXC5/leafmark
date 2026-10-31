package nativeeditor

import (
	"github.com/egoist/mygo/ui"
	"leafmark/internal/diagram"
	"leafmark/internal/mathlayout"
)

type faceKey struct {
	bold, italic, code bool
	size               int
	family             string
	weight             int
}

// faces 缓存同一字体的塑形结果，避免每帧重复 Shape。
type shapeCache struct {
	faces map[faceKey]ui.Font
	text  map[shapeKey][]ui.Glyph
	// aged 是上一代塑形结果：text 写满后整代退到这里，仍在用的条目命中时搬回 text。
	// 一帧里用到的字符串多于一代容量时也不会全部重新塑形。
	aged       map[shapeKey][]ui.Glyph
	family     string
	lineHeight float32
	weight     int
	// readingWidth 是正文栏的最大宽度，随设置里的阅读宽度变化。
	readingWidth float32
	// caret 是折叠光标的文档偏移（有选区时为 -1）：光标落在行内公式内部时显示源码。
	caret        int
	maths        map[mathKey]*mathlayout.Box
	diagrams     map[diagramKey]*diagram.Layout
	diagramStyle diagram.Style
	// footnotes 是脚注标签到显示编号的映射，每次排版按引用出现的顺序重算。
	footnotes map[string]int
	// source 为真时代码块按源码模式排：无代码块内边距，并用 sourceSpans 着色。
	source      bool
	sourceSpans []SourceSpan
	// sourceKinds 是 sourceSpans 按 rune 展开的着色表，逐字形查色不必再扫全部区间。
	sourceKinds []uint8
	images      map[string]*Picture
	collapsed   map[int]bool
}

type shapeKey struct {
	face faceKey
	text string
}

func newShapeCache() *shapeCache {
	return &shapeCache{faces: map[faceKey]ui.Font{}, text: map[shapeKey][]ui.Glyph{}, family: defaultFontFamily, lineHeight: 1.4, weight: 400, readingWidth: readingWidth, caret: -1, maths: map[mathKey]*mathlayout.Box{}, diagrams: map[diagramKey]*diagram.Layout{}, diagramStyle: diagram.Style{Font: ui.Font{Family: "system-ui, sans-serif", Size: 16}}}
}

func (c *shapeCache) font(size float32, bold, italic, code bool) ui.Font {
	k := faceKey{bold: bold, italic: italic, code: code, size: int(size * 100), family: c.family, weight: c.weight}
	if f, ok := c.faces[k]; ok {
		return f
	}
	family := c.family
	weight := c.weight
	if code {
		family = `"Geist Mono", "SFMono-Regular", "Consolas", monospace`
	}
	if bold {
		weight = 700
	}
	f := ui.Font{Family: family, Size: size, Weight: weight, Italic: italic}
	c.faces[k] = f
	return f
}

func (c *shapeCache) shape(text string, size float32, bold, italic, code bool) []ui.Glyph {
	k := shapeKey{face: faceKey{bold: bold, italic: italic, code: code, size: int(size * 100), family: c.family, weight: c.weight}, text: text}
	if g, ok := c.text[k]; ok {
		return append([]ui.Glyph(nil), g...)
	}
	g, ok := c.aged[k]
	if !ok {
		g = ui.Shape(text, c.font(size, bold, italic, code))
	}
	// 限制二分测量与长期编辑留下的旧字符串，字形副本归当前排版持有。
	if len(c.text) >= shapeGeneration {
		c.aged, c.text = c.text, map[shapeKey][]ui.Glyph{}
	}
	c.text[k] = g
	return append([]ui.Glyph(nil), g...)
}

// shapeGeneration 是一代字形缓存的条目数，两代合计是缓存上限。
const shapeGeneration = 16384

// reset 丢弃全部塑形结果，字号或字体变化后调用。
func (c *shapeCache) reset() {
	clear(c.text)
	clear(c.aged)
}

func (c *shapeCache) metrics(size float32, bold, code bool) ui.FontMetrics {
	return c.font(size, bold, false, code).Metrics()
}

func measure(cache *shapeCache, text string, size float32, bold, italic, code bool) float32 {
	if text == "" {
		return 0
	}
	return advanceOf(cache.shape(text, size, bold, italic, code))
}

func advanceOf(g []ui.Glyph) float32 {
	var m float32
	for _, gl := range g {
		if end := gl.X + gl.Advance; end > m {
			m = end
		}
	}
	return m
}

func shiftGlyphs(g []ui.Glyph, dx float32) {
	if dx == 0 {
		return
	}
	for i := range g {
		g[i].X += dx
	}
}
