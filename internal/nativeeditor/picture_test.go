package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"leafmark/internal/diagram"
	"leafmark/internal/mathlayout"
	"leafmark/internal/richtext"
	"math"
	"sync"
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

func TestBlockImageFillsContentWidth(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		width, height, content float32
		size                   string
		wantHeight             float32
	}{
		{"横图", 800, 600, 720, "", 540},
		{"长图", 400, 1600, 720, "", 2880},
		{"小图放大", 100, 50, 720, "", 360},
		{"窄正文", 800, 600, 360, "", 270},
		{"指定宽度", 800, 600, 720, "240", 180},
		{"指定宽度不超正文", 800, 600, 360, "900", 270},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := blockBox{w: tc.content}
			h := placeImage(&box, richtext.Block{URL: "图", Size: tc.size}, map[string]*Picture{"图": {w: tc.width, h: tc.height}}, 16, nil)
			if h != tc.wantHeight+8 {
				t.Fatalf("图片应按正文或指定宽度等比排版，高度得到 %v，期望 %v", h, tc.wantHeight+8)
			}
		})
	}
}

func TestDecodeSVGLightSchemeAndPaintGuards(t *testing.T) {
	raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100" viewBox="0 0 200 100">
<rect width="200" height="100" style="fill: light-dark(rgb(10, 20, 30), rgb(240, 240, 240));"/>
<text x="20" y="50" style="fill: light-dark(#112233, #eeeeee); font-size: 20px;">浅色文字</text>
</svg>`)
	pic := DecodeSVG(raw)
	if pic == nil || len(pic.texts) != 1 {
		t.Fatalf("含 light-dark() 的 SVG 应正常解析：%+v", pic)
	}
	if pic.texts[0].color != ui.RGB(0x11, 0x22, 0x33) {
		t.Fatalf("文字颜色应采用 light-dark() 的浅色分支：%v", pic.texts[0].color)
	}

	// 空指针、空矢量、非正或 NaN/Inf 目标矩形都不应 panic。
	var nilPic *Picture
	nilPic.paint(nil, ui.Rect{W: 100, H: 100}, false)
	pic.paint(nil, ui.Rect{W: 100, H: 100}, false)
	empty := &Picture{w: 100, h: 50}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(100, 50).Draw(func(p *ui.Painter, r ui.Rect) {
			empty.paint(p, r, true)
			pic.paint(p, ui.Rect{W: 0, H: 50}, false)
			pic.paint(p, ui.Rect{W: float32(math.NaN()), H: 50}, false)
			pic.paint(p, ui.Rect{W: float32(math.Inf(1)), H: 50}, false)
		})
	}, 100, 50)
	tt.Frame()
}

func TestRenderedCacheEvictsOldestHalfWithoutThrashing(t *testing.T) {
	c := newShapeCache()
	chartSrc := "graph LR\n  A[入口] --> B[出口]"

	// 模拟拖动窗口宽度超过 64 个不同宽度，同时保持热点宽度活跃。
	hotWidth := float32(500)
	hotLay := c.diagram(chartSrc, hotWidth)
	if hotLay == nil {
		t.Fatal("流程图应能正常布局")
	}
	// 验证不同整数宽度仍保持精确对齐缓存，不合并为同一宽度。
	lay420 := c.diagram(chartSrc, 420)
	lay421 := c.diagram(chartSrc, 421)
	if lay420 == nil || lay421 == nil || lay420 == lay421 {
		t.Fatal("相邻整数宽度应独立布局缓存以保留精确对齐语义")
	}

	var recentLay *diagram.Layout
	for w := 501; w <= 580; w++ {
		if w%10 == 0 {
			// 周期性访问热点宽度，刷新其访问序。
			if got := c.diagram(chartSrc, hotWidth); got != hotLay {
				t.Fatalf("热点流程图在拖动至宽度 %d 时不应被误淘汰", w)
			}
		}
		recentLay = c.diagram(chartSrc, float32(w))
		if len(c.diagrams) > diagramCacheLimit {
			t.Fatalf("流程图缓存超出上限：%d > %d", len(c.diagrams), diagramCacheLimit)
		}
	}
	if len(c.diagrams) <= diagramCacheLimit/2 {
		t.Fatalf("达到上限时应仅淘汰旧条目而非全清，当前条目数：%d", len(c.diagrams))
	}
	if got := c.diagram(chartSrc, hotWidth); got != hotLay {
		t.Fatal("淘汰后热点流程图应仍命中原缓存对象")
	}
	if got := c.diagram(chartSrc, 580); got != recentLay {
		t.Fatal("淘汰后最新宽度的流程图应仍命中原缓存对象")
	}

	// 公式缓存超过 512 上限时同样淘汰最久未用的一半，不发生一刀切全清。
	hotMath := c.math("x^2 + y^2", 16, true)
	if hotMath == nil {
		t.Fatal("公式应能正常布局")
	}
	var lastMathSrc string
	var lastMathBox *mathlayout.Box
	for i := 0; i < mathCacheLimit+40; i++ {
		if i%100 == 0 {
			if got := c.math("x^2 + y^2", 16, true); got != hotMath {
				t.Fatalf("热点公式在第 %d 次插入时不应被误淘汰", i)
			}
		}
		lastMathSrc = fmt.Sprintf("x_%d + %d", i, i)
		lastMathBox = c.math(lastMathSrc, 16, false)
		if len(c.maths) > mathCacheLimit {
			t.Fatalf("公式缓存超出上限：%d > %d", len(c.maths), mathCacheLimit)
		}
	}
	if len(c.maths) <= mathCacheLimit/2 {
		t.Fatalf("公式缓存达到上限时应保留最近一半条目，当前条目数：%d", len(c.maths))
	}
	if got := c.math("x^2 + y^2", 16, true); got != hotMath {
		t.Fatal("淘汰后热点公式应仍命中原缓存对象")
	}
	if got := c.math(lastMathSrc, 16, false); got != lastMathBox {
		t.Fatal("淘汰后最新公式应仍命中原缓存对象")
	}

	// 多实例并发读写与淘汰应在 -race 下无数据竞争。
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			local := newShapeCache()
			for w := 300; w < 380; w++ {
				local.diagram(chartSrc, float32(w))
			}
			for i := 0; i < 64; i++ {
				local.math(fmt.Sprintf("a_%d_%d", id, i), 16, false)
			}
		}(g)
	}
	wg.Wait()
}
