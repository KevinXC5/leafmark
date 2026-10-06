package desktop

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// sourceKind 是源码着色的一类语法。0 表示普通正文。
type sourceKind uint8

const (
	sourcePlain sourceKind = iota
	sourceHeading
	sourceMark
	sourceLink
	sourceCode
	sourceMath
)

// sourceSpan 是一段连续、同色的源码，按 rune 计，半开区间。
type sourceSpan struct {
	start, end int
	kind       sourceKind
}

// sourceHighlight 扫描常用 Markdown：ATX 标题、强调与删除线、链接、
// 行内代码、围栏代码块，以及行内和块级公式。围栏代码块内部不再着色，
// 避免示例源码里的标记被当成语法。
func sourceHighlight(text string) []sourceSpan {
	runes := []rune(text)
	n := len(runes)
	var spans []sourceSpan
	add := func(start, end int, kind sourceKind) {
		if end > start {
			spans = append(spans, sourceSpan{start, end, kind})
		}
	}
	i := 0
	for i < n {
		if runes[i] == '\n' {
			i++
			continue
		}
		line := i
		for i < n && runes[i] != '\n' {
			i++
		}
		end := i
		if mark, width, ok := sourceFence(runes[line:end]); ok {
			closeAt := sourceFenceClose(runes, i, mark, width)
			add(line, closeAt, sourceCode)
			i = closeAt
			continue
		}
		if block, ok := sourceMathFence(runes[line:end]); ok {
			closeAt := sourceMathClose(runes, i, block)
			add(line, closeAt, sourceMath)
			i = closeAt
			continue
		}
		highlightLine(runes[line:end], line, add)
	}
	return spans
}

// sourceFence 识别一行开头的围栏。缩进最多三个空格，标记是至少三个同样的 ` 或 ~。
func sourceFence(line []rune) (rune, int, bool) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || (line[i] != '`' && line[i] != '~') {
		return 0, 0, false
	}
	mark := line[i]
	j := i
	for j < len(line) && line[j] == mark {
		j++
	}
	if j-i < 3 {
		return 0, 0, false
	}
	return mark, j - i, true
}

// sourceFenceClose 从开围栏所在行之后找到同样字符、至少同样长的关闭围栏。
// 找不到时整篇剩余源码都算代码，与 CommonMark 一致。
func sourceFenceClose(runes []rune, after int, mark rune, width int) int {
	i := after
	if i < len(runes) && runes[i] == '\n' {
		i++
	}
	for i < len(runes) {
		line := i
		for i < len(runes) && runes[i] != '\n' {
			i++
		}
		if fenceLine(runes[line:i], mark, width) {
			if i < len(runes) {
				i++
			}
			return i
		}
		if i < len(runes) {
			i++
		}
	}
	return len(runes)
}

func fenceLine(line []rune, mark rune, width int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != mark {
		return false
	}
	j := i
	for j < len(line) && line[j] == mark {
		j++
	}
	if j-i < width {
		return false
	}
	for _, r := range line[j:] {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

// sourceMathFence 识别独占一行的 $$。
func sourceMathFence(line []rune) (int, bool) {
	i := skipSpace(line, 0)
	if i+2 > len(line) || line[i] != '$' || line[i+1] != '$' {
		return 0, false
	}
	for _, r := range line[i+2:] {
		if r != ' ' && r != '\t' {
			return 0, false
		}
	}
	return 2, true
}

func sourceMathClose(runes []rune, after, width int) int {
	i := after
	if i < len(runes) && runes[i] == '\n' {
		i++
	}
	for i < len(runes) {
		line := i
		for i < len(runes) && runes[i] != '\n' {
			i++
		}
		if w, ok := sourceMathFence(runes[line:i]); ok && w >= width {
			if i < len(runes) {
				i++
			}
			return i
		}
		if i < len(runes) {
			i++
		}
	}
	return len(runes)
}

func highlightLine(line []rune, base int, add func(int, int, sourceKind)) {
	if start, ok := atxHeading(line); ok {
		add(base, base+len(line), sourceHeading)
		highlightInline(line[start:], base+start, add, true)
		return
	}
	highlightInline(line, base, add, false)
}

// atxHeading 返回标题文字的起点。最多三个空格、一到六个 #，后面要有空格或行尾。
func atxHeading(line []rune) (int, bool) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	marks := 0
	for i < len(line) && line[i] == '#' && marks < 6 {
		i++
		marks++
	}
	if marks == 0 || (i < len(line) && line[i] != ' ' && line[i] != '\t') {
		return 0, false
	}
	return i, true
}

// highlightInline 着色一行里的代码、公式、链接和强调。
// inHeading 时普通文字已经是标题色，只覆盖其中的代码、公式和链接。
func highlightInline(line []rune, base int, add func(int, int, sourceKind), inHeading bool) {
	n := len(line)
	for i := 0; i < n; i++ {
		switch line[i] {
		case '\\':
			if i+1 < n {
				i++
			}
		case '`':
			if end := closeRun(line, i, '`'); end > i+1 {
				add(base+i, base+end, sourceCode)
				i = end - 1
			}
		case '$':
			if end := closeMath(line, i); end > i+1 {
				add(base+i, base+end, sourceMath)
				i = end - 1
			}
		case '[':
			if label, url, ok := inlineLink(line, i); ok {
				add(base+i, base+url, sourceLink)
				i = url - 1
				_ = label
			}
		case '*', '_', '~':
			if inHeading {
				continue
			}
			if end := closeMark(line, i); end > i {
				add(base+i, base+end, sourceMark)
				i = end - 1
			}
		}
	}
}

// closeRun 找到与 line[at] 同样长的闭合标记，返回闭合标记之后的位置。
func closeRun(line []rune, at int, mark rune) int {
	width := 0
	for at+width < len(line) && line[at+width] == mark {
		width++
	}
	if width == 0 {
		return at
	}
	for i := at + width; i+width <= len(line); i++ {
		if line[i] != mark {
			continue
		}
		j := i
		for j < len(line) && line[j] == mark {
			j++
		}
		if j-i == width {
			return j
		}
		i = j - 1
	}
	return at
}

// closeMath 找到成对的 $ 或 $$。公式里不能换行，也不能紧贴空白。
func closeMath(line []rune, at int) int {
	width := 1
	if at+1 < len(line) && line[at+1] == '$' {
		width = 2
	}
	start := at + width
	if start >= len(line) || line[start] == ' ' || line[start] == '\t' {
		return at
	}
	for i := start; i+width <= len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			i++
			continue
		}
		if line[i] != '$' {
			continue
		}
		if width == 2 && (i+1 >= len(line) || line[i+1] != '$') {
			continue
		}
		if i > start && (line[i-1] == ' ' || line[i-1] == '\t') {
			continue
		}
		return i + width
	}
	return at
}

// closeMark 识别 **、__、*、_、~~ 包裹的强调。标记两侧不能都是空白。
func closeMark(line []rune, at int) int {
	mark := line[at]
	width := 1
	if at+1 < len(line) && line[at+1] == mark {
		width = 2
	}
	if mark == '~' {
		if width != 2 {
			return at
		}
	} else if mark != '*' && mark != '_' {
		return at
	}
	start := at + width
	if start >= len(line) || line[start] == ' ' || line[start] == '\t' {
		return at
	}
	for i := start; i+width <= len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			i++
			continue
		}
		if line[i] != mark {
			continue
		}
		j := i
		for j < len(line) && line[j] == mark && j-i < width {
			j++
		}
		if j-i != width {
			continue
		}
		if i > start && (line[i-1] == ' ' || line[i-1] == '\t') {
			continue
		}
		return j
	}
	return at
}

// inlineLink 识别 [文字](地址)，返回文字结束和整个链接结束的位置。
func inlineLink(line []rune, at int) (label, end int, ok bool) {
	if at >= len(line) || line[at] != '[' {
		return 0, 0, false
	}
	label = -1
	for i := at + 1; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			i++
			continue
		}
		if line[i] == ']' {
			label = i
			break
		}
		if line[i] == '[' || line[i] == '\n' {
			return 0, 0, false
		}
	}
	if label < 0 || label+1 >= len(line) || line[label+1] != '(' {
		return 0, 0, false
	}
	for i := label + 2; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			i++
			continue
		}
		if line[i] == ')' {
			return label, i + 1, true
		}
		if line[i] == '(' || line[i] == '\n' {
			return 0, 0, false
		}
	}
	return 0, 0, false
}

func skipSpace(line []rune, i int) int {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

// sourceIndex 返回 query 在 text 里从 from 起的下一处 rune 偏移，没有时为 -1。
func sourceIndex(text, query []rune, from int) int {
	if len(query) == 0 || from > len(text) {
		return -1
	}
	if from < 0 {
		from = 0
	}
	for i := from; i+len(query) <= len(text); i++ {
		if sourceEqual(text[i:i+len(query)], query) {
			return i
		}
	}
	return -1
}

func sourceEqual(a, b []rune) bool {
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sourceReplace 把 text 里的 query 换成 replacement。all 为假时只换第一处。
// 返回替换次数和新文本；query 为空时不替换。
func sourceReplace(text, query, replacement string, all bool) (int, string) {
	if query == "" {
		return 0, text
	}
	runes := []rune(text)
	q := []rune(query)
	rep := []rune(replacement)
	var b strings.Builder
	b.Grow(len(text))
	count := 0
	at := 0
	for {
		next := sourceIndex(runes, q, at)
		if next < 0 {
			b.WriteString(string(runes[at:]))
			break
		}
		b.WriteString(string(runes[at:next]))
		b.WriteString(replacement)
		count++
		at = next + len(q)
		if !all {
			b.WriteString(string(runes[at:]))
			break
		}
		_ = rep
	}
	return count, b.String()
}

// sourceRune 把字节偏移换成 rune 偏移，供和文本域的选区对齐。
func sourceRune(text string, byteAt int) int {
	if byteAt < 0 {
		return 0
	}
	if byteAt > len(text) {
		byteAt = len(text)
	}
	return utf8.RuneCountInString(text[:byteAt])
}

// sourceColors 是源码语法色，沿用书写界面的暖纸与灰蓝，不另用一套高饱和色。
type sourceColors struct {
	heading, mark, link, code, math ui.Color
}

func sourcePalette(dark bool) sourceColors {
	if dark {
		return sourceColors{
			heading: ui.Hex("#a6b7c8"),
			mark:    ui.Hex("#a9adb5"),
			link:    ui.Hex("#a6b7c8"),
			code:    ui.Hex("#a9adb5"),
			math:    ui.Hex("#a6b7c8"),
		}
	}
	return sourceColors{
		heading: ui.Hex("#bd7858"),
		mark:    ui.Hex("#797168"),
		link:    ui.Hex("#bd7858"),
		code:    ui.Hex("#797168"),
		math:    ui.Hex("#47423c"),
	}
}
