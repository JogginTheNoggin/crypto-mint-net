//go:build !(linux || darwin || freebsd || netbsd || openbsd || windows)

package p2p

import "errors"

// statDisk is unknown here: large entries are fetched without a free
// space check.
func statDisk(dir string) (free, total uint64, err error) {
	return 0, 0, errors.New("free space unknown on this system")
}
