package desktop

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"leafmark/internal/documents"
)

const welcome = "# Make room for a thought.\n\nA quiet afternoon. A simple document.\nLet your words keep pace with your mind.\n\n> Writing turns a passing thought into something clear.\n\n## Start with something small\n\nOpen a Markdown file and pick up where you left off. Headings, lists and quotes take shape as you type, right here in your document.\n\nMake space for **what matters**.\n\n### Three things for today\n\n- [ ] Capture a sudden idea\n- [ ] Write the first paragraph\n- [x] Leave a thought for tomorrow\n\n```javascript\nconst thought = 'Start here';\nwrite(thought);\n```\n\n## 写下一点想法\n\n中文、English 和 emoji 🌿 都可以在这里书写。选中一段文字，试试加粗或撤销。\n"

// Run 装配桌面服务、创建主窗口并运行应用。
func Run() {
	workspace := NewWorkspace()
	files := &Files{store: documents.NewStore("A small thought.md", welcome), workspace: workspace}
	prepareVerification(files)
	mygo.Bind(files)
	mygo.Bind(workspace)
	mygo.Bind(NewAssets(files))
	updates := &Updates{}
	mygo.Bind(updates)
	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title: "Leafmark · 叶笺", URL: "/", Width: 1200, Height: 900,
			MinWidth: 760, MinHeight: 560, StateKey: "main",
			TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: 52,
			BackgroundColor: "light-dark(#faf9f5, #222426)",
			Vibrancy:        mygo.VibrancySidebar, AutoHideMenuBar: true,
		}
		if runtime.GOOS == "darwin" {
			opts.TrafficLightPosition = &mygo.Point{X: 20, Y: 18}
		}
		win := mygo.NewWindow(opts)
		startVerification(win, files)
		installCloseHandler(win, files)
		updates.start()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
