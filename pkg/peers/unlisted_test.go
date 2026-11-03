package peers_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// D157: an imported address is unlisted, never picked until promoted,
// keeps a known address's states, and survives a save; labels are
// checked and saved.
func TestUnlistedAndLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), peers.FileName)
	b, err := peers.Open(path, peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Add("10.0.0.1:1", []wire.Section{series}, peers.FromRegistry, t0); err != nil {
		t.Fatal(err)
	}
	if err := b.AddUnlisted("10.0.0.1:1", []wire.Section{series, other}, t0); err != nil {
		t.Fatal(err)
	}
	if err := b.AddUnlisted("10.1.0.1:1", []wire.Section{series}, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if st, _ := b.State("10.0.0.1:1", series); st != peers.Candidate {
		t.Fatalf("an import changed a known state to %v", st)
	}
	if st, _ := b.State("10.0.0.1:1", other); st != peers.Unlisted {
		t.Fatalf("a new mint policy of a known address is %v", st)
	}
	if got := b.Pick(series, 10, nil, t0); len(got) != 1 || got[0] != "10.0.0.1:1" {
		t.Fatalf("Pick took an unlisted address: %v", got)
	}
	if n := b.PromoteImported(series, 1); n != 1 {
		t.Fatalf("promoted %d", n)
	}
	if got := b.Pick(series, 10, nil, t0); len(got) != 2 {
		t.Fatalf("after promoting: %v", got)
	}
	if st, _ := b.State("10.0.0.1:1", other); st != peers.Unlisted {
		t.Fatal("promoting one mint policy promoted another")
	}

	for _, bad := range []string{"a\x1b[8Ab", strings.Repeat("x", peers.MaxLabel+1), "\xff"} {
		if _, err := b.SetLabel("10.0.0.1:1", bad); err == nil {
			t.Fatalf("label %q taken", bad)
		}
	}
	if ok, err := b.SetLabel("10.0.0.1:1", "home node"); !ok || err != nil {
		t.Fatalf("label: %v %v", ok, err)
	}
	if ok, _ := b.SetLabel("10.9.0.1:1", "x"); ok {
		t.Fatal("labelled an address the book lacks")
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	b2, err := peers.Open(path, peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := b2.Record("10.0.0.1:1")
	if r.Label != "home node" || !r.Has(peers.Unlisted) {
		t.Fatalf("after a restart: %+v", r)
	}
}
