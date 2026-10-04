package desktop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func newTestUpdates() *Updates {
	return &Updates{
		versionFn: func() string { return "1.0.0" },
		enabledFn: func() bool { return true },
		checkFn: func(context.Context) (*mygo.Update, error) {
			return nil, errors.New("测试未配置检查依赖")
		},
		installFn: func(context.Context, *mygo.Update, func(int64, int64)) error {
			return errors.New("测试未配置安装依赖")
		},
	}
}

func requireUpdateStatus(t *testing.T, got, want UpdateStatus) {
	t.Helper()
	if got != want {
		t.Fatalf("更新状态不符：得到 %+v，期望 %+v", got, want)
	}
}

func requireUpdateError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("错误提示不符：得到 %v，期望 %q", err, want)
	}
}

func TestUpdatesRejectConcurrentOperations(t *testing.T) {
	for _, active := range []string{"check", "install"} {
		for _, rejected := range []string{"check", "install"} {
			t.Run(active+"/"+rejected, func(t *testing.T) {
				u := newTestUpdates()
				up := &mygo.Update{Version: "1.1.0", Notes: "更新说明"}
				u.pending = up
				started := make(chan struct{})
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				var checkCalls, installCalls atomic.Int32
				// 用通道保持首个操作执行中，确保并发拒绝测试不依赖调度时机。
				block := func(ctx context.Context) error {
					close(started)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				u.checkFn = func(ctx context.Context) (*mygo.Update, error) {
					if checkCalls.Add(1) > 1 || active != "check" {
						return nil, errors.New("并发检查进入依赖")
					}
					return up, block(ctx)
				}
				u.installFn = func(ctx context.Context, got *mygo.Update, _ func(int64, int64)) error {
					if installCalls.Add(1) > 1 || active != "install" {
						return errors.New("并发安装进入依赖")
					}
					if got != up {
						return errors.New("安装未使用待安装更新")
					}
					return block(ctx)
				}
				type result struct {
					status UpdateStatus
					err    error
				}
				done := make(chan result, 1)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				t.Cleanup(cancel)
				t.Cleanup(unblock)
				go func() {
					var status UpdateStatus
					var err error
					if active == "check" {
						status, err = u.Check(ctx)
					} else {
						status, err = u.Install(ctx)
					}
					done <- result{status, err}
				}()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("首个操作未进入依赖")
				}
				var status UpdateStatus
				var err error
				if rejected == "check" {
					status, err = u.Check(ctx)
				} else {
					status, err = u.Install(ctx)
				}
				requireUpdateError(t, err, "正在检查或安装更新，请稍后再试")
				requireUpdateStatus(t, status, UpdateStatus{})
				want := UpdateStatus{Version: "1.0.0", Enabled: true, Available: up.Version, Notes: up.Notes}
				requireUpdateStatus(t, u.Status(), want)
				unblock()
				select {
				case got := <-done:
					if got.err != nil {
						t.Fatalf("首个操作失败：%v", got.err)
					}
					want.Installed = active == "install"
					requireUpdateStatus(t, got.status, want)
				case <-ctx.Done():
					t.Fatal("首个操作未完成")
				}
				wantCheck, wantInstall := int32(1), int32(0)
				if active == "install" {
					wantCheck, wantInstall = 0, 1
				}
				if checkCalls.Load() != wantCheck || installCalls.Load() != wantInstall {
					t.Fatalf("被拒绝操作调用了依赖：检查 %d 次，安装 %d 次", checkCalls.Load(), installCalls.Load())
				}
			})
		}
	}
}

func TestUpdatesCheckFailurePreservesPendingAndAllowsRetry(t *testing.T) {
	u := newTestUpdates()
	previous := &mygo.Update{Version: "1.1.0", Notes: "先前更新"}
	u.pending = previous
	next := &mygo.Update{Version: "1.2.0", Notes: "最新更新"}
	calls := 0
	u.checkFn = func(context.Context) (*mygo.Update, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("网络故障")
		}
		return next, nil
	}
	status, err := u.Check(context.Background())
	requireUpdateError(t, err, "检查更新失败，请检查网络连接后重试")
	requireUpdateStatus(t, status, UpdateStatus{})
	requireUpdateStatus(t, u.Status(), UpdateStatus{Version: "1.0.0", Enabled: true, Available: previous.Version, Notes: previous.Notes})
	if u.pending != previous {
		t.Fatal("检查失败丢失了待安装更新")
	}
	status, err = u.Check(context.Background())
	if err != nil {
		t.Fatalf("检查重试失败：%v", err)
	}
	requireUpdateStatus(t, status, UpdateStatus{Version: "1.0.0", Enabled: true, Available: next.Version, Notes: next.Notes})
	if calls != 2 || u.pending != next {
		t.Fatal("检查重试未更新待安装更新")
	}
}

func TestUpdatesInstallFailurePreservesPendingAndAllowsRetry(t *testing.T) {
	u := newTestUpdates()
	up := &mygo.Update{Version: "1.1.0", Notes: "更新说明"}
	u.checkFn = func(context.Context) (*mygo.Update, error) { return up, nil }
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatalf("准备更新失败：%v", err)
	}
	calls := 0
	u.installFn = func(_ context.Context, got *mygo.Update, _ func(int64, int64)) error {
		calls++
		if got != up {
			t.Fatal("安装重试未使用原待安装更新")
		}
		if calls == 1 {
			return errors.New("安装故障")
		}
		return nil
	}
	status, err := u.Install(context.Background())
	requireUpdateError(t, err, "安装更新失败，当前版本仍可继续使用，请稍后重试")
	requireUpdateStatus(t, status, UpdateStatus{})
	want := UpdateStatus{Version: "1.0.0", Enabled: true, Available: up.Version, Notes: up.Notes}
	requireUpdateStatus(t, u.Status(), want)
	status, err = u.Install(context.Background())
	if err != nil {
		t.Fatalf("安装重试失败：%v", err)
	}
	want.Installed = true
	requireUpdateStatus(t, status, want)
	requireUpdateStatus(t, u.Status(), want)
	if calls != 2 {
		t.Fatalf("安装依赖调用次数不符：%d", calls)
	}
}

func TestUpdatesInstalledPreventsRepeatedInstallAndCheck(t *testing.T) {
	u := newTestUpdates()
	up := &mygo.Update{Version: "1.1.0", Notes: "更新说明"}
	checkCalls, installCalls := 0, 0
	u.checkFn = func(context.Context) (*mygo.Update, error) {
		checkCalls++
		return up, nil
	}
	u.installFn = func(context.Context, *mygo.Update, func(int64, int64)) error {
		installCalls++
		return nil
	}
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatalf("检查失败：%v", err)
	}
	status, err := u.Install(context.Background())
	if err != nil {
		t.Fatalf("安装失败：%v", err)
	}
	want := UpdateStatus{Version: "1.0.0", Enabled: true, Available: up.Version, Notes: up.Notes, Installed: true}
	requireUpdateStatus(t, status, want)
	status, err = u.Install(context.Background())
	requireUpdateError(t, err, "没有需要安装的更新")
	requireUpdateStatus(t, status, UpdateStatus{})
	status, err = u.Check(context.Background())
	if err != nil {
		t.Fatalf("安装后检查未返回已有状态：%v", err)
	}
	requireUpdateStatus(t, status, want)
	requireUpdateStatus(t, u.Status(), want)
	if checkCalls != 1 || installCalls != 1 {
		t.Fatalf("安装后重复调用依赖：检查 %d 次，安装 %d 次", checkCalls, installCalls)
	}
}

func TestUpdatesRestartRequiresInstalledUpdate(t *testing.T) {
	u := newTestUpdates()
	calls := 0
	u.restartFn = func() { calls++ }
	requireUpdateError(t, u.Restart(), "请先完成更新安装")
	if calls != 0 {
		t.Fatal("安装完成前调用了重启")
	}
	u.installed = true
	if err := u.Restart(); err != nil || calls != 1 {
		t.Fatalf("安装后未调用安全重启入口：%v，调用 %d 次", err, calls)
	}
}

func TestUpdatesInstallReceivesProgressCallback(t *testing.T) {
	u := newTestUpdates()
	u.pending = &mygo.Update{Version: "1.1.0"}
	u.installFn = func(_ context.Context, _ *mygo.Update, progress func(int64, int64)) error {
		if progress == nil {
			t.Fatal("安装未接入进度回调")
		}
		return nil
	}
	if _, err := u.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUpdatesDisabled(t *testing.T) {
	u := newTestUpdates()
	u.enabledFn = func() bool { return false }
	u.checkFn = func(context.Context) (*mygo.Update, error) {
		t.Fatal("禁用状态不应调用检查依赖")
		return nil, nil
	}
	requireUpdateStatus(t, u.Status(), UpdateStatus{Version: "1.0.0"})
	status, err := u.Check(context.Background())
	requireUpdateError(t, err, "当前构建无法自动更新，请使用正式版并将应用安装到可写目录")
	requireUpdateStatus(t, status, UpdateStatus{})
}
