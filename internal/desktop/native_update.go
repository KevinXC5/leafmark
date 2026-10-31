package desktop

import (
	"context"
)

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
