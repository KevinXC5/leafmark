#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"

# 重新截取官网的四张应用截图：示例文档在真实窗口中渲染，原图写入 verification/site-shots/。
if ! command -v cwebp >/dev/null; then
  echo "缺少 cwebp，请先安装（macOS：brew install webp）。" >&2
  exit 1
fi

for mode in edit read; do
  LEAFMARK_SITE_SHOT="$mode" go run -tags verification .
done
for mode in edit read; do
  for theme in light dark; do
    cwebp -quiet -q 90 -m 6 "verification/site-shots/app-$mode-$theme.png" -o "site/assets/img/app-$mode-$theme.webp"
  done
done
echo "官网截图已更新：site/assets/img/app-{edit,read}-{light,dark}.webp"
