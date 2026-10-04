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
	installFn func(context.Context, *mygo.Update) error
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
		return u.installFn(ctx, up)
	}
	return up.Install(ctx, nil)
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
			if err == nil && status.Available != "" && !status.Installed {
				res, err := mygo.Dialog.Message(mygo.MessageOptions{
					Type: mygo.MessageInfo, Message: "Leafmark " + status.Available + " 可供更新",
					Detail:  "当前版本：" + status.Version + "\n升级后下次启动生效，不会关闭当前文档。\n\n更新日志\n" + status.Notes,
					Buttons: []string{"确定升级", "取消"}, DefaultButton: 1, CancelButton: 1,
				})
				if err == nil && res.Button == 0 {
					if _, err := u.Install(context.Background()); err != nil {
						mygo.Dialog.Error("更新未完成", err.Error())
					}
				}
			}
			// 检查失败时保持安静，手动检查入口可以随时重试。
			timer.Reset(24 * time.Hour)
			<-timer.C
		}
	}()
}
