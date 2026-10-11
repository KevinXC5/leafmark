//go:build desktoptest

package desktop

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

func TestNativeOutlineVeinGeometry(t *testing.T) {
	for _, height := range []float32{31.2, 39, 62.4, 72.8} {
		rows := []ui.Rect{{X: 24, Y: 117, W: 216, H: height + nativeOutlineGap}}
		for i := 1; i < 3; i++ {
			previous := rows[i-1]
			rows = append(rows, ui.Rect{X: previous.X, Y: previous.Y + previous.H, W: previous.W, H: previous.H})
		}
		var previous nativeOutlineVein
		for i, row := range rows {
			v := nativeOutlineVeinGeometry(row, i+1, i == 0, i == len(rows)-1)
			if math.Abs(float64(v.center-(row.Y+height/2))) > .001 {
				t.Fatalf("叶脉应对齐实际按钮中点：高度 %v，几何 %+v", height, v)
			}
			if i == 0 && v.top != v.center || i == len(rows)-1 && v.bottom != v.center {
				t.Fatalf("首尾主脉应止于标题中点：%+v", v)
			}
			if i > 0 && (v.top != previous.bottom || v.mainX != previous.mainX || v.tipX <= previous.tipX) {
				t.Fatalf("主脉应连续且深层侧脉更长：%+v → %+v", previous, v)
			}
			previous = v
		}
		single := nativeOutlineVeinGeometry(rows[0], 1, true, true)
		if single.top != single.bottom || single.top != single.center {
			t.Fatalf("单个标题不应留下悬空主脉：%+v", single)
		}
	}
}

type outlineSelectionEditor struct {
	stubEditor
	at int
}

func (e *outlineSelectionEditor) Selection() (int, int) { return e.at, e.at }
func (e *outlineSelectionEditor) SetSelection(start, end int) {
	e.at = start
}

func TestNativeOutlineVeinsKeepNavigationAndSpacing(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	markdown := "# 根\n\n正文\n\n## 分枝\n\n正文\n\n###### 末梢\n"
	app.adopt(app.files.Current(), markdown)
	editor := &outlineSelectionEditor{stubEditor: stubEditor{text: markdown}}
	app.active().editor = editor
	view := ui.NewTester(app.View, 1100, 760)
	headings := nativeOutline(markdown)
	for _, size := range []float32{12, 15, 24, 28} {
		app.setFont(size)
		view.Frame()
		var previous ui.Rect
		for i, h := range headings {
			label := "大纲：" + h.text
			row, ok := view.Find(label)
			if !ok {
				t.Fatalf("叶脉大纲缺少原有按钮 %q", label)
			}
			if i > 0 && math.Abs(float64(row.Y-(previous.Y+previous.H+nativeOutlineGap))) > .01 {
				t.Fatalf("主脉容器必须覆盖实际行间距：%+v → %+v", previous, row)
			}
			text, ok := view.Find(h.text)
			if !ok || text.X <= row.X+nativeOutlineTip(h.level) || text.X+text.W > row.X+row.W {
				t.Fatalf("标题文字应位于叶脉末端之后且保持在按钮内：%+v %+v", row, text)
			}
			if err := view.Click(label); err != nil {
				t.Fatal(err)
			}
			if editor.at != h.at {
				t.Fatalf("大纲点击未定位到标题：得到 %d，期望 %d", editor.at, h.at)
			}
			previous = row
		}
	}
}

func TestNativeOutlineEmptyLeafIsCentered(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "没有标题的正文")
	view := ui.NewTester(app.View, 1100, 760)
	for _, size := range []float32{15, 28} {
		app.setFont(size)
		view.Frame()
		leaf, found := view.Find("大纲空状态叶子")
		text, textFound := view.Find("文档中的标题会显示在这里")
		if !found || !textFound || leaf.W < 48 || leaf.W > 56 || math.Abs(float64(leaf.H-leaf.W)) > .01 || leaf.Y+leaf.H >= text.Y {
			t.Fatalf("空状态叶子应在原有文案上方且尺寸克制：%+v %+v", leaf, text)
		}
		if math.Abs(float64((leaf.X+leaf.W/2)-(text.X+text.W/2))) > .01 {
			t.Fatalf("叶子与文案应水平居中：%+v %+v", leaf, text)
		}
	}
}

func TestNativeOutlineUsesModelRuneCoordinates(t *testing.T) {
	markdown := "# 同名\n\n中文😀段落\n\n| 甲 | 乙 |\n| --- | --- |\n| 同名 | 内容 |\n\n```text\n# 同名\n```\n\n## 同名\n\n尾声\n"
	headings := nativeOutline(markdown)
	if len(headings) != 2 {
		t.Fatalf("应只列出模型中的两条标题，得到 %v", headings)
	}
	doc := richtext.Parse(markdown)
	for _, h := range headings {
		b := doc.Blocks()[doc.BlockIndexAt(h.at)]
		if b.Kind != richtext.Heading || b.Level != h.level {
			t.Fatalf("标题 rune 坐标错误：%+v", h)
		}
	}
	if headings[0].at != 0 || headings[1].at <= headings[0].at {
		t.Fatalf("重复标题不能定位到同一处：%v", headings)
	}
}
func TestNativeVisualRestoresCompactChrome(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "# 山中来信\n\n正文")
	view := ui.NewTester(app.View, 1100, 760)
	side, ok := view.Find("文档导航")
	if !ok || side.X != 8 || side.Y != 8 || side.W != sidebarDefaultWidth-16 || side.H != 744 {
		t.Fatalf("浮动侧栏尺寸不符：%+v", side)
	}
	if !view.HasText("大纲：山中来信") || !view.HasText("叶笺") || !view.HasText("UTF-8") {
		t.Fatal("原版大纲、品牌或状态栏缺失")
	}
	if view.HasText("加粗") || view.HasText("保存文档") {
		t.Fatal("格式/保存操作不应常驻占据正文")
	}
	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("格式", "加粗"); err != nil {
		t.Fatal(err)
	}
	e := app.active().editor.(*stubEditor)
	if len(e.formats) != 1 || e.formats[0] != "bold" {
		t.Fatalf("格式菜单未接现有命令：%v", e.formats)
	}
	if err := view.Click("新建文档"); err != nil {
		t.Fatal(err)
	}
	if len(app.tabs) != 2 {
		t.Fatal("+ 应创建文档")
	}
}
func TestNativeVisualNarrowWindowKeepsActionsInBounds(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "正文")
	for i := 0; i < 12; i++ {
		app.adopt(app.files.New(), "另一个文档")
	}
	view := ui.NewTester(app.View, 480, 600)
	if _, ok := view.Find("文档导航"); ok {
		t.Fatal("窄窗口应收起侧栏，保留正文空间")
	}
	for _, label := range []string{"新建文档", "更多操作", "展开侧栏", "状态栏"} {
		r, ok := view.Find(label)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 480 || r.Y+r.H > 600 {
			t.Fatalf("窄窗口操作溢出：%s %+v", label, r)
		}
	}
	checkSelected := func() {
		t.Helper()
		view.Frame()
		name := app.files.Current().Name
		button, found := view.Find(name)
		close, closeFound := view.Find("关闭 " + name)
		create, _ := view.Find("新建文档")
		if !found || !closeFound || button.X < 40 || close.X+close.W > create.X {
			t.Fatalf("当前标签及关闭按钮应完整可见：%+v %+v，新建按钮 %+v", button, close, create)
		}
	}
	checkSelected()
	app.selectTab(0)
	checkSelected()
	app.selectTab(len(app.tabs) - 1)
	checkSelected()
	if err := view.Click("新建文档"); err != nil {
		t.Fatal(err)
	}
	checkSelected()
	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("设置"); err != nil {
		t.Fatal(err)
	}
	r, ok := view.Find("设置导航")
	if !ok || r.X != 8 || r.Y != 8 {
		t.Fatalf("设置应是全窗口布局：%+v", r)
	}
	if err := view.Click("通用"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("检查更新") {
		t.Fatal("设置需保留更新功能")
	}
	if err := view.Click("关闭设置"); err != nil {
		t.Fatal(err)
	}
	if app.settingsOn {
		t.Fatal("应返回原有书写标签")
	}
}

// TestFindBarReplaceAndReadingMode 核对查找栏的替换按钮，以及阅读模式在菜单和状态栏上的表现。
func TestFindBarReplaceAndReadingMode(t *testing.T) {
	app, _ := newTestApp(t)
	app.adopt(app.files.Current(), "正文 正文")
	editor := &readingEditor{stubEditor: stubEditor{text: "正文 正文", findHit: true}}
	app.active().editor = editor
	app.findOn = true
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	for _, label := range []string{"查找", "替换为", "替换", "全部替换"} {
		if _, ok := view.Find(label); !ok {
			t.Fatalf("查找栏缺少 %q", label)
		}
	}
	if err := view.Click("全部替换"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	// 查找词为空时替换无事可做，正文与提示都保持原样。
	if editor.text != "正文 正文" || app.findNote != "" {
		t.Fatalf("空查找不应触发替换：%q %q", editor.text, app.findNote)
	}
	app.findQuery, app.replaceText = "正文", "替换"
	if err := view.Click("替换"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if editor.text != "替换 正文" || app.findNote != "已替换 1 处" {
		t.Fatalf("替换按钮未触发单处替换：%q %q", editor.text, app.findNote)
	}

	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("进入阅读模式"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !app.reading || !editor.readOnly || !view.HasText("• 阅读模式") {
		t.Fatal("菜单应进入阅读模式，状态栏应标明")
	}
	// 阅读模式下替换按钮已禁用，点击不生效，正文保持不变。
	if err := view.Click("全部替换"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if editor.text != "替换 正文" {
		t.Fatalf("阅读模式的替换按钮仍可替换：%q", editor.text)
	}
	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("格式", "加粗"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("阅读模式下格式菜单应不可用：%v", err)
	}
	view.CloseMenu()
	if err := view.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("退出阅读模式"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if app.reading || !view.HasText("• 原位编辑") {
		t.Fatal("应退出阅读模式并恢复状态栏")
	}
}

// 字号设置作用于整个界面：侧栏、标签和状态栏的文字与正文一起缩放，默认字号下保持设计尺寸。
func TestNativeFontSizeScalesWholeInterface(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	app.adopt(app.files.Current(), "# 山中来信\n\n正文")
	tab := app.files.Current().Name
	view := ui.NewTester(app.View, 1100, 760)
	measure := func() map[string]ui.Rect {
		view.Frame()
		out := map[string]ui.Rect{}
		for _, label := range []string{"叶笺", tab, "UTF-8", "状态栏", "大纲：山中来信"} {
			r, ok := view.Find(label)
			if !ok {
				t.Fatalf("界面缺少 %s", label)
			}
			out[label] = r
		}
		return out
	}
	base := measure()
	if base["状态栏"].H != 34 || base["大纲：山中来信"].H != 39 {
		t.Fatalf("默认字号下界面应保持设计尺寸：%+v", base)
	}
	app.setFont(24)
	large := measure()
	for _, label := range []string{"叶笺", "UTF-8", "状态栏", "大纲：山中来信"} {
		if got, want := large[label].H/base[label].H, float32(24)/uiBaseFontSize; got < want-.05 || got > want+.05 {
			t.Fatalf("%s 应随字号放大到 %.2f 倍，得到 %.2f", label, want, got)
		}
	}
	if large[tab].W <= base[tab].W {
		t.Fatalf("标签文字应随字号放大：%+v → %+v", base[tab], large[tab])
	}
	app.setFont(12)
	if small := measure(); small["UTF-8"].H >= base["UTF-8"].H {
		t.Fatalf("调小字号时界面文字应随之缩小：%+v", small["UTF-8"])
	}
}

// 拖动侧栏右缘改变宽度，松手后写入设置；宽度受上下限约束，双击恢复默认。
func TestSidebarResizeByDragging(t *testing.T) {
	app, _ := newTestApp(t)
	useStubEditors(t)
	view := ui.NewTester(app.View, 1100, 760)
	view.Frame()
	handle, ok := view.Find("调整侧栏宽度")
	if !ok {
		t.Fatal("侧栏缺少拖动手柄")
	}
	x, y := handle.X+handle.W/2, handle.Y+handle.H/2
	view.Press(x, y)
	view.Move(x+80, y)
	view.Release(x+80, y)
	view.Frame()
	if app.settings.SidebarWidth != sidebarDefaultWidth+80 {
		t.Fatalf("拖动后侧栏宽度应为 %d，得到 %v", sidebarDefaultWidth+80, app.settings.SidebarWidth)
	}
	// 设置在后台协程写盘，等它落地。
	deadline := time.Now().Add(2 * time.Second)
	for loadNativeSettings().SidebarWidth != sidebarDefaultWidth+80 {
		if time.Now().After(deadline) {
			t.Fatalf("松手后应保存侧栏宽度，得到 %v", loadNativeSettings().SidebarWidth)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if moved, _ := view.Find("调整侧栏宽度"); moved.X-handle.X != 80 {
		t.Fatalf("手柄应随侧栏右移 80，实际 %v", moved.X-handle.X)
	}
	handle, _ = view.Find("调整侧栏宽度")
	x = handle.X + handle.W/2
	view.Press(x, y)
	view.Move(x+1000, y)
	view.Release(x+1000, y)
	view.Frame()
	if app.settings.SidebarWidth != sidebarMaxWidth {
		t.Fatalf("侧栏宽度应限制在 %d，得到 %v", sidebarMaxWidth, app.settings.SidebarWidth)
	}
	if got := sidebarWidth(sidebarMaxWidth, 760); got != 340 {
		t.Fatalf("窄窗口下正文区应至少保留 420，侧栏得到 %v", got)
	}
	if got := sidebarWidth(100, 1100); got != sidebarMinWidth {
		t.Fatalf("侧栏宽度不应小于 %d，得到 %v", sidebarMinWidth, got)
	}
}
