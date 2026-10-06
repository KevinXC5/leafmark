package desktop

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

func (u *Updates) install(ctx context.Context, up *mygo.Update, progress func(int64, int64)) error {
	if u.installFn != nil {
		return u.installFn(ctx, up, progress)
	}
	return up.Install(ctx, progress)
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
		status.Notes = releaseNotesSince(u.pending.Notes, status.Version)
	}
	return status
}

// parseVersion 解析 "0.6.1" 或 "v0.6.1" 这样的三段版本号。
func parseVersion(s string) (v [3]int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != len(v) {
		return v, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// releaseNotesSince 从按 "### 版本号" 分节的更新日志里取出比 current 新的版本，
// 跨版本升级时能看到中间每个版本的变化。日志没有分节或 current 不是正式版本号时原样返回；
// 只剩一个版本时去掉版本标题，窗口标题已经写明了版本。
func releaseNotesSince(notes, current string) string {
	type section struct {
		version [3]int
		lines   []string
	}
	var sections []section
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		if rest, found := strings.CutPrefix(line, "### "); found {
			if v, ok := parseVersion(rest); ok {
				sections = append(sections, section{version: v})
			}
		}
		if len(sections) == 0 {
			// 版本标题之前的内容不属于任何版本，说明日志没有按版本分节。
			if strings.TrimSpace(line) != "" {
				return notes
			}
			continue
		}
		last := &sections[len(sections)-1]
		last.lines = append(last.lines, line)
	}
	if len(sections) == 0 {
		return notes
	}
	kept := sections
	if base, ok := parseVersion(current); ok {
		kept = nil
		for _, s := range sections {
			if slices.Compare(s.version[:], base[:]) > 0 {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			kept = sections[:1]
		}
	}
	if len(kept) == 1 {
		return strings.TrimSpace(strings.Join(kept[0].lines[1:], "\n"))
	}
	var b strings.Builder
	for _, s := range kept {
		b.WriteString(strings.TrimSpace(strings.Join(s.lines, "\n")))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
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
	if err := u.installWithTimeout(ctx, up, time.Minute); err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return UpdateStatus{}, errors.New("更新已取消，可稍后重试")
		case errors.Is(err, context.DeadlineExceeded):
			return UpdateStatus{}, errors.New("更新下载或安装超时，请检查网络连接后重试")
		default:
			return UpdateStatus{}, errors.New("安装更新失败，当前版本仍可继续使用，请稍后重试")
		}
	}
	u.mu.Lock()
	u.installed = true
	u.mu.Unlock()
	return u.Status(), nil
}

// installWithTimeout 在连接或下载长时间无进展时取消请求；总大小未知也按实际字节判断。
func (u *Updates) installWithTimeout(ctx context.Context, up *mygo.Update, idle time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var timedOut atomic.Bool
	var mu sync.Mutex
	var downloaded, total int64
	finished, complete := false, false
	lastProgress := time.Now()
	var timer *time.Timer
	timer = time.AfterFunc(idle, func() {
		mu.Lock()
		defer mu.Unlock()
		if finished || complete {
			return
		}
		if remaining := idle - time.Since(lastProgress); remaining > 0 {
			timer.Reset(remaining)
			return
		}
		timedOut.Store(true)
		cancel()
	})
	defer func() {
		mu.Lock()
		finished = true
		timer.Stop()
		mu.Unlock()
	}()
	err := u.install(ctx, up, func(n, size int64) {
		mu.Lock()
		if !finished && (n > downloaded || size != total) {
			downloaded, total = n, size
			lastProgress = time.Now()
			complete = size > 0 && n >= size
			// 下载完成后留给签名校验和替换安装包的时间由总超时控制。
			if complete {
				timer.Stop()
			} else {
				timer.Reset(idle)
			}
		}
		mu.Unlock()
		u.progress(n, size)
	})
	if err != nil {
		if timedOut.Load() {
			return context.DeadlineExceeded
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
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
