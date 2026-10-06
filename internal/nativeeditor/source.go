package nativeeditor

import (
	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

// SourceSpan 是源码模式的一段着色区间，坐标是全文的 rune 半开区间。
// Kind 取 0 正文、1 标题、2 强调、3 链接、4 代码、5 公式，与 SetSourcePalette 的下标对齐。
type SourceSpan struct {
	Start, End int
	Kind       uint8
}

// NewPlainText 用一整块等宽纯文本创建编辑器。正文保留全部标记字符，
// Markdown 返回文档纯文本，包括末尾换行和正文里的围栏。
func NewPlainText(text string) *Editor {
	e := New("")
	e.sourceMode = true
	e.doc = richtext.NewPlainText(text)
	return e
}

// SetSourceMode 开关源码模式。打开时把当前 Markdown 收成一块可编辑的等宽纯文本，
// 标记字符留在正文里，不画代码块底色，也不额外增减首尾换行。
// 关闭时按当前纯文本重新解析成所见即所得文档。
func (e *Editor) SetSourceMode(on bool) {
	if e.sourceMode == on {
		return
	}
	e.cancelCompose()
	if on {
		text := e.doc.Markdown()
		e.doc = richtext.NewPlainText(text)
	} else {
		e.doc = richtext.Parse(e.doc.Text())
	}
	e.sourceMode = on
	e.anchor, e.focus = 0, 0
	e.pending = 0
}

// SetSourceSyntax 设置源码着色。回调只读全文，不得改文档；返回的区间按出现顺序覆盖字形。
func (e *Editor) SetSourceSyntax(fn func(text string) []SourceSpan) {
	e.sourceSyntax = fn
	e.sourceLaid = false
}

// SetSourcePalette 设置源码着色。下标与 SourceSpan.Kind 对齐，0 不使用，正文仍用主题文字色。
func (e *Editor) SetSourcePalette(colors [6]ui.Color) {
	e.sourcePalette = colors
	e.sourceSet = true
}

// Scroll 返回内容滚动偏移。源码模式用它确认大纲跳转已经滚离文首。
// SetSelection 把 reveal 置上后，下一帧 View 才会改这个值。
func (e *Editor) Scroll() (x, y float32) { return e.scroll.X, e.scroll.Y }

// Bounds 返回最近一帧滚动元素在父容器中的矩形，含侧栏占掉的 X。右边界是 X+W。
func (e *Editor) Bounds() ui.Rect { return e.bounds }

// ScrollOffset 返回纵向滚动偏移，与 Scroll 的 y 相同。
func (e *Editor) ScrollOffset() float32 { return e.scroll.Y }

// ScrollPending 报告 SetSelection、查找或编辑之后，光标是否还等着滚进视野。
// View 处理完滚动后为 false。
func (e *Editor) ScrollPending() bool { return e.reveal }
