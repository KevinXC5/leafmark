package richtext

import (
	"bytes"
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

// rawHTML 把未进入模型的原文交给通用渲染器，脚注、原始 HTML 等仍能导出。
var rawHTML = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote),
	goldmark.WithRendererOptions(ghtml.WithUnsafe()),
)

// HTML 把文档写成 HTML 片段，供导出与打印使用。
func (d *Document) HTML() string { return d.html(nil, Renderers{}) }

// HTMLWith 与 HTML 相同，图片地址先经 image 改写（例如换成内嵌数据），返回空串时保留原地址。
func (d *Document) HTMLWith(image func(url string) string) string {
	return d.html(image, Renderers{})
}

func (d *Document) html(image func(url string) string, r Renderers) string {
	var out strings.Builder
	open := []openBox{} // 已打开的容器，外层在前
	var simple *simpleList
	closeTo := func(depth int) {
		for len(open) > depth {
			box := open[len(open)-1]
			open = open[:len(open)-1]
			if box.item {
				out.WriteString("</li>")
			}
			out.WriteString("</" + box.tag + ">\n")
		}
	}
	notes := map[string]int{}
	var noteOrder []string
	noteText := noteNumbers(d.blocks)
	refs := map[string]int{}
	// 引用式定义（含脚注）先整篇收集再渲染一次。按 Raw 块各自渲染时，
	// 定义和引用不在同一块里，交叉脚注会被 goldmark 丢掉。
	defined := collectReferenceDefs(d.blocks)
	for i := range d.blocks {
		b := &d.blocks[i]
		// 引用式定义不进入可见正文，也不参与列表开关，避免空段落或截断相邻列表。
		if b.Kind == ReferenceDef {
			continue
		}
		// 脚注定义只在文末列出，不插在正文流里。
		if b.Footnote != nil && b.Footnote.Label != "" {
			if _, seen := notes[b.Footnote.Label]; !seen {
				notes[b.Footnote.Label] = b.Footnote.Number
				noteOrder = append(noteOrder, b.Footnote.Label)
			}
			continue
		}
		chain := containerChain(b)
		// 简单列表、引用和提示块没有容器上下文，按相邻块的类型与层级开关。
		if len(b.Containers) == 0 {
			closeTo(0)
			writeSimple(&out, b, image, r, noteText, &refs, &simple)
			continue
		}
		closeSimple(&out, simple, 0)
		simple = nil
		// 容器变化时先合上不再共用的层，再打开新的层。相同 ID 表示仍是同一个容器。
		shared := 0
		for shared < len(open) && shared < len(chain) && open[shared].id == chain[shared].ID {
			shared++
		}
		closeTo(shared)
		for _, c := range chain[shared:] {
			openContainer(&out, &open, c)
		}
		// 列表项的第一叶才打开 li；同一项里的续段继续写在这个 li 中。
		if item, ok := listItem(chain); ok && (len(open) == 0 || !open[len(open)-1].item) {
			if len(open) > 0 {
				open[len(open)-1].item = true
			}
			writeItemOpen(&out, item)
		}
		// 列表项里的段落直接写文字，不套 p，与简单列表的输出一致；续段各自成块。
		if b.Kind == Paragraph && len(chain) > 0 && (chain[len(chain)-1].Kind == List || chain[len(chain)-1].Kind == Task) {
			writeRunsNumbered(&out, b.Runs, r, noteText, &refs, image)
			out.WriteByte('\n')
			continue
		}
		writeLeaf(&out, b, image, r, noteText, &refs, defined)
	}
	closeTo(0)
	closeSimple(&out, simple, 0)
	writeFootnotes(&out, d.blocks, noteOrder, notes, r, &refs)
	return out.String()
}

// simpleList 记住还没合上的简单列表，用来处理嵌套与相邻列表项。
type simpleList struct {
	tags []string
}

// writeSimple 写出没有容器上下文的块，列表按层级嵌套，其余块各自成段。
func writeSimple(out *strings.Builder, b *editBlock, image func(url string) string, r Renderers, numbers map[string]string, refs *map[string]int, simple **simpleList) {
	if b.Kind != List && b.Kind != Task {
		closeSimple(out, *simple, 0)
		*simple = nil
		switch b.Kind {
		case Quote:
			out.WriteString("<blockquote><p>")
			writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
			out.WriteString("</p></blockquote>\n")
		case Callout:
			kind, title := "note", ""
			if b.Callout != nil {
				kind, title = strings.ToLower(b.Callout.Type), b.Callout.Title
			}
			out.WriteString(`<div class="callout callout-` + html.EscapeString(kind) + `">`)
			if title != "" {
				out.WriteString(`<p class="callout-title">` + html.EscapeString(title) + "</p>")
			}
			if len(b.Runs) > 0 {
				out.WriteString("<p>")
				writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
				out.WriteString("</p>")
			}
			out.WriteString("</div>\n")
		default:
			writeLeaf(out, b, image, r, numbers, refs, "")
		}
		return
	}
	if *simple == nil {
		*simple = &simpleList{}
	}
	list := *simple
	level := max(1, b.Level)
	tag := "ul"
	if b.Ordered {
		tag = "ol"
	}
	// 顶层有序与无序交替时是两个列表。
	if level == 1 && len(list.tags) > 0 && list.tags[0] != tag {
		closeSimple(out, list, 0)
	}
	if len(list.tags) >= level {
		closeSimple(out, list, level)
		out.WriteString("</li>\n")
	}
	for len(list.tags) < level {
		open := "<" + tag
		if tag == "ol" && b.Start > 1 {
			open += ` start="` + strconv.Itoa(b.Start) + `"`
		}
		out.WriteString(open + ">\n")
		list.tags = append(list.tags, tag)
		if len(list.tags) < level {
			out.WriteString("<li>")
		}
	}
	writeItemOpen(out, Container{Kind: b.Kind, Checked: b.Checked})
	writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
}

func closeSimple(out *strings.Builder, list *simpleList, depth int) {
	if list == nil {
		return
	}
	for len(list.tags) > depth {
		out.WriteString("</li></" + list.tags[len(list.tags)-1] + ">\n")
		list.tags = list.tags[:len(list.tags)-1]
	}
}

// openBox 是导出时已经写出开头标签的容器。
type openBox struct {
	id   int
	tag  string
	item bool // 列表容器里当前有一个未关闭的 li
}

// listItem 取出最内层的列表项容器。续段的自身 Kind 可能是段落或代码，不能靠它判断。
func listItem(chain []Container) (Container, bool) {
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Kind == List || chain[i].Kind == Task {
			return chain[i], true
		}
	}
	return Container{}, false
}

// containerChain 给出这一叶块从外到内的容器。没有上下文时，简单列表、引用和提示块自身就是容器。
func containerChain(b *editBlock) []Container {
	if len(b.Containers) > 0 {
		return b.Containers
	}
	switch b.Kind {
	case List, Task, Quote, Callout:
		return []Container{{
			Kind: b.Kind, ID: -1, Level: b.Level, Ordered: b.Ordered, Start: b.Start, Checked: b.Checked, Callout: b.Callout,
		}}
	default:
		return nil
	}
}

func openContainer(out *strings.Builder, open *[]openBox, c Container) {
	box := openBox{id: c.ID}
	switch c.Kind {
	case List, Task:
		box.tag = "ul"
		if c.Ordered {
			box.tag = "ol"
		}
		openTag := "<" + box.tag
		if box.tag == "ol" && c.Start > 1 {
			openTag += ` start="` + strconv.Itoa(c.Start) + `"`
		}
		out.WriteString(openTag + ">\n")
	case Callout:
		box.tag = "div"
		kind, title := "note", ""
		if c.Callout != nil {
			kind, title = strings.ToLower(c.Callout.Type), c.Callout.Title
		}
		// 来自 <details> 的折叠块按 HTML 写出，不套提示块样式。
		if c.IsHTML() {
			box.tag = "details"
			openAttr := ""
			if c.Callout != nil && c.Callout.Fold == "+" {
				openAttr = " open"
			}
			out.WriteString("<details" + openAttr + ">\n")
			if title != "" {
				out.WriteString("<summary>" + html.EscapeString(title) + "</summary>\n")
			}
			break
		}
		out.WriteString(`<div class="callout callout-` + html.EscapeString(kind) + `">`)
		if title != "" {
			out.WriteString(`<p class="callout-title">` + html.EscapeString(title) + "</p>")
		}
		out.WriteByte('\n')
	default:
		box.tag = "blockquote"
		out.WriteString("<blockquote>\n")
	}
	*open = append(*open, box)
}

func writeItemOpen(out *strings.Builder, c Container) {
	if c.Kind == Task || c.Checked {
		checked := ""
		if c.Checked {
			checked = " checked"
		}
		out.WriteString(`<li class="task"><input type="checkbox" disabled` + checked + "> ")
		return
	}
	out.WriteString("<li>")
}

func writeLeaf(out *strings.Builder, b *editBlock, image func(url string) string, r Renderers, numbers map[string]string, refs *map[string]int, defined string) {
	switch b.Kind {
	case Heading:
		tag := "h" + strconv.Itoa(max(1, min(b.Level, 6)))
		out.WriteString("<" + tag + ">")
		writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
		out.WriteString("</" + tag + ">\n")
	case List, Task:
		writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
		out.WriteByte('\n')
	case Quote, Callout:
		if len(b.Runs) > 0 {
			out.WriteString("<p>")
			writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
			out.WriteString("</p>\n")
		}
	case Code:
		class := ""
		if b.Lang != "" {
			class = ` class="language-` + html.EscapeString(strings.Fields(b.Lang)[0]) + `"`
		}
		out.WriteString("<pre><code" + class + ">" + html.EscapeString(b.Code) + "</code></pre>\n")
	case Horizontal:
		out.WriteString("<hr>\n")
	case TableBlock:
		writeTableHTML(out, b.Table, image, r, numbers, refs)
	case Image:
		title := ""
		if b.Title != "" {
			title = ` title="` + html.EscapeString(b.Title) + `"`
		}
		src := b.URL
		if image != nil {
			if mapped := image(src); mapped != "" {
				src = mapped
			}
		}
		out.WriteString(`<p><img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(b.Alt) + `"` + title + "></p>\n")
	case Raw:
		source := b.Raw
		if source == "" {
			source = b.source
		}
		if writeRenderedRaw(out, source, r) {
			break
		}
		var buf bytes.Buffer
		if err := rawHTML.Convert([]byte(rawWithDefs(source, defined)), &buf); err != nil {
			out.WriteString("<pre>" + html.EscapeString(source) + "</pre>\n")
			break
		}
		// 原始 HTML 里的脚本不进入导出文件，其余内容保持原样。
		out.WriteString(stripActiveContent(buf.String()))
	default:
		out.WriteString("<p>")
		writeRunsNumbered(out, b.Runs, r, numbers, refs, image)
		out.WriteString("</p>\n")
	}
}

// collectReferenceDefs 收集全文里的链接定义与脚注定义。
// 只取以定义标记开头的行，避免把普通段落里的冒号当成定义。
func collectReferenceDefs(blocks []editBlock) string {
	var out strings.Builder
	seen := map[string]bool{}
	for i := range blocks {
		b := &blocks[i]
		if b.Kind != Raw {
			continue
		}
		source := b.Raw
		if source == "" {
			source = b.source
		}
		for _, line := range strings.Split(source, "\n") {
			trim := strings.TrimSpace(line)
			if !strings.HasPrefix(trim, "[") || !strings.Contains(trim, "]:") {
				continue
			}
			if seen[trim] {
				continue
			}
			seen[trim] = true
			out.WriteString(trim)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// rawWithDefs 渲染单个 Raw 块时带上全文定义，本块已经含有的定义不重复附加。
func rawWithDefs(source, defined string) string {
	if defined == "" {
		return source
	}
	var extra strings.Builder
	for _, line := range strings.Split(defined, "\n") {
		if line == "" || strings.Contains(source, line) {
			continue
		}
		extra.WriteString(line)
		extra.WriteByte('\n')
	}
	if extra.Len() == 0 {
		return source
	}
	return source + "\n\n" + extra.String()
}

// footnoteRefID 给一处脚注引用分配回跳锚点。同一标签第一次是 fnref-标签，之后加序号。
func footnoteRefID(label string, refs *map[string]int) string {
	if refs == nil {
		return "fnref-" + html.EscapeString(label)
	}
	if *refs == nil {
		*refs = map[string]int{}
	}
	(*refs)[label]++
	id := "fnref-" + html.EscapeString(label)
	if n := (*refs)[label]; n > 1 {
		id += "-" + strconv.Itoa(n)
	}
	return id
}

// imageTag 把行内图片写成 img。地址先经 image 改写，与块级图片一致。
func imageTag(img *InlineImage, image func(url string) string) string {
	if img == nil {
		return ""
	}
	src := img.URL
	if image != nil {
		if mapped := image(src); mapped != "" {
			src = mapped
		}
	}
	title := ""
	if img.Title != "" {
		title = ` title="` + html.EscapeString(img.Title) + `"`
	}
	return `<img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(img.Alt) + `"` + title + `>`
}

// writeRunsNumbered 写出一段文字。numbers 给出脚注标签对应的显示编号，为空时沿用原文。
// refs 记录每个脚注标签已经写出的引用次数，用来生成不重复的回跳锚点。
func writeRunsNumbered(out *strings.Builder, runs []Run, render Renderers, numbers map[string]string, refs *map[string]int, image func(url string) string) {
	for _, r := range runs {
		if r.Image != nil {
			out.WriteString(imageTag(r.Image, image))
			continue
		}
		if r.Marks&MarkMath != 0 && render.InlineMath != nil {
			if svg, descent, ok := render.InlineMath(r.Text); ok && isSafeSVG(svg) {
				// 基线对齐：把 SVG 下移自身基线以下的深度，与周围文字的基线重合。
				out.WriteString(`<span class="math" style="vertical-align:-` + formatPx(descent) + `px">` + svg + `</span>`)
				continue
			}
		}
		text := strings.ReplaceAll(html.EscapeString(r.Text), "\n", "<br>\n")
		// 标签由内到外套，顺序固定，输出稳定。公式没有排版结果时仍用源码。
		for _, m := range []struct {
			mark Mark
			tag  string
		}{{MarkCode, "code"}, {MarkKbd, "kbd"}, {MarkMath, `span class="math"`}, {MarkSub, "sub"}, {MarkSup, "sup"}, {MarkUnderline, "u"}, {MarkStrike, "del"}, {MarkItalic, "em"}, {MarkBold, "strong"}, {MarkHighlight, "mark"}} {
			if r.Marks&m.mark != 0 {
				text = "<" + m.tag + ">" + text + "</" + strings.Fields(m.tag)[0] + ">"
			}
		}
		if r.Link != nil && r.Link.Footnote != "" {
			label := html.EscapeString(r.Link.Footnote)
			// 引用显示文档内编号，锚点仍用标签。同一标签被引用多次时，回跳锚点带序号，避免 id 重复。
			shown := text
			if numbers != nil {
				if n, ok := numbers[r.Link.Footnote]; ok {
					shown = "<sup>" + html.EscapeString(n) + "</sup>"
				}
			}
			text = `<a class="footnote-ref" href="#fn-` + label + `" id="` + footnoteRefID(r.Link.Footnote, refs) + `">` + shown + "</a>"
		} else if r.Link != nil {
			title := ""
			if r.Link.Title != "" {
				title = ` title="` + html.EscapeString(r.Link.Title) + `"`
			}
			text = `<a href="` + html.EscapeString(r.Link.URL) + `"` + title + ">" + text + "</a>"
		}
		out.WriteString(text)
	}
}

// noteNumbers 给出脚注标签到显示编号的映射。
func noteNumbers(blocks []editBlock) map[string]string {
	out := map[string]string{}
	for i := range blocks {
		if f := blocks[i].Footnote; f != nil && f.Label != "" {
			if _, seen := out[f.Label]; !seen {
				out[f.Label] = strconv.Itoa(max(1, f.Number))
			}
		}
	}
	return out
}

func writeFootnotes(out *strings.Builder, blocks []editBlock, order []string, numbers map[string]int, r Renderers, refs *map[string]int) {
	if len(order) == 0 {
		return
	}
	out.WriteString("<section class=\"footnotes\">\n<ol>\n")
	for _, label := range order {
		out.WriteString(`<li id="fn-` + html.EscapeString(label) + `"`)
		if n := numbers[label]; n > 1 {
			out.WriteString(` value="` + strconv.Itoa(n) + `"`)
		}
		out.WriteString(">\n")
		for i := range blocks {
			note := blocks[i].Footnote
			if note == nil || note.Label != label {
				continue
			}
			writeLeaf(out, &blocks[i], nil, r, noteNumbers(blocks), refs, "")
		}
		// 多次引用时回跳到第一处；只有一处时锚点不带序号。
		back := "fnref-" + html.EscapeString(label)
		if refs != nil && (*refs)[label] > 1 {
			back += "-1"
		}
		out.WriteString(`<a class="footnote-back" href="#` + back + `">↩</a>` + "\n</li>\n")
	}
	out.WriteString("</ol>\n</section>\n")
}

// writeRenderedRaw 把独占一块的公式或流程图写成 SVG。不能排版时返回 false，调用方继续按原文导出。
func writeRenderedRaw(out *strings.Builder, source string, r Renderers) bool {
	if src, ok := displayMathSource(source); ok && r.DisplayMath != nil {
		if svg, drawn := r.DisplayMath(src); drawn && isSafeSVG(svg) {
			out.WriteString(`<div class="math-display">` + svg + "</div>\n")
			return true
		}
	}
	if src, ok := mermaidSource(source); ok && r.Diagram != nil {
		if svg, drawn := r.Diagram(src); drawn && isSafeSVG(svg) {
			out.WriteString(`<div class="diagram">` + svg + "</div>\n")
			return true
		}
	}
	return false
}

// isSafeSVG 只接受单个自包含的 svg 根：允许 path、text、g、rect、line 与 font-family，
// 拒绝事件属性、脚本、foreignObject、外部资源，以及包在其他标记里的 SVG。
func isSafeSVG(svg string) bool {
	s := strings.TrimSpace(svg)
	root, ok := singleSVGRoot(s)
	if !ok {
		return false
	}
	return svgTokensSafe(root)
}

// singleSVGRoot 确认去掉首尾空白后只剩一个 svg 元素，且没有前后的其他标记。
func singleSVGRoot(s string) (string, bool) {
	if len(s) < len("<svg></svg>") || !strings.HasPrefix(strings.ToLower(s), "<svg") {
		return "", false
	}
	i := len("<svg")
	if i < len(s) && isNameByte(s[i]) {
		return "", false
	}
	depth := 0
	end := -1
	for tok := range scanHTML(s) {
		switch tok.kind {
		case tokStart:
			if tok.name == "svg" {
				depth++
			}
		case tokEnd:
			if tok.name == "svg" {
				depth--
				if depth == 0 {
					end = tok.end
				}
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 || strings.TrimSpace(s[end:]) != "" {
		return "", false
	}
	return s[:end], true
}

// svgTokensSafe 按标记扫描 SVG，不把属性值或文字里的字面量当成标签。
func svgTokensSafe(s string) bool {
	allowed := map[string]bool{
		"svg": true, "g": true, "path": true, "text": true, "rect": true, "line": true,
		"title": true, "desc": true, "tspan": true, "polyline": true, "polygon": true,
		"circle": true, "ellipse": true, "defs": true, "use": true,
	}
	for tok := range scanHTML(s) {
		switch tok.kind {
		case tokStart, tokEnd:
			if !allowed[tok.name] {
				return false
			}
		case tokAttr:
			if !svgAttrSafe(tok.name, tok.value) {
				return false
			}
		case tokText:
			if strings.ContainsAny(tok.value, "<>") {
				return false
			}
		case tokBad:
			return false
		}
	}
	return true
}

// svgAttrSafe 拒绝事件、脚本、样式表和会拉外部资源的地址。font-family 允许。
func svgAttrSafe(name, value string) bool {
	if name == "" || strings.HasPrefix(name, "on") {
		return false
	}
	switch name {
	case "href", "src", "xlink:href":
		return value == "" || strings.HasPrefix(value, "#")
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") {
		return false
	}
	if strings.Contains(lower, "url(") && !strings.Contains(lower, "url(#") {
		return false
	}
	return true
}

type htmlTokKind int

const (
	tokStart htmlTokKind = iota
	tokEnd
	tokAttr
	tokText
	tokBad
)

type htmlTok struct {
	kind        htmlTokKind
	name, value string
	end         int
}

// scanHTML 是最小的 HTML 标记扫描：区分标签、属性、引号内的值与正文。
// 不执行、不补全标签，遇到无法判断的结构产出 tokBad。
func scanHTML(s string) func(func(htmlTok) bool) {
	return func(yield func(htmlTok) bool) {
		i := 0
		for i < len(s) {
			if s[i] != '<' {
				j := strings.IndexByte(s[i:], '<')
				if j < 0 {
					j = len(s) - i
				}
				if !yield(htmlTok{kind: tokText, value: s[i : i+j]}) {
					return
				}
				i += j
				continue
			}
			if strings.HasPrefix(s[i:], "<!--") {
				j := strings.Index(s[i+4:], "-->")
				if j < 0 {
					yield(htmlTok{kind: tokBad})
					return
				}
				i += 4 + j + 3
				continue
			}
			j := i + 1
			closing := false
			if j < len(s) && s[j] == '/' {
				closing = true
				j++
			}
			nameStart := j
			for j < len(s) && isNameByte(s[j]) {
				j++
			}
			if j == nameStart {
				yield(htmlTok{kind: tokBad})
				return
			}
			name := strings.ToLower(s[nameStart:j])
			selfClose := false
			for j < len(s) && s[j] != '>' {
				for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r' || s[j] == '/') {
					if s[j] == '/' {
						selfClose = true
					}
					j++
				}
				if j >= len(s) || s[j] == '>' {
					break
				}
				attrStart := j
				for j < len(s) && (isNameByte(s[j]) || s[j] == ':' || s[j] == '-') {
					j++
				}
				if j == attrStart {
					yield(htmlTok{kind: tokBad})
					return
				}
				attr := strings.ToLower(s[attrStart:j])
				for j < len(s) && unicode.IsSpace(runeAt(s, j)) {
					j += runeLen(s, j)
				}
				value := ""
				if j < len(s) && s[j] == '=' {
					j++
					for j < len(s) && unicode.IsSpace(runeAt(s, j)) {
						j += runeLen(s, j)
					}
					if j >= len(s) {
						yield(htmlTok{kind: tokBad})
						return
					}
					switch s[j] {
					case '"', '\'':
						quote := s[j]
						j++
						v := j
						for j < len(s) && s[j] != quote {
							j++
						}
						if j >= len(s) {
							yield(htmlTok{kind: tokBad})
							return
						}
						value = s[v:j]
						j++
					default:
						v := j
						for j < len(s) && !unicode.IsSpace(runeAt(s, j)) && s[j] != '>' && s[j] != '/' {
							j++
						}
						value = s[v:j]
					}
				}
				if !closing && !yield(htmlTok{kind: tokAttr, name: attr, value: html.UnescapeString(value)}) {
					return
				}
			}
			if j >= len(s) || s[j] != '>' {
				yield(htmlTok{kind: tokBad})
				return
			}
			j++
			kind := tokStart
			if closing {
				kind = tokEnd
			}
			if !yield(htmlTok{kind: kind, name: name, end: j}) {
				return
			}
			if selfClose && !closing {
				if !yield(htmlTok{kind: tokEnd, name: name, end: j}) {
					return
				}
			}
			i = j
		}
	}
}

func isNameByte(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func runeAt(s string, i int) rune {
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

func runeLen(s string, i int) int {
	_, n := utf8.DecodeRuneInString(s[i:])
	if n <= 0 {
		return 1
	}
	return n
}

// stripActiveContent 去掉原始 HTML 中会执行的部分：script 与 foreignObject 整段删除，
// 事件处理属性按标记去掉。正文和属性值里形如 onclick="文字" 的普通文本保留。
func stripActiveContent(s string) string {
	// 没有脚本和事件时原样返回，避免改动正文空白。
	if !htmlHasActive(s) {
		return s
	}
	return rewriteSafeHTML(s)
}

// htmlHasActive 判断原文里是否真有脚本、foreignObject 或事件属性。
func htmlHasActive(s string) bool {
	for tok := range scanHTML(s) {
		switch tok.kind {
		case tokBad:
			return true
		case tokStart:
			if tok.name == "script" || tok.name == "foreignobject" {
				return true
			}
		case tokAttr:
			if strings.HasPrefix(tok.name, "on") {
				return true
			}
		}
	}
	return false
}

// rewriteSafeHTML 按标记重写：丢掉 script、foreignObject 和事件属性，其余标签与正文原样保留。
func rewriteSafeHTML(s string) string {
	var out strings.Builder
	skipUntil := ""
	i := 0
	for i < len(s) {
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				j = len(s) - i
			}
			if skipUntil == "" {
				out.WriteString(s[i : i+j])
			}
			i += j
			continue
		}
		if strings.HasPrefix(s[i:], "<!--") {
			j := strings.Index(s[i+4:], "-->")
			if j < 0 {
				break
			}
			end := i + 4 + j + 3
			if skipUntil == "" {
				out.WriteString(s[i:end])
			}
			i = end
			continue
		}
		end, name, closing := tagEnd(s, i)
		if end < 0 {
			if skipUntil == "" {
				out.WriteString(html.EscapeString(s[i:]))
			}
			break
		}
		lower := strings.ToLower(name)
		if skipUntil != "" {
			if closing && lower == skipUntil {
				skipUntil = ""
			}
			i = end
			continue
		}
		if !closing && (lower == "script" || lower == "foreignobject") {
			if !tagSelfCloses(s[i:end]) {
				skipUntil = lower
			}
			i = end
			continue
		}
		out.WriteString(tagWithoutEvents(s[i:end]))
		i = end
	}
	return out.String()
}

// tagEnd 返回从 '<' 开始的标签结束位置（'>' 之后）和标签名。
func tagEnd(s string, i int) (end int, name string, closing bool) {
	j := i + 1
	if j < len(s) && s[j] == '/' {
		closing = true
		j++
	}
	start := j
	for j < len(s) && isNameByte(s[j]) {
		j++
	}
	if j == start {
		return -1, "", false
	}
	name = s[start:j]
	inQuote := byte(0)
	for j < len(s) {
		if inQuote != 0 {
			if s[j] == inQuote {
				inQuote = 0
			}
			j++
			continue
		}
		if s[j] == '"' || s[j] == '\'' {
			inQuote = s[j]
			j++
			continue
		}
		if s[j] == '>' {
			return j + 1, name, closing
		}
		j++
	}
	return -1, name, closing
}

func tagSelfCloses(tag string) bool {
	t := strings.TrimSpace(tag)
	return strings.HasSuffix(t, "/>")
}

// tagWithoutEvents 去掉一个标签上的事件属性，无引号的值一并去掉。
func tagWithoutEvents(tag string) string {
	var out strings.Builder
	i := 0
	if i < len(tag) && tag[i] == '<' {
		out.WriteByte('<')
		i++
	}
	if i < len(tag) && tag[i] == '/' {
		out.WriteByte('/')
		i++
	}
	start := i
	for i < len(tag) && isNameByte(tag[i]) {
		i++
	}
	out.WriteString(tag[start:i])
	for i < len(tag) && tag[i] != '>' {
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\n' || tag[i] == '\r') {
			out.WriteByte(tag[i])
			i++
		}
		if i >= len(tag) || tag[i] == '>' || tag[i] == '/' {
			break
		}
		attrStart := i
		for i < len(tag) && (isNameByte(tag[i]) || tag[i] == ':' || tag[i] == '-') {
			i++
		}
		attr := strings.ToLower(tag[attrStart:i])
		valStart := i
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t') {
			i++
		}
		if i < len(tag) && tag[i] == '=' {
			i++
			for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t') {
				i++
			}
			if i < len(tag) && (tag[i] == '"' || tag[i] == '\'') {
				quote := tag[i]
				i++
				for i < len(tag) && tag[i] != quote {
					i++
				}
				if i < len(tag) {
					i++
				}
			} else {
				for i < len(tag) && tag[i] != ' ' && tag[i] != '\t' && tag[i] != '>' && tag[i] != '/' {
					i++
				}
			}
		}
		if strings.HasPrefix(attr, "on") {
			continue
		}
		out.WriteString(tag[attrStart:i])
		_ = valStart
	}
	if i < len(tag) {
		out.WriteString(tag[i:])
	}
	return out.String()
}

func formatPx(v float32) string {
	s := strconv.FormatFloat(float64(v), 'f', 2, 32)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

func writeTableHTML(out *strings.Builder, tb *TableData, image func(url string) string, r Renderers, numbers map[string]string, refs *map[string]int) {
	if tb == nil {
		return
	}
	out.WriteString("<table>\n")
	for ri, row := range tb.Rows {
		cell := "td"
		if tb.Header && ri == 0 {
			cell = "th"
			out.WriteString("<thead>")
		} else if ri == 0 || (tb.Header && ri == 1) {
			out.WriteString("<tbody>")
		}
		out.WriteString("<tr>")
		for ci, c := range row {
			align := ""
			if ci < len(tb.Aligns) {
				switch tb.Aligns[ci] {
				case AlignLeft:
					align = ` style="text-align:left"`
				case AlignCenter:
					align = ` style="text-align:center"`
				case AlignRight:
					align = ` style="text-align:right"`
				}
			}
			out.WriteString("<" + cell + align + ">")
			writeRunsNumbered(out, c.Runs, r, numbers, refs, image)
			out.WriteString("</" + cell + ">")
		}
		out.WriteString("</tr>")
		if tb.Header && ri == 0 {
			out.WriteString("</thead>")
		}
		out.WriteString("\n")
	}
	if len(tb.Rows) > 1 || !tb.Header {
		out.WriteString("</tbody>")
	}
	out.WriteString("</table>\n")
}
