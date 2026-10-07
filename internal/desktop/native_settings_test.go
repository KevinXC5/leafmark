//go:build desktoptest

package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// styledEditor 在桩编辑器之上记录排版设置，用来核对设置确实传到了编辑器。
type styledEditor struct {
	stubEditor
	family     string
	lineHeight float32
	width      float32
	focus      bool
	typewriter bool
}

func (e *styledEditor) SetFontFamily(family string) { e.family = family }
func (e *styledEditor) SetLineHeight(v float32)     { e.lineHeight = v }
func (e *styledEditor) SetReadingWidth(v float32)   { e.width = v }
func (e *styledEditor) SetFocusMode(on bool)        { e.focus = on }
func (e *styledEditor) SetTypewriter(on bool)       { e.typewriter = on }

func useStyledEditors(t *testing.T) {
	t.Helper()
	prev := newDocumentEditor
	newDocumentEditor = func(markdown string) documentEditor {
		return &styledEditor{stubEditor: stubEditor{text: markdown}}
	}
	t.Cleanup(func() { newDocumentEditor = prev })
}

func TestSettingsValidationFallsBackPerField(t *testing.T) {
	bad := nativeSettings{Font: "comic", FontSize: 99, LineHeight: 9, ReadingWidth: "huge", Theme: "neon", FocusMode: true}
	bad.Shortcuts = defaultNativeShortcuts()
	bad.Shortcuts.Save = "s" // 没有修饰键
	got := bad.normalized()
	want := defaultNativeSettings()
	want.AutoSave, want.FocusMode = false, true
	if got != want {
		t.Fatalf("非法字段应逐项回落默认：%+v", got)
	}
	if err := saveNativeSettings(bad); err == nil {
		t.Fatal("不应保存无效设置")
	}
	// 旧版设置文件只有三个字段，其余保持默认。
	t.Setenv("LEAFMARK_NATIVE_DATA_DIR", t.TempDir())
	dir, _ := nativeDataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativeSettingsName), []byte(`{"fontSize":18,"theme":"dark","autoSave":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	old := loadNativeSettings()
	if old.FontSize != 18 || old.Theme != "dark" || old.AutoSave || old.Font != "newsreader" || old.LineHeight != 1.4 || old.Shortcuts != defaultNativeShortcuts() {
		t.Fatalf("旧设置迁移错误：%+v", old)
	}
}

func TestSettingsApplyToEveryEditor(t *testing.T) {
	app, _ := newTestApp(t)
	useStyledEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.changeSettings(func(s *nativeSettings) {
		s.Font, s.FontSize, s.LineHeight, s.ReadingWidth, s.FocusMode, s.Typewriter = "mono", 18, 1.6, "narrow", true, true
	})
	check := func(where string) {
		t.Helper()
		e := app.active().editor.(*styledEditor)
		if e.size != 18 || e.family != editorFonts["mono"] || e.lineHeight != 1.6 || e.width != 600 || !e.focus || !e.typewriter {
			t.Fatalf("%s：设置未应用到编辑器：%+v", where, e)
		}
	}
	check("已打开的标签")
	if app.sidebar != 0 {
		t.Fatal("专注模式应收起侧栏")
	}
	app.adopt(app.files.New(), "")
	check("新标签")
	app.command("source")
	app.command("source")
	check("切回原位编辑")
	app.changeSettings(func(s *nativeSettings) { s.FocusMode = false })
	if app.sidebar != 248 {
		t.Fatal("关闭专注模式应恢复侧栏")
	}
}

func TestShortcutValidationConflictAndParsing(t *testing.T) {
	for raw, want := range map[string]string{"mod-S": "Mod-s", "Shift-Alt-F3": "Alt-Shift-F3", "cmd-,": "Meta-,", "Ctrl-Alt-k": "Ctrl-Alt-k"} {
		if got, problem := validateShortcut(raw); got != want || problem != "" {
			t.Fatalf("%q 规范化为 %q（%s），预期 %q", raw, got, problem, want)
		}
	}
	for _, raw := range []string{"s", "Shift-s", "Mod-Ctrl-s", "Mod-Mod-s", "Mod-enter", "Mod-c", "Mod-z", "Mod-Shift-n", "Mod-p", "Alt-F4", "Mod-F5", "Hyper-s"} {
		if got, problem := validateShortcut(raw); problem == "" {
			t.Fatalf("%q 应被拒绝，得到 %q", raw, got)
		}
	}
	bindings := defaultNativeShortcuts()
	if other := shortcutConflict(bindings, "find", "Mod-s"); other != "save" {
		t.Fatalf("应与保存冲突：%q", other)
	}
	// Mod 在两个平台分别等于 Meta 与 Ctrl，显式写法同样算冲突。
	if shortcutConflict(bindings, "find", "Ctrl-s") != "save" || shortcutConflict(bindings, "find", "Meta-s") != "save" {
		t.Fatal("跨平台重合未被发现")
	}
	if shortcutConflict(bindings, "find", "Mod-f") != "" || shortcutConflict(bindings, "find", "Mod-Alt-f") != "" {
		t.Fatal("自身或未占用的组合不应冲突")
	}
	bindings.Bold = "Mod-s"
	if checkedShortcuts(bindings) != defaultNativeShortcuts() {
		t.Fatal("互相冲突的绑定应整体回落默认")
	}
	defaults := defaultNativeShortcuts()
	for _, item := range shortcutActions {
		binding := *defaults.binding(item.id)
		mods, key, ok := parseShortcut(binding)
		if !ok || recordedShortcut(mods, key) != binding {
			t.Fatalf("%s 解析与录制不互逆：%q → %q", item.label, binding, recordedShortcut(mods, key))
		}
	}
	if recordedShortcut(ui.Cmd, ui.KeyEnter) != "" {
		t.Fatal("不支持的主键不应生成绑定")
	}
}

func TestCloseShortcutOnlyClosesSelectedTab(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "未保存的欢迎文档")
	app.syncAll()
	welcomeID := app.active().id
	path := filepath.Join(t.TempDir(), "当前文档.md")
	if err := os.WriteFile(path, []byte("已保存正文"), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := app.files.store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	app.adopt(doc, doc.Content)
	view := ui.NewTester(app.View, 1100, 760)
	view.Key(ui.Cmd, ui.KeyW)
	view.Frame()
	if app.tabByID(doc.ID) != nil || app.tabByID(welcomeID) == nil || app.closePrompt {
		t.Fatal("关闭当前已保存标签不应询问其他标签或关闭它们")
	}
	// 应用只在 macOS 安装此菜单；Windows 的默认窗口菜单仍可关闭窗口。
	if runtime.GOOS != "darwin" {
		return
	}
	menu := nativeApplicationMenu(app)
	var inspect func([]*mygo.MenuItem)
	inspect = func(items []*mygo.MenuItem) {
		for _, item := range items {
			if item.Role == mygo.RoleClose || item.Role == mygo.RoleFileMenu {
				t.Fatal("系统菜单不能抢占关闭标签的快捷键")
			}
			inspect(item.Submenu)
		}
	}
	inspect(menu.Items())
	if item := menu.ItemByID("close-tab"); item == nil || item.Click == nil || item.Accelerator != "" {
		t.Fatal("菜单关闭应调用标签命令，快捷键由可自定义绑定统一处理")
	}
}

func TestCustomShortcutDrivesCommands(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	view.Key(ui.Cmd, ui.KeyF)
	view.Frame()
	if !app.findOn {
		t.Fatal("默认的查找快捷键未生效")
	}
	app.findOn = false
	app.shortcutAction = "find"
	app.recordShortcut("Mod-s")
	if app.shortcutCandidate != "" || app.shortcutStatus != "与“保存”冲突，请录制其他组合键。" {
		t.Fatalf("冲突的组合不应成为候选：%q %q", app.shortcutCandidate, app.shortcutStatus)
	}
	app.recordShortcut("Mod-Alt-f")
	if app.shortcutCandidate != "Mod-Alt-f" {
		t.Fatalf("可用组合未成为候选：%q", app.shortcutStatus)
	}
	app.changeSettings(func(s *nativeSettings) { s.Shortcuts.Find = "Mod-Alt-f" })
	view.Key(ui.Cmd, ui.KeyF)
	view.Frame()
	if app.findOn {
		t.Fatal("旧绑定不应继续生效")
	}
	view.Key(ui.Cmd|ui.Alt, ui.KeyF)
	view.Frame()
	if !app.findOn {
		t.Fatal("新绑定未生效")
	}
}

func TestSettingsPageSectionsAndControls(t *testing.T) {
	app, _ := newTestApp(t)
	useStyledEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.settings.Theme = "light"
	app.settingsOn = true
	for _, size := range [][2]int{{1100, 760}, {660, 620}, {500, 620}} {
		view := ui.NewTester(app.View, size[0], size[1])
		for _, tab := range settingsTabs {
			app.settingsSection = tab.id
			view.Frame()
			if !view.HasText(tab.intro) {
				t.Fatalf("%v：“%s”页没有渲染", size, tab.label)
			}
		}
	}
	view := ui.NewTester(app.View, 1100, 760)
	app.settingsSection = "editor"
	view.Frame()
	var tops []float32
	for _, tab := range settingsTabs {
		r, found := view.Find(tab.label)
		if !found || r.H != 42 || r.W != 218 {
			t.Fatalf("分类标签“%s”尺寸不符：%+v", tab.label, r)
		}
		tops = append(tops, r.Y)
	}
	for i := 1; i < len(tops); i++ {
		if tops[i]-tops[i-1] != 48 {
			t.Fatalf("分类标签应逐个排开，间隔 6：%v", tops)
		}
	}
	side, ok := view.Find("设置导航")
	if !ok || side.X != 8 || side.Y != 8 || side.W != 248 || side.H != 744 {
		t.Fatalf("设置侧栏尺寸不符：%+v", side)
	}
	for _, text := range []string{"Leafmark", "排版与布局", "书写与保存", "个性化", "留一点空间，给书写。", "调整即时生效", "恢复默认设置"} {
		if !view.HasText(text) {
			t.Fatalf("编辑器页缺少 %q", text)
		}
	}
	click := func(label string) {
		t.Helper()
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
		view.Frame()
	}
	click("增大字号")
	click("宽")
	click("切换自动保存")
	if app.settings.FontSize != 16 || app.settings.ReadingWidth != "wide" || app.settings.AutoSave {
		t.Fatalf("控件未改变设置：%+v", app.settings)
	}
	if e := app.active().editor.(*styledEditor); e.size != 16 || e.width != 1040 {
		t.Fatalf("设置未即时作用到编辑器：%+v", e)
	}
	if err := view.Click("选择正文字体"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("等宽"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("选择行高"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("1.8"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if app.settings.Font != "mono" || app.settings.LineHeight != 1.8 {
		t.Fatalf("下拉未改变设置：%+v", app.settings)
	}
	click("前往外观")
	if app.settingsSection != "appearance" || !view.HasText("Leafmark 深色") {
		t.Fatal("个性化入口应跳到外观页")
	}
	click("深色")
	if app.settings.Theme != "dark" {
		t.Fatalf("外观模式未切换：%s", app.settings.Theme)
	}
	view.SetSize(1100, 1000) // 快捷键列表较长，放大窗口让底部按钮进入可视区
	click("快捷键")
	click("自定义快捷键…")
	if !app.shortcutOpen || !view.HasText("快捷键设置") || !view.HasText("当前绑定：Mod-s") {
		t.Fatalf("快捷键设置对话框未打开：%v %v", app.shortcutOpen, view.Texts())
	}
	click("返回")
	click("通用")
	if app.shortcutOpen || app.settingsSection != "general" {
		t.Fatalf("未进入通用页：%v %s %v", app.shortcutOpen, app.settingsSection, view.Texts())
	}
	click("切换专注模式")
	click("切换打字机模式")
	if !app.settings.FocusMode || !app.settings.Typewriter {
		t.Fatalf("书写习惯开关未生效：%+v", app.settings)
	}
	if !view.HasText("当前版本：开发版") || !view.HasText("检查更新") {
		t.Fatal("通用页缺少软件更新")
	}
	app.recent = nil
	if err := view.Click("开始清理恢复草稿"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !view.HasText("清理后无法恢复，是否继续？") {
		t.Fatal("清理前应先确认")
	}
	click("确认清理")
	if app.confirmClear != "" || app.settingsStatus != "清理恢复草稿完成" {
		t.Fatalf("清理状态错误：%q", app.settingsStatus)
	}
	click("恢复默认设置")
	want := defaultNativeSettings()
	if app.settings != want {
		t.Fatalf("未恢复默认设置：%+v", app.settings)
	}
	// 设置页里不响应文档快捷键，Escape 返回书写。
	tabs := len(app.tabs)
	view.Key(ui.Cmd, ui.KeyN)
	view.Frame()
	if len(app.tabs) != tabs {
		t.Fatal("设置页里不应新建文档")
	}
	view.Key(0, ui.KeyEscape)
	view.Frame()
	if app.settingsOn {
		t.Fatal("Escape 应返回书写")
	}
}
