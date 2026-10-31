package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"image"
	"io"
	"net/http"
	"strings"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

const (
	remoteLimit  = 2 << 20 // 单张远程图最多 2MB
	remoteTotal  = 8 << 20
	remoteActive = 3
)

func (e *Editor) ensureImages() {
	for _, b := range e.doc.Blocks() {
		if b.Kind == richtext.Image {
			e.wantImage(b.URL)
		}
		for _, r := range b.Runs {
			if r.Image != nil {
				e.wantImage(r.Image.URL)
			}
		}
	}
}

func (e *Editor) wantImage(url string) {
	if url == "" || len(e.images) >= maxCachedImages {
		return
	}
	if _, ok := e.images[url]; ok {
		return
	}
	if _, ok := e.fetched[url]; ok {
		return
	}
	if safeURL(url, true) {
		if e.remoteN >= remoteActive || e.remoteB >= remoteTotal {
			return
		}
		e.fetched[url] = struct{}{}
		e.remoteN++
		// 后台只持有固定的 client、channel；文档、位图和预算由 View 消化。
		go fetchImage(e.client, e.imgCh, url, e.notifyImage)
		return
	}
	e.fetched[url] = struct{}{}
	if e.read != nil {
		// 本地路径只能交给桌面的授权回调，控件自己从不打开磁盘路径。
		if pic := e.read(url); pictureAllowed(pic) {
			e.images[url] = pic
		}
	} else if bm := decodeDataURI(url); bm != nil {
		e.images[url] = BitmapPicture(bm)
	}
}

func (e *Editor) notifyImage() {
	e.wakeMu.RLock()
	fn := e.wake
	e.wakeMu.RUnlock()
	if fn != nil {
		fn()
	}
}

type imageResult struct {
	url  string
	data []byte
}

const maxCachedImages = 32

const maxImagePixels = 16 << 20

// drainImages 在 UI 线程解码，后台从不读写图片缓存或布局。
func (e *Editor) drainImages(c *ui.Context) {
	for {
		select {
		case res := <-e.imgCh:
			e.remoteN--
			if len(res.data) == 0 || e.remoteB+len(res.data) > remoteTotal || len(e.images) >= maxCachedImages {
				continue
			}
			e.remoteB += len(res.data)
			pic := BitmapPicture(decodeImage(res.data))
			if pic == nil {
				pic = DecodeSVG(res.data)
			}
			if pic != nil {
				e.images[res.url] = pic
				if c != nil {
					c.Invalidate()
				}
			}
		default:
			return
		}
	}
}

func fetchImage(client *http.Client, ch chan<- imageResult, url string, wake func()) {
	result := imageResult{url: url}
	defer func() { ch <- result; wake() }()
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, remoteLimit+1))
	if err != nil || len(data) == 0 || len(data) > remoteLimit {
		return
	}
	result.data = data
}

func decodeImage(raw []byte) *ui.Bitmap {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil
	}
	bm, err := ui.DecodeBitmap(raw)
	if err != nil {
		return nil
	}
	return bm
}

func decodeDataURI(uri string) *ui.Bitmap {
	if !strings.HasPrefix(strings.ToLower(uri), "data:image/") {
		return nil
	}
	head, body, ok := strings.Cut(uri, ",")
	if !ok || !strings.HasSuffix(strings.ToLower(head), ";base64") || len(body) > base64.StdEncoding.EncodedLen(remoteLimit) {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil || len(raw) > remoteLimit {
		return nil
	}
	return decodeImage(raw)
}
