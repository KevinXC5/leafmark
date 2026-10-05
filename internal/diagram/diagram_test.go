package diagram

import (
	"fmt"
	"image/png"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func mustParse(t *testing.T, src string) *Graph {
	t.Helper()
	g, err := Parse(src)
	if err != nil {
		t.Fatalf("解析失败：%v\n%s", err, src)
	}
	return g
}

func TestParseNodeShapes(t *testing.T) {
	g := mustParse(t, "graph TD\nA\nB[矩形]\nC(圆角)\nD([体育场])\nE((圆形))\nF{菱形}\nG{{六边形}}\nH[(圆柱)]\nI[/平行四边形/]\nJ>旗形]\nK[\"带 [括号] 的<br>两行\"]\n")
	want := []struct {
		id, text string
		shape    Shape
	}{{"A", "A", ShapeRect}, {"B", "矩形", ShapeRect}, {"C", "圆角", ShapeRound}, {"D", "体育场", ShapeStadium}, {"E", "圆形", ShapeCircle}, {"F", "菱形", ShapeDiamond}, {"G", "六边形", ShapeHexagon}, {"H", "圆柱", ShapeCylinder}, {"I", "平行四边形", ShapeParallelogram}, {"J", "旗形", ShapeFlag}, {"K", "带 [括号] 的\n两行", ShapeRect}}
	if len(g.Nodes) != len(want) {
		t.Fatalf("节点数=%d", len(g.Nodes))
	}
	for i, w := range want {
		if n := g.Nodes[i]; n.ID != w.id || n.Text != w.text || n.Shape != w.shape {
			t.Fatalf("第%d个节点错误：%+v，预期 %+v", i, *n, w)
		}
	}
}

func TestParseLinks(t *testing.T) {
	for _, item := range []struct {
		src        string
		line       LineStyle
		head, tail Arrow
		label      string
		length     int
	}{
		{"A --> B", LineSolid, ArrowNormal, ArrowNone, "", 1},
		{"A --- B", LineSolid, ArrowNone, ArrowNone, "", 1},
		{"A -.-> B", LineDotted, ArrowNormal, ArrowNone, "", 1},
		{"A -.- B", LineDotted, ArrowNone, ArrowNone, "", 1},
		{"A ==> B", LineThick, ArrowNormal, ArrowNone, "", 1},
		{"A === B", LineThick, ArrowNone, ArrowNone, "", 1},
		{"A --o B", LineSolid, ArrowCircle, ArrowNone, "", 1},
		{"A --x B", LineSolid, ArrowCross, ArrowNone, "", 1},
		{"A <--> B", LineSolid, ArrowNormal, ArrowNormal, "", 1},
		{"A ----> B", LineSolid, ArrowNormal, ArrowNone, "", 3},
		{"A -->|文字| B", LineSolid, ArrowNormal, ArrowNone, "文字", 1},
		{"A -- 文字 --> B", LineSolid, ArrowNormal, ArrowNone, "文字", 1},
		{"A -. 文字 .-> B", LineDotted, ArrowNormal, ArrowNone, "文字", 1},
		{"A == 文字 ==> B", LineThick, ArrowNormal, ArrowNone, "文字", 1},
	} {
		g := mustParse(t, "flowchart LR\n"+item.src)
		if len(g.Edges) != 1 {
			t.Fatalf("%q 连线数=%d", item.src, len(g.Edges))
		}
		e := g.Edges[0]
		if e.From != "A" || e.To != "B" || e.Line != item.line || e.Head != item.head || e.Tail != item.tail || e.Label != item.label || e.Length != item.length {
			t.Fatalf("%q 解析为 %+v", item.src, *e)
		}
	}
}

func TestParseChainsGroupsCommentsAndIgnoredStatements(t *testing.T) {
	g := mustParse(t, `graph TD; A --> B --> C; A & B --> D & E
%% 注释
classDef hot fill:#f96
class A hot
style B fill:#bbf,stroke:#333
linkStyle 0 stroke:#f00
click A "https://example.com"
subgraph 组一 [第一组]
  direction LR
  X[甲] --> Y
  subgraph 内层
    Z
  end
end
A[后定义的文字]
`)
	if g.Direction != TopDown || len(g.Edges) != 7 {
		t.Fatalf("方向或连线数错误：%d %d", g.Direction, len(g.Edges))
	}
	if g.Node("A").Text != "后定义的文字" {
		t.Fatalf("后出现的定义应覆盖文字：%q", g.Node("A").Text)
	}
	if len(g.Subgraphs) != 2 || g.Subgraphs[0].Title != "第一组" || g.Subgraphs[1].Parent != 0 {
		t.Fatalf("子图错误：%+v", g.Subgraphs)
	}
	if strings.Join(g.Subgraphs[0].Nodes, ",") != "X,Y" || strings.Join(g.Subgraphs[1].Nodes, ",") != "Z" {
		t.Fatalf("子图成员错误：%v %v", g.Subgraphs[0].Nodes, g.Subgraphs[1].Nodes)
	}
}

func TestParseRejectsOtherDiagramsAndNeverPanics(t *testing.T) {
	for _, src := range []string{"", "sequenceDiagram\nA->>B: hi", "gantt\ntitle x", "pie\n\"a\": 1", "graph TD\nsubgraph x\nA", "graph TD\nA[未闭合"} {
		if g, err := Parse(src); err == nil {
			t.Fatalf("%q 应返回错误，得到 %+v", src, g)
		}
	}
	pieces := []string{"graph TD\n", "A", "B", "-->", "---", "-.->", "==>", "|x|", "[", "]", "(", ")", "{", "}", "((", "subgraph ", "end\n", "\n", ";", "&", "\"", "%%", " ", "<br>", "--", "o", "x", ">", "/", "\\"}
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 5000; i++ {
		var src strings.Builder
		src.WriteString("graph LR\n")
		for n := rng.Intn(14); n >= 0; n-- {
			src.WriteString(pieces[rng.Intn(len(pieces))])
		}
		if g, err := Parse(src.String()); err == nil {
			arrange(g, fakeMetrics())
		}
	}
	full := "flowchart TB\nA[开始] --> B{判断}\nB -->|是| C(处理)\nB -->|否| D((结束))\nC --> D"
	for i := 0; i <= len(full); i++ {
		if g, err := Parse(full[:i]); err == nil {
			arrange(g, fakeMetrics())
		}
	}
}

func fakeMetrics() metrics {
	return metrics{k: 1, lineH: 18, width: func(s string) float32 { return float32(len([]rune(s))) * 9 }}
}

// checkGeometry 核对节点互不重叠，且所有内容都落在画布之内。
func checkGeometry(t *testing.T, name string, geo geometry) {
	t.Helper()
	inside := func(x, y float32) bool {
		return x >= -.01 && y >= -.01 && x <= geo.width+.01 && y <= geo.height+.01
	}
	for i, a := range geo.nodes {
		if !inside(a.x-a.w/2, a.y-a.h/2) || !inside(a.x+a.w/2, a.y+a.h/2) {
			t.Fatalf("%s：节点 %s 超出画布 %vx%v：%+v", name, a.node.ID, geo.width, geo.height, a)
		}
		for _, b := range geo.nodes[i+1:] {
			if abs(a.x-b.x) < (a.w+b.w)/2-.01 && abs(a.y-b.y) < (a.h+b.h)/2-.01 {
				t.Fatalf("%s：节点 %s 与 %s 重叠", name, a.node.ID, b.node.ID)
			}
		}
	}
	for _, r := range geo.routes {
		for _, p := range r.pts {
			if !inside(p[0], p[1]) {
				t.Fatalf("%s：连线 %s→%s 超出画布", name, r.edge.From, r.edge.To)
			}
		}
	}
	for _, f := range geo.frames {
		if !inside(f.x, f.y) || !inside(f.x+f.w, f.y+f.h) {
			t.Fatalf("%s：子图 %s 超出画布", name, f.title)
		}
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestLayoutDirectionsAndSeparation(t *testing.T) {
	body := "\nA[开始] --> B{判断}\nB -->|是| C(处理)\nB -->|否| D((结束))\nC --> D\nD --> A\nC --> C\nA ---> E\n"
	for _, dir := range []string{"TD", "BT", "LR", "RL"} {
		g := mustParse(t, "graph "+dir+body)
		geo := arrange(g, fakeMetrics())
		checkGeometry(t, dir, geo)
		at := map[string]placed{}
		for _, n := range geo.nodes {
			at[n.node.ID] = n
		}
		a, b := at["A"], at["B"]
		ok := map[string]bool{"TD": b.y > a.y, "BT": b.y < a.y, "LR": b.x > a.x, "RL": b.x < a.x}[dir]
		if !ok {
			t.Fatalf("%s：B 应在 A 的下游：A=%+v B=%+v", dir, a, b)
		}
		if len(geo.routes) != len(g.Edges) {
			t.Fatalf("%s：连线数=%d", dir, len(geo.routes))
		}
		for _, r := range geo.routes {
			from, to := at[r.edge.From], at[r.edge.To]
			first, last := r.pts[0], r.pts[len(r.pts)-1]
			if r.loop {
				continue
			}
			if abs(first[0]-from.x) > from.w/2+.5 || abs(first[1]-from.y) > from.h/2+.5 || abs(last[0]-to.x) > to.w/2+.5 || abs(last[1]-to.y) > to.h/2+.5 {
				t.Fatalf("%s：连线 %s→%s 的端点没有落在节点边上", dir, r.edge.From, r.edge.To)
			}
		}
	}
}

func TestLayoutSubgraphFramesContainMembers(t *testing.T) {
	g := mustParse(t, "graph TD\nS --> A\nsubgraph 外层\n A --> B\n subgraph 内层\n  C --> D\n end\n B --> C\nend\nD --> T\n")
	geo := arrange(g, fakeMetrics())
	checkGeometry(t, "子图", geo)
	if len(geo.frames) != 2 || geo.frames[0].title != "外层" {
		t.Fatalf("外框错误：%+v", geo.frames)
	}
	outer, inner := geo.frames[0], geo.frames[1]
	if inner.x < outer.x || inner.y < outer.y || inner.x+inner.w > outer.x+outer.w || inner.y+inner.h > outer.y+outer.h {
		t.Fatalf("内层外框应在外层之内：%+v %+v", outer, inner)
	}
	for _, n := range geo.nodes {
		in := n.x-n.w/2 >= outer.x && n.x+n.w/2 <= outer.x+outer.w && n.y-n.h/2 >= outer.y && n.y+n.h/2 <= outer.y+outer.h
		member := strings.Contains("ABCD", n.node.ID)
		if in != member {
			t.Fatalf("节点 %s 与外层外框的包含关系错误", n.node.ID)
		}
	}
}

func TestLayoutLargeGraphFinishesQuickly(t *testing.T) {
	var src strings.Builder
	src.WriteString("graph TD\n")
	rng := rand.New(rand.NewSource(3))
	for i := 1; i < 100; i++ {
		fmt.Fprintf(&src, "N%d --> N%d\n", rng.Intn(i), i)
		if i%7 == 0 {
			fmt.Fprintf(&src, "N%d --> N%d\n", i, rng.Intn(i))
		}
	}
	g := mustParse(t, src.String())
	start := time.Now()
	geo := arrange(g, fakeMetrics())
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("100 个节点布局耗时 %v", took)
	}
	checkGeometry(t, "大图", geo)
}

func sampleDiagrams(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../../tests/fixtures/samples/叶脉笔记.md")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile("(?s)```mermaid\n(.*?)```").FindAllStringSubmatch(string(data), -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatal("示例文档里没有 mermaid 代码块")
	}
	return out
}

func testStyle() Style {
	return Style{Font: ui.Font{Family: "system-ui", Size: 14}, Text: ui.RGB(48, 46, 43), NodeFill: ui.RGB(241, 238, 230), NodeLine: ui.RGB(189, 120, 88), Edge: ui.RGB(121, 113, 104), LabelFill: ui.RGB(250, 249, 245), GroupFill: ui.RGB(245, 243, 236), GroupLine: ui.RGB(232, 225, 215)}
}

// TestPaintSamples 在离屏窗口里画出示例与各种外形，确认 Paint 不 panic、限宽生效。
// 设置环境变量 DIAGRAM_PNG 时把画面写到该路径，供目视核对。
func TestPaintSamples(t *testing.T) {
	sources := append(sampleDiagrams(t),
		"flowchart TD\nA([开始]) --> B{条件成立？}\nB -->|是| C[处理数据]\nB -->|否| D[/输入/]\nC --> E[(数据库)]\nD -.-> E\nE ==> F((结束))\nF --o G{{六边形}}\nG --x H>旗形]\nC --> C\nH --> A\n",
		"graph LR\nsubgraph 前端\n U[用户] --> W[界面<br>两行文字]\nend\nsubgraph 后端\n S[服务] --> DB[(存储)]\nend\nW -->|请求| S\nS -. 响应 .-> W\n",
	)
	var layouts []*Layout
	height := float32(16)
	for _, src := range sources {
		l := mustParse(t, src).Layout(testStyle(), 560)
		if l.Width > 560.5 || l.Width <= 0 || l.Height <= 0 {
			t.Fatalf("限宽未生效或尺寸为空：%vx%v", l.Width, l.Height)
		}
		checkGeometry(t, "绘制样例", l.geo)
		layouts = append(layouts, l)
		height += l.Height + 16
	}
	wide := mustParse(t, sources[0]).Layout(testStyle(), 0)
	if narrow := mustParse(t, sources[0]).Layout(testStyle(), wide.Width/2); narrow.Width > wide.Width/2+.5 || narrow.k >= 1 {
		t.Fatalf("图宽于限宽时应等比缩小：%v → %v", wide.Width, narrow.Width)
	}
	view := func(c *ui.Context) {
		ui.Box(c).Size(600, height).Background(ui.RGB(250, 249, 245)).Draw(func(p *ui.Painter, r ui.Rect) {
			y := r.Y + 16
			for _, l := range layouts {
				l.Paint(p, r.X+20, y)
				y += l.Height + 16
			}
		})
	}
	tt := ui.NewTester(view, 600, int(height))
	tt.SetScale(2)
	tt.Frame()
	img := tt.Image()
	inked := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 200 {
			inked++
		}
	}
	if inked == 0 {
		t.Fatal("离屏画面里没有画出任何内容")
	}
	if path := os.Getenv("DIAGRAM_PNG"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}
