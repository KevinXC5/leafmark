# Leafmark · 叶笺

面向 macOS / Windows 的轻量本地 Markdown 编辑器，支持原位编辑、阅读模式、多标签和本地文件夹导航。

官网：<https://kevinxc5.github.io/leafmark/>

## 下载

从[官网](https://kevinxc5.github.io/leafmark/)或 [GitHub Releases](https://github.com/KevinXC5/leafmark/releases) 下载：macOS 按芯片选择 arm64 或 amd64 的 DMG，Windows 按架构选择 Setup 安装程序。

- 应用会自动检查更新，也可在“设置 → 通用 → 软件更新”手动检查；更新包经过签名校验，安装后下次启动生效。
- 安装包尚未配置 Apple 公证与 Windows 代码签名，首次运行时系统可能显示来源验证提示。

## 功能

- 文档管理：多标签、本地文件打开、保存、另存为、独立撤销历史、自动保存与草稿恢复。
- 文件导航：文件夹树、名称筛选、近期文件、新建和重命名。
- 编辑：原位 Markdown 编辑、源码模式、查找替换、表格编辑、链接与图片插入。
- 阅读与输出：阅读模式、代码高亮、脚注、公式、提示块、上下标、Mermaid、HTML / Markdown 导出与系统打印 PDF。
- 个性化：字体与版面、浅色 / 深色主题、专注模式、打字机模式和快捷键自定义。

## 使用说明

### 文档与文件夹

- 每个文档是一个独立标签，各自保留内容、选区和撤销历史；标签上的圆点表示有未保存的修改。
- 在侧栏“文档”页打开一个文件夹作为工作区，可浏览、筛选、新建和重命名其中的 Markdown 文件。
- 文件被其他程序修改后，自动保存会暂停，可从文件菜单重新加载。

### 编辑

- **原位编辑**：Markdown 直接渲染在正文里，光标所在行显示语法标记；随时可切到源码模式查看完整 Markdown。
- **格式**：选中文字后出现浮动工具栏，完整操作在格式菜单；侧栏大纲按标题生成，点击跳转。
- **表格**：以真实表格显示，双击进入编辑器，可增删、移动行列和调整对齐。
- **代码块与公式**：代码块按语言高亮并可一键复制，`$…$` 与 `$$` 公式原位显示。
- **图片**：支持 URL 和本地文件；已保存文档的本地图片会复制到 `./assets/<文档名>/`。

### 阅读与导出

- 阅读模式按 CommonMark / GFM 渲染，并支持脚注、公式、提示块、高亮、上下标、折叠内容和 Mermaid 图形。
- 可导出离线可打开的 HTML 和 Markdown 副本；PDF 通过系统打印生成。

### 自动保存与恢复

- 开启自动保存后，停止输入两秒即保存；未命名文档需先另存为。
- 意外退出后，未保存的内容可恢复为新标签。

### 快捷键

快捷键可在设置中修改。`Mod` 在 macOS 上为 ⌘，在 Windows 上为 Ctrl。

| 动作 | 默认快捷键 |
| --- | --- |
| 新建 / 打开 | Mod+N / Mod+O |
| 保存 / 另存为 | Mod+S / Mod+Shift+S |
| 关闭标签 | Mod+W |
| 查找替换 | Mod+F |
| 加粗 / 斜体 | Mod+B / Mod+I |
| 链接 | Mod+K |
| 阅读切换 | Mod+Shift+R |
| 设置 | Mod+, |

### 已知限制

- 文档超过约一百万字符时，代码块和表格以源码显示。
- 工作区不显示隐藏文件、`node_modules` 和符号链接。
- 多个程序同时写入同一文件时，仍可能互相覆盖。
- Windows 版本已完成构建，尚未实机验证。

## 开发

### 技术栈

| 技术 | 职责 |
| --- | --- |
| TypeScript、CodeMirror 6 | 界面与编辑器，在系统 WebView 中运行 |
| Go | 本地文件读写、工作区、图片与路径校验 |
| MyGo | 创建原生窗口，连接前端与 Go 服务，负责桌面打包与签名更新 |
| Vite | 前端热更新与资源构建 |
| Bun | 依赖管理、命令入口和前端测试 |

应用没有远程服务端，运行已打包的应用无需安装任何开发工具。

### 环境与命令

需要 Go 1.27.1+ 和 Bun 1.4.2+。

| 命令 | 用途 |
| --- | --- |
| `bun install --frozen-lockfile` | 按锁文件安装依赖 |
| `bun run dev` | 启动桌面开发应用，前端热更新 |
| `bun run dev:web` | 仅启动网页开发服务；本地文件与系统能力需在桌面应用中验证 |
| `bun run generate` | 修改 Go 导出服务后，重新生成 `src/platform/mygo.ts` |
| `bun run check` | 类型检查、前端测试和 Go 测试 |
| `bun run check:release` | 在 `check` 之上增加竞态检测、`go vet` 和验证构建标签 |
| `bun run verify:native` | 在真实 macOS WKWebView 中自动验收并截图，结果写入 `verification/` |
| `bun run build:local` | 构建当前系统与架构的应用，产物位于 `build/<平台>-<架构>/` |
| `bun run build -- -platform …` | 发布构建，例如 `darwin/universal,windows/amd64,windows/arm64` |
| `bun run preview:export` | 生成单文件离线界面预览 `Leafmark-preview.html` |

### 项目结构

```text
main.go                 Go 启动入口
internal/desktop/       MyGo 服务、原生对话框、窗口生命周期、更新与原生验证
internal/documents/     文档、标签、保存基线、编码与外部修改检测
internal/workspace/     工作区目录树、路径授权与近期文件
internal/assets/        图片验证、读取与文档附件导入
src/main.ts             前端入口，加载 src/app/bootstrap.ts
src/app/                应用装配、主界面模板与全局样式
src/editor/             编辑操作、文档统计与源码高亮
src/editor/live/        Markdown、代码块、公式与表格的原位渲染
src/editor/dialogs/     链接、图片与表格编辑对话框
src/markdown/           阅读与导出渲染、Mermaid、扩展语法
src/documents/          文档会话、草稿同步与恢复快照
src/file-browser/       工作区文件树与近期文档
src/settings/           应用偏好与快捷键设置
src/platform/mygo.ts    MyGo 自动生成的前后端接口，不手工编辑
tests/                  前端测试，目录与 src/ 对应
scripts/                构建、预览与浏览器验证脚本
site/                   官网静态页面
design/                 设计源文件（logo 高清原图），不随安装包分发
.github/workflows/      版本发布与官网发布流程
```

职责边界：

- 前端只通过 `src/platform/mygo.ts` 调用 `internal/desktop` 中的服务；业务逻辑放在 `documents`、`workspace`、`assets` 三个业务包，它们不依赖 MyGo。
- 保存基线和磁盘内容以 Go 的文档状态为准；前端负责编辑器状态、选区、撤销历史和草稿同步。
- 文件访问必须经过授权：相对图片以已打开文档的 ID 为入口，工作区路径由工作区包校验，不接受前端传入的任意绝对路径。

### 实现要点

- **保存**：写入同目录临时文件后原子替换，保留原文件的 BOM、CRLF 和权限；保存前校验磁盘内容摘要，发现外部修改即暂停自动保存。
- **渲染安全**：原始 HTML 按文本处理，渲染结果经 DOMPurify 净化，链接拒绝危险协议；Mermaid 使用严格模式并限制代码规模。
- **限额**：工作区最多扫描 10,000 个条目和 64 层目录；恢复草稿单份 1 MiB、总量 5 MiB；近期文件保留 30 条；文档超过 1,000,000 个 UTF-16 代码单元时，代码块与表格回退为源码显示。

### 测试与验收

- 前端测试覆盖编辑、渲染安全、表格、会话、草稿恢复、设置与文件导航；Go 测试覆盖文档状态、编码、文件操作、路径授权与图片处理。
- 原生验证覆盖中文与 emoji 输入、撤销重做、磁盘保存、多标签草稿、自动保存、工作区、相对图片和外部修改检测。
- 以下内容需人工验收：输入法组合输入、系统打印、原生文件选择框，以及 Windows 实机运行。

### 发布

`package.json` 的 `version` 是版本号的唯一来源。更新版本号并提交后，推送同名标签即可发布：

```bash
git tag v0.2.0
git push origin main v0.2.0
```

`.github/workflows/release.yml` 的流程：

1. **准备**：核对标签与版本号，用 DeepSeek 把上一版本以来的提交说明整理成更新日志，创建发布草稿。
2. **检查与构建**（并行）：运行 `check:release`；同时在 macOS 构建两个架构的 DMG，在 Ubuntu 构建两个架构的 Windows 安装程序，上传到草稿。
3. **发布**：检查与构建都通过后，核对更新清单、生成 `checksums.txt` 并公开发布。

- 需要两个 Actions Secret：`DEEPSEEK_API_KEY` 和 `MYGO_UPDATER_PRIVATE_KEY`（更新包的 Ed25519 签名私钥，公钥在 `mygo.config.ts` 中）。
- 应用内的更新提示取自更新日志开头的概述，由构建任务写入 `.github/release-notes.md` 的 `## 版本号` 小节；仓库中的该文件只保存固定的下载与安装说明。
- 已公开的版本不会被覆盖，对它重新运行流程会直接成功结束。流程也可以手动触发，用于重试尚未公开的标签。

### 官网

官网源码在 `site/`，是没有依赖和构建步骤的静态页面，`site/` 有变化时由 `.github/workflows/pages.yml` 发布到 GitHub Pages。本地预览：

```bash
python3 -m http.server 4173 -d site
```

- 版本号和安装包直链在浏览器中从最新 Release 读取，发布新版本后无需修改官网；安装包文件名需保持 `darwin-arm64.dmg`、`windows-amd64.exe` 这类结尾。
- `site/assets/img/app-*.webp` 是应用窗口内容在 1280×820、二倍像素密度下的截图，界面改版后重新截取；左上角的窗口控制按钮由样式补画。

## 许可证

Leafmark 使用 [MIT 许可证](LICENSE)。可以自由使用、修改、分发和商用，但必须保留版权声明和许可声明；软件按“原样”提供，不提供担保。

MyGo、mygo-cli 和 mygo-runtime 也是 MIT。界面使用的 Inter、Newsreader 和 Geist Mono 为 SIL Open Font License 1.1。DOMPurify 为 MPL-2.0 或 Apache-2.0，elkjs 为 EPL-2.0。再分发安装包时须同时提供 [第三方声明](THIRD_PARTY_NOTICES.md)，并保留各组件自己的版权与许可证。
