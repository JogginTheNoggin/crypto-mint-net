//go:build unix

package nodepolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Lock takes an exclusive lock on a data directory, so two nodes never use
// the same data at once, and returns the function that releases it. The
// operating system releases the lock if the process dies, so a crash never
// leaves a directory locked.
func Lock(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "LOCK"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("%w: %s", ErrLocked, dir)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
