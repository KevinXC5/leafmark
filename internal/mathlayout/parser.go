package mathlayout

import (
	"fmt"
	"strings"
	"unicode"
)

type nodeKind uint8

const (
	nSymbol    nodeKind = iota // 单个符号：text、cls
	nText                      // 正体文字或函数名以外的文字串：text、cls
	nGroup                     // {…}：list
	nFrac                      // 分式：a 分子、b 分母
	nSqrt                      // 根式：a 被开方式、b 根指数（可空）
	nScript                    // 上下标：a 基（可空）、sub、sup、primes
	nOp                        // 大型运算符或函数名：text
	nLeftRight                 // \left … \right：left、right、list
	nDelim                     // \big 系列定界符：left、size、cls
	nAccent                    // 重音与上下划线：acc、a
	nSpace                     // 间距：size（mu）
	nStyle                     // \displaystyle 等：level
	nEnv                       // 环境与多行公式：table
)

type node struct {
	kind nodeKind
	cls  class
	text string
	list []*node
	a, b *node

	sub, sup *node
	primes   int
	limits   int8 // 1 强制上下限在正上正下，-1 强制在右侧，0 按运算符默认

	bigSym   bool // nOp：大型运算符（否则是函数名）
	opLimits bool // nOp：块级样式下默认把上下限放在正上正下

	left, right rune // 定界符；0 表示空
	size        float32
	acc         accent
	level       int
	fracStyle   int8 // 1 \dfrac，2 \tfrac
	binom       bool
	table       *tableNode
}

// tableSpec 描述一种环境的排法。
type tableSpec struct {
	aligns  string  // 各列对齐方式（l、c、r），列数超出时循环使用
	colSep  float32 // 列间距，em
	rowGap  float32 // 行间额外间距，em
	pad     float32 // 左定界符之后的间距，em
	pairs   bool    // 按“右对齐、左对齐”成对排列，对内不留间距（aligned）
	display bool    // 单元格沿用外层样式；否则降为行内样式
	fenced  bool
	left    rune
	right   rune
}

type tableNode struct {
	rows [][][]*node
	spec tableSpec
}

var environments = map[string]tableSpec{
	"matrix":      {aligns: "c", colSep: 1},
	"pmatrix":     {aligns: "c", colSep: 1, fenced: true, left: '(', right: ')'},
	"bmatrix":     {aligns: "c", colSep: 1, fenced: true, left: '[', right: ']'},
	"Bmatrix":     {aligns: "c", colSep: 1, fenced: true, left: '{', right: '}'},
	"vmatrix":     {aligns: "c", colSep: 1, fenced: true, left: '|', right: '|'},
	"Vmatrix":     {aligns: "c", colSep: 1, fenced: true, left: 0x2016, right: 0x2016},
	"array":       {aligns: "c", colSep: 1},
	"cases":       {aligns: "l", colSep: 1, pad: 0.17, fenced: true, left: '{'},
	"aligned":     {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"align":       {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"align*":      {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"split":       {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"gathered":    {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"gather":      {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"gather*":     {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"equation":    {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"equation*":   {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"smallmatrix": {aligns: "c", colSep: 0.6},
}

const (
	maxDepth  = 64    // 嵌套层数上限，防止畸形输入耗尽栈
	maxLength = 20000 // 源码字符数上限
)

type tokKind uint8

const (
	tEOF tokKind = iota
	tChar
	tCmd
)

type token struct {
	kind tokKind
	r    rune
	cmd  string // 不含反斜杠
	pos  int
}

func (t token) isChar(r rune) bool  { return t.kind == tChar && t.r == r }
func (t token) isCmd(s string) bool { return t.kind == tCmd && t.cmd == s }
func (t token) isRowBreak() bool    { return t.isCmd(`\`) || t.isCmd("cr") }
func (t token) describe() string {
	switch t.kind {
	case tChar:
		return string(t.r)
	case tCmd:
		return `\` + t.cmd
	}
	return "公式结尾"
}

type parser struct {
	src     []rune
	pos     int
	depth   int
	variant variantKind
}

// parse 把 TeX 数学源码解析成节点树。
func parse(src string) (*node, error) {
	p := &parser{src: []rune(src)}
	if len(p.src) > maxLength {
		return nil, fmt.Errorf("公式过长（%d 个字符，上限 %d）", len(p.src), maxLength)
	}
	rows, err := p.parseRows()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.errorf(t, "多余的 %s", t.describe())
	}
	if len(rows) == 1 && len(rows[0]) == 1 {
		return &node{kind: nGroup, list: rows[0][0]}, nil
	}
	// 顶层出现 \\ 或 &：有 & 时按 aligned 排，否则各行居中。
	spec := environments["gathered"]
	for _, row := range rows {
		if len(row) > 1 {
			spec = environments["aligned"]
		}
	}
	return &node{kind: nEnv, table: &tableNode{rows: rows, spec: spec}}, nil
}

func (p *parser) errorf(t token, format string, args ...any) error {
	return fmt.Errorf("第 %d 个字符：%s", t.pos+1, fmt.Sprintf(format, args...))
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && unicode.IsSpace(p.src[p.pos]) {
		p.pos++
	}
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

func (p *parser) next() token {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return token{kind: tEOF, pos: p.pos}
	}
	start := p.pos
	r := p.src[p.pos]
	p.pos++
	if r != '\\' {
		return token{kind: tChar, r: r, pos: start}
	}
	if p.pos >= len(p.src) {
		return token{kind: tCmd, pos: start}
	}
	if !isLetter(p.src[p.pos]) {
		c := p.src[p.pos]
		p.pos++
		if unicode.IsSpace(c) {
			c = ' '
		}
		return token{kind: tCmd, cmd: string(c), pos: start}
	}
	from := p.pos
	for p.pos < len(p.src) && isLetter(p.src[p.pos]) {
		p.pos++
	}
	return token{kind: tCmd, cmd: string(p.src[from:p.pos]), pos: start}
}

func (p *parser) peek() token {
	save := p.pos
	t := p.next()
	p.pos = save
	return t
}

// endsList 判断记号是否结束当前列表（不消费）。
func endsList(t token) bool {
	switch t.kind {
	case tEOF:
		return true
	case tChar:
		return t.r == '}' || t.r == '&'
	}
	return t.isRowBreak() || t.cmd == "right" || t.cmd == "end"
}

func (p *parser) enter(t token) error {
	p.depth++
	if p.depth > maxDepth {
		return p.errorf(t, "嵌套超过 %d 层", maxDepth)
	}
	return nil
}

// parseList 解析一串原子，停在终止记号之前。stopBracket 为 true 时 ] 也算终止。
func (p *parser) parseList(stopBracket bool) ([]*node, error) {
	var list []*node
	for {
		t := p.peek()
		if endsList(t) || stopBracket && t.isChar(']') {
			return list, nil
		}
		n, err := p.parseItem()
		if err != nil {
			return nil, err
		}
		list = append(list, n)
	}
}

// parseRows 解析以 & 分列、\\ 分行的内容，停在其他终止记号之前。
func (p *parser) parseRows() ([][][]*node, error) {
	var rows [][][]*node
	var row [][]*node
	for {
		cell, err := p.parseList(false)
		if err != nil {
			return nil, err
		}
		row = append(row, cell)
		t := p.peek()
		if t.isChar('&') {
			p.next()
			continue
		}
		if !t.isRowBreak() {
			break
		}
		p.next()
		p.skipRowSpacing()
		rows = append(rows, row)
		row = nil
	}
	rows = append(rows, row)
	// 末尾的 \\ 不产生空行。
	if last := rows[len(rows)-1]; len(rows) > 1 && len(last) == 1 && len(last[0]) == 0 {
		rows = rows[:len(rows)-1]
	}
	return rows, nil
}

// skipRowSpacing 跳过 \\[2pt] 这样的行距参数；方括号里不是长度时保留为公式内容。
func (p *parser) skipRowSpacing() {
	save := p.pos
	if !p.next().isChar('[') {
		p.pos = save
		return
	}
	from := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != ']' && p.pos-from < 16 {
		p.pos++
	}
	if p.pos >= len(p.src) || p.src[p.pos] != ']' || !isDimension(string(p.src[from:p.pos])) {
		p.pos = save
		return
	}
	p.pos++
}

func isDimension(s string) bool {
	s = strings.TrimSpace(s)
	for _, unit := range []string{"pt", "em", "ex", "mm", "cm", "in", "mu", "px"} {
		if num, ok := strings.CutSuffix(s, unit); ok && num != "" {
			return strings.Trim(strings.TrimSpace(num), "0123456789.-+") == ""
		}
	}
	return false
}

// parseItem 解析一个原子连同它的上下标、撇号和 \limits。
func (p *parser) parseItem() (*node, error) {
	var base *node
	first := p.peek()
	if !first.isChar('^') && !first.isChar('_') && !first.isChar('\'') {
		n, err := p.parseNucleus()
		if err != nil {
			return nil, err
		}
		base = n
	}
	var s *node
	script := func() *node {
		if s == nil {
			s = &node{kind: nScript, a: base}
		}
		return s
	}
loop:
	for {
		t := p.peek()
		switch {
		case t.isCmd("limits"):
			p.next()
			script().limits = 1
		case t.isCmd("nolimits"):
			p.next()
			script().limits = -1
		case t.isChar('^'):
			p.next()
			if script().sup != nil {
				return nil, p.errorf(t, "重复的上标")
			}
			arg, err := p.parseArg("^")
			if err != nil {
				return nil, err
			}
			s.sup = arg
		case t.isChar('_'):
			p.next()
			if script().sub != nil {
				return nil, p.errorf(t, "重复的下标")
			}
			arg, err := p.parseArg("_")
			if err != nil {
				return nil, err
			}
			s.sub = arg
		case t.isChar('\''):
			p.next()
			script().primes++
		default:
			break loop
		}
	}
	if s == nil {
		return base, nil
	}
	// x^\prime 与 x' 等价：字体里的撇号本身已经抬高，不能再当上标缩小。
	if n := countPrimes(s.sup); n > 0 {
		s.primes += n
		s.sup = nil
	}
	if s.sup == nil && s.sub == nil && s.primes == 0 && s.limits == 0 {
		return base, nil
	}
	return s, nil
}

// countPrimes 在节点只由 \prime 组成时返回个数，否则返回 0。
func countPrimes(n *node) int {
	if n == nil {
		return 0
	}
	list := []*node{n}
	if n.kind == nGroup {
		list = n.list
	}
	for _, item := range list {
		if item.kind != nSymbol || item.text != "′" {
			return 0
		}
	}
	return len(list)
}

// parseArg 解析命令或上下标的一个参数：{…} 或单个原子。
func (p *parser) parseArg(owner string) (*node, error) {
	t := p.peek()
	switch {
	case t.isChar('{'):
		return p.parseGroup()
	case endsList(t), t.isChar('^'), t.isChar('_'), t.isChar('\''):
		return nil, p.errorf(t, "%s 缺少参数", owner)
	}
	return p.parseNucleus()
}

func (p *parser) parseGroup() (*node, error) {
	open := p.next()
	if !open.isChar('{') {
		return nil, p.errorf(open, "此处应为 {")
	}
	if err := p.enter(open); err != nil {
		return nil, err
	}
	list, err := p.parseList(false)
	if err != nil {
		return nil, err
	}
	if t := p.next(); !t.isChar('}') {
		return nil, p.errorf(open, "{ 没有闭合")
	}
	p.depth--
	return &node{kind: nGroup, list: list}, nil
}

// parseNucleus 解析不带上下标的一个原子。
func (p *parser) parseNucleus() (*node, error) {
	if p.peek().isChar('{') {
		return p.parseGroup()
	}
	t := p.next()
	if t.kind == tChar {
		return p.charNode(t)
	}
	name := t.cmd
	if name == "" {
		return nil, p.errorf(t, "孤立的反斜杠")
	}
	if s, ok := symbols[name]; ok {
		return &node{kind: nSymbol, text: string(s.r), cls: s.cls}, nil
	}
	if op, ok := bigOps[name]; ok {
		return &node{kind: nOp, cls: clsOp, text: string(op.r), bigSym: true, opLimits: op.limits}, nil
	}
	if limits, ok := functions[name]; ok {
		text := name
		if alt, ok := functionText[name]; ok {
			text = alt
		}
		return &node{kind: nOp, cls: clsOp, text: text, opLimits: limits}, nil
	}
	if mu, ok := spaces[name]; ok {
		return &node{kind: nSpace, cls: clsNone, size: mu}, nil
	}
	if acc, ok := accents[name]; ok {
		arg, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		return &node{kind: nAccent, acc: acc, a: arg}, nil
	}
	if v, ok := variantCommands[name]; ok {
		saved := p.variant
		p.variant = v
		arg, err := p.parseArg(`\` + name)
		p.variant = saved
		return arg, err
	}
	if v, ok := textCommands[name]; ok {
		text, err := p.rawText(t)
		if err != nil {
			return nil, err
		}
		return &node{kind: nText, text: styledText(text, v)}, nil
	}
	if cls, size, ok := bigDelimiter(name); ok {
		r, err := p.parseDelimiter(t)
		if err != nil {
			return nil, err
		}
		return &node{kind: nDelim, cls: cls, left: r, size: size}, nil
	}
	switch name {
	case "frac", "dfrac", "tfrac", "binom", "dbinom", "tbinom":
		num, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		den, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		n := &node{kind: nFrac, a: num, b: den, binom: strings.HasSuffix(name, "binom")}
		switch name[0] {
		case 'd':
			n.fracStyle = 1
		case 't':
			n.fracStyle = 2
		}
		return n, nil
	case "sqrt":
		return p.parseSqrt(t)
	case "left":
		return p.parseLeftRight(t)
	case "begin":
		return p.parseEnvironment(t)
	case "displaystyle":
		return &node{kind: nStyle, cls: clsNone, level: 0}, nil
	case "textstyle":
		return &node{kind: nStyle, cls: clsNone, level: 1}, nil
	case "scriptstyle":
		return &node{kind: nStyle, cls: clsNone, level: 2}, nil
	case "scriptscriptstyle":
		return &node{kind: nStyle, cls: clsNone, level: 3}, nil
	case "operatorname":
		limits := false
		if p.pos < len(p.src) && p.src[p.pos] == '*' {
			p.pos++
			limits = true
		}
		text, err := p.rawText(t)
		if err != nil {
			return nil, err
		}
		return &node{kind: nOp, cls: clsOp, text: text, opLimits: limits}, nil
	case "bmod":
		return &node{kind: nText, cls: clsBin, text: "mod"}, nil
	case "overset", "underset", "stackrel":
		script, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		base, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		n := &node{kind: nScript, a: base, limits: 1}
		if name == "underset" {
			n.sub = script
		} else {
			n.sup = script
		}
		if name == "stackrel" {
			n.cls = clsRel
		}
		return n, nil
	case "nonumber", "notag":
		return &node{kind: nGroup}, nil
	case "middle":
		return nil, p.errorf(t, `不支持 \middle`)
	case "limits", "nolimits":
		return nil, p.errorf(t, `\%s 只能跟在运算符后面`, name)
	}
	return nil, p.errorf(t, `不认识的命令 \%s`, name)
}

func (p *parser) charNode(t token) (*node, error) {
	r := t.r
	switch {
	case r == '~':
		return &node{kind: nSpace, cls: clsNone, size: spaces[" "]}, nil
	case isLetter(r), r >= '0' && r <= '9':
		return &node{kind: nSymbol, text: string(styled(r, p.variant))}, nil
	}
	if s, ok := chars[r]; ok {
		return &node{kind: nSymbol, text: string(s.r), cls: s.cls}, nil
	}
	if r > 0x7F && unicode.IsPrint(r) {
		return &node{kind: nSymbol, text: string(r), cls: runeClass[r]}, nil
	}
	return nil, p.errorf(t, "不能直接使用字符 %q", r)
}

// bigDelimiter 识别 \big、\Bigl、\biggr 等命令，返回定界符的类别与总高度（em）。
func bigDelimiter(name string) (class, float32, bool) {
	if size, ok := bigSizes[name]; ok {
		return clsOrd, size, true
	}
	if name == "" {
		return clsOrd, 0, false
	}
	size, ok := bigSizes[name[:len(name)-1]]
	if !ok {
		return clsOrd, 0, false
	}
	switch name[len(name)-1] {
	case 'l':
		return clsOpen, size, true
	case 'r':
		return clsClose, size, true
	case 'm':
		return clsRel, size, true
	}
	return clsOrd, 0, false
}

// parseDelimiter 读取 \left、\right、\big 之后的定界符。
func (p *parser) parseDelimiter(owner token) (rune, error) {
	t := p.next()
	key := string(t.r)
	if t.kind == tCmd {
		key = `\` + t.cmd
	}
	if r, ok := delimiters[key]; ok && t.kind != tEOF {
		return r, nil
	}
	return 0, p.errorf(t, `\%s 后面应为定界符，实际是 %s`, owner.cmd, t.describe())
}

func (p *parser) parseLeftRight(t token) (*node, error) {
	left, err := p.parseDelimiter(t)
	if err != nil {
		return nil, err
	}
	if err := p.enter(t); err != nil {
		return nil, err
	}
	list, err := p.parseList(false)
	if err != nil {
		return nil, err
	}
	end := p.next()
	if !end.isCmd("right") {
		return nil, p.errorf(t, `\left 缺少配对的 \right`)
	}
	right, err := p.parseDelimiter(end)
	if err != nil {
		return nil, err
	}
	p.depth--
	return &node{kind: nLeftRight, cls: clsInner, left: left, right: right, list: list}, nil
}

func (p *parser) parseSqrt(t token) (*node, error) {
	n := &node{kind: nSqrt}
	if open := p.peek(); open.isChar('[') {
		p.next()
		if err := p.enter(open); err != nil {
			return nil, err
		}
		index, err := p.parseList(true)
		if err != nil {
			return nil, err
		}
		if !p.next().isChar(']') {
			return nil, p.errorf(open, `\sqrt 的 [ 没有闭合`)
		}
		p.depth--
		n.b = &node{kind: nGroup, list: index}
	}
	arg, err := p.parseArg(`\sqrt`)
	if err != nil {
		return nil, err
	}
	n.a = arg
	return n, nil
}

// braceName 读取 {name} 形式的原文参数，用于环境名和 array 的列格式。
func (p *parser) braceName(owner token) (string, error) {
	if !p.next().isChar('{') {
		return "", p.errorf(owner, `\%s 后面应为 {…}`, owner.cmd)
	}
	from := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != '}' {
		if p.src[p.pos] == '{' || p.src[p.pos] == '\\' {
			return "", p.errorf(owner, `\%s 的参数里不能有 %c`, owner.cmd, p.src[p.pos])
		}
		p.pos++
	}
	if p.pos >= len(p.src) {
		return "", p.errorf(owner, `\%s 的 { 没有闭合`, owner.cmd)
	}
	name := strings.TrimSpace(string(p.src[from:p.pos]))
	p.pos++
	return name, nil
}

func (p *parser) parseEnvironment(t token) (*node, error) {
	name, err := p.braceName(t)
	if err != nil {
		return nil, err
	}
	spec, ok := environments[name]
	if !ok {
		return nil, p.errorf(t, "不支持的环境 %s", name)
	}
	if name == "array" {
		cols, err := p.braceName(t)
		if err != nil {
			return nil, err
		}
		var aligns []rune
		for _, r := range cols {
			if r == 'l' || r == 'c' || r == 'r' {
				aligns = append(aligns, r)
			}
		}
		if len(aligns) > 0 {
			spec.aligns = string(aligns)
		}
	}
	if err := p.enter(t); err != nil {
		return nil, err
	}
	rows, err := p.parseRows()
	if err != nil {
		return nil, err
	}
	end := p.next()
	if !end.isCmd("end") {
		return nil, p.errorf(t, `\begin{%s} 缺少配对的 \end`, name)
	}
	endName, err := p.braceName(end)
	if err != nil {
		return nil, err
	}
	if endName != name {
		return nil, p.errorf(end, `\begin{%s} 与 \end{%s} 不匹配`, name, endName)
	}
	p.depth--
	cls := clsOrd
	if spec.fenced {
		cls = clsInner
	}
	return &node{kind: nEnv, cls: cls, table: &tableNode{rows: rows, spec: spec}}, nil
}

// rawText 读取 \text{…} 的原文：保留空格（连续空白合并为一个），不解析数学命令。
func (p *parser) rawText(owner token) (string, error) {
	if !p.next().isChar('{') {
		return "", p.errorf(owner, `\%s 后面应为 {…}`, owner.cmd)
	}
	var out strings.Builder
	depth := 1
	space := false
	for p.pos < len(p.src) {
		r := p.src[p.pos]
		p.pos++
		switch {
		case r == '{':
			depth++
			continue
		case r == '}':
			depth--
			if depth == 0 {
				if space {
					out.WriteByte(' ')
				}
				return out.String(), nil
			}
			continue
		case unicode.IsSpace(r):
			space = true
			continue
		case r == '\\':
			if p.pos >= len(p.src) {
				return "", p.errorf(owner, "孤立的反斜杠")
			}
			r = p.src[p.pos]
			p.pos++
			switch {
			case unicode.IsSpace(r):
				space = true
				continue
			case isLetter(r):
				return "", p.errorf(owner, `\%s 里不支持命令`, owner.cmd)
			}
		}
		if space {
			out.WriteByte(' ')
			space = false
		}
		out.WriteRune(r)
	}
	return "", p.errorf(owner, `\%s 的 { 没有闭合`, owner.cmd)
}

// styledText 把文字里的字母和数字换成变体对应的字符；正体原样返回。
func styledText(s string, v variantKind) string {
	if v == varRoman {
		return s
	}
	return strings.Map(func(r rune) rune { return styled(r, v) }, s)
}
