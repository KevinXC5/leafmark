package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
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
	nativeRecoveryMu.Lock()
	defer nativeRecoveryMu.Unlock()
	if ticket != nativeRecoveryGeneration.Load() {
		return nil
	}
	return writeNativeRecovery(drafts)
}

func readNativeRecovery() []nativeDraft { drafts, _ := readNativeRecoveryState(); return drafts }

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

func writeNativeRecovery(drafts []nativeDraft) error {
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
	return writeNativeFile(nativeRecoveryName, raw)
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

func saveNativeSettings(settings nativeSettings) error {
	if settings != settings.normalized() {
		return errors.New("设置含有无效的值")
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeNativeFile(nativeSettingsName, raw)
}

func writeNativeFile(name string, raw []byte) error {
	dir, err := nativeDataDir()
	if err != nil {
		return err
	}
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
