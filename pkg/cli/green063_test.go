package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// GREEN-063: a bounty from post to paid and through a restart, as a new
// user. A finished offer left poo bounty list with no command to see it
// again (only bounties-done.jsonl by hand), and the grinder's wallet
// showed 0 after being paid: a paid ack names the payment's tids, not the
// mint it is in, and the grinder's held offers and acks are gone after a
// restart. poo bounty list -done N lists the finished offers, and
// poo wallet watch -tid TID watches a payment's mint.
func TestBountyDoneListAndWatchByTID(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POO_PASSPHRASE", "pw")
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	dir := filepath.Join(root, "policies", "default")
	editPolicy(t, dir, func(p *nodepolicy.Policy) {
		p.Bounty = &nodepolicy.Bounty{Post: true, MaxAmount: 100, Budget: 1000}
	})
	stop := startNode(t, dir)

	payer := append([]string{"-wallet", filepath.Join(root, "payer.json")}, r...)
	cli(t, nil, append([]string{"wallet", "create"}, payer...)...)
	cli(t, nil, append([]string{"wallet", "watch", "-serial", "5"}, payer...)...)
	cli(t, nil, append([]string{"wallet", "claim", "-serial", "5"}, payer...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)

	// An offer closed with nothing owed is finished: it leaves the list,
	// and -done brings it back, through a restart.
	var posted rpc.BountyPostReply
	cli(t, &posted, append([]string{"bounty", "post", "-entry", built.Hash, "-strength", "8", "-amount", "10"}, r...)...)
	cli(t, nil, append([]string{"bounty", "cancel", posted.Offer.ID.String()}, r...)...)
	stop()
	defer startNode(t, dir)()
	var list rpc.BountyReply
	cli(t, &list, append([]string{"bounty", "list"}, r...)...)
	if len(list.Mine) != 0 || len(list.Done) != 0 {
		t.Fatalf("a finished offer in the plain list: %+v", list)
	}
	cli(t, &list, append([]string{"bounty", "list", "-done", "5"}, r...)...)
	if len(list.Done) != 1 || list.Done[0].ID != posted.Offer.ID || list.Done[0].State != "closed" {
		t.Fatalf("-done: %+v", list.Done)
	}

	// The grinder's wallet watches nothing until told the payment's mint;
	// -tid finds it from the tid alone.
	grinder := append([]string{"-wallet", filepath.Join(root, "grinder.json")}, r...)
	cli(t, nil, append([]string{"wallet", "create"}, grinder...)...)
	var addr struct{ Address string }
	cli(t, &addr, append([]string{"wallet", "address"}, grinder...)...)
	var sent struct {
		Legs []struct{ TID string }
	}
	cli(t, &sent, append([]string{"wallet", "pay", "-to", addr.Address, "-amount", "10"}, payer...)...)
	cli(t, nil, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
	var bal struct{ Total uint64 }
	cli(t, &bal, append([]string{"wallet", "balance"}, grinder...)...)
	if bal.Total != 0 {
		t.Fatalf("a wallet watching no mint holds %d", bal.Total)
	}
	cli(t, nil, append([]string{"wallet", "watch", "-tid", sent.Legs[0].TID}, grinder...)...)
	cli(t, &bal, append([]string{"wallet", "balance"}, grinder...)...)
	if bal.Total != 10 {
		t.Fatalf("after watch -tid the wallet holds %d, want 10", bal.Total)
	}
	if _, err := runErr(append([]string{"wallet", "watch", "-tid", strings.Repeat("ab", 32)}, grinder...)...); err == nil || !strings.Contains(err.Error(), "the node must hold the transaction") {
		t.Fatalf("watch -tid of an unknown transaction: %v", err)
	}
}
