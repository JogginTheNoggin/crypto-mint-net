package bounty

import "testing"

// BLUE-041 (RED-123): a held offer of more than MaxAcks layers reads
// filled once the payer accepted every one of this node's layers; an
// accept repeated while its ack is kept counts once.
func TestBlue041HeldFilledPastMaxAcks(t *testing.T) {
	g, _ := Open(Config{Take: true}, "")
	const n = 40
	o := testOffer(20, n)
	if !g.Receive("payer", o) {
		t.Fatal("offer not kept")
	}
	state := func() string { return g.HeldOffers()[0].State }
	for i := 1; i < n; i++ {
		g.Acked("payer", Ack{Offer: o.ID(), Nonce: uint64(i), Outcome: Accepted})
		g.Acked("payer", Ack{Offer: o.ID(), Nonce: uint64(i), Outcome: Accepted})
	}
	if s := state(); s != StateOpen {
		t.Fatalf("%d of %d layers accepted: %s", n-1, n, s)
	}
	g.Acked("payer", Ack{Offer: o.ID(), Nonce: n, Outcome: Accepted})
	if s := state(); s != StateFilled {
		t.Fatalf("all %d layers accepted: %s", n, s)
	}
}
