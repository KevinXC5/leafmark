package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	MaxDocumentBytes = 16 << 20
	MaxScanEntries   = 10000
	MaxScanDepth     = 64
	MaxRecent        = 30
)

var (
	ErrNoWorkspace = errors.New("请先选择工作区文件夹")
	ErrInvalidPath = errors.New("路径必须是工作区内的可见相对路径，不能包含符号链接")
	ErrExists      = errors.New("目标已存在，不能覆盖")
	// ErrParentMissing 仍可用 errors.Is(err, os.ErrNotExist) 判断，文案直接面向用户。
	ErrParentMissing error = parentMissingError{}
	ErrScanLimit           = errors.New("工作区扫描超过限制，请选择较小的文件夹")
)

type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
type Node struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Children  []Node `json:"children,omitempty"`
}
type Document struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Relative string `json:"relative"`
	Content  string `json:"content"`
	// Raw 仅供后端加载授权快照，前端 JSON 不携带原始字节。
	Raw []byte `json:"-"`
}
type RecentDocument struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	OpenedAt time.Time `json:"openedAt"`
}
type State struct {
	Workspace *Folder          `json:"workspace"`
	Recent    []RecentDocument `json:"recent"`
}

// Store 的绝对路径授权入口仅供后端调用，绑定到前端的服务只暴露相对路径操作。
type Store struct {
	mu      sync.Mutex
	dataDir string
	state   State
}

func New() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewAt(filepath.Join(dir, "Leafmark"))
}

// NewAt 支持测试使用独立的数据目录，正常应用应使用 New。
func NewAt(dataDir string) (*Store, error) {
	s := &Store{dataDir: dataDir, state: State{Recent: []RecentDocument{}}}
	f, err := os.Open(filepath.Join(dataDir, "workspace.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("工作区配置过大")
	}
	if err := json.Unmarshal(raw, &s.state); err != nil {
		return nil, fmt.Errorf("读取工作区配置：%w", err)
	}
	if s.state.Recent == nil {
		s.state.Recent = []RecentDocument{}
	}
	if len(s.state.Recent) > MaxRecent {
		s.state.Recent = s.state.Recent[:MaxRecent]
	}
	if s.state.Workspace != nil && (!filepath.IsAbs(s.state.Workspace.Path) || filepath.Clean(s.state.Workspace.Path) != s.state.Workspace.Path) {
		return nil, errors.New("工作区配置路径无效")
	}
	return s, nil
}

func (s *Store) Current() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.state)
}
func cloneState(state State) State {
	out := state
	out.Recent = append([]RecentDocument{}, state.Recent...)
	if state.Workspace != nil {
		folder := *state.Workspace
		out.Workspace = &folder
	}
	return out
}
func (s *Store) commit(next State) error {
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(s.dataDir, "workspace.json"), raw, 0600); err != nil {
		return err
	}
	s.state = next
	return nil
}

// SelectFolder 的路径必须来自原生选择框，不能直接接受前端传入的路径。
func (s *Store) SelectFolder(path string) (*Folder, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	root.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneState(s.state)
	next.Workspace = &Folder{Name: filepath.Base(path), Path: path}
	if err := s.commit(next); err != nil {
		return nil, err
	}
	folder := *next.Workspace
	return &folder, nil
}

func markdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}
func validRelative(path string) (string, error) {
	// 同时拒绝两种分隔符的混用和盘符，避免跨平台调用带来路径语义变化。
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "\\:\x00") {
		return "", ErrInvalidPath
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") || part == "node_modules" {
			return "", ErrInvalidPath
		}
	}
	if !filepath.IsLocal(path) {
		return "", ErrInvalidPath
	}
	return filepath.FromSlash(path), nil
}
func (s *Store) root() (*os.Root, error) {
	if s.state.Workspace == nil {
		return nil, ErrNoWorkspace
	}
	// 根目录被替换成符号链接时也拒绝打开，不自动扩展用户授权范围。
	actual, err := filepath.EvalSymlinks(s.state.Workspace.Path)
	if err != nil {
		return nil, err
	}
	if actual != s.state.Workspace.Path {
		return nil, ErrInvalidPath
	}
	root, err := os.OpenRoot(s.state.Workspace.Path)
	if err != nil {
		return nil, err
	}
	if err := verifyRoot(root, s.state.Workspace.Path); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

type parentMissingError struct{}

func (parentMissingError) Error() string {
	return "上级文件夹不存在，请先创建上级文件夹"
}
func (parentMissingError) Is(target error) bool { return target == os.ErrNotExist }

func checked(root *os.Root, relative string, allowMissing bool) (string, error) {
	path, err := validRelative(relative)
	if err != nil {
		return "", err
	}
	parts := strings.Split(path, string(filepath.Separator))
	for i := range parts {
		current := filepath.Join(parts[:i+1]...)
		info, err := root.Lstat(current)
		if allowMissing && i == len(parts)-1 && errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		if i < len(parts)-1 && errors.Is(err, os.ErrNotExist) {
			return "", ErrParentMissing
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) {
			return "", ErrInvalidPath
		}
	}
	return path, nil
}

func (s *Store) Tree() ([]Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	count := 0
	return scan(root, ".", 0, &count)
}
func scan(root *os.Root, path string, depth int, count *int) ([]Node, error) {
	if depth > MaxScanDepth {
		return nil, ErrScanLimit
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	nodes := []Node{}
	// 分批读取，限制包括被过滤项在内的总数，避免巨大目录一次占满内存。
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			(*count)++
			if *count > MaxScanEntries {
				return nil, ErrScanLimit
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			relative := filepath.Join(path, name)
			info, err := root.Lstat(relative)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			node := Node{Name: name, Path: filepath.ToSlash(relative), Directory: info.IsDir()}
			if info.IsDir() {
				if _, err := checked(root, node.Path, false); err != nil {
					return nil, err
				}
				node.Children, err = scan(root, relative, depth+1, count)
				if err != nil {
					return nil, err
				}
			} else if !info.Mode().IsRegular() || !markdown(name) {
				continue
			}
			nodes = append(nodes, node)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Directory != nodes[j].Directory {
			return nodes[i].Directory
		}
		return nodes[i].Name < nodes[j].Name
	})
	return nodes, nil
}

func (s *Store) OpenDocument(relative string) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	path, err := checked(root, relative, false)
	if err != nil {
		return nil, err
	}
	if !markdown(path) {
		return nil, errors.New("仅支持 Markdown 文档")
	}
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("目标不是普通文件")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := verifyOpened(root, path, f, info); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxDocumentBytes {
		return nil, errors.New("文档不能超过 16 MB")
	}
	if !utf8.Valid(raw) || strings.ContainsRune(string(raw), '\x00') {
		return nil, errors.New("请打开 UTF-8 编码的 Markdown 文档")
	}
	absolute := filepath.Join(s.state.Workspace.Path, path)
	if err := s.remember(absolute); err != nil {
		return nil, err
	}
	content := strings.ReplaceAll(strings.TrimPrefix(string(raw), "\ufeff"), "\r\n", "\n")
	return &Document{Name: filepath.Base(path), Path: absolute, Relative: filepath.ToSlash(path), Content: content, Raw: raw}, nil
}
func (s *Store) CreateFile(relative string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root()
	if err != nil {
		return err
	}
	defer root.Close()
	path, err := checked(root, relative, true)
	if err != nil {
		return err
	}
	if !markdown(path) {
		return errors.New("新建文件必须使用 .md 或 .markdown 扩展名")
	}
	// O_EXCL 原子保证文件存在时绝不覆盖；空文件本身无需临时替换。
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return ErrExists
	}
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(syncErr, closeErr)
}
func (s *Store) CreateFolder(relative string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root()
	if err != nil {
		return err
	}
	defer root.Close()
	path, err := checked(root, relative, true)
	if err != nil {
		return err
	}
	if err := root.Mkdir(path, 0755); errors.Is(err, os.ErrExist) {
		return ErrExists
	} else {
		return err
	}
}
func (s *Store) Rename(oldRelative, newRelative string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root()
	if err != nil {
		return err
	}
	defer root.Close()
	oldPath, err := checked(root, oldRelative, false)
	if err != nil {
		return err
	}
	newPath, err := checked(root, newRelative, true)
	if err != nil {
		return err
	}
	info, err := root.Lstat(oldPath)
	if err != nil {
		return err
	}
	if !info.IsDir() && (!info.Mode().IsRegular() || !markdown(oldPath) || !markdown(newPath)) {
		return errors.New("仅支持重命名 Markdown 文件或文件夹")
	}
	if _, err := root.Lstat(newPath); err == nil {
		return ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info.IsDir() && strings.HasPrefix(newPath, oldPath+string(filepath.Separator)) {
		return ErrInvalidPath
	}
	if err := renameExclusive(root, oldPath, newPath); err != nil {
		return err
	}
	next := cloneState(s.state)
	oldAbsolute := filepath.Join(s.state.Workspace.Path, oldPath)
	newAbsolute := filepath.Join(s.state.Workspace.Path, newPath)
	for i, recent := range next.Recent {
		if recent.Path == oldAbsolute || strings.HasPrefix(recent.Path, oldAbsolute+string(filepath.Separator)) {
			next.Recent[i].Path = newAbsolute + strings.TrimPrefix(recent.Path, oldAbsolute)
			next.Recent[i].Name = filepath.Base(next.Recent[i].Path)
		}
	}
	// 文件重命名成功后即使配置保存失败也不回滚，避免回滚覆盖外部新建文件。
	if err := s.commit(next); err != nil {
		return fmt.Errorf("重命名已完成，但保存近期记录失败：%w", err)
	}
	return nil
}
func (s *Store) Recent() []RecentDocument { return s.Current().Recent }
func (s *Store) Remember(relative string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root()
	if err != nil {
		return err
	}
	defer root.Close()
	path, err := checked(root, relative, false)
	if err != nil {
		return err
	}
	info, err := root.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !markdown(path) {
		return errors.New("仅支持普通 Markdown 文档")
	}
	return s.remember(filepath.Join(s.state.Workspace.Path, path))
}

// RememberAuthorized 供 Files.Open/Save 在原生对话框授权后记录工作区以外的文档，不绑定前端。
func (s *Store) RememberAuthorized(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !markdown(path) {
		return errors.New("仅支持普通 Markdown 文档")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.remember(path)
}
func (s *Store) remember(path string) error {
	next := cloneState(s.state)
	next.Recent = []RecentDocument{{Name: filepath.Base(path), Path: path, OpenedAt: time.Now().UTC()}}
	for _, recent := range s.state.Recent {
		if recent.Path != path && len(next.Recent) < MaxRecent {
			next.Recent = append(next.Recent, recent)
		}
	}
	return s.commit(next)
}

// WriteHTML 的目标路径只允许来自原生保存框，不向前端暴露直接按路径写入的方法。
func WriteHTML(path, html string) error {
	if len(html) > MaxDocumentBytes {
		return errors.New("HTML 不能超过 16 MB")
	}
	if !utf8.ValidString(html) || strings.ContainsRune(html, '\x00') {
		return errors.New("HTML 必须是 UTF-8 文本")
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".html" && ext != ".htm" {
		return errors.New("导出文件必须使用 .html 或 .htm 扩展名")
	}
	mode := os.FileMode(0644)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("导出目标不是普通文件")
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return atomicWrite(path, []byte(html), mode)
}
func atomicWrite(path string, raw []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".leafmark-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp) // 只清理本次创建的临时文件，不删除用户文件。
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
	return os.Rename(temp, path)
}
