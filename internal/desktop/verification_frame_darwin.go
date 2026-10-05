//go:build verification && darwin

package desktop

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// 验收可能在被其他窗口遮挡的桌面运行；主动请求 AppKit 绘制真实视图，
// 避免 CADisplayLink 暂停时只捕获启动帧。此入口不进入正式构建。
func refreshVerificationWindow(win *mygo.Window) {
	mygo.RunOnMain(func() {
		window := objc.ID(win.NativeHandle())
		if window == 0 {
			return
		}
		refreshVerificationView(window.Send(objc.RegisterName("contentView")))
	})
}

func refreshVerificationView(view objc.ID) {
	if view == 0 {
		return
	}
	children := view.Send(objc.RegisterName("subviews"))
	count := children.Send(objc.RegisterName("count"))
	for i := uintptr(0); i < uintptr(count); i++ {
		refreshVerificationView(children.Send(objc.RegisterName("objectAtIndex:"), i))
	}
	view.Send(objc.RegisterName("setNeedsDisplay:"), true)
	view.Send(objc.RegisterName("displayIfNeeded"))
}
