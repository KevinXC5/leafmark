# Leafmark · 叶笺

基于 `Leafmark.pen` 的 macOS / Windows 轻量 Markdown 编辑器。使用 MyGo、Go、TypeScript 和 CodeMirror 6。

## 运行

需要 Go 1.27.1+ 和 Bun 1.4.2+：

```bash
bun install --frozen-lockfile
bun run dev
```

Mac 成品：

```bash
open build/darwin-universal/Leafmark.app
```

## 功能

多标签、本地文件打开／保存／另存为、文件夹树和近期文件、查找替换、原位 Markdown 编辑、独立撤销历史、表格编辑、链接与图片插入、阅读模式、HTML／Markdown 导出和系统打印 PDF。

设置支持字体、字号、行高、阅读宽度、深浅与系统主题、自动保存、草稿恢复、专注／打字机模式和快捷键自定义。阅读模式支持 GFM、代码高亮、脚注、公式、提示块、上下标和 Mermaid。

[功能与使用](docs/功能实现与使用.md)包含操作说明、限制与验证边界。AI 不包含在当前版本。

## 验证和构建

```bash
bun run typecheck
bun test
go test -race ./...
go vet ./...
bun run verify:native
bun run build -- -platform darwin/universal,windows/amd64,windows/arm64
```

原生验证在 macOS WKWebView 中通过程序接口测试编辑、真实磁盘保存、多标签和自动保存，并自动退出。Windows 产物尚未实机运行。中文文本和 emoji 已验证，拼音候选词与组合输入仍需人工验收。

## 预览与记录

- [开发环境与验证](docs/开发环境与验证.md)：已有环境、新安装工具、实际命令和步骤。
- [功能与使用](docs/功能实现与使用.md)：当前功能、快捷键及验证边界。
- [视觉对照](docs/视觉对照.md)：与原型逐项对照的尺寸、配色、字体与已知差异。
- [离线界面预览](Leafmark-preview.html)：脚本、样式、字体和 Mermaid 内嵌的单文件，可直接查看（该文件为生成产物，未纳入版本库）。
- `docs/images/`：主窗口、弹窗、设置与阅读模式的当前界面截图。
- `verification/`：验证输入样例与运行输出，输出文件不纳入版本库。

重新生成单文件预览：

```bash
bun run preview:export
```
