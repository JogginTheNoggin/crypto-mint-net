//go:build unix

package wallet

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on the file at path, waiting for it,
// and returns the function that releases it. The operating system
// releases the lock if the process dies, so a crash never leaves the
// wallet locked (GUI-019, D145).
func lockFile(path string, busy func()) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	// Tried first without waiting, so a command that must wait can say
	// so (RED-042, D148).
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		if busy != nil {
			busy()
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
