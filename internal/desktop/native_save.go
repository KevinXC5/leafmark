package desktop

import (
	"errors"
	"log"
	"time"

	"github.com/egoist/mygo"
	"leafmark/internal/documents"
)

const autoSaveDelay = 2 * time.Second

func (a *nativeApp) saveActive(saveAs bool) {
	tab := a.active()
	if tab == nil || tab.editor == nil {
		return
	}
	doc, ok := a.document(tab.id)
	if !ok {
		return
	}
	content := tab.editor.Markdown()
	a.saveDocument(doc.ID, content, saveAs, nil)
}

// saveDocument 在后台写盘。generation 用于丢弃过期结果，不把旧正文写回编辑器。
func (a *nativeApp) saveDocument(id, content string, saveAs bool, done func(ok bool)) {
	if a.saving[id] {
		if done != nil {
			done(false)
		}
		return
	}
	if err := a.files.store.Draft(id, content); err != nil {
		a.notice = err.Error()
		if done != nil {
			done(false)
		}
		return
	}
	gen := a.saveGen[id] + 1
	a.saveGen[id] = gen
	a.saving[id] = true
	win := a.window()
	go func() {
		saved, err := a.saveSnapshot(win, id, content, saveAs)
		a.update(func() {
			defer func() {
				if a.saveGen[id] == gen {
					a.saving[id] = false
				}
			}()
			if a.saveGen[id] != gen {
				if done != nil {
					done(false)
				}
				return
			}
			if err != nil {
				if errors.Is(err, documents.ErrConflict) {
					a.conflicts[id] = true
				}
				a.notice = err.Error()
				if done != nil {
					done(false)
				}
				return
			}
			if saved == nil {
				if done != nil {
					done(false)
				}
				return
			}
			delete(a.conflicts, id)
			// 写盘期间用户继续输入时，只更新路径，正文以编辑器为准。
			if tab := a.tabByID(id); tab != nil && tab.editor != nil && tab.editor.Markdown() != content {
				_ = a.files.store.Draft(id, tab.editor.Markdown())
			}
			a.refreshTitle()
			a.persistRecovery()
			a.reloadNavigation()
			if done != nil {
				done(true)
			}
		})
	}()
}

// saveSnapshot 只使用发起保存的标签快照；切换标签不会改变保存目标。
func (a *nativeApp) saveSnapshot(win *mygo.Window, id, content string, saveAs bool) (*documents.Document, error) {
	doc, ok := a.document(id)
	if !ok {
		return nil, documents.ErrStale
	}
	path := doc.Path
	if path == "" || saveAs {
		var err error
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "保存 Markdown 文档", DefaultPath: doc.Name, Filters: []mygo.FileFilter{{Name: "Markdown 文档", Extensions: []string{"md", "markdown"}}}})
		if err != nil || path == "" {
			return nil, err
		}
	}
	saved, err := a.files.store.SaveSnapshot(id, content, path)
	if err != nil {
		return nil, err
	}
	if a.files.workspace != nil {
		if err := a.files.workspace.rememberAuthorized(saved.Path); err != nil {
			log.Printf("记录近期文件失败：%v", err)
		}
	}
	return &saved, nil
}

func (a *nativeApp) reloadActive(discard bool) {
	if tab := a.active(); tab != nil {
		a.reloadDocument(tab.id, discard)
	}
}

func (a *nativeApp) reloadDocument(id string, discard bool) {
	tab := a.tabByID(id)
	if tab == nil || tab.editor == nil || a.saving[id] {
		return
	}
	a.syncEditor(tab)
	before := tab.editor.Markdown()
	editor := tab.editor
	go func() {
		path, raw, err := a.files.store.ReadSnapshot(id)
		a.update(func() {
			if err != nil {
				a.notice = err.Error()
				return
			}
			tab := a.tabByID(id)
			if tab == nil || tab.editor != editor {
				return
			}
			if a.saving[id] || tab.editor.Markdown() != before {
				a.notice = "读取期间文档发生变化，请再次重新加载"
				return
			}
			a.syncEditor(tab)
			doc, err := a.files.store.ReloadSnapshot(id, path, raw, discard)
			if err != nil {
				if errors.Is(err, documents.ErrDirty) && !discard {
					a.confirmReload(id)
					return
				}
				a.notice = err.Error()
				return
			}
			delete(a.conflicts, id)
			a.replaceEditor(id, doc.Content)
			a.persistRecovery()
		})
	}()
}

func (a *nativeApp) confirmReload(id string) {
	win := a.window()
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: win, Type: mygo.MessageWarning, Message: "文件已在外部修改",
			Detail: "重新加载会丢弃当前未保存的修改。", Buttons: []string{"重新加载", "保留"},
			DefaultButton: 1, CancelButton: 1,
		})
		if err != nil || res.Button != 0 {
			return
		}
		a.update(func() { a.reloadDocument(id, true) })
	}()
}

// noteEdits 记下正文发生变化的时刻。编辑器未暴露改动时间，不能用打开时刻代替。
func (a *nativeApp) noteEdits(now time.Time) {
	for _, tab := range a.tabs {
		if tab.editor == nil {
			continue
		}
		text := tab.editor.Markdown()
		if prev, ok := a.seenText[tab.id]; !ok || prev != text {
			if ok {
				a.editedAt[tab.id] = now
			}
			a.seenText[tab.id] = text
		}
	}
}

// scheduleAutoSave 在视图里按帧检查，距上次改动满 2 秒才写已有路径。
func (a *nativeApp) scheduleAutoSave(now time.Time) {
	a.noteEdits(now)
	if !a.settings.AutoSave {
		return
	}
	for _, tab := range a.tabs {
		if tab.editor == nil {
			continue
		}
		since := a.editedAt[tab.id]
		if since.IsZero() || now.Sub(since) < autoSaveDelay || a.saving[tab.id] {
			continue
		}
		doc, ok := a.document(tab.id)
		if !ok || doc.Path == "" || a.conflicts[tab.id] {
			continue
		}
		content := tab.editor.Markdown()
		if !doc.Dirty && doc.Content == content {
			delete(a.editedAt, tab.id)
			continue
		}
		delete(a.editedAt, tab.id)
		if err := a.files.store.Draft(tab.id, content); err != nil && !errors.Is(err, documents.ErrStale) {
			log.Printf("同步草稿失败：%v", err)
		}
		a.saveDocument(tab.id, content, false, nil)
	}
}
