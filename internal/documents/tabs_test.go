package documents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTabsKeepDraftsAndDiskSnapshots(t *testing.T) {
	dir := t.TempDir()
	firstPath, secondPath := filepath.Join(dir, "甲.md"), filepath.Join(dir, "乙.md")
	os.WriteFile(firstPath, []byte("甲"), 0600)
	os.WriteFile(secondPath, []byte("乙"), 0600)
	s := NewStore("欢迎", "")
	first, err := s.Load(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	s.Draft(first.ID, "甲的草稿")
	second, err := s.Load(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	s.Draft(second.ID, "乙的草稿")
	selected, err := s.Select(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Content != "甲的草稿" || !selected.Dirty {
		t.Fatal("切换丢失草稿")
	}
	again, err := s.Load(firstPath)
	if err != nil || again.ID != first.ID || again.Content != selected.Content {
		t.Fatal("重复打开未复用标签")
	}
	if _, err := s.Save(first.ID, "甲的草稿", firstPath); err != nil {
		t.Fatal(err)
	}
	s.Select(second.ID)
	if s.Current().Content != "乙的草稿" {
		t.Fatal("保存其他标签覆盖草稿")
	}
	s.Close(second.ID)
	if s.Current().ID != first.ID {
		t.Fatal("关闭后未选择相邻标签")
	}
}
func TestSaveAsRejectsTargetOpenInAnotherTab(t *testing.T) {
 dir := t.TempDir()
 a, b := filepath.Join(dir,"a.md"), filepath.Join(dir,"b.md")
 os.WriteFile(a, []byte("甲"),0600); os.WriteFile(b, []byte("乙"),0600)
 s := NewStore("","")
 first, _ := s.Load(a); s.Load(b); s.Select(first.ID)
 if _, err := s.Save(first.ID, "甲覆盖", b); err == nil { t.Fatal("允许覆盖另一个标签的文件") }
 raw, _ := os.ReadFile(b); if string(raw) != "乙" { t.Fatal("拒绝后目标仍被覆盖") }
}
func TestCloseLastTabLeavesNoGhostDocument(t *testing.T) {
	s := NewStore("未命名.md", "")
	s.Close(s.Current().ID)
	if s.Current().ID != "" || len(s.List()) != 0 {
		t.Fatal("最后一个标签关闭后仍有幽灵文档")
	}
	s.New("新文档.md", "")
	if len(s.List()) != 1 {
		t.Fatal("新建标签数量错误")
	}
}
