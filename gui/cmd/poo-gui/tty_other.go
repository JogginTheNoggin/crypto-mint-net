//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

// isTerminal reports whether f is a character device, the closest test
// the standard library gives here. It only decides what SIGHUP does, and
// Windows never sends SIGHUP.
func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
