// makensis 仅为项目构建切换 NSIS 压缩器，不修改生成脚本或系统工具。
// 由 scripts/build-desktop.sh 编译到临时目录并置于 PATH 最前，替 MyGo 调用真实的 makensis。
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var setCompressor = regexp.MustCompile(`(?im)^[ \t]*SetCompressor\b[^\r\n]*`)

// useZlib 把脚本中的压缩器指令替换为 zlib，并返回替换次数；其他字节保持不变。
func useZlib(script []byte) ([]byte, int) {
	count := len(setCompressor.FindAllIndex(script, -1))
	return setCompressor.ReplaceAllLiteral(script, []byte("SetCompressor zlib")), count
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	real := os.Getenv("LEAFMARK_MAKENSIS_REAL")
	if real == "" {
		real = "/opt/homebrew/bin/makensis"
	}
	if !isRealTool(real) {
		fmt.Fprintln(os.Stderr, "[leafmark] 真实 makensis 路径无效，请设置 LEAFMARK_MAKENSIS_REAL。")
		return 1
	}

	cmd := exec.Command(real)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	var script []byte
	scriptIndex, hasStdin, noCD := -1, false, false
	for i, arg := range args {
		if scriptIndex < 0 && strings.HasSuffix(strings.ToLower(arg), ".nsi") {
			if info, err := os.Stat(arg); err == nil && info.Mode().IsRegular() {
				scriptIndex = i
			}
		}
		hasStdin = hasStdin || arg == "-"
		noCD = noCD || strings.EqualFold(arg, "-NOCD")
	}
	args = append([]string(nil), args...)
	switch {
	case scriptIndex >= 0:
		path, err := filepath.Abs(args[scriptIndex])
		if err == nil {
			script, err = os.ReadFile(path)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "[leafmark] 读取 NSIS 脚本失败：", err)
			return 1
		}
		// NSIS 读取文件时默认切换到脚本目录；改用 stdin 后保留该相对路径语义。
		if !noCD {
			cmd.Dir = filepath.Dir(path)
		}
		args[scriptIndex] = "-"
	case hasStdin:
		var err error
		if script, err = io.ReadAll(os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, "[leafmark] 读取 NSIS 脚本失败：", err)
			return 1
		}
	default:
		// 版本查询等命令原样转发，供 MyGo 检测编译器与签名能力。
		cmd.Args = append(cmd.Args, args...)
		return exitCode(cmd.Run())
	}

	// 命令行 /X 的压缩选择会被脚本覆盖，直接替换指令并保留其他字节。
	script, count := useZlib(script)
	if count > 0 {
		fmt.Fprintln(os.Stderr, "[leafmark] NSIS 使用 zlib 压缩构建安装包。")
	}
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin = bytes.NewReader(script)
	return exitCode(cmd.Run())
}

// isRealTool 确认路径是普通文件且不是本程序自身，避免递归调用。
func isRealTool(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	self, err := os.Executable()
	if err != nil {
		return true
	}
	selfInfo, err := os.Stat(self)
	return err != nil || !os.SameFile(info, selfInfo)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return exit.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "[leafmark] 运行 makensis 失败：", err)
	return 1
}
