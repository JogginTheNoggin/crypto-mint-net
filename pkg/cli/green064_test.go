package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// GREEN-064: a fast and a slow grinder against one payer's offers of
// several strengths, as a new user. -max-strength over the node's own
// bounty.max_strength asked payers for offers the node then dropped, with
// no word; working an offer the node dropped said only "no offer with
// that id was received"; and list -ask -max-strength still listed the
// stronger offers held from an earlier ask.
func TestBountyMaxStrengthFlow(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POON_PASSPHRASE", "pw")
	payer, fast, slow := t.TempDir(), t.TempDir(), t.TempDir()
	pr, fr, sr := []string{"-root", payer}, []string{"-root", fast}, []string{"-root", slow}
	pAddr := freeAddr(t)
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", pAddr}, pr...)...)
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", freeAddr(t)}, fr...)...)
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", freeAddr(t)}, sr...)...)
	dirs := []string{filepath.Join(payer, "policies", "default"), filepath.Join(fast, "policies", "default"), filepath.Join(slow, "policies", "default")}
	editPolicy(t, dirs[0], func(p *nodepolicy.Policy) {
		p.Bounty = &nodepolicy.Bounty{Post: true, MaxAmount: 100, Budget: 1000}
		p.P2P.AllowLocalPeers = true
	})
	editPolicy(t, dirs[1], func(p *nodepolicy.Policy) {
		p.Bounty = &nodepolicy.Bounty{Take: true}
		p.P2P.Peers, p.P2P.AllowLocalPeers = []string{pAddr}, true
	})
	editPolicy(t, dirs[2], func(p *nodepolicy.Policy) {
		p.Bounty = &nodepolicy.Bounty{Take: true, MaxStrength: 16}
		p.P2P.Peers, p.P2P.AllowLocalPeers = []string{pAddr}, true
	})
	for _, d := range dirs {
		defer startNode(t, d)()
	}
	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, strings.Repeat("a1", 32)), "-serial", "5"}, pr...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1", "-work", "4000"}, pr...)...)

	// Both grinders connected with the feature before the offers go out.
	for _, r := range [][]string{fr, sr} {
		deadline := time.Now().Add(20 * time.Second)
		for {
			if _, err := runErr(append([]string{"bounty", "list", "-ask"}, r...)...); err == nil {
				var ps struct {
					Peers []struct {
						ClaimedFeatures []struct{ Name string } `json:"claimed_features"`
					}
				}
				cli(t, &ps, append([]string{"peers"}, r...)...)
				if len(ps.Peers) > 0 && len(ps.Peers[0].ClaimedFeatures) > 0 {
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("the grinders did not connect")
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	BountyAskWait = 500 * time.Millisecond
	defer func() { BountyAskWait = 2 * time.Second }()
	ids := map[int]string{}
	for _, s := range []string{"8", "20"} {
		var posted rpc.BountyPostReply
		cli(t, &posted, append([]string{"bounty", "post", "-entry", built.Hash, "-strength", s, "-amount", "10"}, pr...)...)
		ids[int(posted.Offer.Offer.Strength)] = posted.Offer.ID.String()
	}

	// The fast grinder holds both; -max-strength lists only what it asks.
	var seen rpc.BountyReply
	for i := 0; i < 20 && len(seen.Held) < 2; i++ {
		cli(t, &seen, append([]string{"bounty", "list", "-ask"}, fr...)...)
	}
	if len(seen.Held) != 2 {
		t.Fatalf("fast grinder holds %d offers, want 2", len(seen.Held))
	}
	cli(t, &seen, append([]string{"bounty", "list", "-ask", "-max-strength", "10"}, fr...)...)
	if len(seen.Held) != 1 || seen.Held[0].Offer.Strength != 8 {
		t.Fatalf("list -ask -max-strength 10 shows %+v", seen.Held)
	}

	// The slow grinder: asking above its own setting is refused, and the
	// stronger offer it dropped says why it is not held.
	if _, err := runErr(append([]string{"bounty", "list", "-ask", "-max-strength", "22"}, sr...)...); err == nil || !strings.Contains(err.Error(), "over this node's bounty.max_strength 16") {
		t.Fatalf("ask over the node's own setting: %v", err)
	}
	if _, err := runErr(append([]string{"bounty", "work", ids[20], "-to", strings.Repeat("cd", 32)}, sr...)...); err == nil || !strings.Contains(err.Error(), "does not keep offers over bounty.max_strength 16") {
		t.Fatalf("work on an offer over the node's setting: %v", err)
	}
}
