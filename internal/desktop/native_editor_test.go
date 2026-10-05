//go:build desktoptest

package desktop

// 本包测试不链接尚未完成的排版实现，只保留桌面装配需要的行为。
var newDocumentEditor = func(markdown string) documentEditor {
	return &stubEditor{text: markdown}
}
