package nativeeditor

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

func placeCode(box *blockBox, code, lang string, fontSize float32, cache *shapeCache, lay *layout, source bool) float32 {
	size := float32(14)
	if source {
		size = fontSize
	}
	m := cache.metrics(size, false, true)
	lineH := max(size*1.75, m.Ascent+m.Descent)
	if source && cache.lineHeight > 0 {
		lineH = max(size*cache.lineHeight, m.Ascent+m.Descent)
	}
	parts := strings.Split(code, "\n")
	// 代码正文以换行结尾时，末尾的空行不单独占一行高度。源码模式保留它，光标才能停在文末换行之后。
	if !source && len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	y := float32(14)
	pad := float32(codePad)
	wrap := float32(0)
	if source {
		// 源码是整篇正文，不留代码块的内边距，超宽的行按栏宽折行。
		y, pad = 0, 8
		wrap = box.w - pad*2
		if wrap < 40 {
			wrap = 40
		}
	}
	runesBefore := 0
	for _, part := range parts {
		pieces := []string{part}
		if source {
			pieces = wrapSource(cache, part, wrap, size)
		}
		for pi, piece := range pieces {
			ln := line{block: box.index, y: box.y + y, ascent: m.Ascent + (lineH-m.Ascent-m.Descent)/2, height: lineH, prefix: pad, text: piece, origin: box.origin + runesBefore, count: len([]rune(piece)), newline: pi == len(pieces)-1}
			g := cache.shape(piece, size, false, false, true)
			shiftGlyphs(g, pad)
			ln.glyphs = g
			if source {
				ln.spans = sourceSpans(g, ln.origin, cache.sourceKinds)
			} else {
				ln.spans = codeSpans(g, piece, lang)
			}
			ln.width = advanceOf(g)
			lay.lines = append(lay.lines, ln)
			box.lines = append(box.lines, len(lay.lines)-1)
			y += lineH
			runesBefore += ln.count
			if pi == len(pieces)-1 {
				runesBefore++
			}
		}
	}
	if n := len(lay.lines); n > 0 && !strings.HasSuffix(code, "\n") {
		lay.lines[n-1].newline = false
	}
	if source {
		return y + 8
	}
	return y + 14
}

// wrapSource 把一行源码按栏宽折成多段，不拆 rune 簇。空行保留成一段。
func wrapSource(cache *shapeCache, part string, width, size float32) []string {
	rs := []rune(part)
	if len(rs) == 0 {
		return []string{""}
	}
	var out []string
	start := 0
	for start < len(rs) {
		rest := rs[start:]
		fit := fitRunes(cache, styled{text: string(rest), code: true}, rest, width, size, false)
		if fit <= 0 || fit > len(rest) {
			fit = 1
		}
		out = append(out, string(rest[:fit]))
		start += fit
	}
	return out
}

// sourceKindTable 把着色区间展开成每个 rune 的种类。区间重叠时先出现的生效，与逐个区间查找一致。
func sourceKindTable(spans []SourceSpan, length int) []uint8 {
	if len(spans) == 0 || length <= 0 {
		return nil
	}
	kinds := make([]uint8, length)
	for i := len(spans) - 1; i >= 0; i-- {
		sp := spans[i]
		for at := max(sp.Start, 0); at < min(sp.End, length); at++ {
			kinds[at] = sp.Kind
		}
	}
	return kinds
}

// sourceSpans 按着色表给一行源码着色。origin 是这一行在全文里的 rune 偏移，种类为 0 的不单独成段。
func sourceSpans(glyphs []ui.Glyph, origin int, kinds []uint8) []paintSpan {
	if len(kinds) == 0 || len(glyphs) == 0 {
		return nil
	}
	var out []paintSpan
	for _, g := range glyphs {
		kind := uint8(0)
		if at := origin + g.Cluster; at >= 0 && at < len(kinds) {
			kind = kinds[at]
		}
		if n := len(out); n > 0 && out[n-1].source == kind {
			out[n-1].glyphs = append(out[n-1].glyphs, g)
			continue
		}
		out = append(out, paintSpan{glyphs: []ui.Glyph{g}, source: kind})
	}
	return out
}

// codeSpans 给一行代码做最小着色：字符串字面量用强调色，行注释用次要色。
func codeSpans(glyphs []ui.Glyph, text, lang string) []paintSpan {
	rs := []rune(text)
	inks := make([]inkKind, len(rs)+1)
	hash := false
	switch strings.ToLower(lang) {
	case "py", "python", "sh", "bash", "zsh", "shell", "yaml", "yml", "toml", "ruby", "rb", "r", "perl", "dockerfile", "makefile":
		hash = true
	}
	colored := false
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '"' || r == '\'' || r == '`':
			end := i + 1
			for end < len(rs) && rs[end] != r {
				if rs[end] == '\\' && r != '`' {
					end++
				}
				end++
			}
			if end >= len(rs) {
				continue // 未闭合的引号不着色
			}
			for j := i; j <= end; j++ {
				inks[j] = inkAccent
			}
			colored = true
			i = end
		case (r == '/' && i+1 < len(rs) && rs[i+1] == '/' && (i == 0 || rs[i-1] != ':')) || (hash && r == '#'):
			for j := i; j < len(rs); j++ {
				inks[j] = inkMuted
			}
			colored = true
			i = len(rs)
		}
	}
	if !colored {
		return nil
	}
	var spans []paintSpan
	for _, g := range glyphs {
		ink := inks[max(0, min(g.Cluster, len(rs)))]
		if n := len(spans); n > 0 && spans[n-1].ink == ink {
			spans[n-1].glyphs = append(spans[n-1].glyphs, g)
			continue
		}
		spans = append(spans, paintSpan{glyphs: []ui.Glyph{g}, ink: ink})
	}
	return spans
}

// fitTableWidths 保留能完整显示的短列，只有超出公平份额的长列需要换行。
func fitTableWidths(natural []float32, available float32) []float32 {
	widths := append([]float32(nil), natural...)
	total := float32(0)
	for _, w := range widths {
		total += w
	}
	if total <= available {
		for i := range widths {
			widths[i] *= available / total
		}
		return widths
	}
	remaining, count := available, len(widths)
	fixed := make([]bool, count)
	for count > 0 {
		share := remaining / float32(count)
		changed := false
		for i, w := range widths {
			if !fixed[i] && w <= share {
				fixed[i] = true
				remaining -= w
				count--
				changed = true
			}
		}
		if !changed {
			for i := range widths {
				if !fixed[i] {
					widths[i] = share
				}
			}
			break
		}
	}
	return widths
}

func placeTable(box *blockBox, table *richtext.TableData, fontSize float32, cache *shapeCache, lay *layout) float32 {
	if table == nil || len(table.Rows) == 0 {
		return fontSize + 8
	}
	cols := 0
	for _, row := range table.Rows {
		cols = max(cols, len(row))
	}
	if cols == 0 {
		return fontSize + 8
	}
	// 先满足短列的自然宽度，再由长列分摊剩余空间，避免说明列挤碎短表头。
	widths := make([]float32, cols)
	for c := 0; c < cols; c++ {
		widths[c] = 48
		for ri, row := range table.Rows {
			if c >= len(row) {
				continue
			}
			oldWeight := cache.weight
			if table.Header && ri == 0 {
				cache.weight = 600
			}
			var natural float32
			for _, seg := range runsToStyled(row[c].Runs) {
				natural += measure(cache, seg.text, styledSize(seg, fontSize), seg.bold, seg.italic, seg.code)
			}
			cache.weight = oldWeight
			widths[c] = max(widths[c], natural+cellPad*2+1)
		}
	}
	widths = fitTableWidths(widths, box.w)
	box.cols = append(box.cols, 0)
	for _, w := range widths {
		box.cols = append(box.cols, box.cols[len(box.cols)-1]+w)
	}
	y := float32(0)
	runesBefore := 0
	box.rows = append(box.rows, 0)
	for ri, row := range table.Rows {
		if ri > 0 {
			runesBefore++
		}
		rowH := fontSize*cache.lineHeight + 14.4
		for ci := 0; ci < cols; ci++ {
			if ci > 0 && ci < len(row) {
				runesBefore++
			}
			var runs []richtext.Run
			if ci < len(row) {
				runs = row[ci].Runs
			}
			cell := blockBox{index: box.index, x: box.x, y: box.y + y + 7.2, w: widths[ci] - cellPad*2, origin: box.origin + runesBefore}
			oldWeight := cache.weight
			if table.Header && ri == 0 {
				cache.weight = 600
			}
			h := placeText(&cell, richtext.Block{Kind: richtext.Paragraph, Runs: runs}, cell.w, fontSize, 0, cache, lay)
			cache.weight = oldWeight
			rowH = max(rowH, h+14.4)
			for _, idx := range cell.lines {
				ln := &lay.lines[idx]
				dx := box.cols[ci] + cellPad
				if ci < len(table.Aligns) {
					switch table.Aligns[ci] {
					case richtext.AlignCenter:
						dx += (cell.w - advanceEnd(*ln)) / 2
					case richtext.AlignRight:
						dx += cell.w - advanceEnd(*ln)
					}
				}
				ln.prefix = dx
				shiftGlyphs(ln.glyphs, dx)
				for _, span := range ln.spans {
					shiftGlyphs(span.glyphs, dx)
				}
				ln.width = advanceOf(ln.glyphs)
				box.lines = append(box.lines, idx)
			}
			runesBefore += len([]rune(visibleRuns(runs)))
		}
		y += rowH
		box.rows = append(box.rows, y)
	}
	return y
}

// 原文仍由文档模型保留，类型提示不假装已经渲染复杂语法。
func rawContentLabel(source string) string {
	s := strings.TrimSpace(source)
	switch {
	case strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n"):
		return "文档属性 · 原文保留"
	case strings.Contains(s, "```mermaid"):
		return "Mermaid 图表 · 原文保留"
	case strings.HasPrefix(s, "$$") || strings.Contains(s, "$"):
		return "数学公式 · 原文保留"
	case strings.Contains(s, "[!"):
		return "提示块 · 原文保留"
	case strings.HasPrefix(s, "<"):
		return "HTML 内容 · 原文保留"
	case strings.HasPrefix(s, "[^") || strings.Contains(s, "[^"):
		return "脚注 · 原文保留"
	case strings.HasPrefix(s, "[") && strings.Contains(s, "]:"):
		return "链接定义 · 原文保留"
	case strings.HasPrefix(s, "-") || strings.HasPrefix(s, "*"):
		return "嵌套列表 · 原文保留"
	default:
		return "扩展内容 · 原文保留"
	}
}

func placePlaceholder(box *blockBox, b richtext.Block, fontSize float32, cache *shapeCache, lay *layout) float32 {
	kind := b.Kind
	text := rawContentLabel(b.Raw)
	if kind == richtext.Horizontal {
		text = ""
	}
	m := cache.metrics(fontSize*0.9, false, false)
	h := m.Ascent + m.Descent + 16
	if kind == richtext.Horizontal {
		h = 18
	}
	ln := line{block: box.index, y: box.y + 8, ascent: m.Ascent, height: h, text: text, origin: box.origin, count: 0, prefix: 12}
	if text != "" {
		ln.glyphs = cache.shape(text, fontSize*0.9, false, false, false)
		shiftGlyphs(ln.glyphs, 12)
		ln.width = advanceOf(ln.glyphs)
		box.w = min(box.w, ln.width+12)
	}
	lay.lines = append(lay.lines, ln)
	box.lines = append(box.lines, len(lay.lines)-1)
	return h
}

func placeImage(box *blockBox, b richtext.Block, images map[string]*Picture, fontSize float32, cache *shapeCache) float32 {
	box.alt = b.Alt
	h := float32(72)
	if bm := images[b.URL]; bm != nil {
		w, ih := bm.Size()
		box.image = bm
		if w > 0 {
			shown := box.w
			// 嵌入语法指定了宽度时按该宽度显示，不超过正文宽度。
			if want := float32(richtext.ImageWidth(b.Size)); want > 0 && want < shown {
				shown = want
			}
			scale := shown / float32(w)
			h = float32(ih) * scale
			// 默认铺满正文宽度，高度只由原图比例决定，避免高图被整体缩小。
		}
	}
	if b.Alt != "" {
		h += fontSize + 6
		_ = cache
	}
	return h + 8
}
