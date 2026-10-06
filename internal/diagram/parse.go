package diagram

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 规模上限：超过后返回错误，由调用方退回占位卡片，避免畸形输入拖慢界面。
const (
	maxNodes      = 2000
	maxEdges      = 6000
	maxEdgeLength = 8
	maxNesting    = 16
)

// Parse 解析 Mermaid 源码（围栏代码块的正文，不含 ```mermaid 行）。
// 支持常用流程图、时序图、甘特图、饼图、类图、状态图与 ER 图子集；无效语法返回 error。
func Parse(src string) (*Graph, error) {
	stmts := splitStatements(src)
	if len(stmts) == 0 {
		return nil, errors.New("diagram: 内容为空")
	}
	word, rest := cutWord(stmts[0].text)
	switch word {
	case "graph", "flowchart", "flowchart-elk":
	case "sequenceDiagram", "gantt", "pie", "classDiagram", "stateDiagram", "stateDiagram-v2", "erDiagram":
		other := stmts[1:]
		if word == "pie" {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "showData"))
		}
		if rest != "" {
			other = append([]statement{{text: rest, line: stmts[0].line}}, other...)
		}
		return parseOther(word, other)
	default:
		return nil, fmt.Errorf("diagram: 不支持的图类型 %q", clip(word))
	}
	p := &parser{g: &Graph{classes: map[string]Attributes{}}, index: map[string]int{}}
	if dir, more := cutWord(rest); dir != "" {
		if d, ok := parseDirection(dir); ok {
			p.g.Direction = d
			rest = more
		}
	}
	// 方向后面同一行还有内容时，当作第一条语句。
	if rest = strings.TrimSpace(rest); rest != "" {
		stmts[0].text = rest
	} else {
		stmts = stmts[1:]
	}
	for _, st := range stmts {
		if err := p.statement(st.text); err != nil {
			return nil, fmt.Errorf("diagram: 第 %d 行: %w", st.line, err)
		}
	}
	return p.finish()
}

type statement struct {
	text string
	line int
}

// splitStatements 去掉注释与指令，按换行和分号切成语句。
// 引号、括号和 |标签| 内的分号不算分隔符。
func splitStatements(src string) []statement {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = blankFrontmatter(src)

	var out []statement
	var b strings.Builder
	line, start := 1, 1
	depth, quote, pipe := 0, false, false
	flush := func() {
		if t := strings.TrimSpace(b.String()); t != "" {
			out = append(out, statement{t, start})
		}
		b.Reset()
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			flush()
			line++
			start = line
			depth, quote, pipe = 0, false, false
			continue
		case quote:
			quote = c != '"'
		case c == '"':
			quote = true
		case c == '%' && depth == 0 && !pipe && strings.HasPrefix(src[i:], "%%"):
			end := -1
			if strings.HasPrefix(src[i:], "%%{") {
				if j := strings.Index(src[i:], "}%%"); j >= 0 {
					end = i + j + 3
				}
			}
			if end < 0 {
				if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
					end = i + j
				} else {
					end = len(src)
				}
			}
			line += strings.Count(src[i:end], "\n")
			i = end - 1
			continue
		case c == '|' && depth == 0:
			pipe = !pipe
		case pipe:
		case c == '[' || c == '(' || c == '{':
			depth++
		case c == ']' || c == ')' || c == '}':
			if depth > 0 {
				depth--
			}
		case c == ';' && depth == 0:
			flush()
			start = line
			continue
		}
		b.WriteByte(c)
	}
	flush()
	return out
}

// blankFrontmatter 把开头的 YAML frontmatter 换成空行，保留行号。
func blankFrontmatter(src string) string {
	lines := strings.Split(src, "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 || strings.TrimSpace(lines[first]) != "---" {
		return src
	}
	for j := first + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			for k := first; k <= j; k++ {
				lines[k] = ""
			}
			return strings.Join(lines, "\n")
		}
	}
	return src
}

func cutWord(s string) (word, rest string) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 24 {
		return string(r[:24]) + "…"
	}
	return s
}

func parseDirection(s string) (Direction, bool) {
	switch s {
	case "TD", "TB", "v":
		return TopDown, true
	case "BT", "^":
		return BottomUp, true
	case "LR", ">":
		return LeftRight, true
	case "RL", "<":
		return RightLeft, true
	}
	return TopDown, false
}

type parser struct {
	g     *Graph
	index map[string]int // 节点 ID → 下标
	stack []int          // 尚未 end 的子图
	auto  int
}

func (p *parser) statement(text string) error {
	word, rest := cutWord(text)
	switch word {
	case "subgraph":
		return p.openSubgraph(rest)
	case "end":
		if rest == "" {
			if len(p.stack) == 0 {
				return errors.New("多余的 end")
			}
			p.stack = p.stack[:len(p.stack)-1]
			return nil
		}
	case "direction":
		if d, ok := parseDirection(rest); ok {
			if len(p.stack) > 0 {
				sg := p.g.Subgraphs[p.stack[len(p.stack)-1]]
				sg.Direction = d
				sg.HasDirection = true
			} else {
				p.g.Direction = d
			}
			return nil
		}
	case "classDef", "class", "style", "linkStyle", "click":
		return p.styleStatement(word, rest)
	case "accTitle", "accTitle:", "accDescr", "accDescr:":
		return nil
	}
	return p.edgeStatement(text)
}

func (p *parser) openSubgraph(rest string) error {
	if len(p.stack) >= maxNesting {
		return errors.New("subgraph 嵌套过深")
	}
	id, title := "", ""
	switch i := strings.IndexByte(rest, '['); {
	case rest == "":
	case i >= 0 && strings.HasSuffix(rest, "]"):
		id = strings.TrimSpace(rest[:i])
		title = cleanText(rest[i+1 : len(rest)-1])
	default:
		title = cleanText(rest)
		id = title
	}
	if id == "" {
		p.auto++
		id = fmt.Sprintf("subgraph%d", p.auto)
	}
	parent := -1
	if len(p.stack) > 0 {
		parent = p.stack[len(p.stack)-1]
	}
	p.g.Subgraphs = append(p.g.Subgraphs, &Subgraph{ID: id, Title: strings.ReplaceAll(title, "\n", " "), Parent: parent})
	p.stack = append(p.stack, len(p.g.Subgraphs)-1)
	return nil
}

// node 返回节点下标，首次出现时创建；节点归入它第一次出现时所在的子图。
func (p *parser) node(id string) int {
	i, ok := p.index[id]
	if !ok {
		i = len(p.g.Nodes)
		p.g.Nodes = append(p.g.Nodes, &Node{ID: id, Text: id, group: -1})
		p.index[id] = i
	}
	if n := p.g.Nodes[i]; n.group < 0 && len(p.stack) > 0 {
		n.group = p.stack[len(p.stack)-1]
	}
	return i
}

// edgeStatement 解析“节点组 (连线 节点组)*”，节点组是用 & 连接的若干节点。
func (p *parser) edgeStatement(text string) error {
	sc := &scanner{s: text}
	prev, err := p.group(sc)
	if err != nil {
		return err
	}
	for {
		sc.skip()
		if sc.eof() {
			return nil
		}
		ln, err := sc.link()
		if err != nil {
			return err
		}
		if ln == nil {
			return fmt.Errorf("无法识别 %q", clip(sc.rest()))
		}
		next, err := p.group(sc)
		if err != nil {
			return err
		}
		for _, a := range prev {
			for _, b := range next {
				if len(p.g.Edges) >= maxEdges {
					return errors.New("连线过多")
				}
				p.g.Edges = append(p.g.Edges, &Edge{Label: ln.label, Line: ln.line, Head: ln.head, Tail: ln.tail, Length: ln.length, from: a, to: b})
			}
		}
		prev = next
	}
}

func (p *parser) group(sc *scanner) ([]int, error) {
	var ids []int
	for {
		sc.skip()
		id := sc.ident()
		if id == "" {
			if sc.eof() {
				return nil, errors.New("缺少节点")
			}
			return nil, fmt.Errorf("此处需要节点: %q", clip(sc.rest()))
		}
		if _, ok := p.index[id]; !ok && len(p.g.Nodes) >= maxNodes {
			return nil, errors.New("节点过多")
		}
		i := p.node(id)
		shape, text, ok, err := sc.shape()
		if err != nil {
			return nil, err
		}
		if ok {
			n := p.g.Nodes[i]
			n.Shape, n.Text = shape, cleanText(text)
		}
		if strings.HasPrefix(sc.rest(), ":::") {
			sc.pos += 3
			p.g.Nodes[i].Classes = append(p.g.Nodes[i].Classes, sc.ident())
		}
		ids = append(ids, i)
		sc.skip()
		if sc.peek() != '&' {
			return ids, nil
		}
		sc.pos++
	}
}

// finish 处理子图与节点重名、空子图，并补全对外字段。
func (p *parser) finish() (*Graph, error) {
	if len(p.stack) > 0 {
		return nil, errors.New("diagram: subgraph 缺少 end")
	}
	g := p.g
	if len(g.Subgraphs) > 0 {
		p.resolveSubgraphs()
	}
	for _, e := range g.Edges {
		e.From, e.To = g.Nodes[e.from].ID, g.Nodes[e.to].ID
	}
	for _, n := range g.Nodes {
		if n.group >= 0 {
			sg := g.Subgraphs[n.group]
			sg.Nodes = append(sg.Nodes, n.ID)
		}
	}
	return g, nil
}

// resolveSubgraphs 做两件事：没有成员的子图改成普通节点；
// 连到子图 ID 的连线改接到子图内第一个节点，同名的占位节点删除。
func (p *parser) resolveSubgraphs() {
	g := p.g
	ns := len(g.Subgraphs)
	alias := make([]bool, len(g.Nodes), len(g.Nodes)+ns)
	for _, sg := range g.Subgraphs {
		if i, ok := p.index[sg.ID]; ok {
			alias[i] = true
		}
	}
	count := make([]int, ns)
	for i, n := range g.Nodes {
		if alias[i] {
			continue
		}
		for s := n.group; s >= 0; s = g.Subgraphs[s].Parent {
			count[s]++
		}
	}
	removed := make([]bool, ns)
	// 内层子图下标更大，倒序处理能让空的内层先变成外层的成员。
	for s := ns - 1; s >= 0; s-- {
		if count[s] > 0 {
			continue
		}
		sg := g.Subgraphs[s]
		removed[s] = true
		i, ok := p.index[sg.ID]
		if !ok {
			i = len(g.Nodes)
			g.Nodes = append(g.Nodes, &Node{ID: sg.ID, Text: sg.ID})
			alias = append(alias, false)
			p.index[sg.ID] = i
		}
		n := g.Nodes[i]
		if n.Text == n.ID && sg.Title != "" {
			n.Text = sg.Title
		}
		n.group = sg.Parent
		if alias[i] {
			alias[i] = false
			for a := sg.Parent; a >= 0; a = g.Subgraphs[a].Parent {
				count[a]++
			}
		}
	}

	// 剩下的重名节点都对应非空子图，找出各子图的代表节点。
	rep := make([]int, ns)
	for s := range rep {
		rep[s] = -1
	}
	for i, n := range g.Nodes {
		if alias[i] {
			continue
		}
		for s := n.group; s >= 0 && rep[s] < 0; s = g.Subgraphs[s].Parent {
			rep[s] = i
		}
	}
	nodeMap := make([]int, len(g.Nodes))
	kept := g.Nodes[:0:0]
	for i, n := range g.Nodes {
		if alias[i] {
			continue
		}
		nodeMap[i] = len(kept)
		kept = append(kept, n)
	}
	for i := range g.Nodes {
		if alias[i] {
			nodeMap[i] = nodeMap[rep[p.aliasTarget(i, removed)]]
		}
	}
	for _, e := range g.Edges {
		e.from, e.to = nodeMap[e.from], nodeMap[e.to]
	}
	g.Nodes = kept

	sgMap := make([]int, ns)
	subs := g.Subgraphs[:0:0]
	for s, sg := range g.Subgraphs {
		sgMap[s] = -1
		if !removed[s] {
			sgMap[s] = len(subs)
			subs = append(subs, sg)
		}
	}
	for _, sg := range subs {
		if sg.Parent >= 0 {
			sg.Parent = sgMap[sg.Parent]
		}
	}
	for _, n := range g.Nodes {
		if n.group >= 0 {
			n.group = sgMap[n.group]
		}
	}
	g.Subgraphs = subs
}

// aliasTarget 返回与节点 i 同名、且保留下来的第一个子图。
func (p *parser) aliasTarget(i int, removed []bool) int {
	id := p.g.Nodes[i].ID
	for s, sg := range p.g.Subgraphs {
		if sg.ID == id && !removed[s] {
			return s
		}
	}
	return 0
}

type scanner struct {
	s   string
	pos int
}

func (sc *scanner) eof() bool    { return sc.pos >= len(sc.s) }
func (sc *scanner) rest() string { return sc.s[sc.pos:] }

func (sc *scanner) peek() byte {
	if sc.eof() {
		return 0
	}
	return sc.s[sc.pos]
}

func (sc *scanner) skip() {
	for !sc.eof() && (sc.s[sc.pos] == ' ' || sc.s[sc.pos] == '\t') {
		sc.pos++
	}
}

func isIDRune(r rune) bool {
	return r != utf8.RuneError && (r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r))
}

// ident 读取节点 ID：字母、数字、下划线，以及夹在其间的 - 和 .（如 my-node）。
// 连线总以 --、-.、== 开头，所以 ID 里的 - 后面必须紧跟 ID 字符。
func (sc *scanner) ident() string {
	start := sc.pos
	for !sc.eof() {
		r, size := utf8.DecodeRuneInString(sc.s[sc.pos:])
		if isIDRune(r) {
			sc.pos += size
			continue
		}
		if (r == '-' || r == '.') && sc.pos > start {
			if next, _ := utf8.DecodeRuneInString(sc.s[sc.pos+1:]); isIDRune(next) {
				sc.pos++
				continue
			}
		}
		break
	}
	return sc.s[start:sc.pos]
}

type shapeClose struct {
	close string
	shape Shape
}

// 形状的开闭符号，长的在前；开符号相同而闭符号不同时取先出现的那个。
var shapeDelims = []struct {
	open   string
	closes []shapeClose
}{
	{"(((", []shapeClose{{")))", ShapeDoubleCircle}}},
	{"((", []shapeClose{{"))", ShapeCircle}}},
	{"([", []shapeClose{{"])", ShapeStadium}}},
	{"[(", []shapeClose{{")]", ShapeCylinder}}},
	{"[[", []shapeClose{{"]]", ShapeSubroutine}}},
	{"[/", []shapeClose{{"/]", ShapeParallelogram}, {"\\]", ShapeTrapezoid}}},
	{"[\\", []shapeClose{{"\\]", ShapeParallelogramAlt}, {"/]", ShapeTrapezoidAlt}}},
	{"{{", []shapeClose{{"}}", ShapeHexagon}}},
	{"[", []shapeClose{{"]", ShapeRect}}},
	{"(", []shapeClose{{")", ShapeRound}}},
	{"{", []shapeClose{{"}", ShapeDiamond}}},
	{">", []shapeClose{{"]", ShapeFlag}}},
}

// shape 读取紧跟在 ID 后的形状与文字。
func (sc *scanner) shape() (Shape, string, bool, error) {
	rest := sc.rest()
	opened := false
	for _, d := range shapeDelims {
		if !strings.HasPrefix(rest, d.open) {
			continue
		}
		opened = true
		body := rest[len(d.open):]
		// 引号包住的文字可以含任意括号。
		if t := strings.TrimLeft(body, " \t"); strings.HasPrefix(t, `"`) {
			j := strings.IndexByte(t[1:], '"')
			if j < 0 {
				return 0, "", false, errors.New("引号未闭合")
			}
			after := strings.TrimLeft(t[j+2:], " \t")
			for _, c := range d.closes {
				if strings.HasPrefix(after, c.close) {
					sc.pos = len(sc.s) - len(after) + len(c.close)
					return c.shape, t[1 : j+1], true, nil
				}
			}
			continue
		}
		best, shape, width := -1, ShapeRect, 0
		for _, c := range d.closes {
			if j := findClose(body, d.open, c.close); j >= 0 && (best < 0 || j < best) {
				best, shape, width = j, c.shape, len(c.close)
			}
		}
		if best >= 0 {
			sc.pos += len(d.open) + best + width
			return shape, body[:best], true, nil
		}
		// 长符号配不上时退回更短的，例如 A[(注) 文字]。
	}
	if opened {
		return 0, "", false, fmt.Errorf("节点形状未闭合: %q", clip(rest))
	}
	return 0, "", false, nil
}

// findClose 找闭符号的位置。单字符括号按嵌套配对，如 A(甲(乙))。
func findClose(body, open, close string) int {
	if len(open) != 1 || len(close) != 1 || open == ">" {
		return strings.Index(body, close)
	}
	depth := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case open[0]:
			depth++
		case close[0]:
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

type link struct {
	label      string
	line       LineStyle
	head, tail Arrow
	length     int
}

func arrowOf(c byte) Arrow {
	switch c {
	case 'o':
		return ArrowCircle
	case 'x':
		return ArrowCross
	}
	return ArrowNormal
}

func run(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

func isHead(s string, i int) bool {
	return i < len(s) && (s[i] == '>' || s[i] == 'o' || s[i] == 'x')
}

// indexOutsideQuotes 找引号之外第一次出现 sub 的位置。
func indexOutsideQuotes(s, sub string) int {
	quote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			quote = !quote
		} else if !quote && strings.HasPrefix(s[i:], sub) {
			return i
		}
	}
	return -1
}

// link 读取一条连线；当前位置不是连线时返回 nil。
// 规则与 Mermaid 的词法一致：线身后紧跟 >、o、x 即为端点标记，
// 所以 A --ok--> B 这类写法需要在文字两侧留空格。
func (sc *scanner) link() (*link, error) {
	s, i := sc.s, sc.pos
	ln := &link{length: 1}
	if i+1 < len(s) && strings.IndexByte("<ox", s[i]) >= 0 && (s[i+1] == '-' || s[i+1] == '=') {
		ln.tail = arrowOf(s[i])
		i++
	}
	switch rest := s[i:]; {
	case strings.HasPrefix(rest, "--"), strings.HasPrefix(rest, "=="):
		ch := rest[0]
		if ch == '=' {
			ln.line = LineThick
		}
		n := run(rest, ch)
		i += n
		switch {
		case isHead(s, i):
			ln.head = arrowOf(s[i])
			ln.length = n - 1
			i++
		case n >= 3:
			ln.length = n - 2
		default:
			// 文字写在线中间：A -- 文字 --> B
			j := indexOutsideQuotes(s[i:], string([]byte{ch, ch}))
			if j < 0 {
				return nil, errors.New("连线未闭合")
			}
			ln.label = cleanText(s[i : i+j])
			i += j
			n = run(s[i:], ch)
			i += n
			if isHead(s, i) {
				ln.head = arrowOf(s[i])
				ln.length = n - 1
				i++
			} else {
				ln.length = n - 2
			}
		}
	case strings.HasPrefix(rest, "-."):
		ln.line = LineDotted
		i++
		n := run(s[i:], '.')
		i += n
		if i < len(s) && s[i] == '-' {
			i++
		} else {
			// A -. 文字 .-> B
			j := indexOutsideQuotes(s[i:], ".-")
			if j < 0 {
				return nil, errors.New("连线未闭合")
			}
			for j > 0 && s[i+j-1] == '.' {
				j--
			}
			ln.label = cleanText(s[i : i+j])
			i += j
			n = run(s[i:], '.')
			i += n + 1
		}
		ln.length = n
		if isHead(s, i) {
			ln.head = arrowOf(s[i])
			i++
		}
	case strings.HasPrefix(rest, "~~~"):
		ln.line = LineInvisible
		n := run(rest, '~')
		ln.length = n - 2
		i += n
	default:
		return nil, nil
	}
	ln.length = min(max(ln.length, 1), maxEdgeLength)
	sc.pos = i
	sc.skip()
	if sc.peek() == '|' {
		j := strings.IndexByte(s[sc.pos+1:], '|')
		if j < 0 {
			return nil, errors.New("连线文字未闭合")
		}
		ln.label = cleanText(s[sc.pos+1 : sc.pos+1+j])
		sc.pos += j + 2
	}
	return ln, nil
}

var (
	brTag     = regexp.MustCompile(`(?i)<br\s*/?>`)
	inlineTag = regexp.MustCompile(`(?i)</?(b|i|u|em|strong|span|small|sub|sup|code)(\s[^>]*)?>`)
	entity    = regexp.MustCompile(`#(\w+);`)
)

var namedEntities = map[string]string{"quot": `"`, "amp": "&", "lt": "<", "gt": ">", "nbsp": " ", "apos": "'"}

// cleanText 把源码里的节点或连线文字整理成显示文字：
// 去掉引号，<br> 变换行，还原 #quot; 这类转义。
func cleanText(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range []byte{'"', '`'} {
		if len(s) >= 2 && s[0] == q && s[len(s)-1] == q {
			s = s[1 : len(s)-1]
		}
	}
	s = brTag.ReplaceAllString(s, "\n")
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = inlineTag.ReplaceAllString(s, "")
	s = entity.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := namedEntities[name]; ok {
			return v
		}
		if n, err := strconv.Atoi(name); err == nil && n > 0 && n <= unicode.MaxRune {
			return string(rune(n))
		}
		return m
	})
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}
