//go:build desktoptest

package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

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
	if !ok || side.X != 8 || side.Y != 8 || side.W != 248 || side.H != 744 {
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
	for _, label := range []string{"更多操作", "展开侧栏", "状态栏"} {
		r, ok := view.Find(label)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 480 || r.Y+r.H > 600 {
			t.Fatalf("窄窗口操作溢出：%s %+v", label, r)
		}
	}
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
