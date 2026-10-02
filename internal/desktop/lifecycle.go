package desktop

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"leafmark/internal/documents"
)

// installCloseHandler 在关闭前同步所有草稿，并处理未保存文档。
func installCloseHandler(win *mygo.Window, files *Files) {
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
}
