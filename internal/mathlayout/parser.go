package mathlayout

import (
	"fmt"
	"strings"
	"unicode"
)

type nodeKind uint8

const (
	nSymbol     nodeKind = iota // 单个符号：text、cls
	nText                       // 正体文字或函数名以外的文字串：text、cls
	nGroup                      // {…}：list
	nFrac                       // 分式：a 分子、b 分母
	nSqrt                       // 根式：a 被开方式、b 根指数（可空）
	nScript                     // 上下标：a 基（可空）、sub、sup、primes
	nOp                         // 大型运算符或函数名：text
	nLeftRight                  // \left … \right：left、right、list
	nDelim                      // \big 系列定界符：left、size、cls
	nAccent                     // 重音与上下划线：acc、a
	nSpace                      // 间距：size（mu）
	nStyle                      // \displaystyle 等：level
	nEnv                        // 环境与多行公式：table
	nMiddle                     // 与所在 left/right 同高的中间定界符
	nPhantom                    // 保留尺寸但不绘制
	nBoxed                      // 矩形边框
	nNot                        // 关系符上的否定斜线
	nDecoration                 // 上下括号及带标注箭头
	nColor                      // 颜色：a 非空时只染 a，否则染所在行的其余部分
	nTag                        // \tag 的编号文字：text
	nCancel                     // 删除线：text 为 cancel、bcancel 或 xcancel
	nSmash                      // 保留宽度但不占高度
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
	tint        [3]uint8
}

// tableSpec 描述一种环境的排法。
type tableSpec struct {
	aligns   string  // 各列对齐方式（l、c、r），列数超出时循环使用
	colSep   float32 // 列间距，em
	rowGap   float32 // 行间额外间距，em
	pad      float32 // 左定界符之后的间距，em
	pairs    bool    // 按“右对齐、左对齐”成对排列，对内不留间距（aligned）
	multline bool    // 首行左对齐、末行右对齐
	small    bool    // 单元格使用上标样式
	display  bool    // 单元格沿用外层样式；否则降为行内样式
	fenced   bool
	left     rune
	right    rune
	vlines   []int // array 列格式里的竖线：画在第几列之前，等于列数时画在最右侧
}

type tableNode struct {
	rows   [][][]*node
	spec   tableSpec
	hlines []int // \hline：画在第几行之前，等于行数时画在最下方
}

var environments = map[string]tableSpec{
	"matrix":       {aligns: "c", colSep: 1},
	"pmatrix":      {aligns: "c", colSep: 1, fenced: true, left: '(', right: ')'},
	"bmatrix":      {aligns: "c", colSep: 1, fenced: true, left: '[', right: ']'},
	"Bmatrix":      {aligns: "c", colSep: 1, fenced: true, left: '{', right: '}'},
	"vmatrix":      {aligns: "c", colSep: 1, fenced: true, left: '|', right: '|'},
	"Vmatrix":      {aligns: "c", colSep: 1, fenced: true, left: 0x2016, right: 0x2016},
	"array":        {aligns: "c", colSep: 1},
	"cases":        {aligns: "l", colSep: 1, pad: 0.17, fenced: true, left: '{'},
	"aligned":      {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"align":        {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"align*":       {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"split":        {aligns: "rl", colSep: 1.5, rowGap: 0.3, pairs: true, display: true},
	"gathered":     {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"gather":       {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"gather*":      {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"equation":     {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"equation*":    {aligns: "c", colSep: 1, rowGap: 0.3, display: true},
	"eqnarray":     {aligns: "rcl", colSep: 0.3, rowGap: 0.3, display: true},
	"eqnarray*":    {aligns: "rcl", colSep: 0.3, rowGap: 0.3, display: true},
	"smallmatrix":  {aligns: "c", colSep: 0.6, small: true},
	"psmallmatrix": {aligns: "c", colSep: 0.6, small: true, fenced: true, left: '(', right: ')'},
	"bsmallmatrix": {aligns: "c", colSep: 0.6, small: true, fenced: true, left: '[', right: ']'},
	"Bsmallmatrix": {aligns: "c", colSep: 0.6, small: true, fenced: true, left: '{', right: '}'},
	"vsmallmatrix": {aligns: "c", colSep: 0.6, small: true, fenced: true, left: '|', right: '|'},
	"Vsmallmatrix": {aligns: "c", colSep: 0.6, small: true, fenced: true, left: 0x2016, right: 0x2016},
	"subarray":     {aligns: "c", colSep: 0.3, small: true},
	"substack":     {aligns: "c", colSep: 0.3, small: true},
	"alignedat":    {aligns: "rl", pairs: true, display: true, rowGap: 0.3},
	"alignat":      {aligns: "rl", pairs: true, display: true, rowGap: 0.3},
	"alignat*":     {aligns: "rl", pairs: true, display: true, rowGap: 0.3},
	"flalign":      {aligns: "rl", colSep: 1.5, pairs: true, display: true, rowGap: 0.3},
	"flalign*":     {aligns: "rl", colSep: 1.5, pairs: true, display: true, rowGap: 0.3},
	"multline":     {aligns: "c", display: true, rowGap: 0.3, multline: true},
	"multline*":    {aligns: "c", display: true, rowGap: 0.3, multline: true},
	"multlined":    {aligns: "c", display: true, rowGap: 0.3, multline: true},
	"dcases":       {aligns: "l", colSep: 1, pad: 0.17, fenced: true, left: '{', display: true},
	"rcases":       {aligns: "l", colSep: 1, pad: 0.17, fenced: true, right: '}'},
	"drcases":      {aligns: "l", colSep: 1, pad: 0.17, fenced: true, right: '}', display: true},
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
	src        []rune
	pos        int
	depth      int
	variant    variantKind
	fenceDepth int
	tagDepth   int     // 允许出现 \tag 的嵌套层数；-1 表示当前位置不允许
	textMath   []*node // \text 里 $…$ 的内容，按占位字符的顺序存放
}

// parse 把 TeX 数学源码解析成节点树。
func parse(src string) (*node, error) {
	p := &parser{src: []rune(src)}
	if len(p.src) > maxLength {
		return nil, fmt.Errorf("公式过长（%d 个字符，上限 %d）", len(p.src), maxLength)
	}
	rows, hlines, err := p.parseRows()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.errorf(t, "多余的 %s", t.describe())
	}
	if len(hlines) > 0 {
		return nil, fmt.Errorf(`\hline 只能用在环境里`)
	}
	if len(rows) == 1 && len(rows[0]) == 1 && rowTag(rows[0]) == nil {
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
		if t.isCmd("over") || t.isCmd("atop") || t.isCmd("choose") {
			p.next()
			den, err := p.parseList(stopBracket)
			if err != nil {
				return nil, err
			}
			for _, item := range den {
				if item.kind == nFrac && item.text == "infix" {
					return nil, p.errorf(t, "同一组内只能有一个中缀分式命令")
				}
			}
			n := &node{kind: nFrac, a: &node{kind: nGroup, list: list}, b: &node{kind: nGroup, list: den}, binom: t.cmd == "choose", text: "infix"}
			if t.cmd == "atop" {
				n.size = -1
			}
			return []*node{n}, nil
		}
		n, err := p.parseItem()
		if err != nil {
			return nil, err
		}
		list = append(list, n)
	}
}

// rowTag 返回一行里 \tag 给出的编号节点，没有时返回 nil。
func rowTag(row [][]*node) *node {
	for _, cell := range row {
		for _, n := range cell {
			if n.kind == nTag {
				return n
			}
		}
	}
	return nil
}

// parseRows 解析以 & 分列、\\ 分行的内容，停在其他终止记号之前。
// 第二个返回值是 \hline 的位置：画在第几行之前。
func (p *parser) parseRows() ([][][]*node, []int, error) {
	var rows [][][]*node
	var row [][]*node
	var hlines []int
	variant := p.variant
	for {
		if len(row) == 0 {
			for p.peek().isCmd("hline") || p.peek().isCmd("hdashline") {
				p.next()
				hlines = append(hlines, len(rows))
			}
		}
		// 旧式字体切换只作用到单元格结尾。
		p.variant = variant
		cell, err := p.parseList(false)
		p.variant = variant
		if err != nil {
			return nil, nil, err
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
	return rows, hlines, nil
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
			if base != nil && base.kind == nDecoration && !strings.HasPrefix(base.text, "x") {
				s.limits = 1
			}
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
	variant := p.variant
	list, err := p.parseList(false)
	p.variant = variant
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
		return &node{kind: nSymbol, text: string(styledSymbol(s.r, p.variant)), cls: s.cls}, nil
	}
	if v, ok := variantSwitches[name]; ok {
		p.variant = v
		return &node{kind: nSpace, cls: clsNone}, nil
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
		return p.parseText(t, v)
	}
	if cls, size, ok := bigDelimiter(name); ok {
		r, err := p.parseDelimiter(t)
		if err != nil {
			return nil, err
		}
		return &node{kind: nDelim, cls: cls, left: r, size: size}, nil
	}
	switch name {
	case "frac", "cfrac", "dfrac", "tfrac", "binom", "dbinom", "tbinom":
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
		case 'd', 'c':
			n.fracStyle = 1
		case 't':
			n.fracStyle = 2
		}
		return n, nil
	case "overbrace", "underbrace", "overbracket", "underbracket", "overparen", "underparen":
		arg, err := p.parseArg(`\` + name)
		return &node{kind: nDecoration, a: arg, text: name}, err
	case "xrightarrow", "xleftarrow":
		var below *node
		if p.peek().isChar('[') {
			p.next()
			if err := p.enter(t); err != nil {
				return nil, err
			}
			list, err := p.parseList(true)
			if err != nil {
				return nil, err
			}
			if !p.next().isChar(']') {
				return nil, p.errorf(t, "箭头下标的 [ 没有闭合")
			}
			p.depth--
			below = &node{kind: nGroup, list: list}
		}
		above, err := p.parseArg(`\` + name)
		return &node{kind: nDecoration, text: name, sup: above, sub: below, cls: clsRel}, err
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
		text, err := p.plainText(t)
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
		// 叠在关系符或运算符上的标注不改变它的类别。
		n.cls = base.class()
		if base.kind == nGroup && len(base.list) == 1 && base.cls == clsOrd {
			n.cls = base.list[0].class()
		}
		if name == "stackrel" {
			n.cls = clsRel
		}
		return n, nil
	case "nonumber", "notag":
		return &node{kind: nSpace, cls: clsNone}, nil
	case "label":
		if _, err := p.braceName(t); err != nil {
			return nil, err
		}
		return &node{kind: nSpace, cls: clsNone}, nil
	case "tag":
		star := p.pos < len(p.src) && p.src[p.pos] == '*'
		if star {
			p.pos++
		}
		if p.depth != p.tagDepth {
			return nil, p.errorf(t, `\tag 只能直接写在公式的一行里`)
		}
		text, err := p.plainText(t)
		if err != nil {
			return nil, err
		}
		if !star {
			text = "(" + text + ")"
		}
		return &node{kind: nTag, cls: clsNone, text: text}, nil
	case "color":
		tint, err := p.parseColor(t)
		return &node{kind: nColor, cls: clsNone, tint: tint}, err
	case "textcolor":
		tint, err := p.parseColor(t)
		if err != nil {
			return nil, err
		}
		arg, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		return &node{kind: nColor, a: arg, cls: arg.class(), tint: tint}, nil
	case "hspace", "hskip", "kern", "mkern", "mskip":
		if name == "hspace" && p.pos < len(p.src) && p.src[p.pos] == '*' {
			p.pos++
		}
		mu, err := p.parseDimension(t)
		return &node{kind: nSpace, cls: clsNone, size: mu}, err
	case "cancel", "bcancel", "xcancel", "smash":
		arg, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		kind := nCancel
		if name == "smash" {
			kind = nSmash
		}
		return &node{kind: kind, a: arg, text: name, cls: arg.class()}, nil
	case "mathstrut", "strut":
		return &node{kind: nPhantom, text: "vphantom", a: &node{kind: nSymbol, text: "("}}, nil
	case "middle":
		if p.fenceDepth == 0 || p.depth != p.fenceDepth {
			return nil, p.errorf(t, `\middle 必须位于配对的 \left 与 \right 之间`)
		}
		r, err := p.parseDelimiter(t)
		return &node{kind: nMiddle, cls: clsRel, left: r}, err
	case "substack":
		if !p.next().isChar('{') {
			return nil, p.errorf(t, `\substack 缺少 {…}`)
		}
		if err := p.enter(t); err != nil {
			return nil, err
		}
		tagDepth := p.tagDepth
		p.tagDepth = -1
		rows, hlines, err := p.parseRows()
		p.tagDepth = tagDepth
		if err != nil {
			return nil, err
		}
		if len(hlines) > 0 {
			return nil, p.errorf(t, `\substack 里不能使用 \hline`)
		}
		if !p.next().isChar('}') {
			return nil, p.errorf(t, `\substack 的 { 没有闭合`)
		}
		p.depth--
		return &node{kind: nEnv, table: &tableNode{rows: rows, spec: environments["substack"]}}, nil
	case "boxed", "phantom", "hphantom", "vphantom", "not":
		arg, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		kind := nPhantom
		if name == "boxed" {
			kind = nBoxed
		}
		if name == "not" {
			// 有现成否定字形的关系符直接换用，笔画位置比叠一条斜线准确。
			if r := []rune(arg.text); arg.kind == nSymbol && len(r) == 1 && negated[r[0]] != 0 {
				return &node{kind: nSymbol, text: string(negated[r[0]]), cls: arg.cls}, nil
			}
			kind = nNot
		}
		return &node{kind: kind, a: arg, text: name, cls: arg.class()}, nil
	case "mathord", "mathop", "mathbin", "mathrel", "mathopen", "mathclose", "mathpunct", "mathinner":
		arg, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		classes := map[string]class{"mathord": clsOrd, "mathop": clsOp, "mathbin": clsBin, "mathrel": clsRel, "mathopen": clsOpen, "mathclose": clsClose, "mathpunct": clsPunct, "mathinner": clsInner}
		return &node{kind: nGroup, list: []*node{arg}, cls: classes[name]}, nil
	case "pmod", "pod", "mod":
		arg, err := p.parseArg(`\` + name)
		if err != nil {
			return nil, err
		}
		list := []*node{{kind: nSpace, cls: clsNone, size: 18}}
		if name != "mod" {
			list = append(list, &node{kind: nSymbol, text: "(", cls: clsOpen})
		}
		if name != "pod" {
			list = append(list, &node{kind: nText, text: "mod"}, &node{kind: nSpace, cls: clsNone, size: 6})
		}
		list = append(list, arg)
		if name != "mod" {
			list = append(list, &node{kind: nSymbol, text: ")", cls: clsClose})
		}
		return &node{kind: nGroup, list: list}, nil
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
		return &node{kind: nSymbol, text: string(styledSymbol(r, p.variant)), cls: runeClass[r]}, nil
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
	savedFence := p.fenceDepth
	p.fenceDepth = p.depth
	list, err := p.parseList(false)
	p.fenceDepth = savedFence
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
	if base, starred := strings.CutSuffix(name, "matrix*"); starred && !ok {
		// mathtools 的带星号矩阵：可选参数指定各列的对齐方式。
		if spec, ok = environments[base+"matrix"]; ok && p.peek().isChar('[') {
			p.next()
			align := p.next()
			if !align.isChar('l') && !align.isChar('c') && !align.isChar('r') || !p.next().isChar(']') {
				return nil, p.errorf(t, `%s 的对齐参数应为 [l]、[c] 或 [r]`, name)
			}
			spec.aligns = string(align.r)
		}
	}
	if !ok {
		return nil, p.errorf(t, "不支持的环境 %s", name)
	}
	if name == "alignat" || name == "alignat*" || name == "alignedat" {
		count, err := p.braceName(t)
		if err != nil {
			return nil, err
		}
		if count == "" || strings.Trim(count, "0123456789") != "" || strings.Trim(count, "0") == "" {
			return nil, p.errorf(t, "对齐列组数必须为正整数")
		}
	}
	if name == "array" || name == "subarray" {
		cols, err := p.braceName(t)
		if err != nil {
			return nil, err
		}
		var aligns []rune
		for _, r := range cols {
			switch r {
			case 'l', 'c', 'r':
				aligns = append(aligns, r)
			case '|':
				if name == "array" {
					spec.vlines = append(spec.vlines, len(aligns))
				}
			}
		}
		if len(aligns) > 0 {
			spec.aligns = string(aligns)
		}
	}
	if err := p.enter(t); err != nil {
		return nil, err
	}
	tagDepth := p.tagDepth
	p.tagDepth = -1
	if spec.display {
		p.tagDepth = p.depth
	}
	rows, hlines, err := p.parseRows()
	p.tagDepth = tagDepth
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
	return &node{kind: nEnv, cls: cls, table: &tableNode{rows: rows, spec: spec, hlines: hlines}}, nil
}

// parseColor 读取 {颜色名}、{#RRGGBB} 或 [HTML]{RRGGBB}。
func (p *parser) parseColor(owner token) ([3]uint8, error) {
	var none [3]uint8
	hex := false
	if p.peek().isChar('[') {
		p.next()
		from := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != ']' {
			p.pos++
		}
		if p.pos >= len(p.src) || string(p.src[from:p.pos]) != "HTML" {
			return none, p.errorf(owner, `\%s 的颜色模型只支持 [HTML]`, owner.cmd)
		}
		p.pos++
		hex = true
	}
	name, err := p.braceName(owner)
	if err != nil {
		return none, err
	}
	if digits, ok := strings.CutPrefix(name, "#"); ok || hex {
		if len(digits) == 3 {
			digits = string([]byte{digits[0], digits[0], digits[1], digits[1], digits[2], digits[2]})
		}
		var c [3]uint8
		if len(digits) == 6 {
			if _, err := fmt.Sscanf(strings.ToLower(digits), "%02x%02x%02x", &c[0], &c[1], &c[2]); err == nil && strings.Trim(strings.ToLower(digits), "0123456789abcdef") == "" {
				return c, nil
			}
		}
		return none, p.errorf(owner, "颜色 %s 不是 #RGB 或 #RRGGBB", name)
	}
	if c, ok := namedColors[name]; ok {
		return c, nil
	}
	return none, p.errorf(owner, "不认识的颜色 %s", name)
}

// dimensionUnits 是各长度单位折合的 mu 数，按 1em = 10pt = 18mu 换算。
var dimensionUnits = map[string]float32{
	"mu": 1, "em": 18, "ex": 7.75, "pt": 1.8, "px": 1.35, "bp": 1.8, "mm": 5.12, "cm": 51.2, "in": 130, "pc": 21.6,
}

// parseDimension 读取 {1em} 或紧跟在命令后的 1em，返回折合的 mu 数。
func (p *parser) parseDimension(owner token) (float32, error) {
	p.skipSpace()
	braced := p.pos < len(p.src) && p.src[p.pos] == '{'
	if braced {
		p.pos++
	}
	from := p.pos
	for p.pos < len(p.src) && p.pos-from < 24 && (strings.ContainsRune("+-. ", p.src[p.pos]) || p.src[p.pos] >= '0' && p.src[p.pos] <= '9') {
		p.pos++
	}
	number := strings.ReplaceAll(string(p.src[from:p.pos]), " ", "")
	unit := ""
	if p.pos+2 <= len(p.src) {
		unit = string(p.src[p.pos : p.pos+2])
	}
	scale, ok := dimensionUnits[unit]
	var value float32
	if _, err := fmt.Sscanf(number, "%g", &value); err != nil || !ok || value != value || value > 1000 || value < -1000 {
		return 0, p.errorf(owner, `\%s 后面应为长度，如 1em`, owner.cmd)
	}
	p.pos += 2
	if braced {
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != '}' {
			return 0, p.errorf(owner, `\%s 的 { 没有闭合`, owner.cmd)
		}
		p.pos++
	}
	return value * scale, nil
}

// textMathBase 是 \text 里 $…$ 的占位字符起点，位于私用区。
const textMathBase = 0xE000

// parseText 解析 \text{…}：文字用正体，其中的 $…$ 仍按公式排。
func (p *parser) parseText(owner token, v variantKind) (*node, error) {
	saved := p.textMath
	p.textMath = nil
	text, err := p.rawText(owner)
	maths := p.textMath
	p.textMath = saved
	if err != nil {
		return nil, err
	}
	if len(maths) == 0 {
		return &node{kind: nText, text: styledText(text, v)}, nil
	}
	var list []*node
	var run []rune
	flush := func() {
		if len(run) > 0 {
			list = append(list, &node{kind: nText, text: styledText(string(run), v)})
			run = nil
		}
	}
	for _, r := range text {
		if i := int(r - textMathBase); i >= 0 && i < len(maths) {
			flush()
			list = append(list, maths[i])
			continue
		}
		run = append(run, r)
	}
	flush()
	return &node{kind: nGroup, list: list}, nil
}

// plainText 读取不能含 $…$ 的文字参数（函数名、编号）。
func (p *parser) plainText(owner token) (string, error) {
	saved := p.textMath
	p.textMath = nil
	text, err := p.rawText(owner)
	maths := p.textMath
	p.textMath = saved
	if err == nil && len(maths) > 0 {
		return "", p.errorf(owner, `\%s 的参数里不能有 $…$`, owner.cmd)
	}
	return text, err
}

// inlineMath 解析 \text 里从当前位置到下一个 $ 的公式，返回占位字符。
func (p *parser) inlineMath(owner token) (string, error) {
	from := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != '$' {
		if p.src[p.pos] == '\\' {
			p.pos++
		}
		p.pos++
	}
	if p.pos >= len(p.src) {
		return "", p.errorf(owner, `\%s 里的 $ 没有闭合`, owner.cmd)
	}
	sub := &parser{src: p.src[from:p.pos], depth: p.depth, tagDepth: -1}
	p.pos++
	list, err := sub.parseList(false)
	if err != nil {
		return "", err
	}
	if t := sub.peek(); t.kind != tEOF {
		return "", p.errorf(owner, `\%s 的 $…$ 里有多余的 %s`, owner.cmd, t.describe())
	}
	if len(p.textMath) >= 0x400 {
		return "", p.errorf(owner, `\%s 里的 $…$ 太多`, owner.cmd)
	}
	p.textMath = append(p.textMath, &node{kind: nGroup, list: list})
	return string(rune(textMathBase + len(p.textMath) - 1)), nil
}

// rawText 保留文字空格，并解析常见转义、嵌套文字样式与文字符号。
func (p *parser) rawText(owner token) (string, error) {
	if !p.next().isChar('{') {
		return "", p.errorf(owner, `\%s 后面应为 {…}`, owner.cmd)
	}
	if err := p.enter(owner); err != nil {
		return "", err
	}
	defer func() { p.depth-- }()
	var out strings.Builder
	space := false
	write := func(s string) {
		if space {
			out.WriteByte(' ')
			space = false
		}
		out.WriteString(s)
	}
	for p.pos < len(p.src) {
		r := p.src[p.pos]
		p.pos++
		switch {
		case r == '}':
			if space {
				out.WriteByte(' ')
			}
			return out.String(), nil
		case r == '{':
			p.pos--
			s, err := p.rawText(owner)
			if err != nil {
				return "", err
			}
			write(s)
		case unicode.IsSpace(r) || r == '~':
			space = true
		case r == '$':
			s, err := p.inlineMath(owner)
			if err != nil {
				return "", err
			}
			write(s)
		case r >= textMathBase && r < textMathBase+0x400:
			return "", p.errorf(owner, "不能直接使用字符 %q", r)
		case r == '\\':
			p.pos--
			t := p.next()
			if v, ok := textCommands[t.cmd]; ok {
				s, err := p.rawText(t)
				if err != nil {
					return "", err
				}
				write(styledText(s, v))
				continue
			}
			if v, ok := variantCommands[t.cmd]; ok {
				s, err := p.rawText(t)
				if err != nil {
					return "", err
				}
				write(styledText(s, v))
				continue
			}
			if _, ok := spaces[t.cmd]; ok {
				space = true
				continue
			}
			if s, ok := textSymbols[t.cmd]; ok {
				write(s)
				continue
			}
			if s, ok := symbols[t.cmd]; ok && len(t.cmd) == 1 {
				write(string(s.r))
				continue
			}
			return "", p.errorf(t, `文字里不支持命令 \%s`, t.cmd)
		default:
			write(string(r))
		}
	}
	return "", p.errorf(owner, `\%s 的 { 没有闭合`, owner.cmd)
}

var textSymbols = map[string]string{
	"textbackslash": "\\", "textasciitilde": "~", "textasciicircum": "^",
	"textless": "<", "textgreater": ">", "textbar": "|", "textbraceleft": "{", "textbraceright": "}",
	"textunderscore": "_", "textemdash": "—", "textendash": "–", "textellipsis": "…",
	"textdegree": "°", "textcopyright": "©", "copyright": "©", "textregistered": "®", "texttrademark": "™",
	"S": "§", "P": "¶", "ldots": "…", "dots": "…", "LaTeX": "LaTeX", "TeX": "TeX",
	"ae": "æ", "AE": "Æ", "oe": "œ", "OE": "Œ", "ss": "ß", "o": "ø", "O": "Ø", "l": "ł", "L": "Ł",
}

// styledText 把文字里的字母和数字换成变体对应的字符；正体原样返回。
func styledText(s string, v variantKind) string {
	if v == varRoman {
		return s
	}
	return strings.Map(func(r rune) rune { return styled(r, v) }, s)
}
