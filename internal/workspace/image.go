package workspace

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
)

const MaxImageBytes = 8 << 20
const MaxImagePixels = 16 << 20

// ReadImage 的路径必须来自原生选择框，前端不能调用此函数直接指定路径。
func ReadImage(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("请选择普通图片文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("请选择普通图片文件")
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > MaxImageBytes {
		return "", errors.New("图片不能超过 8 MB")
	}
	return imageDataURI(raw)
}
func imageDataURI(raw []byte) (string, error) {
	if len(raw) > MaxImageBytes {
		return "", errors.New("图片不能超过 8 MB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", errors.New("请选择有效的 PNG、JPEG 或 GIF 图片")
	}
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}[format]
	if mime == "" || config.Width <= 0 || config.Height <= 0 {
		return "", errors.New("不支持此图片格式")
	}
	// 同时限制解码后的像素数，避免小体积压缩图片造成过量内存分配。
	if int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return "", errors.New("图片像素数过大")
	}
	if _, decodedFormat, err := image.Decode(bytes.NewReader(raw)); err != nil || decodedFormat != format {
		return "", errors.New("图片内容损坏")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
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
