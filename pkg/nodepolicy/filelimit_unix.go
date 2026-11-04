//go:build unix

package nodepolicy

import "syscall"

func openFileLimit() (uint64, bool) {
	var r syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &r); err != nil {
		return 0, false
	}
	return uint64(r.Cur), true
}
