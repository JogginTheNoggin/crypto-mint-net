//go:build linux || darwin || freebsd || netbsd || openbsd

package p2p

import "syscall"

// statDisk returns the bytes free to this process and the size of the
// disk that holds dir.
func statDisk(dir string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), uint64(st.Blocks) * uint64(st.Bsize), nil
}
