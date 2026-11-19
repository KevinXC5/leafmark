package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
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

func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 189, G: 120, B: 88, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAssetsReadImageRawAndImageReader(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "note.md")
	pngBytes := makeTestPNG(t)
	svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="#bd7858"/></svg>`)
	if err := os.WriteFile(docPath, []byte("# 笔记\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), pngBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), svgBytes, 0644); err != nil {
		t.Fatal(err)
	}

	store := documents.NewStore("未命名.md", "")
	unsaved := store.Current()
	doc, err := store.Load(docPath)
	if err != nil {
		t.Fatal(err)
	}
	service := NewAssets(&Files{store: store})

	if _, _, err := service.ReadImageRaw(unsaved.ID, "pic.png"); err == nil {
		t.Fatal("未保存文档不应允许 ReadImageRaw")
	}
	if _, _, err := service.ReadImageRaw("invalid-id", "pic.png"); !errors.Is(err, documents.ErrStale) {
		t.Fatalf("未授权 ID 应返回 ErrStale：%v", err)
	}

	raw, format, err := service.ReadImageRaw(doc.ID, "pic.png")
	if err != nil || format != "png" || !bytes.Equal(raw, pngBytes) {
		t.Fatalf("ReadImageRaw PNG 结果异常：format=%q len=%d err=%v", format, len(raw), err)
	}
	rawSVG, formatSVG, err := service.ReadImageRaw(doc.ID, "icon.svg")
	if err != nil || formatSVG != "svg" || !bytes.Equal(rawSVG, svgBytes) {
		t.Fatalf("ReadImageRaw SVG 结果异常：format=%q len=%d err=%v", formatSVG, len(rawSVG), err)
	}

	reader := imageReader(service, doc.ID)
	if pic := reader("pic.png"); pic == nil {
		t.Fatal("imageReader 直读本地 PNG 失败")
	}
	if pic := reader("icon.svg"); pic == nil {
		t.Fatal("imageReader 直读本地 SVG 失败")
	}
	dataPNG := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	if pic := reader(dataPNG); pic == nil {
		t.Fatal("imageReader 解码 data URI PNG 失败")
	}
	dataSVG := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svgBytes)
	if pic := reader(dataSVG); pic == nil {
		t.Fatal("imageReader 解码 data URI SVG 失败")
	}
	if pic := reader("../escape.png"); pic != nil {
		t.Fatal("越界路径不应读取成功")
	}
}
