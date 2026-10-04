//go:build verification && !darwin

package desktop

import mygo "github.com/egoist/mygo"

// 原生点击只在 macOS 的验证构建中实现。
func suiteTap(*mygo.Window, float64, float64, int) {}
