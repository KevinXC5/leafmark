package richtext

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// md 只用于结构识别。序列化不经过它，未编辑内容直接回写源码。
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
	),
	goldmark.WithParserOptions(inlineMarkParsers(), footnoteParsers(), footnoteInlines()),
)

// NewPlainText 把全文收成单个代码块。Text 与 Code 都等于 text，
// 围栏和全部尾换行留在正文里，不按 Markdown 解析。
func NewPlainText(text string) *Document {
	d := &Document{clean: true, blocks: []editBlock{{
		Block:    Block{Kind: Code, Code: text},
		source:   text,
		plain:    true,
		indented: false,
	}}}
	d.reindex()
	return d
}

// Parse 把 Markdown 解析成文档。空字符串得到一个空段落。
func Parse(markdown string) *Document {
	front := frontMatterLen(markdown)
	if front == 0 {
		return parseBody(markdown)
	}
	// 文首的 YAML 属性整段原样保留，不按正文解析成分隔线和标题。
	d := parseBody(markdown[front:])
	raw := rawBlock(markdown[:front])
	raw.gap, d.prefix = d.prefix, ""
	if front == len(markdown) {
		d.blocks = nil
	}
	d.blocks = append([]editBlock{raw}, d.blocks...)
	d.reindex()
	return d
}

// frontMatterLen 返回文首 YAML 属性块的字节数，没有时为 0。
// 属性块以首行的 --- 开始，到下一行单独的 --- 或 ... 结束，中间至少有一行内容。
func frontMatterLen(markdown string) int {
	rest, ok := strings.CutPrefix(markdown, "---")
	if !ok {
		return 0
	}
	at := 3
	for line := range strings.SplitAfterSeq(rest, "\n") {
		text := strings.TrimRight(line, " \t\r\n")
		if at == 3 {
			if text != "" {
				return 0
			}
		} else if text == "---" || text == "..." {
			if at == 3+len(strings.SplitAfterN(rest, "\n", 2)[0]) {
				return 0
			}
			return at + len(line)
		}
		at += len(line)
	}
	return 0
}

func parseBody(markdown string) *Document {
	src := []byte(markdown)
	reader := text.NewReader(src)
	root := md.Parser().Parse(reader)

	d := &Document{clean: true}
	var covered int
	nextContainerID := 1
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		start, stop := nodeSpan(n, src)
		if fence, ok := n.(*ast.FencedCodeBlock); ok && fence.Lines().Len() == 0 {
			start, stop = emptyFenceSpan(src, covered)
		}
		if start < 0 || stop < start || stop > len(src) {
			start, stop = covered, len(src)
		}
		if start < covered {
			start = covered
		}
		// 区间算不出来或落在已处理内容之前时不能越界，退化成空区间。
		stop = max(stop, start)
		if start > covered {
			// 块与块之间的空白（含空行）记在前一块上，保证回写逐字节一致。
			gap := string(src[covered:start])
			if len(d.blocks) == 0 {
				if strings.Trim(gap, " \t\r\n") != "" {
					d.blocks = append(d.blocks, rawBlock(gap))
				} else {
					d.prefix += gap
				}
			} else {
				d.blocks[len(d.blocks)-1].gap += gap
			}
		}
		b := convertBlock(n, src, 1)
		b.source = string(src[start:stop])
		if html, isHTML := n.(*ast.HTMLBlock); isHTML {
			if blocks, supported := convertHTML(html, src, &nextContainerID); supported && len(blocks) > 0 {
				group := 0
				if len(blocks) > 1 {
					group = len(d.blocks) + 1
				}
				for j := range blocks {
					blocks[j].group = group
					if j == 0 {
						blocks[j].source = b.source
					}
				}
				d.blocks = append(d.blocks, blocks...)
				covered = stop
				continue
			}
		}
		if expanded, ok := expandContainer(n, src, &nextContainerID); ok {
			group := len(d.blocks) + 1
			for j := range expanded {
				expanded[j].group = group
			}
			expanded[0].source = b.source
			d.blocks = append(d.blocks, expanded...)
		} else if list, ok := n.(*ast.List); ok {
			if items, supported := convertListItems(list, src); supported && listCovers(items, src[start:stop]) {
				d.blocks = append(d.blocks, items...)
			} else {
				d.blocks = append(d.blocks, b)
			}
		} else {
			d.blocks = append(d.blocks, b)
		}
		covered = stop
	}
	if covered < len(src) {
		if len(d.blocks) == 0 {
			d.blocks = append(d.blocks, rawBlock(string(src[covered:])))
		} else {
			d.blocks[len(d.blocks)-1].gap += string(src[covered:])
		}
	}
	if len(d.blocks) == 0 {
		d.blocks = []editBlock{{Block: Block{Kind: Paragraph}}}
	}
	numberFootnotes(d)
	d.reindex()
	return d
}

// listCovers 确认展开后的各项源码与间隙拼起来正好是原列表，否则整表保留。
// 列表区间末尾多出的空行并入最后一项的间隙。
func listCovers(items []editBlock, source []byte) bool {
	var joined strings.Builder
	for _, item := range items {
		joined.WriteString(item.source)
		joined.WriteString(item.gap)
	}
	rest, ok := strings.CutPrefix(string(source), joined.String())
	if !ok || strings.Trim(rest, " \t\r\n") != "" {
		return false
	}
	items[len(items)-1].gap += rest
	return true
}

func rawBlock(s string) editBlock {
	return editBlock{Block: Block{Kind: Raw, Raw: s}, source: s}
}

// nodeSpan 返回节点在源码中的字节区间，尽量含行尾换行。
func nodeSpan(n ast.Node, src []byte) (int, int) {
	switch v := n.(type) {
	case *ast.FencedCodeBlock:
		return fencedSpan(v, src)
	case *ast.CodeBlock:
		return linesSpan(v.Lines(), src, true)
	case *ast.HTMLBlock:
		return htmlSpan(v, src)
	case *ast.ThematicBreak:
		// 分隔线没有行内容，只有起始位置，区间取它所在的整行。
		start := v.Pos()
		if start < 0 || start > len(src) {
			return -1, -1
		}
		for start > 0 && src[start-1] != '\n' {
			start--
		}
		stop := start
		for stop < len(src) && src[stop] != '\n' {
			stop++
		}
		return start, min(stop+1, len(src))
	case *ast.Heading:
		return blockLines(v.Lines(), src)
	case *ast.Paragraph:
		return blockLines(v.Lines(), src)
	case *ast.TextBlock:
		return linesSpan(v.Lines(), src, true)
	case *ast.List:
		return childrenSpan(v, src)
	case *ast.ListItem:
		return listItemSpan(v, src)
	case *ast.Blockquote:
		return childrenSpan(v, src)
	case *extast.Table:
		start, stop := childrenSpan(v, src)
		if stop >= 0 {
			// 单元格区间不含末尾竖线；表格块必须覆盖最后一行，防止尾部被当成 gap。
			for stop < len(src) && (stop == 0 || src[stop-1] != '\n') {
				stop++
			}
		}
		return start, stop
	case *ast.LinkReferenceDefinition:
		return linkRefSpan(v, src)
	default:
		if n.Type() == ast.TypeBlock {
			if n.Lines() != nil && n.Lines().Len() > 0 {
				return blockLines(n.Lines(), src)
			}
			return childrenSpan(n, src)
		}
		return -1, -1
	}
}

// blockLines 把内容行扩成整行，包含行首标记和行尾换行。
func blockLines(lines *text.Segments, src []byte) (int, int) {
	start, stop := linesSpan(lines, src, true)
	if start < 0 {
		return start, stop
	}
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	return start, stop
}

func linesSpan(lines *text.Segments, src []byte, withBreak bool) (int, int) {
	if lines == nil || lines.Len() == 0 {
		return -1, -1
	}
	start := lines.At(0).Start
	last := lines.At(lines.Len() - 1)
	stop := last.Stop
	if withBreak && stop < len(src) && src[stop] == '\n' {
		stop++
		if stop < len(src) && src[stop-2] == '\r' {
			// 区间已经含 \n，\r 在 stop-2 处时一并算进来。
		}
		if stop >= 2 && src[stop-2] == '\r' {
			// \r\n：Start 可能落在 \r 之后，无需回退。
		}
	}
	if start > 0 && src[start-1] == '\r' && (start == stop || src[start] != '\n') {
		start--
	}
	return start, stop
}

func childrenSpan(n ast.Node, src []byte) (int, int) {
	start, stop := -1, -1
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		a, b := nodeSpan(c, src)
		if a < 0 {
			continue
		}
		if start < 0 || a < start {
			start = a
		}
		if b > stop {
			stop = b
		}
	}
	if start < 0 {
		return -1, -1
	}
	// 容器开头的标记（>、-、表格）可能在第一个子节点之前，按行首回退。
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	if stop < len(src) && src[stop] == '\n' {
		stop++
	}
	return start, stop
}

func listItemSpan(n *ast.ListItem, src []byte) (int, int) {
	start, stop := childrenSpan(n, src)
	if start < 0 {
		return -1, -1
	}
	// 标记符与缩进：goldmark 的 Offset 是内容起点，标记在它前面。
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	return start, stop
}

func fencedSpan(n *ast.FencedCodeBlock, src []byte) (int, int) {
	if n.Lines().Len() == 0 && n.Info == nil {
		return -1, -1
	}
	start := -1
	if n.Info != nil {
		start = n.Info.Segment.Start
	} else if n.Lines().Len() > 0 {
		start = n.Lines().At(0).Start
		for start > 0 && src[start-1] != '\n' {
			start--
		}
		if start > 0 {
			start--
		}
	}
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	stop := start
	if n.Lines().Len() > 0 {
		stop = n.Lines().At(n.Lines().Len() - 1).Stop
	}
	// 结束围栏在正文之后，向后扫到下一个非空行结束。
	i := stop
	if i < len(src) && (src[i] == '\n' || src[i] == '\r') {
		if src[i] == '\r' && i+1 < len(src) && src[i+1] == '\n' {
			i += 2
		} else {
			i++
		}
	}
	fenceStart := i
	for i < len(src) && src[i] != '\n' && src[i] != '\r' {
		i++
	}
	line := src[fenceStart:i]
	if isFenceCloser(line) {
		stop = i
		if stop < len(src) && src[stop] == '\r' {
			stop++
		}
		if stop < len(src) && src[stop] == '\n' {
			stop++
		}
	} else if stop < len(src) && src[stop] == '\n' {
		stop++
	}
	return start, stop
}

func isFenceCloser(line []byte) bool {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') && i < 3 {
		i++
	}
	if i >= len(line) {
		return false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return false
	}
	n := 0
	for i < len(line) && line[i] == ch {
		n++
		i++
	}
	if n < 3 {
		return false
	}
	for _, c := range line[i:] {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

func htmlSpan(n *ast.HTMLBlock, src []byte) (int, int) {
	start, stop := linesSpan(n.Lines(), src, true)
	if n.HasClosure() {
		cl := n.ClosureLine
		if cl.Stop > stop {
			stop = cl.Stop
		}
		if stop < len(src) && src[stop] == '\n' {
			stop++
		}
	}
	return start, stop
}

func linkRefSpan(n *ast.LinkReferenceDefinition, src []byte) (int, int) {
	start, stop := linesSpan(n.Lines(), src, true)
	return start, stop
}

func convertBlock(n ast.Node, src []byte, depth int) editBlock {
	switch v := n.(type) {
	case *ast.Heading:
		runs, ok := inlineRuns(v, src)
		if !ok {
			return rawOf(n, src)
		}
		return editBlock{Block: Block{Kind: Heading, Level: v.Level, Runs: runs}}
	case *ast.Paragraph:
		if img, ok := loneImage(v, src); ok {
			return img
		}
		runs, ok := inlineRuns(v, src)
		if !ok {
			return rawOf(n, src)
		}
		return editBlock{Block: Block{Kind: Paragraph, Runs: runs}}
	case *ast.ThematicBreak:
		return editBlock{Block: Block{Kind: Horizontal}}
	case *ast.FencedCodeBlock:
		if strings.EqualFold(string(v.Language(src)), "mermaid") {
			return rawOf(n, src)
		}
		return editBlock{Block: Block{
			Kind: Code,
			Lang: string(v.Language(src)),
			Code: codeText(v.Lines(), src),
		}}
	case *ast.CodeBlock:
		return editBlock{Block: Block{Kind: Code, Code: codeText(v.Lines(), src)}}
	case *ast.Blockquote:
		return convertQuote(v, src, depth)
	case *ast.List:
		return convertList(v, src, depth)
	case *extast.Table:
		return convertTable(v, src)
	case *ast.LinkReferenceDefinition:
		return editBlock{Block: Block{Kind: ReferenceDef, Raw: string(v.Label), URL: string(v.Destination), Title: string(v.Title)}}
	default:
		return rawOf(n, src)
	}
}

func rawOf(n ast.Node, src []byte) editBlock {
	a, b := nodeSpan(n, src)
	s := ""
	if a >= 0 && b >= a && b <= len(src) {
		s = string(src[a:b])
	}
	return editBlock{Block: Block{Kind: Raw, Raw: trimTrailingBreak(s)}}
}

func trimTrailingBreak(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
		if len(s) > 0 && s[len(s)-1] == '\r' {
			s = s[:len(s)-1]
		}
	}
	return s
}

// loneImage 识别独占一行的 ![alt](url)，引用式图片不认。
func loneImage(p *ast.Paragraph, src []byte) (editBlock, bool) {
	var img *ast.Image
	count := 0
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Image:
			img = t
			count++
		case *ast.Text:
			if len(bytes.TrimSpace(t.Value(src))) != 0 {
				return editBlock{}, false
			}
		default:
			return editBlock{}, false
		}
	}
	if count != 1 || img == nil {
		return editBlock{}, false
	}
	alt := string(img.Text(src))
	return editBlock{Block: Block{
		Kind:  Image,
		Alt:   alt,
		URL:   string(img.Destination),
		Title: string(img.Title),
	}}, true
}

var calloutMarker = regexp.MustCompile(`^\[!([A-Za-z][A-Za-z0-9_-]*)\]([+-])?(?:[ \t]+(.*))?$`)

func convertCallout(q *ast.Blockquote, p *ast.Paragraph, src []byte, depth int) (editBlock, bool) {
	if p.Lines().Len() == 0 {
		return editBlock{}, false
	}
	firstLine := p.Lines().At(0)
	first := strings.TrimSpace(string(firstLine.Value(src)))
	match := calloutMarker.FindStringSubmatch(first)
	if match == nil {
		return editBlock{}, false
	}
	var body strings.Builder
	for i := 1; i < p.Lines().Len(); i++ {
		line := p.Lines().At(i)
		body.Write(line.Value(src))
	}
	// 头部不参与正文坐标；正文重新解析行内结构，未知节点仍保留整块源码。
	var runs []Run
	if body.Len() > 0 {
		data := []byte(body.String())
		root := md.Parser().Parse(text.NewReader(data))
		child := root.FirstChild()
		if child == nil || child.Kind() != ast.KindParagraph || child.NextSibling() != nil {
			return rawOf(q, src), true
		}
		var ok bool
		runs, ok = inlineRuns(child, data)
		if !ok {
			return rawOf(q, src), true
		}
	}
	return editBlock{Block: Block{Kind: Callout, Level: depth, Callout: &CalloutData{Type: match[1], Fold: match[2], Title: match[3]}, Runs: runs}}, true
}

func convertQuote(q *ast.Blockquote, src []byte, depth int) editBlock {
	child := q.FirstChild()
	if child == nil || child.NextSibling() != nil {
		return rawOf(q, src)
	}
	switch c := child.(type) {
	case *ast.Paragraph:
		if callout, ok := convertCallout(q, c, src, depth); ok {
			return callout
		}
		runs, ok := inlineRuns(c, src)
		if !ok {
			return rawOf(q, src)
		}
		return editBlock{Block: Block{Kind: Quote, Level: depth, Runs: runs}}
	case *ast.Blockquote:
		inner := convertQuote(c, src, depth+1)
		if inner.Kind == Raw {
			return rawOf(q, src)
		}
		return inner
	default:
		return rawOf(q, src)
	}
}

// listPiece 是展开后的一个列表项及其自身行在源码中的区间。
type listPiece struct {
	block editBlock
	a, z  int
}

// convertListItems 把列表逐项展开成块，嵌套列表按深度记在 Level 上。
// 列表项里出现列表以外的块级内容时返回 false，由调用方整表保留。
func convertListItems(list *ast.List, src []byte) ([]editBlock, bool) {
	var pieces []listPiece
	if !flattenList(list, src, 1, &pieces) || len(pieces) == 0 {
		return nil, false
	}
	out := make([]editBlock, 0, len(pieces))
	for i, piece := range pieces {
		b := piece.block
		b.source = string(src[piece.a:piece.z])
		if i > 0 {
			prev := pieces[i-1].z
			if prev > piece.a {
				return nil, false
			}
			out[i-1].gap = string(src[prev:piece.a])
		}
		out = append(out, b)
	}
	return out, true
}

func flattenList(list *ast.List, src []byte, depth int, out *[]listPiece) bool {
	if depth > 6 {
		return false
	}
	number := list.Start
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			return false
		}
		parent := li.FirstChild()
		if parent == nil {
			return false
		}
		switch parent.(type) {
		case *ast.TextBlock, *ast.Paragraph:
		default:
			return false
		}
		a, z := nodeSpan(parent, src)
		if a < 0 || z < a || z > len(src) {
			return false
		}
		// 标记符与缩进在内容之前，同一行内回退到行首。
		for a > 0 && src[a-1] != '\n' {
			a--
		}
		b := listFromInline(list, parent, src, depth)
		if b.Kind == Raw {
			// 未支持行内语法只占当前项，邻项的编号和源码区间保持独立。
			b = editBlock{Block: Block{Kind: Raw, Raw: trimTrailingBreak(string(src[a:z])), Level: depth, Ordered: list.IsOrdered()}}
		}
		if b.Ordered {
			b.Start = number
		}
		number++
		indent := 0
		for a+indent < z && (src[a+indent] == ' ' || src[a+indent] == '\t') {
			indent++
		}
		b.indent = string(src[a : a+indent])
		*out = append(*out, listPiece{block: b, a: a, z: z})
		for child := parent.NextSibling(); child != nil; child = child.NextSibling() {
			nested, ok := child.(*ast.List)
			if !ok || !flattenList(nested, src, depth+1, out) {
				return false
			}
		}
	}
	return true
}

func convertList(list *ast.List, src []byte, depth int) editBlock {
	// 多项或含嵌套块的列表整表保留为 Raw，避免拆开后丢标记和缩进。
	// 单项且内容是纯行内时拆成可编辑的 List/Task。
	item := list.FirstChild()
	if item == nil || item.NextSibling() != nil {
		return rawOf(list, src)
	}
	li, ok := item.(*ast.ListItem)
	if !ok {
		return rawOf(list, src)
	}
	tb := li.FirstChild()
	if tb == nil || tb.NextSibling() != nil {
		return rawOf(list, src)
	}
	block, ok := tb.(*ast.TextBlock)
	if !ok {
		// 段落形式的松散列表同样只接受纯行内。
		p, pok := tb.(*ast.Paragraph)
		if !pok {
			return rawOf(list, src)
		}
		return listFromInline(list, p, src, depth)
	}
	return listFromInline(list, block, src, depth)
}

func listFromInline(list *ast.List, parent ast.Node, src []byte, depth int) editBlock {
	checked, isTask := taskState(parent)
	runs, ok := inlineRuns(parent, src)
	if !ok {
		return rawOf(list, src)
	}
	// 任务复选框本身不是可见正文。
	kind := List
	if isTask {
		kind = Task
	}
	return editBlock{Block: Block{
		Kind:    kind,
		Level:   depth,
		Ordered: list.IsOrdered(),
		Start:   list.Start,
		Checked: checked,
		Runs:    runs,
	}}
}

func taskState(parent ast.Node) (checked bool, ok bool) {
	if parent == nil {
		return false, false
	}
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if box, is := c.(*extast.TaskCheckBox); is {
			return box.IsChecked, true
		}
	}
	return false, false
}

func convertTable(table *extast.Table, src []byte) editBlock {
	tb := &TableData{Aligns: make([]Align, len(table.Alignments))}
	for i, a := range table.Alignments {
		switch a {
		case extast.AlignLeft:
			tb.Aligns[i] = AlignLeft
		case extast.AlignCenter:
			tb.Aligns[i] = AlignCenter
		case extast.AlignRight:
			tb.Aligns[i] = AlignRight
		default:
			tb.Aligns[i] = AlignNone
		}
	}
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		header := false
		var cells []Cell
		switch r := row.(type) {
		case *extast.TableHeader:
			header = true
			for cell := r.FirstChild(); cell != nil; cell = cell.NextSibling() {
				runs, ok := inlineRuns(cell, src)
				if !ok {
					return rawOf(table, src)
				}
				cells = append(cells, Cell{Runs: runs})
			}
		case *extast.TableRow:
			for cell := r.FirstChild(); cell != nil; cell = cell.NextSibling() {
				runs, ok := inlineRuns(cell, src)
				if !ok {
					return rawOf(table, src)
				}
				cells = append(cells, Cell{Runs: runs})
			}
		default:
			return rawOf(table, src)
		}
		if header {
			tb.Header = true
		}
		tb.Rows = append(tb.Rows, cells)
	}
	return editBlock{Block: Block{Kind: TableBlock, Table: tb}}
}

func codeText(lines *text.Segments, src []byte) string {
	if lines == nil {
		return ""
	}
	var buf bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(src))
		// goldmark 的代码行通常已经带换行，缺的才补。
		if seg.Stop <= seg.Start || src[seg.Stop-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// inlineRuns 把行内节点收成 Run。遇到不认识的节点返回 false，调用方整块改 Raw。
func inlineRuns(n ast.Node, src []byte) ([]Run, bool) {
	display, formulas := mathIn(n, src)
	if display {
		return nil, false
	}
	var runs []Run
	ok := walkInline(n, src, 0, nil, &runs)
	if !ok {
		return nil, false
	}
	// 公式跨越强调、链接等节点时无法落在单个 Run 上，整块保留原文。
	found := 0
	for _, r := range runs {
		if r.Marks&MarkMath != 0 {
			found++
		}
	}
	if found != formulas {
		return nil, false
	}
	return mergeRuns(runs), true
}

// appendText 把一段普通文本写成 Run，成对美元符号之间的内容记为公式。
func appendText(out *[]Run, raw []byte, marks Mark, link *Link) {
	masked := maskEscapedDollars(raw)
	last := 0
	for _, m := range pairedMath.FindAllIndex(masked, -1) {
		appendRun(out, html.UnescapeString(unescape(raw[last:m[0]])), marks, link)
		*out = append(*out, Run{Text: string(raw[m[0]+1 : m[1]-1]), Marks: marks | MarkMath, Link: cloneLink(link)})
		last = m[1]
	}
	appendRun(out, html.UnescapeString(unescape(raw[last:])), marks, link)
}

// maskEscapedDollars 把被奇数个反斜杠转义的美元换成空格，长度不变。
func maskEscapedDollars(raw []byte) []byte {
	data := append([]byte(nil), raw...)
	for i, c := range data {
		if c != '$' {
			continue
		}
		slashes := 0
		for j := i - 1; j >= 0 && data[j] == '\\'; j-- {
			slashes++
		}
		if slashes%2 != 0 {
			data[i] = ' '
		}
	}
	return data
}

func walkInline(n ast.Node, src []byte, marks Mark, link *Link, out *[]Run) bool {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *extast.TaskCheckBox:
			continue
		case *ast.Text:
			// 下划线、反斜杠会把同一行文本拆成相邻节点，合并后公式才完整。
			raw := append([]byte(nil), t.Segment.Value(src)...)
			for !t.HardLineBreak() && !t.SoftLineBreak() {
				next, ok := c.NextSibling().(*ast.Text)
				if !ok {
					break
				}
				raw = append(raw, next.Segment.Value(src)...)
				c, t = next, next
			}
			appendText(out, raw, marks, link)
			if t.HardLineBreak() || t.SoftLineBreak() {
				appendRun(out, "\n", marks, link)
			}
		case *ast.String:
			s := string(t.Value)
			if !t.IsCode() && !t.IsRaw() {
				s = html.UnescapeString(unescape([]byte(s)))
			}
			appendRun(out, s, marks, link)
		case *ast.CodeSpan:
			var buf bytes.Buffer
			for g := t.FirstChild(); g != nil; g = g.NextSibling() {
				tx, ok := g.(*ast.Text)
				if !ok {
					return false
				}
				buf.Write(tx.Segment.Value(src))
			}
			appendRun(out, buf.String(), marks|MarkCode, link)
		case *ast.Emphasis:
			next := marks
			if t.Level >= 2 {
				next |= MarkBold
			} else {
				next |= MarkItalic
			}
			if !walkInline(t, src, next, link, out) {
				return false
			}
		case *footnoteInline:
			appendRun(out, t.label, marks|MarkSup, &Link{Footnote: t.label})
		case *markedInline:
			if t.mark == MarkSub || t.mark == MarkSup {
				appendRun(out, t.value, marks|t.mark, link)
			} else if !walkInline(t, src, marks|t.mark, link, out) {
				return false
			}
		case *extast.Strikethrough:
			if !walkInline(t, src, marks|MarkStrike, link, out) {
				return false
			}
		case *ast.Link:
			inner := &Link{URL: string(t.Destination), Title: string(t.Title)}
			if t.Reference != nil {
				inner.Ref = string(t.Reference.Value)
			}
			if !walkInline(t, src, marks, inner, out) {
				return false
			}
		case *ast.Image:
			img := &InlineImage{Alt: string(t.Text(src)), URL: string(t.Destination), Title: string(t.Title)}
			if t.Reference != nil {
				img.Ref = string(t.Reference.Value)
			}
			*out = append(*out, Run{Text: objectReplacement, Marks: marks, Link: cloneLink(link), Image: img})
		case *ast.RawHTML:
			if !applyInlineHTML(string(t.Text(src)), &marks) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// applyInlineHTML 识别行内的 u、kbd 等标签，只切换样式，不把标签写进正文。
func applyInlineHTML(raw string, marks *Mark) bool {
	text := strings.TrimSpace(raw)
	closing := strings.HasPrefix(text, "</")
	name := strings.Trim(text, "</> ")
	if i := strings.IndexAny(name, " \t"); i >= 0 {
		name = name[:i]
	}
	var bit Mark
	switch strings.ToLower(name) {
	case "u", "ins":
		bit = MarkUnderline
	case "kbd":
		bit = MarkKbd
	case "b", "strong":
		bit = MarkBold
	case "i", "em":
		bit = MarkItalic
	case "s", "del", "strike":
		bit = MarkStrike
	case "mark":
		bit = MarkHighlight
	case "sub":
		bit = MarkSub
	case "sup":
		bit = MarkSup
	case "code":
		bit = MarkCode
	default:
		return false
	}
	if closing {
		*marks &^= bit
	} else {
		*marks |= bit
	}
	return true
}

func appendRun(out *[]Run, s string, marks Mark, link *Link) {
	if s == "" {
		return
	}
	*out = append(*out, Run{Text: s, Marks: marks, Link: cloneLink(link)})
}

func mergeRuns(runs []Run) []Run {
	if len(runs) == 0 {
		return nil
	}
	out := []Run{runs[0]}
	for _, r := range runs[1:] {
		last := &out[len(out)-1]
		if last.Image == nil && r.Image == nil && last.Marks == r.Marks && sameLink(last.Link, r.Link) {
			last.Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}

func cloneLink(l *Link) *Link {
	if l == nil {
		return nil
	}
	c := *l
	return &c
}

func sameLink(a, b *Link) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.URL == b.URL && a.Title == b.Title && a.Footnote == b.Footnote && a.Ref == b.Ref
}

func sameInlineImage(a, b *InlineImage) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func cloneInlineImage(img *InlineImage) *InlineImage {
	if img == nil {
		return nil
	}
	c := *img
	return &c
}

// unescape 还原 CommonMark 反斜杠转义，得到用户看到的字符。
func unescape(b []byte) string {
	if !bytes.Contains(b, []byte{'\\'}) {
		return string(b)
	}
	var buf bytes.Buffer
	for i := 0; i < len(b); {
		if b[i] == '\\' && i+1 < len(b) {
			_, n := utf8.DecodeRune(b[i+1:])
			if isASCIIPunct(b[i+1]) {
				buf.WriteByte(b[i+1])
				i += 2
				continue
			}
			_ = n
		}
		buf.WriteByte(b[i])
		i++
	}
	return buf.String()
}

func isASCIIPunct(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

// 空围栏没有内容行区间，从尚未覆盖的源码中定位开闭围栏。
func emptyFenceSpan(src []byte, from int) (int, int) {
	start := from
	for start < len(src) {
		end := start
		for end < len(src) && src[end] != '\n' {
			end++
		}
		line := bytes.TrimSpace(src[start:end])
		if len(line) >= 3 && (line[0] == '`' || line[0] == '~') {
			stop := min(end+1, len(src))
			closeEnd := stop
			for closeEnd < len(src) && src[closeEnd] != '\n' {
				closeEnd++
			}
			if isFenceCloser(bytes.TrimSpace(src[stop:closeEnd])) {
				stop = min(closeEnd+1, len(src))
			}
			return start, stop
		}
		start = end + 1
	}
	return from, len(src)
}

// pairedMath 要求两侧美元符号紧贴正文，单个价格与转义美元不算公式。
var pairedMath = regexp.MustCompile(`\$[^\s$](?:[^$\n]*[^\s$])?\$`)

// mathIn 报告行内内容是否含独立成行的 $$，以及成对美元公式的个数。
func mathIn(n ast.Node, src []byte) (display bool, formulas int) {
	var raw strings.Builder
	var collect func(ast.Node)
	collect = func(parent ast.Node) {
		for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.CodeSpan:
				raw.WriteByte('\n')
			case *ast.Text:
				raw.Write(t.Segment.Value(src))
				if t.SoftLineBreak() || t.HardLineBreak() {
					raw.WriteByte('\n')
				}
			case *ast.String:
				raw.Write(t.Value)
			case *markedInline:
				if t.mark == MarkSub || t.mark == MarkSup {
					raw.WriteString(t.value)
				} else {
					collect(t)
				}
			default:
				collect(c)
			}
		}
	}
	collect(n)
	data := maskEscapedDollars([]byte(raw.String()))
	// 双美元是块级公式，无论独占一行还是写在同一行都整块保留。
	if bytes.Contains(data, []byte("$$")) {
		return true, 0
	}
	return false, len(pairedMath.FindAll(data, -1))
}
