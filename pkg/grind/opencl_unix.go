//go:build (linux || darwin) && (amd64 || arm64)

package grind

import (
	"errors"
	"runtime"

	"github.com/ebitengine/purego"
)

// openCLLibrary opens the system's OpenCL library: the ICD loader on
// Linux, the framework on macOS.
func openCLLibrary() (uintptr, error) {
	names := []string{"libOpenCL.so.1", "libOpenCL.so"}
	if runtime.GOOS == "darwin" {
		names = []string{"/System/Library/Frameworks/OpenCL.framework/OpenCL"}
	}
	for _, n := range names {
		if lib, err := purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			return lib, nil
		}
	}
	return 0, errors.New("no OpenCL library (" + names[0] + "): install the GPU vendor's OpenCL driver")
}
