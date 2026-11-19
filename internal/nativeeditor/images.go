package nativeeditor

import (
	"bytes"
	"encoding/base64"
	"image"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"leafmark/internal/richtext"
)

const (
	remoteLimit     = 2 << 20 // 单张远程图最多 2MB
	remoteTotal     = 8 << 20
	remoteActive    = 3
	imageRetryDelay = 15 * time.Second
)

func (e *Editor) ensureImages(now time.Time) {
	rev := e.doc.Revision()
	if e.imagesDocRev == rev && (e.imagesRetryAt.IsZero() || now.Before(e.imagesRetryAt)) {
		return
	}
	var urls []string
	referenced := map[string]struct{}{}
	addURL := func(u string) {
		if u == "" {
			return
		}
		if _, seen := referenced[u]; !seen {
			referenced[u] = struct{}{}
			urls = append(urls, u)
		}
	}
	for _, b := range e.doc.Blocks() {
		if b.Kind == richtext.Image {
			addURL(b.URL)
		}
		for _, r := range b.Runs {
			if r.Image != nil {
				addURL(r.Image.URL)
			}
		}
		if b.Table != nil {
			for _, row := range b.Table.Rows {
				for _, cell := range row {
					for _, r := range cell.Runs {
						if r.Image != nil {
							addURL(r.Image.URL)
						}
					}
				}
			}
		}
	}
	// 回收已从文档删除的图片，释放缓存名额与远程字节预算。
	pruned := false
	for url := range e.images {
		if _, ok := referenced[url]; !ok {
			delete(e.images, url)
			if n, ok := e.remoteBytes[url]; ok {
				e.remoteB = max(0, e.remoteB-n)
				delete(e.remoteBytes, url)
			}
			pruned = true
		}
	}
	for url := range e.fetched {
		if _, ok := referenced[url]; !ok {
			delete(e.fetched, url)
		}
	}
	for url, at := range e.failedAt {
		if _, ok := referenced[url]; !ok {
			delete(e.failedAt, url)
			continue
		}
		// 缓存回收释放了空间或退避到期时，允许重新拉取。
		if pruned || !now.Before(at.Add(imageRetryDelay)) {
			delete(e.failedAt, url)
			delete(e.fetched, url)
		}
	}
	if pruned {
		e.imageRev++
	}
	for _, url := range urls {
		e.wantImage(url)
	}
	e.imagesRetryAt = e.nextImageRetry()
	e.imagesDocRev = rev
}

func (e *Editor) nextImageRetry() time.Time {
	var next time.Time
	for _, at := range e.failedAt {
		due := at.Add(imageRetryDelay)
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return next
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
			e.imageRev++
		}
	} else if bm := decodeDataURI(url); bm != nil {
		e.images[url] = BitmapPicture(bm)
		e.imageRev++
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
func (e *Editor) drainImages(c *ui.Context, now time.Time) {
	for {
		select {
		case res := <-e.imgCh:
			e.remoteN = max(0, e.remoteN-1)
			e.imagesDocRev = 0
			if len(res.data) == 0 {
				if e.failedAt == nil {
					e.failedAt = map[string]time.Time{}
				}
				e.failedAt[res.url] = now
				e.imagesRetryAt = e.nextImageRetry()
				continue
			}
			if e.remoteB+len(res.data) > remoteTotal || len(e.images) >= maxCachedImages {
				// 预算或缓存名额已满时保留 fetched 标记并进入退避，待退避到期或有图片被回收后再重试，避免每帧死循环拉取。
				if e.failedAt == nil {
					e.failedAt = map[string]time.Time{}
				}
				e.failedAt[res.url] = now
				e.imagesRetryAt = e.nextImageRetry()
				continue
			}
			pic := BitmapPicture(decodeImage(res.data))
			if pic == nil {
				pic = DecodeSVG(res.data)
			}
			if pic == nil {
				if e.failedAt == nil {
					e.failedAt = map[string]time.Time{}
				}
				e.failedAt[res.url] = now
				e.imagesRetryAt = e.nextImageRetry()
				continue
			}
			if e.remoteBytes == nil {
				e.remoteBytes = map[string]int{}
			}
			e.remoteB += len(res.data)
			e.remoteBytes[res.url] = len(res.data)
			delete(e.failedAt, res.url)
			e.images[res.url] = pic
			e.imageRev++
			if c != nil {
				c.Invalidate()
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
