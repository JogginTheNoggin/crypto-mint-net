//go:build windows

package p2p

import "golang.org/x/sys/windows"

// statDisk returns the bytes free to this process and the size of the
// disk that holds dir.
func statDisk(dir string) (free, total uint64, err error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, err
	}
	var avail, size, all uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &size, &all); err != nil {
		return 0, 0, err
	}
	return avail, size, nil
}
