package desktop

import (
	"errors"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/diagram"
	"leafmark/internal/mathlayout"
	"leafmark/internal/nativeeditor"
	"leafmark/internal/richtext"
	"leafmark/internal/workspace"
)

// sourceEditor 是源码模式的编辑器。正文由 nativeeditor 的等宽纯文本绘制，
// 选区、滚动、输入法、撤销和只读都走它的公开接口。MyGo 文本域没有这些接口。
type sourceEditor struct {
	ed   *nativeeditor.Editor
	size float32
	// bounds 是上一帧编辑区的边界，供验收确认它铺满书写区域。
	bounds ui.Rect
	// 记录已应用的配色主题，仅在深浅色切换时更新，避免每帧重复下发。
	paletteSet  bool
	paletteDark bool
	// 查找从上一处之后继续。源码整篇都在一个代码块里，查找按全文匹配，不走富文本的跳过规则。
	findFrom int
	findHit  string
}

const sourceFontFamily = `"Geist Mono", "SFMono-Regular", "Consolas", monospace`

var (
	sourceLightPalette = buildSourcePalette(false)
	sourceDarkPalette  = buildSourcePalette(true)
)

func buildSourcePalette(dark bool) [6]ui.Color {
	colors := sourcePalette(dark)
	var palette [6]ui.Color
	palette[sourceHeading] = colors.heading
	palette[sourceMark] = colors.mark
	palette[sourceLink] = colors.link
	palette[sourceCode] = colors.code
	palette[sourceMath] = colors.math
	return palette
}

func cachedSourcePalette(dark bool) [6]ui.Color {
	if dark {
		return sourceDarkPalette
	}
	return sourceLightPalette
}

func newSourceEditor(markdown string, size float32) *sourceEditor {
	ed := nativeeditor.NewPlainText(markdown)
	s := &sourceEditor{ed: ed}
	s.FontSize(size)
	ed.SetSourceSyntax(sourceSyntax)
	s.applyPalette(false)
	return s
}

// sourceSyntax 把源码扫描结果转成编辑器的着色区间。Kind 与 sourceKind 对齐。
func sourceSyntax(text string) []nativeeditor.SourceSpan {
	spans := sourceHighlight(text)
	out := make([]nativeeditor.SourceSpan, 0, len(spans))
	for _, sp := range spans {
		out = append(out, nativeeditor.SourceSpan{Start: sp.start, End: sp.end, Kind: uint8(sp.kind)})
	}
	return out
}

func (s *sourceEditor) applyPalette(dark bool) {
	s.ed.SetFontFamily(sourceFontFamily)
	s.ed.SetLineHeight(1.7)
	s.ed.SetSourcePalette(cachedSourcePalette(dark))
	s.paletteSet = true
	s.paletteDark = dark
}

func (s *sourceEditor) View(c *ui.Context) {
	if dark := c.Theme().Dark; !s.paletteSet || s.paletteDark != dark {
		s.applyPalette(dark)
	}
	box := ui.Box(c).Fill().MinHeight(0)
	box.Children(func() { s.ed.View(c) })
	s.bounds = box.Bounds()
}

// Bounds 返回编辑区边界。View 尚未布局时为零。
func (s *sourceEditor) Bounds() ui.Rect { return s.ed.Bounds() }

// Scroll 返回内容滚动偏移，数值来自编辑器自己的滚动状态。
func (s *sourceEditor) Scroll() (x, y float32) { return s.ed.Scroll() }

// Jumping 报告选区是否还等着滚进视野。状态来自编辑器，View 滚完后才变 false。
func (s *sourceEditor) Jumping() bool { return s.ed.ScrollPending() }

func (s *sourceEditor) Markdown() string { return s.ed.Markdown() }
func (s *sourceEditor) Text() string     { return s.ed.Text() }
func (s *sourceEditor) Changed() bool    { return s.ed.Changed() }

func (s *sourceEditor) Undo()                                             { s.ed.Undo(); s.findHit = "" }
func (s *sourceEditor) Redo()                                             { s.ed.Redo(); s.findHit = "" }
func (s *sourceEditor) Format(string)                                     {}
func (s *sourceEditor) SetReadPicture(func(string) *nativeeditor.Picture) {}
func (s *sourceEditor) Selection() (int, int)                             { return s.ed.Selection() }
func (s *sourceEditor) SetSelection(start, end int) {
	s.ed.SetSelection(start, end)
	s.findHit = ""
}
func (s *sourceEditor) HandleInput(c *ui.Context, ev ui.InputEvent) bool {
	taken := s.ed.HandleInput(c, ev)
	if taken && (ev.Kind == ui.InputText || ev.Kind == ui.InputCommand) {
		s.findHit = ""
	}
	return taken
}
func (s *sourceEditor) FontSize(size float32) {
	if size <= 0 {
		size = 14
	}
	// 源码比正文小一号，沿用原来文本域的 FontSize(size-1)。
	s.size = size
	s.ed.FontSize(size - 1)
	s.ed.SetFontFamily(sourceFontFamily)
	s.ed.SetLineHeight(1.7)
}

// runeOffsetToByte 返回第 runeOffset 个 rune 对应的字节起点；超出文本长度时返回 len(text)。
func runeOffsetToByte(text string, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	for i := range text {
		if runeOffset == 0 {
			return i
		}
		runeOffset--
	}
	return len(text)
}

// Find 选中下一处 query，并由编辑器把它滚进视野。从头再找时回到文首。
func (s *sourceEditor) Find(query string) bool {
	if query == "" {
		return false
	}
	text := s.Text()
	from := s.findFrom
	if s.findHit != query || from < 0 {
		from = 0
	}
	byteFrom := runeOffsetToByte(text, from)
	idx := -1
	if rel := strings.Index(text[byteFrom:], query); rel >= 0 {
		idx = byteFrom + rel
	}
	if idx < 0 && from > 0 {
		idx = strings.Index(text, query)
	}
	if idx < 0 {
		s.findHit = ""
		return false
	}
	at := utf8.RuneCountInString(text[:idx])
	qLen := utf8.RuneCountInString(query)
	s.ed.SetSelection(at, at+qLen)
	s.findFrom = at + qLen
	s.findHit = query
	return true
}

// Replace 把 query 换成 replacement。all 为假时只换当前选中的这一处，
// 没有选中时换下一处。返回替换次数；只读或找不到时为 0。撤销由编辑器记录。
func (s *sourceEditor) Replace(query, replacement string, all bool) int {
	if s.ed.ReadOnly() || query == "" {
		return 0
	}
	if !all {
		runes := []rune(s.Text())
		q := []rune(query)
		start, end := s.Selection()
		if start < 0 || end > len(runes) || end-start != len(q) || !sourceEqual(runes[start:end], q) {
			if !s.Find(query) {
				return 0
			}
		}
		n := s.ed.Replace(query, replacement, false)
		if n > 0 {
			s.findHit = ""
		}
		return n
	}
	// 全文替换收成一步。逐处替换会各记一次撤销。
	count, next := sourceReplace(s.Text(), query, replacement, true)
	if count == 0 {
		return 0
	}
	s.ed.SetSelection(0, len([]rune(s.Text())))
	s.ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: next})
	s.findHit = ""
	return count
}

// SetReadOnly 让源码只能选择、复制和查找。阅读模式通过它接入。
func (s *sourceEditor) SetReadOnly(on bool) { s.ed.SetReadOnly(on) }

func (s *sourceEditor) InsertLink(label, url string) {
	s.append("[" + label + "](" + url + ")")
}
func (s *sourceEditor) InsertImage(alt, markdownPath string) {
	s.append("![" + alt + "](" + markdownPath + ")")
}
func (s *sourceEditor) append(markdown string) {
	if s.ed.ReadOnly() {
		return
	}
	text := s.Text()
	suffix := markdown + "\n"
	if text != "" && !strings.HasSuffix(text, "\n") {
		suffix = "\n" + suffix
	}
	at := len([]rune(text))
	s.ed.SetSelection(at, at)
	s.ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: suffix})
}

// sourceMode 报告标签是否处于源码模式。
func sourceMode(tab *nativeTab) bool {
	if tab == nil {
		return false
	}
	_, ok := tab.editor.(*sourceEditor)
	return ok
}

// toggleSource 在所见即所得与源码之间切换当前标签。正文不变，保留大致光标与滚动位置，撤销历史从切换后重新开始。
func (a *nativeApp) toggleSource() {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	fromSource := sourceMode(tab)
	oldStart, _ := tab.editor.Selection()
	oldLen := utf8.RuneCountInString(tab.editor.Text())
	markdown := tab.editor.Markdown()
	if fromSource {
		fresh := newDocumentEditor(markdown)
		a.configureEditor(fresh)
		a.connectEditor(fresh)
		if a.assets != nil {
			fresh.SetReadPicture(imageReader(a.assets, tab.id))
		}
		tab.editor = fresh
	} else {
		tab.editor = newSourceEditor(markdown, a.settings.FontSize)
		a.configureEditor(tab.editor)
	}
	if oldStart > 0 {
		newLen := utf8.RuneCountInString(tab.editor.Text())
		target := mapModeOffset(markdown, fromSource, oldStart, oldLen, newLen)
		tab.editor.SetSelection(target, target)
	}
}

// mapModeOffset 按标题章节的分段比例，把光标偏移从源码或原位模式映射到另一侧；
// 文档没有标题或两侧标题数不一致时，按整篇正文比例估算。
func mapModeOffset(markdown string, fromSource bool, oldOffset, oldLen, newLen int) int {
	if oldOffset <= 0 || oldLen <= 0 || newLen <= 0 {
		return 0
	}
	if oldOffset >= oldLen {
		return newLen
	}
	var oldHeadings, newHeadings []nativeHeading
	if fromSource {
		oldHeadings = nativeSourceOutline(markdown)
		newHeadings = nativeOutline(markdown)
	} else {
		oldHeadings = nativeOutline(markdown)
		newHeadings = nativeSourceOutline(markdown)
	}
	oldStart, oldEnd := 0, oldLen
	newStart, newEnd := 0, newLen
	if len(oldHeadings) > 0 && len(oldHeadings) == len(newHeadings) {
		sec := -1
		for i, h := range oldHeadings {
			if h.at == oldOffset {
				return min(max(newHeadings[i].at, 0), newLen)
			}
			if h.at < oldOffset {
				sec = i
			} else {
				break
			}
		}
		if sec < 0 {
			oldEnd = oldHeadings[0].at
			newEnd = newHeadings[0].at
		} else {
			oldStart = oldHeadings[sec].at
			newStart = newHeadings[sec].at
			if sec+1 < len(oldHeadings) {
				oldEnd = oldHeadings[sec+1].at
				newEnd = newHeadings[sec+1].at
			}
		}
	}
	oldStart = min(max(oldStart, 0), oldLen)
	oldEnd = min(max(oldEnd, oldStart), oldLen)
	newStart = min(max(newStart, 0), newLen)
	newEnd = min(max(newEnd, newStart), newLen)
	if oldEnd <= oldStart || newEnd <= newStart {
		return newStart
	}
	ratio := float64(oldOffset-oldStart) / float64(oldEnd-oldStart)
	target := newStart + int(ratio*float64(newEnd-newStart)+0.5)
	return min(max(target, newStart), newEnd)
}

// ask 打开一个单行输入框；ok 在主线程收到去掉首尾空白后的非空输入。
func (a *nativeApp) ask(title, hint, initial string, ok func(string)) {
	a.promptKind, a.promptTitle, a.promptHint = "text", title, hint
	a.promptText = initial
	a.promptOpen = true
	a.promptOK = func(raw string) {
		if raw = strings.TrimSpace(raw); raw != "" {
			ok(raw)
		}
	}
}

// askWorkspacePath 在工作区里新建文档或文件夹，parent 是相对工作区根的上级目录。
func (a *nativeApp) askWorkspacePath(kind, parent string) {
	if a.workspace == nil || a.folderName == "" {
		a.notice = "请先打开一个文件夹作为工作区"
		return
	}
	initial := ""
	if parent != "" {
		initial = parent + "/"
	}
	title, hint := "新建 Markdown 文件", "相对于工作区根目录。文件名须使用 .md 或 .markdown 扩展名。"
	if kind == "folder" {
		title, hint = "新建文件夹", "相对于工作区根目录；上级文件夹须已存在。"
	}
	a.ask(title, hint, initial, func(relative string) {
		if kind == "file" && path.Ext(relative) == "" {
			relative += ".md"
		}
		go func() {
			var err error
			if kind == "folder" {
				err = a.workspace.CreateFolder(relative)
			} else {
				err = a.workspace.CreateFile(relative)
			}
			a.update(func() {
				if err != nil {
					a.notice = err.Error()
					return
				}
				if dir := path.Dir(relative); dir != "." {
					a.openDirs[dir] = true
				}
				a.reloadNavigation()
				if kind == "file" {
					a.openWorkspace(relative)
				}
			})
		}()
	})
}

// workspaceRoot 返回当前工作区的绝对路径，没有工作区时为空。
func (a *nativeApp) workspaceRoot() string {
	if a.workspace == nil {
		return ""
	}
	state, err := a.workspace.Current()
	if err != nil || state.Workspace == nil {
		return ""
	}
	return state.Workspace.Path
}

// askRename 只改名称。已打开的文档及其所在文件夹须先关闭相关标签，避免标签仍指向旧路径。
func (a *nativeApp) askRename(node workspace.Node) {
	root := a.workspaceRoot()
	if root == "" {
		return
	}
	target := filepath.Join(root, filepath.FromSlash(node.Path))
	for _, doc := range a.files.List() {
		if doc.Path == "" {
			continue
		}
		if doc.Path == target || (node.Directory && strings.HasPrefix(doc.Path, target+string(filepath.Separator))) {
			a.notice = "请先关闭相关标签再重命名"
			return
		}
	}
	a.ask("重命名", "仅修改名称，不能包含路径分隔符。", node.Name, func(name string) {
		if strings.ContainsAny(name, `/\`) {
			a.notice = "名称不能包含路径分隔符"
			return
		}
		next := name
		if dir := path.Dir(node.Path); dir != "." {
			next = dir + "/" + name
		}
		if next == node.Path {
			return
		}
		go func() {
			err := a.workspace.Rename(node.Path, next)
			a.update(func() {
				if err != nil {
					a.notice = err.Error()
					return
				}
				if a.openDirs[node.Path] {
					delete(a.openDirs, node.Path)
					a.openDirs[next] = true
				}
				a.reloadNavigation()
			})
		}()
	})
}

// treeMenu 是工作区条目的右键菜单。
func (a *nativeApp) treeMenu(node workspace.Node) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if node.Directory {
			if m.Item("新建文档…").Chosen() {
				a.askWorkspacePath("file", node.Path)
			}
			if m.Item("新建文件夹…").Chosen() {
				a.askWorkspacePath("folder", node.Path)
			}
			m.Separator()
		} else if m.Item("打开").Chosen() {
			a.openWorkspace(node.Path)
		}
		if m.Item("重命名…").Chosen() {
			a.askRename(node)
		}
		if m.Item("在文件夹中显示").Chosen() {
			if root := a.workspaceRoot(); root != "" {
				mygo.Shell.ShowItemInFolder(filepath.Join(root, filepath.FromSlash(node.Path)))
			}
		}
	}
}

const externalInterval = 2 * time.Second

// watchExternal 每两秒核对一次当前文档的磁盘内容：没有本地修改时直接换成磁盘上的新内容，
// 有未保存修改时暂停自动保存并提示，由用户决定是否重新加载。
func (a *nativeApp) watchExternal(now time.Time) {
	if a.externalBusy || now.Sub(a.externalAt) < externalInterval {
		return
	}
	a.externalAt = now
	tab := a.active()
	if tab == nil || tab.editor == nil || a.saving[tab.id] || a.conflicts[tab.id] {
		return
	}
	doc, ok := a.document(tab.id)
	if !ok || doc.Path == "" {
		return
	}
	id := tab.id
	a.externalBusy = true
	go func() {
		state, err := a.files.CheckExternal(id)
		a.update(func() {
			a.externalBusy = false
			if err != nil || !state.Changed || state.Missing || a.saving[id] {
				return
			}
			tab := a.tabByID(id)
			if tab == nil {
				return
			}
			doc, _ := a.document(id)
			if doc.Dirty || a.pendingEdit(tab) {
				a.conflicts[id] = true
				a.notice = "文件已在外部修改，可从“⋯”菜单重新加载"
				return
			}
			a.reloadDocument(id, false)
		})
	}()
}

// exportDark 在主线程取主题快照，导出期间不随设置或系统外观变化。
func (a *nativeApp) exportDark() bool {
	switch a.settings.Theme {
	case "light":
		return false
	case "dark":
		return true
	default:
		return a.effectiveDark
	}
}

// exportSnapshot 在主线程取得当前文档的只读快照，避免后台协程访问编辑器状态。
func (a *nativeApp) exportSnapshot() (id, name, markdown string, ok bool) {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return "", "", "", false
	}
	doc, _ := a.document(tab.id)
	name = doc.Name
	if name == "" {
		name = "未命名.md"
	}
	return tab.id, name, tab.editor.Markdown(), true
}

// renderExportBody 把 Markdown 快照渲染为导出用的 HTML 正文，本地图片内嵌为数据地址，可在后台协程执行。
func renderExportBody(assets *Assets, id, markdown string, dark bool) string {
	var imageCache map[string]string
	return richtext.Parse(markdown).HTMLWithRenderers(func(url string) string {
		lower := strings.ToLower(strings.TrimSpace(url))
		if assets == nil || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return ""
		}
		if cached, ok := imageCache[url]; ok {
			return cached
		}
		uri, err := assets.ReadImage(id, url)
		if err != nil {
			uri = ""
		}
		if imageCache == nil {
			imageCache = map[string]string{}
		}
		imageCache[url] = uri
		return uri
	}, exportRenderers(dark))
}

// exportBody 生成当前文档的 HTML 正文，本地图片内嵌为数据地址，导出的文件可以单独打开。
func (a *nativeApp) exportBody(dark bool) (name, body string, ok bool) {
	id, name, markdown, ok := a.exportSnapshot()
	if !ok {
		return "", "", false
	}
	return name, renderExportBody(a.assets, id, markdown, dark), true
}

// exportStyle 是导出文档的版式：纸面配色与应用一致，打印时去掉背景色块以外的装饰。
var exportStyle = exportThemeStyle(false) + exportThemeStyle(true) + exportLayoutStyle

func exportThemeStyle(dark bool) string {
	t := nativeTheme(dark)
	selector, scheme := ":root", "light"
	if dark {
		selector, scheme = ":root.dark", "dark"
	}
	hex := func(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A) }
	return fmt.Sprintf("%s { color-scheme: %s; --paper: %s; --ink: %s; --heading: %s; --muted: %s; --accent: %s; --surface: %s; --code-surface: %s; --callout: %s; --border: %s; --rule: %s; --bullet: %s; --mark: #e9b94959; }\n",
		selector, scheme, hex(t.Background), hex(t.Text), hex(t.Text), hex(t.TextMuted), hex(t.Accent), hex(t.Surface),
		hex(nativeeditor.CodeBackground(t)), hex(nativeeditor.CalloutBackground(t)), hex(t.Border), hex(nativeeditor.QuoteRule(t)), hex(t.TextMuted.Alpha(.75)))
}

const exportLayoutStyle = `
* { box-sizing: border-box; }
body { margin: 0; background: var(--paper); color: var(--ink); font: 16px/1.6 "Newsreader", "Songti SC", "STSong", "Noto Serif CJK SC", "SimSun", Georgia, serif; }
main { max-width: 856px; margin: 0 auto; padding: 48px 40px 80px; }
h1, h2, h3, h4, h5, h6 { color: var(--heading); font-weight: 500; line-height: 1.35; margin: 1.4em 0 .6em; }
h1 { font-size: 1.89em; margin-top: 0; } h2 { font-size: 1.28em; } h3 { font-size: 1.06em; }
p, ul, ol, blockquote, pre, table, .callout { margin: 0 0 1.3em; }
a { color: var(--accent); }
ul, ol { padding-left: 1.4em; } li > ul, li > ol { margin-bottom: 0; }
ul > li::marker { color: var(--bullet); }
li + li { margin-top: calc(4em / 15); }
li.task { list-style: none; margin-left: -1.4em; }
blockquote { margin-left: 0; padding: 10px 16px; border-left: 3px solid var(--rule); border-radius: 10px; background: var(--surface); color: var(--muted); }
blockquote p { margin: 0; }
.callout { padding: 12px 16px; border-radius: 10px; background: var(--callout); font: 13px/1.5 "Inter", system-ui, sans-serif; }
.callout p { margin: 0; } .callout-title { font-weight: 600; }
code, kbd { font: .82em "Geist Mono", "SFMono-Regular", Consolas, monospace; background: var(--surface); border-radius: 3px; padding: 1px 4px; }
pre { padding: 14px 18px; border-radius: 10px; background: var(--code-surface); overflow-x: auto; }
pre code { background: none; padding: 0; font-size: 14px; line-height: 1.75; }
mark { background: var(--mark); color: inherit; border-radius: 2px; }
.math { font-style: italic; }
table { width: 100%; border-collapse: separate; border-spacing: 0; }
th, td { padding: 7px 11px; border-bottom: 1px solid var(--border); text-align: left; }
th { background: var(--surface); font-weight: 600; border-bottom: 0; } th:first-child { border-radius: 10px 0 0 10px; } th:last-child { border-radius: 0 10px 10px 0; } th:only-child { border-radius: 10px; } tbody tr:last-child > * { border-bottom: 0; }
hr { border: 0; border-top: 1px solid var(--border); margin: 2em 0; }
img { max-width: 100%; }
.math svg { vertical-align: middle; }
.math-display { margin: 0 0 1.3em; text-align: center; }
.math-display svg { max-width: 100%; height: auto; }
.diagram { margin: 0 0 1.3em; text-align: center; }
.diagram svg { max-width: 100%; height: auto; }
.footnotes { margin-top: 2.6em; padding-top: 1em; border-top: 1px solid var(--border); color: var(--muted); font-size: .92em; }
.footnotes ol { padding-left: 1.4em; }
.footnote-back { margin-left: .4em; text-decoration: none; }
@media print { html, body { background: var(--paper); -webkit-print-color-adjust: exact; print-color-adjust: exact; } main { max-width: none; padding: 0; } pre, table, blockquote, .callout, img, .diagram, .math-display { break-inside: avoid; } h1, h2, h3 { break-after: avoid; } }
`

// exportRenderers 把公式和流程图排成自包含的 SVG。排版失败时返回 false，导出退回源码。
func exportRenderers(dark bool) richtext.Renderers {
	diagramStyle := exportDiagramStyle(dark)
	return richtext.Renderers{
		InlineMath: func(src string) (string, float32, bool) {
			box, err := mathlayout.Layout(src, exportFontSize, false)
			if err != nil || box == nil {
				return "", 0, false
			}
			svg, err := box.SVG()
			return svg, box.Descent, err == nil
		},
		DisplayMath: func(src string) (string, bool) {
			box, err := mathlayout.Layout(src, exportFontSize, true)
			if err != nil || box == nil {
				return "", false
			}
			svg, err := box.SVG()
			return svg, err == nil
		},
		Diagram: func(src string) (string, bool) { return exportDiagramWithStyle(src, diagramStyle) },
	}
}

// exportDiagram 把 Mermaid 源码排成 SVG。排版失败时返回 false，导出退回源码。
func exportDiagram(src string, dark bool) (string, bool) {
	return exportDiagramWithStyle(src, exportDiagramStyle(dark))
}

func exportDiagramWithStyle(src string, style diagram.Style) (string, bool) {
	graph, err := diagram.Parse(src)
	if err != nil || graph == nil {
		return "", false
	}
	svg := graph.Layout(style, exportContentWidth).SVG()
	return svg, svg != ""
}

// exportDiagramStyle 使用导出时的主题快照，与正文和原生图表配色一致。
func exportDiagramStyle(dark bool) diagram.Style {
	return nativeeditor.DiagramStyle(nativeTheme(dark))
}

const (
	exportFontSize     = 16  // 与导出正文的字号一致，公式基线才能对齐
	exportContentWidth = 776 // 856 的版心减去两侧 40 的内边距
)

// exportDocument 把正文包成完整的 HTML 文档。
func exportDocument(name, body string, dark bool) string {
	class := ""
	if dark {
		class = ` class="dark"`
	}
	title := strings.TrimSuffix(name, filepath.Ext(name))
	escaped := html.EscapeString(title)
	return "<!doctype html>\n<html lang=\"zh-CN\"" + class + ">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>" + escaped + "</title>\n<style>" + exportStyle + "</style>\n</head>\n<body>\n<main>\n" + body + "</main>\n</body>\n</html>\n"
}

// exportName 把文档名换成导出用的文件名建议。
func exportName(name, ext string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" {
		name = "未命名"
	}
	return strings.TrimSuffix(name, filepath.Ext(name)) + ext
}

func (a *nativeApp) exportHTML() {
	dark := a.exportDark()
	id, name, markdown, ok := a.exportSnapshot()
	if !ok {
		return
	}
	win, assets := a.window(), a.assets
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "导出 HTML", DefaultPath: exportName(name, ".html"),
			Filters:           []mygo.FileFilter{{Name: "HTML 文档", Extensions: []string{"html", "htm"}}},
			CreateDirectories: true,
		})
		if err == nil && target != "" {
			docHTML := exportDocument(name, renderExportBody(assets, id, markdown, dark), dark)
			err = workspace.WriteHTML(target, docHTML)
		}
		a.exportDone(target, err)
	}()
}

func (a *nativeApp) exportMarkdown() {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	doc, _ := a.document(tab.id)
	id, content := tab.id, tab.editor.Markdown()
	win := a.window()
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "导出 Markdown 副本", DefaultPath: exportName(doc.Name, ".md"),
			Filters:           []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
			CreateDirectories: true,
		})
		if err == nil && target != "" {
			err = a.files.store.ExportCopy(id, content, target)
		}
		a.exportDone(target, err)
	}()
}

// writeExportPDF 通过同目录临时文件原子写入 PDF，拒绝覆盖符号链接或非普通文件。
func writeExportPDF(target string, pdf []byte) error {
	mode := os.FileMode(0644)
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("导出目标不是普通文件")
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".leafmark-*.pdf.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(pdf); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// exportPDF 用系统自带的网页引擎把导出的 HTML 排成 A4 页面。
// 引擎只在导出期间以不可见窗口存在，主界面仍是原生绘制。
func (a *nativeApp) exportPDF() {
	if a.exporting {
		return
	}
	dark := a.exportDark()
	id, name, markdown, ok := a.exportSnapshot()
	if !ok {
		return
	}
	win, assets := a.window(), a.assets
	a.exporting = true
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "导出 PDF", DefaultPath: exportName(name, ".pdf"),
			Filters:           []mygo.FileFilter{{Name: "PDF 文档", Extensions: []string{"pdf"}}},
			CreateDirectories: true,
		})
		if err == nil && target != "" {
			docHTML := exportDocument(name, renderExportBody(assets, id, markdown, dark), dark)
			var pdf []byte
			if pdf, err = renderPDF(docHTML); err == nil {
				err = writeExportPDF(target, pdf)
			}
		}
		a.update(func() {
			a.exporting = false
			a.setExportNotice(target, err)
		})
	}()
}

func (a *nativeApp) setExportNotice(target string, err error) {
	switch {
	case err != nil:
		a.notice = "导出失败：" + err.Error()
	case target != "":
		a.notice = "已导出 " + filepath.Base(target)
	}
}

func (a *nativeApp) exportDone(target string, err error) {
	a.update(func() { a.setExportNotice(target, err) })
}

// renderPDF 在不可见的网页窗口里加载 HTML 并打印成 PDF。须在后台协程调用。
func renderPDF(docHTML string) ([]byte, error) {
	var page *mygo.Window
	loaded := make(chan error, 2)
	mygo.RunOnMain(func() {
		page = mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: 794, Height: 1123})
		if page == nil {
			loaded <- errors.New("无法创建导出窗口")
			return
		}
		page.Page().OnDidFinishLoad(func() { loaded <- nil })
		page.Page().OnDidFailLoad(func(e *mygo.LoadError) { loaded <- fmt.Errorf("加载导出内容失败：%v", e) })
		page.Page().LoadHTML(docHTML, "")
	})
	if page == nil {
		return nil, errors.New("无法创建导出窗口")
	}
	defer mygo.RunOnMain(func() { page.Destroy() })
	select {
	case err := <-loaded:
		if err != nil {
			return nil, err
		}
	case <-time.After(20 * time.Second):
		return nil, errors.New("排版导出内容超时")
	}
	pdf, err := page.Page().PrintToPDF(mygo.PDFOptions{PageSize: mygo.PageA4, Background: true, Margins: &mygo.Margins{Top: 0.6, Right: 0.6, Bottom: 0.6, Left: 0.6}})
	if err != nil {
		return nil, err
	}
	if len(pdf) == 0 {
		return nil, errors.New("没有生成 PDF 内容")
	}
	return pdf, nil
}

// rawEditor 由真实编辑器实现：双击公式、图表等原文块时请求编辑源码。
type rawEditor interface {
	SetEditRaw(fn func(index int, source string))
	ReplaceRaw(index int, markdown string) bool
}

// editRaw 打开源码编辑框；确定后整块替换，改成受支持的语法即成为可直接编辑的内容。
func (a *nativeApp) editRaw(editor rawEditor, index int, source string) {
	if a.reading {
		return
	}
	a.rawText, a.rawOpen = source, true
	a.rawOK = func(markdown string) {
		if !a.reading && editor.ReplaceRaw(index, markdown) {
			a.syncEditor(a.active())
			a.refreshTitle()
		}
	}
}

func (a *nativeApp) viewRawEditor(c *ui.Context) {
	ui.Modal(c, &a.rawOpen, func() {
		ui.Text(c, "编辑源码").Bold().FontSize(scaled(c, 18))
		ui.Text(c, "公式、图表与其他扩展语法以 Markdown 原文保存。清空内容会删除这一块。").FontSize(scaled(c, 12)).TextColor(c.Theme().TextMuted)
		ui.TextArea(c, &a.rawText).Width(520).Height(240).Font(sourceFontFamily).FontSize(scaled(c, 13)).LineHeight(1.6).Label("原文源码").AutoFocus()
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Border(0, ui.Transparent).Cursor(ui.CursorPointer).Clicked() {
				a.rawOpen = false
			}
			if ui.PrimaryButton(c, "确定").Cursor(ui.CursorPointer).Clicked() || c.Shortcut(ui.Cmd, ui.KeyEnter) {
				if a.rawOK != nil {
					a.rawOK(a.rawText)
				}
				a.rawOpen = false
			}
		})
	})
}
