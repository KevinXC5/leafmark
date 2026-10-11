// Package assets 提供文档目录内的图片读取与原生授权图片导入。
package assets

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
)

const MaxImageBytes = 8 << 20
const MaxImagePixels = 16 << 20

var ErrInvalidPath = errors.New("图片路径必须位于文档目录内，且不能包含符号链接")

type ImportResult struct {
	Path    string `json:"path"`
	DataURI string `json:"dataURI"`
}

// ReadImage 的文档路径只能由调用方从已打开文档的授权记录取得。
func ReadImage(documentPath, resource string) (string, error) {
	if documentPath == "" || !filepath.IsAbs(documentPath) {
		return "", ErrInvalidPath
	}
	relative, err := resourcePath(resource)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(filepath.Dir(documentPath))
	if err != nil {
		return "", err
	}
	defer root.Close()
	u, _ := url.Parse(resource)
	return readResource(root, filepath.FromSlash(u.Path), relative)
}

// FindImage 先按相对路径读取；文件不存在时按 Obsidian 的习惯，在文档目录和所属工作区里
// 按文件名或路径后缀查找。workspaceRoot 只能来自工作区的授权记录，文档不在其中时不搜索它。
func FindImage(documentPath, workspaceRoot, resource string) (string, error) {
	uri, err := ReadImage(documentPath, resource)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return uri, err
	}
	relative, pathErr := resourcePath(resource)
	if pathErr != nil {
		return "", pathErr
	}
	dir := filepath.Dir(documentPath)
	dirs := []string{dir}
	if rel, relErr := filepath.Rel(workspaceRoot, dir); workspaceRoot != "" && filepath.IsAbs(workspaceRoot) && relErr == nil && rel != "." && filepath.IsLocal(rel) {
		dirs = append(dirs, workspaceRoot)
	}
	for _, dir := range dirs {
		root, openErr := os.OpenRoot(dir)
		if openErr != nil {
			continue
		}
		found, ok := locate(root, relative)
		if ok {
			uri, err = readResource(root, found, found)
		}
		root.Close()
		if ok {
			return uri, err
		}
	}
	return "", err
}

const maxSearchEntries = 10000
const maxSearchDepth = 64

// locate 在 root 下找文件名或路径后缀与 relative 相同的普通文件，不区分大小写，
// 多个匹配时取层级最浅的。隐藏目录、node_modules 和符号链接不进入。
func locate(root *os.Root, relative string) (string, bool) {
	want := "/" + strings.ToLower(filepath.ToSlash(relative))
	best, bestDepth, seen := "", 0, 0
	_ = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == "." {
			return nil
		}
		if seen++; seen > maxSearchEntries {
			return fs.SkipAll
		}
		depth := strings.Count(p, "/")
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || depth >= maxSearchDepth {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.HasSuffix("/"+strings.ToLower(p), want) {
			return nil
		}
		if best == "" || depth < bestDepth || depth == bestDepth && p < best {
			best, bestDepth = p, depth
		}
		return nil
	})
	return filepath.FromSlash(best), best != ""
}

// readResource 读取 root 下的 relative。written 是清理前的路径，用来检查被 .. 消去的片段。
func readResource(root *os.Root, written, relative string) (string, error) {
	// 清理路径前也检查被 .. 消去的片段，避免符号链接藏在规范化路径中。
	prefix := ""
	for _, part := range strings.Split(written, string(filepath.Separator)) {
		if prefix == "" {
			prefix = part
		} else {
			prefix += string(filepath.Separator) + part
		}
		info, err := root.Lstat(prefix)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", ErrInvalidPath
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", ErrInvalidPath
		}
	}
	parent, err := directory(root, filepath.Dir(relative), false)
	if err != nil {
		return "", err
	}
	defer parent.Close()
	f, err := openRegular(parent, filepath.Base(relative))
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, uri, _, err := readImage(f)
	return uri, err
}

func resourcePath(resource string) (string, error) {
	u, err := url.Parse(resource)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(resource, "#") {
		return "", ErrInvalidPath
	}
	p := u.Path
	if p == "" || strings.ContainsAny(p, "\\\x00:") || strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}
	p = filepath.Clean(filepath.FromSlash(p))
	if !filepath.IsLocal(p) || p == "." {
		return "", ErrInvalidPath
	}
	return p, nil
}

// 逐级固定目录句柄，并比较打开前后的身份；os.Root 保证即使发生替换也不会逃逸根目录。
func directory(root *os.Root, relative string, create bool) (*os.Root, error) {
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if relative == "." {
		return current, nil
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		info, err := current.Lstat(part)
		if create && errors.Is(err, os.ErrNotExist) {
			if err = current.Mkdir(part, 0755); err != nil && !errors.Is(err, os.ErrExist) {
				current.Close()
				return nil, err
			}
			info, err = current.Lstat(part)
		}
		if err != nil {
			current.Close()
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			current.Close()
			return nil, ErrInvalidPath
		}
		next, err := current.OpenRoot(part)
		current.Close()
		if err != nil {
			return nil, err
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			next.Close()
			return nil, ErrInvalidPath
		}
		current = next
	}
	return current, nil
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalidPath
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		f.Close()
		return nil, ErrInvalidPath
	}
	return f, nil
}

func readImage(f *os.File) ([]byte, string, string, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, "", "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", "", errors.New("请选择普通图片文件")
	}
	if info.Size() > MaxImageBytes {
		return nil, "", "", errors.New("图片不能超过 8 MB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxImageBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(raw) > MaxImageBytes {
		return nil, "", "", errors.New("图片不能超过 8 MB")
	}
	if isSVG(raw) {
		return raw, "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(raw), "svg", nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}[format]
	if err != nil || mime == "" || config.Width <= 0 || config.Height <= 0 {
		return nil, "", "", errors.New("请选择有效的 PNG、JPEG、GIF、WebP 或 SVG 图片")
	}
	// 先检查像素数，再实际解码，避免压缩图片造成过量内存分配。
	if config.Width > MaxImagePixels/config.Height {
		return nil, "", "", errors.New("图片像素数过大")
	}
	if _, decoded, err := image.Decode(bytes.NewReader(raw)); err != nil || decoded != format {
		return nil, "", "", errors.New("图片内容损坏")
	}
	return raw, "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), format, nil
}

// isSVG 检查根元素是否是 SVG 命名空间下的 svg，只读到根元素为止。
func isSVG(raw []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == "svg" && start.Name.Space == "http://www.w3.org/2000/svg"
		}
	}
}

// ReadAuthorizedImage 的 source 必须来自原生选择框，不能由前端直接指定。
func ReadAuthorizedImage(source string) (string, error) {
	_, uri, _, err := readSourceImage(source)
	return uri, err
}

// 原生授权读取和导入共用文件检查与解码，拒绝源文件符号链接。
func readSourceImage(source string) ([]byte, string, string, error) {
	sourceRoot, err := os.OpenRoot(filepath.Dir(source))
	if err != nil {
		return nil, "", "", err
	}
	defer sourceRoot.Close()
	f, err := openRegular(sourceRoot, filepath.Base(source))
	if err != nil {
		return nil, "", "", err
	}
	defer f.Close()
	return readImage(f)
}

// ImportImage 的 source 必须来自原生选择框；原始字节只读取一次，用于验证、复制与返回。
func ImportImage(documentPath, source string) (*ImportResult, error) {
	raw, uri, format, err := readSourceImage(source)
	if err != nil {
		return nil, err
	}
	result := &ImportResult{DataURI: uri}
	if documentPath == "" {
		return result, nil
	}
	if !filepath.IsAbs(documentPath) {
		return nil, ErrInvalidPath
	}
	root, err := os.OpenRoot(filepath.Dir(documentPath))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	base := strings.TrimSuffix(filepath.Base(documentPath), filepath.Ext(documentPath))
	if base == "" || base == "." || base == ".." {
		return nil, ErrInvalidPath
	}
	relative := filepath.Join("assets", base)
	parent, err := directory(root, relative, true)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	name := filepath.Base(source)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	// 保留匹配实际格式的原文件名；伪装或缺失的扩展名按真实格式修正。
	actualExt := strings.ToLower(ext)
	if actualExt != "."+format && !(format == "jpeg" && actualExt == ".jpg") {
		ext = map[string]string{"png": ".png", "jpeg": ".jpg", "gif": ".gif", "webp": ".webp", "svg": ".svg"}[format]
	}
	for suffix := 0; ; suffix++ {
		name = stem + ext
		if suffix > 0 {
			name = fmt.Sprintf("%s-%d%s", stem, suffix, ext)
		}
		// O_EXCL 原子拒绝覆盖，包括现有符号链接和并发导入创建的文件。
		out, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		_, writeErr := out.Write(raw)
		if writeErr == nil {
			writeErr = out.Sync()
		}
		closeErr := out.Close()
		if writeErr != nil || closeErr != nil {
			parent.Remove(name)
			return nil, errors.Join(writeErr, closeErr)
		}
		path := "./" + filepath.ToSlash(filepath.Join(relative, name))
		result.Path = (&url.URL{Path: path}).String()
		result.Path = strings.NewReplacer("(", "%28", ")", "%29").Replace(result.Path)
		return result, nil
	}
}
