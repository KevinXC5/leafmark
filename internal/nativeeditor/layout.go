package nativeeditor

import (
	"strconv"
	"strings"

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
	innerGap     = 12 // 同一容器内相邻块的间距
	listItemGap  = 4  // 同级列表项在默认字号 15 下的额外间距
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
	image       *Picture
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

// flow 按当前宽度把文档排成行。contentW 是可视内容宽度。
func flow(doc *richtext.Document, contentW, fontSize float32, cache *shapeCache, images map[string]*Picture) layout {
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
				// 同级项略微拉开，子列表仍紧挨父项；不同顶层列表保留块间距。
				if isListItem(blocks[i+1]) && (blocks[i+1].Level > 1 || blocks[i+1].Ordered == topOrdered) {
					gap = listItemGap * fontSize / 15
					if blocks[i+1].Level > b.Level {
						gap = 0
					}
				}
			}
		case nextListItem(chain, next, shared):
			// 同级项留出小幅间距，多段项沿用较大的间距；进入子列表不额外留白。
			gap = looseGap
			if marker {
				gap = listItemGap * fontSize / 15
			}
			if len(next) > len(chain) {
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
		seen = true
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
