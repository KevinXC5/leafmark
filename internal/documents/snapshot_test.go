package documents

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSnapshotDoesNotReopenPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	target := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(target, []byte("未经授权的内容"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	s := NewStore("", "")
	doc, err := s.LoadSnapshot(path, []byte("已授权快照"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != "已授权快照" || doc.Path != path {
		t.Fatalf("重新读取了路径：%+v", doc)
	}
}
func TestLoadSnapshotPreservesEncodingAndDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	original := string(rune(0xfeff)) + "# 中文\r\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewStore("", "")
	doc, err := s.LoadSnapshot(path, []byte(original))
	if err != nil || doc.Content != "# 中文\n" {
		t.Fatalf("快照加载失败：%v", err)
	}
	if err := s.Draft(doc.ID, "# 中文\n新行\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(doc.ID, "# 中文\n新行\n", path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != original+"新行\r\n" {
		t.Fatalf("编码未保留：%q", raw)
	}
	if err := os.WriteFile(path, []byte("外部修改"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(doc.ID, "再次修改", path); !errors.Is(err, ErrConflict) {
		t.Fatalf("摘要未生效：%v", err)
	}
}
func TestLoadSnapshotValidatesWithoutFilesystem(t *testing.T) {
	s := NewStore("", "")
	path := filepath.Join(t.TempDir(), "不存在.md")
	if _, err := s.LoadSnapshot(path, []byte("正文")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadSnapshot("relative.md", []byte("正文")); err == nil {
		t.Fatal("允许相对快照路径")
	}
	if _, err := s.LoadSnapshot(path, []byte{0xff}); err == nil {
		t.Fatal("允许无效编码")
	}
}
