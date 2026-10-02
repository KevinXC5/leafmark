//go:build !verification

package desktop

import "github.com/egoist/mygo"

func prepareVerification(*Files)             {}
func startVerification(*mygo.Window, *Files) {}
