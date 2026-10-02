#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

# 在加入 wrapper 之前解析真实工具，避免递归调用；PATH 仅影响本次构建。
if [[ -z "${LEAFMARK_MAKENSIS_REAL:-}" ]]; then
  LEAFMARK_MAKENSIS_REAL="$(command -v makensis || true)"
fi
if [[ -n "$LEAFMARK_MAKENSIS_REAL" ]]; then
  export LEAFMARK_MAKENSIS_REAL
  export PATH="$project_root/scripts/tools:$PATH"
fi

# 支持直接传 MyGo 参数，也兼容 bun run build -- -platform ... 的分隔符。
if [[ "${1:-}" == "--" ]]; then
  shift
fi
exec bun "$project_root/node_modules/mygo-cli/bin/mygo.js" build "$@"
