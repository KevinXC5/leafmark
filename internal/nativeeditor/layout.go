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
	innerGap     = 12 // 同一容器内相邻块的间距
	looseGap     = 8  // 含多段正文的列表项与下一项的间距
	footGap      = 10 // 相邻脚注定义的间距
	footScale    = .9 // 脚注定义区的字号比例
	calloutFont  = `"Inter", system-ui, sans-serif`
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
	underline, kbd          bool
	link                    string
	footnote                string     // 脚注引用的标签，glyphs 里只有一个占位字形
	image                   string     // 行内图片地址，display 是替代文字
	display                 []ui.Glyph // 脚注编号或行内图片替代文字
	ink                     inkKind
	source                  uint8           // 源码着色种类，0 表示按 ink 或主题正文色
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
	muted       bool            // 位于引用容器内，文字用次要色
	lines       []int           // 在 layout.lines 中的下标
}

// layout 是一帧的排版结果。
type layout struct {
	lines    []line
	blocks   []blockBox
	frames   []frame
	height   float32
	width    float32
	fontSize float32
}

// frame 是容器的装饰：引用竖线与底色、提示块底色与标题、脚注编号与回跳，
// 以及首块不是正文的列表项标记。容器本身不占正文坐标。
type frame struct {
	kind        richtext.Kind
	foot        string // 脚注定义的标签
	x, y, w, h  float32
	title       []ui.Glyph
	titleAscent float32
	label       []ui.Glyph // 脚注编号或有序列表编号
	labelW      float32
	bullet      bool
	checked     bool
	rule        bool // 脚注定义区起始处的分隔线
	marker      bool // 需要自己画列表标记
	// 首行位置，编号与标记对齐到它。
	lineY, lineH, ascent float32
	back                 []ui.Glyph // 脚注回跳箭头
	backX, backBase      float32
	backRect             ui.Rect
	details              bool
	folded               bool
	foldID               int
}

// openFrame 是排版途中尚未结束的容器。
type openFrame struct {
	c           richtext.Container
	frame       int     // 在 layout.frames 中的下标
	left, right float32 // 容器内正文相对正文栏的左右留白
	markerLeft  float32 // 列表项标记的左缘
	anchor      bool    // 等待首块排完后记下首行位置
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
	images      map[string]*ui.Bitmap
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
	cache.footnotes = footnoteNumbers(blocks)
	cache.images = images
	baseX := (contentW - inner) / 2
	origin := 0
	topOrdered := false
	var stack []openFrame
	for i, b := range blocks {
		if b.Kind == richtext.ReferenceDef {
			continue
		}
		chain := b.Containers
		// 上一块结束时栈里只剩与本块共有的容器，这里打开本块新进入的那些。
		fresh := false
		for k := len(stack); k < len(chain); k++ {
			var parent *openFrame
			if k > 0 {
				parent = &stack[k-1]
			}
			// 标记的字号与字体跟随它所在的引用或提示块。
			size, _, family, _ := ambient(chain[:k+1], fontSize)
			oldFamily := cache.family
			if family != "" {
				cache.family = family
			}
			of, top := lay.open(chain[k], parent, b, i > 0 && blocks[i-1].Footnote != nil, baseX, inner, y, fontSize, size, cache)
			cache.family = oldFamily
			stack = append(stack, of)
			y += top
			fresh = k == len(chain)-1
		}
		left, right := float32(0), float32(0)
		if n := len(stack); n > 0 {
			left, right = stack[n-1].left, stack[n-1].right
		}
		// 列表项容器的首块带标记；同一项里后续的块只是对齐到正文左缘的普通块。
		marker := false
		if n := len(chain); n > 0 && isListItem(b) {
			if marker = fresh && isListContainer(chain[n-1]); marker {
				left = stack[n-1].markerLeft
				b.Level = 1
			} else {
				b.Kind = richtext.Paragraph
			}
		}
		size, ratio, family, muted := ambient(chain, fontSize)
		box := blockBox{kind: b.Kind, index: i, x: baseX + left, y: y, w: max(inner-left-right, 40), origin: origin}
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
			h := placeCode(&box, b.Code, b.Lang, fontSize, cache, &lay, cache.source)
			box.h = h
			box.runes = len([]rune(b.Code))
		case richtext.TableBlock:
			h := placeTable(&box, b.Table, fontSize, cache, &lay)
			box.h = h
			box.runes = tableRunes(b.Table)
		default:
			oldFamily := cache.family
			if family != "" {
				cache.family = family
			}
			box.muted = muted
			h := placeText(&box, b, box.w, size, ratio, cache, &lay)
			cache.family = oldFamily
			box.h = h
			box.runes = len([]rune(visibleRuns(b.Runs)))
		}
		for k := range stack {
			if stack[k].anchor {
				lay.anchor(stack[k].frame, box, fontSize)
				stack[k].anchor = false
			}
		}
		lay.blocks = append(lay.blocks, box)
		y += box.h
		var next []richtext.Container
		if i+1 < len(blocks) {
			next = blocks[i+1].Containers
		}
		shared := sharedContainers(chain, next)
		for k := len(stack) - 1; k >= shared; k-- {
			y += lay.close(stack[k], box, y, fontSize, cache)
		}
		stack = stack[:shared]
		gap := float32(blockGap)
		switch {
		case i+1 >= len(blocks):
		case len(chain) == 0 && len(next) == 0:
			if isListItem(b) {
				if b.Level <= 1 {
					topOrdered = b.Ordered
				}
				// 同一列表的各项紧挨；顶层有序与无序交替时是两个列表，保留块间距。
				if isListItem(blocks[i+1]) && (blocks[i+1].Level > 1 || blocks[i+1].Ordered == topOrdered) {
					gap = 0
				}
			}
		case nextListItem(chain, next, shared):
			// 同一列表的相邻项紧挨；上一项含多段正文时略微拉开。
			gap = looseGap
			if marker {
				gap = 0
			}
		case shared > 0:
			gap = innerGap
		case b.Footnote != nil && blocks[i+1].Footnote != nil:
			gap = footGap
		}
		if b.Kind == richtext.TableBlock {
			gap += tableAfter
		}
		if i+1 < len(blocks) && blocks[i+1].Kind == richtext.TableBlock {
			gap += tableBefore
		}
		y += gap
		origin += box.runes
		if i+1 < len(blocks) {
			origin++ // 块间结构换行
		}
	}
	lay.height = y + bottomPad
	for _, ln := range lay.lines {
		lay.width = max(lay.width, padX*2+advanceEnd(ln))
	}
	return lay
}

func isListContainer(c richtext.Container) bool {
	return c.Footnote == "" && (c.Kind == richtext.List || c.Kind == richtext.Task)
}

func sameContainer(a, b richtext.Container) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Footnote == b.Footnote
}

// sharedContainers 返回两条容器链从外层起相同的层数。
func sharedContainers(a, b []richtext.Container) int {
	n := 0
	for n < len(a) && n < len(b) && sameContainer(a[n], b[n]) {
		n++
	}
	return n
}

// nextListItem 判断下一块是否是与当前块同属一个列表（同级、子级或回到外层）的新列表项。
func nextListItem(chain, next []richtext.Container, shared int) bool {
	if len(next) != shared+1 || !isListContainer(next[shared]) {
		return false
	}
	if len(chain) > shared {
		return isListContainer(chain[shared])
	}
	return shared > 0 && isListContainer(chain[shared-1])
}

// ambient 给出容器内正文的字号、行高倍率、字体与是否用次要色，取决于最近的引用或提示块。
func ambient(chain []richtext.Container, fontSize float32) (size, ratio float32, family string, muted bool) {
	size = fontSize
	foot := false
	for k := len(chain) - 1; k >= 0; k-- {
		switch c := chain[k]; {
		case c.Footnote != "":
			foot = true
		case c.Kind == richtext.Callout:
			return 13, 1.5, calloutFont, false
		case c.Kind == richtext.Quote:
			return size, 1.7, "", true
		}
	}
	if foot {
		size = fontSize * footScale
	}
	return size, 0, "", false
}

// footnoteNumbers 按引用在正文中首次出现的顺序编号，没有被引用的定义排在其后。
func footnoteNumbers(blocks []richtext.Block) map[string]int {
	var numbers map[string]int
	add := func(label string) {
		if label == "" || numbers[label] != 0 {
			return
		}
		if numbers == nil {
			numbers = map[string]int{}
		}
		numbers[label] = len(numbers) + 1
	}
	visit := func(runs []richtext.Run) {
		for _, r := range runs {
			if r.Link != nil {
				add(r.Link.Footnote)
			}
		}
	}
	for _, b := range blocks {
		visit(b.Runs)
		if b.Table != nil {
			for _, row := range b.Table.Rows {
				for _, cell := range row {
					visit(cell.Runs)
				}
			}
		}
	}
	for _, b := range blocks {
		if b.Footnote != nil {
			add(b.Footnote.Label)
		}
	}
	return numbers
}

// markerWidth 是列表项标记占掉的正文左侧宽度，与 placeText 的取值一致。
func markerWidth(c richtext.Container, fontSize float32, cache *shapeCache) float32 {
	switch {
	case c.Kind == richtext.Task:
		return 28
	case c.Ordered:
		return measure(cache, strconv.Itoa(max(1, c.Start))+". ", fontSize, false, false, false)
	default:
		return 12
	}
}

// open 开始一个容器：记下装饰的起点，返回容器内的留白和它占掉的顶部高度。
func (lay *layout) open(c richtext.Container, parent *openFrame, first richtext.Block, afterFootnote bool, baseX, inner, y, fontSize, size float32, cache *shapeCache) (openFrame, float32) {
	of := openFrame{c: c, frame: -1}
	if parent != nil {
		of.left, of.right = parent.left, parent.right
	}
	f := frame{kind: c.Kind, x: baseX + of.left, y: y, w: max(inner-of.left-of.right, 40)}
	top := float32(0)
	switch {
	case c.Footnote != "":
		f.foot = c.Footnote
		f.rule = !afterFootnote
		f.label = cache.shape(strconv.Itoa(max(1, cache.footnotes[c.Footnote]))+".", fontSize*footScale, false, false, false)
		f.labelW = advanceOf(f.label)
		of.left += max(f.labelW+6, 22)
		of.anchor = true
	case c.Kind == richtext.Quote:
		of.left += quoteInset
		of.right += 16
		top = 10
	case c.Kind == richtext.Callout && detailsContainer(c):
		f.details, f.foldID = true, c.ID
		f.folded = c.Callout.Fold != "+"
		if cache.collapsed != nil {
			if on, ok := cache.collapsed[c.ID]; ok {
				f.folded = on
			}
		}
		if c.Callout.Title != "" {
			font := ui.Font{Family: calloutFont, Size: 13}
			f.title, f.titleAscent = ui.Shape(c.Callout.Title, font), font.Metrics().Ascent
		}
		top = 32
		if f.folded {
			of.left += 10000
		} else {
			of.left += 32
			of.right += 12
		}
	case c.Kind == richtext.Callout:
		if c.Callout != nil && c.Callout.Title != "" {
			font := ui.Font{Family: calloutFont, Size: 13}
			f.title, f.titleAscent = ui.Shape(c.Callout.Title, font), font.Metrics().Ascent
			top = 23
		}
		of.left += 37
		of.right += 12
		top += 12
	case isListContainer(c):
		of.markerLeft = of.left
		if parent != nil && isListContainer(parent.c) {
			of.markerLeft = parent.markerLeft + nestIndent
		}
		f.x = baseX + of.markerLeft
		of.left = of.markerLeft + markerWidth(c, size, cache)
		// 首块不是正文（代码块、子列表等）时没有块来画标记，由装饰自己画。
		if !isListItem(first) || !sameContainer(first.Containers[len(first.Containers)-1], c) {
			f.marker, f.checked, f.bullet = true, c.Checked, c.Kind == richtext.List && !c.Ordered
			if c.Kind == richtext.List && c.Ordered {
				f.label = cache.shape(strconv.Itoa(max(1, c.Start))+". ", size, false, false, false)
			}
			of.anchor = true
		}
	}
	if c.Footnote != "" || !isListContainer(c) || f.marker {
		lay.frames = append(lay.frames, f)
		of.frame = len(lay.frames) - 1
	}
	return of, top
}

// anchor 记下容器首块的首行位置，供编号和标记对齐。
func (lay *layout) anchor(index int, box blockBox, fontSize float32) {
	if index < 0 {
		return
	}
	f := &lay.frames[index]
	f.lineY, f.lineH, f.ascent = box.y, fontSize*1.4, fontSize
	if len(box.lines) > 0 {
		ln := lay.lines[box.lines[0]]
		f.lineY, f.lineH, f.ascent = ln.y, ln.height, ln.ascent
	}
}

// close 结束一个容器，返回它占掉的底部高度。last 是容器里的最后一块。
func (lay *layout) close(of openFrame, last blockBox, y, fontSize float32, cache *shapeCache) float32 {
	if of.frame < 0 {
		return 0
	}
	f := &lay.frames[of.frame]
	bottom := float32(0)
	switch {
	case f.foot != "":
		// 回跳箭头跟在定义最后一行的末尾。
		if len(last.lines) > 0 {
			ln := lay.lines[last.lines[len(last.lines)-1]]
			f.back = cache.shape("↩", fontSize*footScale, false, false, false)
			f.backX, f.backBase = last.x+advanceEnd(ln)+6, ln.y+ln.ascent
			f.backRect = ui.Rect{X: f.backX - 3, Y: ln.y, W: advanceOf(f.back) + 6, H: ln.height}
		}
	case f.kind == richtext.Quote:
		bottom = 10
	case f.kind == richtext.Callout && f.details && f.folded:
		bottom = 0
	case f.kind == richtext.Callout:
		bottom = 12
	}
	f.h = y + bottom - f.y
	return bottom
}

// detailsContainer 只认 HTML 折叠块，普通提示块的 Fold 不触发折叠。
func detailsContainer(c richtext.Container) bool {
	return c.Kind == richtext.Callout && c.IsHTML() && c.Callout != nil && c.Callout.Type == "details"
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
	underline, kbd      bool
	link                string
	image               string // 行内图片地址
	footnote            string // 脚注引用，text 是标签
	math                bool   // 行内公式，text 是 TeX 源码
	partial             bool   // 已被拆到下一行的剩余部分
}

// htmlMarks 取出只有 HTML 标签才能表达的行内样式：下划线与按键提示。
func htmlMarks(marks richtext.Mark) (underline, kbd bool) {
	return marks&richtext.MarkUnderline != 0, marks&richtext.MarkKbd != 0
}

func runsToStyled(runs []richtext.Run) []styled {
	out := make([]styled, 0, len(runs))
	for _, r := range runs {
		if r.Text == "" {
			continue
		}
		s := styled{text: r.Text, bold: r.Marks&richtext.MarkBold != 0, italic: r.Marks&(richtext.MarkItalic|richtext.MarkMath) != 0, math: r.Marks&richtext.MarkMath != 0, code: r.Marks&richtext.MarkCode != 0, strike: r.Marks&richtext.MarkStrike != 0, highlight: r.Marks&richtext.MarkHighlight != 0, sub: r.Marks&richtext.MarkSub != 0, sup: r.Marks&richtext.MarkSup != 0}
		s.underline, s.kbd = htmlMarks(r.Marks)
		// 按键提示用等宽字体，外面再画一个键帽边框。
		s.code = s.code || s.kbd
		if r.Link != nil {
			s.link, s.footnote = r.Link.URL, r.Link.Footnote
		}
		if r.Image != nil {
			s.image = r.Image.URL
			if r.Image.Alt != "" {
				s.text = r.Image.Alt
			}
		}
		out = append(out, s)
	}
	return out
}

// placeText 排一个文字块。ratio 大于 0 时覆盖正文的行高倍率，供引用与提示块容器使用。
func placeText(box *blockBox, b richtext.Block, inner, fontSize, ratio float32, cache *shapeCache, lay *layout) float32 {
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
		cache.family = calloutFont
		defer func() { cache.family = oldFamily }()
		if b.Callout != nil && b.Callout.Title != "" {
			box.title = ui.Shape(b.Callout.Title, ui.Font{Family: calloutFont, Size: 13})
			box.titleAscent = ui.Font{Family: calloutFont, Size: 13}.Metrics().Ascent
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
			if seg.image != "" && !seg.partial {
				alt := seg.text
				if alt == "" || alt == "\ufffc" {
					alt = "图片"
				}
				g := cache.shape(alt, size*.9, false, false, false)
				w := advanceOf(g) + 8
				if bm := cache.images[seg.image]; bm != nil {
					pw, ph := bm.Size()
					h := size * 1.15
					if ph > 0 {
						w = h * float32(pw) / float32(ph)
					}
					if w > 160 {
						w = 160
					}
					if w < 12 {
						w = 12
					}
					g = nil
				}
				if x-inset+w > width && built.Len() > 0 {
					break
				}
				shiftGlyphs(g, x+4)
				hold := []ui.Glyph{{Cluster: utf8.RuneCountInString(built.String()), Runes: len(rs), X: x, Advance: w}}
				ln.spans = append(ln.spans, paintSpan{glyphs: hold, image: seg.image, display: g})
				ln.glyphs = append(ln.glyphs, hold...)
				x += w
				built.WriteString(seg.text)
				pending = pending[1:]
				continue
			}
			if seg.footnote != "" && !seg.partial {
				// 脚注引用显示成上标编号，整个标签是一个不可拆分的盒子。
				number := cache.shape(strconv.Itoa(max(1, cache.footnotes[seg.footnote])), size*.75, false, false, false)
				w := advanceOf(number) + 2
				if x-inset+w > width && built.Len() > 0 {
					break
				}
				shiftGlyphs(number, 1)
				g := []ui.Glyph{{Cluster: utf8.RuneCountInString(built.String()), Runes: len(rs), X: x, Advance: w}}
				ln.spans = append(ln.spans, paintSpan{glyphs: g, dy: -size * .35, footnote: seg.footnote, display: number})
				ln.glyphs = append(ln.glyphs, g...)
				x += w
				built.WriteString(seg.text)
				pending = pending[1:]
				continue
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
			ln.spans = append(ln.spans, paintSpan{glyphs: g, dy: dy, highlight: seg.highlight, code: seg.code, strike: seg.strike, underline: seg.underline, kbd: seg.kbd, link: seg.link})
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
		lineRatio := cache.lineHeight
		if ratio > 0 {
			lineRatio = ratio
		}
		if b.Kind == richtext.Heading {
			lineRatio = 1.4
			if b.Level == 1 {
				lineRatio = 1.3
			}
		}
		if b.Kind == richtext.Quote {
			lineRatio = 1.7
		}
		if b.Kind == richtext.Callout {
			lineRatio = 1.5
		}
		ln.height = max(size*lineRatio, m.Ascent+m.Descent)
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
	// 整段放得下时不必二分：多数行都走这里，也不会往字形缓存里塞一串前缀。
	if measure(cache, string(rs), styledSize(seg, size), headingBold || seg.bold, seg.italic, seg.code) <= width {
		return len(rs)
	}
	// 二分找到放得下的最长前缀，并尽量在空白处断开。
	lo, hi := 0, len(rs)-1
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

func placeCode(box *blockBox, code, lang string, fontSize float32, cache *shapeCache, lay *layout, source bool) float32 {
	size := float32(14)
	if source {
		size = fontSize
	}
	m := cache.metrics(size, false, true)
	lineH := max(size*1.75, m.Ascent+m.Descent)
	if source && cache.lineHeight > 0 {
		lineH = max(size*cache.lineHeight, m.Ascent+m.Descent)
	}
	parts := strings.Split(code, "\n")
	// 代码正文以换行结尾时，末尾的空行不单独占一行高度。源码模式保留它，光标才能停在文末换行之后。
	if !source && len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	y := float32(14)
	pad := float32(codePad)
	wrap := float32(0)
	if source {
		// 源码是整篇正文，不留代码块的内边距，超宽的行按栏宽折行。
		y, pad = 0, 8
		wrap = box.w - pad*2
		if wrap < 40 {
			wrap = 40
		}
	}
	runesBefore := 0
	for _, part := range parts {
		pieces := []string{part}
		if source {
			pieces = wrapSource(cache, part, wrap, size)
		}
		for pi, piece := range pieces {
			ln := line{block: box.index, y: box.y + y, ascent: m.Ascent + (lineH-m.Ascent-m.Descent)/2, height: lineH, prefix: pad, text: piece, origin: box.origin + runesBefore, count: len([]rune(piece)), newline: pi == len(pieces)-1}
			g := cache.shape(piece, size, false, false, true)
			shiftGlyphs(g, pad)
			ln.glyphs = g
			if source {
				ln.spans = sourceSpans(g, ln.origin, cache.sourceKinds)
			} else {
				ln.spans = codeSpans(g, piece, lang)
			}
			ln.width = advanceOf(g)
			lay.lines = append(lay.lines, ln)
			box.lines = append(box.lines, len(lay.lines)-1)
			y += lineH
			runesBefore += ln.count
			if pi == len(pieces)-1 {
				runesBefore++
			}
		}
	}
	if n := len(lay.lines); n > 0 && !strings.HasSuffix(code, "\n") {
		lay.lines[n-1].newline = false
	}
	if source {
		return y + 8
	}
	return y + 14
}

// wrapSource 把一行源码按栏宽折成多段，不拆 rune 簇。空行保留成一段。
func wrapSource(cache *shapeCache, part string, width, size float32) []string {
	rs := []rune(part)
	if len(rs) == 0 {
		return []string{""}
	}
	var out []string
	start := 0
	for start < len(rs) {
		rest := rs[start:]
		fit := fitRunes(cache, styled{text: string(rest), code: true}, rest, width, size, false)
		if fit <= 0 || fit > len(rest) {
			fit = 1
		}
		out = append(out, string(rest[:fit]))
		start += fit
	}
	return out
}

// sourceKindTable 把着色区间展开成每个 rune 的种类。区间重叠时先出现的生效，与逐个区间查找一致。
func sourceKindTable(spans []SourceSpan, length int) []uint8 {
	if len(spans) == 0 || length <= 0 {
		return nil
	}
	kinds := make([]uint8, length)
	for i := len(spans) - 1; i >= 0; i-- {
		sp := spans[i]
		for at := max(sp.Start, 0); at < min(sp.End, length); at++ {
			kinds[at] = sp.Kind
		}
	}
	return kinds
}

// sourceSpans 按着色表给一行源码着色。origin 是这一行在全文里的 rune 偏移，种类为 0 的不单独成段。
func sourceSpans(glyphs []ui.Glyph, origin int, kinds []uint8) []paintSpan {
	if len(kinds) == 0 || len(glyphs) == 0 {
		return nil
	}
	var out []paintSpan
	for _, g := range glyphs {
		kind := uint8(0)
		if at := origin + g.Cluster; at >= 0 && at < len(kinds) {
			kind = kinds[at]
		}
		if n := len(out); n > 0 && out[n-1].source == kind {
			out[n-1].glyphs = append(out[n-1].glyphs, g)
			continue
		}
		out = append(out, paintSpan{glyphs: []ui.Glyph{g}, source: kind})
	}
	return out
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
			h := placeText(&cell, richtext.Block{Kind: richtext.Paragraph, Runs: runs}, cell.w, fontSize, 0, cache, lay)
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
	case strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n"):
		return "文档属性 · 原文保留"
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
	case strings.HasPrefix(s, "[") && strings.Contains(s, "]:"):
		return "链接定义 · 原文保留"
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
