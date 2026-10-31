package desktop

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"leafmark/internal/documents"
)

const (
	nativeRecoveryName = "native-recovery.json"
	nativeSettingsName = "native-settings.json"
	maxRecoveryNote    = 1 << 20
	maxRecoveryBytes   = 5 << 20
)

// nativeDraft 是原生窗口自己的恢复记录，与浏览器 localStorage 互不读写。
type nativeDraft struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// nativeDataDir 是原生草稿和设置的目录。LEAFMARK_NATIVE_DATA_DIR 优先，
// 验证启动时指向独立目录，避免读写用户自己的恢复文件。
func nativeDataDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("LEAFMARK_NATIVE_DATA_DIR")); dir != "" {
		return dir, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Leafmark"), nil
}

var nativeRecoveryGeneration atomic.Uint64
var nativeRecoveryMu sync.Mutex

func writeNativeRecoveryVersion(drafts []nativeDraft, ticket uint64) error {
	dir, err := nativeDataDir()
	if err != nil {
		return err
	}
	return writeNativeRecoveryVersionIn(dir, drafts, ticket)
}

// writeNativeRecoveryVersionIn 写入调用方事先确定的目录。后台写盘要在发起时就定下目录，
// 否则协程执行时环境可能已变（例如测试结束后还原了 LEAFMARK_NATIVE_DATA_DIR），写进用户的真实配置。
func writeNativeRecoveryVersionIn(dir string, drafts []nativeDraft, ticket uint64) error {
	nativeRecoveryMu.Lock()
	defer nativeRecoveryMu.Unlock()
	if ticket != nativeRecoveryGeneration.Load() {
		return nil
	}
	return writeNativeRecoveryIn(dir, drafts)
}

// 损坏或不可读的恢复记录保留原文件，禁止后续空快照覆盖。
func readNativeRecoveryState() ([]nativeDraft, bool) {
	dir, err := nativeDataDir()
	if err != nil {
		return nil, false
	}
	raw, err := os.ReadFile(filepath.Join(dir, nativeRecoveryName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, true
	}
	if err != nil || len(raw) == 0 || len(raw) > maxRecoveryBytes {
		return nil, false
	}
	var drafts []nativeDraft
	if err := json.Unmarshal(raw, &drafts); err != nil {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]nativeDraft, 0, len(drafts))
	valid := true
	for _, draft := range drafts {
		if !validDraft(draft) || seen[draft.ID] {
			valid = false
			continue
		}
		seen[draft.ID] = true
		out = append(out, draft)
	}
	return out, valid
}

func validDraft(draft nativeDraft) bool {
	if draft.ID == "" || draft.Name == "" || strings.ContainsRune(draft.Content, '\x00') || !utf8.ValidString(draft.Content) {
		return false
	}
	if len(draft.Content) > maxRecoveryNote || len(draft.Name) > 255 {
		return false
	}
	if draft.Path != "" && !filepath.IsAbs(draft.Path) {
		return false
	}
	return true
}

func writeNativeRecoveryIn(dir string, drafts []nativeDraft) error {
	clean := make([]nativeDraft, 0, len(drafts))
	seen := map[string]bool{}
	for _, draft := range drafts {
		if !validDraft(draft) || seen[draft.ID] {
			return errors.New("恢复草稿无效，未更新恢复文件")
		}
		seen[draft.ID] = true
		clean = append(clean, draft)
	}
	raw, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > maxRecoveryBytes {
		return errors.New("恢复草稿总量超过 5 MB，未更新恢复文件")
	}
	return writeNativeFile(dir, nativeRecoveryName, raw)
}

func loadNativeSettings() nativeSettings {
	settings := defaultNativeSettings()
	dir, err := nativeDataDir()
	if err != nil {
		return settings
	}
	raw, err := os.ReadFile(filepath.Join(dir, nativeSettingsName))
	if err != nil {
		return settings
	}
	// 缺少的字段保持默认值：先填默认，再让存盘内容覆盖。
	stored := settings
	if json.Unmarshal(raw, &stored) != nil {
		return settings
	}
	return stored.normalized()
}

func saveNativeSettingsIn(dir string, settings nativeSettings) error {
	if settings != settings.normalized() {
		return errors.New("设置含有无效的值")
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeNativeFile(dir, nativeSettingsName, raw)
}

func writeNativeFile(dir, name string, raw []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	f, err := os.CreateTemp(dir, "."+name+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *nativeApp) restoreDrafts(drafts []nativeDraft) {
	if len(drafts) == 0 {
		return
	}
	current := a.files.Current()
	restored := 0
	for _, draft := range drafts {
		doc, err := a.openRecovered(draft)
		if err != nil {
			a.unrestored = append(a.unrestored, draft)
			log.Printf("恢复草稿失败：%v", err)
			continue
		}
		if err := a.files.store.Draft(doc.ID, draft.Content); err != nil {
			a.unrestored = append(a.unrestored, draft)
			log.Printf("写回恢复草稿失败：%v", err)
			continue
		}
		doc.Content = draft.Content
		doc.Dirty = draft.Content != a.savedBaseline(doc.ID)
		if a.tabByID(doc.ID) != nil {
			a.replaceEditor(doc.ID, draft.Content)
		}
		a.adopt(doc, draft.Content)
		restored++
	}
	if restored == 0 {
		return
	}
	// 欢迎文档没有改动时让位给恢复出来的草稿。
	if current.Path == "" && current.Content == welcome && !current.Dirty {
		if a.tabByID(current.ID) != nil && len(a.tabs) > 1 {
			_ = a.files.Close(current.ID)
			a.dropTab(current.ID)
		}
	}
	a.notice = "已恢复上次未保存的草稿"
}

func (a *nativeApp) savedBaseline(id string) string {
	content, err := a.files.store.SavedContent(id)
	if err != nil {
		return ""
	}
	return content
}

func (a *nativeApp) openRecovered(draft nativeDraft) (documents.Document, error) {
	if draft.Path != "" {
		if _, err := os.Stat(draft.Path); err == nil {
			return a.files.store.Load(draft.Path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return documents.Document{}, err
		}
	}
	name := draft.Name
	if name == "" {
		name = "未命名.md"
	}
	return a.files.store.NewNamed(name)
}

func (a *nativeApp) persistSettings() {
	settings := a.settings
	dir, err := nativeDataDir()
	if err != nil {
		log.Printf("保存设置失败：%v", err)
		return
	}
	a.writes.Go(func() {
		if err := saveNativeSettingsIn(dir, settings); err != nil {
			log.Printf("保存设置失败：%v", err)
		}
	})
}

func (a *nativeApp) persistRecovery() {
	a.syncAll()
	if a.recoveryBlocked {
		return
	}
	drafts := append([]nativeDraft{}, a.unrestored...)
	for _, doc := range a.files.store.List() {
		if !doc.Dirty {
			continue
		}
		drafts = append(drafts, nativeDraft{ID: doc.ID, Name: doc.Name, Path: doc.Path, Content: doc.Content})
	}
	dir, err := nativeDataDir()
	if err != nil {
		log.Printf("保存恢复草稿失败：%v", err)
		return
	}
	ticket := nativeRecoveryGeneration.Add(1)
	a.writes.Go(func() {
		if err := writeNativeRecoveryVersionIn(dir, drafts, ticket); err != nil {
			log.Printf("保存恢复草稿失败：%v", err)
		}
	})
}

func (a *nativeApp) clearRecovery() {
	if a.recoveryBlocked || len(a.unrestored) > 0 {
		return
	}
	ticket := nativeRecoveryGeneration.Add(1)
	if err := writeNativeRecoveryVersion(nil, ticket); err != nil {
		log.Printf("清理恢复记录失败：%v", err)
	}
}
