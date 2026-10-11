package nativeeditor

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSVGTextStylesAndPixels(t *testing.T) {
	raw := []byte(`<svg width="400" height="200" viewBox="0 0 400 200"><defs><style>text{fill:#ffffff;font-family:sans-serif}.label{font-size:28px;font-weight:700}</style><text>不显示</text></defs><rect width="400" height="200" fill="#102030"/><text x="200" y="60" text-anchor="middle" class="label" fill="#000000">中文标题 ABC</text><g transform="translate(20,100)"><text font-size="20" style="fill:#ff0000">指标 74%<tspan x="0" dy="30">第二行</tspan></text></g></svg>`)
	pic := DecodeSVG(raw)
	if pic == nil || len(pic.texts) != 3 {
		t.Fatalf("应解析三段可见文字：%+v", pic)
	}
	title := pic.texts[0]
	if title.font.Size != 28 || title.font.Weight != 700 || title.color != ui.RGB(255, 255, 255) || title.anchor != "middle" {
		t.Fatalf("CSS、字体与居中属性未生效：%+v", title)
	}
	if pic.texts[2].x != 20 || pic.texts[2].y != 130 {
		t.Fatalf("tspan 定位错误：%+v", pic.texts[2])
	}
	for _, width := range []float32{400, 800} {
		view := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Size(width, width/2).Draw(func(p *ui.Painter, r ui.Rect) { pic.paint(p, r, false) })
		}, int(width), int(width/2))
		view.Frame()
		img := view.Image()
		white := 0
		for y := int(width * .05); y < int(width*.16); y++ {
			for x := int(width * .15); x < int(width*.85); x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r > 50000 && g > 50000 && b > 50000 {
					white++
				}
			}
		}
		if white < 30 {
			t.Fatalf("缩放到 %v 后标题没有真实画出：%d", width, white)
		}
	}
}

// 原图只用于本地验收，不把用户笔记的附件纳入仓库。
func TestRenderSVGReference(t *testing.T) {
	input, output := os.Getenv("LEAFMARK_SVG_INPUT"), os.Getenv("LEAFMARK_SVG_PNG")
	if input == "" || output == "" {
		t.Skip("未指定 SVG 原图")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	pic := DecodeSVG(raw)
	if pic == nil || len(pic.texts) == 0 {
		t.Fatal("SVG 文字解析失败")
	}
	view := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(pic.w, pic.h).Draw(func(p *ui.Painter, r ui.Rect) { pic.paint(p, r, true) })
	}, int(pic.w), int(pic.h))
	view.Frame()
	f, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, view.Image()); err != nil {
		t.Fatal(err)
	}
	t.Logf("已绘制 %d 段 SVG 文字", len(pic.texts))
}
