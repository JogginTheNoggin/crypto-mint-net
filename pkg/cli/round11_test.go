package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// GUI-017 (D134): with no node answering, a schedule edit changes
// node.json under the data directory's lock, so it is refused while
// another process holds the lock, and node.json is left as it was.
func TestOfflineScheduleEditTakesTheLock(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("the lock is advisory only off Unix")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o700)
	cli(t, nil, "init", "-example", "-dir", dir, "-p2p", "off")
	path := filepath.Join(dir, nodepolicy.FileName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := nodepolicy.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = run([]string{"grind", "schedule", "set", "-threads", "1", "-dir", dir}, &out)
	if !errors.Is(err, nodepolicy.ErrLocked) {
		t.Fatalf("edit under another's lock: %v", err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("node.json changed under another's lock")
	}
	// Viewing needs no lock.
	if err := run([]string{"grind", "schedule", "-dir", dir}, &out); err != nil {
		t.Fatalf("view under another's lock: %v", err)
	}
	unlock()
	cli(t, nil, "grind", "schedule", "set", "-threads", "1", "-dir", dir)
	p, err := nodepolicy.Load(path)
	if err != nil || p.Grind == nil || p.Grind.Threads != 1 {
		t.Fatalf("after the lock: %+v, %v", p.Grind, err)
	}
}
