package nativeeditor

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

func TestRemoteImageOverBudgetDoesNotLoop(t *testing.T) {
	pngBytes := tinyPNG(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	urlA := srv.URL + "/over.png"
	urlKeep := srv.URL + "/keep.png"
	ed := New("![占位](" + urlKeep + ")\n\n![超预算](" + urlA + ")\n")
	ed.client = srv.Client()
	// 预置已缓存图片使剩余远程字节预算刚好不足以容纳 pngBytes，但尚未达到 remoteTotal，允许发起一次请求。
	bm, err := ui.DecodeBitmap(pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	used := remoteTotal - max(1, len(pngBytes)/2)
	ed.images[urlKeep] = BitmapPicture(bm)
	ed.fetched[urlKeep] = struct{}{}
	ed.remoteBytes[urlKeep] = used
	ed.remoteB = used

	t0 := time.Now()
	ed.ensureImages(t0)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ed.drainImages(nil, t0)
		if ed.remoteN == 0 && hits.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ed.ImageReady(urlA) {
		t.Fatal("超预算图片不应进入缓存")
	}
	if _, ok := ed.failedAt[urlA]; !ok {
		t.Fatal("超预算图片应记入退避表 failedAt")
	}
	if ed.imagesRetryAt.IsZero() {
		t.Fatal("超预算图片应设置下次重试时刻 imagesRetryAt")
	}
	// 退避期内重复刷新帧不应再次发 HTTP 请求。
	for i := 1; i <= 5; i++ {
		ed.ensureImages(t0.Add(time.Duration(i) * time.Second))
		ed.drainImages(nil, t0.Add(time.Duration(i)*time.Second))
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("超预算后退避期内不应无限重发请求，实际命中 %d 次", got)
	}

	// 删除占位图片释放空间后，应回收预算并允许立即重试加载成功。
	ed.doc.Delete(0, 2)
	t1 := t0.Add(2 * time.Second)
	ed.ensureImages(t1)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ed.drainImages(nil, t1)
		if ed.ImageReady(urlA) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ed.ImageReady(urlA) {
		t.Fatalf("释放空间后应重试并加载成功，hits=%d remoteB=%d", hits.Load(), ed.remoteB)
	}
}

func TestEnsureImagesScansTableCellImages(t *testing.T) {
	pngBytes := tinyPNG(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cell.png" {
			hits.Add(1)
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cellURL := srv.URL + "/cell.png"
	ed := New("| 名称 | 图标 |\n| --- | --- |\n| 项一 | ![图标](" + cellURL + ") |\n")
	ed.client = srv.Client()
	t0 := time.Now()
	ed.ensureImages(t0)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ed.drainImages(nil, t0)
		if ed.ImageReady(cellURL) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ed.ImageReady(cellURL) || hits.Load() != 1 {
		t.Fatalf("表格内行内图片未被扫描加载：ready=%v hits=%d", ed.ImageReady(cellURL), hits.Load())
	}
}

func TestEditableSpanAfterTableAndReferenceDef(t *testing.T) {
	md := "[ref]: https://example.com\n\n| 列一 | 列二 |\n| --- | --- |\n| 甲 | 乙 |\n\n正文 $x+y$ 尾注[^1] 图![图](a.png) 目标词。\n\n[^1]: 说明\n"
	ed := New(md)

	// 表格单元格内文字应可编辑。
	cellAt := offsetOf(t, ed, "甲")
	if !ed.editableSpan(cellAt, cellAt+1) {
		t.Fatal("表格内普通文字应可编辑")
	}
	// 表格和引用定义之后的行内公式、脚注引用、行内图片必须精确识别为不可编辑。
	mathAt := offsetOf(t, ed, "x+y")
	if ed.editableSpan(mathAt, mathAt+3) {
		t.Fatal("表格和引用定义后的行内公式应不可编辑")
	}
	targetAt := offsetOf(t, ed, "目标词")
	if !ed.editableSpan(targetAt, targetAt+3) {
		t.Fatal("表格和引用定义后的普通正文应可编辑")
	}
	// 替换应准确命中“目标词”且不破坏文档结构。
	if n := ed.Replace("目标词", "新词", false); n != 1 {
		t.Fatalf("期望替换 1 处，实际 %d", n)
	}
	if !strings.Contains(ed.Text(), "新词") || !strings.Contains(ed.Markdown(), "$x+y$") {
		t.Fatalf("替换结果异常：text=%q md=%q", ed.Text(), ed.Markdown())
	}
}

func TestFoldedDetailsDoesNotInflateHeightAndKeepsOffsets(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&body, "<p>这是折叠块内部第 %d 段很长的隐藏正文内容，用来验证折叠时不会被压成 40px 折行撑高文档。</p>", i)
	}
	md := "前文\n\n<details><summary>更多细节</summary>" + body.String() + "</details>\n\n后文内容\n"
	ed := New(md)
	ed.FontSize(16)
	tt := ui.NewTester(ed.View, 680, 500)
	tt.Frame()

	foldedDocH := ed.lay.height
	var detailsFrame frame
	for _, f := range ed.lay.frames {
		if f.details {
			detailsFrame = f
		}
	}
	if !detailsFrame.folded {
		t.Fatal("details 应默认收起")
	}
	if detailsFrame.h > 40 {
		t.Fatalf("收起时 details 卡片高度应为标题高度(32)，实际 %v", detailsFrame.h)
	}
	// 收起时后文的 origin 必须与可见纯文本中“后文内容”的位置完全一致。
	afterBox := blockOf(t, ed, "后文内容")
	wantOrigin := offsetOf(t, ed, "后文内容")
	if afterBox.origin != wantOrigin {
		t.Fatalf("折叠块之后偏移错位：got %d want %d", afterBox.origin, wantOrigin)
	}
	// 上下方向键从“前文”向下移动应直接跳过折叠内容落到“后文内容”，再向上回到“前文”。
	ed.Focus()
	ed.SetSelection(0, 0)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	if at, _ := ed.Selection(); at < wantOrigin {
		t.Fatalf("收起状态下按 Down 应跳过隐藏正文落到后文(%d)，实际 %d", wantOrigin, at)
	}
	tt.Key(0, ui.KeyUp)
	if at, _ := ed.Selection(); at >= wantOrigin {
		t.Fatalf("收起状态下按 Up 应跳过隐藏正文回到前文，实际 %d", at)
	}

	// 展开后文档高度应严格大于收起高度。
	tt.ClickAt(detailsFrame.x+20, detailsFrame.y+16)
	tt.Frame()
	if !(foldedDocH < ed.lay.height) {
		t.Fatalf("折叠文档高度(%v)应严格小于展开高度(%v)", foldedDocH, ed.lay.height)
	}
}

func TestSourceModeCmdEnterAndFormatDirty(t *testing.T) {
	src := "第一行\n第二行\n"
	ed := NewPlainText(src)
	tt := ui.NewTester(ed.View, 640, 480)
	tt.Frame()
	ed.Changed()

	// 源码模式下 Cmd+Enter 不应拆分出新块，也不应修改文档或置脏标记。
	ed.SetSelection(1, 1)
	tt.Key(ui.Cmd, ui.KeyEnter)
	if len(ed.doc.Blocks()) != 1 {
		t.Fatalf("源码模式 Cmd+Enter 破坏了单块不变式：blocks=%d", len(ed.doc.Blocks()))
	}
	if ed.Markdown() != src || ed.Changed() {
		t.Fatalf("源码模式 Cmd+Enter 不应改文档或置脏：md=%q", ed.Markdown())
	}

	// 源码模式或无实际变更的 Format 不应误置 dirty。
	ed.SetSelection(0, 3)
	ed.Format("bold")
	if ed.Changed() {
		t.Fatal("源码模式下 Format(bold) 未修改文档，不应置脏标记")
	}

	// 所见即所得模式下折叠光标切换待输入格式或重复设为 paragraph 也不应误置 dirty。
	we := New("普通段落")
	we.Changed()
	we.SetSelection(1, 1)
	we.Format("bold")
	if we.Changed() {
		t.Fatal("折叠光标 Format(bold) 未修改文档，不应置脏标记")
	}
	we.Format("paragraph")
	if we.Changed() {
		t.Fatal("已是段落再 Format(paragraph) 未修改文档，不应置脏标记")
	}
	// 有选区加粗确实修改文档时应置 dirty。
	we.SetSelection(0, 2)
	we.Format("bold")
	if !we.Changed() {
		t.Fatal("实际修改格式后应置脏标记")
	}
}

func TestSourceModeCacheKeyAndModeSwitchClearsOtherCache(t *testing.T) {
	ed := New("# 标题\n\n正文段落\n")
	tt := ui.NewTester(ed.View, 720, 480)
	tt.Frame()
	if !ed.wysiwygLaid || len(ed.wysiwygLay.lines) == 0 {
		t.Fatal("初始应缓存所见即所得排版")
	}
	ed.SetSourceMode(true)
	if ed.wysiwygLaid || len(ed.wysiwygLay.lines) != 0 {
		t.Fatal("切到源码模式应释放所见即所得排版缓存")
	}
	syntaxCalls := 0
	ed.SetSourceSyntax(func(text string) []SourceSpan {
		syntaxCalls++
		return []SourceSpan{{Start: 0, End: 2, Kind: 1}}
	})
	tt.Frame()
	if !ed.sourceLaid || ed.sourceKey.docRev != ed.doc.Revision() {
		t.Fatalf("源码排版缓存键应记录 docRev：key=%d doc=%d", ed.sourceKey.docRev, ed.doc.Revision())
	}
	beforeCalls := syntaxCalls
	for range 3 {
		tt.Frame()
	}
	if syntaxCalls != beforeCalls {
		t.Fatalf("未编辑时不应重复调用 sourceSyntax：%d → %d", beforeCalls, syntaxCalls)
	}

	// 切回所见即所得模式应释放源码缓存并重新排版。
	ed.SetSourceMode(false)
	if ed.sourceLaid || len(ed.sourceLay.lines) != 0 {
		t.Fatal("切回所见即所得应释放源码排版缓存")
	}
	tt.Frame()
	if !ed.wysiwygLaid || len(ed.lay.lines) == 0 {
		t.Fatal("切回所见即所得后应重新完成排版")
	}
}

func TestFontSizeEqualShortCircuitKeepsCache(t *testing.T) {
	ed := New("缓存命中测试文本")
	tt := ui.NewTester(ed.View, 640, 480)
	tt.Frame()
	cachedBefore := len(ed.cache.text)
	if cachedBefore == 0 {
		t.Fatal("首帧应填充字形缓存")
	}
	laidBefore := ed.flowCount
	// 重复设置相同字号不应清空缓存或触发重排。
	ed.FontSize(ed.fontSize)
	if len(ed.cache.text) != cachedBefore {
		t.Fatalf("相同字号清空了字形缓存：%d → %d", cachedBefore, len(ed.cache.text))
	}
	tt.Frame()
	if ed.flowCount != laidBefore {
		t.Fatalf("相同字号触发了多余重排：%d → %d", laidBefore, ed.flowCount)
	}
	// 下限 clamp 后相等同样应短路。
	ed.FontSize(8)
	tt.Frame()
	cachedAt8 := len(ed.cache.text)
	ed.FontSize(4)
	if len(ed.cache.text) != cachedAt8 {
		t.Fatal("clamp 后相等的字号不应清空字形缓存")
	}
}

func TestLongLineWrapAvoidsCachePollution(t *testing.T) {
	longLine := strings.Repeat("alpha beta gamma delta ", 120)
	ed := NewPlainText(longLine)
	tt := ui.NewTester(ed.View, 420, 480)
	tt.Frame()

	got := strings.Join(sourceLineTexts(ed), "")
	if got != longLine {
		t.Fatalf("长行折行内容丢失或重复：len %d vs %d", len(got), len(longLine))
	}
	if len(ed.lay.lines) < 20 {
		t.Fatalf("长行应被折成多行，实际 %d 行", len(ed.lay.lines))
	}
	// 探针与二分前缀不污染 shapeCache.text，缓存条目应接近实际折出的行数，不会塞入海量递减后缀。
	if len(ed.cache.text) > len(ed.lay.lines)+8 {
		t.Fatalf("长行折行污染了 shapeCache：lines=%d cache=%d", len(ed.lay.lines), len(ed.cache.text))
	}
}

func TestTableAlignedEmptyLeftClickAndVerticalSameColumn(t *testing.T) {
	md := "上方段落文本内容较长\n\n| 左列 | 居中列 | 右对齐列 |\n| :--- | :---: | ---: |\n| A1 | B1 | C1 |\n| A2 | B2 | C2 |\n\n下方段落文本内容较长\n"
	ed, tt := newTest(t, md)
	tt.SetSize(720, 600)
	tt.Frame()

	var table blockBox
	for _, b := range ed.lay.blocks {
		if b.kind == richtext.TableBlock {
			table = b
			break
		}
	}
	if len(table.cols) != 4 {
		t.Fatalf("期望 3 列表格有 4 条列边界，实际 %v", table.cols)
	}
	// 点击右对齐列（第 2 列）数据行文字左侧的空白区域，应落在本列 C1 开头而非前一列 B1。
	c1Off := offsetOf(t, ed, "C1")
	_, c1Y, c1H := ed.PointForOffset(c1Off)
	clickX := table.x + table.cols[2] + 4
	tt.ClickAt(clickX, c1Y+c1H/2)
	if a, _ := ed.Selection(); a != c1Off {
		t.Fatalf("点击右对齐单元格左侧空白误落：got %d want %d (C1)", a, c1Off)
	}

	// 在表格中从第 1 行第 1 列（B1）按 Down 应移到第 2 行第 1 列（B2），而非右侧的 C1。
	b1Off := offsetOf(t, ed, "B1")
	b2Off := offsetOf(t, ed, "B2")
	ed.SetSelection(b1Off, b1Off)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	if at, _ := ed.Selection(); at < b2Off || at > b2Off+2 {
		t.Fatalf("表格内按 Down 应同列跨行到 B2(%d..%d)，实际 %d", b2Off, b2Off+2, at)
	}
	// 再按 Up 应回到 B1。
	tt.Key(0, ui.KeyUp)
	if at, _ := ed.Selection(); at < b1Off || at > b1Off+2 {
		t.Fatalf("表格内按 Up 应同列跨行回 B1(%d..%d)，实际 %d", b1Off, b1Off+2, at)
	}
}

func TestVerticalGoalXAndCrossIndentScreenX(t *testing.T) {
	md := "第一行比较长的普通正文内容\n\n\n> 引用块里的比较长的正文内容\n\n第三行比较长的普通正文内容\n"
	ed, tt := newTest(t, md)
	tt.SetSize(720, 600)
	tt.Frame()

	// 在第一行靠右位置开始连续向下，经过引用块再到第三行，应保持目标横坐标。
	startOff := 8
	startX, _, _ := ed.PointForOffset(startOff)
	ed.SetSelection(startOff, startOff)
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	midOff, _ := ed.Selection()
	midX, _, _ := ed.PointForOffset(midOff)
	if abs32(midX-startX) > 24 {
		t.Fatalf("跨缩进块应按屏幕 X 对齐：startX=%v midX=%v", startX, midX)
	}
	tt.Key(0, ui.KeyDown)
	endOff, _ := ed.Selection()
	endX, _, _ := ed.PointForOffset(endOff)
	if abs32(endX-startX) > 24 {
		t.Fatalf("连续上下移动应记住 goalX：startX=%v endX=%v", startX, endX)
	}
}

func TestBlockRectPointForOffsetAndFrameTaskAndCompose(t *testing.T) {
	// 1. 含 ReferenceDef 时 BlockRect 按 b.index 查找，且 PointForOffset 不污染 e.width。
	ed := New("[ref]: https://example.com\n\n正文块\n")
	if ed.width != 0 {
		t.Fatalf("新建编辑器 width 应为 0，实际 %v", ed.width)
	}
	ed.PointForOffset(1)
	if ed.width != 0 {
		t.Fatalf("PointForOffset 不应污染 e.width，实际 %v", ed.width)
	}
	bi := ed.doc.BlockIndexAt(offsetOf(t, ed, "正文块"))
	if _, _, w, h, ok := ed.BlockRect(bi); !ok || w <= 0 || h <= 0 {
		t.Fatalf("含 ReferenceDef 时 BlockRect(%d) 应命中正文块：ok=%v w=%v h=%v", bi, ok, w, h)
	}

	// 2. LayoutSnapshot 遇到零尺寸图片不产生 NaN/Inf。
	ed.images["zero.png"] = &Picture{}
	ed.lay.blocks = append(ed.lay.blocks, blockBox{kind: richtext.Image, image: &Picture{}, w: 100, h: 40})
	snap := ed.LayoutSnapshot()
	for _, img := range snap.Images {
		if math.IsNaN(float64(img.Width)) || math.IsInf(float64(img.Width), 0) {
			t.Fatalf("LayoutSnapshot 产生非法图片尺寸：%+v", img)
		}
	}

	// 3. 首块非正文的任务项，taskAt 应能命中 frame 上的复选框。
	taskEd, taskTT := newTest(t, "- [ ] ```go\n      x := 1\n      ```\n")
	taskTT.Frame()
	var taskFrame frame
	foundFrame := false
	for _, f := range taskEd.lay.frames {
		if f.marker && f.kind == richtext.Task {
			taskFrame, foundFrame = f, true
		}
	}
	if foundFrame {
		if _, ok := taskEd.taskAt(taskFrame.x+6, taskFrame.lineY+taskFrame.lineH/2); !ok {
			t.Fatalf("taskAt 应命中 frame 上的任务复选框：%+v", taskFrame)
		}
	}

	// 4. 点击编辑器取消 IME 预编辑应同时清空 comCaret，且标题行 IME 继承标题字号。
	imeEd, imeTT := newTest(t, "# 标题文字\n\n正文文字\n")
	imeEd.SetSelection(2, 2)
	imeTT.Frame()
	imeTT.Compose("测试", 1)
	if len(imeEd.composeGlyphs) == 0 || imeEd.composeMetrics.Ascent <= imeEd.cache.metrics(imeEd.fontSize, false, false).Ascent {
		t.Fatalf("标题行 IME 预编辑应继承标题字号：ascent=%v body=%v", imeEd.composeMetrics.Ascent, imeEd.cache.metrics(imeEd.fontSize, false, false).Ascent)
	}
	if imeEd.comCaret != 1 {
		t.Fatalf("组合光标应为 1，实际 %d", imeEd.comCaret)
	}
	x, y, h := imeEd.PointForOffset(0)
	imeTT.ClickAt(x, y+h/2)
	if imeEd.compose != "" || imeEd.comCaret != 0 {
		t.Fatalf("点击应调 cancelCompose 清空 compose 与 comCaret：compose=%q caret=%d", imeEd.compose, imeEd.comCaret)
	}
}

func TestLocalWindowWordAndClusterBounds(t *testing.T) {
	// 构造含超长单词、多行段落和 ZWJ emoji 的文档，核对局部窗口边界计算与全量语义一致。
	longWord := strings.Repeat("a", 400)
	emoji := "👨\u200d👩\u200d👧\u200d👦"
	doc := "hello " + longWord + " world\n" + "前缀 " + emoji + " 后缀"
	ed, tt := newTest(t, doc)
	tt.Frame()

	longStart := len([]rune("hello "))
	longEnd := longStart + len([]rune(longWord))
	if got := ed.wordBound(longStart, 1); got != longEnd {
		t.Fatalf("超长单词向右词边界 = %d，期望 %d", got, longEnd)
	}
	if got := ed.wordBound(longEnd, -1); got != longStart {
		t.Fatalf("超长单词向左词边界 = %d，期望 %d", got, longStart)
	}
	emojiStart := offsetOf(t, ed, emoji)
	emojiEnd := emojiStart + len([]rune(emoji))
	if got := ed.clusterBound(emojiStart, 1); got != emojiEnd {
		t.Fatalf("ZWJ emoji 向右簇边界 = %d，期望 %d", got, emojiEnd)
	}
	if got := ed.clusterBound(emojiEnd, -1); got != emojiStart {
		t.Fatalf("ZWJ emoji 向左簇边界 = %d，期望 %d", got, emojiStart)
	}
}
