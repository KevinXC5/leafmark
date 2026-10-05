package desktop

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// Updates 将版本检查与签名安装交给 MyGo，界面只接收明确的更新状态。
type Updates struct {
	mu        sync.Mutex
	pending   *mygo.Update
	busy      bool
	installed bool

	// 依赖仅在实例创建时替换；未设置时调用 MyGo，避免测试修改全局状态。
	versionFn func() string
	enabledFn func() bool
	checkFn   func(context.Context) (*mygo.Update, error)
	installFn func(context.Context, *mygo.Update, func(int64, int64)) error
	restartFn func()

	// 界面在启动时接上这两个回调：自动检查发现新版本，以及下载的真实字节数。
	// 它们在后台协程里被调用。
	availableFn func(UpdateStatus)
	progressFn  func(downloaded, total int64)
}

func (u *Updates) version() string {
	if u.versionFn != nil {
		return u.versionFn()
	}
	return mygo.App.Version()
}

func (u *Updates) enabled() bool {
	if u.enabledFn != nil {
		return u.enabledFn()
	}
	return mygo.Updater.Enabled()
}

func (u *Updates) check(ctx context.Context) (*mygo.Update, error) {
	if u.checkFn != nil {
		return u.checkFn(ctx)
	}
	return mygo.Updater.Check(ctx)
}

func (u *Updates) install(ctx context.Context, up *mygo.Update) error {
	if u.installFn != nil {
		return u.installFn(ctx, up, u.progress)
	}
	return up.Install(ctx, u.progress)
}

func (u *Updates) progress(downloaded, total int64) {
	if u.progressFn != nil {
		u.progressFn(downloaded, total)
	}
}

// Restart 仅允许安装成功后重启；桌面入口负责先完成文档关闭检查。
func (u *Updates) Restart() error {
	u.mu.Lock()
	installed := u.installed
	u.mu.Unlock()
	if !installed {
		return errors.New("请先完成更新安装")
	}
	if u.restartFn == nil {
		return errors.New("当前窗口无法重启，请关闭应用后重新打开")
	}
	u.restartFn()
	return nil
}

type UpdateStatus struct {
	Version   string `json:"version"`
	Enabled   bool   `json:"enabled"`
	Available string `json:"available"`
	Notes     string `json:"notes"`
	Installed bool   `json:"installed"`
}

func (u *Updates) Status() UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	status := UpdateStatus{Version: u.version(), Enabled: u.enabled(), Installed: u.installed}
	if u.pending != nil {
		status.Available = u.pending.Version
		status.Notes = u.pending.Notes
	}
	return status
}

func (u *Updates) Check(ctx context.Context) (UpdateStatus, error) {
	if !u.enabled() {
		return UpdateStatus{}, errors.New("当前构建无法自动更新，请使用正式版并将应用安装到可写目录")
	}
	u.mu.Lock()
	if u.installed {
		u.mu.Unlock()
		return u.Status(), nil
	}
	if u.busy {
		u.mu.Unlock()
		return UpdateStatus{}, errors.New("正在检查或安装更新，请稍后再试")
	}
	u.busy = true
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	up, err := u.check(ctx)
	if err != nil {
		return UpdateStatus{}, errors.New("检查更新失败，请检查网络连接后重试")
	}
	u.mu.Lock()
	u.pending = up
	u.mu.Unlock()
	return u.Status(), nil
}

func (u *Updates) Install(ctx context.Context) (UpdateStatus, error) {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return UpdateStatus{}, errors.New("正在检查或安装更新，请稍后再试")
	}
	if u.installed || u.pending == nil {
		u.mu.Unlock()
		return UpdateStatus{}, errors.New("没有需要安装的更新")
	}
	up := u.pending
	u.busy = true
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }()
	// 更新不关闭窗口，不中断书写；替换成功后在下次启动使用新版本。
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if err := u.install(ctx, up); err != nil {
		return UpdateStatus{}, errors.New("安装更新失败，当前版本仍可继续使用，请稍后重试")
	}
	u.mu.Lock()
	u.installed = true
	u.mu.Unlock()
	return u.Status(), nil
}

func (u *Updates) start() {
	if !u.enabled() {
		return
	}
	go func() {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		<-timer.C
		for {
			status, err := u.Check(context.Background())
			if err == nil && status.Available != "" && !status.Installed && u.availableFn != nil {
				u.availableFn(status)
			}
			// 检查失败时保持安静，手动检查入口可以随时重试。
			timer.Reset(24 * time.Hour)
			<-timer.C
		}
	}()
}
