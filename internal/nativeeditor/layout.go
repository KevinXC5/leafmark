package nativeeditor

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/diagram"
	"leafmark/internal/mathlayout"
	"leafmark/internal/richtext"
)

const (
	padX         = 40
	padY         = 36
	blockGap     = 21
	nestIndent   = 6.4 // 每层嵌套列表的缩进
	tableBefore  = 12  // 表格上下比普通块多留的间距
	tableAfter   = 22
	lineGap      = 4
	quoteBar     = 3
	quoteInset   = 19
	codePad      = 18
	imageMaxH    = 280
	cellPad      = 11
	readingWidth = 856
	bottomPad    = 80
)

// line 是排版后的一行，Glyphs 与 Text 的 cluster 对齐。
type line struct {
	block   int
	y       float32
	ascent  float32
	height  float32
	width   float32
	glyphs  []ui.Glyph
	text    string
	markers []ui.Glyph
	spans   []paintSpan
	origin  int // 该行第一个 rune 在文档中的偏移
	count   int // 行内 rune 数，不含换行
	newline bool
	prefix  float32 // 列表标记等占掉的左侧宽度
}

type paintSpan struct {
	glyphs                  []ui.Glyph
	dy                      float32
	highlight, code, strike bool
	link                    string
	ink                     inkKind
	math                    *mathlayout.Box // 行内公式，glyphs 里只有一个占位字形记录位置
}

// inkKind 是代码着色用的墨色：字符串用强调色，注释用次要色。
type inkKind uint8

const (
	inkText inkKind = iota
	inkAccent
	inkMuted
)

type blockBox struct {
	kind        richtext.Kind
	index       int
	x, y        float32
	w, h        float32
	origin      int
	runes       int
	image       *ui.Bitmap
	alt         string
	raw         bool
	rows        []float32
	cols        []float32
	title       []ui.Glyph
	titleAscent float32
	checked     bool
	math        *mathlayout.Box // 排好的块级公式
	diagram     *diagram.Layout // 排好的流程图
	bullet      bool            // 无序列表项，圆点由绘制阶段画出
	indent      float32         // 嵌套列表项的左缩进
	lines       []int           // 在 layout.lines 中的下标
}

// layout 是一帧的排版结果。
type layout struct {
	lines    []line
	blocks   []blockBox
	height   float32
	width    float32
	fontSize float32
}

const defaultFontFamily = `"Newsreader", "Songti SC", "STSong", "Noto Serif CJK SC", "SimSun", Georgia, serif`

type faceKey struct {
	bold, italic, code bool
	size               int
	family             string
	weight             int
}

// faces 缓存同一字体的塑形结果，避免每帧重复 Shape。
type shapeCache struct {
	faces      map[faceKey]ui.Font
	text       map[shapeKey][]ui.Glyph
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
	g := ui.Shape(text, c.font(size, bold, italic, code))
	// 限制二分测量与长期编辑留下的旧字符串，字形副本归当前排版持有。
	if len(c.text) >= 2048 {
		clear(c.text)
	}
	c.text[k] = g
	return append([]ui.Glyph(nil), g...)
}

func (c *shapeCache) metrics(size float32, bold, code bool) ui.FontMetrics {
	return c.font(size, bold, false, code).Metrics()
}

// flow 按当前宽度把文档排成行。contentW 是可视内容宽度。
func flow(doc *richtext.Document, contentW, fontSize float32, cache *shapeCache, images map[string]*ui.Bitmap) layout {
	if contentW < 80 {
		contentW = 80
	}
	inner := min(cache.readingWidth, contentW-padX*2)
	if inner < 40 {
		inner = 40
	}
	var lay layout
	lay.fontSize = fontSize
	lay.width = contentW
	y := float32(padY)
	blocks := doc.Blocks()
	origin := 0
	topOrdered := false
	for i, b := range blocks {
		box := blockBox{kind: b.Kind, index: i, x: (contentW - inner) / 2, y: y, w: inner, origin: origin}
		switch b.Kind {
		case richtext.Image:
			h := placeImage(&box, b, images, fontSize, cache)
			box.h = h
			box.runes = 1
		case richtext.Raw, richtext.Horizontal:
			h, rendered := float32(0), false
			if b.Kind == richtext.Raw {
				h, rendered = placeRendered(&box, b.Raw, fontSize, cache, &lay)
			}
			if !rendered {
				h = placePlaceholder(&box, b, fontSize, cache, &lay)
			}
			box.h = h
			box.raw = b.Kind == richtext.Raw
			box.runes = 1
		case richtext.Code:
			h := placeCode(&box, b.Code, b.Lang, fontSize, cache, &lay)
			box.h = h
			box.runes = len([]rune(b.Code))
		case richtext.TableBlock:
			h := placeTable(&box, b.Table, fontSize, cache, &lay)
			box.h = h
			box.runes = tableRunes(b.Table)
		default:
			h := placeText(&box, b, inner, fontSize, cache, &lay)
			box.h = h
			box.runes = len([]rune(visibleRuns(b.Runs)))
		}
		gap := float32(blockGap)
		if isListItem(b) {
			if b.Level <= 1 {
				topOrdered = b.Ordered
			}
			// 同一列表的各项紧挨；顶层有序与无序交替时是两个列表，保留块间距。
			if i+1 < len(blocks) && isListItem(blocks[i+1]) && (blocks[i+1].Level > 1 || blocks[i+1].Ordered == topOrdered) {
				gap = 0
			}
		}
		if b.Kind == richtext.TableBlock {
			gap += tableAfter
		}
		if i+1 < len(blocks) && blocks[i+1].Kind == richtext.TableBlock {
			gap += tableBefore
		}
		y += box.h + gap
		origin += box.runes
		if i+1 < len(blocks) {
			origin++ // 块间结构换行
		}
		lay.blocks = append(lay.blocks, box)
	}
	lay.height = y + bottomPad
	for _, ln := range lay.lines {
		lay.width = max(lay.width, padX*2+advanceEnd(ln))
	}
	return lay
}

func isListItem(b richtext.Block) bool {
	return b.Kind == richtext.List || b.Kind == richtext.Task
}

func visibleRuns(runs []richtext.Run) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

func tableRunes(t *richtext.TableData) int {
	if t == nil {
		return 0
	}
	n := 0
	for ri, row := range t.Rows {
		if ri > 0 {
			n++
		}
		for ci, cell := range row {
			if ci > 0 {
				n++
			}
			n += len([]rune(visibleRuns(cell.Runs)))
		}
	}
	return n
}

func headingSize(level int, base float32) float32 {
	switch level {
	case 1:
		return base * 1.888889
	case 2:
		return base * 1.277778
	case 3:
		return base * 1.055556
	default:
		return base * 1.05
	}
}

type styled struct {
	text                string
	bold, italic, code  bool
	strike              bool
	highlight, sub, sup bool
	link                string
	math                bool // 行内公式，text 是 TeX 源码
	partial             bool // 已被拆到下一行的剩余部分
}

func runsToStyled(runs []richtext.Run) []styled {
	out := make([]styled, 0, len(runs))
	for _, r := range runs {
		if r.Text == "" {
			continue
		}
		s := styled{text: r.Text, bold: r.Marks&richtext.MarkBold != 0, italic: r.Marks&(richtext.MarkItalic|richtext.MarkMath) != 0, math: r.Marks&richtext.MarkMath != 0, code: r.Marks&richtext.MarkCode != 0, strike: r.Marks&richtext.MarkStrike != 0, highlight: r.Marks&richtext.MarkHighlight != 0, sub: r.Marks&richtext.MarkSub != 0, sup: r.Marks&richtext.MarkSup != 0}
		if r.Link != nil {
			s.link = r.Link.URL
		}
		out = append(out, s)
	}
	return out
}

func placeText(box *blockBox, b richtext.Block, inner, fontSize float32, cache *shapeCache, lay *layout) float32 {
	size := fontSize
	bold := false
	inset := float32(0)
	prefix := ""
	switch b.Kind {
	case richtext.Heading:
		size = headingSize(b.Level, fontSize)
		oldWeight := cache.weight
		cache.weight = 500
		defer func() { cache.weight = oldWeight }()
		bold = false
	case richtext.Quote:
		inset = quoteInset
	case richtext.Callout:
		inset = 37
		size = 13
		oldFamily := cache.family
		cache.family = `"Inter", system-ui, sans-serif`
		defer func() { cache.family = oldFamily }()
		if b.Callout != nil && b.Callout.Title != "" {
			box.title = ui.Shape(b.Callout.Title, ui.Font{Family: `"Inter", system-ui, sans-serif`, Size: 13})
			box.titleAscent = ui.Font{Family: `"Inter", system-ui, sans-serif`, Size: 13}.Metrics().Ascent
		}
	case richtext.List, richtext.Task:
		box.indent = float32(max(b.Level-1, 0)) * nestIndent
		if b.Kind == richtext.Task {
			box.checked = b.Checked
			inset = 28 + box.indent
		} else if b.Ordered {
			prefix = strconv.Itoa(max(1, b.Start)) + ". "
			// 多位编号也须完整留在正文左侧，命中起点与绘制保持一致。
			inset = box.indent + measure(cache, prefix, fontSize, false, false, false)
		} else {
			box.bullet = true
			inset = 12 + box.indent
		}
	}
	styledRuns := runsToStyled(b.Runs)
	if len(styledRuns) == 0 {
		styledRuns = []styled{{}}
	}
	width := inner - inset
	if b.Kind == richtext.Quote {
		width -= 16
	}
	if b.Kind == richtext.Callout {
		width -= 12
	}
	if width < 20 {
		width = 20
	}
	y := float32(0)
	if b.Kind == richtext.Quote {
		y = 10
	}
	if b.Kind == richtext.Callout {
		y = 12
		if len(box.title) > 0 && visibleRuns(b.Runs) != "" {
			y += 23
		}
	}
	runesBefore := 0
	// 按 run 软换行。前缀只出现在第一行。
	first := true
	var pending []styled
	pending = append(pending, styledRuns...)
	for len(pending) > 0 || first {
		ln := line{block: box.index, origin: box.origin + runesBefore, prefix: inset}
		x := inset
		if first && prefix != "" {
			g := cache.shape(prefix, fontSize, false, false, false)
			shiftGlyphs(g, box.indent)
			ln.markers = g
		}
		var built strings.Builder
		var mathUp, mathDown float32
		for len(pending) > 0 {
			seg := pending[0]
			rs := []rune(seg.text)
			if len(rs) == 0 {
				pending = pending[1:]
				continue
			}
			if seg.math && !seg.partial {
				// 光标不在公式内部时，把整条公式排成一个不可拆分的盒子。
				start := ln.origin + utf8.RuneCountInString(built.String())
				inside := cache.caret > start && cache.caret < start+len(rs)
				if mb := mathBox(cache, seg, size, inside); mb != nil {
					if x-inset+mb.Width > width && built.Len() > 0 {
						break
					}
					g := []ui.Glyph{{Cluster: utf8.RuneCountInString(built.String()), Runes: len(rs), X: x, Advance: mb.Width}}
					ln.spans = append(ln.spans, paintSpan{glyphs: g, math: mb, highlight: seg.highlight, strike: seg.strike, link: seg.link})
					ln.glyphs = append(ln.glyphs, g...)
					mathUp, mathDown = max(mathUp, mb.Ascent), max(mathDown, mb.Descent)
					x += mb.Width
					built.WriteString(seg.text)
					pending = pending[1:]
					continue
				}
			}
			hard := -1
			for i, r := range rs {
				if r == '\n' {
					hard = i
					break
				}
			}
			candidate := rs
			if hard >= 0 {
				candidate = rs[:hard]
			}
			fit := fitRunes(cache, seg, candidate, width-(x-inset), size, bold)
			if hard == 0 {
				pending[0].text = string(rs[1:])
				ln.newline = true
				break
			}
			if fit == 0 {
				if built.Len() == 0 {
					fit = clusterBounds(rs)[1] // 再窄也至少放一个完整簇
					if len(rs) == 0 {
						fit = 0
					}
				} else {
					break
				}
			}
			piece := string(rs[:fit])
			segSize := styledSize(seg, size)
			g := cache.shape(piece, segSize, bold || seg.bold, seg.italic, seg.code)
			shiftGlyphs(g, x)
			// cluster 是 piece 内的 rune 下标，改成相对整行文本。
			base := utf8.RuneCountInString(built.String())
			// Shape 的 Cluster 已是 piece 内 rune 下标。加上本行已有 rune，供命中使用。
			for i := range g {
				g[i].Cluster += base
			}
			dy := float32(0)
			if seg.sub {
				dy = size * .2
			}
			if seg.sup {
				dy = -size * .35
			}
			ln.spans = append(ln.spans, paintSpan{glyphs: g, dy: dy, highlight: seg.highlight, code: seg.code, strike: seg.strike, link: seg.link})
			ln.glyphs = append(ln.glyphs, g...)
			x = advanceOf(g)
			built.WriteString(piece)
			if hard >= 0 && fit == hard {
				pending[0].text = string(rs[fit+1:])
				pending[0].partial = true
				ln.newline = true
				break
			}
			if fit < len(rs) {
				pending[0].text = string(rs[fit:])
				pending[0].partial = true
				break
			}
			pending = pending[1:]
		}
		ln.text = built.String()
		ln.count = len([]rune(ln.text))
		m := cache.metrics(size, bold, false)
		ratio := cache.lineHeight
		if b.Kind == richtext.Heading {
			ratio = 1.4
			if b.Level == 1 {
				ratio = 1.3
			}
		}
		if b.Kind == richtext.Quote {
			ratio = 1.7
		}
		if b.Kind == richtext.Callout {
			ratio = 1.5
		}
		ln.height = max(size*ratio, m.Ascent+m.Descent)
		ln.ascent = m.Ascent + (ln.height-m.Ascent-m.Descent)/2
		// 分式、求和这类高出一行的行内公式把所在行撑开，基线仍与文字对齐。
		if above, below := max(ln.ascent, mathUp+2), max(ln.height-ln.ascent, mathDown+2); above+below > ln.height {
			ln.ascent, ln.height = above, above+below
		}
		ln.y = box.y + y
		ln.width = x
		// 行尾若还有剩余，这一行是软换行，不占文档 rune。

		lay.lines = append(lay.lines, ln)
		box.lines = append(box.lines, len(lay.lines)-1)
		y += ln.height
		runesBefore += ln.count
		if ln.newline {
			runesBefore++
		}
		first = false
		if len(pending) == 0 {
			break
		}
	}
	if b.Kind == richtext.Quote {
		y += 10
	}
	if b.Kind == richtext.Callout {
		y += 12
	}
	return y
}

// mathBox 返回行内公式的排版结果；光标在公式内部或源码不受支持时返回 nil。
func mathBox(cache *shapeCache, seg styled, size float32, inside bool) *mathlayout.Box {
	if inside {
		return nil
	}
	return cache.math(seg.text, size*1.1, false)
}

func styledSize(seg styled, size float32) float32 {
	if seg.code {
		return size * .78
	}
	if seg.sub || seg.sup {
		return size * .75
	}
	return size
}

func fitRunes(cache *shapeCache, seg styled, rs []rune, width, size float32, headingBold bool) int {
	if width <= 0 {
		return 0
	}
	// 二分找到放得下的最长前缀，并尽量在空白处断开。
	lo, hi := 0, len(rs)
	best := 0
	for lo <= hi {
		mid := (lo + hi) / 2
		if mid == 0 {
			lo = 1
			continue
		}
		w := measure(cache, string(rs[:mid]), styledSize(seg, size), headingBold || seg.bold, seg.italic, seg.code)
		if w <= width {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	bounds := clusterBounds(rs)
	for i := len(bounds) - 1; i >= 0; i-- {
		if bounds[i] <= best {
			best = bounds[i]
			break
		}
	}
	if best == 0 || best == len(rs) {
		return best
	}
	for i := best; i > best/2 && i > 0; i-- {
		if unicode.IsSpace(rs[i-1]) {
			return i
		}
	}
	return best
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

func placeCode(box *blockBox, code, lang string, fontSize float32, cache *shapeCache, lay *layout) float32 {
	size := float32(14)
	m := cache.metrics(size, false, true)
	lineH := max(size*1.75, m.Ascent+m.Descent)
	parts := strings.Split(code, "\n")
	// 代码正文以换行结尾时，末尾的空行不单独占一行高度。
	if len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	y := float32(14)
	runesBefore := 0
	for _, part := range parts {
		ln := line{block: box.index, y: box.y + y, ascent: m.Ascent + (lineH-m.Ascent-m.Descent)/2, height: lineH, prefix: codePad, text: part, origin: box.origin + runesBefore, count: len([]rune(part)), newline: true}
		g := cache.shape(part, size, false, false, true)
		shiftGlyphs(g, codePad)
		ln.glyphs = g
		ln.spans = codeSpans(g, part, lang)
		ln.width = advanceOf(g)
		lay.lines = append(lay.lines, ln)
		box.lines = append(box.lines, len(lay.lines)-1)
		y += lineH
		runesBefore += ln.count + 1
	}
	if n := len(lay.lines); n > 0 && !strings.HasSuffix(code, "\n") {
		lay.lines[n-1].newline = false
	}
	return y + 14
}

// codeSpans 给一行代码做最小着色：字符串字面量用强调色，行注释用次要色。
func codeSpans(glyphs []ui.Glyph, text, lang string) []paintSpan {
	rs := []rune(text)
	inks := make([]inkKind, len(rs)+1)
	hash := false
	switch strings.ToLower(lang) {
	case "py", "python", "sh", "bash", "zsh", "shell", "yaml", "yml", "toml", "ruby", "rb", "r", "perl", "dockerfile", "makefile":
		hash = true
	}
	colored := false
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '"' || r == '\'' || r == '`':
			end := i + 1
			for end < len(rs) && rs[end] != r {
				if rs[end] == '\\' && r != '`' {
					end++
				}
				end++
			}
			if end >= len(rs) {
				continue // 未闭合的引号不着色
			}
			for j := i; j <= end; j++ {
				inks[j] = inkAccent
			}
			colored = true
			i = end
		case (r == '/' && i+1 < len(rs) && rs[i+1] == '/' && (i == 0 || rs[i-1] != ':')) || (hash && r == '#'):
			for j := i; j < len(rs); j++ {
				inks[j] = inkMuted
			}
			colored = true
			i = len(rs)
		}
	}
	if !colored {
		return nil
	}
	var spans []paintSpan
	for _, g := range glyphs {
		ink := inks[max(0, min(g.Cluster, len(rs)))]
		if n := len(spans); n > 0 && spans[n-1].ink == ink {
			spans[n-1].glyphs = append(spans[n-1].glyphs, g)
			continue
		}
		spans = append(spans, paintSpan{glyphs: []ui.Glyph{g}, ink: ink})
	}
	return spans
}

func placeTable(box *blockBox, table *richtext.TableData, fontSize float32, cache *shapeCache, lay *layout) float32 {
	if table == nil || len(table.Rows) == 0 {
		return fontSize + 8
	}
	cols := 0
	for _, row := range table.Rows {
		cols = max(cols, len(row))
	}
	if cols == 0 {
		return fontSize + 8
	}
	// 表格列宽按内容分配；同一列所有行共享边界和对齐方式。
	widths := make([]float32, cols)
	total := float32(0)
	for c := 0; c < cols; c++ {
		widths[c] = 48
		for _, row := range table.Rows {
			if c < len(row) {
				widths[c] = max(widths[c], measure(cache, visibleRuns(row[c].Runs), fontSize, false, false, false)+cellPad*2)
			}
		}
		total += widths[c]
	}
	for c := range widths {
		widths[c] = box.w * widths[c] / total
	}
	box.cols = append(box.cols, 0)
	for _, w := range widths {
		box.cols = append(box.cols, box.cols[len(box.cols)-1]+w)
	}
	y := float32(0)
	runesBefore := 0
	box.rows = append(box.rows, 0)
	for ri, row := range table.Rows {
		if ri > 0 {
			runesBefore++
		}
		rowH := fontSize*cache.lineHeight + 14.4
		for ci := 0; ci < cols; ci++ {
			if ci > 0 && ci < len(row) {
				runesBefore++
			}
			var runs []richtext.Run
			if ci < len(row) {
				runs = row[ci].Runs
			}
			cell := blockBox{index: box.index, x: box.x, y: box.y + y + 7.2, w: widths[ci] - cellPad*2, origin: box.origin + runesBefore}
			oldWeight := cache.weight
			if table.Header && ri == 0 {
				cache.weight = 600
			}
			h := placeText(&cell, richtext.Block{Kind: richtext.Paragraph, Runs: runs}, cell.w, fontSize, cache, lay)
			cache.weight = oldWeight
			rowH = max(rowH, h+14.4)
			for _, idx := range cell.lines {
				ln := &lay.lines[idx]
				dx := box.cols[ci] + cellPad
				if ci < len(table.Aligns) {
					switch table.Aligns[ci] {
					case richtext.AlignCenter:
						dx += (cell.w - advanceEnd(*ln)) / 2
					case richtext.AlignRight:
						dx += cell.w - advanceEnd(*ln)
					}
				}
				ln.prefix = dx
				shiftGlyphs(ln.glyphs, dx)
				for _, span := range ln.spans {
					shiftGlyphs(span.glyphs, dx)
				}
				ln.width = advanceOf(ln.glyphs)
				box.lines = append(box.lines, idx)
			}
			runesBefore += len([]rune(visibleRuns(runs)))
		}
		y += rowH
		box.rows = append(box.rows, y)
	}
	return y
}

// 原文仍由文档模型保留，类型提示不假装已经渲染复杂语法。
func rawContentLabel(source string) string {
	s := strings.TrimSpace(source)
	switch {
	case strings.Contains(s, "```mermaid"):
		return "Mermaid 图表 · 原文保留"
	case strings.HasPrefix(s, "$$") || strings.Contains(s, "$"):
		return "数学公式 · 原文保留"
	case strings.Contains(s, "[!"):
		return "提示块 · 原文保留"
	case strings.HasPrefix(s, "<"):
		return "HTML 内容 · 原文保留"
	case strings.HasPrefix(s, "[^") || strings.Contains(s, "[^"):
		return "脚注 · 原文保留"
	case strings.HasPrefix(s, "-") || strings.HasPrefix(s, "*"):
		return "嵌套列表 · 原文保留"
	default:
		return "扩展内容 · 原文保留"
	}
}

func placePlaceholder(box *blockBox, b richtext.Block, fontSize float32, cache *shapeCache, lay *layout) float32 {
	kind := b.Kind
	text := rawContentLabel(b.Raw)
	if kind == richtext.Horizontal {
		text = ""
	}
	m := cache.metrics(fontSize*0.9, false, false)
	h := m.Ascent + m.Descent + 16
	if kind == richtext.Horizontal {
		h = 18
	}
	ln := line{block: box.index, y: box.y + 8, ascent: m.Ascent, height: h, text: text, origin: box.origin, count: 0, prefix: 12}
	if text != "" {
		ln.glyphs = cache.shape(text, fontSize*0.9, false, false, false)
		shiftGlyphs(ln.glyphs, 12)
		ln.width = advanceOf(ln.glyphs)
		box.w = min(box.w, ln.width+12)
	}
	lay.lines = append(lay.lines, ln)
	box.lines = append(box.lines, len(lay.lines)-1)
	return h
}

func placeImage(box *blockBox, b richtext.Block, images map[string]*ui.Bitmap, fontSize float32, cache *shapeCache) float32 {
	box.alt = b.Alt
	h := float32(72)
	if bm := images[b.URL]; bm != nil {
		w, ih := bm.Size()
		box.image = bm
		if w > 0 {
			scale := box.w / float32(w)
			h = float32(ih) * scale
			if h > imageMaxH {
				h = imageMaxH
			}
			if h < 24 {
				h = 24
			}
		}
	}
	if b.Alt != "" {
		h += fontSize + 6
		_ = cache
	}
	return h + 8
}

// caretX 返回文档偏移在某一行上的横坐标（相对块内容左缘，不含块的 x）。
func (ln line) caretX(offset int) float32 {
	local := offset - ln.origin
	if local <= 0 {
		return ln.prefix
	}
	if ln.count == 0 {
		return ln.prefix
	}
	if local > ln.count {
		local = ln.count
	}
	// 找到 cluster >= local 的第一颗字形，光标在它左边；否则在行尾。
	var prev float32 = ln.prefix
	seen := false
	for _, g := range ln.glyphs {
		if g.Runes <= 0 && g.Cluster == 0 && g.Advance == 0 {
			continue
		}
		// 前缀字形的 cluster 也从 0 起，用 X 是否落在 prefix 之前区分。

		if !seen {
			prev = ln.prefix
			seen = true
		}
		if g.Cluster >= local {
			return g.X
		}
		prev = g.X + g.Advance
	}
	if !seen && ln.prefix > 0 {
		// 只有前缀
		return advanceEnd(ln)
	}
	return prev
}

func advanceEnd(ln line) float32 {
	m := ln.prefix
	for _, g := range ln.glyphs {
		if end := g.X + g.Advance; end > m {
			m = end
		}
	}
	return m
}

// hitOffset 把行内横坐标换成文档 rune 偏移。
func (ln line) hitOffset(x float32) int {
	best, dist := ln.origin, float32(1e30)
	for _, local := range clusterBounds([]rune(ln.text)) {
		at := ln.origin + local
		d := abs32(x - ln.caretX(at))
		if d < dist {
			best, dist = at, d
		}
	}
	return best
}

// lineAt 找到包含该文档偏移的行。偏移落在块间换行上时归到下一行的起点。
func (l layout) lineAt(offset int) int {
	if len(l.lines) == 0 {
		return -1
	}
	for i, ln := range l.lines {
		end := ln.origin + ln.count
		if offset < end {
			return i
		}
		if offset == end {
			if i+1 < len(l.lines) && l.lines[i+1].block == ln.block && l.lines[i+1].origin == end && !ln.newline {
				continue
			}
			return i
		}
	}
	return len(l.lines) - 1
}

// hit 把内容坐标换成文档偏移。
func (l layout) hit(x, y float32) int {
	if len(l.blocks) == 0 {
		return 0
	}
	bi := 0
	for i, b := range l.blocks {
		if y >= b.y {
			bi = i
		}
	}
	b := l.blocks[bi]
	if b.kind == richtext.Image || b.kind == richtext.Raw || b.kind == richtext.Horizontal {
		if y < b.y+b.h/2 {
			return b.origin
		}
		return b.origin + b.runes
	}
	if len(b.lines) == 0 {
		return b.origin
	}
	li := b.lines[0]
	for _, idx := range b.lines {
		ln := l.lines[idx]
		if y >= ln.y && (ln.y > l.lines[li].y || x-b.x >= ln.prefix) {
			li = idx
		}
	}
	ln := l.lines[li]
	return ln.hitOffset(x - b.x)
}
