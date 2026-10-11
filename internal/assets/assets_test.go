package assets

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func imageBytes(t *testing.T, format string) []byte {
	t.Helper()
	if format == "webp" {
		// 来自 golang.org/x/image 的 gopher-doc.1bpp.lossless.webp，内嵌以免测试依赖模块缓存路径。
		raw, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&b, img)
	case "jpeg":
		err = jpeg.Encode(&b, img, nil)
	case "gif":
		err = gif.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFormatsAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"png", "jpeg", "gif", "webp"} {
		t.Run(format, func(t *testing.T) {
			raw := imageBytes(t, format)
			put(t, filepath.Join(dir, "assets", "图片 (1).bin"), raw)
			uri, err := ReadImage(filepath.Join(dir, "note.md"), "./assets/../assets/%E5%9B%BE%E7%89%87%20%281%29.bin")
			if err != nil {
				t.Fatal(err)
			}
			if want := "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(raw); uri != want {
				t.Fatalf("data URI 不匹配：%s", uri)
			}
			source := filepath.Join(dir, "assets", "图片 (1).bin")
			authorizedURI, err := ReadAuthorizedImage(source)
			if err != nil || authorizedURI != uri {
				t.Fatalf("原生授权读取结果不匹配：%v", err)
			}
			imported, err := ImportImage("", source)
			if err != nil {
				t.Fatal(err)
			}
			if imported.DataURI != uri || imported.Path != "" {
				t.Fatalf("未保存导入结果不匹配：%+v", imported)
			}
		})
	}
}

func TestRejectUnsafeResources(t *testing.T) {
	dir := t.TempDir()
	for _, resource := range []string{"", ".", "../outside.png", "assets/../../outside.png", "%2e%2e/outside.png", "/tmp/a.png", "//host/a.png", "https://host/a.png", "data:image/png;base64,a", "file:///tmp/a", "C:/a.png", `assets\a.png`, "a%00.png", "a.png?x", "a.png?", "a.png#x", "a.png#", "%zz"} {
		t.Run(resource, func(t *testing.T) {
			if _, err := ReadImage(filepath.Join(dir, "note.md"), resource); err == nil {
				t.Fatal("应拒绝不安全资源路径")
			}
		})
	}
	if _, err := ReadImage("", "a.png"); err == nil {
		t.Fatal("未保存文档应拒绝相对读取")
	}
}

func TestRejectSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	raw := imageBytes(t, "png")
	put(t, filepath.Join(dir, "real.png"), raw)
	put(t, filepath.Join(outside, "real.png"), raw)
	for name, target := range map[string]string{"file.png": "real.png", "inside": ".", "outside": outside} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, resource := range []string{"file.png", "inside/real.png", "outside/real.png", "inside/../real.png", "outside/../real.png"} {
		if _, err := ReadImage(filepath.Join(dir, "note.md"), resource); err == nil {
			t.Fatalf("应拒绝符号链接 %s", resource)
		}
	}
	if _, err := ReadAuthorizedImage(filepath.Join(dir, "file.png")); err == nil {
		t.Fatal("原生授权读取应拒绝源文件符号链接")
	}
	if _, err := ReadAuthorizedImage(dir); err == nil {
		t.Fatal("原生授权读取应拒绝目录")
	}
	if _, err := ImportImage("", filepath.Join(dir, "file.png")); err == nil {
		t.Fatal("应拒绝源文件符号链接")
	}
	if err := os.Symlink(outside, filepath.Join(dir, "assets")); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportImage(filepath.Join(dir, "note.md"), filepath.Join(dir, "real.png")); err == nil {
		t.Fatal("应拒绝导入目录符号链接")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 1 {
		t.Fatal("不得向外部目录写入")
	}
}

func TestImageValidationAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	valid := imageBytes(t, "png")
	// 三个入口必须执行相同的格式、大小和解码校验。
	check := func(wantValid bool) {
		t.Helper()
		_, relativeErr := ReadImage(filepath.Join(dir, "note.md"), "a.png")
		_, authorizedErr := ReadAuthorizedImage(path)
		_, importErr := ImportImage("", path)
		for name, err := range map[string]error{"相对读取": relativeErr, "授权读取": authorizedErr, "导入": importErr} {
			if (err == nil) != wantValid {
				t.Fatalf("%s 校验结果不匹配：%v", name, err)
			}
		}
	}
	for _, raw := range [][]byte{[]byte("<svg></svg>"), valid[:len(valid)-15], bytes.Repeat([]byte{'x'}, MaxImageBytes+1)} {
		put(t, path, raw)
		check(false)
	}
	// 恰好达到 8 MB 的有效图片应被接受。
	put(t, path, append(valid, make([]byte, MaxImageBytes-len(valid))...))
	check(true)
	var large bytes.Buffer
	if err := png.Encode(&large, image.NewGray(image.Rect(0, 0, 4097, 4096))); err != nil {
		t.Fatal(err)
	}
	put(t, path, large.Bytes())
	check(false)
}

func TestImportAndConcurrentNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), "图片 (1).png")
	raw := imageBytes(t, "png")
	put(t, source, raw)
	doc := filepath.Join(dir, "笔记.md")
	first, err := ImportImage(doc, source)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != "./assets/%E7%AC%94%E8%AE%B0/%E5%9B%BE%E7%89%87%20%281%29.png" {
		t.Fatalf("Markdown 路径不匹配：%s", first.Path)
	}
	uri, err := ReadImage(doc, first.Path)
	if err != nil || uri != first.DataURI {
		t.Fatalf("导入结果无法回读：%v", err)
	}
	decoded, _ := url.PathUnescape(first.Path)
	put(t, filepath.Join(dir, filepath.FromSlash(decoded)), []byte("保留现有文件"))
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := ImportImage(doc, source)
			if err != nil {
				t.Error(err)
				return
			}
			results <- result.Path
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for path := range results {
		if seen[path] || path == first.Path {
			t.Fatalf("路径重复：%s", path)
		}
		seen[path] = true
		decoded, _ := url.PathUnescape(path)
		copied, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(decoded)))
		if err != nil || !bytes.Equal(copied, raw) {
			t.Fatalf("复制字节不匹配：%v", err)
		}
	}
	if len(seen) != 8 {
		t.Fatalf("导入数量不足：%d", len(seen))
	}
	kept, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(decoded)))
	if err != nil || string(kept) != "保留现有文件" {
		t.Fatal("现有图片被覆盖")
	}
}

func TestFindImageByNameAndSVG(t *testing.T) {
	put := func(t *testing.T, path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		put(t, path, data)
	}
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "学习")
	doc := filepath.Join(dir, "教程.md")
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4"/></svg>`)
	put(t, filepath.Join(dir, "assets", "教程", "架构 图.drawio.svg"), svg)
	put(t, filepath.Join(dir, "assets", "其他", "deep", "a.png"), imageBytes(t, "png"))
	put(t, filepath.Join(dir, "assets", "教程", "a.png"), imageBytes(t, "png"))
	put(t, filepath.Join(workspace, "附件", "shared.PNG"), imageBytes(t, "png"))
	put(t, filepath.Join(dir, ".trash", "hidden.png"), imageBytes(t, "png"))
	outside := filepath.Join(t.TempDir(), "outside.png")
	put(t, outside, imageBytes(t, "png"))
	if err := os.Symlink(outside, filepath.Join(dir, "linked.png")); err != nil {
		t.Fatal(err)
	}

	uri, err := FindImage(doc, workspace, "%E6%9E%B6%E6%9E%84%20%E5%9B%BE.drawio.svg")
	if err != nil || !bytes.HasPrefix([]byte(uri), []byte("data:image/svg+xml;base64,")) {
		t.Fatalf("应按文件名找到 SVG：%v %.40s", err, uri)
	}
	// 同名文件取层级最浅的；路径后缀也能匹配。
	shallow, err := FindImage(doc, workspace, "a.png")
	direct, _ := ReadImage(doc, "assets/教程/a.png")
	if err != nil || shallow != direct {
		t.Fatalf("同名文件应取层级最浅的：%v", err)
	}
	if _, err := FindImage(doc, workspace, "deep/a.png"); err != nil {
		t.Fatalf("应按路径后缀查找：%v", err)
	}
	if _, err := FindImage(doc, workspace, "shared.png"); err != nil {
		t.Fatalf("应在工作区里不区分大小写地查找：%v", err)
	}
	if _, err := FindImage(doc, "", "shared.png"); err == nil {
		t.Fatal("没有工作区授权时不应搜索文档目录之外")
	}
	if _, err := FindImage(filepath.Join(t.TempDir(), "外部.md"), workspace, "shared.png"); err == nil {
		t.Fatal("文档不在工作区内时不应搜索工作区")
	}
	for _, resource := range []string{"hidden.png", "linked.png", "outside.png", "../outside.png", "missing.png"} {
		if _, err := FindImage(doc, workspace, resource); err == nil {
			t.Fatalf("不应读到 %s", resource)
		}
	}
	// 不是 SVG 命名空间的 XML 不算图片。
	put(t, filepath.Join(dir, "fake.svg"), []byte("<svg></svg>"))
	if _, err := FindImage(doc, workspace, "fake.svg"); err == nil {
		t.Fatal("应拒绝非 SVG 命名空间的文件")
	}
}
