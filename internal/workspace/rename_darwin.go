//go:build darwin

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/ebitengine/purego"
)

var renameNative struct {
	once  sync.Once
	call  func(int32, string, int32, string, uint32) int32
	errno func() *int32
	err   error
}

func renameExclusive(root *os.Root, oldPath, newPath string) error {
	renameNative.once.Do(func() {
		handle, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			renameNative.err = err
			return
		}
		purego.RegisterLibFunc(&renameNative.call, handle, "renameatx_np")
		purego.RegisterLibFunc(&renameNative.errno, handle, "__error")
	})
	if renameNative.err != nil {
		return renameNative.err
	}
	// 固定两个父目录的描述符，避免通过绝对路径操作时父目录被替换造成越界。
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
	// errno 属于线程本地数据，两次原生调用必须位于同一操作系统线程。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const renameExcl = 0x4
	if renameNative.call(int32(oldParent.Fd()), filepath.Base(oldPath), int32(newParent.Fd()), filepath.Base(newPath), renameExcl) != 0 {
		err := syscall.Errno(*renameNative.errno())
		if errors.Is(err, syscall.EEXIST) {
			return ErrExists
		}
		return err
	}
	return nil
}
