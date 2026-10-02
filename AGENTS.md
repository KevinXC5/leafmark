# Leafmark AI 开发规范

本文件约束 AI 在本项目中的代码与文档修改、构建和验证行为。产品功能与操作说明见 [README.md](README.md)，技术分工与实现边界见[架构与开发](docs/架构与开发.md)。

## 开发与构建

- 日常查看桌面效果使用 `bun run dev`。MyGo 启动原生开发应用，前端通过 Vite 热更新；仅修改样式或前端逻辑时无需重新打包。
- 需要可独立启动的本机应用时运行 `bun run build:local`。默认只构建当前系统与架构，跳过 DMG 和公证；产物位于 `build/<平台>-<架构>/`。
- 发布构建使用 `bun run build -- -platform ...`。多架构、跨平台和安装包生成只在交付对应产物时运行。
- `build:web` 已包含类型检查，桌面构建也会调用它。不要在同一轮紧接着重复执行 `typecheck`、`build:web` 和桌面构建。
- 仅在本轮已经成功生成且之后没有修改前端、入口、资源或配置时，才可给桌面构建传 `-skip-build-command` 复用 `dist/`。验证前端 `.verification-web/` 不能代替发布前端 `dist/`。
- 用户窗口存在未保存文档时，保留原窗口，不强制终止或重启；验证使用独立数据，必要时构建到单独输出目录。

## 测试选择

- 优先使用命令行、Bun 测试与原生自动化。仅在这些方式不能覆盖原生对话框、系统菜单或输入法等行为时使用 computer use，减少耗时的桌面操作。
- 开发过程中根据改动运行相关测试，例如 `bun test tests/file-browser/file-browser.test.ts` 或 `go test ./internal/documents`。有效回归测试即使很短也保留，不能以减少数量代替缩短执行时间。
- 一轮工作完成后运行 `bun run check`：类型检查、前端全量测试和 Go 测试。通过后，没有新增修改或未解决问题就不重复运行。
- 并发、锁、文件生命周期变更运行相关包的 `go test -race`；发布前运行 `bun run check:release`，覆盖竞态、`go vet` 和验证构建标签。
- 渲染、安全转义、附件授权与路径边界、磁盘覆盖、撤销重做、多标签草稿和恢复等测试必须保留。基准测试只在调查性能时手动运行。

## 原生界面验证

- 应用内的布局、原位渲染、侧栏、多标签、保存和本地文件能力优先运行 `bun run verify:native`，不要默认再执行覆盖相同场景的网页全量验证。
- Bun 负责执行命令；真正的原生自动化由 MyGo 的 `win.EvalContext()` 和 `win.CapturePage()` 提供。不能把 `bun test` 或浏览器截图表述为已验证原生应用。
- `verify:native` 构建未压缩的验证前端，运行 `go run -tags verification .`，在真实 macOS WKWebView 中执行断言、截图并自动退出。验证数据使用 `.verification-data/` 和 `verification/`，不得修改用户原始笔记。
- 原生验证逻辑位于 `internal/desktop/verification_enabled.go`；正式构建通过 `verification_disabled.go` 排除验证入口。前端验证入口只在 Vite 的 `verification` 模式启用。
- 阅读 `verification/native-results.json` 确认 `passed`，并查看 `verification/native-window.png` 核对真实视觉。需要验证新的界面行为时补充有针对性的原生断言；不能仅检查进程启动。
- Markdown 问题使用真实原文的只读副本复现，分别检查源码保留、渲染结构和视觉；不支持的语法要明确报告限制。
- `scripts/verify-browser.mjs` 保留作为按需检查：网页预览、浏览器特有行为，或原生入口难以定位的问题。调用前遵守 ego-browser Skill；不作为原生验证的必跑前置步骤。
- 原生验证目前覆盖 macOS WKWebView，不能据此宣称 Windows、原生文件对话框或输入法组合输入已完成验收。

## 分支与工作区

- 默认在当前检出的分支上开发；个人开发通常使用 `main`，不主动创建功能分支或切换分支。
- 是否使用独立 worktree 由用户在创建计划时勾选，或通过明确指令决定；未选择时使用当前工作目录，不自动创建 worktree。
- 用户选择 worktree 时，在分配的独立目录中工作，不切换共享主目录的分支。
- 多个 Agent 共用工作目录时，保留其他 Agent 的改动；提交按顺序执行，默认只暂存本任务涉及的文件，用户明确要求包含全部改动时再统一提交。
- 用户要求提交时，默认提交到当前分支，不因提交操作另建分支。若工具环境的更高优先级指令要求先建分支，先说明冲突，不擅自切换共享目录的分支。

## 修改边界

- 保留用户已有的工作区改动，不恢复或覆盖无关文件。
- 前端源码按功能放入 `src/app/`、`src/editor/`、`src/markdown/`、`src/documents/`、`src/file-browser/` 和 `src/settings/`；原位渲染扩展放入 `src/editor/live/`，编辑对话框放入 `src/editor/dialogs/`。模块专用 CSS 与代码放在同一目录，第三方类型声明放入 `src/types/`，避免在 `src/` 根目录平铺功能文件。
- 前端测试按源码功能目录放入 `tests/`，应用生命周期集成测试放入 `tests/app/`，基准测试与所属模块测试放在一起。迁移模块时同步导入、测试读取的源码路径、脚本入口及文档引用。
- `src/main.ts` 为公共前端入口，加载 `src/app/bootstrap.ts`；HTML 页面和离线预览脚本共用此入口。
- `src/platform/mygo.ts` 为生成接口，输出路径由 `mygo.config.ts` 的 `bindings` 指定；修改 Go 导出服务后运行 `bun run generate`，不手工编辑生成文件。
- 说明、注释和文档使用简体中文；代码标识符遵循现有约定。
- 文档保持三份职责：`README.md` 介绍产品与使用方式，`docs/架构与开发.md` 说明架构、实现和开发流程，`AGENTS.md` 规定 AI 开发行为。相关内容归入对应文档，避免新增职责重叠的说明文件。
- 文档直接呈现客观、完整的最终内容，不保留问答、用户反馈、修改过程或方案讨论的痕迹。
- 原型文件不纳入仓库，文档和代码注释不得将其作为引用来源或使用前提。
