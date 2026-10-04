//go:build darwin

package desktop

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// 抬起事件推迟交付的秒数：只需晚于同一轮事件循环里的按下，远低于可感知的延迟。
const mouseUpDelay = 0.012

// installTapClickFix 修复触控板“轻触点击”在页面里不产生 click 的问题。
// 轻触产生的按下与抬起在同一轮事件循环内到达，WKWebView 会先把抬起交给页面，页面收到
// “抬起、按下”的顺序后不会触发点击。这里让网页视图推迟交付抬起事件，恢复“按下、抬起”的顺序。
func installTapClickFix() {
	view, base := objc.GetClass("MyGoWebView"), objc.GetClass("WKWebView")
	if view == 0 || base == 0 {
		return
	}
	library, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
	if err != nil {
		return
	}
	var implementation func(class objc.Class, name objc.SEL) uintptr
	purego.RegisterLibFunc(&implementation, library, "class_getMethodImplementation")
	mouseUp := objc.RegisterName("mouseUp:")
	deliver := objc.RegisterName("leafmarkDeliverMouseUp:")
	perform := objc.RegisterName("performSelector:withObject:afterDelay:inModes:")
	// 直接取 WKWebView 的实现，不依赖运行时的父类链。
	original := implementation(base, mouseUp)
	if original == 0 {
		return
	}
	mode := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), "kCFRunLoopCommonModes\x00")
	modes := objc.ID(objc.GetClass("NSArray")).Send(objc.RegisterName("arrayWithObject:"), mode).Send(objc.RegisterName("retain"))
	if !view.AddMethod(deliver, objc.NewIMP(func(self objc.ID, _ objc.SEL, event objc.ID) {
		purego.SyscallN(original, uintptr(self), uintptr(mouseUp), uintptr(event))
	}), "v@:@") {
		return
	}
	// MyGoWebView 自身未定义 mouseUp: 时才能添加；已定义则保持原样。
	view.AddMethod(mouseUp, objc.NewIMP(func(self objc.ID, _ objc.SEL, event objc.ID) {
		self.Send(perform, deliver, event, mouseUpDelay, modes)
	}), "v@:@")
}
