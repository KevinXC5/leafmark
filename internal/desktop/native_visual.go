package desktop

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"leafmark/internal/richtext"
	"leafmark/internal/workspace"
)

// 颜色、间距与图标沿用原版应用，GPU 渐变与阴影承载玻璃侧栏。
type nativePalette struct{ glassTop, glassBottom, glassEdge, shadow, tab, outline, outlineInk, switchBG ui.Color }

func nativeColors(dark bool) nativePalette {
	if dark {
		return nativePalette{ui.Hex("#42454b"), ui.Hex("#25282e"), ui.Hex("#687079"), ui.Hex("#00000055"), ui.Hex("#34363b"), ui.Hex("#343b44"), ui.Hex("#d3dce6"), ui.Hex("#292b2f")}
	}
	return nativePalette{ui.Hex("#f5f1e8"), ui.Hex("#eadfd0"), ui.Hex("#fffdf9b0"), ui.Hex("#7052392b"), ui.Hex("#eeeae2"), ui.Hex("#f1dac5"), ui.Hex("#965b3a"), ui.Hex("#e8e0d5")}
}
func nativeGlass(c *ui.Context) *ui.Element {
	p := nativeColors(c.Theme().Dark)
	return ui.Column(c).Radius(18).Gradient(p.glassTop, p.glassBottom, 180).Border(1, p.glassEdge).Shadow(3, 5, 24, 0, p.shadow).Padding(16).Gap(14)
}

// uiBaseFontSize 是界面设计稿对应的正文字号：字号设置取这个值时界面按原尺寸绘制。
const uiBaseFontSize = 15

// uiThemeFontSize 是默认字号下界面文字的基准字号。
const uiThemeFontSize = 12

// scaled 把设计稿里的字号、文字行高和随文图标按字号设置等比缩放，整个界面的文字随设置一起变化。
func scaled(c *ui.Context, v float32) float32 {
	return v * c.Theme().FontSize / uiThemeFontSize
}
func nativeIconButton(c *ui.Context, icon, label string) *ui.Element {
	b := ui.ButtonBase(c).Size(28, 28).Radius(10).Label(label).TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Surface).TextColor(c.Theme().Text)
	}
	b.Children(func() { ui.Icon(c, nativeIcons[icon]).Size(16, 16) })
	return b
}

// nativeCloseButton 是标签上的小号关闭按钮。
func nativeCloseButton(c *ui.Context, label string) *ui.Element {
	b := ui.ButtonBase(c).Size(18, 18).Radius(6).Label(label).TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Border).TextColor(c.Theme().Text)
	}
	b.Children(func() { ui.Icon(c, nativeIcons["x"]).Size(11, 11) })
	return b
}
func nativeTextButton(c *ui.Context, label string) *ui.Element {
	b := ui.ButtonBase(c).Height(scaled(c, 30)).Padding(4, 10).Radius(10).Label(label).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Surface)
	}
	b.Children(func() { ui.Text(c, label).FontSize(scaled(c, 12)).SingleLine() })
	return b
}
func (a *nativeApp) viewTitleBar(c *ui.Context, bar ui.TitleBar) {
	left := float32(12)
	w, _ := c.Size()
	if a.sidebar < 80 || w < 600 {
		left = max(8, bar.Left+8)
	}
	ui.Row(c).Height(52).Padding(8, max(12, bar.Right), 8, left).Gap(4).AlignItems(ui.Center).DragWindow().Children(func() {
		if a.sidebar < 80 || w < 600 {
			if nativeIconButton(c, "panel-left", "展开侧栏").Clicked() {
				a.command("sidebar")
			}
		}
		// 滚动条位于裁剪区下方，标签仍可横向滚动；右侧操作始终完整显示。
		ui.Box(c).Grow(1).MinWidth(0).Height(36).Clip().Children(func() {
			ui.ScrollHorizontal(c).FillWidth().Height(48).Shrink(0).AlignItems(ui.Start).TrackScroll(&a.tabScroll).Children(func() { a.viewTabs(c) })
		})
		if nativeIconButton(c, "plus", "新建文档").Shrink(0).Clicked() {
			a.command("new")
		}
		nativeIconButton(c, "ellipsis", "更多操作").Shrink(0).Menu(a.actionMenu)
	})
}
func (a *nativeApp) actionMenu(m *ui.Menu) {
	choose := func(items []struct{ id, label string }) {
		for _, item := range items {
			entry := m.Item(item.label)
			if a.reading && editingCommand(item.id) {
				entry.Disabled(true)
			}
			if entry.Chosen() {
				a.command(item.id)
			}
		}
	}
	choose([]struct{ id, label string }{{"new", "新建文档"}, {"open", "打开文件…"}, {"folder", "打开文件夹…"}})
	m.Separator()
	choose([]struct{ id, label string }{{"save", "保存文档"}, {"save-as", "另存为…"}, {"reload", "从磁盘重新加载"}, {"close-tab", "关闭标签"}})
	m.Submenu("导出", func(m *ui.Menu) {
		for _, item := range []struct{ id, label string }{{"export-html", "导出 HTML…"}, {"export-pdf", "导出 PDF…"}, {"export-markdown", "导出 Markdown 副本…"}} {
			if m.Item(item.label).Chosen() {
				a.command(item.id)
			}
		}
	})
	m.Separator()
	choose([]struct{ id, label string }{{"find", "查找"}})
	source := "切换到源码模式"
	if sourceMode(a.active()) {
		source = "切换到原位编辑"
	}
	reading := "进入阅读模式"
	if a.reading {
		reading = "退出阅读模式"
	}
	if m.Item(reading).Chosen() {
		a.command("reading")
	}
	if m.Item(source).Chosen() {
		a.command("source")
	}
	m.Submenu("格式", func(m *ui.Menu) {
		for _, item := range nativeFormats {
			if item.id == "" {
				m.Separator()
				continue
			}
			entry := m.Item(item.label)
			// 阅读模式下正文不可改，格式菜单整组置灰。
			if a.reading {
				entry.Disabled(true)
			}
			if entry.Chosen() {
				a.command(item.id)
			}
		}
	})
	m.Separator()
	if m.Item("设置").Chosen() {
		a.command("settings")
	}
}

// nativeFormats 是“格式”菜单的全部条目，空 id 表示分隔线。
var nativeFormats = []struct{ id, label, icon string }{
	{"undo", "撤销", "undo"}, {"redo", "重做", "redo"}, {},
	{"bold", "加粗", "bold"}, {"italic", "斜体", "italic"}, {"strike", "删除线", "strikethrough"}, {"code", "行内代码", "code"}, {},
	{"paragraph", "正文", ""}, {"heading1", "一级标题", "heading-1"}, {"heading2", "二级标题", "heading-2"}, {"heading3", "三级标题", "heading-3"}, {},
	{"bullet", "无序列表", "list"}, {"ordered", "有序列表", "list-ordered"}, {"task", "任务列表", "list-checks"}, {"quote", "引用", "quote"}, {},
	{"link", "链接", "link"}, {"image", "图片", "image"}, {"table", "表格", "table"}, {"codeblock", "代码块", "square-code"}, {"hr", "分隔线", "minus"},
}

func (a *nativeApp) viewTabs(c *ui.Context) {
	ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
		for i, tab := range a.tabs {
			index := i
			doc, _ := a.document(tab.id)
			name := doc.Name
			if name == "" {
				name = "未命名.md"
			}
			ui.Box(c).Key(tab.id).Children(func() {
				row := ui.Row(c).Height(36).MaxWidth(240).Radius(10).Padding(0, 8).AlignItems(ui.Center)
				if i == a.current {
					row.Background(nativeColors(c.Theme().Dark).tab)
					if a.visibleTabID != tab.id {
						row.ScrollIntoView()
						a.visibleTabID = tab.id
					}
				}
				row.Children(func() {
					b := ui.ButtonBase(c).Height(36).MaxWidth(200).MinWidth(0).Gap(8).Padding(0, 4).Label(name).TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer)
					if i == a.current {
						b.TextColor(c.Theme().Text).FontWeight(500)
					}
					b.Children(func() {
						ui.Icon(c, nativeIcons["file-text"]).Size(scaled(c, 12), scaled(c, 12)).Shrink(0)
						ui.Text(c, name).FontSize(scaled(c, 12)).SingleLine().MinWidth(0).Shrink(1)
						if doc.Dirty || a.pendingEdit(tab) {
							ui.Text(c, "●").FontSize(scaled(c, 7)).TextColor(c.Theme().Accent)
						}
					})
					if b.Clicked() {
						a.selectTab(index)
					}
					if nativeCloseButton(c, "关闭 "+name).Clicked() {
						a.selectTab(index)
						a.command("close-tab")
					}
				})
			})
		}
	})
}
func (a *nativeApp) viewSidebar(c *ui.Context) {
	nativeGlass(c).Fill().Label("文档导航").Children(func() {
		bar := c.TitleBar()
		// macOS 的红绿灯压在侧栏左上角，品牌标识让出它的位置。
		brandLeft := float32(0)
		if bar.Left > 0 {
			brandLeft = 82
		}
		ui.Row(c).Height(30).Padding(0, 0, 0, brandLeft).Gap(6).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Icon(c, nativeIcons["leaf"]).Size(scaled(c, 16), scaled(c, 16)).Shrink(0).TextColor(c.Theme().TextMuted)
			ui.Text(c, "叶笺").FontSize(scaled(c, 12)).TextColor(c.Theme().TextMuted)
			ui.Spacer(c)
			if nativeIconButton(c, "panel-left", "收起侧栏").Clicked() {
				a.command("sidebar")
			}
		})
		ui.Row(c).Height(scaled(c, 34)).Padding(3).Gap(3).Radius(10).Background(nativeColors(c.Theme().Dark).switchBG).Children(func() {
			for _, item := range []struct{ id, label string }{{"documents", "文档"}, {"outline", "大纲"}} {
				selected := a.sidebarMode == item.id || (a.sidebarMode == "" && item.id == "outline")
				ink := c.Theme().TextMuted
				if selected {
					ink = c.Theme().Text
				}
				b := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(scaled(c, 28)).Radius(10).Label(item.label).Cursor(ui.CursorPointer)
				if selected {
					b.Background(c.Theme().Background).Shadow(0, 1, 4, 0, nativeColors(c.Theme().Dark).shadow)
				}
				b.Children(func() { ui.Text(c, item.label).FontSize(scaled(c, 11)).TextColor(ink).SingleLine() })
				if b.Clicked() {
					a.sidebarMode = item.id
				}
			}
		})
		// 滚动区向右伸进面板的内边距，滚动条贴着面板边缘，不压在文件名上。
		ui.Scroll(c).Grow(1).MinHeight(0).Margin(0, -13, 0, 0).Padding(0, 13, 0, 0).Children(func() {
			if a.sidebarMode == "documents" {
				a.viewDocuments(c)
			} else {
				a.viewOutline(c)
			}
		})
	})
}

type nativeHeading struct {
	text      string
	level, at int
}

func nativeOutline(markdown string) []nativeHeading {
	doc := richtext.Parse(markdown)
	blocks := doc.Blocks()
	out := []nativeHeading{}
	// 与模型 Text 的线性化规则一致：每块之间一个换行，表格单元格用 tab 分隔。
	at := 0
	for _, b := range blocks {
		if b.Kind == richtext.Heading {
			var s strings.Builder
			for _, r := range b.Runs {
				s.WriteString(r.Text)
			}
			out = append(out, nativeHeading{s.String(), b.Level, at})
		}
		at += nativeBlockRunes(b) + 1
	}
	return out
}

// nativeSourceOutline 使用解析器的源码位置，避免代码围栏里的 # 被误认为标题。
func nativeSourceOutline(markdown string) []nativeHeading {
	raw := []byte(markdown)
	// 复用文档模型识别的 YAML 属性块，保留字节位置但不参与标题解析。
	if blocks := richtext.Parse(markdown).Blocks(); strings.HasPrefix(markdown, "---") && len(blocks) > 0 && blocks[0].Kind == richtext.Raw && strings.HasPrefix(markdown, blocks[0].Raw) {
		for i := range len(blocks[0].Raw) {
			if raw[i] != '\n' && raw[i] != '\r' {
				raw[i] = ' '
			}
		}
	}
	root := goldmark.New().Parser().Parse(text.NewReader(raw))
	out := []nativeHeading{}
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := node.(*ast.Heading)
		if !entering || !ok || h.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		at := h.Lines().At(0).Start
		// 跳到标题整行的起点，包含 ATX 标记；Setext 标题同样适用。
		for at > 0 && raw[at-1] != '\n' {
			at--
		}
		out = append(out, nativeHeading{string(h.Text(raw)), h.Level, utf8.RuneCountInString(markdown[:at])})
		return ast.WalkContinue, nil
	})
	return out
}

func nativeBlockRunes(b richtext.Block) int {
	n := 0
	count := func(runs []richtext.Run) {
		for _, r := range runs {
			n += utf8.RuneCountInString(r.Text)
		}
	}
	switch b.Kind {
	case richtext.Code:
		return utf8.RuneCountInString(b.Code)
	case richtext.Horizontal, richtext.Image, richtext.Raw:
		return 1
	case richtext.TableBlock:
		if b.Table != nil {
			for ri, row := range b.Table.Rows {
				if ri > 0 {
					n++
				}
				for ci, cell := range row {
					if ci > 0 {
						n++
					}
					count(cell.Runs)
				}
			}
		}
	default:
		count(b.Runs)
	}
	return n
}
func (a *nativeApp) viewOutline(c *ui.Context) {
	ui.Text(c, "当前文档").FontSize(scaled(c, 10)).TextColor(c.Theme().TextMuted).LetterSpacing(1.2).Padding(8, 4, 10, 4)
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	markdown := tab.editor.Markdown()
	isSource := sourceMode(tab)
	if markdown != a.outlineMarkdown || isSource != a.outlineSource || a.outlineHeadings == nil {
		a.outlineMarkdown, a.outlineSource = markdown, isSource
		if isSource {
			a.outlineHeadings = nativeSourceOutline(markdown)
		} else {
			a.outlineHeadings = nativeOutline(markdown)
		}
	}
	headings := a.outlineHeadings
	start, _ := tab.editor.Selection()
	active := -1
	for i, h := range headings {
		if h.at <= start {
			active = i
		}
	}
	for i, h := range headings {
		ui.Box(c).Key(fmt.Sprintf("heading:%d", h.at)).FillWidth().Children(func() {
			b := ui.ButtonBase(c).FillWidth().Height(scaled(c, 39)).Margin(0, 0, 6, 0).Padding(8, 8, 8, float32(8+(h.level-1)*12)).Gap(8).Radius(10).Justify(ui.Start).Label("大纲：" + h.text).TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer)
			if i == active {
				p := nativeColors(c.Theme().Dark)
				b.Background(p.outline).TextColor(p.outlineInk).FontWeight(500)
			} else if b.Hovered() {
				b.Background(c.Theme().Surface)
			}
			b.Children(func() {
				ui.Textf(c, "H%d", h.level).FontSize(scaled(c, 9))
				ui.Text(c, h.text).FontSize(scaled(c, 12)).SingleLine().Grow(1).MinWidth(0)
			})
			if b.Clicked() {
				tab.editor.SetSelection(h.at, h.at)
			}
		})
	}
	if len(headings) == 0 {
		ui.Text(c, "文档中的标题会显示在这里").FontSize(scaled(c, 11)).TextColor(c.Theme().TextMuted).Padding(8)
	}
}
func (a *nativeApp) viewDocuments(c *ui.Context) {
	ui.SearchField(c, &a.navQuery).Label("筛选 Markdown 文件").Placeholder("筛选 Markdown…").Height(scaled(c, 30)).FillWidth().FontSize(scaled(c, 11)).Radius(10)
	ui.Row(c).Height(scaled(c, 32)).Gap(4).AlignItems(ui.Center).Children(func() {
		name := a.folderName
		if name == "" {
			name = "打开文件夹…"
		}
		if nativeTextButton(c, name).Grow(1).MinWidth(0).Clicked() {
			a.command("folder")
		}
		nativeIconButton(c, "ellipsis", "文档导航操作").Menu(func(m *ui.Menu) {
			if m.Item("打开文件…").Chosen() {
				a.command("open")
			}
			if m.Item("打开文件夹…").Chosen() {
				a.command("folder")
			}
			m.Separator()
			if m.Item("在工作区新建文档…").Disabled(a.folderName == "").Chosen() {
				a.command("new-file")
			}
			if m.Item("在工作区新建文件夹…").Disabled(a.folderName == "").Chosen() {
				a.command("new-folder")
			}
			if m.Item("刷新").Chosen() {
				a.reloadNavigation()
			}
		})
	})
	a.viewTree(c, a.tree, 0)
	if len(a.recent) > 0 {
		ui.Text(c, "近期文档").FontSize(scaled(c, 10)).TextColor(c.Theme().TextMuted).Padding(12, 4, 8, 4)
		for _, item := range a.recent {
			if !nativeMatches(item.Name, a.navQuery) {
				continue
			}
			path := item.Path
			ui.Box(c).Key("recent:" + path).FillWidth().Children(func() {
				if nativeNavigationItem(c, item.Name, 8).Clicked() {
					a.openRecent(path)
				}
			})
		}
	}
}
func nativeMatches(name, query string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(query)))
}
func nativeTreeMatches(n workspace.Node, query string) bool {
	if nativeMatches(n.Name, query) {
		return true
	}
	for _, child := range n.Children {
		if nativeTreeMatches(child, query) {
			return true
		}
	}
	return false
}
func nativeNavigationItem(c *ui.Context, name string, inset float32) *ui.Element {
	b := ui.ButtonBase(c).FillWidth().Height(scaled(c, 30)).Radius(10).Padding(4, 8, 4, inset).Justify(ui.Start).Gap(6).Label(name).TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(c.Theme().Surface)
	}
	b.Children(func() {
		ui.Icon(c, nativeIcons["file-text"]).Size(scaled(c, 12), scaled(c, 12)).Shrink(0)
		ui.Text(c, name).FontSize(scaled(c, 11)).SingleLine().Grow(1).MinWidth(0)
	})
	return b
}
func (a *nativeApp) viewTree(c *ui.Context, nodes []workspace.Node, depth int) {
	for _, node := range nodes {
		current := node
		if !nativeTreeMatches(current, a.navQuery) {
			continue
		}
		ui.Box(c).Key("tree:" + current.Path).FillWidth().Children(func() {
			if !current.Directory {
				if nativeNavigationItem(c, current.Name, float32(8+depth*12)).ContextMenu(a.treeMenu(current)).Clicked() {
					a.openWorkspace(current.Path)
				}
				return
			}
			open := a.openDirs[current.Path] || strings.TrimSpace(a.navQuery) != ""
			icon := "chevron-right"
			if open {
				icon = "chevron-down"
			}
			b := ui.ButtonBase(c).FillWidth().Height(scaled(c, 30)).Radius(10).Padding(4, 8, 4, float32(8+depth*12)).Justify(ui.Start).Gap(6).Label(current.Name).TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(c.Theme().Surface)
			}
			b.Children(func() {
				ui.Icon(c, nativeIcons[icon]).Size(scaled(c, 12), scaled(c, 12)).Shrink(0)
				ui.Icon(c, nativeIcons["folder"]).Size(scaled(c, 12), scaled(c, 12)).Shrink(0)
				ui.Text(c, current.Name).FontSize(scaled(c, 11)).SingleLine().Grow(1).MinWidth(0)
			})
			b.ContextMenu(a.treeMenu(current))
			if b.Clicked() {
				a.openDirs[current.Path] = !a.openDirs[current.Path]
			}
			if open {
				a.viewTree(c, current.Children, depth+1)
			}
		})
	}
}
func (a *nativeApp) viewEditor(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		if a.findOn {
			a.viewFind(c)
		}
		tab := a.active()
		if tab == nil || tab.editor == nil {
			ui.Text(c, "没有打开的文档").Padding(24)
		} else {
			ui.Box(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				tab.editor.View(c)
				a.viewFormatBar(c, tab.editor)
			})
		}
		a.viewStatus(c)
	})
}

// selectionAnchorer 由真实编辑器实现，报告选区起点在编辑区内的位置。
type selectionAnchorer interface {
	SelectionAnchor() (x, y, lineH, viewW float32, ok bool)
}

// nativeFormatBar 是选区浮动格式栏的按钮顺序。
var nativeFormatBar = []struct{ id, label, icon string }{
	{"bold", "加粗", "bold"}, {"italic", "斜体", "italic"}, {"strike", "删除线", "strikethrough"}, {"code", "行内代码", "code"},
	{"heading1", "一级标题", "heading-1"}, {"heading2", "二级标题", "heading-2"}, {"heading3", "三级标题", "heading-3"},
	{"bullet", "无序列表", "list"}, {"ordered", "有序列表", "list-ordered"}, {"task", "任务列表", "list-checks"}, {"quote", "引用", "quote"},
	{"link", "链接", "link"}, {"image", "图片", "image"}, {"table", "表格", "table"}, {"codeblock", "代码块", "square-code"}, {"hr", "分隔线", "minus"},
}

// viewFormatBar 只在选中文字后浮出，贴在选区上方；放不下时落到选区下方。
func (a *nativeApp) viewFormatBar(c *ui.Context, editor documentEditor) {
	if a.reading {
		return
	}
	anchor, ok := editor.(selectionAnchorer)
	if !ok {
		return
	}
	x, y, lineH, viewW, shown := anchor.SelectionAnchor()
	if !shown {
		return
	}
	const barW, barH = 560, 46
	left := max(8, min(x-12, viewW-barW-8))
	top := y - barH - 6
	if top < 6 {
		top = y + lineH + 6
	}
	bg, line := c.Theme().Background, c.Theme().Border
	if c.Theme().Dark {
		bg, line = ui.Hex("#303238"), ui.Hex("#4d5159")
	}
	bar := ui.Row(c).Absolute().Left(left).Top(top).Padding(6, 8).Gap(2).AlignItems(ui.Center).Radius(9).Background(bg).Border(1, line).Shadow(0, 4, 14, 0, nativeColors(c.Theme().Dark).shadow).Label("格式工具栏")
	bar.Children(func() {
		for _, item := range nativeFormatBar {
			if nativeIconButton(c, item.icon, item.label).Size(32, 32).Clicked() {
				a.command(item.id)
				// 点按钮会带走键盘焦点，格式生效后交还给正文。
				if f, ok := editor.(interface{ Focus() }); ok {
					f.Focus()
				}
			}
		}
	})
}
func (a *nativeApp) viewFind(c *ui.Context) {
	ui.Row(c).Padding(4, 12).Gap(6).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		field := ui.SearchField(c, &a.findQuery).Label("查找").Height(scaled(c, 30)).Grow(1).MinWidth(0)
		if field.Changed() || field.Submitted() {
			a.runFind()
		}
		if nativeIconButton(c, "chevron-down", "下一个").Clicked() {
			a.runFind()
		}
		// 替换框是文本输入，保持文本光标；阅读模式下不能替换，也不暗示可点击。
		ui.TextInput(c, &a.replaceText).Label("替换为").Placeholder("替换为").Height(scaled(c, 30)).Grow(1).MinWidth(0)
		replace := func(label string, all bool) {
			button := nativeTextButton(c, label)
			if a.reading {
				button.Cursor(ui.CursorDefault).Disabled(true).Opacity(.45)
			}
			if button.Clicked() {
				a.runReplace(all)
			}
		}
		replace("替换", false)
		replace("全部替换", true)
		if a.findNote != "" {
			ui.Text(c, a.findNote).FontSize(scaled(c, 11)).TextColor(c.Theme().TextMuted)
		}
		if nativeIconButton(c, "x", "关闭查找").Clicked() {
			a.findOn = false
		}
	})
}

// nativeWordCount 按 Markdown 原文统计字数：汉字逐字计数，其他文字和数字按连续词计数。
func nativeWordCount(markdown string) int {
	count, inWord := 0, false
	for _, r := range markdown {
		switch {
		case r < 128:
			isWord := (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
			if isWord && !inWord {
				count++
			}
			inWord = isWord
		case unicode.Is(unicode.Han, r):
			count++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if !inWord {
				count++
			}
			inWord = true
		case !unicode.IsMark(r):
			inWord = false
		}
	}
	return count
}
func (a *nativeApp) viewStatus(c *ui.Context) {
	tab := a.active()
	count := 0
	status := "就绪"
	if tab != nil && tab.editor != nil {
		// 字数随原文缓存，避免每帧重新扫描整篇文档。
		if markdown := tab.editor.Markdown(); markdown != a.statsMarkdown || a.statsWords < 0 {
			a.statsMarkdown, a.statsWords = markdown, nativeWordCount(markdown)
		}
		count = a.statsWords
		doc, _ := a.document(tab.id)
		status = "已保存"
		if doc.Path == "" {
			status = "未保存"
		}
		if doc.Dirty || a.pendingEdit(tab) {
			status = "未保存"
		}
		if a.saving[tab.id] {
			status = "正在保存…"
		}
		if a.conflicts[tab.id] {
			status = "文件冲突，请另存为"
		}
	}
	ui.Row(c).Height(scaled(c, 34)).Padding(0, 24).Gap(12).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).FontSize(scaled(c, 11)).TextColor(c.Theme().TextMuted).Label("状态栏").Children(func() {
		ui.Row(c).Gap(16).Children(func() { ui.Textf(c, "%d 字", count); ui.Textf(c, "%d 分钟阅读", max(1, (count+299)/300)) })
		ui.Spacer(c)
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, status)
			ui.Text(c, "UTF-8")
			// 按钮文字只反映源码切换的方向，阅读状态单独标注，两者可以同时成立。
			mode, other := "• 原位编辑", "源码"
			if sourceMode(tab) {
				mode, other = "• 源码模式", "原位"
			}
			if a.reading {
				if sourceMode(tab) {
					mode = "• 源码只读"
				} else {
					mode = "• 阅读模式"
				}
			}
			ui.Text(c, mode)
			toggle := ui.ButtonBase(c).Height(scaled(c, 22)).Padding(0, 6).Radius(6).Label("切换原位编辑与源码").Cursor(ui.CursorPointer)
			if toggle.Hovered() {
				toggle.Background(c.Theme().Surface)
			}
			toggle.Children(func() { ui.Text(c, other).FontSize(scaled(c, 11)).FontWeight(500).TextColor(c.Theme().Accent) })
			if toggle.Clicked() {
				a.command("source")
			}
		})
	})
}

// 图标路径取自项目安装的 lucide，沿用其 ISC 授权。
var nativeIcons = map[string]*ui.SVG{
	"square-pen":         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z"/></svg>`)),
	"sun":                ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>`)),
	"command":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M15 6v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3"/></svg>`)),
	"sliders-horizontal": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M10 5H3"/><path d="M12 19H3"/><path d="M14 3v4"/><path d="M16 17v4"/><path d="M21 12h-9"/><path d="M21 19h-5"/><path d="M21 5h-7"/><path d="M8 10v4"/><path d="M8 12H3"/></svg>`)),
	"check":              ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`)),
	"monitor":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="14" x="2" y="3" rx="2"/><line x1="8" x2="16" y1="21" y2="21"/><line x1="12" x2="12" y1="17" y2="21"/></svg>`)),
	"heading-1":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h8"/><path d="M4 18V6"/><path d="M12 18V6"/><path d="m17 12 3-2v8"/></svg>`)),
	"heading-2":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h8"/><path d="M4 18V6"/><path d="M12 18V6"/><path d="M21 18h-4c0-4 4-3 4-6 0-1.5-2-2.5-4-1"/></svg>`)),
	"heading-3":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h8"/><path d="M4 18V6"/><path d="M12 18V6"/><path d="M17.5 10.5c1.7-1 3.5 0 3.5 1.5a2 2 0 0 1-2 2"/><path d="M17 17.5c2 1.5 4 .3 4-1.5a2 2 0 0 0-2-2"/></svg>`)),
	"list-ordered":       ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5h10"/><path d="M11 12h10"/><path d="M11 19h10"/><path d="M4 4h1v5"/><path d="M4 9h2"/><path d="M6.5 20H3.4c0-1 2.6-1.925 2.6-3.5a1.5 1.5 0 0 0-2.6-1.02"/></svg>`)),
	"table":              ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v18"/><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18"/><path d="M3 15h18"/></svg>`)),
	"square-code":        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m10 9-3 3 3 3"/><path d="m14 15 3-3-3-3"/><rect x="3" y="3" width="18" height="18" rx="2"/></svg>`)),
	"minus":              ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/></svg>`)),
	"refresh-cw":         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg>`)),
	"folder-plus":        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M12 10v6"/><path d="M9 13h6"/><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>`)),
	"file-plus":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"/><path d="M14 2v5a1 1 0 0 0 1 1h5"/><path d="M9 15h6"/><path d="M12 18v-6"/></svg>`)),
	"download":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M12 15V3"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/></svg>`)),
	"code-xml":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m18 16 4-4-4-4"/><path d="m6 8-4 4 4 4"/><path d="m14.5 4-5 16"/></svg>`)),
	"leaf":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M11 20a10 10 0 0010-10 25.9 25.9 0 00-1.04-7.281 1 1 0 00-1.755-.325C15.833 5.5 13 5.5 9.8 6.1A7 7 0 0011 20"/><path d="M2 21a5 5 0 012.911-4.544C7.613 15.212 8.351 15.24 11 13"/></svg>`)),
	"panel-left":         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/></svg>`)),
	"file-text":          ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"/><path d="M14 2v5a1 1 0 0 0 1 1h5"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg>`)),
	"plus":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="M12 5v14"/></svg>`)),
	"x":                  ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`)),
	"ellipsis":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/></svg>`)),
	"settings":           ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/></svg>`)),
	"folder":             ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>`)),
	"chevron-right":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`)),
	"chevron-down":       ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`)),
	"search":             ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/></svg>`)),
	"bold":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M6 12h9a4 4 0 0 1 0 8H7a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h7a4 4 0 0 1 0 8"/></svg>`)),
	"italic":             ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><line x1="19" x2="10" y1="4" y2="4"/><line x1="14" x2="5" y1="20" y2="20"/><line x1="15" x2="9" y1="4" y2="20"/></svg>`)),
	"strikethrough":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M16 4H9a3 3 0 0 0-2.83 4"/><path d="M14 12a4 4 0 0 1 0 8H6"/><line x1="4" x2="20" y1="12" y2="12"/></svg>`)),
	"code":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m16 18 6-6-6-6"/><path d="m8 6-6 6 6 6"/></svg>`)),
	"heading":            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M6 12h12"/><path d="M6 20V4"/><path d="M18 20V4"/></svg>`)),
	"quote":              ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M16 3a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2 1 1 0 0 1 1 1v1a2 2 0 0 1-2 2 1 1 0 0 0-1 1v2a1 1 0 0 0 1 1 6 6 0 0 0 6-6V5a2 2 0 0 0-2-2z"/><path d="M5 3a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2 1 1 0 0 1 1 1v1a2 2 0 0 1-2 2 1 1 0 0 0-1 1v2a1 1 0 0 0 1 1 6 6 0 0 0 6-6V5a2 2 0 0 0-2-2z"/></svg>`)),
	"list":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3 5h.01"/><path d="M3 12h.01"/><path d="M3 19h.01"/><path d="M8 5h13"/><path d="M8 12h13"/><path d="M8 19h13"/></svg>`)),
	"list-checks":        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M13 5h8"/><path d="M13 12h8"/><path d="M13 19h8"/><path d="m3 17 2 2 4-4"/><path d="m3 7 2 2 4-4"/></svg>`)),
	"link":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`)),
	"image":              ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/></svg>`)),
	"undo":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7v6h6"/><path d="M21 17a9 9 0 0 0-9-9 9 9 0 0 0-6 2.3L3 13"/></svg>`)),
	"redo":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M21 7v6h-6"/><path d="M3 17a9 9 0 0 1 9-9 9 9 0 0 1 6 2.3l3 2.7"/></svg>`)),
	"arrow-left":         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>`)),
	"moon":               ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/></svg>`)),
}
