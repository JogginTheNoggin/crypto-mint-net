package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-030 (D136): follow, unfollow and init -force change node.json only
// under the data directory's lock, as the schedule does (D134), so none
// writes behind a running node.
func TestPolicyWritesTakeTheLock(t *testing.T) {
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
	id := testutil.ExampleSeries().ID().String()
	unlock, err := nodepolicy.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"unfollow", "-series", id, "-no-reload", "-dir", dir},
		{"follow", "-series", id, "-pool", "0", "-no-reload", "-dir", dir},
		{"init", "-example", "-dir", dir, "-p2p", "off", "-force"},
	} {
		var out bytes.Buffer
		if err := run(args, &out); !errors.Is(err, nodepolicy.ErrLocked) {
			t.Fatalf("%v under another's lock: %v", args, err)
		}
		if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
			t.Fatalf("%v changed node.json under another's lock", args)
		}
	}
	unlock()
	cli(t, nil, "unfollow", "-series", id, "-no-reload", "-dir", dir)
	p, err := nodepolicy.Load(path)
	if err != nil || len(p.Follow) != 0 {
		t.Fatalf("after the lock: %+v, %v", p.Follow, err)
	}
}
