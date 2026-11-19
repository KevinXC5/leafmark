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

func TestSVGTextMultiScaleCacheNoThrashing(t *testing.T) {
	raw := []byte(`<svg width="400" height="200" viewBox="0 0 400 200"><text x="200" y="80" text-anchor="middle" font-size="24">双尺寸缓存 ABC</text></svg>`)
	pic := DecodeSVG(raw)
	if pic == nil || len(pic.texts) != 1 {
		t.Fatalf("应解析出一段文字：%+v", pic)
	}
	txt := &pic.texts[0]

	gSmall, advSmall := txt.shaped(0.5)
	gLarge, advLarge := txt.shaped(1.5)
	if len(gSmall) == 0 || len(gLarge) == 0 || advLarge <= advSmall {
		t.Fatalf("不同缩放比例应得到有效字形与递增宽度：small=%v large=%v", advSmall, advLarge)
	}
	if txt.cacheLen != 2 {
		t.Fatalf("缓存槽位数应为 2，实际 %d", txt.cacheLen)
	}
	// 交替按两种尺寸访问，应直接复用已缓存的切片底层数组与宽度，不反复重塑形。
	for range 4 {
		g1, a1 := txt.shaped(0.5)
		g2, a2 := txt.shaped(1.5)
		if &g1[0] != &gSmall[0] || a1 != advSmall {
			t.Fatalf("0.5 倍缩放应命中缓存复用字形与宽度")
		}
		if &g2[0] != &gLarge[0] || a2 != advLarge {
			t.Fatalf("1.5 倍缩放应命中缓存复用字形与宽度")
		}
	}
	// 极小浮点误差在千分之一量化内应命中同一槽位。
	gNear, aNear := txt.shaped(0.5002)
	if &gNear[0] != &gSmall[0] || aNear != advSmall || txt.cacheLen != 2 {
		t.Fatalf("相近缩放比例应命中同一量化槽位：len=%d", txt.cacheLen)
	}

	// 填满 4 槽后引入第 5 个比例，应淘汰最久未用项并保留最近热项。
	g3, _ := txt.shaped(2.0)
	g4, _ := txt.shaped(2.5)
	if txt.cacheLen != svgTextScaleCacheSize {
		t.Fatalf("缓存槽位数应达到上限 %d，实际 %d", svgTextScaleCacheSize, txt.cacheLen)
	}
	// 刷新 0.5 使其成为最新，此时 1.5 是最久未用项。
	if gHit, _ := txt.shaped(0.5); &gHit[0] != &gSmall[0] {
		t.Fatalf("0.5 倍缩放应仍在 4 槽缓存内")
	}
	_, _ = txt.shaped(3.0)
	if txt.cacheLen != svgTextScaleCacheSize {
		t.Fatalf("超过上限后槽位数应保持 %d，实际 %d", svgTextScaleCacheSize, txt.cacheLen)
	}
	if gHit, _ := txt.shaped(0.5); &gHit[0] != &gSmall[0] {
		t.Fatalf("最近使用的 0.5 倍缩放不应被淘汰")
	}
	if gHit, _ := txt.shaped(2.0); &gHit[0] != &g3[0] {
		t.Fatalf("2.0 倍缩放不应被淘汰")
	}
	if gHit, _ := txt.shaped(2.5); &gHit[0] != &g4[0] {
		t.Fatalf("2.5 倍缩放不应被淘汰")
	}
	// 实际通过 Picture.paint 交替绘制两种尺寸不应报错且命中缓存。
	for _, w := range []float32{200, 600, 200, 600} {
		view := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Size(w, w/2).Draw(func(p *ui.Painter, r ui.Rect) { pic.paint(p, r, false) })
		}, int(w), int(w/2))
		view.Frame()
	}
}

func TestSVGTextNonStrictXMLAndSkipContainers(t *testing.T) {
	// 含未转义 &、未闭合标签，以及 pattern/marker/defs 中不应独立绘制的文字，外加大量图形节点测试写时复制隔离。
	raw := []byte(`<svg width="400" height="200" viewBox="0 0 400 200">
<defs><style>.strokeOnly{stroke:#111;stroke-width:2}.red{fill:#ff0000}</style><text x="10" y="10">defs内跳过</text></defs>
<pattern id="p1" width="20" height="20"><text x="5" y="10">pattern内跳过</text></pattern>
<marker id="m1" markerWidth="10" markerHeight="10"><text x="2" y="8">marker内跳过</text></marker>
<g fill="#0000ff" font-size="18">
  <rect x="0" y="0" width="10" height="10" class="strokeOnly"/>
  <path d="M0 0 L10 10" stroke="#333"/>
  <g transform="translate(10,20)">
    <g class="red"><text x="0" y="20">A & B</text></g>
    <g><br><text x="0" y="50">继承蓝字</text></g>
  </g>
</g>
</svg>`)
	texts := svgTexts(raw, 400, 200)
	if len(texts) != 2 {
		t.Fatalf("应仅解析出 2 段可见文字并跳过 defs/pattern/marker：%+v", texts)
	}
	if texts[0].text != "A & B" || texts[0].color != ui.RGB(255, 0, 0) || texts[0].font.Size != 18 {
		t.Fatalf("第一段文字内容或样式不对：%+v", texts[0])
	}
	if texts[1].text != "继承蓝字" || texts[1].color != ui.RGB(0, 0, 255) || texts[1].font.Size != 18 || texts[1].x != 10 || texts[1].y != 70 {
		t.Fatalf("兄弟分支不应受前一个分支的红色样式污染且坐标应正确：%+v", texts[1])
	}
}
