package documents

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxBytes = 16 << 20

var (
	ErrStale    = errors.New("当前文档已经切换，请重新操作")
	ErrConflict = errors.New("文件已被其他程序修改，为避免覆盖请另存为，或重新打开文件")
)

type Document struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Dirty   bool   `json:"dirty"`
}
type snapshot struct {
	doc    Document
	saved  string
	digest [32]byte
	crlf   bool
	bom    bool
}

type Store struct {
	sessions map[string]snapshot
	order    []string
	mu       sync.Mutex
	// 写盘串行化覆盖准备、冲突检查与回写，避免另存为和自动保存交错。
	saveMu sync.Mutex
	doc    Document
	saved  string
	digest [32]byte
	crlf   bool
	bom    bool
}

func NewStore(name, content string) *Store {
	s := &Store{}
	s.New(name, content)
	return s
}
func (s *Store) New(name, content string) Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	s.doc = Document{ID: rand.Text(), Name: name, Content: content}
	s.saved, s.crlf, s.bom = content, false, false
	s.digest = [32]byte{}
	return s.doc
}
func (s *Store) Current() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doc
}

// 标签页各自保留编码、磁盘快照和草稿，切换时不重读磁盘。
func (s *Store) remember() {
	if s.sessions == nil {
		s.sessions = make(map[string]snapshot)
	}
	if s.doc.ID == "" {
		return
	}
	if _, exists := s.sessions[s.doc.ID]; !exists {
		s.order = append(s.order, s.doc.ID)
	}
	s.sessions[s.doc.ID] = snapshot{s.doc, s.saved, s.digest, s.crlf, s.bom}
}
func (s *Store) selectLocked(id string) error {
	if s.doc.ID == id {
		return nil
	}
	s.remember()
	entry, exists := s.sessions[id]
	if !exists {
		return ErrStale
	}
	s.doc, s.saved, s.digest, s.crlf, s.bom = entry.doc, entry.saved, entry.digest, entry.crlf, entry.bom
	return nil
}
func (s *Store) Select(id string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.selectLocked(id); err != nil {
		return Document{}, err
	}
	return s.doc, nil
}
func (s *Store) List() []Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	list := make([]Document, 0, len(s.order))
	for _, id := range s.order {
		list = append(list, s.sessions[id].doc)
	}
	return list
}
func (s *Store) Close(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	if _, ok := s.sessions[id]; !ok {
		return ErrStale
	}
	delete(s.sessions, id)
	for i, key := range s.order {
		if key == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	if s.doc.ID == id {
		s.doc = Document{}
		s.saved = ""
		s.digest = [32]byte{}
		s.crlf, s.bom = false, false
		if len(s.order) > 0 {
			return s.selectLocked(s.order[len(s.order)-1])
		}
	}
	return nil
}
func validate(content string) error {
	if len(content) > maxBytes {
		return errors.New("验证版本支持最大 16 MB 的文档")
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return errors.New("请打开 UTF-8 编码的 Markdown 文本文件")
	}
	return nil
}
func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if err := validate(string(raw)); err != nil {
		return nil, err
	}
	return raw, nil
}
func (s *Store) Load(path string) (Document, error) {
	// 解析符号链接，保存时更新实际文件，保留原有链接本身。
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Document{}, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Document{}, err
	}
	raw, err := readFile(path)
	if err != nil {
		return Document{}, err
	}
	return s.LoadSnapshot(path, raw)
}

// LoadSnapshot 加载后端已授权读取的原始字节，不再解析链接或访问磁盘。
// 保留原始 BOM、换行和摘要，供后续保存校验使用；此接口不直接绑定前端。
func (s *Store) LoadSnapshot(path string, raw []byte) (Document, error) {
	if !filepath.IsAbs(path) {
		return Document{}, errors.New("文档快照必须使用已授权的绝对路径")
	}
	if err := validate(string(raw)); err != nil {
		return Document{}, err
	}
	path = filepath.Clean(path)
	text := strings.TrimPrefix(string(raw), string(rune(0xfeff)))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	for id, entry := range s.sessions {
		if entry.doc.Path == path {
			if err := s.selectLocked(id); err != nil {
				return Document{}, err
			}
			return s.doc, nil
		}
	}
	s.bom = strings.HasPrefix(string(raw), "\ufeff")
	s.crlf = strings.Contains(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	s.doc = Document{ID: rand.Text(), Name: filepath.Base(path), Path: path, Content: text}
	s.saved, s.digest = text, sha256.Sum256(raw)
	return s.doc, nil
}

// ReadSnapshot 后台只读取已授权文档，不改变当前标签或草稿。
func (s *Store) ReadSnapshot(id string) (string, []byte, error) {
	s.mu.Lock()
	entry, err := s.openedLocked(id)
	s.mu.Unlock()
	if err != nil {
		return "", nil, err
	}
	raw, err := readAuthorizedFile(entry.doc.Path)
	return entry.doc.Path, raw, err
}

// ReloadSnapshot 在主线程确认后提交已读取字节，保留标签 ID 和当前选择。
func (s *Store) ReloadSnapshot(id, path string, raw []byte, discardDirty bool) (Document, error) {
	if err := validate(string(raw)); err != nil {
		return Document{}, err
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	entry, err := s.openedLocked(id)
	if err != nil {
		return Document{}, err
	}
	if path != entry.doc.Path {
		return Document{}, ErrStale
	}
	if entry.doc.Dirty && !discardDirty {
		return Document{}, ErrDirty
	}
	entry.bom = strings.HasPrefix(string(raw), "\ufeff")
	text := strings.TrimPrefix(string(raw), "\ufeff")
	entry.crlf = strings.Contains(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	entry.doc.Content, entry.doc.Dirty = text, false
	entry.saved, entry.digest = text, sha256.Sum256(raw)
	s.sessions[id] = entry
	if s.doc.ID == id {
		s.doc, s.saved, s.digest, s.crlf, s.bom = entry.doc, entry.saved, entry.digest, entry.crlf, entry.bom
	}
	return entry.doc, nil
}

func (s *Store) Draft(id, content string) error {
	if err := validate(content); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc.ID != id {
		s.remember()
		entry, exists := s.sessions[id]
		if !exists {
			return ErrStale
		}
		entry.doc.Content = content
		entry.doc.Dirty = content != entry.saved
		s.sessions[id] = entry
		return nil
	}
	s.doc.Content = content
	s.doc.Dirty = content != s.saved
	return nil
}
func (s *Store) Save(id, content, path string) (Document, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	prepared, err := s.prepareSave(id, content, path, true)
	if err != nil {
		return Document{}, err
	}
	if err := writePrepared(prepared); err != nil {
		return Document{}, err
	}
	return s.finishSave(prepared)
}

// SaveSnapshot 按指定标签的编码和摘要写盘，不要求它仍是当前标签。
// 磁盘读写在锁外完成；完成后只更新该标签，前台当前标签保持不变。
func (s *Store) SaveSnapshot(id, content, path string) (Document, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	prepared, err := s.prepareSave(id, content, path, false)
	if err != nil {
		return Document{}, err
	}
	if err := writePrepared(prepared); err != nil {
		return Document{}, err
	}
	return s.finishSave(prepared)
}

type preparedSave struct {
	id      string
	content string
	path    string
	name    string
	raw     []byte
}

func (s *Store) prepareSave(id, content, path string, requireCurrent bool) (preparedSave, error) {
	if err := validate(content); err != nil {
		return preparedSave{}, err
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return preparedSave{}, err
	}
	if actual, err := filepath.EvalSymlinks(path); err == nil {
		path = actual
	}
	s.mu.Lock()
	s.remember()
	entry, exists := s.sessions[id]
	current := s.doc.ID == id
	others := []string{}
	for key, item := range s.sessions {
		if key != id && item.doc.Path != "" {
			others = append(others, item.doc.Path)
		}
	}
	s.mu.Unlock()
	if !exists || (requireCurrent && !current) {
		return preparedSave{}, ErrStale
	}
	for _, other := range others {
		if sameFilePath(path, other) {
			return preparedSave{}, errors.New("目标文件已经在另一个标签中打开，请切换到该标签或选择其他文件名")
		}
	}
	if path == entry.doc.Path {
		raw, err := readFile(path)
		if err != nil {
			return preparedSave{}, err
		}
		if sha256.Sum256(raw) != entry.digest {
			return preparedSave{}, ErrConflict
		}
	}
	encoded := content
	if entry.crlf {
		encoded = strings.ReplaceAll(encoded, "\n", "\r\n")
	}
	if entry.bom {
		encoded = string(rune(0xfeff)) + encoded
	}
	return preparedSave{id: id, content: content, path: path, name: filepath.Base(path), raw: []byte(encoded)}, nil
}

func writePrepared(prepared preparedSave) error {
	return atomicWrite(prepared.path, prepared.raw)
}

func (s *Store) finishSave(prepared preparedSave) (Document, error) {
	digest := sha256.Sum256(prepared.raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remember()
	entry, exists := s.sessions[prepared.id]
	if !exists {
		return Document{}, ErrStale
	}
	entry.doc.Path, entry.doc.Name = prepared.path, prepared.name
	entry.saved, entry.digest = prepared.content, digest
	// 写盘期间到达的新正文仍保留为未保存草稿。
	entry.doc.Dirty = entry.doc.Content != prepared.content
	s.sessions[prepared.id] = entry
	if s.doc.ID == prepared.id {
		s.doc, s.saved, s.digest = entry.doc, entry.saved, entry.digest
	}
	return entry.doc, nil
}

func sameFilePath(a, b string) bool {
	if a == b {
		return true
	}
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}
func atomicWrite(path string, raw []byte) error {
	mode := os.FileMode(0644)
	if stat, err := os.Stat(path); err == nil {
		if !stat.Mode().IsRegular() {
			return errors.New("目标不是普通文件")
		}
		mode = stat.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// 在相同目录写临时文件并替换，避免中断时留下半个 Markdown 文件。
	f, err := os.CreateTemp(filepath.Dir(path), ".leafmark-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("保存失败：%w", err)
	}
	return nil
}
