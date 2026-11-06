package p2p

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-111 (c): a grant or ack for an offer reaches the waiting call only
// from the peer the offer came from.
func TestBlue037AnswersOnlyFromThePayer(t *testing.T) {
	b, _ := bounty.Open(bounty.Config{Take: true}, "")
	o := bounty.Offer{Kind: bounty.KindLayer, Series: hash.Hash{1}, Entry: hash.Hash{2}, Strength: 20, Amount: 10, Count: 1, OpenSeconds: 3600}
	b.Receive("payer", o)
	h := &Host{cfg: Config{Bounty: b}}
	id := o.ID()
	grants := make(chan bounty.Grant, 1)
	acks := make(chan bounty.Ack, 1)
	h.bwait.grants = map[hash.Hash]chan bounty.Grant{id: grants}
	h.bwait.takes = map[hash.Hash]bounty.Take{id: {Offer: id}}
	h.bwait.acks = map[bountyAckKey]chan bounty.Ack{{id, 7}: acks}
	grant := bountyMsg(wire.TypeBountyGrant, bounty.Grant{Offer: id, Seconds: 240}.Encode())
	ack := bountyMsg(wire.TypeBountyAck, bounty.Ack{Offer: id, Nonce: 7, Outcome: bounty.Accepted}.Encode())
	mallory, payer := &peer{id: "mallory"}, &peer{id: "payer"}
	for _, m := range []wire.Message{grant, ack} {
		if err := h.handleBounty(mallory, m); err != nil {
			t.Fatal(err)
		}
	}
	if len(grants) != 0 || len(acks) != 0 {
		t.Fatal("another peer answered the waiting call")
	}
	for _, m := range []wire.Message{grant, ack} {
		if err := h.handleBounty(payer, m); err != nil {
			t.Fatal(err)
		}
	}
	if len(grants) != 1 || len(acks) != 1 {
		t.Fatal("the payer's answers did not reach the waiting call")
	}
}
