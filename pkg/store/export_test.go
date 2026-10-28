package store

import "os"

// SetFileSync replaces how a log syncs its file, for a test, and returns
// a function that restores it.
func SetFileSync(f func(*os.File) error) func() {
	old := fileSync
	fileSync = f
	return func() { fileSync = old }
}
