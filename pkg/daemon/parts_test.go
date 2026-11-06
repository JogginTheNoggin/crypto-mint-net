package daemon

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// Large entries are put together under the data directory, and a crash's
// leftover parts are removed on the next start (GREEN-033).
func TestPartsDir(t *testing.T) {
	dir := t.TempDir()
	pd := filepath.Join(dir, PartsDirName)
	if err := os.MkdirAll(pd, 0o700); err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(pd, "poon-parts-123")
	keep := filepath.Join(pd, "other")
	for _, f := range []string{left, keep} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := partsDir(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil || got != pd {
		t.Fatalf("partsDir: %q %v", got, err)
	}
	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Fatalf("leftover part kept: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("another file removed: %v", err)
	}
	if fi, err := os.Stat(pd); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("parts dir mode: %v %v", fi.Mode(), err)
	}
}
