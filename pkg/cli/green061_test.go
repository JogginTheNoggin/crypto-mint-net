package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// GREEN-061: running-a-node.md's bounty steps name no pool, and the
// offer went to pool 0 whatever entry it named. Nothing checked that the
// pool holds the entry, so a grinder could work the layer, the payer
// accepted the claim and owed it, and the engine dropped the layer. Now
// the node finds the pool that holds the entry when none is named, and
// refuses a pool that does not hold it.
func TestBountyPostFindsTheEntrysPool(t *testing.T) {
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
	defer startNode(t, dir)()

	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, strings.Repeat("a1", 32)), "-serial", "5"}, r...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
	post := func(args ...string) []string {
		return append(append([]string{"bounty", "post", "-strength", "8", "-amount", "10"}, args...), r...)
	}

	// No pool named: the node offers on the pool that holds the entry.
	var rep rpc.BountyPostReply
	cli(t, &rep, post("-entry", built.Hash)...)
	if rep.Offer.Offer.Pool != 1 {
		t.Fatalf("an offer on pool 1's entry went to pool %d", rep.Offer.Offer.Pool)
	}
	// The wrong pool named: refused, and the error says where it is.
	if _, err := runErr(post("-entry", built.Hash, "-pool", "0")...); err == nil || !strings.Contains(err.Error(), "does not hold entry") || !strings.Contains(err.Error(), "it is in pool 1") {
		t.Fatalf("an offer on an entry pool 0 does not hold: %v", err)
	}
	if _, err := runErr(post("-entry", built.Hash, "-serial", "1")...); err == nil || !strings.Contains(err.Error(), "does not hold entry") {
		t.Fatalf("an offer by -serial in the wrong pool: %v", err)
	}
	// The right one by -serial still works.
	cli(t, &rep, post("-entry", built.Hash, "-serial", "5")...)
	// An entry no pool holds: refused, with where hashes come from.
	if _, err := runErr(post("-entry", strings.Repeat("ab", 32))...); err == nil || !strings.Contains(err.Error(), "no followed pool") {
		t.Fatalf("an offer on an unknown entry: %v", err)
	}
}
