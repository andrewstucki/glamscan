// +build windows

package main

import "os"

func initializeSystemLogger(debug bool) *Logger {
	return NewLogger(debug, os.Stdout, os.Stdout, os.Stdout, os.Stderr)
}
