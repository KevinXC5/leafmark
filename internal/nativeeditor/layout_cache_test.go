package nativeeditor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 120, G: 180, B: 90, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWysiwygLayoutReusedUntilInputsChange(t *testing.T) {
	md := "# 标题\n\n普通段落 $x+y$ 结束。\n\n<details><summary>展开</summary><p>隐藏内容</p></details>\n"
	ed := New(md)
	ed.FontSize(16)
	view := ui.NewTester(ed.View, 900, 640)
	view.Frame()
	if ed.flowCount == 0 {
		t.Fatal("首帧应执行一次排版")
	}

	// 无变化的连续帧与普通文字中移动光标不应触发重排。
	laid := ed.flowCount
	for range 3 {
		view.Frame()
	}
	ed.SetSelection(0, 0)
	view.Frame()
	ed.SetSelection(2, 2)
	view.Frame()
	if ed.flowCount != laid {
		t.Fatalf("无输入变化或普通光标移动不应重排：%d → %d", laid, ed.flowCount)
	}

	// 光标进入行内公式内部时应重排；在同一公式内移动光标不重排；移出后再次重排。
	mathStart := strings.Index(ed.Text(), "x+y")
	if mathStart < 0 {
		t.Fatalf("未找到公式文本：%q", ed.Text())
	}
	runeStart := len([]rune(ed.Text()[:mathStart]))
	ed.SetSelection(runeStart+1, runeStart+1)
	view.Frame()
	if ed.flowCount <= laid {
		t.Fatal("光标进入行内公式后未触发重排")
	}
	laid = ed.flowCount
	ed.SetSelection(runeStart+2, runeStart+2)
	view.Frame()
	if ed.flowCount != laid {
		t.Fatalf("在同一行内公式内部移动光标不应重排：%d → %d", laid, ed.flowCount)
	}
	ed.SetSelection(0, 0)
	view.Frame()
	if ed.flowCount <= laid {
		t.Fatal("光标离开行内公式后未触发重排")
	}

	// 编辑文本、改宽度、改字号、改折叠状态都应触发重排。
	laid = ed.flowCount
	ed.HandleInput(nil, ui.InputEvent{Kind: ui.InputText, Text: "新"})
	view.Frame()
	if ed.flowCount <= laid {
		t.Fatal("编辑正文后未触发重排")
	}

	laid = ed.flowCount
	view.SetSize(520, 640)
	view.Frame()
	if ed.flowCount <= laid {
		t.Fatal("宽度变化后未触发重排")
	}

	laid = ed.flowCount
	ed.FontSize(20)
	view.Frame()
	if ed.flowCount <= laid {
		t.Fatal("字号变化后未触发重排")
	}

	laid = ed.flowCount
	hiddenAt := strings.Index(ed.Text(), "隐藏内容")
	if hiddenAt < 0 {
		t.Fatalf("未找到折叠块正文：%q", ed.Text())
	}
	hiddenRune := len([]rune(ed.Text()[:hiddenAt]))
	ed.SetSelection(hiddenRune, hiddenRune)
	view.Frame()
	if ed.flowCount <= laid {
		t.Fatal("展开折叠块后未触发重排")
	}
}

func TestImageCachePrunesRemovedAndRetriesFailed(t *testing.T) {
	pngBytes := tinyPNG(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flaky.png":
			if hits.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes)
		case "/a.png", "/b.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// 1. 失败请求在退避期内不重复发起，超过退避期后重试并加载成功。
	flakyURL := srv.URL + "/flaky.png"
	ed := New("![图](" + flakyURL + ")\n")
	ed.client = srv.Client()
	t0 := time.Now()
	ed.ensureImages(t0)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ed.drainImages(nil, t0)
		if ed.remoteN == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ed.ImageReady(flakyURL) {
		t.Fatal("首次 500 响应不应标记图片就绪")
	}
	if ed.remoteB != 0 {
		t.Fatalf("失败请求不应占用远程字节预算：%d", ed.remoteB)
	}
	// 退避期内不应再次请求。
	ed.ensureImages(t0.Add(imageRetryDelay / 2))
	if hits.Load() != 1 {
		t.Fatalf("退避期内重复发起了请求：%d", hits.Load())
	}
	// 超过退避期后应重新请求并加载成功。
	t1 := t0.Add(imageRetryDelay + time.Second)
	ed.ensureImages(t1)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ed.drainImages(nil, t1)
		if ed.ImageReady(flakyURL) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ed.ImageReady(flakyURL) || hits.Load() != 2 {
		t.Fatalf("退避到期后未重试成功：ready=%v hits=%d", ed.ImageReady(flakyURL), hits.Load())
	}
	if ed.remoteB != len(pngBytes) {
		t.Fatalf("成功缓存后远程预算=%d，期望 %d", ed.remoteB, len(pngBytes))
	}

	// 2. 删除图片后应回收缓存条目与远程字节预算，并触发 imageRev 递增。
	prevRev := ed.imageRev
	ed.doc.Delete(0, ed.doc.Len())
	ed.ensureImages(t1.Add(time.Second))
	if ed.ImageReady(flakyURL) {
		t.Fatal("已删除的图片应从缓存回收")
	}
	if ed.remoteB != 0 {
		t.Fatalf("回收图片后远程字节预算应归零，实际 %d", ed.remoteB)
	}
	if ed.imageRev <= prevRev {
		t.Fatal("回收图片后未递增 imageRev")
	}
}
