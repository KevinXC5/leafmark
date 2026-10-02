//go:build windows

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func renameExclusive(root *os.Root, oldPath, newPath string) error {
	// MoveFileW 不设置替换标志，目标存在时由 Windows 原子拒绝覆盖。
	// 以不共享删除权限的句柄固定两个父目录的每一级，避免检查后目录被换成联接。
	var handles []syscall.Handle
	defer func() {
		for _, handle := range handles {
			syscall.CloseHandle(handle)
		}
	}()
	for _, path := range []string{filepath.Join(root.Name(), filepath.Dir(oldPath)), filepath.Join(root.Name(), filepath.Dir(newPath))} {
		for current := path; ; current = filepath.Dir(current) {
			name, err := syscall.UTF16PtrFromString(current)
			if err != nil {
				return err
			}
			handle, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
			if err != nil {
				return err
			}
			handles = append(handles, handle)
			var info syscall.ByHandleFileInformation
			if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
				return err
			}
			if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
				return ErrInvalidPath
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	for _, path := range []string{oldPath, newPath} {
		if _, err := checked(root, filepath.ToSlash(path), path == newPath); err != nil {
			return err
		}
	}
	from, err := syscall.UTF16PtrFromString(filepath.Join(root.Name(), oldPath))
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(filepath.Join(root.Name(), newPath))
	if err != nil {
		return err
	}
	err = syscall.MoveFile(from, to)
	if errors.Is(err, syscall.ERROR_ALREADY_EXISTS) || errors.Is(err, syscall.ERROR_FILE_EXISTS) {
		return ErrExists
	}
	return err
}
