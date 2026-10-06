package desktop

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/documents"
	"leafmark/internal/workspace"
)

// documentEditor 是原生编辑器对桌面装配暴露的能力。真实类型由其他包实现。
type documentEditor interface {
	View(c *ui.Context)
	Markdown() string
	Changed() bool
	Undo()
	Redo()
	Format(name string)
	InsertLink(label, url string)
	InsertImage(alt, path string)
	Find(query string) bool
	FontSize(size float32)
	// SetReadImage 的参数是 Markdown 里的图片路径，不是调用方已经准备好的 data URI。
	// 桌面闭包用当前文档授权读取；返回 nil 时编辑器保留替代文字。
	SetReadImage(fn func(markdownPath string) *ui.Bitmap)
	Text() string
	Selection() (start, end int)
	SetSelection(start, end int)
	HandleInput(c *ui.Context, ev ui.InputEvent) bool
}

// concreteEditor 是标签上的具体编辑器。验证可用类型断言取得 *nativeeditor.Editor。
func (t *nativeTab) concreteEditor() any {
	if t == nil {
		return nil
	}
	return t.editor
}

// newDocumentEditor 创建始终排版的原生编辑器。正式文件接 nativeeditor，测试文件换成桩。

const autoSaveDelay = 2 * time.Second

// nativeTab 一个标签持有自己的编辑器，切换时保留撤销栈。
type nativeTab struct {
	id     string
	editor documentEditor
}

// nativeSettings 是原生窗口的偏好设置，保存在本机配置目录。
type nativeSettings struct {
	Font         string          `json:"font"` // newsreader、serif、sans、mono
	FontSize     float32         `json:"fontSize"`
	LineHeight   float32         `json:"lineHeight"`
	ReadingWidth string          `json:"readingWidth"` // narrow、comfort、wide
	Theme        string          `json:"theme"`
	AutoSave     bool            `json:"autoSave"`
	FocusMode    bool            `json:"focusMode"`
	Typewriter   bool            `json:"typewriter"`
	Shortcuts    nativeShortcuts `json:"shortcuts"`
}

func defaultNativeSettings() nativeSettings {
	return nativeSettings{Font: "newsreader", FontSize: 15, LineHeight: 1.4, ReadingWidth: "comfort", Theme: "system", AutoSave: true, Shortcuts: defaultNativeShortcuts()}
}

// editorFonts 是四种正文字体的回退列表；空串表示编辑器内置的默认列表。
var editorFonts = map[string]string{
	"newsreader": "",
	"serif":      `"Songti SC", "STSong", "Noto Serif CJK SC", "SimSun", Georgia, serif`,
	"sans":       `system-ui, -apple-system, "Segoe UI", "PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", sans-serif`,
	"mono":       `"Geist Mono", "SFMono-Regular", "Consolas", monospace`,
}

var readingWidths = map[string]float32{"narrow": 600, "comfort": 856, "wide": 1040}

// normalized 逐字段校验设置，不合法的值回落默认。
func (s nativeSettings) normalized() nativeSettings {
	out := defaultNativeSettings()
	if _, ok := editorFonts[s.Font]; ok {
		out.Font = s.Font
	}
	if s.FontSize >= 12 && s.FontSize <= 28 {
		out.FontSize = float32(int(s.FontSize + .5))
	}
	if s.LineHeight >= 1.3 && s.LineHeight <= 2.2 {
		out.LineHeight = s.LineHeight
	}
	if _, ok := readingWidths[s.ReadingWidth]; ok {
		out.ReadingWidth = s.ReadingWidth
	}
	switch s.Theme {
	case "light", "dark", "system":
		out.Theme = s.Theme
	}
	out.AutoSave, out.FocusMode, out.Typewriter = s.AutoSave, s.FocusMode, s.Typewriter
	out.Shortcuts = checkedShortcuts(s.Shortcuts)
	return out
}

// nativeApp 是主窗口的全部界面状态。View 只在主线程读取；后台 IO 经 Window.Update 写回。
type nativeApp struct {
	files     *Files
	workspace *Workspace
	assets    *Assets
	updates   *Updates

	mu  sync.Mutex
	win *mygo.Window

	tabs    []*nativeTab
	current int
	sidebar float32

	// 空值与 outline 都显示原版默认的大纲页。
	sidebarMode     string
	navQuery        string
	settingsSection string
	outlineMarkdown string
	outlineSource   bool
	statsMarkdown   string // 状态栏字数对应的原文
	statsWords      int
	outlineHeadings []nativeHeading

	settings    nativeSettings
	settingsOn  bool
	findOn      bool
	findQuery   string
	findNote    string
	replaceText string
	reading     bool

	promptOpen bool
	promptText string
	promptKind string
	// promptTitle 与 promptHint 供链接以外的输入框使用。
	promptTitle string
	promptHint  string
	promptOK    func(string)

	notice string

	tree       []workspace.Node
	recent     []workspace.RecentDocument
	folderName string
	openDirs   map[string]bool

	// 每次开始一次保存都递增，返回时序号不符说明用户已继续输入，不能用旧正文覆盖。
	saveGen map[string]uint64
	saving  map[string]bool
	// 外部内容冲突后暂停该标签的自动保存，直到重新加载或另存为。
	conflicts map[string]bool
	// 编辑器未提供改动时间时，由桌面侧记下正文变化的时刻，供 2 秒自动保存使用。
	editedAt map[string]time.Time
	seenText map[string]string

	closed    atomic.Bool
	booted    atomic.Bool
	treeGen   atomic.Uint64
	pendingMu sync.Mutex
	pending   []string

	// verifyStep 由原生验证在真实帧末尾驱动界面，正式运行保持 nil。
	verifyStep      func(*ui.Context)
	closePrompt     bool
	unrestored      []nativeDraft
	recoveryBlocked bool
	updateBusy      bool
	recoveryAt      time.Time
	// externalAt 与 externalBusy 控制对当前文档磁盘变化的轮询。
	externalAt   time.Time
	externalBusy bool
	exporting    bool

	// 设置页的临时状态。
	updateNote        string
	updateOpen        bool
	updateDownloading bool // 正在下载或安装，弹窗显示进度条
	updateCancel      context.CancelFunc
	updateDownloaded  int64
	updateTotal       int64
	updateError       string
	confirmClear      string
	settingsStatus    string
	shortcutOpen      bool
	shortcutAction    string
	shortcutCandidate string
	shortcutStatus    string
	shortcutRecording bool

	// 公式、图表等原文块的源码编辑框。
	rawOpen bool
	rawText string
	rawOK   func(string)
}

func newNativeApp(files *Files, workspace *Workspace, assets *Assets, updates *Updates) *nativeApp {
	return &nativeApp{
		files: files, workspace: workspace, assets: assets, updates: updates,
		sidebar: 248, settings: defaultNativeSettings(),
		openDirs: map[string]bool{}, saveGen: map[string]uint64{}, saving: map[string]bool{}, conflicts: map[string]bool{},
		editedAt: map[string]time.Time{}, seenText: map[string]string{},
	}
}

func (a *nativeApp) attach(win *mygo.Window) {
	a.mu.Lock()
	a.win = win
	a.mu.Unlock()
}

func (a *nativeApp) window() *mygo.Window {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.win
}

// update 把后台结果送回主线程。窗口已关闭时丢弃，避免关闭后改界面。
func (a *nativeApp) update(fn func()) {
	if a.closed.Load() {
		return
	}
	win := a.window()
	if win == nil || win.IsDestroyed() {
		return
	}
	win.Update(func() {
		if a.closed.Load() {
			return
		}
		fn()
	})
}

func (a *nativeApp) active() *nativeTab {
	if a.current < 0 || a.current >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.current]
}

func (a *nativeApp) tabByID(id string) *nativeTab {
	for _, tab := range a.tabs {
		if tab.id == id {
			return tab
		}
	}
	return nil
}

func (a *nativeApp) document(id string) (documents.Document, bool) {
	for _, doc := range a.files.store.List() {
		if doc.ID == id {
			return doc, true
		}
	}
	return documents.Document{}, false
}

func (a *nativeApp) syncEditor(tab *nativeTab) {
	if tab == nil || tab.editor == nil {
		return
	}
	_ = tab.editor.Changed()
	if err := a.files.store.Draft(tab.id, tab.editor.Markdown()); err != nil && !errors.Is(err, documents.ErrStale) {
		log.Printf("同步草稿失败：%v", err)
	}
}

func (a *nativeApp) syncAll() {
	for _, tab := range a.tabs {
		a.syncEditor(tab)
	}
}

func (a *nativeApp) adopt(doc documents.Document, markdown string) {
	if markdown == "" {
		markdown = doc.Content
	}
	// 同一标签再次打开时保留编辑器和撤销历史，只切过去。
	for i, old := range a.tabs {
		if old.id == doc.ID {
			a.syncEditor(a.active())
			a.current = i
			_, _ = a.files.store.Select(doc.ID)
			a.refreshTitle()
			return
		}
	}
	editor := newDocumentEditor(markdown)
	a.configureEditor(editor)
	a.connectEditor(editor)
	if a.assets != nil {
		id := doc.ID
		assets := a.assets
		editor.SetReadImage(imageReader(assets, id))
	}
	a.syncEditor(a.active())
	a.seenText[doc.ID] = markdown
	a.tabs = append(a.tabs, &nativeTab{id: doc.ID, editor: editor})
	a.current = len(a.tabs) - 1
	_, _ = a.files.store.Select(doc.ID)
	a.refreshTitle()
}

// replaceEditor 只用于外部重新加载，刻意丢掉旧撤销历史。
func (a *nativeApp) replaceEditor(id, markdown string) {
	tab := a.tabByID(id)
	if tab == nil {
		return
	}
	fresh := newDocumentEditor(markdown)
	a.configureEditor(fresh)
	a.connectEditor(fresh)
	if a.assets != nil {
		fresh.SetReadImage(imageReader(a.assets, id))
	}
	tab.editor = fresh
	delete(a.seenText, id)
	delete(a.editedAt, id)
	a.refreshTitle()
}

func (a *nativeApp) connectEditor(editor documentEditor) {
	if links, ok := editor.(interface{ SetOpenLink(func(string)) }); ok {
		links.SetOpenLink(func(url string) {
			go func() {
				if err := mygo.Shell.OpenExternal(url); err != nil {
					a.update(func() { a.notice = err.Error() })
				}
			}()
		})
	}
	if raw, ok := editor.(rawEditor); ok {
		raw.SetEditRaw(func(index int, source string) { a.editRaw(raw, index, source) })
	}
	if live, ok := editor.(interface{ SetInvalidate(func()) }); ok {
		live.SetInvalidate(func() {
			if win := a.window(); win != nil && !a.closed.Load() {
				win.Invalidate()
			}
		})
	}
}

func (a *nativeApp) refreshTitle() {
	win := a.window()
	if win == nil {
		return
	}
	title := "Leafmark · 叶笺"
	if tab := a.active(); tab != nil {
		if doc, ok := a.document(tab.id); ok {
			mark := ""
			// 不用 Changed：它会清掉尚未同步的改动标记。
			if doc.Dirty || a.pendingEdit(tab) {
				mark = " •"
			}
			title = doc.Name + mark + " — Leafmark"
		}
	}
	win.SetTitle(title)
}

// pendingEdit 用已见正文判断编辑器里还有没有未同步的修改。
func (a *nativeApp) pendingEdit(tab *nativeTab) bool {
	if tab == nil || tab.editor == nil {
		return false
	}
	seen, ok := a.seenText[tab.id]
	return ok && seen != tab.editor.Markdown()
}

func (a *nativeApp) boot() {
	if !a.booted.CompareAndSwap(false, true) {
		return
	}
	go func() {
		settings := loadNativeSettings()
		drafts, recoveryOK := readNativeRecoveryState()
		a.update(func() {
			a.recoveryBlocked = !recoveryOK
			a.settings = settings
			a.applySettings()
			// 启动时已打开的文档全部做成标签，不只保留当前一份。
			for _, doc := range a.files.store.List() {
				if a.tabByID(doc.ID) == nil {
					a.adopt(doc, doc.Content)
				}
			}
			a.restoreDrafts(drafts)
			if len(a.tabs) == 0 {
				a.adopt(a.files.Current(), "")
			}
			a.openQueued()
			a.reloadNavigation()
		})
	}()
}

func (a *nativeApp) applySettings() {
	for _, tab := range a.tabs {
		if tab.editor != nil {
			a.configureEditor(tab.editor)
		}
	}
}

// configureEditor 把当前设置应用到一个编辑器。排版相关的方法只有真实编辑器实现。
func (a *nativeApp) configureEditor(editor documentEditor) {
	editor.FontSize(a.settings.FontSize)
	if e, ok := editor.(interface{ SetReadOnly(bool) }); ok {
		e.SetReadOnly(a.reading)
	}
	if e, ok := editor.(interface{ SetFontFamily(string) }); ok {
		e.SetFontFamily(editorFonts[a.settings.Font])
	}
	if e, ok := editor.(interface{ SetLineHeight(float32) }); ok {
		e.SetLineHeight(a.settings.LineHeight)
	}
	if e, ok := editor.(interface{ SetReadingWidth(float32) }); ok {
		e.SetReadingWidth(readingWidths[a.settings.ReadingWidth])
	}
	if e, ok := editor.(interface{ SetFocusMode(bool) }); ok {
		e.SetFocusMode(a.settings.FocusMode)
	}
	if e, ok := editor.(interface{ SetTypewriter(bool) }); ok {
		e.SetTypewriter(a.settings.Typewriter)
	}
}

// changeSettings 修改设置后立即生效并保存。
func (a *nativeApp) changeSettings(change func(s *nativeSettings)) {
	focus := a.settings.FocusMode
	change(&a.settings)
	a.settings = a.settings.normalized()
	if a.settings.FocusMode != focus {
		// 专注模式收起侧栏，关闭后恢复。
		a.sidebar = 248
		if a.settings.FocusMode {
			a.sidebar = 0
		}
	}
	a.applySettings()
	a.persistSettings()
}

func (a *nativeApp) restoreDrafts(drafts []nativeDraft) {
	if len(drafts) == 0 {
		return
	}
	current := a.files.Current()
	restored := 0
	for _, draft := range drafts {
		doc, err := a.openRecovered(draft)
		if err != nil {
			a.unrestored = append(a.unrestored, draft)
			log.Printf("恢复草稿失败：%v", err)
			continue
		}
		if err := a.files.store.Draft(doc.ID, draft.Content); err != nil {
			a.unrestored = append(a.unrestored, draft)
			log.Printf("写回恢复草稿失败：%v", err)
			continue
		}
		doc.Content = draft.Content
		doc.Dirty = draft.Content != a.savedBaseline(doc.ID)
		if a.tabByID(doc.ID) != nil {
			a.replaceEditor(doc.ID, draft.Content)
		}
		a.adopt(doc, draft.Content)
		restored++
	}
	if restored == 0 {
		return
	}
	// 欢迎文档没有改动时让位给恢复出来的草稿。
	if current.Path == "" && current.Content == welcome && !current.Dirty {
		if a.tabByID(current.ID) != nil && len(a.tabs) > 1 {
			_ = a.files.Close(current.ID)
			a.dropTab(current.ID)
		}
	}
	a.notice = "已恢复上次未保存的草稿"
}

func (a *nativeApp) savedBaseline(id string) string {
	content, err := a.files.store.SavedContent(id)
	if err != nil {
		return ""
	}
	return content
}

func (a *nativeApp) openRecovered(draft nativeDraft) (documents.Document, error) {
	if draft.Path != "" {
		if _, err := os.Stat(draft.Path); err == nil {
			return a.files.store.Load(draft.Path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return documents.Document{}, err
		}
	}
	name := draft.Name
	if name == "" {
		name = "未命名.md"
	}
	return a.files.store.NewNamed(name)
}

func (a *nativeApp) dropTab(id string) {
	for i, tab := range a.tabs {
		if tab.id != id {
			continue
		}
		a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
		if a.current >= len(a.tabs) {
			a.current = len(a.tabs) - 1
		}
		if a.current < 0 {
			a.current = 0
		}
		return
	}
}

func (a *nativeApp) selectTab(index int) {
	if index < 0 || index >= len(a.tabs) || index == a.current {
		return
	}
	a.syncEditor(a.active())
	a.current = index
	if tab := a.active(); tab != nil {
		_, _ = a.files.store.Select(tab.id)
	}
	a.refreshTitle()
}

func (a *nativeApp) enqueueOpen(paths []string) {
	n := a.files.requestOpen(paths)
	if n == 0 {
		return
	}
	a.pendingMu.Lock()
	a.pending = append(a.pending, paths...)
	a.pendingMu.Unlock()
	if a.booted.Load() {
		a.update(a.openQueued)
	}
}

func (a *nativeApp) openQueued() {
	a.syncEditor(a.active())
	path := a.files.takePending()
	if path == "" {
		a.refreshTitle()
		a.reloadNavigation()
		return
	}
	go func() {
		raw, err := os.ReadFile(path)
		a.update(func() {
			if err != nil {
				a.notice = err.Error()
			} else if doc, loadErr := a.files.store.LoadSnapshot(path, raw); loadErr != nil {
				a.notice = loadErr.Error()
			} else {
				if a.files.workspace != nil {
					if rememberErr := a.files.workspace.rememberAuthorized(doc.Path); rememberErr != nil {
						log.Printf("记录近期文件失败：%v", rememberErr)
					}
				}
				a.adopt(doc, doc.Content)
			}
			a.openQueued()
		})
	}()
}

func (a *nativeApp) reloadNavigation() {
	gen := a.treeGen.Add(1)
	go func() {
		var nodes []workspace.Node
		var recent []workspace.RecentDocument
		folder := ""
		if a.workspace != nil {
			if state, err := a.workspace.Current(); err == nil && state.Workspace != nil {
				folder = state.Workspace.Name
			}
			if tree, err := a.workspace.Tree(); err == nil {
				nodes = tree
			}
			if list, err := a.workspace.Recent(); err == nil {
				recent = list
			}
		}
		a.update(func() {
			if gen != a.treeGen.Load() {
				return
			}
			a.tree = nodes
			a.recent = recent
			a.folderName = folder
		})
	}()
}

// command 只在主线程执行界面动作。
func (a *nativeApp) command(name string) {
	// 阅读模式仍允许保存和导航，所有内容修改入口统一拦截。
	if a.reading && editingCommand(name) {
		a.notice = "阅读模式下不能修改内容"
		return
	}
	switch name {
	case "new":
		a.adopt(a.files.New(), "")
	case "open":
		a.openDialog()
	case "save":
		a.saveActive(false)
	case "save-as":
		a.saveActive(true)
	case "close-tab":
		a.closeActive()
	case "find":
		a.findOn = !a.findOn
	case "settings":
		a.settingsOn = !a.settingsOn
	case "sidebar":
		if a.sidebar < 80 {
			a.sidebar = 248
		} else {
			a.sidebar = 0
		}
	case "undo":
		if tab := a.active(); tab != nil && tab.editor != nil {
			tab.editor.Undo()
		}
	case "redo":
		if tab := a.active(); tab != nil && tab.editor != nil {
			tab.editor.Redo()
		}
	case "bold", "italic", "strike", "code", "heading", "heading1", "heading2", "heading3", "paragraph", "quote", "list", "bullet", "ordered", "task", "table", "codeblock", "hr":
		if tab := a.active(); tab != nil && tab.editor != nil {
			if _, source := tab.editor.(*sourceEditor); source {
				a.notice = "源码模式下请直接输入 Markdown"
			} else {
				tab.editor.Format(name)
			}
		}
	case "reading":
		a.reading = !a.reading
		a.applySettings()
	case "source":
		a.toggleSource()
	case "export-html":
		a.exportHTML()
	case "export-markdown":
		a.exportMarkdown()
	case "export-pdf":
		a.exportPDF()
	case "new-file":
		a.askWorkspacePath("file", "")
	case "new-folder":
		a.askWorkspacePath("folder", "")
	case "link":
		a.askLink()
	case "image":
		a.insertImage()
	case "folder":
		a.chooseFolder()
	case "reload":
		a.reloadActive(false)
	}
	a.syncEditor(a.active())
	a.refreshTitle()
}

func (a *nativeApp) openDialog() {
	win := a.window()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win, Title: "打开 Markdown 文档",
			Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
		})
		if err != nil || len(paths) == 0 {
			if err != nil {
				a.update(func() { a.notice = err.Error() })
			}
			return
		}
		a.enqueueOpen(paths)
	}()
}

func (a *nativeApp) saveActive(saveAs bool) {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	doc, ok := a.document(tab.id)
	if !ok {
		return
	}
	content := tab.editor.Markdown()
	a.saveDocument(doc.ID, content, saveAs, nil)
}

// saveDocument 在后台写盘。generation 用于丢弃过期结果，不把旧正文写回编辑器。
func (a *nativeApp) saveDocument(id, content string, saveAs bool, done func(ok bool)) {
	if a.saving[id] {
		if done != nil {
			done(false)
		}
		return
	}
	if err := a.files.store.Draft(id, content); err != nil {
		a.notice = err.Error()
		if done != nil {
			done(false)
		}
		return
	}
	gen := a.saveGen[id] + 1
	a.saveGen[id] = gen
	a.saving[id] = true
	win := a.window()
	go func() {
		saved, err := a.saveSnapshot(win, id, content, saveAs)
		a.update(func() {
			defer func() {
				if a.saveGen[id] == gen {
					a.saving[id] = false
				}
			}()
			if a.saveGen[id] != gen {
				if done != nil {
					done(false)
				}
				return
			}
			if err != nil {
				if errors.Is(err, documents.ErrConflict) {
					a.conflicts[id] = true
				}
				a.notice = err.Error()
				if done != nil {
					done(false)
				}
				return
			}
			if saved == nil {
				if done != nil {
					done(false)
				}
				return
			}
			delete(a.conflicts, id)
			// 写盘期间用户继续输入时，只更新路径，正文以编辑器为准。
			if tab := a.tabByID(id); tab != nil && tab.editor != nil && tab.editor.Markdown() != content {
				_ = a.files.store.Draft(id, tab.editor.Markdown())
			}
			a.refreshTitle()
			a.persistRecovery()
			a.reloadNavigation()
			if done != nil {
				done(true)
			}
		})
	}()
}

// saveSnapshot 只使用发起保存的标签快照；切换标签不会改变保存目标。
func (a *nativeApp) saveSnapshot(win *mygo.Window, id, content string, saveAs bool) (*documents.Document, error) {
	doc, ok := a.document(id)
	if !ok {
		return nil, documents.ErrStale
	}
	path := doc.Path
	if path == "" || saveAs {
		var err error
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "保存 Markdown 文档", DefaultPath: doc.Name, Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}}})
		if err != nil || path == "" {
			return nil, err
		}
	}
	saved, err := a.files.store.SaveSnapshot(id, content, path)
	if err != nil {
		return nil, err
	}
	if a.files.workspace != nil {
		if err := a.files.workspace.rememberAuthorized(saved.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &saved, nil
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

// saveThen 逐个保存。未保存文档会弹出保存框；取消或失败时调用 failed，不再继续关闭。
func (a *nativeApp) saveThen(dirty []documents.Document, done func()) {
	a.saveThenNotify(dirty, done, nil)
}

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

func (a *nativeApp) chooseFolder() {
	win := a.window()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: win, Title: "选择工作区文件夹", Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		backend, err := a.workspace.backend()
		if err != nil {
			a.update(func() { a.notice = err.Error() })
			return
		}
		if _, err := backend.SelectFolder(paths[0]); err != nil {
			a.update(func() { a.notice = err.Error() })
			return
		}
		a.update(a.reloadNavigation)
	}()
}

func (a *nativeApp) openWorkspace(relative string) {
	a.syncEditor(a.active())
	go func() {
		doc, err := a.workspace.OpenDocument(relative)
		a.update(func() {
			if err != nil {
				a.notice = err.Error()
				return
			}
			loaded, loadErr := a.files.store.LoadSnapshot(doc.Path, doc.Raw)
			if loadErr != nil {
				a.notice = loadErr.Error()
				return
			}
			a.adopt(loaded, loaded.Content)
			a.reloadNavigation()
		})
	}()
}

func (a *nativeApp) openRecent(path string) {
	a.syncEditor(a.active())
	go func() {
		doc, err := a.workspace.OpenRecent(path)
		a.update(func() {
			if err != nil {
				a.notice = err.Error()
				return
			}
			loaded, loadErr := a.files.store.LoadSnapshot(doc.Path, doc.Raw)
			if loadErr != nil {
				a.notice = loadErr.Error()
				return
			}
			a.adopt(loaded, loaded.Content)
		})
	}()
}

func (a *nativeApp) askLink() {
	if a.reading {
		return
	}
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	a.promptKind, a.promptTitle, a.promptHint = "link", "", ""
	a.promptText = ""
	a.promptOpen = true
	editor := tab.editor
	a.promptOK = func(raw string) {
		if a.reading {
			return
		}
		label, url, ok := splitLink(raw)
		if !ok {
			a.notice = "请输入链接文字和地址，用空格分开"
			return
		}
		editor.InsertLink(label, url)
	}
}

func splitLink(raw string) (label, url string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) == 1 {
		return parts[0], parts[0], true
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), parts[1] != ""
}

func (a *nativeApp) insertImage() {
	if a.reading {
		return
	}
	tab := a.active()
	if tab == nil || tab.editor == nil || a.assets == nil {
		return
	}
	doc, ok := a.document(tab.id)
	if !ok {
		return
	}
	id := doc.ID
	editor := tab.editor
	win := a.window()
	go func() {
		imported, err := a.assets.ImportImageIn(win, id)
		a.update(func() {
			if err != nil || imported == nil {
				if err != nil {
					a.notice = err.Error()
				}
				return
			}
			// 对话框返回时模式可能已经改变，不能把图片插进只读正文。
			if a.reading {
				return
			}
			alt := strings.TrimSuffix(filepath.Base(imported.Path), filepath.Ext(imported.Path))
			if alt == "" {
				alt = "图片"
			}
			editor.InsertImage(alt, imported.Path)
		})
	}()
}

func (a *nativeApp) reloadActive(discard bool) {
	if tab := a.active(); tab != nil {
		a.reloadDocument(tab.id, discard)
	}
}

func (a *nativeApp) reloadDocument(id string, discard bool) {
	tab := a.tabByID(id)
	if tab == nil || tab.editor == nil || a.saving[id] {
		return
	}
	a.syncEditor(tab)
	before := tab.editor.Markdown()
	editor := tab.editor
	go func() {
		path, raw, err := a.files.store.ReadSnapshot(id)
		a.update(func() {
			if err != nil {
				a.notice = err.Error()
				return
			}
			tab := a.tabByID(id)
			if tab == nil || tab.editor != editor {
				return
			}
			if a.saving[id] || tab.editor.Markdown() != before {
				a.notice = "读取期间文档发生变化，请再次重新加载"
				return
			}
			a.syncEditor(tab)
			doc, err := a.files.store.ReloadSnapshot(id, path, raw, discard)
			if err != nil {
				if errors.Is(err, documents.ErrDirty) && !discard {
					a.confirmReload(id)
					return
				}
				a.notice = err.Error()
				return
			}
			delete(a.conflicts, id)
			a.replaceEditor(id, doc.Content)
			a.persistRecovery()
		})
	}()
}

func (a *nativeApp) confirmReload(id string) {
	win := a.window()
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: win, Type: mygo.MessageWarning, Message: "文件已在外部修改",
			Detail: "重新加载会丢弃当前未保存的修改。", Buttons: []string{"重新加载", "保留"},
			DefaultButton: 1, CancelButton: 1,
		})
		if err != nil || res.Button != 0 {
			return
		}
		a.update(func() { a.reloadDocument(id, true) })
	}()
}

func (a *nativeApp) runFind() {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	if strings.TrimSpace(a.findQuery) == "" {
		a.findNote = ""
		return
	}
	if tab.editor.Find(a.findQuery) {
		a.findNote = "已定位"
		return
	}
	a.findNote = "未找到"
}

func (a *nativeApp) setTheme(theme string) {
	a.changeSettings(func(s *nativeSettings) { s.Theme = theme })
}

func (a *nativeApp) setFont(size float32) {
	a.changeSettings(func(s *nativeSettings) { s.FontSize = max(12, min(28, size)) })
}

func (a *nativeApp) checkUpdate() {
	if a.updates == nil || a.updateBusy {
		return
	}
	a.updateBusy = true
	go func() {
		status, err := a.updates.Check(context.Background())
		a.update(func() {
			a.updateBusy = false
			a.updateNote = ""
			if err != nil {
				a.updateNote = err.Error()
				a.notice = err.Error()
			} else if status.Installed {
				a.notice = "更新已安装，重启后生效"
			} else if status.Available != "" {
				a.updateOpen, a.updateError = true, ""
			} else {
				a.updateNote = "已是最新版本。"
				a.notice = "当前已是最新版本"
			}
		})
	}()
}

// watchUpdates 把更新服务的后台通知接到界面：自动检查发现新版本时弹出更新窗口，下载时刷新进度。
func (a *nativeApp) watchUpdates() {
	if a.updates == nil {
		return
	}
	a.updates.availableFn = func(UpdateStatus) {
		a.update(func() {
			if !a.updateDownloading {
				a.updateOpen, a.updateError = true, ""
			}
		})
	}
	a.updates.progressFn = func(downloaded, total int64) {
		a.update(func() { a.updateDownloaded, a.updateTotal = downloaded, total })
	}
}

// installUpdate 下载并安装已发现的新版本；进度与结果显示在更新窗口里。
func (a *nativeApp) installUpdate() {
	if a.updates == nil || a.updateDownloading {
		return
	}
	a.updateDownloading, a.updateBusy = true, true
	a.updateDownloaded, a.updateTotal, a.updateError = 0, 0, ""
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel = cancel
	updates := a.updates
	go func() {
		defer cancel()
		_, err := updates.Install(ctx)
		a.update(func() {
			a.updateCancel = nil
			a.updateDownloading, a.updateBusy = false, false
			if err != nil {
				a.updateError = err.Error()
			}
		})
	}()
}

// dismissUpdate 取消当前下载并退出弹窗；后台结束前仍阻止重复安装。
func (a *nativeApp) dismissUpdate() {
	if a.updateCancel != nil {
		a.updateCancel()
	}
	a.updateOpen = false
}

func (a *nativeApp) persistSettings() {
	settings := a.settings
	go func() {
		if err := saveNativeSettings(settings); err != nil {
			log.Printf("保存设置失败：%v", err)
		}
	}()
}

func (a *nativeApp) persistRecovery() {
	a.syncAll()
	if a.recoveryBlocked {
		return
	}
	drafts := append([]nativeDraft{}, a.unrestored...)
	for _, doc := range a.files.store.List() {
		if !doc.Dirty {
			continue
		}
		drafts = append(drafts, nativeDraft{ID: doc.ID, Name: doc.Name, Path: doc.Path, Content: doc.Content})
	}
	ticket := nativeRecoveryGeneration.Add(1)
	go func() {
		if err := writeNativeRecoveryVersion(drafts, ticket); err != nil {
			log.Printf("保存恢复草稿失败：%v", err)
		}
	}()
}

func (a *nativeApp) clearRecovery() {
	if a.recoveryBlocked || len(a.unrestored) > 0 {
		return
	}
	ticket := nativeRecoveryGeneration.Add(1)
	if err := writeNativeRecoveryVersion(nil, ticket); err != nil {
		log.Printf("清理恢复记录失败：%v", err)
	}
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

// imageReader 把 Markdown 图片路径限制在当前文档已授权的资源内。
// 相对路径经 Assets.ReadImage 变成 data URI 再解码；已经是 data URI 的只解码，不读磁盘。
func imageReader(store *Assets, id string) func(string) *ui.Bitmap {
	return func(markdownPath string) *ui.Bitmap {
		if store == nil || markdownPath == "" {
			return nil
		}
		dataURI := markdownPath
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(markdownPath)), "data:") {
			uri, err := store.ReadImage(id, markdownPath)
			if err != nil || uri == "" {
				return nil
			}
			dataURI = uri
		}
		return decodeImageData(dataURI)
	}
}

func decodeImageData(dataURI string) *ui.Bitmap {
	const marker = "base64,"
	i := strings.Index(strings.ToLower(dataURI), marker)
	if i < 0 {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(dataURI[i+len(marker):]))
	if err != nil || len(raw) == 0 || len(raw) > 8<<20 {
		return nil
	}
	bm, err := ui.DecodeBitmap(raw)
	if err != nil {
		return nil
	}
	return bm
}

// noteEdits 记下正文发生变化的时刻。编辑器未暴露改动时间，不能用打开时刻代替。
func (a *nativeApp) noteEdits(now time.Time) {
	for _, tab := range a.tabs {
		if tab.editor == nil {
			continue
		}
		text := tab.editor.Markdown()
		if prev, ok := a.seenText[tab.id]; !ok || prev != text {
			if ok {
				a.editedAt[tab.id] = now
			}
			a.seenText[tab.id] = text
		}
	}
}

// scheduleAutoSave 在视图里按帧检查，距上次改动满 2 秒才写已有路径。
func (a *nativeApp) scheduleAutoSave(now time.Time) {
	a.noteEdits(now)
	if !a.settings.AutoSave {
		return
	}
	for _, tab := range a.tabs {
		if tab.editor == nil {
			continue
		}
		since := a.editedAt[tab.id]
		if since.IsZero() || now.Sub(since) < autoSaveDelay || a.saving[tab.id] {
			continue
		}
		doc, ok := a.document(tab.id)
		if !ok || doc.Path == "" || a.conflicts[tab.id] {
			continue
		}
		content := tab.editor.Markdown()
		if !doc.Dirty && doc.Content == content {
			delete(a.editedAt, tab.id)
			continue
		}
		delete(a.editedAt, tab.id)
		if err := a.files.store.Draft(tab.id, content); err != nil && !errors.Is(err, documents.ErrStale) {
			log.Printf("同步草稿失败：%v", err)
		}
		a.saveDocument(tab.id, content, false, nil)
	}
}
