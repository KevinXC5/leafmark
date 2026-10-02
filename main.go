package main

import (
	"context"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"leafmark/internal/documents"
)

const welcome = "# Make room for a thought.\n\nA quiet afternoon. A simple document.\nLet your words keep pace with your mind.\n\n> Writing turns a passing thought into something clear.\n\n## Start with something small\n\nOpen a Markdown file and pick up where you left off. Headings, lists and quotes take shape as you type, right here in your document.\n\nMake space for **what matters**.\n\n### Three things for today\n\n- [ ] Capture a sudden idea\n- [ ] Write the first paragraph\n- [x] Leave a thought for tomorrow\n\n```javascript\nconst thought = 'Start here';\nwrite(thought);\n```\n\n## 写下一点想法\n\n中文、English 和 emoji 🌿 都可以在这里书写。选中一段文字，试试加粗或撤销。\n"

// Files 仅提供用户通过原生文件选择框授权的文档操作。
type Files struct {
	store     *documents.Store
	workspace *Workspace
}

func (f *Files) OpenWorkspace(relative string) (documents.Document, error) {
	doc, err := f.workspace.OpenDocument(relative)
	if err != nil {
		return documents.Document{}, err
	}
	return f.store.LoadSnapshot(doc.Path, doc.Raw)
}
func (f *Files) OpenRecent(path string) (documents.Document, error) {
	backend, err := f.workspace.backend()
	if err != nil {
		return documents.Document{}, err
	}
	doc, err := backend.OpenRecent(path)
	if err != nil {
		return documents.Document{}, err
	}
	return f.store.LoadSnapshot(doc.Path, doc.Raw)
}

func (f *Files) Current() documents.Document                  { return f.store.Current() }
func (f *Files) List() []documents.Document                   { return f.store.List() }
func (f *Files) Select(id string) (documents.Document, error) { return f.store.Select(id) }
func (f *Files) Close(id string) error                        { return f.store.Close(id) }
func (f *Files) New() documents.Document                      { return f.store.New("未命名.md", "") }
func (f *Files) Draft(id, content string) error               { return f.store.Draft(id, content) }

func (f *Files) NewNamed(name string) (documents.Document, error) {
	return f.store.NewNamed(name)
}

func (f *Files) Session() documents.SessionState {
	return f.store.Sessions()
}

func (f *Files) CheckExternal(id string) (documents.ExternalState, error) {
	return f.store.CheckExternal(id)
}
func (f *Files) Reload(id string, discardDirty bool) (documents.Document, error) {
	return f.store.Reload(id, discardDirty)
}

func (f *Files) Open(ctx context.Context) (*documents.Document, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: "打开 Markdown 文档",
		Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	doc, err := f.store.Load(paths[0])
	if err != nil {
		return nil, err
	}
	if f.workspace != nil {
		if err := f.workspace.rememberAuthorized(doc.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &doc, nil
}

func (f *Files) Save(ctx context.Context, id, content string, saveAs bool) (*documents.Document, error) {
	return f.save(mygo.CallerWindow(ctx), id, content, saveAs)
}

func (f *Files) save(win *mygo.Window, id, content string, saveAs bool) (*documents.Document, error) {
	doc := f.store.Current()
	if doc.ID != id {
		return nil, documents.ErrStale
	}
	path := doc.Path
	if path == "" || saveAs {
		var err error
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "保存 Markdown 文档", DefaultPath: doc.Name,
			Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}},
		})
		if err != nil || path == "" {
			return nil, err
		}
	}
	saved, err := f.store.Save(id, content, path)
	if err != nil {
		return nil, err
	}
	if f.workspace != nil {
		if err := f.workspace.rememberAuthorized(saved.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &saved, nil
}

func main() {
	workspace := NewWorkspace()
	files := &Files{store: documents.NewStore("A small thought.md", welcome), workspace: workspace}
	prepareVerification(files)
	mygo.Bind(files)
	mygo.Bind(workspace)
	mygo.Bind(NewAssets(files))
	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title: "Leafmark · 叶笺", URL: "/", Width: 1200, Height: 900,
			MinWidth: 760, MinHeight: 560, StateKey: "main",
			TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: 40,
			BackgroundColor: "light-dark(#faf9f5, #222426)",
			Vibrancy:        mygo.VibrancySidebar, AutoHideMenuBar: true,
		}
		if runtime.GOOS == "darwin" {
			opts.TrafficLightPosition = &mygo.Point{X: 20, Y: 18}
		}
		win := mygo.NewWindow(opts)
		startVerification(win, files)
		var prompting atomic.Bool
		win.OnClose(func(e *mygo.CloseEvent) {
			e.PreventDefault()
			if !prompting.CompareAndSwap(false, true) {
				return
			}
			go func() {
				defer prompting.Store(false)
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				// 先锁住正文并等待前端草稿落到后端，关闭检查读取完整快照。
				if _, err := win.EvalContext(ctx, "await window.leafmarkLifecycle.prepareClose(); return true;"); err != nil {
					win.Eval("window.leafmarkLifecycle.resume();")
					mygo.Dialog.Error("无法安全关闭", "同步草稿失败，请先保存文档再关闭。")
					return
				}
				defer win.Eval("window.leafmarkLifecycle.resume();")
				dirty := []documents.Document{}
				for _, doc := range files.store.List() {
					if doc.Dirty {
						dirty = append(dirty, doc)
					}
				}
				if len(dirty) == 0 {
					win.Destroy()
					return
				}
				res, err := mygo.Dialog.Message(mygo.MessageOptions{
					Parent: win, Type: mygo.MessageQuestion, Message: "要保存文档后再关闭吗？",
					Detail: "未保存的修改会在关闭窗口后丢失。", Buttons: []string{"保存", "不保存", "取消"},
					DefaultButton: 0, CancelButton: 2,
				})
				if err != nil {
					return
				}
				switch res.Button {
				case 0:
					original := files.store.Current().ID
					for _, doc := range dirty {
						if _, err := files.store.Select(doc.ID); err != nil {
							return
						}
						saved, err := files.save(win, doc.ID, doc.Content, false)
						if err != nil || saved == nil {
							files.store.Select(original)
							if err != nil {
								mygo.Dialog.Error("保存失败", err.Error())
							}
							return
						}
					}
					win.Eval("window.leafmarkLifecycle.discardRecovery();")
					win.Destroy()
				case 1:
					win.Eval("window.leafmarkLifecycle.discardRecovery();")
					win.Destroy()
				}
			}()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
