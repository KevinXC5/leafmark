package desktop

import (
	"context"
	"errors"

	"github.com/egoist/mygo"
	"leafmark/internal/assets"
	"leafmark/internal/documents"
)

// Assets 仅凭已打开文档 ID 取得后端授权路径，不接收前端提供的文档路径。
type Assets struct {
	files *Files
}

func NewAssets(files *Files) *Assets { return &Assets{files: files} }

func (a *Assets) document(id string) (documents.Document, error) {
	if a == nil || a.files == nil || a.files.store == nil || id == "" {
		return documents.Document{}, documents.ErrStale
	}
	doc, ok := a.files.store.Get(id)
	if !ok {
		return documents.Document{}, documents.ErrStale
	}
	return doc, nil
}

func (a *Assets) ReadImage(id, resource string) (string, error) {
	doc, err := a.document(id)
	if err != nil {
		return "", err
	}
	if doc.Path == "" {
		return "", errors.New("请先保存文档后再读取相对图片")
	}
	return assets.FindImage(doc.Path, a.workspaceRoot(), resource)
}

// ReadImageRaw 读取已授权文档相对路径的图片原始字节与格式，免去 Base64 往返与二次解码。
func (a *Assets) ReadImageRaw(id, markdownPath string) ([]byte, string, error) {
	doc, err := a.document(id)
	if err != nil {
		return nil, "", err
	}
	if doc.Path == "" {
		return nil, "", errors.New("请先保存文档后再读取相对图片")
	}
	return assets.FindImageRaw(doc.Path, a.workspaceRoot(), markdownPath)
}

// workspaceRoot 返回当前工作区的授权根目录，供按文件名查找图片；没有工作区时为空。
func (a *Assets) workspaceRoot() string {
	if a.files.workspace == nil {
		return ""
	}
	state, err := a.files.workspace.Current()
	if err != nil || state.Workspace == nil {
		return ""
	}
	return state.Workspace.Path
}

// ImportImageIn 与 ImportImage 相同，但对话框挂在调用方给出的窗口上。
// 原生界面没有网页调用上下文，不能依赖 CallerWindow。
func (a *Assets) ImportImageIn(win *mygo.Window, id string) (*assets.ImportResult, error) {
	return a.importImage(context.Background(), win, id)
}

func (a *Assets) ImportImage(ctx context.Context, id string) (*assets.ImportResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.importImage(ctx, mygo.CallerWindow(ctx), id)
}

func (a *Assets) importImage(ctx context.Context, win *mygo.Window, id string) (*assets.ImportResult, error) {
	if _, err := a.document(id); err != nil {
		return nil, err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: win, Title: "选择图片",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "svg"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 原生选择框等待期间文档可能关闭或另存为，操作前重新取得授权快照。
	doc, err := a.document(id)
	if err != nil {
		return nil, err
	}
	return assets.ImportImage(doc.Path, paths[0])
}
