//go:build verification

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// 官网截图模式由环境变量 LEAFMARK_SITE_SHOT 选择：edit 截原位编辑，read 截阅读模式。
// 示例文档保存在 tests/fixtures/samples/，截图输出到 verification/site-shots/。
var siteShotSamples = map[string]string{"edit": "山中来信.md", "read": "叶脉笔记.md"}

func siteShotMode() string {
	mode := os.Getenv("LEAFMARK_SITE_SHOT")
	if mode == "" {
		return ""
	}
	if _, ok := siteShotSamples[mode]; !ok {
		panic("LEAFMARK_SITE_SHOT 只接受 edit 或 read：" + mode)
	}
	return mode
}

// 示例文档复制到验证工作区后再打开，应用的保存不会改动仓库里的原件。
func prepareSiteShot(root, mode string) (string, error) {
	name := siteShotSamples[mode]
	raw, err := os.ReadFile(filepath.Join(root, "tests", "fixtures", "samples", name))
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "verification", name)
	return path, os.WriteFile(path, raw, 0644)
}

func captureSiteShots(win *mygo.Window, eval func(string) (any, error), mode string) error {
	out := filepath.Join("verification", "site-shots")
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	win.SetContentSize(1280, 820)
	// 只保留示例文档一个标签页。
	if _, err := eval(`const tab=[...document.querySelectorAll('.file-tab')].find(t=>!t.classList.contains('active')); tab?.querySelector('.close-tab').click(); await new Promise(r=>setTimeout(r,400)); return true;`); err != nil {
		return err
	}
	time.Sleep(800 * time.Millisecond)
	for _, theme := range []string{"light", "dark"} {
		setTheme := `document.documentElement.dataset.theme='` + theme + `'; await new Promise(r=>setTimeout(r,200)); `
		settle := `await document.fonts.ready; document.activeElement?.blur?.(); await new Promise(r=>setTimeout(r,600)); if(innerWidth!==1280 || innerHeight!==820) throw Error('窗口内容不是 1280×820：'+innerWidth+'×'+innerHeight); `
		var err error
		if mode == "read" {
			_, err = eval(setTheme + `document.querySelector('#reading-toggle').click(); for(let i=0;i<100 && !document.querySelector('#reading-view figure.mermaid-diagram svg');i++) await new Promise(r=>setTimeout(r,100)); ` + settle + `if(!document.querySelector('#reading-view figure.mermaid-diagram svg')) throw Error('图形未渲染'); return true;`)
		} else {
			_, err = eval(setTheme + `const view=window.leafmarkVerification.editor; view.contentDOM.blur(); view.scrollDOM.scrollTop=0; ` + settle + `if(!document.querySelector('.lm-block-preview') || !document.querySelector('.lm-table-preview')) throw Error('原位预览未渲染'); return true;`)
		}
		if err != nil {
			return err
		}
		png, err := win.CapturePage()
		if err != nil {
			return err
		}
		file := filepath.Join(out, "app-"+mode+"-"+theme+".png")
		if err := os.WriteFile(file, png, 0644); err != nil {
			return err
		}
		fmt.Println("已截图：", file)
		if mode == "read" {
			// 退出阅读模式，下一主题重新进入时图形按新配色渲染。
			if _, err := eval(`document.querySelector('#reading-toggle').click(); await new Promise(r=>setTimeout(r,300)); return true;`); err != nil {
				return err
			}
		}
	}
	return nil
}
