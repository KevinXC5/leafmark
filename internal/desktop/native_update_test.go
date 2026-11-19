//go:build desktoptest

package desktop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

func TestUpdateDialogShowsProgressAndRestart(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	restarted := 0
	app.updates = &Updates{
		versionFn: func() string { return "0.5.0" },
		enabledFn: func() bool { return true },
		checkFn: func(context.Context) (*mygo.Update, error) {
			return &mygo.Update{Version: "0.6.0", Notes: "概述一句。\n\n## 新增功能\n\n- 新增若干**能力**。\n- 另一条。\n"}, nil
		},
		installFn: func(context.Context, *mygo.Update, func(int64, int64)) error { return nil },
		restartFn: func() { restarted++ },
	}
	app.watchUpdates()
	if _, err := app.updates.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 书写界面里也能弹出，不依赖设置页。
	app.updateOpen = true
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	for _, text := range []string{"发现新版本 0.6.0", "当前版本：0.5.0", "概述一句。", "新增功能", "新增若干能力。", "确定升级", "取消"} {
		if !view.HasText(text) {
			t.Fatalf("更新窗口缺少 %q", text)
		}
	}
	for _, text := range view.Texts() {
		if strings.Contains(text, "##") || strings.Contains(text, "**") || strings.HasPrefix(text, "- ") {
			t.Fatalf("更新日志露出了 Markdown 标记：%q", text)
		}
	}
	if _, shown := view.Find("更新下载进度"); shown {
		t.Fatal("开始下载前不应显示进度条")
	}

	for _, step := range []struct {
		downloaded, total int64
		hint              string
		fraction          float64
	}{
		{0, 0, "正在连接下载，请稍候…", -1},
		{512, 0, "正在下载更新，请稍候…", -1},
		{250, 1000, "正在下载更新… 25%", .25},
		{1000, 1000, "下载完成，正在校验并安装更新…", 1},
	} {
		app.updateDownloading, app.updateDownloaded, app.updateTotal = true, step.downloaded, step.total
		hint, fraction, shown := app.updateProgress(false)
		if hint != step.hint || fraction != step.fraction || !shown {
			t.Fatalf("进度 %d/%d：%q %v %v", step.downloaded, step.total, hint, fraction, shown)
		}
		view.Frame()
		if _, ok := view.Find("更新下载进度"); !ok || !view.HasText(step.hint) {
			t.Fatalf("下载中应显示进度条与“%s”", step.hint)
		}
	}
	// 下载期间可用 Escape 取消，取消信号必须传到后台请求。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.updateCancel = cancel
	view.Key(0, ui.KeyEscape)
	view.Frame()
	if app.updateOpen || ctx.Err() != context.Canceled {
		t.Fatal("Escape 应退出窗口并取消下载")
	}
	app.updateOpen = true
	ctx, cancelButton := context.WithCancel(context.Background())
	defer cancelButton()
	app.updateCancel = cancelButton
	view.Frame()
	if err := view.Click("取消"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if app.updateOpen || ctx.Err() != context.Canceled {
		t.Fatal("取消按钮应退出窗口并取消下载")
	}
	app.updateOpen = true

	// 安装失败：收起进度条并说明原因，可以重试。
	app.updateDownloading, app.updateError = false, "安装更新失败，当前版本仍可继续使用，请稍后重试"
	view.Frame()
	if _, ok := view.Find("更新下载进度"); ok || !view.HasText(app.updateError) || !view.HasText("确定升级") {
		t.Fatal("失败后应隐藏进度条并保留升级按钮")
	}

	// 安装完成：满格进度，按钮变成“稍后”与“重启应用”。
	app.updateError = ""
	if _, err := app.updates.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if _, ok := view.Find("更新下载进度"); !ok || !view.HasText("更新已安装，重启应用即可使用新版本。") || !view.HasText("稍后") || view.HasText("确定升级") {
		t.Fatalf("安装完成后的窗口不符：%v", view.Texts())
	}
	if err := view.Click("重启应用"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if restarted != 1 {
		t.Fatalf("“重启应用”未触发重启：%d", restarted)
	}
	if err := view.Click("稍后"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if app.updateOpen {
		t.Fatal("“稍后”应关闭窗口")
	}
}

func TestUpdateCallbacksReachBackend(t *testing.T) {
	app, _ := newTestApp(t)
	var seen [][2]int64
	app.updates = &Updates{
		enabledFn: func() bool { return true },
		installFn: func(_ context.Context, _ *mygo.Update, progress func(int64, int64)) error {
			progress(10, 100)
			progress(100, 100)
			return nil
		},
	}
	app.watchUpdates()
	if app.updates.availableFn == nil || app.updates.progressFn == nil {
		t.Fatal("界面没有接上更新通知")
	}
	// 没有窗口时界面回调不落地，这里只确认后端把真实字节数交给了回调。
	app.updates.progressFn = func(downloaded, total int64) { seen = append(seen, [2]int64{downloaded, total}) }
	app.updates.pending = &mygo.Update{Version: "0.6.0"}
	if _, err := app.updates.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[1] != [2]int64{100, 100} {
		t.Fatalf("下载进度未送达：%v", seen)
	}
}

func TestUpdateProgressRemainsVisibleWithLongNotes(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.updates = &Updates{
		pending:   &mygo.Update{Version: "9.9.9", Notes: strings.Repeat("## 更新内容\n\n- 一项较长的更新说明。\n\n", 80)},
		versionFn: func() string { return "0.5.1" },
		enabledFn: func() bool { return true },
	}
	app.updateOpen, app.updateDownloading = true, true
	app.updateDownloaded, app.updateTotal = 25, 100
	view := ui.NewTester(app.View, 760, 560)
	view.Frame()
	bar, ok := view.Find("更新下载进度")
	if !ok || bar.H < 6 || bar.Y < 0 || bar.Y+bar.H > 560 {
		t.Fatalf("长日志挤掉了进度条：%+v，存在=%v", bar, ok)
	}
	button, ok := view.Find("确定升级")
	if !ok || button.H <= 0 || button.Y+button.H > 560 {
		t.Fatalf("长日志挤掉了升级按钮：%+v，存在=%v", button, ok)
	}
}

func TestUpdateDialogListsEveryVersionSinceCurrent(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.updates = &Updates{
		versionFn: func() string { return "0.5.1" },
		enabledFn: func() bool { return true },
		checkFn: func(context.Context) (*mygo.Update, error) {
			return &mygo.Update{Version: "0.6.1", Notes: "### 0.6.1\n\n修复查找定位。\n\n### 0.6.0\n\n补齐原生编辑能力。\n\n#### 新增功能\n\n- 新增阅读模式。\n\n### 0.5.1\n\n已经装上的版本。\n"}, nil
		},
	}
	if _, err := app.updates.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	app.updateOpen = true
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	for _, text := range []string{"发现新版本 0.6.1", "0.6.1", "修复查找定位。", "0.6.0", "补齐原生编辑能力。", "新增功能", "新增阅读模式。"} {
		if !view.HasText(text) {
			t.Fatalf("更新窗口缺少 %q", text)
		}
	}
	if view.HasText("已经装上的版本。") {
		t.Fatal("更新日志不应包含当前及更早版本的说明")
	}
}

func TestUpdateNotesBulletTopAlignedAndScrollReset(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	longItem := "在长文档中静止浏览或滚动时，编辑器会减少重复渲染与排版计算，降低 CPU 占用，滚动更顺畅，并且确保这条列表说明能够折行显示。"
	notes := "### 0.9.7\n\n本次更新提升了长文档浏览与稳定性。\n\n#### 使用体验\n\n- " + longItem + "\n" +
		strings.Repeat("- 另一条用于撑高更新日志滚动区域的说明内容，让内容明显超出弹窗卡片高度。\n", 14)
	app.updates = &Updates{
		pending:   &mygo.Update{Version: "0.9.7", Notes: notes},
		versionFn: func() string { return "0.9.2" },
		enabledFn: func() bool { return true },
	}
	app.updateOpen = true
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()

	bullet, ok := view.Find("•")
	if !ok {
		t.Fatal("未找到列表圆点")
	}
	item, ok := view.Find(longItem)
	if !ok {
		t.Fatal("未找到多行列表正文")
	}
	if item.H <= bullet.H*1.5 {
		t.Fatalf("测试列表项未折行：bullet=%+v item=%+v", bullet, item)
	}
	diff := bullet.Y - item.Y
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		t.Fatalf("列表圆点应与首行顶部对齐，而非垂直居中：bullet=%+v item=%+v", bullet, item)
	}
	if app.updateNotesScroll.MaxY <= 0 {
		t.Fatalf("长更新日志应触发可滚动区域：MaxY=%v", app.updateNotesScroll.MaxY)
	}

	app.updateNotesScroll.Y = app.updateNotesScroll.MaxY / 2
	view.Frame()
	if app.updateNotesScroll.Y <= 0 {
		t.Fatalf("滚动更新日志后应记录偏移：Y=%v", app.updateNotesScroll.Y)
	}

	app.dismissUpdate()
	if app.updateNotesScroll.Y != 0 || app.updateNotesScroll.MaxY != 0 {
		t.Fatalf("关闭更新弹窗后应重置滚动状态：%+v", app.updateNotesScroll)
	}
}

func TestUpdateNotesViewCachesParsedMarkdown(t *testing.T) {
	// 重置包级缓存避免受其他测试影响。
	releaseNotesCache.mu.Lock()
	releaseNotesCache.ready = false
	releaseNotesCache.markdown = ""
	releaseNotesCache.blocks = nil
	releaseNotesCache.mu.Unlock()

	parses := 0
	prev := parseReleaseNoteBlocks
	parseReleaseNoteBlocks = func(markdown string) []richtext.Block {
		parses++
		return richtext.Parse(markdown).Blocks()
	}
	t.Cleanup(func() { parseReleaseNoteBlocks = prev })

	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.updates = &Updates{
		pending: &mygo.Update{
			Version: "0.9.8",
			Notes:   "### 0.9.8\n\n#### 优化\n\n- 减少动画重绘开销。\n\n### 0.9.7\n\n- 另一项改进。\n",
		},
		versionFn: func() string { return "0.9.6" },
		enabledFn: func() bool { return true },
	}
	app.updateOpen, app.updateDownloading = true, true
	view := ui.NewTester(app.View, 1100, 760)
	for range 8 {
		view.Frame()
	}
	if parses != 1 {
		t.Fatalf("同一更新日志多帧渲染不应重复解析 Markdown，实际解析 %d 次", parses)
	}
	blocks := cachedReleaseNoteBlocks(app.updates.Status().Notes)
	versionHeadings, normalHeadings := 0, 0
	for _, b := range blocks {
		if b.block.Kind == richtext.Heading {
			if b.isVersion {
				versionHeadings++
			} else {
				normalHeadings++
			}
		}
	}
	if versionHeadings != 2 || normalHeadings != 1 {
		t.Fatalf("版本号标题判定异常：version=%d normal=%d", versionHeadings, normalHeadings)
	}

	// 更换更新日志后应重新解析一次。
	app.updates.pending = &mygo.Update{Version: "0.9.9", Notes: "全新日志。"}
	view.Frame()
	view.Frame()
	if parses != 2 {
		t.Fatalf("更换更新日志后应仅重新解析一次，实际解析 %d 次", parses)
	}
}

func TestUpdateCancelAndReopenDialogClearsStaleError(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	app.updates = &Updates{
		pending:   &mygo.Update{Version: "0.9.8", Notes: "新版本说明。"},
		versionFn: func() string { return "0.9.7" },
		enabledFn: func() bool { return true },
	}
	// 模拟取消下载后残留的取消提示，关闭弹窗后在通用设置页再次点击“查看更新”时应清空过时提示。
	app.updateOpen = true
	app.updateError = (updateCanceledError{}).Error()
	app.dismissUpdate()
	if app.updateOpen {
		t.Fatal("取消关闭弹窗后应退出更新窗口")
	}
	if app.updateError != "更新已取消，可稍后重试" {
		t.Fatalf("取消下载后应保留状态记录供后台核对：%q", app.updateError)
	}

	app.settingsOn, app.settingsSection = true, "general"
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	if err := view.Click("查看更新"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !app.updateOpen || app.updateError != "" {
		t.Fatalf("重新点击查看更新后应打开干净弹窗：open=%v err=%q", app.updateOpen, app.updateError)
	}
	if view.HasText("更新已取消，可稍后重试") {
		t.Fatal("重开更新弹窗不应展示上次取消错误提示")
	}
	if !view.HasText("安装完成后可重启应用以使用新版本，重启前会检查未保存的文档。") {
		t.Fatalf("重开更新弹窗应恢复默认提示：%v", view.Texts())
	}
}

func TestUpdateProgressThrottlePreservesTransitionsAndCompletion(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	prevNow := updateProgressNow
	updateProgressNow = func() time.Time { return now }
	t.Cleanup(func() { updateProgressNow = prevNow })

	var emitted [][2]int64
	cb := throttleUpdateProgress(func(downloaded, total int64) {
		emitted = append(emitted, [2]int64{downloaded, total})
	})

	// 1. 首次回调立即上报（连接中）
	cb(0, 0)
	// 2. 10ms 后开始有字节（状态从连接中变为下载中），应立即上报
	now = now.Add(10 * time.Millisecond)
	cb(100, 0)
	// 3. 10ms 后同阶段小幅增长，应被节流
	now = now.Add(10 * time.Millisecond)
	cb(200, 0)
	// 4. 5ms 后得知总大小（状态变化），应立即上报
	now = now.Add(5 * time.Millisecond)
	cb(300, 1000)
	// 5. 20ms 后未满 50ms，应被节流
	now = now.Add(20 * time.Millisecond)
	cb(500, 1000)
	// 6. 再过 30ms（距上次上报满 50ms），应上报
	now = now.Add(30 * time.Millisecond)
	cb(700, 1000)
	// 7. 5ms 后下载完成（1000/1000），即便未满 50ms 也必须立即上报
	now = now.Add(5 * time.Millisecond)
	cb(1000, 1000)
	// 8. 5ms 后重复完成回调，应被节流
	now = now.Add(5 * time.Millisecond)
	cb(1000, 1000)
	// 9. 5ms 后新一轮下载重试（字节数回退），应立即上报
	now = now.Add(5 * time.Millisecond)
	cb(50, 1000)

	want := [][2]int64{
		{0, 0},
		{100, 0},
		{300, 1000},
		{700, 1000},
		{1000, 1000},
		{50, 1000},
	}
	if len(emitted) != len(want) {
		t.Fatalf("节流上报次数不符：got %v, want %v", emitted, want)
	}
	for i := range want {
		if emitted[i] != want[i] {
			t.Fatalf("第 %d 次上报不符：got %v, want %v", i, emitted[i], want[i])
		}
	}
}
