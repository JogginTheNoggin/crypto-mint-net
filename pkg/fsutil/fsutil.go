// Package fsutil writes files so that a crash leaves either the old file
// or the new one, whole.
package fsutil

import (
	"os"
	"path/filepath"
)

// WriteFile writes data to path durably and atomically. It writes a
// temporary file of its own beside path (so concurrent writers never
// tear each other's), sets perm, syncs it, closes it, renames it over
// path and syncs the directory, so the rename is on disk too. Without the
// syncs a crash could leave path empty, short or old after WriteFile had
// returned nil: many file systems may put the rename on disk before the
// data (RED-013, D114; RED-041, D148). Where a directory cannot be synced
// (Windows), that step is skipped. The parent directory is made with
// mode 0700 if missing.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync() // not possible on every platform; the file itself is synced
		d.Close()
	}
	return nil
}
