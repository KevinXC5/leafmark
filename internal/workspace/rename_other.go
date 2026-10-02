//go:build !darwin && !windows && !linux

package workspace

import (
	"errors"
	"os"
)

func renameExclusive(root *os.Root, oldPath, newPath string) error {
	// 未实现原生独占重命名的平台保守拒绝操作，不能退化成可能覆盖文件的 os.Rename。
	return errors.New("此平台暂不支持安全的独占重命名")
}
