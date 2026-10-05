package desktop

import (
	"embed"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// 复用原界面的字体资源，转换为系统文字引擎接受的 TrueType 格式。
//
//go:embed fonts/*.ttf fonts/*LICENSE.txt
var nativeFonts embed.FS

func registerNativeFonts() error {
	entries, err := nativeFonts.ReadDir("fonts")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".ttf") {
			continue
		}
		family := "Inter"
		if strings.HasPrefix(entry.Name(), "newsreader-") {
			family = "Newsreader"
		}
		if strings.HasPrefix(entry.Name(), "geist-mono-") {
			family = "Geist Mono"
		}
		data, err := nativeFonts.ReadFile("fonts/" + entry.Name())
		if err != nil {
			return err
		}
		if err = ui.RegisterFont(data, family); err != nil {
			return fmt.Errorf("注册字体 %s：%w", entry.Name(), err)
		}
	}
	return nil
}
