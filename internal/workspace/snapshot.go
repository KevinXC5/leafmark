package workspace

import (
	"os"
	"path/filepath"
)

// verifyRoot 对照打开后的目录句柄和路径身份，拒绝检查期间替换的根目录或符号链接。
func verifyRoot(root *os.Root, path string) error {
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if actual != path {
		return ErrInvalidPath
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !current.IsDir() || !os.SameFile(opened, current) {
		return ErrInvalidPath
	}
	return nil
}

// verifyOpened 在读取字节之前核对检查前、句柄及检查后三个身份。
// 检查后文件被替换不会改变已打开的句柄；快照加载也不会重新按路径读取。
func verifyOpened(root *os.Root, path string, f *os.File, before os.FileInfo) error {
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(path)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(before, opened) || !os.SameFile(opened, current) {
		return ErrInvalidPath
	}
	return nil
}
