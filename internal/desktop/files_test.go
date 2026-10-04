package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"leafmark/internal/documents"
)

func TestOpenPendingOnlyAcceptsMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	note := filepath.Join(dir, "笔记.md")
	upper := filepath.Join(dir, "README.MARKDOWN")
	image := filepath.Join(dir, "图片.png")
	folder := filepath.Join(dir, "目录.md")
	for path, content := range map[string]string{note: "# 笔记\n", upper: "说明\n", image: "png"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	files := &Files{store: documents.NewStore("welcome.md", "")}
	if n := files.requestOpen([]string{image, folder, filepath.Join(dir, "缺失.md")}); n != 0 {
		t.Fatalf("非 Markdown 文件、目录和缺失文件不应排队，实际 %d", n)
	}
	if n := files.requestOpen([]string{note, image, upper}); n != 2 {
		t.Fatalf("应排队 2 份文档，实际 %d", n)
	}
	for _, want := range []string{"# 笔记\n", "说明\n"} {
		doc, err := files.OpenPending()
		if err != nil || doc == nil || doc.Content != want {
			t.Fatalf("按顺序打开排队文档失败：%v %+v", err, doc)
		}
	}
	if doc, err := files.OpenPending(); err != nil || doc != nil {
		t.Fatalf("队列为空时应返回 nil：%v %+v", err, doc)
	}
}
