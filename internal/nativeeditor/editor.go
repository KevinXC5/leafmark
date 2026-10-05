package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"image"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

const (
	remoteLimit  = 2 << 20 // 单张远程图最多 2MB
	remoteTotal  = 8 << 20
	remoteActive = 3
)

// ReadImage 通过桌面的授权服务读取 Markdown 图片路径。控件不读取任意本地文件。
// 返回 nil 表示无法显示，图片块仍保留替代文字。
type ReadImage = func(markdownPath string) *ui.Bitmap

// Editor 是所见即所得编辑控件。画面只显示排版结果，不显示 Markdown 标记。
type Editor struct {
	doc            *richtext.Document
	anchor         int
	focus          int
	fontSize       float32
	read           ReadImage
	pending        richtext.Mark // 折叠光标上待输入的格式
	compose        string
	comCaret       int
	dragging       bool
	dragUnit       int
	pressAt        time.Time
	clicks         int
	lastOff        int
	scroll         ui.ScrollState
	reveal         bool
	cache          *shapeCache
	images         map[string]*ui.Bitmap
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
	typewriter     bool // 光标所在行保持在视口中部
}

// New 用 Markdown 创建一个编辑器。
func New(markdown string) *Editor {
	return &Editor{
		doc:      richtext.Parse(markdown),
		fontSize: 15,
		cache:    newShapeCache(),
		images:   map[string]*ui.Bitmap{},
		fetched:  map[string]struct{}{},
		imgCh:    make(chan imageResult, 8),
		client:   &http.Client{Timeout: 8 * time.Second},
		wordMod:  wordModifier(),
	}
}

func wordModifier() ui.Modifiers {
	if runtime.GOOS == "darwin" {
		return ui.Alt
	}
	return ui.Ctrl
}

// SetReadImage 设置 Markdown 图片路径的授权读取回调。
func (e *Editor) SetReadImage(fn func(string) *ui.Bitmap) { e.read = fn }

// SetInvalidate 设置后台图片完成时的线程安全窗口刷新回调。
func (e *Editor) SetInvalidate(fn func()) {
	e.wakeMu.Lock()
	e.wake = fn
	e.wakeMu.Unlock()
}

// Markdown 返回当前文档的 Markdown。
func (e *Editor) Markdown() string { return e.doc.Markdown() }

// Changed 报告自上次调用后是否有编辑，并清掉标记。
func (e *Editor) Changed() bool {
	d := e.dirty
	e.dirty = false
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
	clear(e.cache.text)
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
	clear(e.cache.text)
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
	if sel, ok := e.doc.Undo(); ok {
		e.anchor, e.focus = sel.Start, sel.End
		e.pending = 0
		e.dirty = true
		e.reveal = true
	}
}

// Redo 重做。
func (e *Editor) Redo() {
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
	sel := e.doc.InsertImage(e.focus, alt, path)
	e.anchor, e.focus = sel.Start, sel.End
	e.dirty = true
	e.reveal = true
	e.wantImage(path)
}

// Find 选中下一处 query，找到返回 true。
func (e *Editor) Find(query string) bool {
	if query == "" {
		return false
	}
	text := e.doc.Text()
	runes := []rune(text)
	q := []rune(query)
	from := e.focus
	if e.anchor != e.focus {
		from = max(e.anchor, e.focus)
	}
	if at := indexFrom(runes, q, from); at >= 0 {
		e.anchor, e.focus = at, at+len(q)
		e.reveal = true
		return true
	}
	if at := indexFrom(runes, q, 0); at >= 0 && at < from {
		e.anchor, e.focus = at, at+len(q)
		e.reveal = true
		return true
	}
	return false
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
	box := ui.ScrollBoth(c).Fill().MinHeight(0).Focusable().Cursor(ui.CursorText)
	bounds := box.Bounds()
	width, height := bounds.W, bounds.H
	if width <= 0 || height <= 0 {
		width, height = c.Size()
	}
	if abs32(width-e.width) > .5 || abs32(height-e.viewH) > .5 {
		c.Invalidate()
	}
	e.width, e.viewH = width, height
	e.drainImages(c)
	e.ensureImages()
	e.cache.caret = -1
	if e.anchor == e.focus && e.compose == "" {
		e.cache.caret = e.focus
	}
	e.cache.diagramStyle = diagramStyle(c.Theme())
	e.lay = flow(e.doc, width, e.fontSize, e.cache, e.images)
	if e.typewriter {
		// 文末留出余量，最后几行也能停在视口中部。
		e.lay.height += height * .45
	}
	// 组合输入只预排字形，Draw 不写排版缓存或文档。
	e.composeGlyphs = e.cache.shape(e.compose, e.fontSize, false, false, false)
	e.composeMetrics = e.cache.metrics(e.fontSize, false, false)
	caret := min(max(e.comCaret, 0), len([]rune(e.compose)))
	e.composeAdvance = measure(e.cache, string([]rune(e.compose)[:caret]), e.fontSize, false, false, false)
	if e.reveal {
		e.keepCaretVisible()
		e.reveal = false
	}
	if e.wantFocus {
		box.Focus()
		e.wantFocus = false
	}
	box.TrackScroll(&e.scroll).HandleInput(e.onInput(c)).ContextMenu(func(m *ui.Menu) { e.contextMenu(c, m) })
	cr := e.caretRect()
	box.TextCaret(ui.Rect{X: cr[0] - e.scroll.X, Y: cr[1] - e.scroll.Y, W: 1, H: cr[3]})
	theme, now := c.Theme(), c.Now()
	box.Children(func() {
		ui.Box(c).Width(max(width, e.lay.width)).Height(e.lay.height).MinHeight(e.lay.height).Draw(func(p *ui.Painter, r ui.Rect) {
			e.paint(p, r, theme, now)
		})
	})
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
	return e.onInput(c)(ev)
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
// 拖选尚未结束、输入法组合中或选区滚出视口时 ok 为 false。
func (e *Editor) SelectionAnchor() (x, y, lineH, viewW float32, ok bool) {
	sel := e.selection()
	if sel.Start == sel.End || e.dragging || e.compose != "" || e.width <= 0 {
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

func (e *Editor) onInput(c *ui.Context) func(ui.InputEvent) bool {
	return func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			x, y := ev.X+e.scroll.X, ev.Y+e.scroll.Y
			if ev.Button != 0 {
				// 右键落在选区之外时先把光标移过去，菜单才作用于点到的位置。
				if off, sel := e.lay.hit(x, y), e.selection(); off < sel.Start || off > sel.End {
					e.anchor, e.focus = off, off
				}
				return false
			}
			if e.toggleTaskAt(x, y) {
				return true
			}
			off := e.lay.hit(x, y)
			if ev.Clicks == 2 && e.requestRawEdit(off) {
				return true
			}
			if ev.Clicks >= 3 {
				e.selectLine(off)
			} else if ev.Clicks == 2 {
				e.selectWord(off)
			} else {
				e.anchor, e.focus = off, off
			}
			e.dragging = true
			e.compose = ""
			e.reveal = true
			return true
		case ui.InputPointerMove:
			if !e.dragging || ev.Button < 0 {
				return false
			}
			e.focus = e.lay.hit(ev.X+e.scroll.X, ev.Y+e.scroll.Y)
			e.reveal = true
			return true
		case ui.InputPointerUp:
			e.dragging = false
			return true
		case ui.InputScroll:
			return false
		case ui.InputKeyDown:
			return e.onKey(c, ev)
		case ui.InputText:
			e.insertText(ev.Text)
			return true
		case ui.InputCompose:
			e.compose = ev.Text
			e.comCaret = ev.Caret
			if ev.Text == "" {
				e.comCaret = 0
			}
			e.reveal = true
			return true
		case ui.InputCommand:
			return e.onCommand(c, ev.Text)
		default:
			return false
		}
	}
}

func (e *Editor) onKey(c *ui.Context, ev ui.InputEvent) bool {
	mods := ev.Mods &^ ui.Shift
	word := mods == e.wordMod
	switch ev.Key {
	case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd:
		e.move(ev.Key, ev.Mods&ui.Shift != 0, word || mods == ui.Cmd)
		return true
	case ui.KeyBackspace:
		e.backspace(word)
		return true
	case ui.KeyDelete:
		e.deleteForward(word)
		return true
	case ui.KeyEnter:
		if mods == ui.Cmd || mods == ui.Ctrl {
			e.cancelCompose()
			sel := e.doc.CreateParagraphAfter(e.focus)
			e.anchor, e.focus = sel.Start, sel.End
			e.dirty, e.reveal = true, true
			return true
		}
		if mods != 0 {
			return false
		}
		e.split()
		return true
	case ui.KeyTab:
		if mods != 0 {
			return false
		}
		return e.tabKey(ev.Mods&ui.Shift != 0)
	case ui.KeyEscape:
		return false
	case ui.KeyA:
		if mods == ui.Cmd {
			e.anchor, e.focus = 0, e.doc.Len()
			return true
		}
	case ui.KeyC:
		if mods == ui.Cmd {
			e.copy(c, false)
			return true
		}
	case ui.KeyX:
		if mods == ui.Cmd {
			e.copy(c, true)
			return true
		}
	case ui.KeyV:
		if mods == ui.Cmd {
			e.paste(c)
			return true
		}
	case ui.KeyZ:
		if ev.Mods&ui.Shift != 0 && ev.Mods&(ui.Cmd|ui.Ctrl) != 0 {
			e.Redo()
			return true
		}
		if mods == ui.Cmd || mods == ui.Ctrl {
			e.Undo()
			return true
		}
	case ui.KeyY:
		if mods == ui.Cmd || mods == ui.Ctrl {
			e.Redo()
			return true
		}
	case ui.KeyB:
		if mods == ui.Cmd {
			e.Format("bold")
			return true
		}
	}
	// 可打印字符由随后的 InputText 插入。
	return mods == 0 || (runtime.GOOS == "darwin" && mods == ui.Alt)
}

func (e *Editor) onCommand(c *ui.Context, name string) bool {
	switch name {
	case "copy":
		e.copy(c, false)
	case "cut":
		e.copy(c, true)
	case "paste":
		e.paste(c)
	case "selectAll":
		e.anchor, e.focus = 0, e.doc.Len()
	case "undo":
		e.Undo()
	case "redo":
		e.Redo()
	case "delete":
		e.replaceSelection("")
	default:
		return false
	}
	return true
}

func (e *Editor) move(key ui.Key, extend, extra bool) {
	e.cancelCompose()
	n := e.doc.Len()
	next := e.focus
	switch key {
	case ui.KeyLeft:
		if !extend && e.anchor != e.focus {
			next = min(e.anchor, e.focus)
			break
		}
		if extra {
			next = e.wordBound(e.focus, -1)
		} else {
			next = e.clusterBound(e.focus, -1)
		}
	case ui.KeyRight:
		if !extend && e.anchor != e.focus {
			next = max(e.anchor, e.focus)
			break
		}
		if extra {
			next = e.wordBound(e.focus, 1)
		} else {
			next = e.clusterBound(e.focus, 1)
		}
	case ui.KeyHome:
		if extra {
			next = 0
		} else {
			next = e.lineEdge(e.focus, true)
		}
	case ui.KeyEnd:
		if extra {
			next = n
		} else {
			next = e.lineEdge(e.focus, false)
		}
	case ui.KeyUp, ui.KeyDown:
		next = e.vertical(e.focus, key == ui.KeyDown)
	}
	if !extend {
		e.anchor = next
	}
	e.focus = next
	e.pending = 0
	e.reveal = true
}

func (e *Editor) lineEdge(offset int, home bool) int {
	li := e.lay.lineAt(offset)
	if li < 0 {
		return offset
	}
	ln := e.lay.lines[li]
	if home {
		return ln.origin
	}
	return ln.origin + ln.count
}

func (e *Editor) vertical(offset int, down bool) int {
	li := e.lay.lineAt(offset)
	if li < 0 {
		return offset
	}
	ln := e.lay.lines[li]
	x := ln.caretX(offset)
	j := li
	if down {
		if li+1 >= len(e.lay.lines) {
			return e.doc.Len()
		}
		j = li + 1
	} else {
		if li == 0 {
			return 0
		}
		j = li - 1
	}
	return e.lay.lines[j].hitOffset(x)
}

func (e *Editor) wordBound(offset, dir int) int {
	rs := []rune(e.doc.Text())
	if dir < 0 {
		i := offset
		for i > 0 && unicode.IsSpace(rs[i-1]) {
			i--
		}
		if i == 0 {
			return 0
		}
		kind := runeKind(rs[i-1])
		for i > 0 && runeKind(rs[i-1]) == kind {
			i--
		}
		return i
	}
	i := offset
	for i < len(rs) && unicode.IsSpace(rs[i]) {
		i++
	}
	if i >= len(rs) {
		return len(rs)
	}
	kind := runeKind(rs[i])
	for i < len(rs) && runeKind(rs[i]) == kind {
		i++
	}
	return i
}

// clusterBound 按字形簇移动。ZWJ 序列和键帽 emoji 一次删掉，不拆开。
func (e *Editor) clusterBound(offset, dir int) int {
	bounds := clusterBounds([]rune(e.doc.Text()))
	if dir < 0 {
		for i := len(bounds) - 1; i >= 0; i-- {
			if bounds[i] < offset {
				return bounds[i]
			}
		}
		return 0
	}
	for _, at := range bounds {
		if at > offset {
			return at
		}
	}
	return e.doc.Len()
}

// clusterBounds 不把组合符后面的普通文字并进簇；ZWJ 和区域旗帜成对处理。
func clusterBounds(rs []rune) []int {
	out := []int{0}
	regional := 0
	for i := 1; i < len(rs); i++ {
		prev, cur := rs[i-1], rs[i]
		if prev >= 0x1f1e6 && prev <= 0x1f1ff {
			regional++
		} else {
			regional = 0
		}
		join := isClusterContinue(cur) || prev == 0x200d || prev == '\r' && cur == '\n'
		if cur >= 0x1f1e6 && cur <= 0x1f1ff && regional%2 == 1 {
			join = true
		}
		if !join {
			out = append(out, i)
		}
	}
	if len(rs) > 0 {
		out = append(out, len(rs))
	}
	return out
}

func isClusterContinue(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r) || r == 0x200d || r >= 0x1f3fb && r <= 0x1f3ff || r >= 0xe0020 && r <= 0xe007f
}

func runeKind(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	default:
		return 2
	}
}

func (e *Editor) selectWord(offset int) {
	a := e.wordBound(offset, -1)
	b := e.wordBound(a, 1)
	if b < offset {
		b = e.wordBound(offset, 1)
	}
	e.anchor, e.focus = a, b
}

func (e *Editor) selectLine(offset int) {
	e.anchor = e.lineEdge(offset, true)
	e.focus = e.lineEdge(offset, false)
}

func (e *Editor) backspace(word bool) {
	e.cancelCompose()
	if e.anchor != e.focus {
		e.replaceSelection("")
		return
	}
	if e.focus == 0 {
		return
	}
	from := e.clusterBound(e.focus, -1)
	if word {
		from = e.wordBound(e.focus, -1)
	}
	sel := e.doc.Delete(from, e.focus)
	e.anchor, e.focus = sel.Start, sel.End
	e.dirty = true
	e.reveal = true
}

func (e *Editor) deleteForward(word bool) {
	e.cancelCompose()
	if e.anchor != e.focus {
		e.replaceSelection("")
		return
	}
	if e.focus >= e.doc.Len() {
		return
	}
	to := e.clusterBound(e.focus, 1)
	if word {
		to = e.wordBound(e.focus, 1)
	}
	sel := e.doc.Delete(e.focus, to)
	e.anchor, e.focus = sel.Start, sel.End
	e.dirty = true
	e.reveal = true
}

func (e *Editor) split() { e.cancelCompose(); e.replaceSelection("\n") }

func (e *Editor) insertText(text string) {
	if text == "" {
		return
	}
	e.cancelCompose()
	e.replaceSelection(text)
}

func (e *Editor) replaceSelection(text string) {
	before := e.doc.Markdown()
	out := e.doc.Replace(e.selection(), text)
	e.anchor, e.focus = out.Start, out.End
	e.pending = 0
	e.dirty = e.dirty || before != e.doc.Markdown()
	e.reveal = true
}

// 只有 InputText 能提交输入法内容；移动和删除仅取消预编辑串。
func (e *Editor) cancelCompose() { e.compose = ""; e.comCaret = 0 }

func (e *Editor) copy(c *ui.Context, cut bool) {
	if c == nil {
		return
	}
	sel := e.selection()
	if sel.Start == sel.End {
		return
	}
	c.WriteClipboard(e.doc.SliceMarkdown(sel))
	if cut {
		out := e.doc.Delete(sel.Start, sel.End)
		e.anchor, e.focus = out.Start, out.End
		e.dirty = true
		e.reveal = true
	}
}

func (e *Editor) paste(c *ui.Context) {
	if c == nil {
		return
	}
	text := c.ReadClipboard()
	if text == "" {
		return
	}
	e.cancelCompose()
	sel := e.doc.PasteSelection(e.selection(), text)
	e.anchor, e.focus = sel.Start, sel.End
	e.dirty = true
	e.reveal = true
}

func (e *Editor) caretRect() [4]float32 {
	off := e.focus
	if e.compose != "" {
		off = e.focus
	}
	li := e.lay.lineAt(off)
	if li < 0 || len(e.lay.blocks) == 0 {
		return [4]float32{padX, padY, 1, e.fontSize}
	}
	ln := e.lay.lines[li]
	b := e.blockBox(ln.block)
	x := b.x + ln.caretX(off)
	if e.compose != "" {
		x += e.composeAdvance
	}
	return [4]float32{x, ln.y, 1, ln.height}
}

func (e *Editor) composeWidth() float32 {
	if e.compose == "" {
		return 0
	}
	return advanceOf(e.composeGlyphs)
}

func (e *Editor) blockBox(index int) blockBox {
	for _, b := range e.lay.blocks {
		if b.index == index {
			return b
		}
	}
	if len(e.lay.blocks) == 0 {
		return blockBox{}
	}
	return e.lay.blocks[0]
}

func (e *Editor) keepCaretVisible() {
	r := e.caretRect()
	if e.typewriter {
		e.scroll.Y = r[1] + r[3]/2 - e.viewH/2
	}
	if r[1] < e.scroll.Y {
		e.scroll.Y = max(0, r[1]-24)
	}
	if r[1]+r[3] > e.scroll.Y+e.viewH {
		e.scroll.Y = max(0, r[1]+r[3]-e.viewH+24)
	}
	e.scroll.Y = max(0, min(e.scroll.Y, max(0, e.lay.height-e.viewH)))
	if r[0] < e.scroll.X {
		e.scroll.X = max(0, r[0]-24)
	}
	if r[0]+1 > e.scroll.X+e.width {
		e.scroll.X = max(0, r[0]+25-e.width)
	}
}

func (e *Editor) paint(p *ui.Painter, r ui.Rect, theme *ui.Theme, now time.Time) {
	sel := e.selection()
	for _, b := range e.lay.blocks {
		e.paintBlock(p, r, b, theme)
	}
	for _, ln := range e.lay.lines {
		e.paintSelection(p, r, ln, sel, theme)
		e.paintLine(p, r, ln, theme)
	}
	if e.focusMode {
		// 用纸色盖住光标所在块之外的内容，装饰与文字一并淡化。
		active := e.doc.BlockIndexAt(e.focus)
		veil := theme.Background.Alpha(.58)
		for _, b := range e.lay.blocks {
			if b.index != active {
				p.Fill(ui.Rect{X: r.X + b.x - 6, Y: r.Y + b.y - 2, W: b.w + 12, H: b.h + 4}, veil, 0)
			}
		}
	}
	if e.compose != "" {
		e.paintCompose(p, r, theme)
	}
	if int(now.UnixMilli()/530)%2 == 0 && e.anchor == e.focus && e.compose == "" {
		c := e.caretRect()
		p.Fill(ui.Rect{X: r.X + c[0], Y: r.Y + c[1], W: 1.5, H: c[3]}, theme.Accent, 0)
	}
}

func (e *Editor) paintBlock(p *ui.Painter, r ui.Rect, b blockBox, theme *ui.Theme) {
	x, y := r.X+b.x, r.Y+b.y
	switch b.kind {
	case richtext.Callout:
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Surface, 5)
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Accent.Alpha(.04), 5)
		p.Glyphs(b.title, x+37, y+12+b.titleAscent, theme.Text)
		// 提示图标是灯泡轮廓，不进入正文坐标。
		paintBulb(p, x+13, y+12, theme.Accent)
	case richtext.List:
		if b.bullet && len(b.lines) > 0 {
			ln := e.lay.lines[b.lines[0]]
			dot := new(ui.Path).Circle(x+b.indent+4, r.Y+ln.y+ln.height/2+1, 1.7)
			p.FillPath(dot, theme.Accent.Alpha(.75))
		}
	case richtext.Task:
		if len(b.lines) > 0 {
			ln := e.lay.lines[b.lines[0]]
			check := ui.Rect{X: x + b.indent + 2.5, Y: r.Y + ln.y + (ln.height-15)/2 + 1, W: 15, H: 15}
			if b.checked {
				p.Fill(check, theme.Accent, 4)
				tick := new(ui.Path).MoveTo(check.X+3.8, check.Y+7.8).LineTo(check.X+6.5, check.Y+10.4).LineTo(check.X+11.3, check.Y+4.9)
				p.StrokePath(tick, 1.7, theme.Background)
			} else {
				p.Stroke(check, theme.TextMuted.Alpha(.8), 4, 1.5)
			}
		}
	case richtext.Quote:
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Surface, 4)
		p.Fill(ui.Rect{X: x, Y: y, W: quoteBar, H: b.h}, theme.Accent, 1)
	case richtext.Code:
		bg := theme.Surface
		if int(theme.Background.R)+int(theme.Background.G)+int(theme.Background.B) < 384 {
			// 深色主题的代码块比纸面更沉，与引用、提示块的浅底区分。
			bg = theme.Background.Mix(ui.RGB(0, 0, 0), .14)
		}
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, bg, 10)
		p.Stroke(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Border, 10, 1)
	case richtext.Raw:
		if b.math != nil {
			b.math.Paint(p, x+max(0, (b.w-b.math.Width)/2), y+8+b.math.Ascent, theme.Text)
			return
		}
		if b.diagram != nil {
			b.diagram.Paint(p, x+max(0, (b.w-b.diagram.Width)/2), y+8)
			return
		}
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Surface, 6)
		p.Stroke(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Border, 6, 1)
	case richtext.Horizontal:
		p.Fill(ui.Rect{X: x, Y: y + b.h/2, W: b.w, H: 1}, theme.Border, 0)
	case richtext.Image:
		if b.image != nil {
			ih := b.h
			if b.alt != "" {
				ih -= e.fontSize + 6
			}
			p.Image(b.image, ui.Rect{X: x, Y: y, W: b.w, H: ih}, ui.Contain)
		} else {
			p.Fill(ui.Rect{X: x, Y: y, W: min(b.w, 240), H: 64}, theme.Surface, 6)
		}
	case richtext.TableBlock:
		e.paintTableGrid(p, x, y, b, theme)
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

func (e *Editor) paintTableGrid(p *ui.Painter, x, y float32, b blockBox, theme *ui.Theme) {
	blocks := e.doc.Blocks()
	if b.index < len(blocks) && blocks[b.index].Table != nil && blocks[b.index].Table.Header && len(b.rows) > 1 {
		p.Fill(ui.Rect{X: x, Y: y, W: b.w, H: b.rows[1]}, theme.Surface, 10)
	}
	p.Stroke(ui.Rect{X: x, Y: y, W: b.w, H: b.h}, theme.Border, 10, 1)
	for i := 1; i+1 < len(b.rows); i++ {
		p.Fill(ui.Rect{X: x, Y: y + b.rows[i], W: b.w, H: 1}, theme.Border, 0)
	}
	for i := 1; i+1 < len(b.cols); i++ {
		p.Fill(ui.Rect{X: x + b.cols[i], Y: y, W: 1, H: b.h}, theme.Border, 0)
	}
}

func (e *Editor) paintLine(p *ui.Painter, r ui.Rect, ln line, theme *ui.Theme) {
	b := e.blockBox(ln.block)
	baseX := r.X + b.x
	baseY := r.Y + ln.y + ln.ascent
	if len(ln.glyphs) == 0 {
		return
	}
	color := theme.Text
	if b.raw || b.kind == richtext.Quote {
		color = theme.TextMuted
	}
	p.Glyphs(ln.markers, baseX, baseY, color)
	if len(ln.spans) == 0 {
		p.Glyphs(ln.glyphs, baseX, baseY, color)
	} else {
		for _, span := range ln.spans {
			if len(span.glyphs) == 0 {
				continue
			}
			start := span.glyphs[0].X
			end := advanceOf(span.glyphs)
			if span.highlight {
				p.Fill(ui.Rect{X: baseX + start, Y: r.Y + ln.y + 2, W: end - start, H: ln.height - 4}, ui.RGBA(233, 185, 73, .35), 2)
			}
			if span.code {
				p.Fill(ui.Rect{X: baseX + start - 2, Y: r.Y + ln.y + 2, W: end - start + 4, H: ln.height - 4}, theme.Surface, 3)
			}
			ink := color
			switch span.ink {
			case inkAccent:
				ink = theme.Accent
			case inkMuted:
				ink = theme.TextMuted
			}
			if span.link != "" && safeURL(span.link, false) {
				ink = theme.Accent
			}
			if span.math != nil {
				span.math.Paint(p, baseX+start, baseY, ink)
			} else {
				p.Glyphs(span.glyphs, baseX, baseY+span.dy, ink)
			}
			if span.strike {
				p.Fill(ui.Rect{X: baseX + start, Y: baseY - ln.ascent*.35, W: end - start, H: 1}, theme.Text, 0)
			}
			if span.link != "" && safeURL(span.link, false) {
				p.Fill(ui.Rect{X: baseX + start, Y: baseY + 2, W: end - start, H: 1}, theme.Accent, 0)
			}
		}
	}
	// 删除线和链接下划线按字形跨度画，不拆 emoji。
	if len(ln.spans) == 0 {
		e.paintDecorations(p, baseX, baseY, ln, theme)
	}
}

func (e *Editor) paintDecorations(p *ui.Painter, baseX, baseY float32, ln line, theme *ui.Theme) {
	// 装饰按该行对应的 run 样式。这里用字形跨度画一条连续线，链接用强调色。
	blocks := e.doc.Blocks()
	if ln.block < 0 || ln.block >= len(blocks) {
		return
	}
	bl := blocks[ln.block]
	if bl.Kind == richtext.Code || bl.Kind == richtext.Raw || bl.Kind == richtext.Image {
		return
	}
	local := 0
	prefixSkip := ln.prefix
	_ = prefixSkip
	off := ln.origin - e.blockOrigin(ln.block)
	for _, run := range textRuns(bl, ln) {
		rs := []rune(run.text)
		if local >= ln.count {
			break
		}
		// 这一段可能从上一行延续。
		if off > 0 {
			if off >= len(rs) {
				off -= len(rs)
				continue
			}
			rs = rs[off:]
			off = 0
		}
		n := len(rs)
		if local+n > ln.count {
			n = ln.count - local
		}
		if n <= 0 {
			continue
		}
		x0 := ln.caretX(ln.origin + local)
		x1 := ln.caretX(ln.origin + local + n)
		if run.strike {
			p.Fill(ui.Rect{X: baseX + x0, Y: baseY - ln.ascent*0.35, W: x1 - x0, H: 1}, theme.Text, 0)
		}
		if run.link != "" && safeURL(run.link, false) {
			p.Fill(ui.Rect{X: baseX + x0, Y: baseY + 2, W: x1 - x0, H: 1}, theme.Accent, 0)
		}
		local += n
		if local >= ln.count {
			break
		}
	}
}

type deco struct {
	text   string
	strike bool
	link   string
}

func textRuns(bl richtext.Block, ln line) []deco {
	var runs []richtext.Run
	switch bl.Kind {
	case richtext.TableBlock:
		if bl.Table != nil {
			for _, row := range bl.Table.Rows {
				for _, cell := range row {
					runs = append(runs, cell.Runs...)
				}
			}
		}
	default:
		runs = bl.Runs
	}
	out := make([]deco, 0, len(runs))
	for _, r := range runs {
		d := deco{text: r.Text, strike: r.Marks&richtext.MarkStrike != 0}
		if r.Link != nil {
			d.link = r.Link.URL
		}
		out = append(out, d)
	}
	_ = ln
	return out
}

func (e *Editor) blockOrigin(index int) int {
	return e.blockBox(index).origin
}

func (e *Editor) paintSelection(p *ui.Painter, r ui.Rect, ln line, sel richtext.Selection, theme *ui.Theme) {
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
	b := e.blockBox(ln.block)
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
	// caretRect 已含组合光标偏移，绘制时回到组合起点。
	width := e.composeWidth()
	x := r.X + c[0] - e.composeAdvance
	y := r.Y + c[1]
	p.Fill(ui.Rect{X: x, Y: y, W: width + 2, H: c[3]}, theme.Surface, 2)
	g := e.composeGlyphs
	m := e.composeMetrics
	p.Glyphs(g, x, y+m.Ascent, theme.Text)
	p.Fill(ui.Rect{X: x, Y: y + c[3] - 1, W: width, H: 1}, theme.Accent, 0)
}

func (e *Editor) ensureImages() {
	for _, b := range e.doc.Blocks() {
		if b.Kind == richtext.Image {
			e.wantImage(b.URL)
		}
	}
}

func (e *Editor) wantImage(url string) {
	if url == "" || len(e.images) >= maxCachedImages {
		return
	}
	if _, ok := e.images[url]; ok {
		return
	}
	if _, ok := e.fetched[url]; ok {
		return
	}
	if safeURL(url, true) {
		if e.remoteN >= remoteActive || e.remoteB >= remoteTotal {
			return
		}
		e.fetched[url] = struct{}{}
		e.remoteN++
		// 后台只持有固定的 client、channel；文档、位图和预算由 View 消化。
		go fetchImage(e.client, e.imgCh, url, e.notifyImage)
		return
	}
	e.fetched[url] = struct{}{}
	if e.read != nil {
		// 本地路径只能交给桌面的授权回调，控件自己从不打开磁盘路径。
		if bm := e.read(url); bitmapAllowed(bm) {
			e.images[url] = bm
		}
	} else if bm := decodeDataURI(url); bm != nil {
		e.images[url] = bm
	}
}

func (e *Editor) notifyImage() {
	e.wakeMu.RLock()
	fn := e.wake
	e.wakeMu.RUnlock()
	if fn != nil {
		fn()
	}
}

type imageResult struct {
	url  string
	data []byte
}

const maxCachedImages = 32
const maxImagePixels = 16 << 20

func bitmapAllowed(bm *ui.Bitmap) bool {
	if bm == nil {
		return false
	}
	w, h := bm.Size()
	return w > 0 && h > 0 && int64(w)*int64(h) <= maxImagePixels
}

// drainImages 在 UI 线程解码，后台从不读写图片缓存或布局。
func (e *Editor) drainImages(c *ui.Context) {
	for {
		select {
		case res := <-e.imgCh:
			e.remoteN--
			if len(res.data) == 0 || e.remoteB+len(res.data) > remoteTotal || len(e.images) >= maxCachedImages {
				continue
			}
			e.remoteB += len(res.data)
			if bm := decodeImage(res.data); bm != nil {
				e.images[res.url] = bm
				if c != nil {
					c.Invalidate()
				}
			}
		default:
			return
		}
	}
}

func fetchImage(client *http.Client, ch chan<- imageResult, url string, wake func()) {
	result := imageResult{url: url}
	defer func() { ch <- result; wake() }()
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, remoteLimit+1))
	if err != nil || len(data) == 0 || len(data) > remoteLimit {
		return
	}
	result.data = data
}

func decodeImage(raw []byte) *ui.Bitmap {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil
	}
	bm, err := ui.DecodeBitmap(raw)
	if err != nil {
		return nil
	}
	return bm
}

func decodeDataURI(uri string) *ui.Bitmap {
	if !strings.HasPrefix(strings.ToLower(uri), "data:image/") {
		return nil
	}
	head, body, ok := strings.Cut(uri, ",")
	if !ok || !strings.HasSuffix(strings.ToLower(head), ";base64") || len(body) > base64.StdEncoding.EncodedLen(remoteLimit) {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil || len(raw) > remoteLimit {
		return nil
	}
	return decodeImage(raw)
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
	}
	return out
}

// toggleTaskAt 只有点击首行复选框才切换完成状态，文字区域继续用于选区。
func (e *Editor) toggleTaskAt(x, y float32) bool {
	for _, b := range e.lay.blocks {
		if b.kind != richtext.Task || len(b.lines) == 0 {
			continue
		}
		ln := e.lay.lines[b.lines[0]]
		if x < b.x+b.indent || x > b.x+b.indent+20 || y < ln.y || y > ln.y+ln.height {
			continue
		}
		blocks := e.doc.Blocks()
		e.doc.SetChecked(b.index, !blocks[b.index].Checked)
		e.cancelCompose()
		e.dragging = false
		e.dirty = true
		return true
	}
	return false
}
