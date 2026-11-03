package p2p_test

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// D157: an imported address is unlisted until the node is short of peers;
// then the connection manager tries it, unless the node policy says not
// to, when only ConnectImported does.
func TestManagerDialsImported(t *testing.T) {
	a := startNode(t, "a", nil)
	d := startNode(t, "d", nil)
	must(t, d.h.Book().AddUnlisted(a.addr(), []wire.Section{poolSection()}, time.Now()))
	if st, _ := d.bookState(a.addr()); st != peers.Unlisted {
		t.Fatalf("imported as %v", st)
	}
	d.h.Manage()
	eventually(t, "d to peer with the imported a", func() bool { return peerAddrs(d)[a.addr()] })

	b := startNode(t, "b", nil)
	e := startNode(t, "e", func(c *p2p.Config) { c.NoDialImported = true })
	must(t, e.h.Book().AddUnlisted(b.addr(), []wire.Section{poolSection()}, time.Now()))
	e.h.Manage()
	time.Sleep(200 * time.Millisecond)
	if peerAddrs(e)[b.addr()] {
		t.Fatal("dialled an imported address with no_dial_imported")
	}
	if st, _ := e.bookState(b.addr()); st != peers.Unlisted {
		t.Fatalf("no_dial_imported left it %v", st)
	}
	if n := e.h.ConnectImported(); n != 1 {
		t.Fatalf("ConnectImported promoted %d", n)
	}
	eventually(t, "e to peer with b once asked", func() bool { return peerAddrs(e)[b.addr()] })
}

// D157: the owner's add puts an address in the book as a candidate for
// every followed policy, with its label; a blocked one is refused.
func TestAddPeerByHand(t *testing.T) {
	a := startNode(t, "a", nil)
	d := startNode(t, "d", nil)
	must(t, d.h.AddPeer(a.addr(), "friend"))
	r, ok := d.h.Book().Record(a.addr())
	if !ok || r.Label != "friend" || r.Source != peers.FromHand || !r.Has(peers.Candidate) {
		t.Fatalf("added: %+v", r)
	}
	if err := d.h.AddPeer(a.addr(), "\x1b[J"); err == nil {
		t.Fatal("a label with a control character was taken")
	}
	d.h.Block("203.0.114.1:9", -1, false)
	if err := d.h.AddPeer("203.0.114.1:9", ""); err == nil {
		t.Fatal("a blocked address was added")
	}
}
