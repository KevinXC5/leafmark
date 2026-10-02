//go:build linux

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

func renameExclusive(root *os.Root, oldPath, newPath string) error {
	// 不使用普通 rename 作为回退：旧内核不支持 RENAME_NOREPLACE 时应明确报错。
	numbers := map[string]uintptr{"amd64": 316, "386": 353, "arm": 382, "arm64": 276, "riscv64": 276, "loong64": 276, "s390x": 347, "ppc64": 357, "ppc64le": 357, "mips": 4351, "mipsle": 4351, "mips64": 5311, "mips64le": 5311}
	number, ok := numbers[runtime.GOARCH]
	if !ok {
		return errors.New("此架构暂不支持安全的独占重命名")
	}
	oldParent, err := root.Open(filepath.Dir(oldPath))
	if err != nil {
		return err
	}
	defer oldParent.Close()
	newParent, err := root.Open(filepath.Dir(newPath))
	if err != nil {
		return err
	}
	defer newParent.Close()
	oldName, err := syscall.BytePtrFromString(filepath.Base(oldPath))
	if err != nil {
		return err
	}
	newName, err := syscall.BytePtrFromString(filepath.Base(newPath))
	if err != nil {
		return err
	}
	const renameNoReplace = 1
	_, _, errno := syscall.Syscall6(number, oldParent.Fd(), uintptr(unsafe.Pointer(oldName)), newParent.Fd(), uintptr(unsafe.Pointer(newName)), renameNoReplace, 0)
	runtime.KeepAlive(oldName)
	runtime.KeepAlive(newName)
	if errno == syscall.EEXIST {
		return ErrExists
	}
	if errno != 0 {
		return errno
	}
	return nil
}
