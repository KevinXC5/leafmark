//go:build verification

package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// 完整验收模式由 scripts/verify-full.mjs 驱动：LEAFMARK_VERIFY_SUITE 指向步骤 JSON，
// LEAFMARK_VERIFY_OUT 是结果与截图的输出目录。步骤失败不中断，全部执行后以退出码汇总。
type suiteStep struct {
	Name string `json:"name"`
	// 在页面中执行的脚本，抛出异常即失败。
	JS string `json:"js"`
	// 截图文件名（不含扩展名）。
	Shot string `json:"shot"`
	// 原生“选择文件夹”对话框无法自动化，这里让后端直接切换到指定工作区。
	SelectFolder string `json:"selectFolder"`
	Shell        string `json:"shell"`
	// 执行后等待的毫秒数，用于页面重载这类没有返回值可等的操作。
	Sleep int `json:"sleep"`
	// 执行前激活窗口，依赖编辑器焦点的步骤使用。
	Focus bool `json:"focus"`
	// 返回 [x, y] 的页面脚本：向窗口在该点投递一次原生鼠标点击，Gap 为按下到抬起的毫秒数，
	// 0 表示两个事件连续入队，等同触控板“轻触点击”；负数只把指针移到该点，用于悬停。
	Tap string `json:"tap"`
	Gap int    `json:"gap"`
}

const suiteTimeout = 15 * time.Minute

func suiteScript() string { return os.Getenv("LEAFMARK_VERIFY_SUITE") }

func runSuite(win *mygo.Window, files *Files, script string) {
	fail := func(err error) {
		fmt.Fprintln(os.Stderr, "完整验收无法启动：", err)
		mygo.App.Exit(1)
	}
	raw, err := os.ReadFile(script)
	if err != nil {
		fail(err)
		return
	}
	var steps []suiteStep
	if err := json.Unmarshal(raw, &steps); err != nil {
		fail(err)
		return
	}
	out := os.Getenv("LEAFMARK_VERIFY_OUT")
	if out == "" {
		out = filepath.Dir(script)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		fail(err)
		return
	}
	win.SetContentSize(1280, 820)
	mygo.App.Focus()
	win.Focus()
	results := make([]map[string]any, 0, len(steps))
	failed := 0
	for _, step := range steps {
		entry := map[string]any{"name": step.Name, "ok": true}
		started := time.Now()
		problem := func(err error) { entry["ok"], entry["error"] = false, err.Error() }
		if step.Focus {
			mygo.App.Focus()
			win.Focus()
			time.Sleep(300 * time.Millisecond)
		}
		if step.SelectFolder != "" {
			if _, err := files.workspace.store.SelectFolder(step.SelectFolder); err != nil {
				problem(err)
			}
		}
		if step.Shell != "" {
			output, err := exec.Command("bash", "-c", step.Shell).CombinedOutput()
			entry["shell"] = string(output)
			if err != nil {
				problem(err)
			}
		}
		if step.Tap != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			result, err := win.Page().EvalContext(ctx, step.Tap)
			cancel()
			point, _ := result.([]any)
			if err != nil {
				problem(err)
			} else if len(point) != 2 {
				problem(fmt.Errorf("点击位置应为 [x, y]：%v", result))
			} else {
				x, _ := point[0].(float64)
				y, _ := point[1].(float64)
				suiteTap(win, x, y, step.Gap)
				entry["result"] = point
			}
		}
		if step.JS != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			result, err := win.Page().EvalContext(ctx, step.JS)
			cancel()
			if err != nil {
				problem(err)
			} else {
				entry["result"] = result
			}
		}
		if step.Sleep > 0 {
			time.Sleep(time.Duration(step.Sleep) * time.Millisecond)
		}
		if step.Shot != "" {
			time.Sleep(250 * time.Millisecond)
			png, err := win.CapturePage()
			if err == nil {
				err = os.WriteFile(filepath.Join(out, step.Shot+".png"), png, 0644)
			}
			if err != nil {
				problem(err)
			}
		}
		entry["ms"] = time.Since(started).Milliseconds()
		results = append(results, entry)
		if entry["ok"] == true {
			fmt.Println("通过", step.Name)
		} else {
			failed++
			fmt.Println("失败", step.Name, "：", entry["error"])
		}
	}
	encoded, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "results.json"), encoded, 0644); err != nil {
		fail(err)
		return
	}
	if failed > 0 {
		mygo.App.Exit(1)
		return
	}
	mygo.App.Exit(0)
}
