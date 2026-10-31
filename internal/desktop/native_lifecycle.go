package desktop

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/documents"
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
		if !prompting.CompareAndSwap(false, true) {
			e.PreventDefault()
			return
		}
		// OnClose 已在主线程。先同步正文，再把对话框放到后台，避免挡住事件循环。
		dirty := app.dirtyDocuments()
		for _, saving := range app.saving {
			if saving {
				e.PreventDefault()
				prompting.Store(false)
				restarting.Store(false)
				app.notice = "正在保存，请稍后关闭"
				return
			}
		}
		if len(dirty) == 0 {
			prompting.Store(false)
			if restarting.Load() {
				e.PreventDefault()
				finish()
				return
			}
			// 没有需要确认的文档时放行本次关闭。退出应用时 MyGo 逐个关闭窗口，
			// 拦下后再自行销毁会被当成取消退出，窗口消失而进程留在 Dock 里。
			app.markClosed()
			app.clearRecovery()
			return
		}
		e.PreventDefault()
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
				// 侧栏四周各留 8 的外边距，用内边距实现以免高度溢出窗口；右侧外边距由拖动手柄占据。
				ui.Box(c).Width(sidebarWidth(a.settings.SidebarWidth, width)-sidebarHandleWidth).Shrink(0).FillHeight().Padding(8, 8-sidebarHandleWidth, 8, 8).Children(func() { a.viewSidebar(c) })
				a.viewSidebarHandle(c, width)
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

const (
	nativeLightPaper = "#faf9f5"
	nativeDarkPaper  = "#242220"
)

// nativeTheme 统一书写、设置、源码和导出的语义色，也覆盖控件库的交互状态。
func nativeTheme(dark bool) *ui.Theme {
	base := ui.LightTheme()
	if dark {
		base = ui.DarkTheme()
	}
	// 叶笺的纸色，不沿用控件库的纯白默认。
	if !dark {
		base.Background = ui.Hex(nativeLightPaper)
		base.Surface = ui.Hex("#f1eee6")
		base.Text = ui.Hex("#47423c")
		base.TextMuted = ui.Hex("#797168")
		base.Border = ui.Hex("#e8e1d7")
		base.Accent = ui.Hex("#bd7858")
	} else {
		// 深色与浅色同一暖色相，只降低明度，不混入蓝灰。
		base.Background = ui.Hex(nativeDarkPaper)
		base.Surface = ui.Hex("#2e2b28")
		base.Text = ui.Hex("#d6d1ca")
		base.TextMuted = ui.Hex("#a29a90")
		base.Border = ui.Hex("#3b3834")
		base.Accent = ui.Hex("#d49a7b")
	}
	base.SurfaceHover = base.Surface.Mix(base.Text, .06)
	base.SurfacePressed = base.Surface.Mix(base.Text, .12)
	base.AccentHover = base.Accent.Mix(ui.RGB(255, 255, 255), .08)
	base.AccentPressed = base.Accent.Mix(ui.RGB(255, 255, 255), .16)
	// 暖色按钮上使用深墨色，避免深色主题中白字与浅强调色对比不足。
	base.AccentText = ui.Hex(nativeDarkPaper)
	base.Selection = base.Accent.Alpha(.25)
	base.Focus = base.Accent.Alpha(.55)
	return base
}

func (a *nativeApp) applyTheme(c *ui.Context) {
	dark := c.Theme().Dark
	switch a.settings.Theme {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	base := nativeTheme(dark)
	// 滚动条用细而淡的墨色，悬停在滚动区上才出现。
	base.Scrollbar = base.TextMuted.Alpha(0.3)
	base.ScrollbarWidth = 4
	// 界面文字随字号设置等比缩放：默认字号下取基准字号。
	base.Font = "Inter"
	base.FontSize = uiThemeFontSize * a.settings.FontSize / uiBaseFontSize
	c.SetTheme(base)
	a.effectiveDark = dark
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
		ui.Text(c, title).Bold().FontSize(scaled(c, 18))
		if hint != "" {
			ui.Text(c, hint).FontSize(scaled(c, 12)).TextColor(c.Theme().TextMuted)
		}
		ui.TextInput(c, &a.promptText).FillWidth().MinWidth(320).AutoFocus()
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "确定").Cursor(ui.CursorPointer).Clicked() || c.Shortcut(0, ui.KeyEnter) {
				if a.promptOK != nil {
					a.promptOK(a.promptText)
				}
				a.promptOpen = false
				a.syncEditor(a.active())
			}
			if ui.Button(c, "取消").Border(0, ui.Transparent).Cursor(ui.CursorPointer).Clicked() {
				a.promptOpen = false
			}
		})
	})
}

func (a *nativeApp) closeActive() {
	tab := a.active()
	if tab == nil || a.closePrompt || a.saving[tab.id] {
		return
	}
	a.syncEditor(tab)
	doc, ok := a.document(tab.id)
	// 同步后再看存储里的未保存状态，不再次调用 Changed。
	if ok && doc.Dirty {
		a.confirmClose([]documents.Document{doc}, func() { a.finishClose([]string{doc.ID}) })
		return
	}
	a.finishClose([]string{tab.id})
}

func (a *nativeApp) finishClose(ids []string) {
	for _, id := range ids {
		_ = a.files.Close(id)
		a.dropTab(id)
		delete(a.saveGen, id)
		delete(a.saving, id)
		delete(a.editedAt, id)
		delete(a.seenText, id)
	}
	if len(a.tabs) == 0 {
		a.adopt(a.files.New(), "")
	}
	if tab := a.active(); tab != nil {
		_, _ = a.files.store.Select(tab.id)
	}
	a.persistRecovery()
	a.refreshTitle()
}

func (a *nativeApp) confirmClose(dirty []documents.Document, discard func()) {
	a.confirmCloseNotify(dirty, discard, nil)
}

// 一个确认流程持有门闩直到保存或取消结束；不保存只授权丢弃提示时的正文。
func (a *nativeApp) confirmCloseNotify(dirty []documents.Document, done, failed func()) {
	if a.closePrompt {
		if failed != nil {
			failed()
		}
		return
	}
	a.closePrompt = true
	release := func() {
		a.closePrompt = false
		if failed != nil {
			failed()
		}
	}
	finish := func() { a.closePrompt = false; done() }
	win := a.window()
	names := make([]string, 0, len(dirty))
	for _, doc := range dirty {
		names = append(names, doc.Name)
	}
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: win, Type: mygo.MessageQuestion, Message: "要保存文档后再关闭吗？", Detail: "未保存：" + strings.Join(names, "、"), Buttons: []string{"保存", "不保存", "取消"}, DefaultButton: 0, CancelButton: 2})
		a.update(func() {
			if err != nil || res.Button == 2 {
				release()
				return
			}
			if res.Button == 1 {
				a.syncAll()
				if !a.closeSnapshotUnchanged(dirty) {
					release()
					a.notice = "确认期间文档发生变化，请再次关闭以确认新修改"
					return
				}
				finish()
				return
			}
			a.saveThenNotify(dirty, finish, release)
		})
	}()
}

func (a *nativeApp) closeSnapshotUnchanged(dirty []documents.Document) bool {
	for _, before := range dirty {
		if a.saving[before.ID] {
			return false
		}
		if after, ok := a.document(before.ID); ok && after.Content != before.Content {
			return false
		}
	}
	return true
}

// saveThenNotify 逐个保存。未保存文档会弹出保存框；取消或失败时调用 failed，不再继续关闭。
func (a *nativeApp) saveThenNotify(dirty []documents.Document, done func(), failed func()) {
	ids := make([]string, 0, len(dirty))
	for _, doc := range dirty {
		ids = append(ids, doc.ID)
	}
	a.saveThenScoped(dirty, ids, done, failed)
}

func (a *nativeApp) saveThenScoped(dirty []documents.Document, ids []string, done func(), failed func()) {
	if len(dirty) == 0 {
		a.syncAll()
		// 保存完成后仍有新输入时中止关闭，保留输入和恢复记录。
		changed := false
		for _, id := range ids {
			if doc, ok := a.document(id); ok && (doc.Dirty || a.saving[id]) {
				changed = true
			}
		}
		if changed {
			a.notice = "保存期间文档发生变化，请再次关闭以保存新修改"
			if failed != nil {
				failed()
			}
			return
		}
		done()
		return
	}
	doc := dirty[0]
	content := doc.Content
	if tab := a.tabByID(doc.ID); tab != nil && tab.editor != nil {
		a.syncEditor(tab)
		content = tab.editor.Markdown()
	}
	a.saveDocument(doc.ID, content, false, func(ok bool) {
		if !ok {
			if failed != nil {
				failed()
			}
			return
		}
		a.saveThenScoped(dirty[1:], ids, done, failed)
	})
}

// dirtyDocuments 供关闭前读取。调用方须已在主线程同步过编辑器。
func (a *nativeApp) dirtyDocuments() []documents.Document {
	a.syncAll()
	dirty := []documents.Document{}
	for _, doc := range a.files.store.List() {
		if doc.Dirty {
			dirty = append(dirty, doc)
		}
	}
	return dirty
}

// markClosed 拒绝随后的界面回写。关闭确认仍可用已经取出的窗口句柄。
func (a *nativeApp) markClosed() { a.closed.Store(true) }
