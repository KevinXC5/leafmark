package desktop

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"leafmark/internal/documents"
)

// OpenRequested 通知页面有系统交给应用的文档等待打开，载荷为排队数量。
var OpenRequested = mygo.NewEvent[int]("files:open-requested")

// Files 仅提供用户授权的文档操作：原生文件选择框，或经 Finder 打开、拖入窗口交给应用的文档。
type Files struct {
	store     *documents.Store
	workspace *Workspace

	mu      sync.Mutex
	pending []string
}

// requestOpen 记录系统交给应用的 Markdown 文档（Finder 打开方式、拖入窗口或 Dock 图标），
// 路径只来自原生事件，页面不能指定；返回排队数量，由页面在空闲时调用 OpenPending 逐个取用。
func (f *Files) requestOpen(paths []string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		f.pending = append(f.pending, path)
	}
	return len(f.pending)
}

// takePending 取出下一份已登记路径，不读取磁盘，也不改变当前标签。
func (f *Files) takePending() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return ""
	}
	path := f.pending[0]
	f.pending = f.pending[1:]
	return path
}

// OpenPending 打开下一份排队的文档，队列为空时返回 nil。
func (f *Files) OpenPending() (*documents.Document, error) {
	f.mu.Lock()
	if len(f.pending) == 0 {
		f.mu.Unlock()
		return nil, nil
	}
	path := f.pending[0]
	f.pending = f.pending[1:]
	f.mu.Unlock()
	doc, err := f.store.Load(path)
	if err != nil {
		return nil, err
	}
	if f.workspace != nil {
		if err := f.workspace.rememberAuthorized(doc.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &doc, nil
}

func (f *Files) OpenWorkspace(relative string) (documents.Document, error) {
	doc, err := f.workspace.OpenDocument(relative)
	if err != nil {
		return documents.Document{}, err
	}
	return f.store.LoadSnapshot(doc.Path, doc.Raw)
}
func (f *Files) OpenRecent(path string) (documents.Document, error) {
	backend, err := f.workspace.backend()
	if err != nil {
		return documents.Document{}, err
	}
	doc, err := backend.OpenRecent(path)
	if err != nil {
		return documents.Document{}, err
	}
	return f.store.LoadSnapshot(doc.Path, doc.Raw)
}

func (f *Files) Current() documents.Document                  { return f.store.Current() }
func (f *Files) List() []documents.Document                   { return f.store.List() }
func (f *Files) Select(id string) (documents.Document, error) { return f.store.Select(id) }
func (f *Files) Close(id string) error                        { return f.store.Close(id) }
func (f *Files) New() documents.Document                      { return f.store.New("未命名.md", "") }
func (f *Files) Draft(id, content string) error               { return f.store.Draft(id, content) }

func (f *Files) NewNamed(name string) (documents.Document, error) {
	return f.store.NewNamed(name)
}

func (f *Files) Session() documents.SessionState {
	return f.store.Sessions()
}

func (f *Files) CheckExternal(id string) (documents.ExternalState, error) {
	return f.store.CheckExternal(id)
}
func (f *Files) Reload(id string, discardDirty bool) (documents.Document, error) {
	return f.store.Reload(id, discardDirty)
}

func (f *Files) Open(ctx context.Context) (*documents.Document, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "打开 Markdown 文档",
		Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	doc, err := f.store.Load(paths[0])
	if err != nil {
		return nil, err
	}
	if f.workspace != nil {
		if err := f.workspace.rememberAuthorized(doc.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &doc, nil
}

func (f *Files) Save(ctx context.Context, id, content string, saveAs bool) (*documents.Document, error) {
	return f.save(mygo.CallerWindow(ctx), id, content, saveAs)
}

func (f *Files) save(win *mygo.Window, id, content string, saveAs bool) (*documents.Document, error) {
	doc := f.store.Current()
	if doc.ID != id {
		return nil, documents.ErrStale
	}
	path := doc.Path
	if path == "" || saveAs {
		var err error
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "保存 Markdown 文档", DefaultPath: doc.Name,
			Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
		})
		if err != nil || path == "" {
			return nil, err
		}
	}
	saved, err := f.store.Save(id, content, path)
	if err != nil {
		return nil, err
	}
	if f.workspace != nil {
		if err := f.workspace.rememberAuthorized(saved.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &saved, nil
}

func (f *Files) ExportMarkdown(ctx context.Context, id, content string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	doc := f.store.Current()
	if doc.ID != id {
		return "", documents.ErrStale
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "导出 Markdown 副本", DefaultPath: doc.Name,
		Filters:           []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
		CreateDirectories: true,
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := f.store.ExportCopy(id, content, path); err != nil {
		return "", err
	}
	return path, nil
}
