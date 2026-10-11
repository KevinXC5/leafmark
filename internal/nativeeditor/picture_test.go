package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

// drawioSVG 仿照 draw.io 的导出：形状用 light-dark() 配色，文字是 foreignObject 加位图替身。
func drawioSVG(t *testing.T) []byte {
	t.Helper()
	label := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for i := 0; i < len(label.Pix); i += 4 {
		label.Pix[i], label.Pix[i+1], label.Pix[i+2], label.Pix[i+3] = 200, 0, 0, 255
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, label); err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString(raw.Bytes())
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="200px" height="100px" viewBox="0 0 100 50">
<defs><image x="0" y="0" width="5" height="5" xlink:href="data:image/png;base64,` + data + `"/></defs>
<g transform="translate(10,5)">
<rect x="0" y="0" width="80" height="40" fill="#000000" style="fill: light-dark(rgb(0, 0, 255), rgb(20, 33, 42));"/>
<g transform="scale(2)"><switch>
<foreignObject requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility" width="100%" height="100%"><div xmlns="http://www.w3.org/1999/xhtml">文字</div></foreignObject>
<image x="5" y="5" width="20" height="10" xlink:href="data:image/png;base64,` + data + `"/>
<image x="0" y="0" width="1" height="1" xlink:href="data:image/png;base64,` + data + `"/>
</switch></g>
<g transform="rotate(90)"><image x="0" y="0" width="5" height="5" xlink:href="data:image/png;base64,` + data + `"/></g>
</g></svg>`)
}

func TestDecodeSVGOverlays(t *testing.T) {
	pic := DecodeSVG(drawioSVG(t))
	if pic == nil {
		t.Fatal("SVG 应能解析")
	}
	if w, h := pic.Size(); w != 200 || h != 100 {
		t.Fatalf("尺寸应取声明的宽高：%dx%d", w, h)
	}
	// defs 里的、switch 里排在后面的和旋转过的位图都不画；坐标经 viewBox 换到声明的宽高上。
	if len(pic.overlays) != 1 {
		t.Fatalf("应只有一张文字替身：%d", len(pic.overlays))
	}
	if o := pic.overlays[0]; o.x != 40 || o.y != 30 || o.w != 80 || o.h != 40 {
		t.Fatalf("替身位置不对：%+v", o)
	}
	if DecodeSVG([]byte("<svg")) != nil || DecodeSVG(nil) != nil {
		t.Fatal("无效内容不应解析成图片")
	}
	if got := string(lightScheme([]byte(`a: light-dark(rgb(1, 2, 3), var(--x, #000)); b: light-dark(#fff, #000)" c="light-dark(`))); got != `a: rgb(1, 2, 3); b: #fff" c="light-dark(` {
		t.Fatalf("light-dark 应换成浅色值：%q", got)
	}
}

func TestPaintSVGPicture(t *testing.T) {
	pic := DecodeSVG(drawioSVG(t))
	render := func(dark bool) *image.RGBA {
		view := func(c *ui.Context) {
			ui.Box(c).Size(240, 140).Background(ui.RGB(30, 30, 30)).Draw(func(p *ui.Painter, r ui.Rect) {
				pic.paint(p, ui.Rect{X: r.X + 20, Y: r.Y + 20, W: 200, H: 100}, dark)
			})
		}
		tt := ui.NewTester(view, 240, 140)
		tt.Frame()
		return tt.Image()
	}
	near := func(got color.RGBA, r, g, b uint8) bool {
		d := func(a, b uint8) int { return max(int(a), int(b)) - min(int(a), int(b)) }
		return d(got.R, r) < 24 && d(got.G, g) < 24 && d(got.B, b) < 24
	}
	img := render(false)
	if got := img.RGBAAt(20+30, 20+50); !near(got, 0, 0, 255) {
		t.Fatalf("形状应按浅色配色绘制：%v", got)
	}
	if got := img.RGBAAt(20+80, 20+50); !near(got, 200, 0, 0) {
		t.Fatalf("文字替身应叠在形状上：%v", got)
	}
	if got := img.RGBAAt(20+5, 20+5); !near(got, 30, 30, 30) {
		t.Fatalf("浅色主题下透明处应露出背景：%v", got)
	}
	if got := render(true).RGBAAt(20+5, 20+50); !near(got, 250, 249, 245) {
		t.Fatalf("深色主题下应垫一层纸色：%v", got)
	}
}

func TestWikiImageUsesAuthorizedReader(t *testing.T) {
	var asked []string
	ed := New("![[图 1.drawio.svg]]\n\n文字 ![[a.png|40]] 文字\n")
	svg := drawioSVG(t)
	ed.SetReadPicture(func(path string) *Picture {
		asked = append(asked, path)
		if path == "%E5%9B%BE%201.drawio.svg" {
			return DecodeSVG(svg)
		}
		return nil
	})
	tt := ui.NewTester(func(c *ui.Context) { ed.View(c) }, 800, 600)
	tt.Frame()
	if len(asked) != 2 || asked[0] != "%E5%9B%BE%201.drawio.svg" || asked[1] != "a.png" {
		t.Fatalf("嵌入图片应按转义后的相对地址交给授权回调：%v", asked)
	}
	if pic := ed.images["%E5%9B%BE%201.drawio.svg"]; pic == nil || pic.vector == nil {
		t.Fatal("矢量图应进入图片缓存")
	}
	if ed.Markdown() != "![[图 1.drawio.svg]]\n\n文字 ![[a.png|40]] 文字\n" {
		t.Fatalf("原文不应变化：%q", ed.Markdown())
	}
}
