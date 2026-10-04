package desktop

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"leafmark/internal/documents"
)

// installCloseHandler 在关闭前同步所有草稿，并处理未保存文档。
func installCloseHandler(win *mygo.Window, files *Files) func() {
	var prompting, restarting, approved atomic.Bool
	finish := func() {
		if restarting.Swap(false) {
			// 校验完成后让 Relaunch 执行关闭，避免最后一个窗口先触发普通退出。
			approved.Store(true)
			mygo.App.Relaunch()
		} else {
			win.Destroy()
		}
	}
	win.OnClose(func(e *mygo.CloseEvent) {
		if approved.Swap(false) {
			return
		}
		e.PreventDefault()
		if !prompting.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer prompting.Store(false)
			defer restarting.Store(false)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			// 先锁住正文并等待前端草稿落到后端，关闭检查读取完整快照。
			if _, err := win.Page().EvalContext(ctx, "await window.leafmarkLifecycle.prepareClose(); return true;"); err != nil {
				win.Page().Eval("window.leafmarkLifecycle.resume();")
				mygo.Dialog.Error("无法安全关闭", "同步草稿失败，请先保存文档再关闭。")
				return
			}
			defer win.Page().Eval("window.leafmarkLifecycle.resume();")
			dirty := []documents.Document{}
			for _, doc := range files.store.List() {
				if doc.Dirty {
					dirty = append(dirty, doc)
				}
			}
			if len(dirty) == 0 {
				finish()
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
				win.Page().Eval("window.leafmarkLifecycle.discardRecovery();")
				finish()
			case 1:
				win.Page().Eval("window.leafmarkLifecycle.discardRecovery();")
				finish()
			}
		}()
	})
	return func() {
		if prompting.Load() || !restarting.CompareAndSwap(false, true) {
			return
		}
		win.Close()
	}
}
