//go:build desktoptest

package desktop

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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
	// 下载期间不能关掉窗口，也不能重复点击。
	view.Key(0, ui.KeyEscape)
	view.Frame()
	if !app.updateOpen {
		t.Fatal("下载期间窗口不应被关闭")
	}

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
