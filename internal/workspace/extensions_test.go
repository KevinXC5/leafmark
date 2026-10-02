package workspace

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRecentAndClearRequirePersistedAuthorization(t *testing.T) {
	s, dir, data := setup(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	write(t, outside, string(rune(0xfeff))+"# 近期\r\n")
	outside, _ = filepath.EvalSymlinks(outside)
	if _, err := s.OpenRecent(outside); err == nil {
		t.Fatal("允许读取未授权绝对路径")
	}
	if err := s.RememberAuthorized(outside); err != nil {
		t.Fatal(err)
	}
	restored, err := NewAt(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := restored.OpenRecent(outside)
	if err != nil || doc.Content != "# 近期\n" || doc.Path != outside {
		t.Fatalf("近期读取失败：%+v %v", doc, err)
	}
	if _, err := restored.RevealPath(outside); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.RevealPath(dir); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.md")
	write(t, other, "未打开")
	if _, err := restored.RevealPath(other); err == nil {
		t.Fatal("定位允许未授权文件")
	}
	if err := restored.DocumentHint(other); err == nil {
		t.Fatal("图片接口提示允许任意路径")
	}
	if err := restored.DocumentHint(outside); err != nil {
		t.Fatal(err)
	}
	if err := restored.ClearRecent(); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.OpenRecent(outside); err == nil {
		t.Fatal("清空后仍允许读取")
	}
	again, err := NewAt(data)
	if err != nil || len(again.Recent()) != 0 {
		t.Fatalf("清空未持久化：%v", err)
	}
	if again.Current().Workspace.Path != dir {
		t.Fatal("清空近期丢失工作区")
	}
}
func TestRecentRejectsReplacedSymlink(t *testing.T) {
	s, dir, _ := setup(t)
	path := filepath.Join(dir, "note.md")
	write(t, path, "授权")
	if err := s.Remember("note.md"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "secret.md")
	write(t, target, "秘密")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenRecent(path); err == nil {
		t.Fatal("近期文件被替换成链接后仍允许读取")
	}
	if _, err := s.RevealPath(path); err == nil {
		t.Fatal("近期文件被替换成链接后仍允许定位")
	}
}
func TestImageDataURIValidatesContentAndSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	uri, err := ReadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Fatal("data URI 格式错误")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/png;base64,"))
	if err != nil || !bytes.Equal(raw, buf.Bytes()) {
		t.Fatal("图片数据不匹配")
	}
	for _, raw := range [][]byte{[]byte("<svg></svg>"), []byte("不是图片"), buf.Bytes()[:len(buf.Bytes())/2], make([]byte, MaxImageBytes+1)} {
		if _, err := imageDataURI(raw); err == nil {
			t.Fatal("允许损坏、不支持或过大图片")
		}
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadImage(link); err == nil {
		t.Fatal("允许直接读取图片链接")
	}
}
