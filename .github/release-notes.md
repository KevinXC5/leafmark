## 下载与安装

- macOS：Apple 芯片下载 darwin-arm64 DMG，Intel 芯片下载 darwin-amd64 DMG。
- Windows：根据系统架构下载 amd64 或 arm64 的 Setup 安装程序。
- 正式版启动后自动检查更新，也可在“设置 → 通用 → 软件更新”手动检查。更新包通过 Ed25519 签名校验，安装后下次启动生效。
- `update-*.json` 和 `.tar.gz` 文件供自动更新使用；`checksums.txt` 提供发布文件的 SHA-256 校验值。

当前安装包未配置 Apple Developer ID 公证及 Windows Authenticode 签名，首次运行时系统可能显示来源验证提示。自动更新签名独立于操作系统代码签名。
