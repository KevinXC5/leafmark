package richtext

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// markedInline 只供模型识别，不进入 HTML 渲染器。
type markedInline struct {
	ast.BaseInline
	mark  Mark
	value string
}

var kindMarkedInline = ast.NewNodeKind("LeafmarkInlineMark")

func (n *markedInline) Kind() ast.NodeKind            { return kindMarkedInline }
func (n *markedInline) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type highlightProcessor struct{}

func (*highlightProcessor) IsDelimiter(b byte) bool { return b == '=' }
func (*highlightProcessor) CanOpenCloser(a, b *parser.Delimiter) bool {
	return a.Char == b.Char && a.Length >= 2 && b.Length >= 2
}
func (*highlightProcessor) OnMatch(int) ast.Node { return &markedInline{mark: MarkHighlight} }

type inlineMarkParser struct{}

func (*inlineMarkParser) Trigger() []byte                     { return []byte{'=', '~', '^'} }
func (*inlineMarkParser) CloseBlock(ast.Node, parser.Context) {}
func (*inlineMarkParser) Parse(parent ast.Node, reader text.Reader, pc parser.Context) ast.Node {
	line, segment := reader.PeekLine()
	if line[0] == '=' {
		d := parser.ScanDelimiter(line, reader.PrecendingCharacter(), 2, &highlightProcessor{})
		if d == nil || d.OriginalLength != 2 {
			return nil
		}
		d.Segment = segment.WithStop(segment.Start + 2)
		reader.Advance(2)
		pc.PushDelimiter(d)
		return d
	}
	ch := line[0]
	if ch == '~' && len(line) > 1 && line[1] == '~' {
		return nil // 双波浪线交给 GFM 删除线。
	}
	// 上下标沿用旧版纯文本语义，禁止未转义空白，避免把普通波浪线变成删除线。
	for i := 1; i < len(line); {
		if line[i] == ch {
			if i == 1 {
				break
			}
			value := unescapeScript(line[1:i])
			mark := MarkSup
			if ch == '~' {
				mark = MarkSub
			}
			reader.Advance(i + 1)
			return &markedInline{mark: mark, value: value}
		}
		if line[i] == '\\' && i+1 < len(line) {
			i += 2
			continue
		}
		if line[i] == '`' {
			j := i
			for j < len(line) && line[j] == '`' {
				j++
			}
			if end := bytes.Index(line[j:], line[i:j]); end >= 0 {
				i = j + end + j - i
				continue
			}
		}
		r, size := utf8.DecodeRune(line[i:])
		if unicode.IsSpace(r) {
			break
		}
		i += size
	}
	reader.Advance(1)
	return ast.NewTextSegment(segment.WithStop(segment.Start + 1))
}

// unescapeScript 与旧版上下标规则一致，只额外允许转义空格。
func unescapeScript(data []byte) string {
	var out bytes.Buffer
	for i := 0; i < len(data); i++ {
		if data[i] == '\\' && i+1 < len(data) && (isASCIIPunct(data[i+1]) || data[i+1] == ' ') {
			i++
		}
		out.WriteByte(data[i])
	}
	return out.String()
}

func inlineMarkParsers() parser.Option {
	return parser.WithInlineParsers(util.Prioritized(&inlineMarkParser{}, 450))
}
