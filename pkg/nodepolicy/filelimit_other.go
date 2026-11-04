//go:build !unix

package nodepolicy

// openFileLimit is not known on this platform (Windows has no such small
// per-process default).
func openFileLimit() (uint64, bool) { return 0, false }
