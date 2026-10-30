package main

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// openTTY opens a new pseudo-terminal and returns its terminal end, or
// nil if this machine cannot make one.
func openTTY(t *testing.T) *os.File {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	t.Cleanup(func() { m.Close() })
	var n, unlock uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		return nil
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		return nil
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil
	}
	t.Cleanup(func() { s.Close() })
	return s
}
