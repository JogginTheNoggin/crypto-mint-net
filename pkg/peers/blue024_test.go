package peers_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-067, D162: one server behind many host names. Each name the node
// reaches counts in the range it led to, so only book_per_range of them
// stay; and the owner's add takes a reached learned place when the book
// is full anyway.
func TestNamesForOneMachineShareARange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	b, err := peers.Open(path, peers.Limits{MaxRecords: 256, PerRange: 16})
	if err != nil {
		t.Fatal(err)
	}
	same := 0
	for i := 0; i < 256; i++ {
		a := fmt.Sprintf("n%d.evil.example:8646", i)
		add23(t, b, a, peers.FromPeerList, t23)
		b.Tried(a, true, t23)
		if err := b.Resolved(a, "203.0.113.7:8646", t23); errors.Is(err, peers.ErrSameMachine) {
			same++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if n := len(b.Records()); n != 16 || same != 240 {
		t.Fatalf("one machine holds %d records (%d names forgotten)", n, same)
	}
	// The range they share is full for an address in it, and its spread is one.
	if err := b.Add("203.0.113.9:8646", []wire.Section{series}, peers.FromPeerList, t23); !errors.Is(err, peers.ErrRangeFull) {
		t.Fatalf("an address of the shared range: %v", err)
	}
	// Pick spreads by that range: one of the names before a second
	// address from anywhere else.
	add23(t, b, "198.51.100.1:8646", peers.FromPeerList, t23)
	b.Tried("198.51.100.1:8646", true, t23.Add(time.Minute)) // tried after the names
	if got := b.Pick(series, 2, nil, t23.Add(time.Hour)); len(got) != 2 || (got[0] != "198.51.100.1:8646" && got[1] != "198.51.100.1:8646") {
		t.Fatalf("Pick took %v", got)
	}
	b.Delete("198.51.100.1:8646")
	// Via is kept across a save and a load.
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	c, err := peers.Open(path, peers.Limits{MaxRecords: 256, PerRange: 16})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Add("203.0.113.9:8646", []wire.Section{series}, peers.FromPeerList, t23); !errors.Is(err, peers.ErrRangeFull) {
		t.Fatalf("after a load, the shared range: %v", err)
	}
	// The owner's own name for the same machine stays.
	add23(t, c, "mine.example:8646", peers.FromHand, t23)
	c.Tried("mine.example:8646", true, t23)
	if err := c.Resolved("mine.example:8646", "203.0.113.7:8646", t23); err != nil {
		t.Fatal(err)
	}
	if _, held := c.Record("mine.example:8646"); !held {
		t.Fatal("the owner's name was forgotten")
	}
}

// RED-067, D162: a book full of reached learned records, from 256 ranges,
// still takes the owner's address, by hand or from p2p.peers; never a
// learned one.
func TestOwnerAddTakesAReachedPlace(t *testing.T) {
	b, err := peers.Open("", peers.Limits{MaxRecords: 256, PerRange: 16})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		a := fmt.Sprintf("n%d.evil.example:8646", i)
		add23(t, b, a, peers.FromPeerList, t23.Add(time.Duration(i)*time.Second))
		b.Tried(a, true, t23)
	}
	late := t23.Add(365 * 24 * time.Hour)
	if err := b.Add("20.0.0.1:1", []wire.Section{series}, peers.FromReply, late); !errors.Is(err, peers.ErrFull) {
		t.Fatalf("a learned address took a reached place: %v", err)
	}
	add23(t, b, "20.0.0.2:1", peers.FromHand, late)
	add23(t, b, "20.0.0.3:1", peers.FromConfig, late)
	for _, a := range []string{"n0.evil.example:8646", "n1.evil.example:8646"} {
		if _, held := b.Record(a); held {
			t.Fatalf("%s, the oldest, stayed", a)
		}
	}
	// A peer verified now keeps its place.
	v, _ := peers.Open("", peers.Limits{MaxRecords: 256, PerRange: 1})
	if err := v.Verify("30.0.0.1:1", series, peers.Priority, t23); err != nil {
		t.Fatal(err)
	}
	if err := v.Add("30.0.0.2:1", []wire.Section{series}, peers.FromHand, late); !errors.Is(err, peers.ErrRangeFull) {
		t.Fatalf("a verified peer gave way: %v", err)
	}
}
