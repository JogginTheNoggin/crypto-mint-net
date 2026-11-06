package cli

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// editPolicy changes the node policy in dir.
func editPolicy(t *testing.T, dir string, f func(*nodepolicy.Policy)) {
	t.Helper()
	path := filepath.Join(dir, nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f(&p)
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
}

// D196: the bounty market between two nodes. The payer posts an offer for
// a layer on its tip; the grinder asks for offers, leases the offer,
// grinds the layer and claims it; the payer keeps the layer, pays from its
// wallet, and the grinder is told the payment's tid. Nothing about the
// ledger changes: the layer and the payment are ordinary ones.
func TestBountyMarket(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POO_PASSPHRASE", "pw")
	payer, grinder := t.TempDir(), t.TempDir()
	pr, gr := []string{"-root", payer}, []string{"-root", grinder}
	gAddr := freeAddr(t)
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", freeAddr(t)}, pr...)...)
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", gAddr}, gr...)...)
	pdir, gdir := filepath.Join(payer, "policies", "default"), filepath.Join(grinder, "policies", "default")
	editPolicy(t, pdir, func(p *nodepolicy.Policy) {
		p.Bounty = &nodepolicy.Bounty{Post: true, MaxAmount: 100, Budget: 100}
		p.P2P.Peers = []string{gAddr}
		p.P2P.AllowLocalPeers = true
	})
	editPolicy(t, gdir, func(p *nodepolicy.Policy) {
		p.Bounty = &nodepolicy.Bounty{Take: true}
		p.P2P.AllowLocalPeers = true
	})
	defer startNode(t, gdir)()
	defer startNode(t, pdir)()

	// The payer's funds: mint 5, claimed and worked in.
	pw := append([]string{"-wallet", filepath.Join(payer, "w.json")}, pr...)
	cli(t, nil, append([]string{"wallet", "create"}, pw...)...)
	cli(t, nil, append([]string{"wallet", "watch", "-serial", "5"}, pw...)...)
	cli(t, nil, append([]string{"wallet", "claim", "-serial", "5"}, pw...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1", "-work", "4000"}, pr...)...)

	// Wait until the two are connected and both list the feature.
	deadline := time.Now().Add(20 * time.Second)
	for {
		var ps struct {
			Peers []struct {
				ClaimedFeatures []struct{ Name string } `json:"claimed_features"`
			}
		}
		cli(t, &ps, append([]string{"peers"}, pr...)...)
		ok := false
		for _, p := range ps.Peers {
			for _, f := range p.ClaimedFeatures {
				ok = ok || f.Name == "bounty"
			}
		}
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the nodes did not connect with the bounty feature: %+v", ps)
		}
		time.Sleep(200 * time.Millisecond)
	}

	var posted rpc.BountyPostReply
	cli(t, &posted, append([]string{"bounty", "post", "-pool", "1", "-entry", built.Hash, "-strength", "8", "-amount", "10"}, pr...)...)
	if posted.Offer.Offer.Amount != 10 {
		t.Fatalf("post %+v", posted)
	}
	// Over the budget: refused.
	if _, err := runErr(append([]string{"bounty", "post", "-pool", "1", "-entry", built.Hash, "-strength", "8", "-amount", "10", "-count", "10"}, pr...)...); err == nil {
		t.Fatal("an offer over the budget was posted")
	}

	// The grinder asks, and holds the offer.
	BountyAskWait = 500 * time.Millisecond
	defer func() { BountyAskWait = 2 * time.Second }()
	var seen rpc.BountyReply
	for i := 0; i < 20; i++ {
		cli(t, &seen, append([]string{"bounty", "list", "-ask"}, gr...)...)
		if len(seen.Held) > 0 {
			break
		}
	}
	if len(seen.Held) != 1 || seen.Held[0].ID != posted.Offer.ID {
		t.Fatalf("held %+v", seen.Held)
	}
	gw := append([]string{"-wallet", filepath.Join(grinder, "w.json")}, gr...)
	cli(t, nil, append([]string{"wallet", "create"}, gw...)...)
	var addr struct{ Address string }
	cli(t, &addr, append([]string{"wallet", "address"}, gw...)...)

	var worked rpc.BountyWorkReply
	cli(t, &worked, append([]string{"bounty", "work", posted.Offer.ID.String(), "-to", addr.Address}, gr...)...)
	if worked.Grant.Seconds == 0 || !worked.Found || worked.Outcome != "accepted" || worked.Strength < 8 {
		t.Fatalf("work %+v", worked)
	}

	// The payer holds the claim, and pays it.
	var mine rpc.BountyReply
	cli(t, &mine, append([]string{"bounty", "list"}, pr...)...)
	if len(mine.Mine) != 1 || len(mine.Mine[0].Claims) != 1 || mine.Mine[0].Claims[0].PayTo.String() != addr.Address {
		t.Fatalf("payer's offers %+v", mine.Mine)
	}
	var paid BountyPayReply
	cli(t, &paid, append([]string{"bounty", "pay", posted.Offer.ID.String()}, pw...)...)
	if !paid.Told || paid.Amount != 10 || paid.Sent == nil || len(paid.Sent.Legs) == 0 {
		t.Fatalf("pay %+v", paid)
	}
	if _, err := runErr(append([]string{"bounty", "pay", posted.Offer.ID.String()}, pw...)...); err == nil {
		t.Fatal("paid twice")
	}

	// The grinder is told, with the tid.
	var acked rpc.BountyReply
	for i := 0; i < 50; i++ {
		cli(t, &acked, append([]string{"bounty", "list"}, gr...)...)
		if n := len(acked.Held[0].Acks); n > 0 && acked.Held[0].Acks[n-1].Outcome == bounty.Paid {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	acks := acked.Held[0].Acks
	if len(acks) != 2 || acks[1].Outcome != bounty.Paid || acks[1].TIDs[0] != paid.Sent.Legs[0].TID {
		t.Fatalf("grinder's acks %+v", acks)
	}
	if p := acked.Payers; len(p) != 1 {
		t.Fatalf("payer record %+v", p)
	}

	// Worked in at the payer, the grinder's address holds the amount.
	cli(t, nil, append([]string{"build", "-pool", "1", "-work", "4000"}, pr...)...)
	var held struct{ Balance uint64 }
	cli(t, &held, append(append([]string{"address"}, pr...), addr.Address)...)
	if held.Balance != 10 {
		t.Fatalf("the grinder's address holds %d at the payer, want 10", held.Balance)
	}

	// An offer for two layers on the same entry is filled by two work
	// calls: each starts at a random nonce, so the second finds another
	// layer, and neither is one the payer holds (RED-109, RED-110).
	var two rpc.BountyPostReply
	cli(t, &two, append([]string{"bounty", "post", "-pool", "1", "-entry", built.Hash, "-strength", "8", "-amount", "10", "-count", "2"}, pr...)...)
	for i := 0; i < 50; i++ {
		cli(t, &seen, append([]string{"bounty", "list", "-ask"}, gr...)...)
		if len(seen.Held) > 1 {
			break
		}
	}
	nonces := map[uint64]bool{worked.Nonce: true}
	for i := range 2 {
		var w rpc.BountyWorkReply
		cli(t, &w, append([]string{"bounty", "work", two.Offer.ID.String(), "-to", addr.Address}, gr...)...)
		if w.Outcome != "accepted" || nonces[w.Nonce] {
			t.Fatalf("work %d on the two-layer offer: %+v (nonces so far %v)", i, w, nonces)
		}
		nonces[w.Nonce] = true
	}

	// A node with the market off refuses the commands.
	if _, err := runErr(append([]string{"bounty", "cancel", posted.Offer.ID.String()}, gr...)...); err == nil {
		t.Fatal("a grinder cancelled the payer's offer")
	}
	// The first offer, filled and paid, needs nothing more: it moved to
	// the done file and is no longer listed (ORANGE-046 P1). The second is
	// filled and unpaid, so it stays, says so, and cancelling it keeps
	// its claims owed (GUI-GREEN-062).
	var listed rpc.BountyReply
	cli(t, &listed, append([]string{"bounty", "list"}, pr...)...)
	if len(listed.Mine) != 1 || listed.Mine[0].ID != two.Offer.ID || listed.Mine[0].State != bounty.StateFilled || listed.Mine[0].Closes == nil {
		t.Fatalf("listed %+v", listed.Mine)
	}
	if _, err := runErr(append([]string{"bounty", "cancel", posted.Offer.ID.String()}, pr...)...); err == nil {
		t.Fatal("a done offer was cancelled")
	}
	var closed bounty.Mine
	cli(t, &closed, append([]string{"bounty", "cancel", two.Offer.ID.String()}, pr...)...)
	if closed.State != bounty.StateClosed || closed.Owed() != 20 {
		t.Fatalf("cancelled %+v, owes %d", closed, closed.Owed())
	}
	var st rpc.StatusReply
	cli(t, &st, append([]string{"status"}, pr...)...)
	if done, err := os.ReadFile(filepath.Join(pdir, bounty.DoneFileName)); err != nil || !strings.Contains(string(done), posted.Offer.ID.String()) {
		t.Fatalf("the done file: %v", err)
	}
	if !st.Bounty.Post || st.Bounty.Take {
		t.Fatalf("status bounty %+v", st.Bounty)
	}
}
