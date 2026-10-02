package desktop

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"leafmark/internal/workspace"
)

// Workspace 可以直接通过 mygo.Bind(&Workspace{}) 绑定；零值和构造函数均延迟初始化。
type Workspace struct {
	once  sync.Once
	store *workspace.Store
	err   error
}

func NewWorkspace() *Workspace { return &Workspace{} }
func (w *Workspace) backend() (*workspace.Store, error) {
	w.once.Do(func() { w.store, w.err = workspace.New() })
	return w.store, w.err
}
func (w *Workspace) Current() (workspace.State, error) {
	s, err := w.backend()
	if err != nil {
		return workspace.State{}, err
	}
	return s.Current(), nil
}
func (w *Workspace) OpenFolder(ctx context.Context) (*workspace.Folder, error) {
	s, err := w.backend()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "选择工作区文件夹", Directory: true,
		CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.SelectFolder(paths[0])
}
func (w *Workspace) Tree() ([]workspace.Node, error) {
	s, err := w.backend()
	if err != nil {
		return nil, err
	}
	return s.Tree()
}
func (w *Workspace) OpenDocument(relative string) (*workspace.Document, error) {
	s, err := w.backend()
	if err != nil {
		return nil, err
	}
	return s.OpenDocument(relative)
}
func (w *Workspace) CreateFile(relative string) error {
	s, err := w.backend()
	if err != nil {
		return err
	}
	return s.CreateFile(relative)
}
func (w *Workspace) CreateFolder(relative string) error {
	s, err := w.backend()
	if err != nil {
		return err
	}
	return s.CreateFolder(relative)
}
func (w *Workspace) Rename(oldRelative, newRelative string) error {
	s, err := w.backend()
	if err != nil {
		return err
	}
	return s.Rename(oldRelative, newRelative)
}
func (w *Workspace) Recent() ([]workspace.RecentDocument, error) {
	s, err := w.backend()
	if err != nil {
		return nil, err
	}
	return s.Recent(), nil
}

// Remember 只接受工作区相对路径，不用近期记录赋予任意绝对路径访问权限。
func (w *Workspace) Remember(relative string) error {
	s, err := w.backend()
	if err != nil {
		return err
	}
	return s.Remember(relative)
}
func (w *Workspace) ExportHTML(ctx context.Context, name, html string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// 前端名称仅作为保存框中的文件名建议，不能影响默认保存目录。
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" {
		name = "未命名"
	}
	name = strings.TrimSuffix(name, filepath.Ext(name)) + ".html"
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "导出 HTML", DefaultPath: name,
		Filters:           []mygo.FileFilter{{Name: "HTML 文档", Extensions: []string{"html", "htm"}}},
		CreateDirectories: true,
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := workspace.WriteHTML(path, html); err != nil {
		return "", err
	}
	return path, nil
}

func (w *Workspace) OpenRecent(path string) (*workspace.Document, error) {
	s, err := w.backend()
	if err != nil {
		return nil, err
	}
	return s.OpenRecent(path)
}
func (w *Workspace) ClearRecent() error {
	s, err := w.backend()
	if err != nil {
		return err
	}
	return s.ClearRecent()
}
func (w *Workspace) BrowseImage(ctx context.Context, documentPath string) (string, error) {
	s, err := w.backend()
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.DocumentHint(documentPath); err != nil {
		return "", err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "选择图片",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}},
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return workspace.ReadImage(paths[0])
}
func (w *Workspace) ShowInFolder(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s, err := w.backend()
	if err != nil {
		return err
	}
	path, err = s.RevealPath(path)
	if err != nil {
		return err
	}
	mygo.Shell.ShowItemInFolder(path)
	return nil
}

// rememberAuthorized 由 Files 在原生选择/保存文档成功后调用，不会被 mygo 导出。
func (w *Workspace) rememberAuthorized(path string) error {
	s, err := w.backend()
	if err != nil {
		return err
	}
	return s.RememberAuthorized(path)
}
