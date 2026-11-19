package documents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestExportCopyPreservesDocumentState(t *testing.T) {
	s := NewStore("未命名.md", "原文")
	doc := s.Current()
	if err := s.Draft(doc.ID, "草稿"); err != nil {
		t.Fatal(err)
	}
	before := s.Current()
	path := filepath.Join(t.TempDir(), "copy.md")
	if err := s.ExportCopy(doc.ID, "# 中文\n导出内容", path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "# 中文\n导出内容" {
		t.Fatalf("副本内容错误：%v", err)
	}
	if !reflect.DeepEqual(before, s.Current()) {
		t.Fatal("导出改变了当前文档")
	}
	if s.saved != "原文" {
		t.Fatal("导出改变了已保存摘要状态")
	}
	if err := s.ExportCopy("过期", "内容", path); !errors.Is(err, ErrStale) {
		t.Fatalf("未拒绝过期标签：%v", err)
	}
}
func TestExportCopyRejectsAllOpenedPathsAndAliases(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.md"), filepath.Join(dir, "second.md")
	os.WriteFile(first, []byte("原文一"), 0600)
	os.WriteFile(second, []byte("原文二"), 0600)
	s := NewStore("", "")
	if _, err := s.Load(first); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Load(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, second} {
		if err := s.ExportCopy(doc.ID, "覆盖", path); err == nil {
			t.Fatalf("允许覆盖已打开目标：%s", path)
		}
	}
	alias := filepath.Join(dir, "alias.md")
	if err := os.Link(first, alias); err == nil {
		if err := s.ExportCopy(doc.ID, "覆盖", alias); err == nil {
			t.Fatal("允许覆盖已打开文件的硬链接")
		}
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(first, link); err == nil {
		if err := s.ExportCopy(doc.ID, "覆盖", link); err == nil {
			t.Fatal("允许覆盖符号链接目标")
		}
	}
	for path, expected := range map[string]string{first: "原文一", second: "原文二"} {
		raw, _ := os.ReadFile(path)
		if string(raw) != expected {
			t.Fatal("保护失败，文件被覆盖")
		}
	}
	if err := s.ExportCopy(doc.ID, string([]byte{0xff}), filepath.Join(dir, "invalid.md")); err == nil {
		t.Fatal("允许无效 UTF-8")
	}
	if err := s.ExportCopy(doc.ID, "内容", filepath.Join(dir, "copy.txt")); err == nil {
		t.Fatal("允许其他扩展名")
	}
}

func TestExportCopyNormalErrorsAndPreservesBOMCRLF(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "bom.md")
	otherPath := filepath.Join(dir, "other.md")
	if err := os.WriteFile(srcPath, []byte("\ufeff首行\r\n次行\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherPath, []byte("另一文档\n"), 0600); err != nil {
		t.Fatal(err)
	}

	s := NewStore("", "")
	other, err := s.Load(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.Load(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	// 正常导出 .markdown 副本，不改变当前文档的 BOM/CRLF 状态。
	exportPath := filepath.Join(dir, "copy.markdown")
	if err := s.ExportCopy(doc.ID, "副本一行\n副本二行\n", exportPath); err != nil {
		t.Fatalf("正常导出 .markdown 副本失败：%v", err)
	}
	exported, err := os.ReadFile(exportPath)
	if err != nil || string(exported) != "副本一行\n副本二行\n" {
		t.Fatalf("导出副本内容错误：%q %v", exported, err)
	}

	// 错误路径：相对路径、非当前标签 ID、导出目标是目录、导出到当前或其他已打开文档。
	if err := s.ExportCopy(doc.ID, "内容", "relative.md"); err == nil {
		t.Fatal("相对路径导出应被拒绝")
	}
	if err := s.ExportCopy(other.ID, "内容", filepath.Join(dir, "bg.md")); !errors.Is(err, ErrStale) {
		t.Fatalf("非当前标签导出应返回 ErrStale，实际 %v", err)
	}
	dirTarget := filepath.Join(dir, "folder.md")
	if err := os.Mkdir(dirTarget, 0755); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportCopy(doc.ID, "内容", dirTarget); err == nil {
		t.Fatal("导出目标为目录时应被拒绝")
	}
	for _, opened := range []string{srcPath, otherPath} {
		if err := s.ExportCopy(doc.ID, "覆盖", opened); err == nil {
			t.Fatalf("导出到已打开文档 %s 应被拒绝", opened)
		}
	}

	// 导出后再次保存原文档，原文档的 BOM+CRLF 编码仍被完整保留。
	if _, err := s.Save(doc.ID, "首行\n新增行\n", srcPath); err != nil {
		t.Fatal(err)
	}
	savedRaw, err := os.ReadFile(srcPath)
	if err != nil || string(savedRaw) != "\ufeff首行\r\n新增行\r\n" {
		t.Fatalf("导出后原文档保存未保留 BOM+CRLF：%q %v", savedRaw, err)
	}

	// 并发 ExportCopy 与 Get/Draft/List 无竞态。
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			out := filepath.Join(dir, fmt.Sprintf("concurrent-%d.md", idx))
			if err := s.ExportCopy(doc.ID, fmt.Sprintf("并发导出-%d\n", idx), out); err != nil {
				errs <- err
				return
			}
			if _, ok := s.Get(doc.ID); !ok {
				errs <- errors.New("并发 Get 失败")
				return
			}
			if len(s.List()) < 2 {
				errs <- errors.New("并发 List 数量异常")
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
