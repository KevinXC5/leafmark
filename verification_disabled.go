//go:build !verification

package main

import "github.com/egoist/mygo"

func prepareVerification(*Files)             {}
func startVerification(*mygo.Window, *Files) {}
