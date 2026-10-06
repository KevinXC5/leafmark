package desktop

import "strconv"

// editingCommand 集中标记会改变正文的动作，供阅读模式拦截。
func editingCommand(name string) bool {
	switch name {
	case "undo", "redo", "bold", "italic", "strike", "code", "heading", "heading1", "heading2", "heading3", "paragraph", "quote", "list", "bullet", "ordered", "task", "table", "codeblock", "hr", "link", "image":
		return true
	}
	return false
}

func (a *nativeApp) runReplace(all bool) {
	if a.reading {
		a.findNote = "阅读模式下不能替换"
		return
	}
	tab := a.active()
	if tab == nil || tab.editor == nil || a.findQuery == "" {
		return
	}
	editor, ok := tab.editor.(interface {
		Replace(query, replacement string, all bool) int
	})
	if !ok {
		a.findNote = "当前编辑器不支持替换"
		return
	}
	count := editor.Replace(a.findQuery, a.replaceText, all)
	if count == 0 {
		a.findNote = "未找到可替换的内容"
		return
	}
	a.findNote = "已替换 " + strconv.Itoa(count) + " 处"
	a.syncEditor(tab)
	a.refreshTitle()
}
