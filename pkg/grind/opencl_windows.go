//go:build windows && (amd64 || arm64)

package grind

import (
	"errors"

	"golang.org/x/sys/windows"
)

// openCLLibrary opens OpenCL.dll, which GPU drivers install in System32.
// Only System32 is searched, so a file of that name beside the program
// or in the current folder is never loaded (RED-127).
func openCLLibrary() (uintptr, error) {
	h, err := windows.LoadLibraryEx("OpenCL.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return 0, errors.New("no OpenCL library (OpenCL.dll in System32): install the GPU vendor's driver")
	}
	return uintptr(h), nil
}
