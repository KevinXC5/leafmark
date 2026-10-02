package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func (s *Store) listed(path string) bool {
	for _, recent := range s.state.Recent {
		if recent.Path == path {
			return true
		}
	}
	return false
}

// OpenRecent 只允许精确匹配已保存记录的绝对路径，历史列表不能用作目录授权。
func (s *Store) OpenRecent(path string) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !filepath.IsAbs(path) || !s.listed(path) {
		return nil, ErrInvalidPath
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if actual != path {
		return nil, ErrInvalidPath
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := verifyRoot(root, filepath.Dir(path)); err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !markdown(name) {
		return nil, ErrInvalidPath
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := verifyOpened(root, name, f, info); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxDocumentBytes {
		return nil, errors.New("文档不能超过 16 MB")
	}
	if !utf8.Valid(raw) || strings.ContainsRune(string(raw), '\x00') {
		return nil, errors.New("请打开 UTF-8 编码的 Markdown 文档")
	}
	if err := s.remember(path); err != nil {
		return nil, err
	}
	relative := ""
	if s.state.Workspace != nil {
		if rel, err := filepath.Rel(s.state.Workspace.Path, path); err == nil && filepath.IsLocal(rel) {
			relative = filepath.ToSlash(rel)
		}
	}
	return &Document{Name: name, Path: path, Relative: relative, Raw: raw, Content: strings.ReplaceAll(strings.TrimPrefix(string(raw), string(rune(0xfeff))), "\r\n", "\n")}, nil
}

func (s *Store) ClearRecent() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneState(s.state)
	next.Recent = []RecentDocument{}
	return s.commit(next)
}

// RevealPath 只授权工作区根目录或精确匹配的近期文档；不授权任意工作区子路径。
func (s *Store) RevealPath(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	isRoot := s.state.Workspace != nil && path == s.state.Workspace.Path
	if !filepath.IsAbs(path) || (!isRoot && !s.listed(path)) {
		return "", ErrInvalidPath
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if actual != path {
		return "", ErrInvalidPath
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if (isRoot && !info.IsDir()) || (!isRoot && !info.Mode().IsRegular()) {
		return "", ErrInvalidPath
	}
	return path, nil
}
