//go:build !unix && !windows

package nodepolicy

import "os"

// Lock prepares a data directory. On this platform it does not stop two
// nodes from using the same directory; run one node per directory.
func Lock(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return func() {}, nil
}
