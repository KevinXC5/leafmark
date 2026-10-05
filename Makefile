# Leafmark 的开发入口。全部命令只依赖 Go 工具链；MyGo 命令行由 go.mod 的 tool 指令固定版本。

.PHONY: dev build build-local check check-release verify shots doctor

# 启动桌面开发应用，Go 代码变化后重新编译。
dev:
	go tool mygo dev

# 发布构建，例如：make build ARGS="-platform darwin/arm64,windows/amd64"
build:
	bash scripts/build-desktop.sh $(ARGS)

# 只构建当前系统与架构，跳过 DMG 和公证。
build-local:
	bash scripts/build-desktop.sh -skip-dmg -skip-notarize

# 全部 Go 测试；桌面装配的测试用 desktoptest 标签换掉排版实现。
check:
	go test ./...
	go test -tags desktoptest ./internal/desktop

# 发布前检查：在 check 之上增加竞态检测、静态检查和验证构建标签。
check-release: check
	go test -race ./...
	go test -race -tags desktoptest ./internal/desktop
	go vet ./...
	go vet -tags desktoptest ./internal/desktop
	go vet -tags verification ./internal/desktop

# 在真实原生窗口中验收，结果写入 verification/。
verify:
	go run -tags verification .

# 用示例文档重新截取官网的应用截图。
shots:
	bash scripts/site-shots.sh

doctor:
	go tool mygo doctor
