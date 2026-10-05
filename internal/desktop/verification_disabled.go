//go:build !verification

package desktop

import "github.com/egoist/mygo"

func prepareVerification(*Files)                       {}
func startNativeVerification(*mygo.Window, *nativeApp) {}
