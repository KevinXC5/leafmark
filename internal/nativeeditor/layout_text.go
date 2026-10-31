package nativeeditor

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/mathlayout"
	"leafmark/internal/richtext"
)

type styled struct {
	text                string
	bold, italic, code  bool
	strike              bool
	highlight, sub, sup bool
	underline, kbd      bool
	link                string
	image               string // 行内图片地址
	imageBlock          bool   // 独占源文本一行的图片，即使与图注同属一个段落也按正文宽度显示
	imageSize           string
	footnote            string // 脚注引用，text 是标签
	math                bool   // 行内公式，text 是 TeX 源码
	partial             bool   // 已被拆到下一行的剩余部分
}

// htmlMarks 取出只有 HTML 标签才能表达的行内样式：下划线与按键提示。
func htmlMarks(marks richtext.Mark) (underline, kbd bool) {
	return marks&richtext.MarkUnderline != 0, marks&richtext.MarkKbd != 0
}

func runsToStyled(runs []richtext.Run) []styled {
	out := make([]styled, 0, len(runs))
	for ri, r := range runs {
		if r.Text == "" {
			continue
		}
		s := styled{text: r.Text, bold: r.Marks&richtext.MarkBold != 0, italic: r.Marks&(richtext.MarkItalic|richtext.MarkMath) != 0, math: r.Marks&richtext.MarkMath != 0, code: r.Marks&richtext.MarkCode != 0, strike: r.Marks&richtext.MarkStrike != 0, highlight: r.Marks&richtext.MarkHighlight != 0, sub: r.Marks&richtext.MarkSub != 0, sup: r.Marks&richtext.MarkSup != 0}
		s.underline, s.kbd = htmlMarks(r.Marks)
		// 按键提示用等宽字体，外面再画一个键帽边框。
		s.code = s.code || s.kbd
		if r.Link != nil {
			s.link, s.footnote = r.Link.URL, r.Link.Footnote
		}
		if r.Image != nil {
			s.image = r.Image.URL
			s.imageSize = r.Image.Size
			var before, after strings.Builder
			for _, prev := range runs[:ri] {
				before.WriteString(prev.Text)
			}
			for _, next := range runs[ri+1:] {
				after.WriteString(next.Text)
			}
			left, right := before.String(), after.String()
			left = left[strings.LastIndex(left, "\n")+1:]
			if at := strings.IndexByte(right, '\n'); at >= 0 {
				right = right[:at]
			}
			s.imageBlock = strings.TrimSpace(left) == "" && strings.TrimSpace(right) == ""
			if r.Image.Alt != "" {
				s.text = r.Image.Alt
			}
		}
		out = append(out, s)
	}
	return out
}

// placeText 排一个文字块。ratio 大于 0 时覆盖正文的行高倍率，供引用与提示块容器使用。
func placeText(box *blockBox, b richtext.Block, inner, fontSize, ratio float32, cache *shapeCache, lay *layout) float32 {
	size := fontSize
	bold := false
	inset := float32(0)
	prefix := ""
	switch b.Kind {
	case richtext.Heading:
		size = headingSize(b.Level, fontSize)
		oldWeight := cache.weight
		cache.weight = 500
		defer func() { cache.weight = oldWeight }()
		bold = false
	case richtext.Quote:
		inset = quoteInset
	case richtext.Callout:
		inset = 37
		size = 13
		oldFamily := cache.family
		cache.family = calloutFont
		defer func() { cache.family = oldFamily }()
		if b.Callout != nil && b.Callout.Title != "" {
			box.title = ui.Shape(b.Callout.Title, ui.Font{Family: calloutFont, Size: 13})
			box.titleAscent = ui.Font{Family: calloutFont, Size: 13}.Metrics().Ascent
		}
	case richtext.List, richtext.Task:
		box.indent = float32(max(b.Level-1, 0)) * nestIndent
		if b.Kind == richtext.Task {
			box.checked = b.Checked
			inset = 28 + box.indent
		} else if b.Ordered {
			prefix = strconv.Itoa(max(1, b.Start)) + ". "
			// 多位编号也须完整留在正文左侧，命中起点与绘制保持一致。
			inset = box.indent + measure(cache, prefix, fontSize, false, false, false)
		} else {
			box.bullet = true
			inset = 12 + box.indent
		}
	}
	styledRuns := runsToStyled(b.Runs)
	if len(styledRuns) == 0 {
		styledRuns = []styled{{}}
	}
	width := inner - inset
	if b.Kind == richtext.Quote {
		width -= 16
	}
	if b.Kind == richtext.Callout {
		width -= 12
	}
	if width < 20 {
		width = 20
	}
	y := float32(0)
	if b.Kind == richtext.Quote {
		y = 10
	}
	if b.Kind == richtext.Callout {
		y = 12
		if len(box.title) > 0 && visibleRuns(b.Runs) != "" {
			y += 23
		}
	}
	runesBefore := 0
	// 按 run 软换行。前缀只出现在第一行。
	first := true
	var pending []styled
	pending = append(pending, styledRuns...)
	for len(pending) > 0 || first {
		ln := line{block: box.index, origin: box.origin + runesBefore, prefix: inset}
		x := inset
		if first && prefix != "" {
			g := cache.shape(prefix, fontSize, false, false, false)
			shiftGlyphs(g, box.indent)
			ln.markers = g
		}
		var built strings.Builder
		var mathUp, mathDown, imageHeight float32
		for len(pending) > 0 {
			seg := pending[0]
			rs := []rune(seg.text)
			if len(rs) == 0 {
				pending = pending[1:]
				continue
			}
			if seg.math && !seg.partial {
				// 光标不在公式内部时，把整条公式排成一个不可拆分的盒子。
				start := ln.origin + utf8.RuneCountInString(built.String())
				inside := cache.caret > start && cache.caret < start+len(rs)
				if mb := mathBox(cache, seg, size, inside); mb != nil {
					if x-inset+mb.Width > width && built.Len() > 0 {
						break
					}
					g := []ui.Glyph{{Cluster: utf8.RuneCountInString(built.String()), Runes: len(rs), X: x, Advance: mb.Width}}
					ln.spans = append(ln.spans, paintSpan{glyphs: g, math: mb, highlight: seg.highlight, strike: seg.strike, link: seg.link})
					ln.glyphs = append(ln.glyphs, g...)
					mathUp, mathDown = max(mathUp, mb.Ascent), max(mathDown, mb.Descent)
					x += mb.Width
					built.WriteString(seg.text)
					pending = pending[1:]
					continue
				}
			}
			if seg.image != "" && !seg.partial {
				alt := seg.text
				if alt == "" || alt == "\ufffc" {
					alt = "图片"
				}
				g := cache.shape(alt, size*.9, false, false, false)
				w := advanceOf(g) + 8
				if bm := cache.images[seg.image]; bm != nil {
					pw, ph := bm.Size()
					h := size * 1.15
					if ph > 0 {
						w = h * float32(pw) / float32(ph)
					}
					if w > 160 {
						w = 160
					}
					if w < 12 {
						w = 12
					}
					if seg.imageBlock && pw > 0 && ph > 0 {
						w = width
						if want := float32(richtext.ImageWidth(seg.imageSize)); want > 0 {
							w = min(w, want)
						}
						imageHeight = w*float32(ph)/float32(pw) + 2
					}
					g = nil
				}
				if x-inset+w > width && built.Len() > 0 {
					break
				}
				shiftGlyphs(g, x+4)
				hold := []ui.Glyph{{Cluster: utf8.RuneCountInString(built.String()), Runes: len(rs), X: x, Advance: w}}
				ln.spans = append(ln.spans, paintSpan{glyphs: hold, image: seg.image, display: g})
				ln.glyphs = append(ln.glyphs, hold...)
				x += w
				built.WriteString(seg.text)
				pending = pending[1:]
				continue
			}
			if seg.footnote != "" && !seg.partial {
				// 脚注引用显示成上标编号，整个标签是一个不可拆分的盒子。
				number := cache.shape(strconv.Itoa(max(1, cache.footnotes[seg.footnote])), size*.75, false, false, false)
				w := advanceOf(number) + 2
				if x-inset+w > width && built.Len() > 0 {
					break
				}
				shiftGlyphs(number, 1)
				g := []ui.Glyph{{Cluster: utf8.RuneCountInString(built.String()), Runes: len(rs), X: x, Advance: w}}
				ln.spans = append(ln.spans, paintSpan{glyphs: g, dy: -size * .35, footnote: seg.footnote, display: number})
				ln.glyphs = append(ln.glyphs, g...)
				x += w
				built.WriteString(seg.text)
				pending = pending[1:]
				continue
			}
			hard := -1
			for i, r := range rs {
				if r == '\n' {
					hard = i
					break
				}
			}
			candidate := rs
			if hard >= 0 {
				candidate = rs[:hard]
			}
			fit := fitRunes(cache, seg, candidate, width-(x-inset), size, bold)
			if hard == 0 {
				pending[0].text = string(rs[1:])
				ln.newline = true
				break
			}
			if fit == 0 {
				if built.Len() == 0 {
					fit = clusterBounds(rs)[1] // 再窄也至少放一个完整簇
					if len(rs) == 0 {
						fit = 0
					}
				} else {
					break
				}
			}
			piece := string(rs[:fit])
			segSize := styledSize(seg, size)
			g := cache.shape(piece, segSize, bold || seg.bold, seg.italic, seg.code)
			shiftGlyphs(g, x)
			// cluster 是 piece 内的 rune 下标，改成相对整行文本。
			base := utf8.RuneCountInString(built.String())
			// Shape 的 Cluster 已是 piece 内 rune 下标。加上本行已有 rune，供命中使用。
			for i := range g {
				g[i].Cluster += base
			}
			dy := float32(0)
			if seg.sub {
				dy = size * .2
			}
			if seg.sup {
				dy = -size * .35
			}
			ln.spans = append(ln.spans, paintSpan{glyphs: g, dy: dy, highlight: seg.highlight, code: seg.code, strike: seg.strike, underline: seg.underline, kbd: seg.kbd, link: seg.link})
			ln.glyphs = append(ln.glyphs, g...)
			x = advanceOf(g)
			built.WriteString(piece)
			if hard >= 0 && fit == hard {
				pending[0].text = string(rs[fit+1:])
				pending[0].partial = true
				ln.newline = true
				break
			}
			if fit < len(rs) {
				pending[0].text = string(rs[fit:])
				pending[0].partial = true
				break
			}
			pending = pending[1:]
		}
		ln.text = built.String()
		ln.count = len([]rune(ln.text))
		m := cache.metrics(size, bold, false)
		lineRatio := cache.lineHeight
		if ratio > 0 {
			lineRatio = ratio
		}
		if b.Kind == richtext.Heading {
			lineRatio = 1.4
			if b.Level == 1 {
				lineRatio = 1.3
			}
		}
		if b.Kind == richtext.Quote {
			lineRatio = 1.7
		}
		if b.Kind == richtext.Callout {
			lineRatio = 1.5
		}
		ln.height = max(size*lineRatio, m.Ascent+m.Descent)
		ln.ascent = m.Ascent + (ln.height-m.Ascent-m.Descent)/2
		// 分式、求和这类高出一行的行内公式把所在行撑开，基线仍与文字对齐。
		if above, below := max(ln.ascent, mathUp+2), max(ln.height-ln.ascent, mathDown+2); above+below > ln.height {
			ln.ascent, ln.height = above, above+below
		}
		ln.height = max(ln.height, imageHeight)
		ln.y = box.y + y
		ln.width = x
		// 行尾若还有剩余，这一行是软换行，不占文档 rune。

		lay.lines = append(lay.lines, ln)
		box.lines = append(box.lines, len(lay.lines)-1)
		y += ln.height
		runesBefore += ln.count
		if ln.newline {
			runesBefore++
		}
		first = false
		if len(pending) == 0 {
			break
		}
	}
	if b.Kind == richtext.Quote {
		y += 10
	}
	if b.Kind == richtext.Callout {
		y += 12
	}
	return y
}

// mathBox 返回行内公式的排版结果；光标在公式内部或源码不受支持时返回 nil。
func mathBox(cache *shapeCache, seg styled, size float32, inside bool) *mathlayout.Box {
	if inside {
		return nil
	}
	return cache.math(seg.text, size*1.1, false)
}

func styledSize(seg styled, size float32) float32 {
	if seg.code {
		return size * .78
	}
	if seg.sub || seg.sup {
		return size * .75
	}
	return size
}

func fitRunes(cache *shapeCache, seg styled, rs []rune, width, size float32, headingBold bool) int {
	if width <= 0 {
		return 0
	}
	// 整段放得下时不必二分：多数行都走这里，也不会往字形缓存里塞一串前缀。
	if measure(cache, string(rs), styledSize(seg, size), headingBold || seg.bold, seg.italic, seg.code) <= width {
		return len(rs)
	}
	// 二分找到放得下的最长前缀，并尽量在空白处断开。
	lo, hi := 0, len(rs)-1
	best := 0
	for lo <= hi {
		mid := (lo + hi) / 2
		if mid == 0 {
			lo = 1
			continue
		}
		w := measure(cache, string(rs[:mid]), styledSize(seg, size), headingBold || seg.bold, seg.italic, seg.code)
		if w <= width {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	bounds := clusterBounds(rs)
	for i := len(bounds) - 1; i >= 0; i-- {
		if bounds[i] <= best {
			best = bounds[i]
			break
		}
	}
	if best == 0 || best == len(rs) {
		return best
	}
	for i := best; i > best/2 && i > 0; i-- {
		if unicode.IsSpace(rs[i-1]) {
			return i
		}
	}
	return best
}
