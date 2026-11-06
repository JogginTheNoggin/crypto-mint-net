package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// GREEN-038: poon follow -replace does what POST /v1/follow's replace does
// for the GUI (GUI-018): the series' rule becomes exactly the pools and
// mints given, in one change.
func TestFollowReplace(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o700)
	cli(t, nil, "init", "-example", "-dir", dir, "-p2p", "off")
	id := testutil.ExampleSeries().ID().String()
	var r struct{ Follow []nodepolicy.Follow }
	cli(t, &r, "follow", "-series", id, "-pool", "1", "-replace", "-dir", dir)
	if len(r.Follow) != 1 || len(r.Follow[0].Pools) != 1 || r.Follow[0].Pools[0] != 1 || len(r.Follow[0].Mints) != 0 {
		t.Fatalf("after replace -pool 1: %+v", r.Follow)
	}
	r.Follow = nil
	cli(t, &r, "follow", "-series", id, "-mint", "2", "-replace", "-dir", dir)
	if len(r.Follow) != 1 || len(r.Follow[0].Pools) != 0 || len(r.Follow[0].Mints) != 1 || r.Follow[0].Mints[0] != 2 {
		t.Fatalf("after replace -mint 2: %+v", r.Follow)
	}
	if err := run([]string{"unfollow", "-series", id, "-replace", "-dir", dir}, &strings.Builder{}); err == nil {
		t.Fatal("unfollow -replace accepted")
	}
}
