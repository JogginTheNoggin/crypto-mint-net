package p2p_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// category returns the category tn gives the peer at addr, by the address
// it dialed.
func (tn *tnode) category(addr string) string {
	for _, p := range tn.h.Peers() {
		if p.Addr == addr {
			for _, st := range p.Categories {
				return st.Category
			}
		}
	}
	return ""
}

// Slots decide categories: with one priority and one backup slot, the
// first outbound peer is priority, the second backup, and a third is
// closed. A review swaps a backup with a priority peer once it leads on
// the measures both share by more than the margin.
func TestCategoriesAndReview(t *testing.T) {
	tr, err := tracking.New(tracking.Config{}, "")
	must(t, err)
	a := startNode(t, "a", func(c *p2p.Config) {
		c.Slots = peers.Slots{Priority: 1, Backup: 1, Margin: 1}
		c.Tracker = tr
	})
	b, c, d := startNode(t, "b", nil), startNode(t, "c", nil), startNode(t, "d", nil)
	a.connect(b)
	a.connect(c)
	if got := a.category(b.addr()); got != peers.CatPriority.String() {
		t.Fatalf("b is %q", got)
	}
	if got := a.category(c.addr()); got != peers.CatBackup.String() {
		t.Fatalf("c is %q", got)
	}
	if err := a.h.Connect(d.addr()); err == nil || !strings.Contains(err.Error(), "no slot") {
		t.Fatalf("a third outbound peer: %v", err)
	}
	eventually(t, "d to be closed", func() bool { return len(a.h.Peers()) == 2 })
	if st, ok := a.bookState(d.addr()); ok && st.Listed() {
		t.Fatalf("d listed as %v", st)
	}

	// c answers every ping quickly; b misses them.
	sec := ""
	for _, p := range a.h.Peers() {
		for s := range p.Categories {
			sec = s
		}
	}
	now := time.Now()
	for i := 0; i < 5; i++ {
		tr.Ping(c.addr(), sec, true, time.Millisecond, now)
		tr.Ping(b.addr(), sec, false, 0, now)
	}
	a.h.Review()
	if got := a.category(c.addr()); got != peers.CatPriority.String() {
		t.Fatalf("after the review c is %q", got)
	}
	if got := a.category(b.addr()); got != peers.CatBackup.String() {
		t.Fatalf("after the review b is %q", got)
	}
	if st, _ := a.bookState(c.addr()); st != peers.Priority {
		t.Fatalf("book has c as %v", st)
	}
	// A review with nothing to change changes nothing.
	a.h.Review()
	if got := a.category(c.addr()); got != peers.CatPriority.String() {
		t.Fatalf("a second review moved c to %q", got)
	}
}

// A blocked address is refused both ways: it cannot connect, and it is
// not dialed.
func TestBlockedAddressIsRefused(t *testing.T) {
	tr, err := tracking.New(tracking.Config{}, "")
	must(t, err)
	a := startNode(t, "a", func(c *p2p.Config) { c.Tracker = tr })
	b := startNode(t, "b", nil)
	tr.BlockFor(b.addr(), time.Hour, time.Now())
	if err := a.h.Connect(b.addr()); err == nil {
		t.Fatal("dialed a blocked address")
	}
	// From b's side the connection is closed before it is answered.
	trA, err := tracking.New(tracking.Config{LoopbackAsOne: true}, "")
	must(t, err)
	e := startNode(t, "e", func(c *p2p.Config) { c.Tracker = trA })
	trA.BlockFor("127.0.0.1", time.Hour, time.Now())
	conn, err := net.Dial("tcp", e.addr())
	must(t, err)
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("a blocked address was answered")
	}
	if len(e.h.Peers()) != 0 {
		t.Fatal("a blocked peer was kept")
	}
	// Lifting the block lets it back in.
	tr.Lift(b.addr())
	a.connect(b)
}
