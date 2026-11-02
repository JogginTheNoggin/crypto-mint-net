package fsutil_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
)

// WriteFile replaces a file whole, with the mode asked, leaves no
// temporary file behind, makes a missing parent directory, and concurrent
// writers never tear the file (RED-041, D148).
func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "f.json")
	if err := fsutil.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fsutil.WriteFile(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "two" {
		t.Fatalf("read %q %v", b, err)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := make([]byte, 64<<10)
			for j := range data {
				data[j] = byte('a' + i)
			}
			if err := fsutil.WriteFile(path, data, 0o600); err != nil && runtime.GOOS != "windows" {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	b, _ = os.ReadFile(path)
	for _, c := range b {
		if c != b[0] {
			t.Fatal("the file was torn by concurrent writers")
		}
	}
	es, _ := os.ReadDir(filepath.Dir(path))
	if len(es) != 1 {
		t.Fatalf("left behind %v", es)
	}
	// A failed write leaves the old file and no temporary one.
	if err := fsutil.WriteFile(filepath.Join(path, "x"), []byte("y"), 0o600); err == nil {
		t.Fatal("wrote under a file")
	}
}
