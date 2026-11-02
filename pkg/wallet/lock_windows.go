//go:build windows

package wallet

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on the file at path, waiting for it,
// and returns the function that releases it. Windows releases the lock
// if the process dies (GUI-019, D145).
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		windows.UnlockFileEx(h, 0, 1, 0, ol)
		f.Close()
	}, nil
}
