package bounty

import "testing"

// GUI-GREEN-065: a grinder's held offer reads filled once the payer has
// accepted as many of this grinder's layers as the offer asks for, without
// another ask.
func TestGUIGreen065HeldFilledByAccepts(t *testing.T) {
	g, _ := Open(Config{Take: true}, "")
	o := testOffer(20, 2)
	if !g.Receive("payer", o) {
		t.Fatal("offer not kept")
	}
	state := func() string { return g.HeldOffers()[0].State }
	g.Acked("payer", Ack{Offer: o.ID(), Nonce: 1, Outcome: Accepted})
	g.Acked("payer", Ack{Offer: o.ID(), Nonce: 1, Outcome: Accepted})
	if s := state(); s != StateOpen {
		t.Fatalf("one layer of two accepted (twice): %s", s)
	}
	g.Acked("other", Ack{Offer: o.ID(), Nonce: 2, Outcome: Accepted})
	if s := state(); s != StateOpen {
		t.Fatalf("an ack from another peer counted: %s", s)
	}
	g.Acked("payer", Ack{Offer: o.ID(), Nonce: 2, Outcome: Accepted})
	if s := state(); s != StateFilled {
		t.Fatalf("both layers accepted: %s", s)
	}
}
