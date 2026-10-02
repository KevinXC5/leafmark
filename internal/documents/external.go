package documents

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrDirty = errors.New("文档包含未保存修改，请明确确认丢弃后重新加载")

// ExternalState 分别报告磁盘内容变化和原路径缺失，不修改草稿或磁盘摘要。
type ExternalState struct {
	Changed bool `json:"changed"`
	Missing bool `json:"missing"`
}

func (s *Store) openedLocked(id string) (snapshot, error) {
	if id != "" && id == s.doc.ID {
		return snapshot{s.doc, s.saved, s.digest, s.crlf, s.bom}, nil
	}
	entry, exists := s.sessions[id]
	if !exists {
		return snapshot{}, ErrStale
	}
	return entry, nil
}

func (s *Store) CheckExternal(id string) (ExternalState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.openedLocked(id)
	if err != nil {
		return ExternalState{}, err
	}
	if entry.doc.Path == "" {
		return ExternalState{}, nil
	}
	raw, err := readAuthorizedFile(entry.doc.Path)
	if errors.Is(err, os.ErrNotExist) {
		return ExternalState{Missing: true}, nil
	}
	if err != nil {
		return ExternalState{}, err
	}
	return ExternalState{Changed: sha256.Sum256(raw) != entry.digest}, nil
}

// Reload 只读取已打开文档的授权路径；非当前标签的快照更新不改变选中标签。
func (s *Store) Reload(id string, discardDirty bool) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.openedLocked(id)
	if err != nil {
		return Document{}, err
	}
	if entry.doc.Dirty && !discardDirty {
		return Document{}, ErrDirty
	}
	if entry.doc.Path == "" {
		return Document{}, errors.New("未保存的文档没有可重新加载的文件路径")
	}
	raw, err := readAuthorizedFile(entry.doc.Path)
	if err != nil {
		return Document{}, err
	}
	if err := validate(string(raw)); err != nil {
		return Document{}, err
	}
	entry.bom = strings.HasPrefix(string(raw), string(rune(0xfeff)))
	text := strings.TrimPrefix(string(raw), string(rune(0xfeff)))
	entry.crlf = strings.Contains(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	entry.doc.Content, entry.doc.Dirty = text, false
	entry.saved, entry.digest = text, sha256.Sum256(raw)
	if s.doc.ID == id {
		s.doc, s.saved, s.digest, s.crlf, s.bom = entry.doc, entry.saved, entry.digest, entry.crlf, entry.bom
	}
	// 原位更新同一 id，避免重复路径创建标签或丢失标签顺序。
	if s.sessions == nil {
		s.sessions = make(map[string]snapshot)
	}
	if _, exists := s.sessions[id]; !exists {
		s.order = append(s.order, id)
	}
	s.sessions[id] = entry
	return entry.doc, nil
}

// readAuthorizedFile 不跟随后来出现的符号链接。原生打开时已解析链接，
// 因此这里只接受原授权的实际路径；普通文件原位替换仍允许重新加载。
func readAuthorizedFile(path string) ([]byte, error) {
	invalid := errors.New("已打开文档的路径发生变化或不是普通文件，请重新打开")
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || actual != path {
		return nil, invalid
	}
	dir, name := filepath.Dir(path), filepath.Base(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	openedParent, err := parent.Stat()
	if err != nil {
		return nil, err
	}
	verifyParent := func() error {
		actual, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		current, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if actual != dir || !current.IsDir() || !os.SameFile(openedParent, current) {
			return invalid
		}
		return nil
	}
	if err := verifyParent(); err != nil {
		return nil, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, invalid
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	// 读取前核对检查前、打开句柄和检查后的身份，避免路径竞争扩大授权。
	if !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(before, opened) || !os.SameFile(opened, current) {
		return nil, invalid
	}
	if err := verifyParent(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, errors.New("验证版本支持最大 16 MB 的文档")
	}
	return raw, nil
}
