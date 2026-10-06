#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

# 在加入包装器之前解析真实工具，避免递归调用；PATH 仅影响本次构建。
if [[ -z "${LEAFMARK_MAKENSIS_REAL:-}" ]]; then
  LEAFMARK_MAKENSIS_REAL="$(command -v makensis || true)"
fi
if [[ -n "$LEAFMARK_MAKENSIS_REAL" ]]; then
  export LEAFMARK_MAKENSIS_REAL
  # 包装器是 Go 程序，按本机平台编译到临时目录，构建结束后清理。
  tools_dir="$(mktemp -d)"
  trap 'rm -rf "$tools_dir"' EXIT
  GOOS= GOARCH= go build -o "$tools_dir/makensis" ./scripts/tools/makensis
  export PATH="$tools_dir:$PATH"
fi

# MyGo 命令行由 go.mod 的 tool 指令固定版本，参数原样传给 mygo build。
if [[ "${1:-}" == "--" ]]; then
  shift
fi
go tool mygo build "$@"
