package documents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ExportCopy 的路径必须来自原生保存框，仅写出 UTF-8 副本，不更新标签、草稿和摘要。
func (s *Store) ExportCopy(id, content, path string) error {
	if err := validate(content); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return errors.New("导出目标必须来自原生保存框")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".md" && ext != ".markdown" {
		return errors.New("导出文件必须使用 .md 或 .markdown 扩展名")
	}
	// 固定解析后的父目录，最终目标不允许符号链接。
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	path = filepath.Join(parent, filepath.Base(path))
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("导出目标不是普通文件")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc.ID != id {
		return ErrStale
	}
	if s.doc.Path != "" && sameFilePath(path, s.doc.Path) {
		return errors.New("不能将副本导出到已打开的文档")
	}
	for _, entry := range s.sessions {
		if entry.doc.Path != "" && sameFilePath(path, entry.doc.Path) {
			return errors.New("不能将副本导出到已打开的文档")
		}
	}
	return atomicWrite(path, []byte(content))
}
