//go:build verification && !darwin

package desktop

import "github.com/egoist/mygo"

func refreshVerificationWindow(win *mygo.Window) { win.Invalidate() }
