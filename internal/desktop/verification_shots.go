//go:build verification

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

var siteShotSamples = map[string]string{"edit": "山中来信.md", "read": "叶脉笔记.md"}

func siteShotMode() string {
	mode := os.Getenv("LEAFMARK_SITE_SHOT")
	if mode != "" {
		if _, ok := siteShotSamples[mode]; !ok {
			panic("LEAFMARK_SITE_SHOT 只接受 edit 或 read：" + mode)
		}
	}
	return mode
}

// 示例文档复制到验证目录，截图过程不修改原件。
func prepareSiteShot(root, mode string) (string, error) {
	name := siteShotSamples[mode]
	raw, err := os.ReadFile(filepath.Join(root, "tests", "fixtures", "samples", name))
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "verification", name)
	return path, os.WriteFile(path, raw, 0600)
}

func captureNativeSiteShots(win *mygo.Window, app *nativeApp, mode string) error {
	out := filepath.Join("verification", "site-shots")
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		ready := false
		mygo.RunOnMain(func() { ready = app.active() != nil && app.active().editor != nil })
		if ready {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("官网截图初始化超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
	win.SetContentSize(1280, 820)
	for _, theme := range []string{"light", "dark"} {
		mygo.RunOnMain(func() {
			current := app.files.Current().ID
			for _, doc := range app.files.List() {
				if doc.ID != current {
					app.finishClose([]string{doc.ID})
				}
			}
			app.settings.Theme = theme
			app.settings.FontSize = 14
			app.settings.AutoSave = false
			app.sidebarMode = "outline"
			app.applySettings()
			if tab := app.active(); tab != nil {
				if editor, ok := tab.editor.(interface{ SetLineHeight(float32) }); ok {
					editor.SetLineHeight(1.5)
				}
			}
		})
		win.Invalidate()
		refreshVerificationWindow(win)
		time.Sleep(300 * time.Millisecond)
		refreshVerificationWindow(win)
		png, err := win.CapturePage()
		if err != nil {
			return err
		}
		path := filepath.Join(out, "app-"+mode+"-"+theme+".png")
		if err = os.WriteFile(path, png, 0644); err != nil {
			return err
		}
		fmt.Println("已截图：", path)
	}
	return nil
}
