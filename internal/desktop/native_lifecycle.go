package desktop

import (
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// installNativeCloseHandler 在关闭前于主线程同步编辑器，再询问未保存文档。
// 不调用 Page 或 Eval。返回的函数供更新安装后重启：先走同一套关闭检查。
func installNativeCloseHandler(win *mygo.Window, app *nativeApp) func() {
	var prompting, restarting, approved atomic.Bool
	finish := func() {
		app.markClosed()
		app.clearRecovery()
		if restarting.Swap(false) {
			approved.Store(true)
			mygo.App.Relaunch()
			return
		}
		win.Destroy()
	}
	win.OnClose(func(e *mygo.CloseEvent) {
		if approved.Swap(false) {
			return
		}
		e.PreventDefault()
		if !prompting.CompareAndSwap(false, true) {
			return
		}
		// OnClose 已在主线程。先同步正文，再把对话框放到后台，避免挡住事件循环。
		dirty := app.dirtyDocuments()
		for _, saving := range app.saving {
			if saving {
				prompting.Store(false)
				restarting.Store(false)
				app.notice = "正在保存，请稍后关闭"
				return
			}
		}
		if len(dirty) == 0 {
			prompting.Store(false)
			finish()
			return
		}
		app.confirmCloseNotify(dirty, func() {
			// 原提示未包含的标签可能在对话框或保存期间新增输入，必须再次确认。
			known := map[string]bool{}
			for _, doc := range dirty {
				known[doc.ID] = true
			}
			for _, doc := range app.dirtyDocuments() {
				if !known[doc.ID] {
					prompting.Store(false)
					restarting.Store(false)
					app.notice = "关闭期间新增未保存文档，请再次关闭以确认"
					return
				}
			}
			prompting.Store(false)
			finish()
		}, func() { prompting.Store(false); restarting.Store(false) })
	})
	return func() {
		if prompting.Load() || !restarting.CompareAndSwap(false, true) {
			return
		}
		win.Close()
	}
}

// View 绘制原生主界面。每次调用都在主线程，编辑器始终排版，不经网页。
func (a *nativeApp) View(c *ui.Context) {
	a.applyTheme(c)
	a.scheduleAutoSave(c.Now())
	// 编辑器输入在本帧晚些时候处理，定时帧负责发现输入并驱动两秒后的保存。
	c.After(250 * time.Millisecond)
	if a.recoveryAt.IsZero() || c.Now().Sub(a.recoveryAt) >= 2*time.Second {
		a.recoveryAt = c.Now()
		a.persistRecovery()
	}
	a.watchExternal(c.Now())
	a.handleShortcuts(c)
	bar := c.TitleBar()
	if a.settingsOn {
		a.viewSettings(c)
	} else {
		width, _ := c.Size()
		ui.Row(c).Fill().MinWidth(0).Children(func() {
			if a.sidebar >= 80 && width >= 600 {
				// 侧栏四周各留 8 的外边距，用内边距实现以免高度溢出窗口。
				ui.Box(c).Width(264).Shrink(0).FillHeight().Padding(8).Children(func() { a.viewSidebar(c) })
			}
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
				a.viewTitleBar(c, bar)
				ui.Box(c).Grow(1).FillWidth().MinHeight(0).Children(func() { a.viewEditor(c) })
			})
		})
	}
	if a.notice != "" {
		c.Toast(a.notice)
		a.notice = ""
	}
	a.viewPrompt(c)
	a.viewRawEditor(c)
	a.viewUpdateDialog(c)
	// 验证只在真实帧构建完成后插入操作，不走网页。
	if a.verifyStep != nil {
		a.verifyStep(c)
	}
}

func (a *nativeApp) applyTheme(c *ui.Context) {
	dark := c.Theme().Dark
	switch a.settings.Theme {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	base := ui.LightTheme()
	if dark {
		base = ui.DarkTheme()
	}
	// 叶笺的纸色，不沿用控件库的纯白默认。
	if !dark {
		base.Background = ui.Hex("#faf9f5")
		base.Surface = ui.Hex("#f1eee6")
		base.Text = ui.Hex("#47423c")
		base.TextMuted = ui.Hex("#797168")
		base.Border = ui.Hex("#e8e1d7")
		base.Accent = ui.Hex("#bd7858")
	} else {
		base.Background = ui.Hex("#242527")
		base.Surface = ui.Hex("#2d2f33")
		base.Text = ui.Hex("#cbcdd3")
		base.TextMuted = ui.Hex("#a9adb5")
		base.Border = ui.Hex("#3d4046")
		base.Accent = ui.Hex("#a6b7c8")
	}
	// 正文字号由编辑器设置，工具栏和文件导航保持紧凑。
	base.Font = "Inter"
	base.FontSize = 12
	c.SetTheme(base)
}

func (a *nativeApp) handleShortcuts(c *ui.Context) {
	// 设置页里只响应打开/关闭设置，避免误触发新建、保存等文档操作。
	if a.settingsOn {
		if mods, key, ok := parseShortcut(a.settings.Shortcuts.Settings); ok && !a.shortcutOpen && c.Shortcut(mods, key) {
			a.command("settings")
		}
		return
	}
	source := sourceMode(a.active())
	for _, item := range shortcutActions {
		// 源码模式的文字快捷键交给文本域自己处理。
		if source && (item.id == "bold" || item.id == "italic" || item.id == "link") {
			continue
		}
		if mods, key, ok := parseShortcut(*a.settings.Shortcuts.binding(item.id)); ok && c.Shortcut(mods, key) {
			a.command(item.command)
			return
		}
	}
	if source {
		return // 撤销与重做同样留给文本域
	}
	switch {
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ):
		a.command("redo")
	case c.Shortcut(ui.Cmd, ui.KeyZ):
		a.command("undo")
	}
}

func (a *nativeApp) viewPrompt(c *ui.Context) {
	ui.Modal(c, &a.promptOpen, func() {
		title, hint := a.promptTitle, a.promptHint
		if a.promptKind == "link" {
			title, hint = "插入链接", "文字与地址用空格分开；只填地址时两者相同。"
		}
		if title == "" {
			title = "插入"
		}
		ui.Text(c, title).Bold().FontSize(18)
		if hint != "" {
			ui.Text(c, hint).FontSize(12).TextColor(c.Theme().TextMuted)
		}
		ui.TextInput(c, &a.promptText).FillWidth().MinWidth(320).AutoFocus()
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "确定").Clicked() || c.Shortcut(0, ui.KeyEnter) {
				if a.promptOK != nil {
					a.promptOK(a.promptText)
				}
				a.promptOpen = false
				a.syncEditor(a.active())
			}
			if ui.Button(c, "取消").Clicked() {
				a.promptOpen = false
			}
		})
	})
}
