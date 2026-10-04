package documents

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUTF8RoundTripPreservesBOMAndLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "中文.md")
	original := "\ufeff# 标题\r\n\r\n你好 🌿\r\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	originalStat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore("", "")
	doc, err := s.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != "# 标题\n\n你好 🌿\n" {
		t.Fatalf("读取结果：%q", doc.Content)
	}
	updated := doc.Content + "第二行\n"
	if err := s.Draft(doc.ID, updated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(doc.ID, updated, path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != original+"第二行\r\n" {
		t.Fatalf("保存结果：%q", raw)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != originalStat.Mode().Perm() {
		t.Fatal("保存改变了原文件权限")
	}
	if s.Current().Dirty {
		t.Fatal("保存后仍为未保存状态")
	}
}
func TestExternalChangesAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	os.WriteFile(path, []byte("原文"), 0644)
	s := NewStore("", "")
	doc, _ := s.Load(path)
	s.Draft(doc.ID, "我的修改")
	os.WriteFile(path, []byte("外部修改"), 0644)
	if _, err := s.Save(doc.ID, "我的修改", path); !errors.Is(err, ErrConflict) {
		t.Fatalf("预期冲突，得到 %v", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "外部修改" {
		t.Fatal("外部修改被覆盖")
	}
	if !s.Current().Dirty {
		t.Fatal("冲突后草稿状态丢失")
	}
}
func TestSaveKeepsNewerDraftAndRejectsOldDocument(t *testing.T) {
	s := NewStore("未命名.md", "")
	doc := s.Current()
	s.Draft(doc.ID, "新输入")
	path := filepath.Join(t.TempDir(), "note.md")
	saved, err := s.Save(doc.ID, "保存时的内容", path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Content != "新输入" || !saved.Dirty {
		t.Fatal("保存覆盖了新的输入")
	}
	s.New("新文档.md", "")
	s.Close(doc.ID)
	if err := s.Draft(doc.ID, "旧输入"); !errors.Is(err, ErrStale) {
		t.Fatalf("预期过期文档错误：%v", err)
	}
}
func TestSymlinkSavePreservesLink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.md"), filepath.Join(dir, "link.md")
	os.WriteFile(target, []byte("原文"), 0644)
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	s := NewStore("", "")
	doc, err := s.Load(link)
	if err != nil {
		t.Fatal(err)
	}
	s.Draft(doc.ID, "修改")
	if _, err := s.Save(doc.ID, "修改", doc.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatal("符号链接被替换")
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "修改" {
		t.Fatal("未更新链接目标")
	}
}
