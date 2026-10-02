package documents

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExportCopyPreservesDocumentState(t *testing.T) {
	s := NewStore("未命名.md", "原文")
	doc := s.Current()
	if err := s.Draft(doc.ID, "草稿"); err != nil {
		t.Fatal(err)
	}
	before := s.Current()
	path := filepath.Join(t.TempDir(), "copy.md")
	if err := s.ExportCopy(doc.ID, "# 中文\n导出内容", path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "# 中文\n导出内容" {
		t.Fatalf("副本内容错误：%v", err)
	}
	if !reflect.DeepEqual(before, s.Current()) {
		t.Fatal("导出改变了当前文档")
	}
	if s.saved != "原文" {
		t.Fatal("导出改变了已保存摘要状态")
	}
	if err := s.ExportCopy("过期", "内容", path); !errors.Is(err, ErrStale) {
		t.Fatalf("未拒绝过期标签：%v", err)
	}
}
func TestExportCopyRejectsAllOpenedPathsAndAliases(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.md"), filepath.Join(dir, "second.md")
	os.WriteFile(first, []byte("原文一"), 0600)
	os.WriteFile(second, []byte("原文二"), 0600)
	s := NewStore("", "")
	if _, err := s.Load(first); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Load(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, second} {
		if err := s.ExportCopy(doc.ID, "覆盖", path); err == nil {
			t.Fatalf("允许覆盖已打开目标：%s", path)
		}
	}
	alias := filepath.Join(dir, "alias.md")
	if err := os.Link(first, alias); err == nil {
		if err := s.ExportCopy(doc.ID, "覆盖", alias); err == nil {
			t.Fatal("允许覆盖已打开文件的硬链接")
		}
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(first, link); err == nil {
		if err := s.ExportCopy(doc.ID, "覆盖", link); err == nil {
			t.Fatal("允许覆盖符号链接目标")
		}
	}
	for path, expected := range map[string]string{first: "原文一", second: "原文二"} {
		raw, _ := os.ReadFile(path)
		if string(raw) != expected {
			t.Fatal("保护失败，文件被覆盖")
		}
	}
	if err := s.ExportCopy(doc.ID, string([]byte{0xff}), filepath.Join(dir, "invalid.md")); err == nil {
		t.Fatal("允许无效 UTF-8")
	}
	if err := s.ExportCopy(doc.ID, "内容", filepath.Join(dir, "copy.txt")); err == nil {
		t.Fatal("允许其他扩展名")
	}
}
