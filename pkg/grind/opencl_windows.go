//go:build windows && (amd64 || arm64)

package grind

import (
	"errors"
	"syscall"
)

// openCLLibrary opens OpenCL.dll, which GPU drivers install.
func openCLLibrary() (uintptr, error) {
	h, err := syscall.LoadLibrary("OpenCL.dll")
	if err != nil {
		return 0, errors.New("no OpenCL library (OpenCL.dll): install the GPU vendor's driver")
	}
	return uintptr(h), nil
}
