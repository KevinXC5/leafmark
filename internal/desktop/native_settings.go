package desktop

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// settingsPalette 是设置页自己的一套颜色，与书写界面的纸色略有区别。
type settingsPalette struct {
	paper, card, ink, muted, line, accent, control, controlLine, divider           ui.Color
	tabInk, tabSel, tabSelLine, tabIcon, segment, segmentOn, toggleOff, toggleOn   ui.Color
	groupTitle, note, noteLine, back, kbdLine, kbd, close, themeSeg, themeSegOn    ui.Color
	themeInk, previewLine, hintIcon, scopeLine, scopeInk, destination, destChevron ui.Color
}

func settingsColors(dark bool) settingsPalette {
	p := settingsPalette{
		paper: ui.Hex("#faf9f6"), card: ui.Hex("#ffffff"), ink: ui.Hex("#302d29"), muted: ui.Hex("#787269"), line: ui.Hex("#e5e0d8"),
		accent: ui.Hex("#a65136"), control: ui.Hex("#faf8f4"), controlLine: ui.Hex("#dedad2"), divider: ui.Hex("#ede9e2"),
		tabInk: ui.Hex("#655f56"), tabSel: ui.Hex("#eeddd080"), tabSelLine: ui.Hex("#e1c3ae99"), tabIcon: ui.Hex("#b65d3f"),
		segment: ui.Hex("#eeeae3"), segmentOn: ui.Hex("#f2e0d4"), toggleOff: ui.Hex("#dedad2"), toggleOn: ui.Hex("#c16a4c"),
		groupTitle: ui.Hex("#777065"), note: ui.Hex("#777065"), noteLine: ui.Hex("#b4aa993d"), back: ui.Hex("#786a5c"),
		kbdLine: ui.Hex("#beb1a0"), kbd: ui.Hex("#ffffff40"), close: ui.Hex("#9a8879"),
		themeSeg: ui.Hex("#ece5dd"), themeSegOn: ui.Hex("#fffcf7"), themeInk: ui.Hex("#817469"), previewLine: ui.Hex("#d6b6a3"),
		hintIcon: ui.Hex("#a16d55"), scopeLine: ui.Hex("#e1d7cd"), scopeInk: ui.Hex("#9a8879"), destination: ui.Hex("#766e63"), destChevron: ui.Hex("#827b71"),
	}
	if dark {
		p.paper, p.card, p.ink, p.muted, p.line = ui.Hex("#242527"), ui.Hex("#2b2c2f"), ui.Hex("#e5e6e9"), ui.Hex("#a6adb7"), ui.Hex("#45464c")
		p.divider, p.control, p.controlLine, p.accent = ui.Hex("#3b3d42"), ui.Hex("#303236"), ui.Hex("#45464c"), ui.Hex("#d99a7c")
		p.tabInk, p.tabSel, p.tabSelLine, p.tabIcon = p.muted, ui.Hex("#544039"), ui.Hex("#735343"), p.accent
		p.segment, p.segmentOn, p.toggleOff = ui.Hex("#383a3e"), ui.Hex("#544039"), ui.Hex("#4a4c52")
		p.groupTitle, p.note, p.back, p.destination, p.destChevron = p.muted, p.muted, p.muted, p.muted, p.muted
		p.kbdLine, p.kbd, p.close = ui.Hex("#5a5c63"), ui.Hex("#ffffff14"), p.muted
		p.themeSeg, p.themeSegOn, p.themeInk, p.scopeLine, p.scopeInk = ui.Hex("#383a3e"), ui.Hex("#544039"), p.accent, p.line, p.muted
	}
	return p
}

var settingsTabs = []struct{ id, label, icon, intro string }{
	{"editor", "编辑器", "square-pen", "为自己的书写节奏，留一点空间。"},
	{"appearance", "外观", "sun", "让每一份 Markdown，都有适合阅读的样子。"},
	{"shortcuts", "快捷键", "command", "当前快捷键绑定。"},
	{"general", "通用", "sliders-horizontal", "管理书写习惯与本机数据。"},
}

func (a *nativeApp) closeSettings() {
	a.settingsOn, a.shortcutOpen, a.confirmClear = false, false, ""
	a.persistSettings()
}

// viewSettings 是全窗口的设置页：左侧玻璃侧栏切换分类，右侧是该分类的选项。
func (a *nativeApp) viewSettings(c *ui.Context) {
	if !a.shortcutOpen && !a.updateOpen && c.Shortcut(0, ui.KeyEscape) {
		a.closeSettings()
		return
	}
	p := settingsColors(c.Theme().Dark)
	width, _ := c.Size()
	narrow, compact := width < 700, width < 540
	section := 0
	for i, tab := range settingsTabs {
		if tab.id == a.settingsSection {
			section = i
		}
	}
	// 元素一经创建就进入界面树，方向只能在同一个元素上切换。
	page := ui.Column(c).Fill().Padding(8).Gap(8).Background(p.paper).TextColor(p.ink)
	if !compact {
		page.Row()
	}
	page.Children(func() {
		a.viewSettingsSidebar(c, p, section, narrow, compact)
		main := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Padding(30, 30, 20, 30)
		if compact {
			main.Padding(18, 10, 10, 10)
		} else if narrow {
			main.Padding(24, 18, 16, 18)
		} else {
			main.FillHeight()
		}
		main.Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, settingsTabs[section].label).FontSize(28).FontWeight(600).LetterSpacing(-.8).Grow(1).MinWidth(0)
				b := ui.ButtonBase(c).Size(26, 26).Radius(4).Label("关闭设置").TextColor(p.close)
				if b.Hovered() {
					b.Background(p.ink.Alpha(.05))
				}
				b.Children(func() { ui.Icon(c, nativeIcons["x"]).Size(18, 18) })
				if b.Clicked() {
					a.closeSettings()
				}
			})
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 0, 20, 0).Children(func() {
				ui.Text(c, settingsTabs[section].intro).FontSize(13).LineHeight(1.5).TextColor(p.muted).Margin(9, 0, 22, 0)
				switch settingsTabs[section].id {
				case "appearance":
					a.viewSettingsAppearance(c, p, narrow)
				case "shortcuts":
					a.viewSettingsShortcuts(c, p)
				case "general":
					a.viewSettingsGeneral(c, p)
				default:
					a.viewSettingsEditor(c, p)
				}
			})
			ui.Row(c).Padding(10, 0, 0, 0).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, nativeIcons["check"]).Size(13, 13).TextColor(p.accent)
				status := a.settingsStatus
				if status == "" {
					status = "调整即时生效"
				}
				ui.Text(c, status).FontSize(11).TextColor(p.destChevron).Margin(0, 0, 0, -6)
				ui.Spacer(c)
				if settingsLink(c, p, "恢复默认设置").Clicked() {
					shortcuts := a.settings.Shortcuts
					a.changeSettings(func(s *nativeSettings) { *s = defaultNativeSettings(); s.Shortcuts = shortcuts })
					a.settingsStatus = "调整即时生效"
				}
			})
		})
	})
	a.viewShortcutDialog(c, p)
}

func (a *nativeApp) viewSettingsSidebar(c *ui.Context, p settingsPalette, section int, narrow, compact bool) {
	side := nativeGlass(c).Gap(0).Shrink(0).Label("设置导航")
	switch {
	case compact:
		side.FillWidth().Radius(10).Padding(8)
	case narrow:
		side.Width(170).FillHeight().Padding(20, 10, 16, 10)
	default:
		side.Width(248).FillHeight().Padding(24, 14, 18, 14)
	}
	if !compact && c.TitleBar().Left > 0 {
		side.Padding(48, 14, 18, 14) // 让出 macOS 的红绿灯
		if narrow {
			side.Padding(48, 10, 16, 10)
		}
	}
	side.Children(func() {
		if !compact {
			ui.Text(c, "Leafmark").FontSize(22).FontWeight(600).LetterSpacing(-.7).TextColor(p.ink).Margin(0, 12)
			ui.Text(c, "设置").FontSize(12).TextColor(p.muted).Margin(7, 12, 28, 12)
		}
		tabs := ui.Column(c).Gap(6)
		if compact {
			tabs.Row().Gap(3)
		}
		tabs.Label("设置分类").Children(func() {
			for i, tab := range settingsTabs {
				id := tab.id
				item := ui.Box(c).Key("settings:" + id).MinWidth(0)
				if compact {
					item.Grow(1).Basis(0) // 窄窗口里四个标签横向平分
				} else {
					item.FillWidth()
				}
				item.Children(func() {
					selected := i == section
					b := ui.ButtonBase(c).FillWidth().Height(42).Radius(10).Padding(0, 12).Gap(12).Justify(ui.Start).Border(1, ui.Color{}).Label(tab.label)
					ink, icon, weight := p.tabInk, p.muted, 500
					if selected {
						ink, icon, weight = p.accent, p.tabIcon, 600
						b.Background(p.tabSel).BorderColor(p.tabSelLine)
					} else if b.Hovered() {
						b.Background(p.ink.Alpha(.03))
					}
					if compact {
						b.Justify(ui.Center).Padding(0, 4).Gap(5)
					}
					b.Children(func() {
						ui.Icon(c, nativeIcons[tab.icon]).Size(18, 18).TextColor(icon)
						ui.Text(c, tab.label).FontSize(13).FontWeight(weight).TextColor(ink).SingleLine().MinWidth(0)
					})
					if b.Clicked() {
						a.settingsSection, a.confirmClear = id, ""
					}
				})
			}
		})
		if compact {
			return
		}
		ui.Spacer(c)
		ui.Column(c).BorderWidth(1, 0, 0, 0).BorderColor(p.noteLine).Padding(14, 12, 0, 12).Gap(8).Children(func() {
			ui.Text(c, "留一点空间，给书写。").FontSize(11).TextColor(p.note)
			ui.Text(c, "Leafmark").FontSize(10).TextColor(p.destChevron)
		})
		back := ui.ButtonBase(c).Margin(28, 0, 0, 0).Padding(8, 0, 0, 0).Gap(7).Justify(ui.Start).Label("返回书写")
		back.Children(func() {
			ui.Box(c).Padding(3, 6).Radius(4).Border(1, p.kbdLine).Background(p.kbd).Children(func() { ui.Text(c, "Esc").FontSize(10).TextColor(p.back) })
			ui.Text(c, "返回书写").FontSize(11).TextColor(p.back)
		})
		if back.Clicked() {
			a.closeSettings()
		}
	})
}

// settingsGroup 是带小标题的一组选项，卡片内各行用细线分隔。
func settingsGroup(c *ui.Context, p settingsPalette, title string, rows func()) {
	ui.Column(c).FillWidth().Margin(0, 0, 18, 0).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(600).LetterSpacing(1).TextColor(p.groupTitle).Margin(0, 0, 9, 0)
		ui.Column(c).FillWidth().Background(p.card).Border(1, p.line).Radius(10).Padding(0, 16).Children(rows)
	})
}

// settingsRow 是一行选项：左边名称与说明，右边控件。last 为 true 时不画底部分隔线。
func settingsRow(c *ui.Context, p settingsPalette, label, description string, last bool, control func()) {
	row := ui.Row(c).FillWidth().MinHeight(57).Padding(9, 0).Gap(16).AlignItems(ui.Center)
	if !last {
		row.BorderWidth(0, 0, 1, 0).BorderColor(p.divider)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.ink)
			if description != "" {
				ui.Text(c, description).FontSize(11).LineHeight(1.4).TextColor(p.muted)
			}
		})
		control()
	})
}

// settingsSelect 是下拉选择：外观是一枚带箭头的按钮，点开后用系统菜单选值。
func settingsSelect(c *ui.Context, p settingsPalette, label, current string, width float32, options []string, pick func(i int)) {
	b := ui.ButtonBase(c).Width(width).Height(30).Shrink(0).Radius(10).Border(1, p.controlLine).Background(p.control).Padding(0, 9, 0, 10).Gap(6).Label("选择" + label)
	b.Children(func() {
		ui.Text(c, current).FontSize(12).TextColor(p.destination).SingleLine().Grow(1).MinWidth(0)
		ui.Icon(c, nativeIcons["chevron-down"]).Size(14, 14).TextColor(p.destChevron)
	})
	b.Menu(func(m *ui.Menu) {
		for i, option := range options {
			if m.Item(option).Chosen() {
				pick(i)
			}
		}
	})
}

// settingsSegmented 是分段选择。theme 为 true 时用外观页较大的样式。
func settingsSegmented(c *ui.Context, p settingsPalette, labels []string, selected int, theme bool, pick func(i int)) {
	bg, on, ink, gap := p.segment, p.segmentOn, p.accent, float32(2)
	if theme {
		bg, on, ink, gap = p.themeSeg, p.themeSegOn, p.themeInk, 3
	}
	ui.Row(c).Shrink(0).Padding(3).Gap(gap).Radius(10).Background(bg).Children(func() {
		for i, label := range labels {
			b := ui.ButtonBase(c).Radius(10).Padding(5, 10).Label(label)
			size := float32(11)
			if theme {
				b.Padding(8, 16)
				size = 12
			}
			color, weight := p.muted, 400
			if i == selected {
				color = ink
				if !theme {
					weight = 500
				}
				b.Background(on)
			}
			b.Children(func() { ui.Text(c, label).FontSize(size).FontWeight(weight).TextColor(color).SingleLine() })
			if b.Clicked() {
				pick(i)
			}
		}
	})
}

// settingsToggle 是 34×20 的胶囊开关。
func settingsToggle(c *ui.Context, p settingsPalette, label string, on *bool) bool {
	b := ui.SwitchBase(c, on).Size(34, 20).Shrink(0).Radius(10).Label("切换" + label)
	track, left := p.toggleOff, float32(3)
	if *on {
		track, left = p.toggleOn, 17
	}
	b.Background(track).Children(func() {
		ui.Box(c).Absolute().Left(left).Top(3).Size(14, 14).Radius(7).Background(ui.Hex("#ffffff")).Shadow(0, 1, 2, 0, ui.Hex("#54443222"))
	})
	return b.Changed()
}

// settingsLink 是强调色的文字按钮。
func settingsLink(c *ui.Context, p settingsPalette, label string) *ui.Element {
	b := ui.ButtonBase(c).Radius(3).Padding(2, 2).Label(label)
	b.Children(func() { ui.Text(c, label).FontSize(11).TextColor(p.accent).SingleLine() })
	return b
}

// settingsOutlined 是带描边的小按钮，用于清理与自定义快捷键。
func settingsOutlined(c *ui.Context, p settingsPalette, label string) *ui.Element {
	b := ui.ButtonBase(c).Shrink(0).Radius(10).Padding(7, 10).Border(1, p.controlLine).Label(label)
	if b.Hovered() {
		b.Background(p.ink.Alpha(.03))
	}
	b.Children(func() { ui.Text(c, label).FontSize(11).TextColor(p.accent).SingleLine() })
	return b
}

var (
	settingsFontIDs     = []string{"newsreader", "serif", "sans", "mono"}
	settingsFontLabels  = []string{"Newsreader", "系统衬线", "系统无衬线", "等宽"}
	settingsWidthIDs    = []string{"narrow", "comfort", "wide"}
	settingsWidthLabels = []string{"窄", "舒适", "宽"}
	settingsThemeIDs    = []string{"light", "dark", "system"}
	settingsThemeLabels = []string{"浅色", "深色", "跟随系统"}
)

func indexOf(list []string, value string) int {
	for i, item := range list {
		if item == value {
			return i
		}
	}
	return 0
}

func (a *nativeApp) viewSettingsEditor(c *ui.Context, p settingsPalette) {
	settingsGroup(c, p, "排版与布局", func() {
		settingsRow(c, p, "正文字体", "", false, func() {
			at := indexOf(settingsFontIDs, a.settings.Font)
			settingsSelect(c, p, "正文字体", settingsFontLabels[at], 152, settingsFontLabels, func(i int) {
				a.changeSettings(func(s *nativeSettings) { s.Font = settingsFontIDs[i] })
			})
		})
		settingsRow(c, p, "字号", "", false, func() {
			ui.Row(c).Shrink(0).Height(30).Padding(0, 6).Gap(3).AlignItems(ui.Center).Radius(10).Border(1, p.controlLine).Background(p.control).Children(func() {
				step := func(label, sign string, delta float32, disabled bool) {
					b := ui.ButtonBase(c).Size(23, 26).Radius(10).Label(label).Disabled(disabled)
					if disabled {
						b.Opacity(.45)
					}
					b.Children(func() { ui.Text(c, sign).FontSize(16).TextColor(p.destChevron) })
					if b.Clicked() {
						a.setFont(a.settings.FontSize + delta)
					}
				}
				step("减小字号", "−", -1, a.settings.FontSize <= 12)
				ui.Text(c, strconv.Itoa(int(a.settings.FontSize))).FontSize(12).TextColor(p.destination).Width(25).TextAlign(ui.End).Label("字号")
				ui.Text(c, "px").FontSize(12).TextColor(p.destination).Margin(0, 7, 0, 0)
				step("增大字号", "+", 1, a.settings.FontSize >= 28)
			})
		})
		settingsRow(c, p, "行高", "", false, func() {
			var values []float32
			var labels []string
			for i := 0; i < 19; i++ {
				v := float32(130+i*5) / 100
				values, labels = append(values, v), append(labels, strconv.FormatFloat(float64(v), 'f', -1, 32))
			}
			current := strconv.FormatFloat(float64(a.settings.LineHeight), 'f', -1, 32)
			settingsSelect(c, p, "行高", current, 105, labels, func(i int) {
				a.changeSettings(func(s *nativeSettings) { s.LineHeight = values[i] })
			})
		})
		settingsRow(c, p, "阅读宽度", "", true, func() {
			settingsSegmented(c, p, settingsWidthLabels, indexOf(settingsWidthIDs, a.settings.ReadingWidth), false, func(i int) {
				a.changeSettings(func(s *nativeSettings) { s.ReadingWidth = settingsWidthIDs[i] })
			})
		})
	})
	settingsGroup(c, p, "书写与保存", func() {
		settingsRow(c, p, "自动保存", "停止输入两秒后保存已有路径的文档。", true, func() {
			if settingsToggle(c, p, "自动保存", &a.settings.AutoSave) {
				a.persistSettings()
			}
		})
	})
	settingsGroup(c, p, "个性化", func() {
		destination := func(label, description, text string, section string, swatch bool, last bool) {
			settingsRow(c, p, label, description, last, func() {
				b := ui.ButtonBase(c).Shrink(0).Radius(10).Padding(5, 0, 5, 8).Gap(9).Label("前往" + label)
				b.Children(func() {
					if swatch {
						ui.Box(c).Size(14, 14).Radius(7).Background(p.toggleOn).Border(1, p.tabIcon)
					}
					ui.Text(c, text).FontSize(12).TextColor(p.destination).SingleLine()
					ui.Icon(c, nativeIcons["chevron-right"]).Size(16, 16).TextColor(p.destChevron)
				})
				if b.Clicked() {
					a.settingsSection = section
				}
			})
		}
		theme := settingsThemeLabels[indexOf(settingsThemeIDs, a.settings.Theme)]
		destination("外观", "Leafmark 主题 · 浅色 / 深色 / 跟随系统", "Leafmark · "+theme, "appearance", true, false)
		destination("快捷键", "查看并自定义书写快捷键。", "自定义", "shortcuts", false, true)
	})
}

func (a *nativeApp) viewSettingsAppearance(c *ui.Context, p settingsPalette, narrow bool) {
	ui.Row(c).FillWidth().MinHeight(36).Margin(0, 0, 24, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "外观模式").FontSize(13).FontWeight(500).TextColor(p.ink).Grow(1).MinWidth(0)
		settingsSegmented(c, p, settingsThemeLabels, indexOf(settingsThemeIDs, a.settings.Theme), true, func(i int) { a.setTheme(settingsThemeIDs[i]) })
	})
	previews := ui.Column(c).FillWidth().Gap(18)
	if !narrow {
		previews.Row()
	}
	const book = `"Newsreader", "Songti SC", "STSong", "Noto Serif CJK SC", serif`
	previews.Children(func() {
		for _, dark := range []bool{false, true} {
			name, bg, line, ink, nameInk, text, quoteLine, quote, link := "Leafmark 浅色", ui.Hex("#fffcf7"), ui.Hex("#d6b6a3"), ui.Hex("#3d352f"), ui.Hex("#a16d55"), ui.Hex("#817469"), ui.Hex("#c18b6e"), ui.Hex("#a5684e"), ui.Hex("#bc7457")
			if dark {
				name, bg, line, ink, nameInk, text, quote, link = "Leafmark 深色", ui.Hex("#242527"), ui.Hex("#45464c"), ui.Hex("#e5e6e9"), ui.Hex("#b9a092"), ui.Hex("#a6adb7"), ui.Hex("#bac0ca"), ui.Hex("#d99a7c")
			}
			card := ui.Column(c).Key(name).Grow(1).Basis(0).MinWidth(0).MinHeight(340).Padding(24).Gap(18).Radius(10).Border(1, line).Background(bg).Label(name + "预览")
			card.Children(func() {
				ui.Text(c, name).Font(sourceFontFamily).FontSize(11).TextColor(nameInk)
				ui.Text(c, "在宁静中，\n看见清晰。").Font(book).FontSize(29).LineHeight(1.1).TextColor(ink)
				ui.Text(c, "写下一点，再读一遍。\n让下一个想法，慢慢成形。").Font(book).FontSize(16).LineHeight(1.6).TextColor(text)
				ui.Box(c).BorderWidth(0, 0, 0, 2).BorderColor(quoteLine).Padding(3, 0, 3, 12).Children(func() {
					ui.Text(c, "为重要的事，留一点空间。").Font(book).FontSize(15).Italic().TextColor(quote)
				})
				ui.Text(c, "循着一个小小的念头 ↗").FontSize(12).TextColor(link)
			})
		}
	})
	ui.Row(c).FillWidth().Gap(10).Margin(24, 0).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, nativeIcons["monitor"]).Size(16, 16).TextColor(p.hintIcon)
		ui.Text(c, "跟随系统时，自动使用对应的浅色或深色主题。").FontSize(12).TextColor(p.themeInk)
	})
	ui.Box(c).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(p.scopeLine).Padding(16, 0, 0, 0).Children(func() {
		ui.Text(c, "应用于所有打开的文档").FontSize(12).TextColor(p.scopeInk)
	})
}

func (a *nativeApp) viewSettingsShortcuts(c *ui.Context, p settingsPalette) {
	for _, item := range shortcutActions {
		ui.Row(c).Key("shortcut:"+item.id).FillWidth().Padding(14, 0).Gap(20).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.line).Children(func() {
			ui.Text(c, item.label).FontSize(12).TextColor(p.ink).Grow(1).MinWidth(0)
			ui.Box(c).Padding(5, 7).Radius(4).Border(1, p.line).Background(p.control).Children(func() {
				ui.Text(c, *a.settings.Shortcuts.binding(item.id)).Font(sourceFontFamily).FontSize(10).TextColor(p.muted)
			})
		})
	}
	ui.Row(c).Margin(20, 0, 0, 0).Children(func() {
		if settingsOutlined(c, p, "自定义快捷键…").Clicked() {
			a.shortcutOpen, a.shortcutAction = true, shortcutActions[0].id
			a.shortcutCandidate, a.shortcutStatus, a.shortcutRecording = "", "", false
		}
	})
}

func (a *nativeApp) viewSettingsGeneral(c *ui.Context, p settingsPalette) {
	if a.updates != nil {
		status := a.updates.Status()
		version := status.Version
		if version == "" {
			version = "开发版"
		}
		note := a.updateNote
		switch {
		case a.updateBusy:
			note = "正在检查更新…"
		case status.Installed:
			note = "更新已安装，下次启动生效。请先保存文档后再关闭应用。"
		case !status.Enabled:
			note = "开发版或安装目录不可写时无法自动更新。"
		case status.Available != "":
			note = "新版本 " + status.Available + " 可供安装。"
		}
		settingsGroup(c, p, "软件更新", func() {
			settingsRow(c, p, "Leafmark", "当前版本："+version, note == "", func() {
				ui.Row(c).Shrink(0).Gap(14).AlignItems(ui.Center).Children(func() {
					check := settingsLink(c, p, "检查更新")
					if disabled := !status.Enabled || status.Installed || a.updateBusy; disabled {
						check.Disabled(true).Opacity(.45)
					} else if check.Clicked() {
						a.checkUpdate()
					}
					switch {
					case status.Installed:
						if settingsLink(c, p, "重启应用").Clicked() {
							if err := a.updates.Restart(); err != nil {
								a.notice = err.Error()
							}
						}
					case status.Available != "":
						if settingsLink(c, p, "查看更新").Clicked() {
							a.updateOpen = true
						}
					}
				})
			})
			if note != "" {
				ui.Text(c, note).FontSize(11).LineHeight(1.4).TextColor(p.muted).Padding(10, 0)
			}
		})
	}
	settingsGroup(c, p, "书写习惯", func() {
		settingsRow(c, p, "专注模式", "淡化当前段落之外的文字。", false, func() {
			on := a.settings.FocusMode
			if settingsToggle(c, p, "专注模式", &on) {
				a.changeSettings(func(s *nativeSettings) { s.FocusMode = on })
			}
		})
		settingsRow(c, p, "打字机模式", "让光标所在行保持在视窗中央。", true, func() {
			on := a.settings.Typewriter
			if settingsToggle(c, p, "打字机模式", &on) {
				a.changeSettings(func(s *nativeSettings) { s.Typewriter = on })
			}
		})
	})
	ui.Text(c, "恢复默认设置只影响偏好设置，不清理文档。").FontSize(11).LineHeight(1.4).TextColor(p.muted).Margin(4, 0, 0, 0)
	for _, item := range []struct{ label, description string }{
		{"清理恢复草稿", "删除用于意外退出后恢复的草稿。"},
		{"清理历史记录", "清理应用保存的历史记录。"},
	} {
		label := item.label
		ui.Column(c).Key("clear:" + label).FillWidth().Children(func() {
			settingsRow(c, p, label, item.description, false, func() {
				if settingsOutlined(c, p, label).Label("开始" + label).Clicked() {
					a.confirmClear = label
				}
			})
			if a.confirmClear != label {
				return
			}
			ui.Column(c).FillWidth().Padding(12).Margin(10, 0, 0, 0).Gap(10).Radius(5).Background(p.control).Children(func() {
				ui.Text(c, "清理后无法恢复，是否继续？").FontSize(11).TextColor(p.muted)
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if settingsOutlined(c, p, "确认清理").Clicked() {
						a.clearData(label)
					}
					if settingsLink(c, p, "取消").Clicked() {
						a.confirmClear = ""
					}
				})
			})
		})
	}
}

// clearData 执行设置页里的清理操作。
func (a *nativeApp) clearData(label string) {
	a.confirmClear = ""
	if label == "清理恢复草稿" {
		a.clearRecovery()
		a.settingsStatus = label + "完成"
		return
	}
	if a.workspace == nil {
		return
	}
	go func() {
		err := a.workspace.ClearRecent()
		a.update(func() {
			if err != nil {
				a.settingsStatus = "清理未完成，请稍后重试。"
				return
			}
			a.settingsStatus = label + "完成"
			a.reloadNavigation()
		})
	}()
}

// viewReleaseNotes 把 Markdown 写成的更新日志排成小标题、段落与列表，不露出标记符号。
func viewReleaseNotes(c *ui.Context, markdown string) {
	plain := func(runs []richtext.Run) string {
		var b strings.Builder
		for _, r := range runs {
			b.WriteString(r.Text)
		}
		return b.String()
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		for i, b := range richtext.Parse(markdown).Blocks() {
			text := plain(b.Runs)
			switch b.Kind {
			case richtext.Heading:
				top := float32(10)
				if i == 0 {
					top = 0
				}
				ui.Text(c, text).FontSize(14).FontWeight(600).LineHeight(1.5).Margin(top, 0, 0, 0)
			case richtext.List, richtext.Task:
				marker := "•"
				if b.Ordered {
					marker = strconv.Itoa(max(1, b.Start)) + "."
				}
				ui.Row(c).Key("note:"+strconv.Itoa(i)).FillWidth().Gap(8).Padding(0, 0, 0, float32(6+max(b.Level-1, 0)*16)).Children(func() {
					ui.Text(c, marker).FontSize(13).LineHeight(1.8).Shrink(0)
					ui.Text(c, text).FontSize(13).LineHeight(1.8).Grow(1).MinWidth(0)
				})
			case richtext.Code:
				ui.Text(c, strings.TrimRight(b.Code, "\n")).Font(sourceFontFamily).FontSize(12).LineHeight(1.7)
			case richtext.Horizontal, richtext.Image:
			case richtext.Raw:
				ui.Text(c, strings.TrimSpace(b.Raw)).FontSize(13).LineHeight(1.8)
			default:
				if text != "" {
					ui.Text(c, text).FontSize(13).LineHeight(1.8)
				}
			}
		}
	})
}

// updateProgress 返回更新窗口的提示文字与进度（0–1）。shown 为 false 时不画进度条，
// fraction 为负表示还不知道总大小。
func (a *nativeApp) updateProgress(installed bool) (hint string, fraction float64, shown bool) {
	switch {
	case installed:
		return "更新已安装，重启应用即可使用新版本。", 1, true
	case a.updateError != "":
		return a.updateError, 0, false
	case !a.updateDownloading:
		return "安装完成后可重启应用以使用新版本，重启前会检查未保存的文档。", 0, false
	case a.updateTotal <= 0 && a.updateDownloaded <= 0:
		return "正在连接下载，请稍候…", -1, true
	case a.updateTotal <= 0:
		return "正在下载更新，请稍候…", -1, true
	case a.updateDownloaded >= a.updateTotal:
		// 下载结束不代表安装成功：保留满格进度，等待签名校验与替换完成。
		return "下载完成，正在校验并安装更新…", 1, true
	}
	fraction = float64(a.updateDownloaded) / float64(a.updateTotal)
	return fmt.Sprintf("正在下载更新… %d%%", int(fraction*100)), fraction, true
}

// viewUpdateDialog 展示新版本的更新日志；确认后下载安装并显示进度，装好后可直接重启。
// 它挂在主视图上，书写界面和设置页里都能弹出。
func (a *nativeApp) viewUpdateDialog(c *ui.Context) {
	if a.updates == nil || !a.updateOpen {
		return
	}
	p := settingsColors(c.Theme().Dark)
	// 取消按钮、Escape 和点击空白统一取消下载并退出弹窗。
	open := true
	ui.Modal(c, &open, func() {
		status := a.updates.Status()
		version := status.Version
		if version == "" {
			version = "开发版"
		}
		notes := status.Notes
		if notes == "" {
			notes = "此版本未提供更新日志。"
		}
		ui.Text(c, "发现新版本 "+status.Available).Bold().FontSize(18)
		ui.Text(c, "当前版本："+version).FontSize(12).TextColor(p.muted)
		ui.Text(c, "更新日志").FontSize(13).FontWeight(600)
		ui.Scroll(c).Width(500).MinHeight(0).MaxHeight(320).Padding(16).Radius(8).Border(1, p.line).Background(p.control).Children(func() {
			viewReleaseNotes(c, notes)
		})
		hint, fraction, shown := a.updateProgress(status.Installed)
		ui.Text(c, hint).FontSize(12).LineHeight(1.6).TextColor(p.muted).Width(500)
		if shown {
			accent := c.Theme().Accent
			now := c.Now()
			if fraction < 0 {
				c.After(40 * time.Millisecond) // 总大小未知时来回滑动
			}
			ui.Box(c).Width(500).Height(6).Shrink(0).Label("更新下载进度").Draw(func(painter *ui.Painter, r ui.Rect) {
				painter.Fill(r, p.line, 3)
				bar := ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(fraction), H: r.H}
				if fraction < 0 {
					phase := float32(now.UnixMilli()%1400) / 1400
					bar.W = r.W * .3
					bar.X = r.X + (r.W-bar.W)*phase
				}
				if bar.W > 0 {
					painter.Fill(bar, accent, 3)
				}
			})
		}
		ui.Row(c).Width(500).Shrink(0).Gap(8).Justify(ui.End).Children(func() {
			later := "取消"
			if status.Installed {
				later = "稍后"
			}
			if ui.Button(c, later).Clicked() {
				a.dismissUpdate()
			}
			if status.Installed {
				if ui.PrimaryButton(c, "重启应用").Clicked() {
					if err := a.updates.Restart(); err != nil {
						a.updateError = err.Error()
					}
				}
			} else if ui.PrimaryButton(c, "确定升级").Disabled(a.updateDownloading).Clicked() {
				a.installUpdate()
			}
		})
	})
	if !open {
		a.dismissUpdate()
	}
}

// viewShortcutDialog 是快捷键设置对话框：选择动作、录制组合键、校验冲突后应用。
func (a *nativeApp) viewShortcutDialog(c *ui.Context, p settingsPalette) {
	ui.Modal(c, &a.shortcutOpen, func() {
		mod := "Ctrl"
		if runtime.GOOS == "darwin" {
			mod = "⌘"
		}
		ui.Text(c, "快捷键设置").Font(`"Newsreader", "Songti SC", "STSong", serif`).FontSize(26)
		ui.Text(c, fmt.Sprintf("选择动作，然后录制组合键。Mod 在本机对应 %s。", mod)).FontSize(12).LineHeight(1.8).TextColor(p.muted)
		ui.Text(c, "动作").FontSize(12).Margin(10, 0, 0, 0)
		labels := make([]string, len(shortcutActions))
		for i, item := range shortcutActions {
			labels[i] = item.label
		}
		settingsSelect(c, p, "动作", shortcutLabel(a.shortcutAction), 424, labels, func(i int) {
			a.shortcutAction, a.shortcutCandidate, a.shortcutStatus, a.shortcutRecording = shortcutActions[i].id, "", "", false
		})
		current := a.settings.Shortcuts.binding(a.shortcutAction)
		if current == nil {
			return
		}
		ui.Text(c, "当前绑定："+*current).Font(sourceFontFamily).FontSize(11).TextColor(p.muted).Margin(6, 0)
		text := "点击录制组合键"
		if a.shortcutCandidate != "" {
			text = a.shortcutCandidate
		} else if a.shortcutRecording {
			text = "请按下组合键…"
		}
		accent := c.Theme().Accent
		recorder := ui.ButtonBase(c).Width(424).MinHeight(62).Radius(10).Background(c.Theme().Surface).Label("录制组合键").Focusable()
		recorder.Draw(func(painter *ui.Painter, r ui.Rect) { painter.StrokeDashed(r, accent, 10, 1) })
		recorder.Children(func() { ui.Text(c, text).Font(sourceFontFamily).FontSize(13).TextColor(accent) })
		if recorder.Clicked() {
			a.shortcutRecording, a.shortcutCandidate, a.shortcutStatus = true, "", ""
		}
		if a.shortcutRecording {
			recorder.Focus()
			recorder.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown || ev.Key == ui.KeyEscape {
					return false
				}
				a.recordShortcut(recordedShortcut(ev.Mods, ev.Key))
				return true
			})
		}
		ui.Text(c, "至少包含一个 Mod、Ctrl、Meta 或 Alt；Escape 返回。").FontSize(12).LineHeight(1.8).TextColor(p.muted)
		ui.Text(c, a.shortcutStatus).FontSize(12).TextColor(accent).MinHeight(24).Width(424)
		ui.Row(c).Width(424).Gap(8).Justify(ui.End).Padding(18, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
			if settingsOutlined(c, p, "恢复默认").Clicked() {
				a.changeSettings(func(s *nativeSettings) { s.Shortcuts = defaultNativeShortcuts() })
				a.shortcutCandidate, a.shortcutRecording, a.shortcutStatus = "", false, "绑定已生效并保存。"
			}
			apply := ui.ButtonBase(c).Radius(10).Padding(7, 10).Background(accent).Label("应用绑定").Disabled(a.shortcutCandidate == "")
			if a.shortcutCandidate == "" {
				apply.Opacity(.45)
			}
			apply.Children(func() { ui.Text(c, "应用绑定").FontSize(11).TextColor(c.Theme().Background) })
			if apply.Clicked() && a.shortcutCandidate != "" {
				action, candidate := a.shortcutAction, a.shortcutCandidate
				a.changeSettings(func(s *nativeSettings) { *s.Shortcuts.binding(action) = candidate })
				a.shortcutCandidate, a.shortcutRecording, a.shortcutStatus = "", false, "绑定已生效并保存。"
			}
			if settingsOutlined(c, p, "返回").Clicked() {
				a.shortcutOpen = false
			}
		})
	})
}

// recordShortcut 校验一次录制到的组合键，可用时成为待应用的候选。
func (a *nativeApp) recordShortcut(raw string) {
	if raw == "" {
		return // 只按了修饰键，继续等待
	}
	a.shortcutCandidate = ""
	shortcut, problem := validateShortcut(raw)
	if problem != "" {
		a.shortcutStatus = problem
		return
	}
	if other := shortcutConflict(a.settings.Shortcuts, a.shortcutAction, shortcut); other != "" {
		a.shortcutStatus = "与“" + shortcutLabel(other) + "”冲突，请录制其他组合键。"
		return
	}
	a.shortcutCandidate, a.shortcutRecording, a.shortcutStatus = shortcut, false, "组合键可用，点击应用绑定。"
}
