# Leafmark · 叶笺

面向 macOS / Windows 的轻量本地 Markdown 编辑器，支持原位编辑、阅读模式、多标签和本地文件夹导航。

官网：<https://kevinxc5.github.io/leafmark/>

界面使用 TypeScript 与 CodeMirror 6，本地文件与系统能力由 Go 提供，MyGo 负责原生窗口和前后端通信。Bun 用于开发阶段的依赖管理、命令执行和前端测试，Vite 提供前端热更新与资源构建。技术分工与运行流程见[架构与开发](docs/架构与开发.md)。

## 功能

- 文档管理：多标签、本地文件打开、保存、另存为、独立撤销历史、自动保存与草稿恢复。
- 文件导航：文件夹树、名称筛选、近期文件、新建和重命名。
- 编辑：原位 Markdown 编辑、源码模式、查找替换、表格编辑、链接与图片插入。
- 阅读与输出：阅读模式、代码高亮、脚注、公式、提示块、上下标、Mermaid、HTML / Markdown 导出与系统打印 PDF。
- 个性化：字体、字号、行高、阅读宽度、浅色 / 深色 / 系统主题、专注模式、打字机模式和快捷键自定义。

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

## 本地开发

开发环境需要 Go 1.27.1+ 和 Bun 1.4.2+。运行已打包的应用无需安装这些开发工具。

安装依赖并启动桌面开发应用：

```bash
bun install --frozen-lockfile
bun run dev
```

仅启动网页开发服务：

```bash
bun run dev:web
```

桌面开发支持前端热更新。真实本地文件和系统能力通过桌面应用验证。

## 验证与构建

日常检查包括 TypeScript 类型检查、前端测试和 Go 测试：

```bash
bun run check
```

原生自动化验证：

```bash
bun run verify:native
```

原生验证在 macOS WKWebView 中测试编辑、磁盘保存、多标签和自动保存等场景，完成后自动退出。平台覆盖与人工验收范围见[架构与开发](docs/架构与开发.md#验证覆盖与验收边界)。

构建当前系统与架构的应用，跳过 DMG 和公证：

```bash
bun run build:local
```

产物位于 `build/<平台>-<架构>/`。

发布检查与多平台构建：

```bash
bun run check:release
bun run build -- -platform darwin/universal,windows/amd64,windows/arm64
```

## 许可证

Leafmark 使用 [MIT 许可证](LICENSE)。可以自由使用、修改、分发和商用，但必须保留版权声明和许可声明；软件按“原样”提供，不提供担保。

MyGo、mygo-cli 和 mygo-runtime 也是 MIT。界面使用的 Inter、Newsreader 和 Geist Mono 为 SIL Open Font License 1.1。DOMPurify 为 MPL-2.0 或 Apache-2.0，elkjs 为 EPL-2.0。再分发安装包时须同时提供 [第三方声明](THIRD_PARTY_NOTICES.md)，并保留各组件自己的版权与许可证。

## 下载与自动更新

正式版本从 [GitHub Releases](https://github.com/KevinXC5/leafmark/releases) 下载。macOS 的 Apple 芯片使用 darwin-arm64 DMG，Intel 芯片使用 darwin-amd64 DMG；Windows 根据架构选择 amd64 或 arm64 的 Setup 安装程序。

正式版启动十秒后检查更新，持续运行时每二十四小时检查一次。发现新版本后可选择安装或稍后处理；也可在“设置 → 通用 → 软件更新”查看当前版本、手动检查和安装。更新包经过 Ed25519 签名校验，安装后下次启动生效。请保存文档后正常关闭应用，再重新打开。

开发构建和安装目录不可写时禁用自动更新。当前安装包未配置 Apple Developer ID 公证与 Windows Authenticode 签名，首次运行时操作系统可能显示来源验证提示。

## 项目结构

```text
main.go                 Go 启动入口
internal/desktop/       MyGo 服务、原生对话框、窗口生命周期与原生验证
internal/documents/     文档、标签、保存基线、编码与外部修改检测
internal/workspace/     工作区目录树、路径授权与近期文件
internal/assets/        图片验证、读取与文档附件导入
src/main.ts             前端入口
src/app/                应用装配、主界面模板与全局样式
src/editor/             编辑操作、文档统计与源码高亮
src/editor/live/        Markdown、代码块、公式与表格的原位渲染
src/editor/dialogs/     链接、图片与表格编辑对话框
src/markdown/           阅读与导出渲染、Mermaid、扩展语法
src/documents/          文档会话、草稿同步与恢复快照
src/file-browser/       工作区文件树与近期文档
src/settings/           应用偏好与快捷键设置
src/platform/mygo.ts    MyGo 自动生成的前后端接口
src/types/              第三方模块的类型声明
scripts/                构建、预览与浏览器验证脚本
site/                   官网静态页面，由 GitHub Pages 发布
tests/                  按源码功能目录组织的前端测试与基准测试
verification/           原生验证输入与运行输出
```

前端通过生成接口调用桌面服务，桌面服务调用 Go 业务包。职责边界、接口生成与构建流程见[架构与开发](docs/架构与开发.md)。开发与验证约定见 [AGENTS.md](AGENTS.md)。

## 离线预览

生成包含脚本、样式、字体和 Mermaid 的单文件界面预览：

```bash
bun run preview:export
```

产物为根目录的 `Leafmark-preview.html`，可直接打开。预览文件和 `verification/` 中的运行输出不纳入版本库。
