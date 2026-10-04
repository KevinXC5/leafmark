//go:build verification && darwin

package desktop

import (
	"time"

	"github.com/ebitengine/purego/objc"
	mygo "github.com/egoist/mygo"
)

type nsPoint struct{ X, Y float64 }
type nsRect struct{ X, Y, W, H float64 }

// suiteTap 向窗口的事件队列投递一次左键按下与抬起，坐标为内容区内的点（左上角为原点）。
// gap 为按下与抬起之间的毫秒数：0 模拟触控板“轻触点击”，较大的值模拟按压点击，负数只移动指针。
func suiteTap(win *mygo.Window, x, y float64, gap int) {
	post := func(types ...uint) {
		mygo.RunOnMain(func() {
			w := objc.ID(win.NativeHandle())
			frame := objc.Send[nsRect](w.Send(objc.RegisterName("contentView")), objc.RegisterName("frame"))
			number := objc.Send[int](w, objc.RegisterName("windowNumber"))
			info := objc.ID(objc.GetClass("NSProcessInfo")).Send(objc.RegisterName("processInfo"))
			now := objc.Send[float64](info, objc.RegisterName("systemUptime"))
			app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
			for _, typ := range types {
				event := objc.Send[objc.ID](objc.ID(objc.GetClass("NSEvent")),
					objc.RegisterName("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
					typ, nsPoint{x, frame.H - y}, uint(0), now, number, objc.ID(0), 0, 1, float32(1))
				app.Send(objc.RegisterName("postEvent:atStart:"), event, false)
			}
		})
	}
	post(5) // NSEventTypeMouseMoved
	time.Sleep(150 * time.Millisecond)
	if gap < 0 {
		return
	}
	if gap == 0 {
		post(1, 2) // 按下与抬起连续入队
	} else {
		post(1)
		time.Sleep(time.Duration(gap) * time.Millisecond)
		post(2)
	}
	time.Sleep(300 * time.Millisecond)
}
