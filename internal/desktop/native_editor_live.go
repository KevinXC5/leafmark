//go:build !desktoptest

package desktop

import "leafmark/internal/nativeeditor"

// 正式构建使用始终排版的原生编辑器。desktoptest 标签只供本包测试替换。
var newDocumentEditor = func(markdown string) documentEditor {
	return nativeeditor.New(markdown)
}
