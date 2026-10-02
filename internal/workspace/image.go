package workspace

import (
	"path/filepath"

	"leafmark/internal/assets"
)

// 保留原有常量供调用方使用，图片限制统一由 assets 定义。
const MaxImageBytes = assets.MaxImageBytes
const MaxImagePixels = assets.MaxImagePixels

// ReadImage 的路径必须来自原生选择框，前端不能调用此函数直接指定路径。
func ReadImage(path string) (string, error) {
	return assets.ReadAuthorizedImage(path)
}

// DocumentHint 只检查授权记录，不读取内容，也不把该路径传给原生图片选择框。
func (s *Store) DocumentHint(path string) error {
	if path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listed(path) && filepath.IsAbs(path) {
		return nil
	}
	return ErrInvalidPath
}
