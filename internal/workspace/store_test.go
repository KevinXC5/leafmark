package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func setup(t *testing.T) (*Store, string, string) {
	t.Helper()
	dir, data := t.TempDir(), t.TempDir()
	s, err := NewAt(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SelectFolder(dir); err != nil {
		t.Fatal(err)
	}
	// macOS 的临时目录可能带有 /var -> /private/var 链接。
	return s, s.Current().Workspace.Path, data
}
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestTreeFiltersAndStableOrder(t *testing.T) {
	s, dir, _ := setup(t)
	for _, name := range []string{"z.md", "a.MARKDOWN", "image.png", ".secret.md"} {
		write(t, filepath.Join(dir, name), "正文")
	}
	for _, name := range []string{"notes", "node_modules", ".git"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, name, "nested.md"), "正文")
	}
	if err := os.Symlink(dir, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	nodes, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 || nodes[0].Name != "notes" || nodes[1].Name != "a.MARKDOWN" || nodes[2].Name != "z.md" {
		t.Fatalf("文件树不符合预期：%+v", nodes)
	}
	if len(nodes[0].Children) != 1 || nodes[0].Children[0].Path != "notes/nested.md" {
		t.Fatal("嵌套路径错误")
	}
}
func TestPathsRejectTraversalAbsoluteAndSymlinks(t *testing.T) {
	s, dir, _ := setup(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "secret.md"), "秘密")
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "link.md")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret.md", filepath.Join(outside, "secret.md"), "a/../../secret.md", ".hidden.md", "node_modules/a.md", "escape/secret.md", "link.md", "a\\b.md", "C:/secret.md", "./a.md", "a//b.md", ""} {
		t.Run(path, func(t *testing.T) {
			if _, err := s.OpenDocument(path); err == nil {
				t.Fatal("读取允许了无效路径")
			}
			if err := s.CreateFile(path); err == nil {
				t.Fatal("创建允许了无效路径")
			}
			if err := s.CreateFolder(path); err == nil {
				t.Fatal("文件夹创建允许了无效路径")
			}
			if err := s.Remember(path); err == nil {
				t.Fatal("记忆允许了无效路径")
			}
		})
	}
	raw, _ := os.ReadFile(filepath.Join(outside, "secret.md"))
	if string(raw) != "秘密" {
		t.Fatal("工作区外文件被修改")
	}
}
func TestCreateOpenAndPersist(t *testing.T) {
	s, dir, data := setup(t)
	if err := s.CreateFolder("笔记"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateFile("笔记/中文.md"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "笔记", "中文.md")
	write(t, path, "\ufeff# 中文\r\n你好\r\n")
	doc, err := s.OpenDocument("笔记/中文.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != path || doc.Content != "# 中文\n你好\n" || doc.Relative != "笔记/中文.md" {
		t.Fatalf("文档错误：%+v", doc)
	}
	if err := s.CreateFile("笔记/中文.md"); !errors.Is(err, ErrExists) {
		t.Fatalf("预期禁止覆盖：%v", err)
	}
	if err := s.CreateFolder("笔记"); !errors.Is(err, ErrExists) {
		t.Fatalf("预期禁止覆盖：%v", err)
	}
	if err := s.CreateFile("missing/note.md"); err == nil {
		t.Fatal("不应自动创建父文件夹")
	}
	// 上级不存在时给出可读提示，同时仍可按“不存在”判断。
	if err := s.CreateFolder("missing/子目录"); !errors.Is(err, ErrParentMissing) || !errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no such file") {
		t.Fatalf("预期上级文件夹不存在的提示：%v", err)
	}
	if err := s.CreateFile("file.txt"); err == nil {
		t.Fatal("不应创建非 Markdown 文件")
	}
	restored, err := NewAt(data)
	if err != nil {
		t.Fatal(err)
	}
	state := restored.Current()
	if state.Workspace.Path != dir || len(state.Recent) != 1 || state.Recent[0].Path != path {
		t.Fatalf("持久化错误：%+v", state)
	}
	state.Workspace.Path = "被修改"
	state.Recent[0].Path = "被修改"
	if restored.Current().Workspace.Path != dir || restored.Recent()[0].Path != path {
		t.Fatal("调用者修改了内部状态")
	}
	info, err := os.Stat(filepath.Join(data, "workspace.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("配置权限错误：%v", err)
	}
}
func TestRenameNeverOverwritesAndUpdatesRecents(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("此平台暂未实现原生独占重命名")
	}
	s, dir, _ := setup(t)
	s.CreateFolder("notes")
	s.CreateFile("notes/a.md")
	write(t, filepath.Join(dir, "notes/a.md"), "保留内容")
	s.Remember("notes/a.md")
	write(t, filepath.Join(dir, "taken.md"), "目标内容")
	if err := s.Rename("notes/a.md", "taken.md"); !errors.Is(err, ErrExists) {
		t.Fatalf("覆盖检查失败：%v", err)
	}
	if err := s.Rename("notes", "notes/child"); err == nil {
		t.Fatal("允许目录移动到自身内")
	}
	if err := s.Rename("notes", "renamed"); err != nil {
		t.Fatal(err)
	}
	if s.Recent()[0].Path != filepath.Join(dir, "renamed/a.md") {
		t.Fatal("近期文档未跟随目录重命名")
	}
	if err := s.Rename("renamed/a.md", "renamed/b.md"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "renamed/b.md"))
	if err != nil || string(raw) != "保留内容" {
		t.Fatal("重命名丢失内容")
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "taken.md"))
	if string(raw) != "目标内容" {
		t.Fatal("目标被覆盖")
	}
	// 绕过服务预检查直接验证系统的原子禁止覆盖保证。
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := renameExclusive(root, "renamed/b.md", "taken.md"); !errors.Is(err, ErrExists) {
		t.Fatalf("原生独占操作允许覆盖：%v", err)
	}
}
func TestReadValidationAndLimits(t *testing.T) {
	s, dir, _ := setup(t)
	for name, content := range map[string]string{"invalid.md": string([]byte{0xff}), "nul.md": "a\x00b", "big.md": strings.Repeat("a", MaxDocumentBytes+1)} {
		write(t, filepath.Join(dir, name), content)
		if _, err := s.OpenDocument(name); err == nil {
			t.Fatalf("未拒绝 %s", name)
		}
	}
	s.CreateFolder("folder.md")
	if _, err := s.OpenDocument("folder.md"); err == nil {
		t.Fatal("允许读取目录")
	}
}
func TestScanBudgetIncludesFilteredEntries(t *testing.T) {
	s, dir, _ := setup(t)
	for i := 0; i <= MaxScanEntries; i++ {
		write(t, filepath.Join(dir, fmt.Sprintf(".hidden-%d", i)), "")
	}
	if _, err := s.Tree(); !errors.Is(err, ErrScanLimit) {
		t.Fatalf("扫描未受到限制：%v", err)
	}
}
func TestRecentBoundedDeduplicatedAndAuthorized(t *testing.T) {
	s, dir, data := setup(t)
	for i := 0; i < MaxRecent+2; i++ {
		name := fmt.Sprintf("%d.md", i)
		write(t, filepath.Join(dir, name), "")
		if err := s.Remember(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Remember("2.md"); err != nil {
		t.Fatal(err)
	}
	recent := s.Recent()
	if len(recent) != MaxRecent || recent[0].Name != "2.md" || recent[0].OpenedAt.IsZero() {
		t.Fatal("近期排序、去重或数量限制失败")
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	write(t, outside, "授权文档")
	if err := s.Remember(outside); err == nil {
		t.Fatal("相对路径入口允许绝对路径")
	}
	if err := s.RememberAuthorized(outside); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewAt(data)
	if err != nil || reloaded.Recent()[0].Name != "outside.md" {
		t.Fatalf("授权记录未保存：%v", err)
	}
}
func TestAtomicHTMLAndRejectLinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.html")
	write(t, path, "旧 HTML")
	if err := WriteHTML(path, "<!doctype html><p>中文</p>"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "<!doctype html><p>中文</p>" {
		t.Fatal("导出内容错误")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("导出未保留权限")
	}
	link := filepath.Join(dir, "link.html")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteHTML(link, "覆盖"); err == nil {
		t.Fatal("导出允许符号链接")
	}
	if err := WriteHTML(filepath.Join(dir, "note.md"), "覆盖"); err == nil {
		t.Fatal("导出允许非 HTML 扩展名")
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".leafmark-") {
			t.Fatal("临时文件未清理")
		}
	}
}
func TestConcurrentCreatesAndRecents(t *testing.T) {
	s, _, data := setup(t)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.CreateFile("same.md") }()
	}
	wg.Wait()
	close(results)
	created := 0
	for err := range results {
		if err == nil {
			created++
		} else if !errors.Is(err, ErrExists) {
			t.Fatal(err)
		}
	}
	if created != 1 {
		t.Fatalf("成功创建次数：%d", created)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Remember("same.md"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	restored, err := NewAt(data)
	if err != nil || len(restored.Recent()) != 1 {
		t.Fatalf("并发配置持久化错误：%v", err)
	}
}
func TestMissingWorkspaceAndFailedPersistence(t *testing.T) {
	s, err := NewAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tree(); !errors.Is(err, ErrNoWorkspace) {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	write(t, blocked, "保留")
	s.dataDir = blocked
	if _, err := s.SelectFolder(t.TempDir()); err == nil {
		t.Fatal("应报告配置写入失败")
	}
	if s.Current().Workspace != nil {
		t.Fatal("写入失败仍改变了工作区")
	}
	raw, _ := os.ReadFile(blocked)
	if string(raw) != "保留" {
		t.Fatal("配置写入删除了用户文件")
	}
}
