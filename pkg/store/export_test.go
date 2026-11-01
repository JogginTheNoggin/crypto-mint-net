package store

import "os"

// SetFileSync replaces how a log syncs its file, for a test, and returns
// a function that restores it.
func SetFileSync(f func(*os.File) error) func() {
	old := fileSync
	fileSync = f
	return func() { fileSync = old }
}

// SetPieceSize lowers the most data one record on disk holds, for a test,
// and returns a function that restores it.
func SetPieceSize(n int) func() {
	old := pieceSize
	pieceSize = n
	return func() { pieceSize = old }
}
