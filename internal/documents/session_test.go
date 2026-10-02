package documents

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSessionsPreserveEveryDraftAndSavedBaseline(t *testing.T) {
	s := NewStore("草稿.md", "初始内容")
	first := s.Current()
	s.Draft(first.ID, "未保存草稿")
	second := loadExternalTestDocument(t, s, string(rune(0xfeff))+"磁盘内容\r\n")
	s.Draft(second.ID, "磁盘文档草稿")
	third := s.New("当前.md", "当前基线")
	s.Draft(third.ID, "当前草稿")
	before := s.Current()
	state := s.Sessions()
	if state.CurrentID != third.ID || len(state.Documents) != 3 || s.Current() != before {
		t.Fatalf("会话导出改变选择或标签数量：%+v", state)
	}
	for i, expected := range []struct{ id, content, saved string }{
		{first.ID, "未保存草稿", "初始内容"},
		{second.ID, "磁盘文档草稿", "磁盘内容\n"},
		{third.ID, "当前草稿", "当前基线"},
	} {
		entry := state.Documents[i]
		if entry.ID != expected.id || entry.Content != expected.content || entry.SavedContent != expected.saved || !entry.Dirty {
			t.Fatalf("标签 %d 草稿或保存基线错误：%+v", i, entry)
		}
		saved, err := s.SavedContent(expected.id)
		if err != nil || saved != expected.saved {
			t.Fatalf("独立基线读取错误：%q %v", saved, err)
		}
	}
	// JSON 扁平携带 Document 字段和 savedContent，前端可直接恢复全部标签。
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		CurrentID string `json:"currentId"`
		Documents []struct {
			ID           string `json:"id"`
			SavedContent string `json:"savedContent"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.CurrentID != third.ID || decoded.Documents[1].ID != second.ID || decoded.Documents[1].SavedContent != "磁盘内容\n" {
		t.Fatalf("会话 JSON 格式错误：%s %v", raw, err)
	}
	state.Documents[0].Content = "调用方修改"
	if s.Sessions().Documents[0].Content != "未保存草稿" {
		t.Fatal("导出结果不应共享可变存储")
	}
	writeExternalTestDocument(t, second.Path, "重新加载内容")
	if _, err := s.Reload(second.ID, true); err != nil {
		t.Fatal(err)
	}
	entry := s.Sessions().Documents[1]
	if entry.SavedContent != "重新加载内容" || entry.Content != entry.SavedContent || entry.Dirty || s.Current().ID != third.ID {
		t.Fatalf("非当前标签重载基线错误：%+v", entry)
	}
}

func TestSessionsEmptyAndClosedIDs(t *testing.T) {
	s := NewStore("草稿.md", "内容")
	id := s.Current().ID
	if err := s.Close(id); err != nil {
		t.Fatal(err)
	}
	state := s.Sessions()
	if state.CurrentID != "" || state.Documents == nil || len(state.Documents) != 0 {
		t.Fatalf("空会话应导出空数组：%+v", state)
	}
	if _, err := s.SavedContent(id); !errors.Is(err, ErrStale) {
		t.Fatalf("关闭标签不应可读取基线：%v", err)
	}
}

func TestNewNamedAcceptsOnlyMarkdownBasenames(t *testing.T) {
	s := NewStore("原文.md", "原文")
	for _, name := range []string{"恢复草稿.md", "Notes.MARKDOWN"} {
		doc, err := s.NewNamed(name)
		if err != nil || doc.Name != name || doc.Path != "" || doc.Content != "" || doc.Dirty || doc.ID == "" {
			t.Fatalf("合法草稿名称失败：%q %+v %v", name, doc, err)
		}
	}
	before := s.Current()
	count := len(s.Sessions().Documents)
	for _, name := range []string{"", " ", ".md", ".markdown", "../note.md", "/note.md", "dir/note.md", `dir\note.md`, `C:note.md`, "note.txt", "note.md\x00", string([]byte{0xff}) + ".md"} {
		if _, err := s.NewNamed(name); err == nil {
			t.Fatalf("非法草稿名称被接受：%q", name)
		}
		if s.Current() != before || len(s.Sessions().Documents) != count {
			t.Fatal("非法名称改变了会话")
		}
	}
}
