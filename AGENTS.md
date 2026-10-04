# Leafmark AI 开发规范

本文件规定 AI 的开发、验证和提交行为。产品说明、架构与开发流程见 [README.md](README.md)。

## 执行约定

- 项目操作以本文件为主，用户明确要求优先于项目默认约定；工具、技能和插件的默认流程仅作补充。不可覆盖的系统级指令仍须遵守，冲突时先说明来源与处理方式。
- 默认在当前分支和工作目录开发、提交，不主动创建或切换分支。仅在用户明确指定或在计划中选择时使用独立 worktree；在其分配目录内操作，不切换共享目录的分支。高优先级指令要求另建分支时，先说明冲突。
- 保留已有及其他 Agent 的改动，不恢复、覆盖无关文件。提交按顺序执行，默认只暂存本任务文件；用户要求统一提交时包含全部改动。
- 优先使用命令行、Bun 测试和原生自动化；仅在无法覆盖原生对话框、系统菜单或输入法时使用 computer use。

## 开发与构建

- 日常查看桌面效果运行 `bun run dev`，由 MyGo 启动原生应用、Vite 热更新；前端修改无需重新打包。
- 本机独立应用运行 `bun run build:local`，默认仅构建当前系统与架构，跳过 DMG 和公证，统一复用 `build/<平台>-<架构>/`，不为验证批次创建 `fast-check`、`refinement` 等输出目录。发布构建使用 `bun run build -- -platform ...`，仅在交付对应产物时构建多架构、跨平台或安装包；macOS 按架构分别构建，仅在用户明确要求时生成通用包。
- `build:web` 包含类型检查，桌面构建也会调用它，不连续重复执行 `typecheck`、`build:web` 和桌面构建。仅在本轮成功生成 `dist/` 且之后未修改前端、入口、资源或配置时，使用 `-skip-build-command`；`.verification-web/` 不能替代 `dist/`。
- 用户窗口有未保存文档时，不强制终止或重启；使用独立验证数据。确需避开正在使用的应用包时，构建到系统临时目录，验证完成后清理临时产物。

## 测试与界面验收

- 开发中运行相关测试，例如 `bun test tests/file-browser/file-browser.test.ts`、`go test ./internal/documents`；完成后运行 `bun run check`，覆盖类型检查、前端全量测试和 Go 测试。通过后无新增修改或未解决问题，不重复运行。
- 并发、锁和文件生命周期变更运行相关包的 `go test -race`；发布前运行 `bun run check:release`，覆盖竞态、`go vet` 和验证构建标签。保留有效回归测试，尤其是渲染、安全转义、附件授权、路径边界、磁盘覆盖、撤销重做、多标签草稿及恢复测试；基准测试仅在调查性能时运行。
- 应用布局、原位渲染、侧栏、多标签、保存及本地文件能力优先用 `bun run verify:native` 验收。它构建验证前端并运行 `go run -tags verification .`，通过 MyGo 的 `win.Page().EvalContext()`、`win.CapturePage()` 在真实 macOS WKWebView 中断言、截图并退出；数据位于 `.verification-data/` 和 `verification/`，不得修改用户笔记。
- 验收须读取 `verification/native-results.json` 确认 `passed`，查看 `verification/native-window.png` 核对视觉。新界面行为补充针对性的原生断言；Markdown 问题用真实原文的只读副本检查源码、渲染结构和视觉，并说明不支持的语法。
- 原生验证入口位于 `internal/desktop/verification_enabled.go`，正式构建由 `verification_disabled.go` 排除；前端入口仅在 Vite `verification` 模式启用。原生验证按运行环境覆盖 macOS WKWebView 或 Windows WebView2，须核对结果中的 `platform` 与 `arch`；不能由 macOS 结果推断 Windows 已验收，也不能由 Windows amd64 结果推断 ARM64 已验收。原生文件对话框、输入法、安装、真实拖放与升级重启须单独验收，不能将 Bun 测试或浏览器截图称为原生验证。
- `tests/fixtures/samples/` 的 `山中来信.md` 与 `叶脉笔记.md` 是统一的示例文档：需要完整文档做渲染测试、视觉核对或官网截图时使用它们，不另造示例。官网截图用 `bun run shots:site` 生成，修改示例文档或影响截图的界面后重新运行，并查看 `verification/site-shots/` 下的原图核对视觉。
- `scripts/verify-browser.mjs` 仅按需检查网页预览、浏览器特有行为或原生入口难以定位的问题，调用前遵守 ego-browser Skill；不默认重复执行与原生验证相同的网页全量检查。

## 代码与文档

- 前端按功能放入 `src/app/`、`src/editor/`、`src/markdown/`、`src/documents/`、`src/file-browser/`、`src/settings/`；原位渲染放入 `src/editor/live/`，编辑对话框放入 `src/editor/dialogs/`，专用 CSS 与模块同目录，第三方类型声明放入 `src/types/`。避免在 `src/` 根目录平铺功能文件。
- 测试按功能放入 `tests/`，应用生命周期集成测试放入 `tests/app/`，基准测试与所属模块测试同目录。迁移模块时同步导入、测试源码路径、脚本入口和文档引用。
- `src/main.ts` 是公共入口，加载 `src/app/bootstrap.ts`，供 HTML 页面及离线预览脚本共用。`src/platform/mygo.ts` 是生成接口，输出由 `mygo.config.ts` 的 `bindings` 指定；修改 Go 导出服务后运行 `bun run generate`，不手工编辑。
- 说明、注释和文档使用简体中文，代码标识符遵循既有约定。文档直接呈现最终内容，不保留讨论或修改过程。
- 文档职责保持清晰：`README.md` 介绍产品、用法、架构和开发流程，`AGENTS.md` 规定 AI 行为，不新增职责重叠的说明文件。原型不纳入仓库，文档和代码注释不得将其作为引用来源或使用前提。
