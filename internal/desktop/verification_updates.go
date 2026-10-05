//go:build verification

package desktop

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"
)

// verifyNativeUpdateFlow 验证后台检查、进度回调与真实窗口刷新，不替换应用安装包。
func verifyNativeUpdateFlow(win *mygo.Window, app *nativeApp) error {
	steps := make(chan int64)
	defer close(steps)
	var original *Updates
	mygo.RunOnMain(func() {
		original = app.updates
		app.updates = &Updates{
			versionFn: func() string { return "0.5.1" },
			enabledFn: func() bool { return true },
			checkFn: func(context.Context) (*mygo.Update, error) {
				return &mygo.Update{Version: "9.9.9", Notes: "验收用更新日志。\n\n## 问题修复\n\n- 验证后台下载进度。\n"}, nil
			},
			installFn: func(ctx context.Context, _ *mygo.Update, progress func(int64, int64)) error {
				for {
					select {
					case downloaded, ok := <-steps:
						if !ok {
							return fmt.Errorf("验收结束")
						}
						progress(downloaded, 100)
						if downloaded == 100 {
							return nil
						}
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			},
		}
		app.watchUpdates()
		app.checkUpdate()
	})
	defer mygo.RunOnMain(func() {
		app.updates = original
		app.updateOpen, app.updateDownloading, app.updateBusy = false, false, false
		app.updateDownloaded, app.updateTotal, app.updateError = 0, 0, ""
	})
	wait := func(test func() bool) error {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			ok := false
			mygo.RunOnMain(func() { ok = test() })
			if ok {
				return nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return fmt.Errorf("更新界面状态等待超时")
	}
	if err := wait(func() bool { return app.updateOpen && !app.updateBusy }); err != nil {
		return fmt.Errorf("手动检查未直接弹窗：%w", err)
	}
	mygo.RunOnMain(app.installUpdate)
	for _, downloaded := range []int64{25, 75, 100} {
		select {
		case steps <- downloaded:
		case <-time.After(3 * time.Second):
			return fmt.Errorf("安装未接收进度")
		}
		if err := wait(func() bool {
			if app.updateDownloaded != downloaded || app.updateTotal != 100 {
				return false
			}
			if downloaded == 100 {
				return app.updates.Status().Installed && !app.updateDownloading
			}
			return app.updateDownloading
		}); err != nil {
			return fmt.Errorf("进度 %d%% 未刷新：%w", downloaded, err)
		}
		refreshVerificationWindow(win)
		time.Sleep(150 * time.Millisecond)
		shot, err := win.CapturePage()
		if err != nil {
			return err
		}
		if err := os.WriteFile(fmt.Sprintf("verification/native-update-progress-%d.png", downloaded), shot, 0644); err != nil {
			return err
		}
	}
	return nil
}
