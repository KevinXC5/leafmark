package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leafmark/internal/documents"
)

func TestAuthorizedSnapshotSurvivesPathReplacement(t *testing.T) {
	s, dir, _ := setup(t)
	path := filepath.Join(dir, "note.md")
	original := string(rune(0xfeff)) + "# 授权\r\n"
	write(t, path, original)
	for _, recent := range []bool{false, true} {
		write(t, path, original)
		var doc *Document
		var err error
		if recent {
			doc, err = s.OpenRecent(path)
		} else {
			doc, err = s.OpenDocument("note.md")
		}
		if err != nil {
			t.Fatal(err)
		}
		if string(doc.Raw) != original {
			t.Fatal("快照不是原始字节")
		}
		encoded, err := json.Marshal(doc)
		if err != nil || strings.Contains(string(encoded), "Raw") || strings.Contains(string(encoded), "raw") {
			t.Fatal("原始字节泄露到前端 JSON")
		}
		write(t, path, "替换后的内容")
		loaded, err := documents.NewStore("", "").LoadSnapshot(doc.Path, doc.Raw)
		if err != nil || loaded.Content != "# 授权\n" {
			t.Fatalf("快照重读了路径：%+v %v", loaded, err)
		}
	}
}
func TestVerifyOpenedRejectsReplacementAndLinks(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	path := filepath.Join(dir, "note.md")
	write(t, path, "授权")
	before, err := root.Lstat("note.md")
	if err != nil {
		t.Fatal(err)
	}
	f, err := root.Open("note.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := verifyOpened(root, "note.md", f, before); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement.md")
	write(t, replacement, "其他文件")
	if err := os.Rename(path, filepath.Join(dir, "old.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("replacement.md", path); err != nil {
		t.Fatal(err)
	}
	// 已打开授权句柄对应旧 inode；路径检查必须拒绝换成链接的名称。
	if err := verifyOpened(root, "note.md", f, before); err == nil {
		t.Fatal("没有拒绝检查后的链接替换")
	}
	newFile, err := root.Open("note.md")
	if err != nil {
		t.Fatal(err)
	}
	defer newFile.Close()
	if err := verifyOpened(root, "note.md", newFile, before); err == nil {
		t.Fatal("没有拒绝打开其他文件的句柄")
	}
}
func TestVerifyRootRejectsReplacement(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "folder")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(dir, filepath.Join(parent, "old")); err != nil {
		t.Skip(err)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := verifyRoot(root, dir); err == nil {
		t.Fatal("根目录被替换后身份校验仍通过")
	}
}
