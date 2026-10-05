package desktop

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
	"leafmark/internal/workspace"
)

// sourceEditor 是源码模式的编辑器：一个等宽的纯文本域，直接编辑 Markdown。
type sourceEditor struct {
	text    string
	size    float32
	changed bool
}

const sourceFontFamily = `"Geist Mono", "SFMono-Regular", "Consolas", monospace`

func (s *sourceEditor) View(c *ui.Context) {
	size := s.size
	if size <= 0 {
		size = 14
	}
	ui.Scroll(c).Fill().MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Padding(36, 40, 80, 40).Children(func() {
			area := ui.TextAreaBase(c, &s.text).Grow(1).MaxWidth(856).MinHeight(240).Font(sourceFontFamily).FontSize(size - 1).LineHeight(1.7).Label("Markdown 源码")
			if area.Changed() {
				s.changed = true
			}
		})
	})
}
func (s *sourceEditor) Markdown() string { return s.text }
func (s *sourceEditor) Text() string     { return s.text }
func (s *sourceEditor) Changed() bool {
	changed := s.changed
	s.changed = false
	return changed
}

// 撤销、重做由文本域自己处理；排版命令在源码模式下不生效。
func (s *sourceEditor) Undo()                                       {}
func (s *sourceEditor) Redo()                                       {}
func (s *sourceEditor) Format(string)                               {}
func (s *sourceEditor) SetReadImage(func(string) *ui.Bitmap)        {}
func (s *sourceEditor) Selection() (int, int)                       { return 0, 0 }
func (s *sourceEditor) SetSelection(int, int)                       {}
func (s *sourceEditor) HandleInput(*ui.Context, ui.InputEvent) bool { return false }
func (s *sourceEditor) FontSize(size float32)                       { s.size = size }
func (s *sourceEditor) Find(query string) bool                      { return query != "" && strings.Contains(s.text, query) }
func (s *sourceEditor) InsertLink(label, url string)                { s.append("[" + label + "](" + url + ")") }
func (s *sourceEditor) InsertImage(alt, markdownPath string) {
	s.append("![" + alt + "](" + markdownPath + ")")
}
func (s *sourceEditor) append(markdown string) {
	if s.text != "" && !strings.HasSuffix(s.text, "\n") {
		s.text += "\n"
	}
	s.text += markdown + "\n"
	s.changed = true
}

// sourceMode 报告标签是否处于源码模式。
func sourceMode(tab *nativeTab) bool {
	if tab == nil {
		return false
	}
	_, ok := tab.editor.(*sourceEditor)
	return ok
}

// toggleSource 在所见即所得与源码之间切换当前标签。正文不变，撤销历史从切换后重新开始。
func (a *nativeApp) toggleSource() {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	markdown := tab.editor.Markdown()
	if sourceMode(tab) {
		fresh := newDocumentEditor(markdown)
		a.configureEditor(fresh)
		a.connectEditor(fresh)
		if a.assets != nil {
			fresh.SetReadImage(imageReader(a.assets, tab.id))
		}
		tab.editor = fresh
		return
	}
	tab.editor = &sourceEditor{text: markdown, size: a.settings.FontSize}
}

// ask 打开一个单行输入框；ok 在主线程收到去掉首尾空白后的非空输入。
func (a *nativeApp) ask(title, hint, initial string, ok func(string)) {
	a.promptKind, a.promptTitle, a.promptHint = "text", title, hint
	a.promptText = initial
	a.promptOpen = true
	a.promptOK = func(raw string) {
		if raw = strings.TrimSpace(raw); raw != "" {
			ok(raw)
		}
	}
}

// askWorkspacePath 在工作区里新建文档或文件夹，parent 是相对工作区根的上级目录。
func (a *nativeApp) askWorkspacePath(kind, parent string) {
	if a.workspace == nil || a.folderName == "" {
		a.notice = "请先打开一个文件夹作为工作区"
		return
	}
	initial := ""
	if parent != "" {
		initial = parent + "/"
	}
	title, hint := "新建 Markdown 文件", "相对于工作区根目录。文件名须使用 .md 或 .markdown 扩展名。"
	if kind == "folder" {
		title, hint = "新建文件夹", "相对于工作区根目录；上级文件夹须已存在。"
	}
	a.ask(title, hint, initial, func(relative string) {
		if kind == "file" && path.Ext(relative) == "" {
			relative += ".md"
		}
		go func() {
			var err error
			if kind == "folder" {
				err = a.workspace.CreateFolder(relative)
			} else {
				err = a.workspace.CreateFile(relative)
			}
			a.update(func() {
				if err != nil {
					a.notice = err.Error()
					return
				}
				if dir := path.Dir(relative); dir != "." {
					a.openDirs[dir] = true
				}
				a.reloadNavigation()
				if kind == "file" {
					a.openWorkspace(relative)
				}
			})
		}()
	})
}

// workspaceRoot 返回当前工作区的绝对路径，没有工作区时为空。
func (a *nativeApp) workspaceRoot() string {
	if a.workspace == nil {
		return ""
	}
	state, err := a.workspace.Current()
	if err != nil || state.Workspace == nil {
		return ""
	}
	return state.Workspace.Path
}

// askRename 只改名称。已打开的文档及其所在文件夹须先关闭相关标签，避免标签仍指向旧路径。
func (a *nativeApp) askRename(node workspace.Node) {
	root := a.workspaceRoot()
	if root == "" {
		return
	}
	target := filepath.Join(root, filepath.FromSlash(node.Path))
	for _, doc := range a.files.List() {
		if doc.Path == "" {
			continue
		}
		if doc.Path == target || (node.Directory && strings.HasPrefix(doc.Path, target+string(filepath.Separator))) {
			a.notice = "请先关闭相关标签再重命名"
			return
		}
	}
	a.ask("重命名", "仅修改名称，不能包含路径分隔符。", node.Name, func(name string) {
		if strings.ContainsAny(name, `/\`) {
			a.notice = "名称不能包含路径分隔符"
			return
		}
		next := name
		if dir := path.Dir(node.Path); dir != "." {
			next = dir + "/" + name
		}
		if next == node.Path {
			return
		}
		go func() {
			err := a.workspace.Rename(node.Path, next)
			a.update(func() {
				if err != nil {
					a.notice = err.Error()
					return
				}
				if a.openDirs[node.Path] {
					delete(a.openDirs, node.Path)
					a.openDirs[next] = true
				}
				a.reloadNavigation()
			})
		}()
	})
}

// treeMenu 是工作区条目的右键菜单。
func (a *nativeApp) treeMenu(node workspace.Node) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if node.Directory {
			if m.Item("新建文档…").Chosen() {
				a.askWorkspacePath("file", node.Path)
			}
			if m.Item("新建文件夹…").Chosen() {
				a.askWorkspacePath("folder", node.Path)
			}
			m.Separator()
		} else if m.Item("打开").Chosen() {
			a.openWorkspace(node.Path)
		}
		if m.Item("重命名…").Chosen() {
			a.askRename(node)
		}
		if m.Item("在文件夹中显示").Chosen() {
			if root := a.workspaceRoot(); root != "" {
				mygo.Shell.ShowItemInFolder(filepath.Join(root, filepath.FromSlash(node.Path)))
			}
		}
	}
}

const externalInterval = 2 * time.Second

// watchExternal 每两秒核对一次当前文档的磁盘内容：没有本地修改时直接换成磁盘上的新内容，
// 有未保存修改时暂停自动保存并提示，由用户决定是否重新加载。
func (a *nativeApp) watchExternal(now time.Time) {
	if a.externalBusy || now.Sub(a.externalAt) < externalInterval {
		return
	}
	a.externalAt = now
	tab := a.active()
	if tab == nil || tab.editor == nil || a.saving[tab.id] || a.conflicts[tab.id] {
		return
	}
	doc, ok := a.document(tab.id)
	if !ok || doc.Path == "" {
		return
	}
	id := tab.id
	a.externalBusy = true
	go func() {
		state, err := a.files.CheckExternal(id)
		a.update(func() {
			a.externalBusy = false
			if err != nil || !state.Changed || state.Missing || a.saving[id] {
				return
			}
			tab := a.tabByID(id)
			if tab == nil {
				return
			}
			doc, _ := a.document(id)
			if doc.Dirty || a.pendingEdit(tab) {
				a.conflicts[id] = true
				a.notice = "文件已在外部修改，可从“⋯”菜单重新加载"
				return
			}
			a.reloadDocument(id, false)
		})
	}()
}

// exportBody 生成当前文档的 HTML 正文，本地图片内嵌为数据地址，导出的文件可以单独打开。
func (a *nativeApp) exportBody() (name, body string, ok bool) {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return "", "", false
	}
	doc, _ := a.document(tab.id)
	name = doc.Name
	if name == "" {
		name = "未命名.md"
	}
	id := tab.id
	body = richtext.Parse(tab.editor.Markdown()).HTMLWith(func(url string) string {
		lower := strings.ToLower(strings.TrimSpace(url))
		if a.assets == nil || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return ""
		}
		uri, err := a.assets.ReadImage(id, url)
		if err != nil {
			return ""
		}
		return uri
	})
	return name, body, true
}

// exportStyle 是导出文档的版式：纸面配色与应用一致，打印时去掉背景色块以外的装饰。
const exportStyle = `
:root { color-scheme: light; }
* { box-sizing: border-box; }
body { margin: 0; background: #faf9f5; color: #47423c; font: 16px/1.6 "Newsreader", "Songti SC", "STSong", "Noto Serif CJK SC", "SimSun", Georgia, serif; }
main { max-width: 856px; margin: 0 auto; padding: 48px 40px 80px; }
h1, h2, h3, h4, h5, h6 { color: #302e2b; font-weight: 500; line-height: 1.35; margin: 1.4em 0 .6em; }
h1 { font-size: 1.89em; margin-top: 0; } h2 { font-size: 1.28em; } h3 { font-size: 1.06em; }
p, ul, ol, blockquote, pre, table, .callout { margin: 0 0 1.3em; }
a { color: #bd7858; }
ul, ol { padding-left: 1.4em; } li > ul, li > ol { margin-bottom: 0; }
li.task { list-style: none; margin-left: -1.4em; }
blockquote { margin-left: 0; padding: 10px 16px; border-left: 3px solid #bd7858; border-radius: 4px; background: #f1eee6; color: #797168; }
blockquote p { margin: 0; }
.callout { padding: 12px 16px; border-radius: 5px; background: #f3ece5; font: 13px/1.5 "Inter", system-ui, sans-serif; }
.callout p { margin: 0; } .callout-title { font-weight: 600; }
code { font: .82em "Geist Mono", "SFMono-Regular", Consolas, monospace; background: #f1eee6; border-radius: 3px; padding: 1px 4px; }
pre { padding: 14px 18px; border: 1px solid #e8e1d7; border-radius: 10px; background: #f1eee6; overflow-x: auto; }
pre code { background: none; padding: 0; font-size: 14px; line-height: 1.75; }
mark { background: #f6e3b4; color: inherit; border-radius: 2px; }
.math { font-style: italic; }
table { width: 100%; border-collapse: separate; border-spacing: 0; border: 1px solid #e8e1d7; border-radius: 10px; overflow: hidden; }
th, td { padding: 7px 11px; border-bottom: 1px solid #e8e1d7; border-right: 1px solid #e8e1d7; text-align: left; }
th { background: #f1eee6; font-weight: 600; } tr > :last-child { border-right: 0; } tbody tr:last-child > * { border-bottom: 0; }
hr { border: 0; border-top: 1px solid #e8e1d7; margin: 2em 0; }
img { max-width: 100%; }
@media print { body { background: #fff; } main { max-width: none; padding: 0; } pre, table, blockquote, .callout, img { break-inside: avoid; } h1, h2, h3 { break-after: avoid; } }
`

// exportDocument 把正文包成完整的 HTML 文档。
func exportDocument(name, body string) string {
	title := strings.TrimSuffix(name, filepath.Ext(name))
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(title)
	return "<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>" + escaped + "</title>\n<style>" + exportStyle + "</style>\n</head>\n<body>\n<main>\n" + body + "</main>\n</body>\n</html>\n"
}

// exportName 把文档名换成导出用的文件名建议。
func exportName(name, ext string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" {
		name = "未命名"
	}
	return strings.TrimSuffix(name, filepath.Ext(name)) + ext
}

func (a *nativeApp) exportHTML() {
	name, body, ok := a.exportBody()
	if !ok {
		return
	}
	html := exportDocument(name, body)
	win := a.window()
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "导出 HTML", DefaultPath: exportName(name, ".html"),
			Filters:           []mygo.FileFilter{{Name: "HTML 文档", Extensions: []string{"html", "htm"}}},
			CreateDirectories: true,
		})
		if err == nil && target != "" {
			err = workspace.WriteHTML(target, html)
		}
		a.exportDone(target, err)
	}()
}

func (a *nativeApp) exportMarkdown() {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	doc, _ := a.document(tab.id)
	id, content := tab.id, tab.editor.Markdown()
	win := a.window()
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "导出 Markdown 副本", DefaultPath: exportName(doc.Name, ".md"),
			Filters:           []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
			CreateDirectories: true,
		})
		if err == nil && target != "" {
			err = a.files.store.ExportCopy(id, content, target)
		}
		a.exportDone(target, err)
	}()
}

// exportPDF 用系统自带的网页引擎把导出的 HTML 排成 A4 页面。
// 引擎只在导出期间以不可见窗口存在，主界面仍是原生绘制。
func (a *nativeApp) exportPDF() {
	if a.exporting {
		return
	}
	name, body, ok := a.exportBody()
	if !ok {
		return
	}
	html := exportDocument(name, body)
	win := a.window()
	a.exporting = true
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "导出 PDF", DefaultPath: exportName(name, ".pdf"),
			Filters:           []mygo.FileFilter{{Name: "PDF 文档", Extensions: []string{"pdf"}}},
			CreateDirectories: true,
		})
		if err == nil && target != "" {
			var pdf []byte
			if pdf, err = renderPDF(html); err == nil {
				err = os.WriteFile(target, pdf, 0644)
			}
		}
		a.update(func() { a.exporting = false })
		a.exportDone(target, err)
	}()
}

func (a *nativeApp) exportDone(target string, err error) {
	a.update(func() {
		switch {
		case err != nil:
			a.notice = "导出失败：" + err.Error()
		case target != "":
			a.notice = "已导出 " + filepath.Base(target)
		}
	})
}

// renderPDF 在不可见的网页窗口里加载 HTML 并打印成 PDF。须在后台协程调用。
func renderPDF(html string) ([]byte, error) {
	var page *mygo.Window
	loaded := make(chan error, 2)
	mygo.RunOnMain(func() {
		page = mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: 794, Height: 1123})
		page.Page().OnDidFinishLoad(func() { loaded <- nil })
		page.Page().OnDidFailLoad(func(e *mygo.LoadError) { loaded <- fmt.Errorf("加载导出内容失败：%v", e) })
		page.Page().LoadHTML(html, "")
	})
	defer page.Destroy()
	select {
	case err := <-loaded:
		if err != nil {
			return nil, err
		}
	case <-time.After(20 * time.Second):
		return nil, errors.New("排版导出内容超时")
	}
	pdf, err := page.Page().PrintToPDF(mygo.PDFOptions{PageSize: mygo.PageA4, Background: true, Margins: &mygo.Margins{Top: 0.6, Right: 0.6, Bottom: 0.6, Left: 0.6}})
	if err != nil {
		return nil, err
	}
	if len(pdf) == 0 {
		return nil, errors.New("没有生成 PDF 内容")
	}
	return pdf, nil
}

// rawEditor 由真实编辑器实现：双击公式、图表等原文块时请求编辑源码。
type rawEditor interface {
	SetEditRaw(fn func(index int, source string))
	ReplaceRaw(index int, markdown string) bool
}

// editRaw 打开源码编辑框；确定后整块替换，改成受支持的语法即成为可直接编辑的内容。
func (a *nativeApp) editRaw(editor rawEditor, index int, source string) {
	a.rawText, a.rawOpen = source, true
	a.rawOK = func(markdown string) {
		if editor.ReplaceRaw(index, markdown) {
			a.syncEditor(a.active())
			a.refreshTitle()
		}
	}
}

func (a *nativeApp) viewRawEditor(c *ui.Context) {
	ui.Modal(c, &a.rawOpen, func() {
		ui.Text(c, "编辑源码").Bold().FontSize(18)
		ui.Text(c, "公式、图表与其他扩展语法以 Markdown 原文保存。清空内容会删除这一块。").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextArea(c, &a.rawText).Width(520).Height(240).Font(sourceFontFamily).FontSize(13).LineHeight(1.6).Label("原文源码").AutoFocus()
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.rawOpen = false
			}
			if ui.PrimaryButton(c, "确定").Clicked() || c.Shortcut(ui.Cmd, ui.KeyEnter) {
				if a.rawOK != nil {
					a.rawOK(a.rawText)
				}
				a.rawOpen = false
			}
		})
	})
}
