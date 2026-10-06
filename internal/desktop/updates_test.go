package desktop

import (
	"context"
	"errors"
	"fmt"
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

func TestUpdatesCancelAllowsRetry(t *testing.T) {
	u := newTestUpdates()
	u.pending = &mygo.Update{Version: "1.1.0"}
	started := make(chan struct{})
	u.installFn = func(ctx context.Context, _ *mygo.Update, progress func(int64, int64)) error {
		progress(12, 100)
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := u.Install(ctx); done <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("下载未开始")
	}
	cancel()
	select {
	case err := <-done:
		requireUpdateError(t, err, "更新已取消，可稍后重试")
	case <-time.After(3 * time.Second):
		t.Fatal("取消未终止下载")
	}
	if u.Status().Installed {
		t.Fatal("取消后不能标记为已安装")
	}
	u.installFn = func(context.Context, *mygo.Update, func(int64, int64)) error { return nil }
	if status, err := u.Install(context.Background()); err != nil || !status.Installed {
		t.Fatalf("取消后未能重试：%+v，%v", status, err)
	}
}

func TestUpdatesStalledDownloadTimesOut(t *testing.T) {
	for _, total := range []int64{0, 100} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			u := newTestUpdates()
			u.installFn = func(ctx context.Context, _ *mygo.Update, progress func(int64, int64)) error {
				progress(12, total)
				<-ctx.Done()
				return ctx.Err()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := u.installWithTimeout(ctx, &mygo.Update{}, 20*time.Millisecond)
			if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				t.Fatalf("无进展应触发下载超时：%v，父上下文：%v", err, ctx.Err())
			}
		})
	}
}

func TestUpdatesIdleTimeoutDoesNotInterruptInstallation(t *testing.T) {
	u := newTestUpdates()
	u.installFn = func(ctx context.Context, _ *mygo.Update, progress func(int64, int64)) error {
		progress(100, 100)
		select {
		case <-time.After(60 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := u.installWithTimeout(context.Background(), &mygo.Update{}, 20*time.Millisecond); err != nil {
		t.Fatalf("下载完成后的安装不应受下载空闲超时影响：%v", err)
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

func TestReleaseNotesSinceKeepsVersionsNewerThanCurrent(t *testing.T) {
	notes := "### 0.6.1\n\n修复查找。\n\n#### 问题修复\n\n- 查找定位。\n\n### 0.6.0\r\n\r\n补齐编辑能力。\n\n### 0.5.1\n\n旧版本说明。\n"
	for _, tc := range []struct{ name, notes, current, want string }{
		{"跨版本升级列出中间版本", notes, "0.5.1", "### 0.6.1\n\n修复查找。\n\n#### 问题修复\n\n- 查找定位。\n\n### 0.6.0\n\n补齐编辑能力。"},
		{"只差一个版本时不带版本标题", notes, "v0.6.0", "修复查找。\n\n#### 问题修复\n\n- 查找定位。"},
		{"当前版本不低于日志时保留最新一节", notes, "0.6.1", "修复查找。\n\n#### 问题修复\n\n- 查找定位。"},
		{"开发版看到全部版本", "### 0.6.1\n\n甲。\n\n### 0.6.0\n\n乙。\n", "", "### 0.6.1\n\n甲。\n\n### 0.6.0\n\n乙。"},
		{"未分节的日志原样返回", "概述一句。\n\n### 问题修复\n\n- 一条。\n", "0.5.1", "概述一句。\n\n### 问题修复\n\n- 一条。\n"},
		{"概述在版本标题之前时原样返回", "概述。\n\n### 0.6.1\n\n甲。", "0.5.1", "概述。\n\n### 0.6.1\n\n甲。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseNotesSince(tc.notes, tc.current); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
