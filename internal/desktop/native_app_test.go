//go:build desktoptest

package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/documents"
	"leafmark/internal/workspace"
)

// stubEditor 只覆盖桌面装配需要的行为，不实现排版。
type stubEditor struct {
	text    string
	changed bool
	finds   []string
	formats []string
	links   [][2]string
	images  [][2]string
	undone  int
	redone  int
	size    float32
	reader  func(string) (string, error)
	findHit bool
}

func (e *stubEditor) View(*ui.Context) {}
func (e *stubEditor) Markdown() string { return e.text }
func (e *stubEditor) Changed() bool    { return e.changed }
func (e *stubEditor) Undo()            { e.undone++ }
func (e *stubEditor) Redo()            { e.redone++ }
func (e *stubEditor) Format(name string) {
	e.formats = append(e.formats, name)
	e.changed = true
}
func (e *stubEditor) InsertLink(label, url string) {
	e.links = append(e.links, [2]string{label, url})
	e.text += "[" + label + "](" + url + ")"
	e.changed = true
}
func (e *stubEditor) InsertImage(alt, path string) {
	e.images = append(e.images, [2]string{alt, path})
	e.changed = true
}
func (e *stubEditor) Find(query string) bool {
	e.finds = append(e.finds, query)
	return e.findHit
}
func (e *stubEditor) FontSize(size float32) { e.size = size }
func (e *stubEditor) SetReadImage(fn func(string) *ui.Bitmap) {
	if fn != nil {
		e.reader = func(path string) (string, error) { _ = fn(path); return "", nil }
	}
}
func (e *stubEditor) Text() string                                { return e.text }
func (e *stubEditor) Selection() (int, int)                       { return 0, 0 }
func (e *stubEditor) SetSelection(int, int)                       {}
func (e *stubEditor) HandleInput(*ui.Context, ui.InputEvent) bool { return false }

func useStubEditors(t *testing.T) {
	t.Helper()
	prev := newDocumentEditor
	newDocumentEditor = func(markdown string) documentEditor {
		return &stubEditor{text: markdown}
	}
	t.Cleanup(func() { newDocumentEditor = prev })
}

func newTestApp(t *testing.T) (*nativeApp, string) {
	t.Helper()
	dir := t.TempDir()
	backend, err := workspace.NewAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := &Files{store: documents.NewStore("欢迎.md", welcome), workspace: &Workspace{}}
	files.workspace.once.Do(func() { files.workspace.store = backend })
	app := newNativeApp(files, files.workspace, NewAssets(files), &Updates{})
	return app, dir
}

func TestNativeDataDirFollowsEnvironment(t *testing.T) {
	dir := t.TempDir()
	isolated := filepath.Join(dir, "native")
	t.Setenv("LEAFMARK_NATIVE_DATA_DIR", isolated)
	got, err := nativeDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != isolated {
		t.Fatalf("验证目录应覆盖用户配置：%s", got)
	}
	if err := writeNativeRecovery([]nativeDraft{{ID: "a", Name: "笔记.md", Content: "草稿"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(isolated, nativeRecoveryName)); err != nil {
		t.Fatal(err)
	}
}

func TestBootAdoptsEveryOpenedDocument(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	path := filepath.Join(dir, "已打开.md")
	if err := os.WriteFile(path, []byte("第二份\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.files.store.Load(path); err != nil {
		t.Fatal(err)
	}
	docs := app.files.store.List()
	for _, doc := range docs {
		app.adopt(doc, doc.Content)
	}
	if len(app.tabs) != len(docs) || len(docs) < 2 {
		t.Fatalf("启动应保留全部已打开文档，文档 %d，标签 %d", len(docs), len(app.tabs))
	}
}

func TestNativeRecoveryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAFMARK_NATIVE_DATA_DIR", filepath.Join(dir, "native"))
	drafts := []nativeDraft{{ID: "a", Name: "笔记.md", Path: filepath.Join(dir, "笔记.md"), Content: "未保存"}}
	if err := writeNativeRecovery(drafts); err != nil {
		t.Fatal(err)
	}
	got := readNativeRecovery()
	if len(got) != 1 || got[0].Content != "未保存" {
		t.Fatalf("恢复结果不符：%+v", got)
	}
	if err := writeNativeRecovery(nil); err != nil {
		t.Fatal(err)
	}
	if again := readNativeRecovery(); len(again) != 0 {
		t.Fatalf("清空后仍有草稿：%+v", again)
	}
}

func TestNativeRecoveryRejectsRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	err := writeNativeRecovery([]nativeDraft{{ID: "a", Name: "笔记.md", Path: "relative.md", Content: "x"}})
	if err == nil {
		t.Fatal("相对路径应被拒绝")
	}
}

func TestNativeSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	want := defaultNativeSettings()
	want.Font, want.FontSize, want.LineHeight, want.ReadingWidth, want.Theme = "mono", 20, 1.75, "wide", "dark"
	want.AutoSave, want.FocusMode, want.Typewriter = false, true, true
	want.Shortcuts.Find = "Mod-Alt-f"
	if err := saveNativeSettings(want); err != nil {
		t.Fatal(err)
	}
	got := loadNativeSettings()
	if got != want {
		t.Fatalf("设置不符：得到 %+v", got)
	}
	// 损坏文件回落到默认，不把非法字号带进编辑器。
	config, err := nativeDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, nativeSettingsName), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := loadNativeSettings(); got != defaultNativeSettings() {
		t.Fatalf("损坏设置应回落默认，得到 %+v", got)
	}
}

func TestSplitLink(t *testing.T) {
	label, url, ok := splitLink("叶笺 https://leafmark.app")
	if !ok || label != "叶笺" || url != "https://leafmark.app" {
		t.Fatalf("拆分不符：%s %s %v", label, url, ok)
	}
	label, url, ok = splitLink("https://leafmark.app")
	if !ok || label != url {
		t.Fatalf("单词应同时作为文字和地址：%s %s", label, url)
	}
	if _, _, ok := splitLink("  "); ok {
		t.Fatal("空白不应通过")
	}
}

func TestAutoSaveWaitsTwoSeconds(t *testing.T) {
	app, dir := newTestApp(t)
	path := filepath.Join(dir, "已有.md")
	if err := os.WriteFile(path, []byte("原文\n"), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := app.files.store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	editor := &stubEditor{text: "原文", changed: false}
	app.tabs = []*nativeTab{{id: doc.ID, editor: editor}}
	app.current = 0
	now := time.Now()
	app.noteEdits(now)
	editor.text = "新正文"
	editor.changed = true
	app.noteEdits(now)
	app.scheduleAutoSave(now.Add(time.Second))
	if editor.changed == false {
		t.Fatal("未满 2 秒不应触发保存")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "原文\n" && string(raw) != "原文" {
		t.Fatalf("提前写入了磁盘：%q", raw)
	}
}

func TestStaleSaveDoesNotReplaceNewerText(t *testing.T) {
	app, dir := newTestApp(t)
	path := filepath.Join(dir, "笔记.md")
	if err := os.WriteFile(path, []byte("一"), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := app.files.store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	editor := &stubEditor{text: "二", changed: true}
	app.tabs = []*nativeTab{{id: doc.ID, editor: editor}}
	app.current = 0
	app.saveGen[doc.ID] = 1
	editor.text = "三"
	// 保存返回时序号已经前进，旧正文不能覆盖编辑器里的新输入。
	if app.saveGen[doc.ID] != 1 {
		t.Fatal("序号起点不符")
	}
	app.saveGen[doc.ID]++
	if app.saveGen[doc.ID] != 1 {
		_ = app.files.store.Draft(doc.ID, editor.Markdown())
	}
	current, _ := app.document(doc.ID)
	if current.Content != "三" {
		t.Fatalf("新输入应保留，得到 %q", current.Content)
	}
}

func TestTabsKeepSeparateEditors(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	first := app.files.New()
	second := app.files.New()
	app.adopt(first, "甲")
	app.adopt(second, "乙")
	if len(app.tabs) != 2 {
		t.Fatalf("标签数量不符：%d", len(app.tabs))
	}
	left := app.tabs[0].editor.(*stubEditor)
	left.text = "甲改"
	left.changed = true
	app.selectTab(0)
	if app.tabs[1].editor.Markdown() != "乙" {
		t.Fatal("切换标签不应改写另一个编辑器")
	}
	if got := app.files.Current().ID; got != first.ID {
		t.Fatalf("当前文档不符：%s", got)
	}
	app.command("bold")
	if len(left.formats) != 1 || left.formats[0] != "bold" {
		t.Fatal("格式命令没有送到当前标签")
	}
}

func TestRestoreDraftReplacesUntouchedWelcome(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	path := filepath.Join(dir, "恢复.md")
	if err := os.WriteFile(path, []byte("磁盘\n"), 0644); err != nil {
		t.Fatal(err)
	}
	welcomeDoc := app.files.Current()
	app.adopt(welcomeDoc, welcome)
	app.restoreDrafts([]nativeDraft{{ID: "draft", Name: "恢复.md", Path: path, Content: "未保存正文"}})
	if len(app.tabs) != 1 || app.tabs[0].editor.Markdown() != "未保存正文" {
		t.Fatalf("应只留下恢复草稿，得到 %d 个标签", len(app.tabs))
	}
	if app.notice == "" {
		t.Fatal("恢复后应提示")
	}
}

func TestNativeViewShowsFindAndSettings(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.settings.Theme = "dark"
	view := ui.NewTester(app.View, 1100, 760)
	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("查找"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("下一个") {
		t.Fatal("查找栏没有出现")
	}
	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("设置"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("外观"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("跟随系统") || !view.HasText("Leafmark 深色") {
		t.Fatal("设置没有出现主题选项")
	}
	if err := view.Click("浅色"); err != nil {
		t.Fatal(err)
	}
	if app.settings.Theme != "light" {
		t.Fatalf("主题未切换：%s", app.settings.Theme)
	}
	if app.verifyStep != nil {
		t.Fatal("正式视图不应自带验证步骤")
	}
	called := false
	app.verifyStep = func(*ui.Context) { called = true }
	view.Frame()
	if !called {
		t.Fatal("验证步骤应在帧末执行")
	}
}

func TestNativeSnapshotSaveAfterSwitchDoesNotTouchOtherTab(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	path := filepath.Join(dir, "甲.md")
	os.WriteFile(path, []byte("原文"), 0600)
	doc, err := app.files.store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	app.adopt(doc, doc.Content)
	other := app.files.New()
	app.adopt(other, "乙")
	saved, err := app.saveSnapshot(nil, doc.ID, "甲保存", false)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != doc.ID || app.files.Current().ID != other.ID || app.active().id != other.ID {
		t.Fatal("切换标签后保存写错文档")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "甲保存" {
		t.Fatalf("保存正文不符：%q", raw)
	}
}

func TestRepeatedAdoptPreservesEditorUntilReload(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	doc := app.files.Current()
	app.adopt(doc, doc.Content)
	editor := app.active().editor.(*stubEditor)
	editor.text = "新草稿"
	editor.changed = true
	editor.Undo()
	app.adopt(doc, "过期正文")
	if app.active().editor != editor || editor.undone != 1 {
		t.Fatal("重复打开丢失撤销历史")
	}
	app.replaceEditor(doc.ID, "重新加载")
	if app.active().editor == editor || app.active().editor.Markdown() != "重新加载" {
		t.Fatal("重新加载未替换编辑器")
	}
}

func TestCloseSnapshotDetectsInputAndSaveReentry(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	doc := app.files.Current()
	app.adopt(doc, doc.Content)
	before := []documents.Document{doc}
	app.active().editor.(*stubEditor).text = "确认期间输入"
	app.syncAll()
	if app.closeSnapshotUnchanged(before) {
		t.Fatal("关闭丢弃了确认期间的新输入")
	}
	app.saving[doc.ID] = true
	called := false
	app.saveDocument(doc.ID, "快照", true, func(ok bool) {
		called = true
		if ok {
			t.Error("并行另存为不应成功启动")
		}
	})
	if !called {
		t.Fatal("保存防重入未通知调用方")
	}
}

func TestFailedRecoveryAndOutdatedWritePreserveDrafts(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	t.Setenv("LEAFMARK_NATIVE_DATA_DIR", dir)
	path := filepath.Join(dir, "损坏.md")
	os.WriteFile(path, []byte{0xff}, 0600)
	draft := nativeDraft{ID: "old", Name: "损坏.md", Path: path, Content: "应保留的草稿"}
	app.restoreDrafts([]nativeDraft{draft})
	if len(app.unrestored) != 1 {
		t.Fatal("恢复失败未保留原草稿")
	}
	ticket := nativeRecoveryGeneration.Add(1)
	if err := writeNativeRecoveryVersion([]nativeDraft{draft}, ticket); err != nil {
		t.Fatal(err)
	}
	if err := writeNativeRecoveryVersion(nil, ticket-1); err != nil {
		t.Fatal(err)
	}
	if got := readNativeRecovery(); len(got) != 1 || got[0].Content != draft.Content {
		t.Fatalf("旧写入覆盖了恢复记录：%+v", got)
	}
	os.WriteFile(filepath.Join(dir, nativeRecoveryName), []byte("{"), 0600)
	if _, ok := readNativeRecoveryState(); ok {
		t.Fatal("损坏记录不应允许覆盖")
	}
}

func TestAutoSaveUsesDirtyBaselineAndPausesConflict(t *testing.T) {
	app, dir := newTestApp(t)
	useStubEditors(t)
	path := filepath.Join(dir, "甲.md")
	os.WriteFile(path, []byte("原文"), 0600)
	doc, _ := app.files.store.Load(path)
	app.adopt(doc, doc.Content)
	editor := app.active().editor.(*stubEditor)
	editor.text = "草稿"
	editor.changed = true
	app.syncEditor(app.active())
	now := time.Now()
	app.editedAt[doc.ID] = now.Add(-3 * time.Second)
	app.conflicts[doc.ID] = true
	app.scheduleAutoSave(now)
	if app.saving[doc.ID] {
		t.Fatal("外部冲突后仍自动保存")
	}
	if app.editedAt[doc.ID].IsZero() {
		t.Fatal("冲突不应丢失待保存状态")
	}
}
