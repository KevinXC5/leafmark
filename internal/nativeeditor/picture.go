package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Picture 是编辑器里一张可绘制的图片。位图直接绘制；矢量图先画形状，
// 再叠加原生排版的文字和 SVG 内嵌的位图。
type Picture struct {
	bitmap   *ui.Bitmap
	vector   *ui.SVG
	w, h     float32
	overlays []pictureOverlay
	texts    []svgText
}

// pictureOverlay 是矢量图里的一张内嵌位图，坐标以图片自身的宽高为单位。
type pictureOverlay struct {
	bitmap     *ui.Bitmap
	x, y, w, h float32
}

const maxSVGBytes = 8 << 20
const maxSVGOverlays = 512

// vectorPaper 是深色主题下垫在矢量图后面的纸色。矢量图通常按浅色背景配色，内嵌的文字位图也是深色字。
var vectorPaper = ui.RGB(250, 249, 245)

// BitmapPicture 把位图包成 Picture；位图为空时返回 nil。
func BitmapPicture(bm *ui.Bitmap) *Picture {
	if bm == nil {
		return nil
	}
	w, h := bm.Size()
	return &Picture{bitmap: bm, w: float32(w), h: float32(h)}
}

// DecodeSVG 解析 SVG 的形状、常用文字及 draw.io 内嵌的位图替身。
func DecodeSVG(raw []byte) *Picture {
	if len(raw) == 0 || len(raw) > maxSVGBytes {
		return nil
	}
	vector, err := ui.ParseSVG(lightScheme(raw))
	if err != nil {
		return nil
	}
	w, h := vector.Size()
	if w <= 0 || h <= 0 || float64(w)*float64(h) > maxImagePixels {
		return nil
	}
	return &Picture{vector: vector, w: w, h: h, overlays: svgOverlays(raw, w, h), texts: svgTexts(lightScheme(raw), w, h)}
}

// Size 返回图片的像素尺寸，矢量图取其声明的宽高。
func (pic *Picture) Size() (w, h int) {
	if pic == nil {
		return 0, 0
	}
	return int(math.Round(float64(pic.w))), int(math.Round(float64(pic.h)))
}

// paint 把图片等比放进 r 并居中。
func (pic *Picture) paint(p *ui.Painter, r ui.Rect, dark bool) {
	if pic == nil || pic.w <= 0 || pic.h <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	if pic.bitmap != nil {
		p.Image(pic.bitmap, r, ui.Contain)
		return
	}
	k := min(r.W/pic.w, r.H/pic.h)
	fit := ui.Rect{X: r.X + (r.W-pic.w*k)/2, Y: r.Y + (r.H-pic.h*k)/2, W: pic.w * k, H: pic.h * k}
	if dark {
		p.Fill(fit, vectorPaper, min(6, fit.H/4))
	}
	p.Image(pic.vector, fit, ui.FillBox)
	for i := range pic.texts {
		pic.texts[i].paint(p, fit, k)
	}
	for _, o := range pic.overlays {
		p.Image(o.bitmap, ui.Rect{X: fit.X + o.x*k, Y: fit.Y + o.y*k, W: o.w * k, H: o.h * k}, ui.FillBox)
	}
}

func pictureAllowed(pic *Picture) bool {
	if pic == nil || pic.bitmap == nil && pic.vector == nil {
		return false
	}
	return pic.w > 0 && pic.h > 0 && float64(pic.w)*float64(pic.h) <= maxImagePixels
}

// lightScheme 把 CSS 的 light-dark(浅, 深) 换成浅色值，形状与内嵌的文字位图保持同一套配色。
func lightScheme(raw []byte) []byte {
	const name = "light-dark("
	if !bytes.Contains(raw, []byte(name)) {
		return raw
	}
	var out bytes.Buffer
	out.Grow(len(raw))
	for {
		i := bytes.Index(raw, []byte(name))
		if i < 0 {
			break
		}
		depth, comma, end := 1, -1, -1
		for j := i + len(name); j < len(raw) && end < 0; j++ {
			switch raw[j] {
			case '(':
				depth++
			case ')':
				if depth--; depth == 0 {
					end = j
				}
			case ',':
				if depth == 1 && comma < 0 {
					comma = j
				}
			case '"', '<', '>':
				// 属性或标签在函数闭合前结束，按原文保留。
				j = len(raw)
			}
		}
		if end < 0 || comma < 0 {
			out.Write(raw[:i+len(name)])
			raw = raw[i+len(name):]
			continue
		}
		out.Write(raw[:i])
		out.Write(bytes.TrimSpace(raw[i+len(name) : comma]))
		raw = raw[end+1:]
	}
	out.Write(raw)
	return out.Bytes()
}

// affine 是二维仿射矩阵 [a c e; b d f]。
type affine struct{ a, b, c, d, e, f float64 }

var identityAffine = affine{a: 1, d: 1}

func (m affine) mul(n affine) affine {
	return affine{
		a: m.a*n.a + m.c*n.b, b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d, d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e, f: m.b*n.e + m.d*n.f + m.f,
	}
}

// svgOverlays 收集 SVG 里以 data URI 内嵌的位图。switch 只取第一个可用的子元素：
// foreignObject 无法原生绘制，跳过它之后轮到的位图就是文字的替身。
func svgOverlays(raw []byte, width, height float32) []pictureOverlay {
	type frame struct {
		m      affine
		choice bool // 是否 switch
		chosen bool
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	dec.CharsetReader = nil
	stack := []frame{{m: identityAffine}}
	var out []pictureOverlay
	budget := maxSVGBytes
	root := true
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			top := &stack[len(stack)-1]
			name := t.Name.Local
			attrs := map[string]string{}
			for _, a := range t.Attr {
				attrs[a.Name.Local] = a.Value
			}
			skip := false
			switch name {
			case "defs", "foreignObject", "pattern", "mask", "clipPath", "symbol", "marker", "style", "script", "title", "desc", "metadata":
				skip = true
			}
			if top.choice && !skip {
				if top.chosen || attrs["requiredExtensions"] != "" {
					skip = true
				} else {
					top.chosen = true
				}
			}
			if skip {
				if dec.Skip() != nil {
					return out
				}
				continue
			}
			m := top.m
			if root {
				root = false
				m = viewBoxAffine(attrs["viewBox"], float64(width), float64(height))
			} else if v := attrs["transform"]; v != "" {
				m = m.mul(parseAffine(v))
			}
			if name == "image" && len(out) < maxSVGOverlays {
				if o, used, ok := overlayOf(attrs, m, budget); ok {
					budget -= used
					out = append(out, o)
				}
			}
			stack = append(stack, frame{m: m, choice: name == "switch"})
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func overlayOf(attrs map[string]string, m affine, budget int) (pictureOverlay, int, bool) {
	// 旋转或倾斜的位图不画，原生绘制只支持轴对齐的矩形。
	if math.Abs(m.b) > 1e-6 || math.Abs(m.c) > 1e-6 || m.a <= 0 || m.d <= 0 {
		return pictureOverlay{}, 0, false
	}
	head, body, ok := strings.Cut(strings.TrimSpace(attrs["href"]), ",")
	if !ok || !strings.HasPrefix(strings.ToLower(head), "data:image/") || !strings.HasSuffix(strings.ToLower(head), ";base64") {
		return pictureOverlay{}, 0, false
	}
	if base64.StdEncoding.DecodedLen(len(body)) > budget {
		return pictureOverlay{}, 0, false
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body), ""))
	if err != nil {
		return pictureOverlay{}, 0, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return pictureOverlay{}, 0, false
	}
	x, y := svgNumber(attrs["x"]), svgNumber(attrs["y"])
	w, h := svgNumber(attrs["width"]), svgNumber(attrs["height"])
	if w <= 0 || h <= 0 {
		return pictureOverlay{}, 0, false
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		return pictureOverlay{}, 0, false
	}
	return pictureOverlay{
		bitmap: bm,
		x:      float32(m.a*x + m.e), y: float32(m.d*y + m.f),
		w: float32(m.a * w), h: float32(m.d * h),
	}, len(data), true
}

// viewBoxAffine 把 viewBox 里的坐标换到图片声明的宽高上。
func viewBoxAffine(viewBox string, width, height float64) affine {
	v := svgNumbers(viewBox)
	if len(v) != 4 || v[2] <= 0 || v[3] <= 0 {
		return identityAffine
	}
	sx, sy := width/v[2], height/v[3]
	return affine{a: sx, d: sy, e: -v[0] * sx, f: -v[1] * sy}
}

// parseAffine 解析 transform 属性里依次书写的变换。
func parseAffine(s string) affine {
	m := identityAffine
	for {
		open := strings.IndexByte(s, '(')
		if open < 0 {
			return m
		}
		end := strings.IndexByte(s[open:], ')')
		if end < 0 {
			return m
		}
		name := strings.Trim(s[:open], " \t\r\n,")
		v := svgNumbers(s[open+1 : open+end])
		s = s[open+end+1:]
		arg := func(i int, def float64) float64 {
			if i < len(v) {
				return v[i]
			}
			return def
		}
		switch name {
		case "translate":
			m = m.mul(affine{a: 1, d: 1, e: arg(0, 0), f: arg(1, 0)})
		case "scale":
			sx := arg(0, 1)
			m = m.mul(affine{a: sx, d: arg(1, sx)})
		case "matrix":
			if len(v) == 6 {
				m = m.mul(affine{a: v[0], b: v[1], c: v[2], d: v[3], e: v[4], f: v[5]})
			}
		case "rotate":
			sin, cos := math.Sincos(arg(0, 0) * math.Pi / 180)
			cx, cy := arg(1, 0), arg(2, 0)
			m = m.mul(affine{a: 1, d: 1, e: cx, f: cy}).mul(affine{a: cos, b: sin, c: -sin, d: cos}).mul(affine{a: 1, d: 1, e: -cx, f: -cy})
		case "skewX":
			m = m.mul(affine{a: 1, d: 1, c: math.Tan(arg(0, 0) * math.Pi / 180)})
		case "skewY":
			m = m.mul(affine{a: 1, d: 1, b: math.Tan(arg(0, 0) * math.Pi / 180)})
		}
	}
}

func svgNumbers(s string) []float64 {
	var out []float64
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

func svgNumber(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 64)
	if err != nil {
		return 0
	}
	return v
}
