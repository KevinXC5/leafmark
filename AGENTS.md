# Leafmark AI 开发规范

本文件规定 AI 的开发、验证和提交行为。产品说明、架构与开发流程见 [README.md](README.md)。

## 执行约定

- 项目操作以本文件为主，用户明确要求优先于项目默认约定；工具、技能和插件的默认流程仅作补充。不可覆盖的系统级指令仍须遵守，冲突时先说明来源与处理方式。
- 默认在当前分支和工作目录开发、提交，不主动创建或切换分支。仅在用户明确指定或在计划中选择时使用独立 worktree；在其分配目录内操作，不切换共享目录的分支。高优先级指令要求另建分支时，先说明冲突。
- 保留已有及其他 Agent 的改动，不恢复、覆盖无关文件。提交按顺序执行，默认只暂存本任务文件；用户要求统一提交时包含全部改动。
- 优先使用命令行、Go 测试和原生窗口验收；仅在无法覆盖原生对话框、系统菜单或输入法时使用 computer use。

## 开发与构建

- 整个项目只有 Go 代码，不需要 Node.js 或 Bun。命令入口是 `Makefile`；MyGo 命令行由 `go.mod` 的 `tool` 指令固定版本，通过 `go tool mygo` 调用。应用名称、文件关联与更新源在 `mygo.json` 中配置。正式版本号取自发布标签，发布流程在构建前把它写入 `mygo.json`；仓库里的 `version` 只用于本机构建，发版时不需要修改。
- 桌面应用由 `desktop.Run` 启动，窗口内容是 MyGo GPU 绘制的原生界面，不加载网页。编辑器在 `internal/nativeeditor`，文档模型在 `internal/richtext`，公式排版在 `internal/mathlayout`，流程图在 `internal/diagram`，窗口装配在 `internal/desktop`。
- 日常查看桌面效果运行 `make dev`。它执行 `go tool mygo dev`，Go 代码变化后重新编译。
- 本机独立应用运行 `make build-local`，默认仅构建当前系统与架构，跳过 DMG 和公证，统一复用 `build/<平台>-<架构>/`，不为验证批次创建 `fast-check`、`refinement` 等输出目录。发布构建使用 `make build ARGS="-platform ..."`，仅在交付对应产物时构建多架构、跨平台或安装包；macOS 按架构分别构建，仅在用户明确要求时生成通用包。
- 不要重新引入网页前端、`package.json` 或以网页为入口的构建步骤。唯一借用系统网页引擎的地方是导出 PDF：`renderPDF` 在不可见窗口里加载导出的 HTML 并打印，不得把它扩展成界面的一部分。
- 用户窗口有未保存文档时，不强制终止或重启；使用独立验证数据。确需避开正在使用的应用包时，构建到系统临时目录，验证完成后清理临时产物。

## 测试与界面验收

- 开发中运行相关测试，例如 `go test ./internal/nativeeditor`、`go test ./internal/richtext`、`go test ./internal/documents`。`internal/desktop` 的界面装配测试带构建标签，须用 `go test -tags desktoptest ./internal/desktop` 运行，不带标签时这些测试不会执行。完成后运行 `make check`。通过后无新增修改或未解决问题，不重复运行。
- 并发、锁和文件生命周期变更运行相关包的 `go test -race`；发布前运行 `make check-release`，覆盖竞态、`go vet` 和验证构建标签。保留有效回归测试，尤其是原文回写、附件授权、路径边界、磁盘覆盖、撤销重做、多标签草稿及恢复测试；基准测试仅在调查性能时运行。
- 应用布局、所见即所得排版、侧栏、多标签、保存、设置及本地文件能力用 `make verify` 验收。它执行 `go run -tags verification .`，启动真实 GPU 原生窗口。数据位于 `.verification-data/` 和 `verification/`，不得修改用户笔记。
- 验收须读取 `verification/native-results.json` 确认 `passed`，并核对 `platform` 与 `arch`，再查看 `verification/` 下的 `native-window.png`、`native-format-bar.png`、`native-source-mode.png` 和 `native-settings-*.png`。新界面行为补充针对性的原生断言。不能由 macOS 结果推断 Windows 已验收，也不能由 Windows amd64 结果推断 ARM64 已验收。
- `ui.Tester` 驱动的 Go 测试不是原生窗口验证。原生文件对话框、右键菜单、输入法、安装、真实拖放与升级重启须单独人工验收，不能把 `ui.Tester` 的结果称为原生验证。`ui.Tester` 按文字或名称查找元素时取第一个匹配项，可点击控件的名称要与同页的说明文字区分开。
- 原生验证入口位于 `internal/desktop/verification_enabled.go`，正式构建由 `verification_disabled.go` 排除。验证只操作原生界面，不通过 `Page().Eval` 之类的网页接口做断言。
- `tests/fixtures/samples/` 的 `山中来信.md` 与 `叶脉笔记.md` 是统一的示例文档：需要完整文档做渲染测试或视觉核对时使用它们，不另造示例。官网截图用 `make shots`，它在真实原生窗口中打开示例副本并截图；调整影响截图的界面后重新生成，查看 `verification/site-shots/` 的原图核对视觉。截图不能替代编辑、保存和文件生命周期断言。
- 公式与流程图的视觉核对可用离屏渲染：`MATHLAYOUT_PNG=<路径> go test ./internal/mathlayout -run TestRenderSamples` 与 `DIAGRAM_PNG=<路径> go test ./internal/diagram -run TestPaintSamples`。

## 代码与文档

- 桌面代码放在 `internal/desktop/`、`internal/nativeeditor/`、`internal/richtext/`、`internal/mathlayout/`、`internal/diagram/`。文档、工作区和图片规则放在 `internal/documents/`、`internal/workspace/`、`internal/assets/`，这些包不依赖 MyGo。`mathlayout` 与 `diagram` 只把源码排成可绘制的结果，不持有文档状态。
- `internal/desktop` 按职责分文件：`native_app.go` 是状态与命令，`native_visual.go` 是书写界面，`native_settings.go` 与 `native_shortcuts.go` 是设置页与快捷键，`native_features.go` 是源码模式、工作区操作与导出，`native_lifecycle.go` 是视图入口与关闭流程，`native_recovery.go` 是设置与草稿的持久化。新功能放进对应文件，不在根目录平铺。
- 界面的尺寸、颜色、圆角与文案是既定设计，修改界面时保持现有数值与观感一致，不用控件库的默认外观替代。图标取自 Lucide，以内嵌 SVG 的形式放在 `nativeIcons`。
- 即时模式下元素一经创建就进入界面树：需要在行与列之间切换时，在同一个元素上调用 `.Row()`，不要先建一个再用另一个覆盖变量。`ui.Button(...).Key()` 会 panic，`Key` 放在外层容器上。
- 测试放入对应 Go 包。迁移模块时同步测试、脚本入口和文档引用。
- 说明、注释和文档使用简体中文，代码标识符遵循既有约定。文档直接呈现最终内容，不保留讨论或修改过程。
- 文档职责保持清晰：`README.md` 介绍产品、用法、架构和开发流程，`AGENTS.md` 规定 AI 行为，不新增职责重叠的说明文件。原型不纳入仓库，文档和代码注释不得将其作为引用来源或使用前提。
- 描述桌面能力时以原生实现及测试为准。公式为常用的 TeX 数学子集，HTML 为语义标签子集，Mermaid 为流程图、时序图、甘特图、饼图、类图、状态图和 ER 图的常用语法；不得宣称完整 LaTeX、任意 HTML/CSS 或全部 Mermaid 语法兼容。扩展语法须覆盖原文回写、结构编辑与撤销重做；新增 SVG 导出须同时核对原生绘制和离线导出，涉及中文字体回退时明确平台差异。
