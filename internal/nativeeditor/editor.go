package nativeeditor

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// ReadImage 通过桌面的授权服务读取 Markdown 图片路径。控件不读取任意本地文件。
// 返回 nil 表示无法显示，图片块仍保留替代文字。
type ReadImage = func(markdownPath string) *ui.Bitmap

// ReadPicture 与 ReadImage 相同，但也能返回矢量图。
type ReadPicture = func(markdownPath string) *Picture

// Editor 是所见即所得编辑控件。画面只显示排版结果，不显示 Markdown 标记。
type Editor struct {
	doc            *richtext.Document
	anchor         int
	focus          int
	fontSize       float32
	read           ReadPicture
	pending        richtext.Mark // 折叠光标上待输入的格式
	compose        string
	comCaret       int
	dragging       bool
	scroll         ui.ScrollState
	reveal         bool
	cache          *shapeCache
	images         map[string]*Picture
	fetched        map[string]struct{}
	remoteN        int
	remoteB        int
	imgCh          chan imageResult
	client         *http.Client
	lay            layout
	width          float32
	viewH          float32
	wakeMu         sync.RWMutex
	wake           func()
	composeGlyphs  []ui.Glyph
	composeMetrics ui.FontMetrics
	composeAdvance float32
	dirty          bool // Changed 尚未被读取
	wordMod        ui.Modifiers
	wantFocus      bool // 下一帧把键盘焦点交还给正文
	focusMode      bool // 淡化光标所在块之外的内容
	editRaw        func(index int, source string)
	typewriter     bool    // 光标所在行保持在视口中部
	revealed       bool    // 滚动位置来自上一次光标定位
	revealY        float32 // 上一次定位得到的纵向滚动
	readOnly       bool    // 阅读模式：只能选择、复制、查找和点击链接
	openLink       func(url string)
	hand           bool // 指针下是可点击内容，显示小手
	pressHot       hot  // 按下时指针下的可点击内容，松开时仍在其上且没有拖选才触发
	// 指针最近一次在视口内的位置与修饰键：滚动、重排或切换模式后内容在指针下变了，按它重算指针形状。
	hoverX, hoverY float32
	hoverMods      ui.Modifiers
	hovered        bool
	sourceMode     bool // 整篇是一块等宽纯文本，Markdown 原样返回
	sourceSyntax   func(string) []SourceSpan
	sourcePalette  [6]ui.Color
	sourceSet      bool
	sourceKey      sourceLayoutKey // 上一次源码排版的输入，未变时复用 sourceLay
	sourceLay      layout
	sourceLaid     bool
	bounds         ui.Rect      // 最近一帧 Scroll 元素在父容器中的矩形
	collapsed      map[int]bool // details 容器的视图折叠，不写入文档
}

// New 用 Markdown 创建一个编辑器。
func New(markdown string) *Editor {
	return &Editor{
		doc:       richtext.Parse(markdown),
		collapsed: map[int]bool{},
		fontSize:  15,
		cache:     newShapeCache(),
		images:    map[string]*Picture{},
		fetched:   map[string]struct{}{},
		imgCh:     make(chan imageResult, 8),
		client:    &http.Client{Timeout: 8 * time.Second},
		wordMod:   wordModifier(),
	}
}

// SetReadImage 设置 Markdown 图片路径的授权读取回调。
func (e *Editor) SetReadImage(fn func(string) *ui.Bitmap) {
	if fn == nil {
		e.read = nil
		return
	}
	e.read = func(path string) *Picture { return BitmapPicture(fn(path)) }
}

// ImageReady 报告某个图片地址是否已经取得可绘制的图片。
func (e *Editor) ImageReady(url string) bool { return e.images[url] != nil }

// SetReadPicture 设置能返回矢量图的授权读取回调，与 SetReadImage 二选一。
func (e *Editor) SetReadPicture(fn func(string) *Picture) { e.read = fn }

// SetInvalidate 设置后台图片完成时的线程安全窗口刷新回调。
func (e *Editor) SetInvalidate(fn func()) {
	e.wakeMu.Lock()
	e.wake = fn
	e.wakeMu.Unlock()
}

// Markdown 返回当前文档的 Markdown。源码模式就是纯文本，含标记和末尾换行。
func (e *Editor) Markdown() string {
	if e.sourceMode {
		return e.doc.Text()
	}
	return e.doc.Markdown()
}

// Changed 报告自上次调用后是否有编辑，并清掉标记。
func (e *Editor) Changed() bool {
	d := e.dirty
	e.dirty = false
	if d {
	}
	return d
}

// FontSize 设置正文字号（DIP）。
func (e *Editor) FontSize(size float32) {
	if size < 8 {
		size = 8
	}
	if size > 96 {
		size = 96
	}
	e.fontSize = size
	e.cache.reset()
	clear(e.cache.faces)
}

// SetFontFamily 设置正文的系统字体回退列表，塑形、命中与组合输入共享此字体。
func (e *Editor) SetFontFamily(family string) {
	if strings.TrimSpace(family) == "" {
		family = defaultFontFamily
	}
	if e.cache.family == family {
		return
	}
	e.cache.family = family
	clear(e.cache.faces)
	e.cache.reset()
}

// SetLineHeight 设置正文行高倍率。
func (e *Editor) SetLineHeight(ratio float32) {
	if ratio >= 1 && ratio <= 3 {
		e.cache.lineHeight = ratio
	}
}

// SetReadingWidth 设置正文栏的最大宽度。
func (e *Editor) SetReadingWidth(width float32) {
	if width >= 320 && width <= 2000 {
		e.cache.readingWidth = width
	}
}

// SetFocusMode 开关专注模式：光标所在块之外的内容淡化显示。
func (e *Editor) SetFocusMode(on bool) { e.focusMode = on }

// SetTypewriter 开关打字机模式：移动光标或输入时，所在行保持在视口中部。
func (e *Editor) SetTypewriter(on bool) {
	if e.typewriter != on {
		e.typewriter, e.reveal = on, on
	}
}

// Undo 撤销。
func (e *Editor) Undo() {
	if e.readOnly {
		return
	}
	if sel, ok := e.doc.Undo(); ok {
		e.anchor, e.focus = sel.Start, sel.End
		e.pending = 0
		e.dirty = true
		e.reveal = true
	}
}

// Redo 重做。
func (e *Editor) Redo() {
	if e.readOnly {
		return
	}
	if sel, ok := e.doc.Redo(); ok {
		e.anchor, e.focus = sel.Start, sel.End
		e.pending = 0
		e.dirty = true
		e.reveal = true
	}
}

// Format 切换行内样式或块类型。
// 行内：bold、italic、code、strike。
// 块级：paragraph、heading（1–6 用 heading1…heading6）、quote、bullet、ordered、task。
func (e *Editor) Format(name string) {
	if e.readOnly {
		return
	}
	if mark, ok := markByName(name); ok {
		e.doc.ToggleMark(e.selection(), mark)
		e.dirty = true
		if e.anchor == e.focus {
			e.pending = e.doc.MarksAt(e.focus)
		}
		return
	}
	if !e.insertBlock(name) && !e.formatBlock(name) {
		return
	}
	e.dirty = true
	e.reveal = true
}

func (e *Editor) formatBlock(name string) bool {
	blocks := e.doc.Blocks()
	bi := e.doc.BlockIndexAt(e.selection().Start)
	if bi < 0 || bi >= len(blocks) {
		return false
	}
	if blocks[bi].Kind != richtext.Paragraph && blocks[bi].Kind != richtext.Heading && blocks[bi].Kind != richtext.Quote && blocks[bi].Kind != richtext.List && blocks[bi].Kind != richtext.Task {
		return false
	}
	switch name {
	case "paragraph", "heading", "heading1", "heading2", "heading3", "heading4", "heading5", "heading6", "quote", "bullet", "list", "ordered", "task":
	default:
		return false
	}
	kind, level := richtext.Paragraph, 0
	switch name {
	case "heading", "heading1":
		kind, level = richtext.Heading, 1
	case "heading2":
		kind, level = richtext.Heading, 2
	case "heading3":
		kind, level = richtext.Heading, 3
	case "heading4":
		kind, level = richtext.Heading, 4
	case "heading5":
		kind, level = richtext.Heading, 5
	case "heading6":
		kind, level = richtext.Heading, 6
	case "quote":
		kind, level = richtext.Quote, 1
	case "bullet", "list":
		kind, level = richtext.List, 1
	case "ordered":
		kind, level = richtext.List, 1
	case "task":
		kind, level = richtext.Task, 1
	}
	// 再点一次同样的块类型，回到正文段落。
	cur := blocks[bi]
	if cur.Kind == kind && (kind != richtext.Heading || cur.Level == level) && (kind != richtext.List || cur.Ordered == (name == "ordered")) && name != "paragraph" {
		e.doc.SetBlock(e.selection(), richtext.Paragraph, 0)
		return true
	}
	if kind == richtext.List || kind == richtext.Task || kind == richtext.Quote {
		// 已在列表里时保留原有层级，只换类型。
		if isListItem(cur) && kind != richtext.Quote {
			level = max(1, cur.Level)
		}
	}
	e.doc.SetBlock(e.selection(), kind, level)
	if kind == richtext.List {
		e.doc.SetOrdered(bi, name == "ordered")
	}
	return true
}

// InsertLink 插入链接。地址不安全时只插入文字。
func (e *Editor) InsertLink(label, url string) {
	if e.readOnly {
		return
	}
	if !safeURL(url, false) {
		e.replaceSelection(label)
		return
	}
	sel := e.doc.InsertLinkSelection(e.selection(), label, url)
	e.anchor, e.focus = sel.Start, sel.End
	e.dirty = true
	e.reveal = true
}

// InsertImage 在光标处插入独立图片块。
func (e *Editor) InsertImage(alt, path string) {
	if e.readOnly {
		return
	}
	sel := e.doc.InsertImage(e.focus, alt, path)
	e.anchor, e.focus = sel.Start, sel.End
	e.dirty = true
	e.reveal = true
	e.wantImage(path)
}

// Find 选中下一处 query，找到返回 true。公式、脚注和原文占位里的匹配会被跳过。
func (e *Editor) Find(query string) bool {
	if query == "" {
		return false
	}
	at := e.nextEditable(query, e.focus, e.anchor != e.focus)
	if at < 0 {
		return false
	}
	e.anchor, e.focus = at, at+len([]rune(query))
	e.reveal = true
	return true
}

func indexFrom(text, q []rune, from int) int {
	if from < 0 {
		from = 0
	}
	if len(q) == 0 || from > len(text) {
		return -1
	}
	for i := from; i+len(q) <= len(text); i++ {
		match := true
		for j := range q {
			if text[i+j] != q[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// Selection 返回当前选区（rune，半开区间）。
func (e *Editor) Selection() (start, end int) {
	s := e.selection()
	return s.Start, s.End
}

// SetSelection 设置选区，供验收定位光标。
func (e *Editor) SetSelection(start, end int) {
	n := e.doc.Len()
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > n {
		start = n
	}
	if end > n {
		end = n
	}
	e.anchor, e.focus = start, end
	e.reveal = true
}

func (e *Editor) selection() richtext.Selection {
	if e.anchor <= e.focus {
		return richtext.Selection{Start: e.anchor, End: e.focus}
	}
	return richtext.Selection{Start: e.focus, End: e.anchor}
}

// View 构建一帧。编辑器占满父容器并可滚动。
func (e *Editor) View(c *ui.Context) {
	// 不铺底色：纸面由窗口提供，侧栏投影才能落到正文区。
	box := ui.ScrollBoth(c).Fill().MinHeight(0).Focusable()
	bounds := box.Bounds()
	width, height := bounds.W, bounds.H
	if width <= 0 || height <= 0 {
		width, height = c.Size()
	}
	resized := abs32(width-e.width) > .5 || abs32(height-e.viewH) > .5
	if resized {
		c.Invalidate()
	}
	e.width, e.viewH = width, height
	e.bounds = bounds
	e.drainImages(c)
	e.ensureImages()
	e.cache.caret = -1
	if e.anchor == e.focus && e.compose == "" && !e.readOnly {
		e.cache.caret = e.focus
	}
	e.cache.source = e.sourceMode
	e.cache.collapsed = e.collapsed
	e.cache.diagramStyle = DiagramStyle(c.Theme())
	e.lay = e.reflow(width)
	if e.typewriter {
		// 文末留出余量，最后几行也能停在视口中部。
		e.lay.height += height * .45
	}
	// 指针形状属于整个元素：按指针下的内容算出是否显示小手，每帧交给元素。
	if e.hovered {
		e.updatePointer(e.hoverX+e.scroll.X, e.hoverY+e.scroll.Y, e.hoverMods)
	}
	pointer := ui.CursorText
	if e.hand {
		pointer = ui.CursorPointer
	}
	box.Cursor(pointer)
	// 组合输入只预排字形，Draw 不写排版缓存或文档。
	e.composeGlyphs = e.cache.shape(e.compose, e.fontSize, false, false, false)
	e.composeMetrics = e.cache.metrics(e.fontSize, false, false)
	caret := min(max(e.comCaret, 0), len([]rune(e.compose)))
	e.composeAdvance = measure(e.cache, string([]rune(e.compose)[:caret]), e.fontSize, false, false, false)
	if e.reveal {
		if e.expandToCaret() {
			e.lay = e.reflow(width)
		}
		e.keepCaretVisible()
		e.reveal = false
		e.revealed, e.revealY = true, e.scroll.Y
	} else if resized && e.revealed && e.scroll.Y == e.revealY {
		// 上次定位用的是旧视口尺寸（新建编辑器的首帧还没有真实边界）。
		// 用户没有自己滚动过时按新尺寸重算，不展开折叠块，也不打断手动滚动。
		e.keepCaretVisible()
		e.revealY = e.scroll.Y
	}
	if e.wantFocus {
		box.Focus()
		e.wantFocus = false
	}
	// 输入与菜单回调可在构建结束后执行，只捕获不受帧生命周期限制的窗口服务。
	services := c.Services()
	box.TrackScroll(&e.scroll).HandleInput(e.onInput(services)).ContextMenu(func(m *ui.Menu) { e.contextMenu(services, m) })
	cr := e.caretRect()
	box.TextCaret(ui.Rect{X: cr[0] - e.scroll.X, Y: cr[1] - e.scroll.Y, W: 1, H: cr[3]})
	theme, now := c.Theme(), c.Now()
	box.Children(func() {
		ui.Box(c).Width(max(width, e.lay.width)).Height(e.lay.height).MinHeight(e.lay.height).Draw(func(p *ui.Painter, r ui.Rect) {
			e.paint(p, r, theme, now)
		})
	})
}

// sourceLayoutKey 是源码排版的全部输入。源码整篇是一块等宽纯文本，
// 版面只取决于正文与这些版面参数，不受光标、主题和图片影响。
type sourceLayoutKey struct {
	text                                    string
	width, fontSize, lineHeight, readingCol float32
	family                                  string
	weight                                  int
	syntax                                  bool
}

// reflow 按当前宽度排版。源码模式下正文和版面参数没变时复用上一次的结果，
// 滚动和光标闪烁的帧不必重新着色、折行。返回的排版与缓存共用切片，调用方只读。
func (e *Editor) reflow(width float32) layout {
	if !e.sourceMode {
		e.cache.sourceSpans, e.cache.sourceKinds = nil, nil
		return flow(e.doc, width, e.fontSize, e.cache, e.images)
	}
	key := sourceLayoutKey{text: e.doc.Text(), width: width, fontSize: e.fontSize, lineHeight: e.cache.lineHeight, readingCol: e.cache.readingWidth, family: e.cache.family, weight: e.cache.weight, syntax: e.sourceSyntax != nil}
	if e.sourceLaid && key == e.sourceKey {
		return e.sourceLay
	}
	e.cache.sourceSpans, e.cache.sourceKinds = nil, nil
	if e.sourceSyntax != nil {
		e.cache.sourceSpans = e.sourceSyntax(key.text)
		e.cache.sourceKinds = sourceKindTable(e.cache.sourceSpans, e.doc.Len())
	}
	e.sourceLay = flow(e.doc, width, e.fontSize, e.cache, e.images)
	e.sourceKey, e.sourceLaid = key, true
	return e.sourceLay
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// HandleInput 处理一次输入。剪贴板命令需要 c，没有时忽略复制粘贴。
// 原生验收和 ui.Tester 都走这个入口。
func (e *Editor) HandleInput(c *ui.Context, ev ui.InputEvent) bool {
	return e.onInput(c.Services())(ev)
}

// Text 返回当前可见纯文本。
func (e *Editor) Text() string { return e.doc.Text() }

// PointForOffset 返回文档偏移对应的内容坐标（光标左上角与行高）。
func (e *Editor) PointForOffset(offset int) (x, y, h float32) {
	n := e.doc.Len()
	if offset < 0 {
		offset = 0
	}
	if offset > n {
		offset = n
	}
	if e.width < 120 {
		e.lay = flow(e.doc, 720, e.fontSize, e.cache, e.images)
		e.width = 720
	}
	li := e.lay.lineAt(offset)
	if li < 0 {
		return padX, padY, e.fontSize
	}
	ln := e.lay.lines[li]
	b := e.blockBox(ln.block)
	return b.x + ln.caretX(offset), ln.y, ln.height
}

// Focus 让正文在下一帧取回键盘焦点，供工具栏操作后继续输入。
func (e *Editor) Focus() {
	e.wantFocus = true
}

// SelectionAnchor 返回非空选区起点在编辑区视口内的位置，供浮动格式栏定位。
// 拖选尚未结束、输入法组合中、选区滚出视口或处于阅读模式时 ok 为 false。
func (e *Editor) SelectionAnchor() (x, y, lineH, viewW float32, ok bool) {
	sel := e.selection()
	if sel.Start == sel.End || e.dragging || e.compose != "" || e.width <= 0 || e.readOnly {
		return 0, 0, 0, 0, false
	}
	x, y, lineH = e.PointForOffset(sel.Start)
	x -= e.scroll.X
	y -= e.scroll.Y
	if y+lineH < 0 || y > e.viewH {
		return 0, 0, 0, 0, false
	}
	return x, y, lineH, e.width, true
}

// BlockRect 返回块的内容矩形。越界时 ok 为 false。
func (e *Editor) BlockRect(index int) (x, y, w, h float32, ok bool) {
	if e.lay.width == 0 {
		e.lay = flow(e.doc, 720, e.fontSize, e.cache, e.images)
	}
	if index < 0 || index >= len(e.lay.blocks) {
		return 0, 0, 0, 0, false
	}
	b := e.lay.blocks[index]
	return b.x, b.y, b.w, b.h, true
}

func markByName(name string) (richtext.Mark, bool) {
	switch name {
	case "bold":
		return richtext.MarkBold, true
	case "italic":
		return richtext.MarkItalic, true
	case "code":
		return richtext.MarkCode, true
	case "strike":
		return richtext.MarkStrike, true
	default:
		return 0, false
	}
}

// safeURL 拒绝 javascript: 等危险协议。image 为真时只允许 http(s)。
func safeURL(url string, image bool) bool {
	u := strings.TrimSpace(url)
	if u == "" || strings.ContainsAny(u, "\n\r\t\\") {
		return false
	}
	for _, r := range u {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "data:") {
		return !image && strings.HasPrefix(low, "data:image/")
	}
	scheme, rest, ok := strings.Cut(u, ":")
	if !ok || strings.ContainsAny(scheme, "/?#") {
		return !image
	}
	_ = rest
	switch strings.ToLower(scheme) {
	case "http", "https":
		return true
	case "mailto", "tel":
		return !image
	default:
		return false
	}
}

// LayoutSnapshot 返回最近一帧的只读排版数据，坐标是滚动前的内容坐标。
type LayoutSnapshot struct {
	Width, Height, ViewHeight, ScrollX, ScrollY float32
	Lines                                       []LineSnapshot
	Images                                      []ImageSnapshot
}

// ImageSnapshot 记录图片实际显示尺寸与所在正文块的可用宽度。
type ImageSnapshot struct {
	Width, Height, ContentWidth float32
	TextRuns                    int
}
type LineSnapshot struct {
	Text          string
	Origin, Count int
	X, Y, Height  float32
}

func (e *Editor) LayoutSnapshot() LayoutSnapshot {
	out := LayoutSnapshot{Width: e.lay.width, Height: e.lay.height, ViewHeight: e.viewH, ScrollX: e.scroll.X, ScrollY: e.scroll.Y}
	for _, ln := range e.lay.lines {
		out.Lines = append(out.Lines, LineSnapshot{Text: ln.text, Origin: ln.origin, Count: ln.count, X: e.blockBox(ln.block).x + ln.prefix, Y: ln.y, Height: ln.height})
		for _, span := range ln.spans {
			pic := e.images[span.image]
			if pic == nil || len(span.glyphs) == 0 {
				continue
			}
			w, h := pic.Size()
			scale := min(span.glyphs[0].Advance/float32(w), (ln.height-2)/float32(h))
			out.Images = append(out.Images, ImageSnapshot{Width: float32(w) * scale, Height: float32(h) * scale, ContentWidth: e.blockBox(ln.block).w, TextRuns: len(pic.texts)})
		}
	}
	for _, b := range e.lay.blocks {
		if b.image == nil {
			continue
		}
		h := b.h - 8
		if b.alt != "" {
			h -= e.fontSize + 6
		}
		w, ih := b.image.Size()
		scale := min(b.w/float32(w), h/float32(ih))
		out.Images = append(out.Images, ImageSnapshot{Width: float32(w) * scale, Height: float32(ih) * scale, ContentWidth: b.w, TextRuns: len(b.image.texts)})
	}
	return out
}

// toggleTaskAt 只有点击首行复选框才切换完成状态，文字区域继续用于选区。
func (e *Editor) toggleTaskAt(x, y float32) bool {
	bi, ok := e.taskAt(x, y)
	if !ok || e.readOnly {
		return false
	}
	e.doc.SetChecked(bi, !e.doc.Blocks()[bi].Checked)
	e.cancelCompose()
	e.dragging = false
	e.dirty = true
	return true
}
