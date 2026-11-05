package p2p_test

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Black's term 3 for D186: a peer that lists no features, or lies about
// them and its release, still gets and serves a heavier ledger through the
// protocol-1 requests. Nothing is gated on what a hello claims.
func TestHeavierWhateverTheHelloClaims(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*wire.Hello)
	}{
		{"lists nothing", func(h *wire.Hello) { h.Release, h.Features = "", nil }},
		{"lies", func(h *wire.Hello) {
			h.Release = "99.0.0"
			h.Features = []wire.Feature{{Name: "aaa-not-real", Ver: 9}, {Name: "zzz-not-real", Ver: 1}}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// a claims oddly and serves the heavier ledger to b.
			a := startNode(t, "a", func(cf *p2p.Config) { p2p.SetHelloEdit(cf, c.edit) })
			b := startNode(t, "b", nil)
			last, owner := grow(t, a, nil, "alice", 3, 32)
			copyLedger(t, a, b)
			last, owner = growTagged(t, a, last, owner, "a", 4, 32)
			b.connect(a)
			eventuallyFor(t, 30*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)

			// Now b holds a heavier one and a, still claiming oddly, gets it.
			growTagged(t, b, last, owner, "b", 4, 32)
			eventuallyFor(t, 30*time.Second, "a to reach b's ledger", func() bool { return a.state() == b.state() }, a, b)

			// b shows a's claim as a's own, and a shows b's real one.
			var seen bool
			for _, p := range b.h.Peers() {
				ch := wire.Hello{}
				c.edit(&ch)
				if p.Release == ch.Release && len(p.ClaimedFeatures) == len(ch.Features) {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("b does not show a's claim: %+v", b.h.Peers())
			}
			for _, p := range a.h.Peers() {
				if p.Release != node.RunningRelease().Version || len(p.ClaimedFeatures) != len(wire.OwnFeatures) {
					t.Fatalf("a shows b as %+v", p)
				}
			}
		})
	}
}
