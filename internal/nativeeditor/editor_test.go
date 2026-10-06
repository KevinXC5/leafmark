package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

func newTest(t *testing.T, md string) (*Editor, *ui.Tester) {
	t.Helper()
	ed := New(md)
	ed.FontSize(16)
	tt := ui.NewTester(ed.View, 640, 480)
	tt.ClickAt(40, 30)
	return ed, tt
}

func TestClickAndType(t *testing.T) {
	ed, tt := newTest(t, "你好")
	tt.ClickAt(48, 36)
	tt.Type("世界")
	if !strings.Contains(ed.Text(), "世界") {
		t.Fatalf("输入后文本 = %q", ed.Text())
	}
	if !strings.Contains(ed.Markdown(), "世界") {
		t.Fatalf("Markdown = %q", ed.Markdown())
	}
	if !ed.Changed() {
		t.Fatal("输入后应报告变化")
	}
	if ed.Changed() {
		t.Fatal("Changed 应在读取后清掉")
	}
}

func TestBoldFormat(t *testing.T) {
	ed, tt := newTest(t, "hello")
	ed.SetSelection(0, 5)
	tt.Frame()
	ed.Format("bold")
	tt.Frame()
	md := ed.Markdown()
	if !strings.Contains(md, "**hello**") && !strings.Contains(md, "__hello__") {
		t.Fatalf("加粗后 Markdown = %q", md)
	}
	// 画面上仍是纯文本，不出现星号。
	if strings.Contains(ed.Text(), "*") {
		t.Fatalf("可见文本不应含标记: %q", ed.Text())
	}
}

func TestChineseEmojiCluster(t *testing.T) {
	ed, tt := newTest(t, "甲👍乙")
	tt.Frame()
	text := ed.Text()
	if text != "甲👍乙" {
		t.Fatalf("文本 = %q", text)
	}
	// 光标按 rune 移动，emoji 算一个单位，不会拆成代理项。
	ed.SetSelection(0, 0)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyRight)
	start, end := ed.Selection()
	if start != 2 || end != 2 {
		t.Fatalf("越过 emoji 后光标 = %d,%d 文本长 %d", start, end, len([]rune(text)))
	}
	x0, _, _ := ed.PointForOffset(1)
	x1, _, _ := ed.PointForOffset(2)
	if x1 <= x0 {
		t.Fatalf("emoji 应占宽度 x0=%v x1=%v", x0, x1)
	}
}

func TestEnterSplitsParagraph(t *testing.T) {
	ed, tt := newTest(t, "甲乙")
	ed.SetSelection(1, 1)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if strings.Count(ed.Text(), "\n") < 1 {
		t.Fatalf("换行后文本 = %q", ed.Text())
	}
	blocks := 0
	for _, b := range strings.Split(ed.Markdown(), "\n\n") {
		if strings.TrimSpace(b) != "" {
			blocks++
		}
	}
	if blocks < 2 {
		t.Fatalf("应拆成两块: %q", ed.Markdown())
	}
}

func TestCrossBlockDelete(t *testing.T) {
	ed, tt := newTest(t, "甲乙\n\n丙丁")
	tt.Frame()
	n := ed.doc.Len()
	if n < 4 {
		t.Fatalf("长度 %d 文本 %q", n, ed.Text())
	}
	// 删掉第一块尾字、换行和第二块首字。
	ed.SetSelection(1, 4)
	tt.Frame()
	tt.Key(0, ui.KeyBackspace)
	got := ed.Text()
	if strings.Contains(got, "\n") {
		t.Fatalf("跨块删除后不应仍有换行: %q", got)
	}
	if !strings.Contains(got, "甲") || !strings.Contains(got, "丁") {
		t.Fatalf("应保留两侧文字: %q", got)
	}
}

func TestComposeDoesNotCommit(t *testing.T) {
	ed, tt := newTest(t, "甲")
	ed.SetSelection(1, 1)
	tt.Frame()
	before := ed.Markdown()
	tt.Compose("ni", 2)
	if ed.Markdown() != before {
		t.Fatalf("组合中不应落盘: %q", ed.Markdown())
	}
	if _, ok := tt.TextCaret(); !ok {
		t.Fatal("应暴露 TextCaret 给输入法")
	}
	tt.Type("你")
	if !strings.Contains(ed.Text(), "你") {
		t.Fatalf("确认后应插入: %q", ed.Text())
	}
}

func TestUndoRedo(t *testing.T) {
	ed, tt := newTest(t, "甲")
	ed.SetSelection(1, 1)
	tt.Frame()
	tt.Type("乙")
	if !strings.Contains(ed.Text(), "乙") {
		t.Fatalf("插入失败 %q", ed.Text())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if strings.Contains(ed.Text(), "乙") {
		t.Fatalf("撤销后仍在: %q", ed.Text())
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if !strings.Contains(ed.Text(), "乙") {
		t.Fatalf("重做后应恢复: %q", ed.Text())
	}
}

func TestRawPlaceholderKeepsSource(t *testing.T) {
	src := "<div>\n自定义\n</div>\n\n段落"
	ed, tt := newTest(t, src)
	tt.Frame()
	md := ed.Markdown()
	if !strings.Contains(md, "<div>") {
		t.Fatalf("Raw 源码应保留: %q", md)
	}
	if strings.Contains(ed.Text(), "<div>") {
		t.Fatalf("正文不应露出源码: %q", ed.Text())
	}
	img := tt.Image()
	if img == nil {
		t.Fatal("应能截到一帧")
	}
}

func TestRejectsUnsafeLink(t *testing.T) {
	ed := New("甲")
	ed.SetSelection(1, 1)
	ed.InsertLink("点我", "javascript:alert(1)")
	if strings.Contains(ed.Markdown(), "javascript:") {
		t.Fatalf("危险链接不应写入: %q", ed.Markdown())
	}
	ed.InsertLink("站", "https://example.com")
	if !strings.Contains(ed.Markdown(), "https://example.com") {
		t.Fatalf("安全链接应写入: %q", ed.Markdown())
	}
}

func TestImageDataURI(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var raw []byte
	w := &byteWriter{buf: &raw}
	if err := png.Encode(w, img); err != nil {
		t.Fatal(err)
	}
	ed := New("")
	ed.SetReadImage(func(uri string) *ui.Bitmap {
		if !strings.HasPrefix(uri, "data:image/png") {
			t.Fatalf("不应读取非 data URI: %s", uri)
		}
		bm, err := ui.DecodeBitmap(raw)
		if err != nil {
			t.Fatal(err)
		}
		return bm
	})
	ed.InsertImage("图", "data:image/png;base64,xx")
	if !strings.Contains(ed.Markdown(), "![图](data:image/png;base64,xx)") && !strings.Contains(ed.Markdown(), "图") {
		t.Fatalf("图片应写入: %q", ed.Markdown())
	}
	tt := ui.NewTester(ed.View, 400, 300)
	tt.Frame()
}

type byteWriter struct{ buf *[]byte }

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func TestZWJBackspace(t *testing.T) {
	ed, tt := newTest(t, "甲👨‍👩‍👧‍👦乙")
	tt.Frame()
	rs := []rune(ed.Text())
	// 家庭 emoji 是 ZWJ 序列，退格应整簇删除。
	ed.SetSelection(1+len([]rune("👨‍👩‍👧‍👦")), 1+len([]rune("👨‍👩‍👧‍👦")))
	tt.Frame()
	before := len([]rune(ed.Text()))
	tt.Key(0, ui.KeyBackspace)
	got := []rune(ed.Text())
	if string(got) != "甲乙" {
		t.Fatalf("ZWJ 退格后 = %q（原 %d rune，现 %d）原文 %q", string(got), before, len(got), string(rs))
	}
}

func TestBlockFormats(t *testing.T) {
	ed, _ := newTest(t, "标题")
	ed.Format("heading1")
	if !strings.HasPrefix(ed.Markdown(), "# ") {
		t.Fatalf("标题 Markdown = %q", ed.Markdown())
	}
	ed.Format("quote")
	if !strings.Contains(ed.Markdown(), "> ") {
		t.Fatalf("引用 Markdown = %q", ed.Markdown())
	}
	ed.Format("task")
	if !strings.Contains(ed.Markdown(), "- [ ] ") {
		t.Fatalf("任务 Markdown = %q", ed.Markdown())
	}
	ed.Format("ordered")
	if !strings.Contains(ed.Markdown(), ". ") {
		t.Fatalf("有序 Markdown = %q", ed.Markdown())
	}
}

func TestLongDocumentScroll(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 80; i++ {
		b.WriteString("第")
		b.WriteString(strings.Repeat("行", 1))
		b.WriteByte('\n')
		b.WriteByte('\n')
	}
	ed := New(b.String())
	tt := ui.NewTester(ed.View, 320, 200)
	tt.Frame()
	ed.SetSelection(ed.doc.Len(), ed.doc.Len())
	tt.Frame()
	tt.SetSize(480, 240)
	tt.Frame()
	if ed.scroll.Y <= 0 && ed.lay.height > 240 {
		t.Fatalf("长文末尾应滚出视口，scroll=%v 高度=%v", ed.scroll.Y, ed.lay.height)
	}
}

func TestWordAndHome(t *testing.T) {
	ed, tt := newTest(t, "hello world")
	ed.SetSelection(11, 11)
	tt.Frame()
	tt.Key(wordModifier(), ui.KeyLeft) // 按词移动的修饰键随平台不同
	s, _ := ed.Selection()
	if s != 6 {
		t.Fatalf("词移动应到 6，实际 %d", s)
	}
	tt.Key(0, ui.KeyHome)
	s, _ = ed.Selection()
	if s != 0 {
		t.Fatalf("Home 应到行首，实际 %d", s)
	}
}

func TestViewportWrapScrollAndHit(t *testing.T) {
	ed := New(strings.Repeat("中文正文与软换行", 120))
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Height(72)
			ui.Row(c).Fill().Grow(1).MinHeight(0).Children(func() {
				ui.Box(c).Width(90)
				ui.Box(c).Fill().Grow(1).MinHeight(0).Children(func() { ed.View(c) })
			})
		})
	}, 420, 260)
	tt.Frame()
	if abs32(ed.width-330) > 1 || abs32(ed.viewH-188) > 1 {
		t.Fatalf("真实视口=%v×%v", ed.width, ed.viewH)
	}
	if ed.scroll.MaxY <= 0 || len(ed.lay.lines) < 3 {
		t.Fatalf("长文未形成滚动内容: %+v", ed.scroll)
	}
	ln := ed.lay.lines[2]
	at := ln.origin + 2
	x, y, h := ed.PointForOffset(at)
	tt.ClickAt(90+x, 72+y+h/2)
	a, b := ed.Selection()
	if a != at || b != at {
		t.Fatalf("软换行点击选区=%d,%d 期望%d", a, b, at)
	}
	ed.SetSelection(ed.doc.Len(), ed.doc.Len())
	tt.Frame()
	_, y, h = ed.PointForOffset(ed.doc.Len())
	if y-ed.scroll.Y < 0 || y+h-ed.scroll.Y > ed.viewH+1 {
		t.Fatalf("末尾光标不可见 y=%v h=%v scroll=%v view=%v", y, h, ed.scroll.Y, ed.viewH)
	}
	oldLines := len(ed.lay.lines)
	tt.SetSize(700, 260)
	tt.Frame()
	if abs32(ed.width-610) > 1 || len(ed.lay.lines) >= oldLines {
		t.Fatalf("宽度变化未重排: width=%v lines=%d/%d", ed.width, len(ed.lay.lines), oldLines)
	}
	tt.Scroll(150, 140, 0, -100)
	tt.Frame()
	if ed.scroll.Y >= ed.scroll.MaxY {
		t.Fatal("鼠标滚动未改变位置")
	}
}

func TestRepeatedPaintKeepsLayoutAndDocument(t *testing.T) {
	ed, tt := newTest(t, "- **重复文字**重复文字\n\n> 重复文字\n\n普通重复文字")
	tt.Compose("ni", 1)
	before := ed.LayoutSnapshot()
	md := ed.Markdown()
	g := append([]ui.Glyph(nil), ed.lay.lines[0].glyphs...)
	_ = tt.Image()
	_ = tt.Image()
	tt.Frame()
	if ed.Markdown() != md || !reflect.DeepEqual(before, ed.LayoutSnapshot()) || !reflect.DeepEqual(g, ed.lay.lines[0].glyphs) {
		t.Fatal("重复绘制改变文档或排版")
	}
}

func TestCompositionNavigationDoesNotCommit(t *testing.T) {
	for _, key := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyBackspace, ui.KeyDelete, ui.KeyEnter} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			ed, tt := newTest(t, "甲乙")
			ed.SetSelection(1, 1)
			tt.Frame()
			tt.Compose("ni", 2)
			tt.Key(0, key)
			if strings.Contains(ed.Markdown(), "ni") {
				t.Fatalf("组合串被按键写入: %q", ed.Markdown())
			}
		})
	}
	ed, tt := newTest(t, "甲乙")
	ed.SetSelection(0, 1)
	tt.Frame()
	before := ed.Markdown()
	tt.Compose("ni", 2)
	if ed.Changed() || ed.Markdown() != before {
		t.Fatal("预编辑改变文档")
	}
	tt.Type("你")
	tt.Key(ui.Cmd, ui.KeyZ)
	if ed.Markdown() != before {
		t.Fatalf("输入法替换应一步撤销: %q", ed.Markdown())
	}
}

func TestClusterMovementAndDeletion(t *testing.T) {
	for _, cluster := range []string{"👨‍👩‍👧‍👦", "é", "👍🏽", "🇨🇳", "1️⃣"} {
		t.Run(cluster, func(t *testing.T) {
			ed, tt := newTest(t, "甲"+cluster+"乙")
			n := len([]rune(cluster))
			ed.SetSelection(1, 1)
			tt.Frame()
			tt.Key(0, ui.KeyRight)
			a, _ := ed.Selection()
			if a != 1+n {
				t.Fatalf("右移拆簇: %d", a)
			}
			tt.Key(0, ui.KeyLeft)
			a, _ = ed.Selection()
			if a != 1 {
				t.Fatalf("左移拆簇: %d", a)
			}
			tt.Key(0, ui.KeyDelete)
			if ed.Text() != "甲乙" {
				t.Fatalf("向前删除: %q", ed.Text())
			}
			tt.Key(ui.Cmd, ui.KeyZ)
			ed.SetSelection(n+1, n+1)
			tt.Frame()
			tt.Key(0, ui.KeyBackspace)
			if ed.Text() != "甲乙" {
				t.Fatalf("退格: %q", ed.Text())
			}
		})
	}
}

func TestReplacementUndoSingleStep(t *testing.T) {
	for _, enter := range []bool{false, true} {
		ed, tt := newTest(t, "甲乙丙丁")
		before := ed.Markdown()
		ed.SetSelection(1, 3)
		tt.Frame()
		if enter {
			tt.Key(0, ui.KeyEnter)
		} else {
			tt.Type("替换")
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		if ed.Markdown() != before {
			t.Fatalf("替换撤销分成多步: %q", ed.Markdown())
		}
		a, b := ed.Selection()
		if a != 1 || b != 3 {
			t.Fatalf("撤销未恢复选区: %d,%d", a, b)
		}
	}
}

func TestTableOffsetsAndHit(t *testing.T) {
	ed, tt := newTest(t, "|甲|乙|\n|---|---|\n|丙|丁|\n\n尾文")
	for i, r := range []rune(ed.Text()) {
		if r == '\n' || r == '\t' {
			continue
		}
		x, y, h := ed.PointForOffset(i)
		tt.ClickAt(x, y+h/4)
		a, _ := ed.Selection()
		if a != i {
			t.Fatalf("表格偏移%d(%c)命中%d", i, r, a)
		}
	}
}

func TestAuthorizedLocalImageAndRemoteWake(t *testing.T) {
	var raw bytes.Buffer
	_ = png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	ed := New("![授权](assets/a.png)\n\n![未授权](file:///secret.png)")
	var paths []string
	ed.SetReadImage(func(path string) *ui.Bitmap {
		paths = append(paths, path)
		if path != "assets/a.png" {
			return nil
		}
		bm, _ := ui.DecodeBitmap(raw.Bytes())
		return bm
	})
	tt := ui.NewTester(ed.View, 300, 200)
	tt.Frame()
	if ed.images["assets/a.png"] == nil || ed.images["file:///secret.png"] != nil || len(paths) != 2 {
		t.Fatalf("附件授权调用=%v", paths)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(raw.Bytes()) }))
	defer srv.Close()
	wake := make(chan struct{}, 4)
	ed.SetInvalidate(func() { wake <- struct{}{} })
	ed.InsertImage("远程", srv.URL)
	tt.Frame()
	select {
	case <-wake:
	case <-time.After(3 * time.Second):
		t.Fatal("异步结果没有唤醒窗口")
	}
	if ed.images[srv.URL] != nil {
		t.Fatal("后台线程写入图片缓存")
	}
	tt.Frame()
	if ed.images[srv.URL] == nil || ed.remoteN != 0 {
		t.Fatal("UI线程没有消化异步结果")
	}
}

func TestExitSpecialBlockAndUndo(t *testing.T) {
	for _, src := range []string{"```go\ncode\n```", "|甲|乙|\n|---|---|\n|丙|丁|", "![图](missing.png)", "<div>原文</div>"} {
		ed, tt := newTest(t, src)
		before := ed.Markdown()
		ed.SetSelection(ed.doc.Len(), ed.doc.Len())
		tt.Frame()
		tt.Key(ui.Cmd, ui.KeyEnter)
		tt.Type("新段落")
		if !strings.Contains(ed.Markdown(), "新段落") || ed.doc.Blocks()[len(ed.doc.Blocks())-1].Kind != richtext.Paragraph {
			t.Fatalf("退出特殊块失败: %q", ed.Markdown())
		}
		tt.Key(ui.Cmd, ui.KeyZ)
		tt.Key(ui.Cmd, ui.KeyZ)
		if ed.Markdown() != before {
			t.Fatalf("退出撤销未保留原文: %q", ed.Markdown())
		}
	}
}

func TestTaskCheckboxClickAndUndo(t *testing.T) {
	ed, tt := newTest(t, "- [ ] 待办")
	// newTest 的聚焦点击已可能落在复选框，显式恢复未完成状态再验收。
	if ed.doc.Blocks()[0].Checked {
		ed.Undo()
		tt.Frame()
	}
	x, y, _, _, _ := ed.BlockRect(0)
	tt.ClickAt(x+5, y+6)
	if !ed.doc.Blocks()[0].Checked || !strings.Contains(ed.Markdown(), "[x]") {
		t.Fatalf("复选框点击无效: %q", ed.Markdown())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if ed.doc.Blocks()[0].Checked {
		t.Fatal("任务完成状态不能撤销")
	}
}

// 方法集必须直接满足 desktop 的回调签名，不能使用不同的命名函数类型。
var _ interface{ SetReadImage(func(string) *ui.Bitmap) } = (*Editor)(nil)
var _ interface{ SetInvalidate(func()) } = (*Editor)(nil)

func TestSoftWrapBoundaryAndGlyphCache(t *testing.T) {
	ed := New("> " + strings.Repeat("重复文字", 30))
	tt := ui.NewTester(ed.View, 210, 160)
	tt.Frame()
	ln := ed.lay.lines[1]
	x, y, h := ed.PointForOffset(ln.origin)
	if abs32(y-ln.y) > .1 {
		t.Fatalf("软换行起点归到上一行: %v / %v", y, ln.y)
	}
	tt.ClickAt(x, y+h/2)
	a, _ := ed.Selection()
	if a != ln.origin {
		t.Fatalf("软换行行首命中=%d 期望%d", a, ln.origin)
	}
	cached := ed.cache.shape("重复文字", 16, false, false, false)
	shiftGlyphs(cached, 80)
	next := ed.cache.shape("重复文字", 16, false, false, false)
	if next[0].X >= 80 {
		t.Fatal("排版污染共享字形缓存")
	}
}

func TestImageDecodeLimits(t *testing.T) {
	if decodeDataURI("data:text/plain;base64,YQ==") != nil {
		t.Fatal("接受非图片data URI")
	}
	if decodeDataURI("data:image/png;base64,"+strings.Repeat("A", base64.StdEncoding.EncodedLen(remoteLimit)+1)) != nil {
		t.Fatal("接受超限data URI")
	}
	ed := New("")
	for i := 0; i < maxCachedImages; i++ {
		ed.images[fmt.Sprint(i)] = nil
	}
	called := false
	ed.SetReadImage(func(string) *ui.Bitmap { called = true; return nil })
	ed.wantImage("assets/a.png")
	if called {
		t.Fatal("超出缓存数量仍调用解码器")
	}
}

func TestOrderedToBulletFormat(t *testing.T) {
	ed, tt := newTest(t, "项目")
	ed.Format("ordered")
	tt.Frame()
	ed.Format("bullet")
	tt.Frame()
	if !strings.HasPrefix(ed.Markdown(), "- ") {
		t.Fatalf("有序列表无法切回无序: %q", ed.Markdown())
	}
}

func TestOrderedListShowsActualNumber(t *testing.T) {
	ed := New("98. 第一项\n99. 第二项\n100. 第三项")
	tt := ui.NewTester(ed.View, 320, 240)
	tt.Frame()
	if len(ed.lay.lines) != 3 {
		t.Fatalf("有序列表行数=%d", len(ed.lay.lines))
	}
	for i, ln := range ed.lay.lines {
		want := ui.Shape(fmt.Sprintf("%d. ", 98+i), ui.Font{Family: defaultFontFamily, Size: ed.fontSize, Weight: 400})
		if !reflect.DeepEqual(ln.markers, want) {
			t.Fatalf("第%d项前缀未使用真实序号%d", i, 98+i)
		}
		if advanceOf(ln.markers) > ln.prefix {
			t.Fatalf("多位序号覆盖正文: marker=%v prefix=%v", advanceOf(ln.markers), ln.prefix)
		}
		x, y, h := ed.PointForOffset(ln.origin)
		tt.ClickAt(x, y+h/2)
		a, _ := ed.Selection()
		if a != ln.origin {
			t.Fatalf("编号后行首命中=%d 期望%d", a, ln.origin)
		}
	}
}

func TestOriginalTypographyGeometry(t *testing.T) {
	ed := New("# 标题\n\n正文\n\n- 第一项\n- 第二项\n\n> 引用")
	ed.FontSize(14)
	ed.SetLineHeight(1.5)
	tt := ui.NewTester(ed.View, 1016, 600)
	tt.Frame()
	if ed.lay.blocks[0].x != 80 || ed.lay.blocks[0].w != 856 || ed.lay.blocks[0].y != 36 {
		t.Fatalf("正文未对齐原设计：%+v", ed.lay.blocks[0])
	}
	first, second := ed.lay.blocks[2], ed.lay.blocks[3]
	if abs32(second.y-first.y-first.h) > .1 {
		t.Fatal("相邻列表项出现段落间距")
	}
	quote := ed.lay.blocks[4]
	ln := ed.lay.lines[quote.lines[0]]
	if ln.y != quote.y+10 || ln.prefix != 19 {
		t.Fatal("引用内边距不匹配")
	}
}

func TestFontChangeRebuildsGlyphsAndComposition(t *testing.T) {
	ed, tt := newTest(t, "中文Wi宽度")
	tt.Compose("Wi", 1)
	before := append([]ui.Glyph(nil), ed.composeGlyphs...)
	ed.SetFontFamily("monospace")
	tt.Frame()
	if reflect.DeepEqual(before, ed.composeGlyphs) {
		t.Fatal("组合输入仍复用旧字体")
	}
	if len(ed.cache.text) == 0 {
		t.Fatal("更换字体后未重建缓存")
	}
	for key := range ed.cache.text {
		if key.face.family != "monospace" {
			t.Fatalf("残留旧字体缓存：%q", key.face.family)
		}
	}
	ln := ed.lay.lines[0]
	x, y, h := ed.PointForOffset(2)
	tt.ClickAt(x, y+h/2)
	a, _ := ed.Selection()
	if a != ln.origin+2 {
		t.Fatalf("新字体字形命中=%d", a)
	}
}

func TestAlignedTableAndInlineMarksHit(t *testing.T) {
	ed, tt := newTest(t, "|左|右|\n|:---|---:|\n|很长的左侧文字|x|\n\n甲==高亮==H~2~x^2^")
	table := ed.lay.blocks[0]
	if len(table.cols) != 3 || len(table.rows) != 3 {
		t.Fatal("缺少表格内部边界")
	}
	for i, r := range []rune(ed.Text()) {
		if r == '\n' || r == '\t' {
			continue
		}
		x, y, h := ed.PointForOffset(i)
		tt.ClickAt(x, y+h/4)
		a, _ := ed.Selection()
		if a != i {
			t.Fatalf("对齐表格/行内样式偏移%d命中%d", i, a)
		}
	}
}

func TestUnsupportedCardsIdentifyContent(t *testing.T) {
	for _, tc := range []struct{ source, label string }{
		{"```mermaid\nnosuchdiagram\nA->>B: 你好\n```", "Mermaid"},
		{"$$\\nosuchcommand{x}$$", "数学公式"},
		{"<custom-element>内容</custom-element>", "HTML"},
	} {
		ed, tt := newTest(t, tc.source)
		tt.Frame()
		if !strings.Contains(ed.lay.lines[0].text, tc.label) || ed.lay.blocks[0].w > 300 {
			t.Fatalf("占位卡片未简洁标明类型：%+v", ed.lay.blocks[0])
		}
		if ed.Markdown() != tc.source {
			t.Fatal("占位改变原文")
		}
	}
}

func TestSelectionAnchorOnlyForVisibleSelection(t *testing.T) {
	ed, tt := newTest(t, "第一段文字\n\n第二段文字")
	tt.Frame()
	if _, _, _, _, ok := ed.SelectionAnchor(); ok {
		t.Fatal("折叠光标不应浮出格式栏")
	}
	ed.SetSelection(1, 3)
	tt.Frame()
	x, y, h, w, ok := ed.SelectionAnchor()
	wantX, wantY, wantH := ed.PointForOffset(1)
	if !ok || x != wantX || y != wantY || h != wantH || w <= 0 {
		t.Fatalf("选区锚点错误：%v %v %v %v %v", x, y, h, w, ok)
	}
}

func TestNestedListAndMathLayout(t *testing.T) {
	ed, tt := newTest(t, "1. 走了 $s = vt$ 里\n\n- 外层\n  - 内层\n- [x] 完成\n")
	tt.Frame()
	blocks := ed.lay.blocks
	if len(blocks) != 4 {
		t.Fatalf("块数=%d", len(blocks))
	}
	if blocks[1].indent != 0 || blocks[2].indent != nestIndent || !blocks[1].bullet || !blocks[2].bullet || !blocks[3].checked {
		t.Fatalf("列表层级或标记错误：%+v", blocks)
	}
	// 有序与无序是两个列表，中间保留块间距；同一列表的各项紧挨。
	if gap := blocks[1].y - blocks[0].y - blocks[0].h; abs32(gap-blockGap) > .01 {
		t.Fatalf("不同列表间距=%v", gap)
	}
	if gap := blocks[2].y - blocks[1].y - blocks[1].h; abs32(gap) > .01 {
		t.Fatalf("同一列表间距=%v", gap)
	}
	if ed.Text() != "走了 s = vt 里\n外层\n内层\n完成" {
		t.Fatalf("公式或嵌套列表露出源码：%q", ed.Text())
	}
}

func TestCodeBlockSkipsTrailingBlankLineAndColorsStrings(t *testing.T) {
	ed, tt := newTest(t, "```go\nfmt.Println(\"字\") // 注释\n```\n")
	tt.Frame()
	lines := ed.lay.blocks[0].lines
	if len(lines) != 1 {
		t.Fatalf("代码块行数=%d", len(lines))
	}
	inks := map[inkKind]bool{}
	for _, span := range ed.lay.lines[lines[0]].spans {
		inks[span.ink] = true
	}
	if !inks[inkText] || !inks[inkAccent] || !inks[inkMuted] {
		t.Fatalf("代码着色缺失：%+v", inks)
	}
}

func TestTabShiftsListLevelAndMovesBetweenCells(t *testing.T) {
	ed, tt := newTest(t, "- 一\n- 二\n")
	tt.Frame()
	ed.SetSelection(2, 2)
	if !ed.tabKey(false) || ed.Markdown() != "- 一\n  - 二\n" {
		t.Fatalf("Tab 未缩进列表项：%q", ed.Markdown())
	}
	if !ed.tabKey(true) || ed.Markdown() != "- 一\n- 二\n" {
		t.Fatalf("Shift+Tab 未提升列表项：%q", ed.Markdown())
	}
	ed, tt = newTest(t, "| 甲 | 乙 |\n| --- | --- |\n| 1 | 2 |\n")
	tt.Frame()
	ed.SetSelection(0, 0)
	ed.tabKey(false)
	if a, _ := ed.Selection(); a != 2 {
		t.Fatalf("Tab 未进入下一格：%d", a)
	}
	ed.tabKey(true)
	if a, _ := ed.Selection(); a != 0 {
		t.Fatalf("Shift+Tab 未回到上一格：%d", a)
	}
	ed.SetSelection(6, 6)
	ed.tabKey(false)
	if ed.Text() != "甲\t乙\n1\t2\n\t" {
		t.Fatalf("末格 Tab 未追加行：%q", ed.Text())
	}
	ed, tt = newTest(t, "正文")
	tt.Frame()
	if ed.tabKey(false) {
		t.Fatal("普通段落不应吞掉 Tab")
	}
}

func TestFormatInsertsTableCodeBlockAndRule(t *testing.T) {
	for name, kind := range map[string]string{"table": "| --- |", "codeblock": "```", "hr": "---"} {
		ed, tt := newTest(t, "正文")
		tt.Frame()
		ed.SetSelection(1, 1)
		ed.Format(name)
		tt.Frame()
		if !strings.HasPrefix(ed.Markdown(), "正文\n\n") || !strings.Contains(ed.Markdown(), kind) || !ed.Changed() {
			t.Fatalf("%s 未插入：%q", name, ed.Markdown())
		}
		if a, _ := ed.Selection(); a <= 2 {
			t.Fatalf("%s 插入后光标未进入新块：%d", name, a)
		}
		ed.Undo()
		if name == "hr" {
			ed.Undo()
		}
		if ed.Markdown() != "正文" {
			t.Fatalf("%s 撤销未恢复：%q", name, ed.Markdown())
		}
	}
}

func TestReadingWidthAndTypewriterSettings(t *testing.T) {
	ed := New(strings.Repeat("一段正文。\n\n", 80))
	tt := ui.NewTester(ed.View, 1400, 500)
	tt.Frame()
	if w := ed.lay.blocks[0].w; w != readingWidth {
		t.Fatalf("默认阅读宽度=%v", w)
	}
	ed.SetReadingWidth(600)
	tt.Frame()
	if b := ed.lay.blocks[0]; b.w != 600 || abs32(b.x-400) > .5 {
		t.Fatalf("阅读宽度未生效或未居中：%+v", b)
	}
	ed.SetReadingWidth(10)
	tt.Frame()
	if ed.lay.blocks[0].w != 600 {
		t.Fatal("非法宽度不应生效")
	}
	plain := ed.lay.height
	ed.SetTypewriter(true)
	ed.SetSelection(200, 200)
	tt.Frame()
	tt.Frame()
	_, y, h := ed.PointForOffset(200)
	if center := y + h/2 - ed.scroll.Y; abs32(center-250) > 2 {
		t.Fatalf("打字机模式光标未居中：%v", center)
	}
	if ed.lay.height <= plain {
		t.Fatal("打字机模式应在文末留出余量")
	}
	ed.SetFocusMode(true)
	tt.Frame()
}

func TestDoubleClickRawRequestsSourceEditAndReplaceRaw(t *testing.T) {
	ed, tt := newTest(t, "前文\n\n```mermaid\nsequenceDiagram\nA->>B: 旧\n```\n\n后文\n")
	tt.Frame()
	var gotIndex int
	var gotSource string
	ed.SetEditRaw(func(index int, source string) { gotIndex, gotSource = index, source })
	// 测试器不产生双击计数，直接走双击时调用的入口。
	if !ed.requestRawEdit(ed.blockOrigin(1)) || ed.requestRawEdit(0) {
		t.Fatal("只有占位块应请求源码编辑")
	}
	if gotIndex != 1 || !strings.Contains(gotSource, "A->>B: 旧") {
		t.Fatalf("双击占位块未请求编辑：%d %q", gotIndex, gotSource)
	}
	if !ed.ReplaceRaw(1, "```mermaid\nsequenceDiagram\nA->>B: 新\n```") || !strings.Contains(ed.Markdown(), "A->>B: 新") {
		t.Fatalf("替换占位原文错误：%q", ed.Markdown())
	}
	if !ed.ReplaceRaw(1, "改成**正文**") || !strings.Contains(ed.Text(), "改成正文") {
		t.Fatalf("改成受支持语法后应成为正文：%q", ed.Text())
	}
	if ed.ReplaceRaw(1, "x") {
		t.Fatal("非占位块不应被替换")
	}
	ed.Undo()
	ed.Undo()
	if !strings.Contains(ed.Markdown(), "A->>B: 旧") {
		t.Fatalf("撤销未恢复原文：%q", ed.Markdown())
	}
}

func TestDisplayMathAndFlowchartRenderInPlace(t *testing.T) {
	source := "前文\n\n$$\nQ_n = Q \\cdot r^n\n$$\n\n```mermaid\ngraph LR\n  A[叶柄] --> B[主脉]\n```\n\n后文\n"
	ed, tt := newTest(t, source)
	tt.Frame()
	math, chart := ed.lay.blocks[1], ed.lay.blocks[2]
	if math.math == nil || math.h < math.math.Ascent+math.math.Descent || len(math.lines) != 1 || ed.lay.lines[math.lines[0]].text != "" {
		t.Fatalf("块级公式未排版：%+v", math)
	}
	if chart.diagram == nil || chart.diagram.Width > chart.w+.5 || chart.h < chart.diagram.Height {
		t.Fatalf("流程图未排版或超出正文宽度：%+v", chart)
	}
	if ed.Markdown() != source || ed.Text() != "前文\n￼\n￼\n后文" {
		t.Fatalf("渲染不应改变原文与坐标：%q", ed.Text())
	}
	// 渲染后的块仍是一个整体：点在上面只落在块的前后，输入不会写进源码。
	x, y, w, h, _ := ed.BlockRect(2)
	tt.ClickAt(x+w/2, y+h/4)
	tt.Type("误输入")
	if ed.Markdown() != source {
		t.Fatalf("渲染块被直接改写：%q", ed.Markdown())
	}
	tt.Image()
}

func TestInlineMathRendersUntilCaretEntersIt(t *testing.T) {
	ed, tt := newTest(t, "分式 $\\frac{a}{b}$ 之后\n\n普通一行\n")
	ed.SetSelection(0, 0)
	tt.Frame()
	first, plain := ed.lay.lines[0], ed.lay.lines[1]
	var box *paintSpan
	for i := range first.spans {
		if first.spans[i].math != nil {
			box = &first.spans[i]
		}
	}
	if box == nil {
		t.Fatalf("行内公式未排成盒子：%+v", first.spans)
	}
	if g := box.glyphs[0]; g.Cluster != 3 || g.Runes != len([]rune(`\frac{a}{b}`)) || g.Advance != box.math.Width {
		t.Fatalf("公式占位字形与文档坐标不一致：%+v", g)
	}
	if first.height <= plain.height {
		t.Fatal("高出一行的公式应把所在行撑开")
	}
	// 光标停在公式两端时保持排版，移进内部后展开成源码。
	end := 3 + len([]rune(`\frac{a}{b}`))
	for _, at := range []int{3, end} {
		ed.SetSelection(at, at)
		tt.Frame()
		if ln := ed.lay.lines[0]; len(ln.spans) < 2 || ln.spans[1].math == nil {
			t.Fatalf("光标在公式边上（%d）时不应展开", at)
		}
	}
	ed.SetSelection(5, 5)
	tt.Frame()
	for _, span := range ed.lay.lines[0].spans {
		if span.math != nil {
			t.Fatal("光标进入公式后应显示源码")
		}
	}
	if !strings.Contains(ed.lay.lines[0].text, `\frac{a}{b}`) {
		t.Fatalf("源码未显示：%q", ed.lay.lines[0].text)
	}
	tt.Type("x")
	if ed.Markdown() != "分式 $\\fxrac{a}{b}$ 之后\n\n普通一行\n" {
		t.Fatalf("公式内部输入错误：%q", ed.Markdown())
	}
	tt.Image()
}
