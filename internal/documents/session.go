package documents

import (
	"errors"
	"path/filepath"
	"strings"
)

// NewNamed 仅接受 Markdown 文件名，恢复无路径草稿时不能借名称传入路径。
func (s *Store) NewNamed(name string) (Document, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if strings.TrimSpace(name) == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\:\x00") || (ext != ".md" && ext != ".markdown") || len(name) == len(ext) {
		return Document{}, errors.New("草稿名称必须是不含路径的 Markdown 文件名")
	}
	if err := validate(name); err != nil {
		return Document{}, err
	}
	return s.New(name, ""), nil
}

// ExportedSession 提供编辑器草稿及其保存基线，不暴露磁盘摘要和编码细节。
type ExportedSession struct {
	Document
	SavedContent string `json:"savedContent"`
}

type SessionState struct {
	CurrentID string            `json:"currentId"`
	Documents []ExportedSession `json:"documents"`
}

// Sessions 原子导出窗口内所有标签，用于页面刷新后恢复草稿和准确的 dirty 基线。
func (s *Store) Sessions() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	state := SessionState{CurrentID: s.doc.ID, Documents: make([]ExportedSession, 0, len(s.order))}
	for _, id := range s.order {
		entry := s.sessions[id]
		state.Documents = append(state.Documents, ExportedSession{Document: entry.doc, SavedContent: entry.saved})
	}
	return state
}

func (s *Store) SavedContent(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.openedLocked(id)
	if err != nil {
		return "", err
	}
	return entry.saved, nil
}
