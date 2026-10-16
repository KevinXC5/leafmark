package desktop

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"leafmark/internal/documents"
	// 注册中文目录，让 MyGo 的菜单、控件和系统面板使用简体中文。
	_ "leafmark/internal/locales"
)

const welcome = "# Make room for a thought.\n\nA quiet afternoon. A simple document.\nLet your words keep pace with your mind.\n\n> Writing turns a passing thought into something clear.\n\n## Start with something small\n\nOpen a Markdown file and pick up where you left off. Headings, lists and quotes take shape as you type, right here in your document.\n\nMake space for **what matters**.\n\n### Three things for today\n\n- [ ] Capture a sudden idea\n- [ ] Write the first paragraph\n- [x] Leave a thought for tomorrow\n\n```javascript\nconst thought = 'Start here';\nwrite(thought);\n```\n\n## 写下一点想法\n\n中文、English 和 emoji 🌿 都可以在这里书写。选中一段文字，试试加粗或撤销。\n"

// Run 装配原生桌面窗口并运行应用。文档服务仍复用 Files、Workspace 与 Assets，
// 窗口内容由 MyGo 自己绘制，不再绑定网页，也不经 Page 或 Eval 读写编辑器。
func Run() {
	workspace := NewWorkspace()
	files := &Files{store: documents.NewStore("A small thought.md", welcome), workspace: workspace}
	prepareVerification(files)
	updates := &Updates{}
	assets := NewAssets(files)
	app := newNativeApp(files, workspace, assets, updates)
	// 启动前注册，才能收到双击或“打开方式”启动应用时带来的文档。
	// 窗口尚未创建时只排队，创建后由主线程打开。
	mygo.App.OnOpenFile(func(path string) {
		app.enqueueOpen([]string{path})
	})
	mygo.App.WhenReady(func() {
		if err := registerNativeFonts(); err != nil {
			log.Printf("加载原生字体失败：%v", err)
		}
		opts := mygo.WindowOptions{
			Title: "Leafmark · 叶笺", Width: 1200, Height: 900,
			MinWidth: 760, MinHeight: 560, StateKey: "main",
			TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: nativeTitleBarHeight,
			BackgroundColor: "light-dark(" + nativeLightPaper + ", " + nativeDarkPaper + ")",
			Vibrancy:        mygo.VibrancySidebar, AutoHideMenuBar: true,
			Content: ui.View(app.View),
		}
		if runtime.GOOS == "darwin" {
			// 按钮直径 12，顶边 20 让中心落在标题栏中线 26 上。
			opts.TrafficLightPosition = &mygo.Point{X: 20, Y: 20}
		}
		win := mygo.NewWindow(opts)
		app.attach(win)
		if runtime.GOOS == "darwin" {
			mygo.App.SetMenu(nativeApplicationMenu(app))
		}
		win.OnFileDrop(func(e *mygo.FileDropEvent) {
			app.enqueueOpen(e.Paths)
		})
		startNativeVerification(win, app)
		updates.restartFn = installNativeCloseHandler(win, app)
		app.watchUpdates()
		updates.start()
		app.boot()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
