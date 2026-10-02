package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"leafmark/internal/documents"
)

func TestAssetsRequiresOpenDocumentID(t *testing.T) {
	store := documents.NewStore("未命名.md", "")
	draft := store.Current()
	service := NewAssets(&Files{store: store})
	if _, err := service.ReadImage(draft.ID, "a.png"); err == nil {
		t.Fatal("未保存文档不能读取相对图片")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("# 笔记"), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	store.New("另一个文档.md", "")
	opened, err := service.document(doc.ID)
	if err != nil || opened.Path != doc.Path {
		t.Fatalf("应允许已打开的后台标签：%+v，%v", opened, err)
	}
	if err := store.Close(doc.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{doc.ID, path, "", "伪造 ID"} {
		if _, err := service.ReadImage(id, "a.png"); !errors.Is(err, documents.ErrStale) {
			t.Fatalf("应拒绝未授权 ID %q：%v", id, err)
		}
		if _, err := service.ImportImage(context.Background(), id); !errors.Is(err, documents.ErrStale) {
			t.Fatalf("选择图片前应拒绝未授权 ID %q：%v", id, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ImportImage(ctx, draft.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("应在选择图片前检查取消：%v", err)
	}
	if _, err := NewAssets(nil).ReadImage(draft.ID, "a.png"); !errors.Is(err, documents.ErrStale) {
		t.Fatalf("空服务不应授予授权：%v", err)
	}
}
