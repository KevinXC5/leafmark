package richtext

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// wikiImage 是 Obsidian 的图片嵌入 ![[名称]]、![[名称|说明]] 或 ![[名称|宽度]]。
type wikiImage struct {
	ast.BaseInline
	target string
	alt    string
	size   string
}

var kindWikiImage = ast.NewNodeKind("LeafmarkWikiImage")

func (n *wikiImage) Kind() ast.NodeKind         { return kindWikiImage }
func (n *wikiImage) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type wikiImageParser struct{}

var wikiSize = regexp.MustCompile(`^[1-9][0-9]{0,4}(x[1-9][0-9]{0,4})?$`)

// wikiImageExts 是按图片嵌入处理的扩展名；其他 ![[…]] 是笔记或附件嵌入，保持原文。
var wikiImageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}

func (*wikiImageParser) Trigger() []byte                     { return []byte{'!'} }
func (*wikiImageParser) CloseBlock(ast.Node, parser.Context) {}
func (*wikiImageParser) Parse(parent ast.Node, r text.Reader, pc parser.Context) ast.Node {
	line, _ := r.PeekLine()
	if !bytes.HasPrefix(line, []byte("![[")) {
		return nil
	}
	end := bytes.Index(line, []byte("]]"))
	if end <= 3 {
		return nil
	}
	inner := string(line[3:end])
	if strings.ContainsAny(inner, "[]\r\n") {
		return nil
	}
	parts := strings.Split(inner, "|")
	n := &wikiImage{target: strings.TrimSpace(parts[0])}
	if _, ok := wikiURL(n.target); !ok {
		return nil
	}
	if last := strings.TrimSpace(parts[len(parts)-1]); len(parts) > 1 && wikiSize.MatchString(last) {
		n.size = last
		parts = parts[:len(parts)-1]
	}
	n.alt = strings.TrimSpace(strings.Join(parts[1:], "|"))
	r.Advance(end + 2)
	return n
}

// 优先级高于链接解析器，否则 ![[ 会先被当作普通文字和方括号吃掉。
func wikiImageInlines() parser.Option {
	return parser.WithInlineParsers(util.Prioritized(&wikiImageParser{}, 150))
}

// wikiURL 把嵌入里的文件名换成相对地址，逐段转义后与普通图片走同一条读取路径。
func wikiURL(target string) (string, bool) {
	target = strings.TrimLeft(target, "/")
	if target == "" || strings.ContainsAny(target, "\\:#^\x00") || !wikiImageExts[strings.ToLower(path.Ext(target))] {
		return "", false
	}
	segments := strings.Split(target, "/")
	for i, s := range segments {
		if s == "" {
			return "", false
		}
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/"), true
}

// wikiTarget 是 wikiURL 的逆过程；地址被改成嵌入语法写不出的形式时返回 false。
func wikiTarget(address string) (string, bool) {
	target, err := url.PathUnescape(address)
	if err != nil || strings.ContainsAny(target, "|[]\r\n") {
		return "", false
	}
	if back, ok := wikiURL(target); !ok || back != address {
		return "", false
	}
	return target, true
}

// wikiEmbed 写出嵌入语法；不能用嵌入语法表达时返回 false，由调用方改写成普通图片。
func wikiEmbed(address, alt, size string) (string, bool) {
	target, ok := wikiTarget(address)
	if !ok || strings.ContainsAny(alt, "[]\r\n") {
		return "", false
	}
	// 说明的末段形如尺寸时，重新解析会把它当成尺寸。
	if last := alt[strings.LastIndex(alt, "|")+1:]; size == "" && wikiSize.MatchString(strings.TrimSpace(last)) {
		return "", false
	}
	out := "![[" + target
	if alt != "" {
		out += "|" + alt
	}
	if size != "" {
		out += "|" + size
	}
	return out + "]]", true
}

// ImageWidth 返回嵌入语法指定的显示宽度，没有指定时为 0。
func ImageWidth(size string) int {
	w, _, _ := strings.Cut(size, "x")
	n, _ := strconv.Atoi(w)
	return n
}
