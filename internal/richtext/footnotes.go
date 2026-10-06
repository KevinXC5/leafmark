package richtext

import (
	"bytes"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type footnoteInline struct {
	ast.BaseInline
	label string
}

var kindFootnoteInline = ast.NewNodeKind("LeafmarkFootnote")

func (n *footnoteInline) Kind() ast.NodeKind         { return kindFootnoteInline }
func (n *footnoteInline) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type footnoteInlineParser struct{}

func (*footnoteInlineParser) Trigger() []byte                     { return []byte{'['} }
func (*footnoteInlineParser) CloseBlock(ast.Node, parser.Context) {}
func (*footnoteInlineParser) Parse(parent ast.Node, r text.Reader, pc parser.Context) ast.Node {
	line, _ := r.PeekLine()
	if !bytes.HasPrefix(line, []byte("[^")) {
		return nil
	}
	end := bytes.IndexByte(line, ']')
	if end <= 2 {
		return nil
	}
	if bytes.ContainsAny(line[2:end], "\r\n[]") {
		return nil
	}
	r.Advance(end + 1)
	return &footnoteInline{label: string(line[2:end])}
}

// goldmark 默认把脚注移动到文末；保留其定义解析器，但禁止搬动节点，以保持原文顺序。
type footnoteDefinitionParser struct{ parser.BlockParser }

func (*footnoteDefinitionParser) Close(ast.Node, text.Reader, parser.Context) {}
func footnoteParsers() parser.Option {
	return parser.WithBlockParsers(util.Prioritized(&footnoteDefinitionParser{extension.NewFootnoteBlockParser()}, 50))
}
func footnoteInlines() parser.Option {
	return parser.WithInlineParsers(util.Prioritized(&footnoteInlineParser{}, 100))
}

// FootnoteTarget 返回定义或首个引用的正文坐标，用于原生编辑器的往返跳转。
func (d *Document) FootnoteTarget(label string, back bool) (int, bool) {
	if back {
		for at := range d.index {
			link := d.LinkAt(at)
			if link != nil && link.Footnote == label {
				return at, true
			}
		}
		return 0, false
	}
	for i, b := range d.blocks {
		if b.Footnote != nil && b.Footnote.Label == label {
			start, _ := d.blockRange(i)
			return start, true
		}
	}
	return 0, false
}

func numberFootnotes(d *Document) {
	numbers := map[string]int{}
	for _, b := range d.blocks {
		for _, r := range b.Runs {
			if r.Link != nil && r.Link.Footnote != "" {
				label := r.Link.Footnote
				if numbers[label] == 0 {
					numbers[label] = len(numbers) + 1
				}
			}
		}
	}
	for i := range d.blocks {
		if f := d.blocks[i].Footnote; f != nil {
			if numbers[f.Label] == 0 {
				numbers[f.Label] = len(numbers) + 1
			}
			f.Number = numbers[f.Label]
		}
	}
}

var _ ast.Node = (*extast.Footnote)(nil)
