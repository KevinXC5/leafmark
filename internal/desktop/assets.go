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
	for _, doc := range a.files.store.List() {
		if doc.ID == id {
			return doc, nil
		}
	}
	return documents.Document{}, documents.ErrStale
}

func (a *Assets) ReadImage(id, resource string) (string, error) {
	doc, err := a.document(id)
	if err != nil {
		return "", err
	}
	if doc.Path == "" {
		return "", errors.New("请先保存文档后再读取相对图片")
	}
	return assets.ReadImage(doc.Path, resource)
}

func (a *Assets) ImportImage(ctx context.Context, id string) (*assets.ImportResult, error) {
	if _, err := a.document(id); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "选择图片",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp"}}},
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
