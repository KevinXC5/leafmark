package assets

import (
	"bytes"
	"encoding/base64"
	"errors"
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

// solidPNG 生成带有唯一颜色的合法 PNG，便于区分不同目录下的同名图片。
func solidPNG(t *testing.T, r, g, b uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: r, G: g, B: b, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadImageRawAndFindImageRaw(t *testing.T) {
	writeFile := func(t *testing.T, path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		put(t, path, data)
	}

	workspace := t.TempDir()
	docDir := filepath.Join(workspace, "笔记", "专题")
	doc := filepath.Join(docDir, "说明.md")
	outside := t.TempDir()

	pngLocal := solidPNG(t, 10, 20, 30)
	pngWorkspace := solidPNG(t, 200, 100, 50)
	svgRaw := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><circle cx="4" cy="4" r="3"/></svg>`)

	writeFile(t, filepath.Join(docDir, "assets", "图 1.png"), pngLocal)
	writeFile(t, filepath.Join(docDir, "diagrams", "结构.svg"), svgRaw)
	writeFile(t, filepath.Join(workspace, "共享资源", "cross.PNG"), pngWorkspace)
	writeFile(t, filepath.Join(outside, "secret.png"), pngLocal)

	// 1. ReadImageRaw：正常读取 PNG 与 SVG，返回原始字节与准确格式。
	gotPNG, format, err := ReadImageRaw(doc, "./assets/%E5%9B%BE%201.png")
	if err != nil || format != "png" || !bytes.Equal(gotPNG, pngLocal) {
		t.Fatalf("ReadImageRaw 读取 PNG 失败：format=%q err=%v", format, err)
	}
	gotSVG, format, err := ReadImageRaw(doc, "diagrams/%E7%BB%93%E6%9E%84.svg")
	if err != nil || format != "svg" || !bytes.Equal(gotSVG, svgRaw) {
		t.Fatalf("ReadImageRaw 读取 SVG 失败：format=%q err=%v", format, err)
	}

	// 2. FindImageRaw：文档目录内按文件名查找 PNG 与 SVG，以及跨工作区按名称不区分大小写查找。
	foundSVG, format, err := FindImageRaw(doc, workspace, "%E7%BB%93%E6%9E%84.svg")
	if err != nil || format != "svg" || !bytes.Equal(foundSVG, svgRaw) {
		t.Fatalf("FindImageRaw 查找 SVG 失败：format=%q err=%v", format, err)
	}
	foundCross, format, err := FindImageRaw(doc, workspace, "cross.png")
	if err != nil || format != "png" || !bytes.Equal(foundCross, pngWorkspace) {
		t.Fatalf("FindImageRaw 跨工作区查找失败：format=%q err=%v", format, err)
	}
	// 无工作区授权或文档不在工作区内时，不应跨目录找到工作区资源。
	if _, _, err := FindImageRaw(doc, "", "cross.png"); err == nil {
		t.Fatal("无工作区授权时 FindImageRaw 不应找到文档目录外的图片")
	}
	if _, _, err := FindImageRaw(filepath.Join(outside, "other.md"), workspace, "cross.png"); err == nil {
		t.Fatal("文档不在工作区内时 FindImageRaw 不应搜索工作区")
	}

	// 3. 路径逃逸与非法文档路径拒绝。
	for _, bad := range []string{"", ".", "../secret.png", "assets/../../secret.png", "%2e%2e/secret.png", "/tmp/a.png", "https://example.com/a.png", "a.png?v=1", "a.png#top", `assets\a.png`} {
		if _, _, err := ReadImageRaw(doc, bad); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("ReadImageRaw 应拒绝路径 %q，实际 %v", bad, err)
		}
		if _, _, err := FindImageRaw(doc, workspace, bad); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("FindImageRaw 应拒绝路径 %q，实际 %v", bad, err)
		}
	}
	if _, _, err := ReadImageRaw("", "图 1.png"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("空文档路径应返回 ErrInvalidPath，实际 %v", err)
	}
	if _, _, err := ReadImageRaw("relative/doc.md", "图 1.png"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("相对文档路径应返回 ErrInvalidPath，实际 %v", err)
	}

	// 4. 符号链接拒绝（包括文件符号链接、目录符号链接以及被 .. 消除的符号链接片段）。
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(docDir, "sym.png")); err != nil {
		t.Skipf("当前环境不支持创建符号链接：%v", err)
	}
	if err := os.Symlink(outside, filepath.Join(docDir, "symdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "ws-symdir")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"sym.png", "symdir/secret.png", "symdir/../assets/图 1.png"} {
		if _, _, err := ReadImageRaw(doc, bad); err == nil {
			t.Fatalf("ReadImageRaw 应拒绝符号链接路径 %q", bad)
		}
		if _, _, err := FindImageRaw(doc, workspace, bad); err == nil {
			t.Fatalf("FindImageRaw 应拒绝符号链接路径 %q", bad)
		}
	}
	if _, _, err := FindImageRaw(doc, workspace, "secret.png"); err == nil {
		t.Fatal("FindImageRaw 不应通过工作区符号链接目录找到外部图片")
	}

	// 5. 超大文件与超大像素拒绝，恰好 8 MB 的合法图片应接受。
	writeFile(t, filepath.Join(docDir, "too-big.png"), bytes.Repeat([]byte{'x'}, MaxImageBytes+1))
	if _, _, err := ReadImageRaw(doc, "too-big.png"); err == nil {
		t.Fatal("ReadImageRaw 应拒绝超过 8 MB 的文件")
	}
	if _, _, err := FindImageRaw(doc, workspace, "too-big.png"); err == nil {
		t.Fatal("FindImageRaw 应拒绝超过 8 MB 的文件")
	}
	exactLimit := append(append([]byte(nil), pngLocal...), make([]byte, MaxImageBytes-len(pngLocal))...)
	writeFile(t, filepath.Join(docDir, "exact-8mb.png"), exactLimit)
	if got, fmtName, err := ReadImageRaw(doc, "exact-8mb.png"); err != nil || fmtName != "png" || len(got) != MaxImageBytes {
		t.Fatalf("恰好 8 MB 的有效 PNG 应被接受：fmt=%q len=%d err=%v", fmtName, len(got), err)
	}
	var hugePixel bytes.Buffer
	if err := png.Encode(&hugePixel, image.NewGray(image.Rect(0, 0, 4097, 4096))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(docDir, "huge-pixel.png"), hugePixel.Bytes())
	if _, _, err := ReadImageRaw(doc, "huge-pixel.png"); err == nil {
		t.Fatal("ReadImageRaw 应拒绝像素数超限的图片")
	}
	if _, _, err := FindImageRaw(doc, workspace, "huge-pixel.png"); err == nil {
		t.Fatal("FindImageRaw 应拒绝像素数超限的图片")
	}

	// 6. 损坏图片与伪装 SVG 拒绝。
	corruptCases := map[string][]byte{
		"garbage.png":    []byte("not a valid image payload"),
		"truncated.png":  pngLocal[:12], // 截断在 IHDR 之前，无法解析图片配置
		"bad-ns.svg":     []byte(`<svg width="4" height="4"></svg>`),
		"broken-xml.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"`),
	}
	for name, payload := range corruptCases {
		writeFile(t, filepath.Join(docDir, "bad", name), payload)
		if _, _, err := ReadImageRaw(doc, "bad/"+name); err == nil {
			t.Fatalf("ReadImageRaw 应拒绝损坏图片 %s", name)
		}
		if _, _, err := FindImageRaw(doc, workspace, name); err == nil {
			t.Fatalf("FindImageRaw 应拒绝损坏图片 %s", name)
		}
	}
}

func TestFindImageDocumentDirPriorityAndSkipDir(t *testing.T) {
	writeFile := func(t *testing.T, path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		put(t, path, data)
	}

	workspace := t.TempDir()
	docDir := filepath.Join(workspace, "合集", "子目录")
	doc := filepath.Join(docDir, "文章.md")

	docLocalPNG := solidPNG(t, 11, 22, 33)
	wsRootPNG := solidPNG(t, 44, 55, 66)
	wsOtherShallowPNG := solidPNG(t, 77, 88, 99)
	wsOtherDeepPNG := solidPNG(t, 111, 122, 133)

	// 文档目录内的同名图片层级较深，而工作区根目录也有同名图片；
	// 文档目录必须优先于工作区其他目录。
	writeFile(t, filepath.Join(docDir, "assets", "deep", "same.png"), docLocalPNG)
	writeFile(t, filepath.Join(workspace, "same.png"), wsRootPNG)

	// 文档目录内没有 only-in-ws.png，工作区其他目录有两个层级不同的同名文件，应取最浅的。
	writeFile(t, filepath.Join(workspace, "shared", "only-in-ws.png"), wsOtherShallowPNG)
	writeFile(t, filepath.Join(workspace, "shared", "sub", "only-in-ws.png"), wsOtherDeepPNG)

	raw, format, err := FindImageRaw(doc, workspace, "same.png")
	if err != nil || format != "png" || !bytes.Equal(raw, docLocalPNG) {
		t.Fatalf("FindImageRaw 应优先返回文档目录内的图片：format=%q err=%v", format, err)
	}
	uri, err := FindImage(doc, workspace, "same.png")
	wantURI := encodeDataURI(docLocalPNG, "png")
	if err != nil || uri != wantURI {
		t.Fatalf("FindImage 应优先返回文档目录内的图片：err=%v", err)
	}

	// 指定工作区子目录路径后缀时，文档目录查不到，回退到工作区其他目录仍能命中。
	rawOther, format, err := FindImageRaw(doc, workspace, "only-in-ws.png")
	if err != nil || format != "png" || !bytes.Equal(rawOther, wsOtherShallowPNG) {
		t.Fatalf("FindImageRaw 回退工作区应返回最浅层匹配：format=%q err=%v", format, err)
	}
	uriOther, err := FindImage(doc, workspace, "only-in-ws.png")
	if err != nil || uriOther != encodeDataURI(wsOtherShallowPNG, "png") {
		t.Fatalf("FindImage 回退工作区结果不匹配：err=%v", err)
	}

	// 直接验证 locate 的 skipDir 跳过已搜索的文档子目录，避免重复遍历。
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writeFile(t, filepath.Join(docDir, "only-in-doc.png"), docLocalPNG)
	if found, ok := locate(root, "only-in-doc.png", "合集/子目录"); ok {
		t.Fatalf("skipDir 应跳过文档子目录，但仍命中了 %q", found)
	}
	if found, ok := locate(root, "only-in-doc.png", ""); !ok || filepath.ToSlash(found) != "合集/子目录/only-in-doc.png" {
		t.Fatalf("未设置 skipDir 时应能命中文档子目录图片：ok=%v found=%q", ok, found)
	}
}
