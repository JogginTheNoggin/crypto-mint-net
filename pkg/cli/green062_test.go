package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// GREEN-062: turning the bounty market on with poon policy limits -set as
// a new user. bounty.post=on was refused as "want NAME=VALUE"; posting
// with no spending limits said "over bounty.max_amount 0" and -set gave
// no warning; a node that only posts asked peers for offers it then
// dropped, and work said the offer "was not received"; the market-off
// error pointed at node.json.
func TestBountyTurnedOnWithLimits(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	lim := func(v any, sets ...string) {
		t.Helper()
		args := []string{"policy", "limits"}
		for _, s := range sets {
			args = append(args, "-set", s)
		}
		cli(t, v, append(args, r...)...)
	}

	// Off: the error names the command, not node.json.
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	if _, err := runErr(append([]string{"bounty", "list"}, r...)...); err == nil || !strings.Contains(err.Error(), "poon policy limits -set bounty.take=on") {
		t.Fatalf("market off: %v", err)
	}
	stop()

	// on/off words for an on/off setting; a bad one says what it takes.
	var rep rpc.LimitsReply
	lim(&rep, "bounty.post=on")
	if len(rep.Notes) != 1 || !strings.Contains(rep.Notes[0], "bounty.max_amount and bounty.budget are not set") {
		t.Fatalf("post on with no spending limits: notes %q", rep.Notes)
	}
	if _, err := runErr(append([]string{"policy", "limits", "-set", "bounty.post=maybe"}, r...)...); err == nil || !strings.Contains(err.Error(), "1 (on) or 0 (off)") {
		t.Fatalf("bounty.post=maybe: %v", err)
	}
	if _, err := runErr(append([]string{"policy", "limits", "-set", "p2p.max_book=lots"}, r...)...); err == nil || !strings.Contains(err.Error(), "not a whole number") {
		t.Fatalf("p2p.max_book=lots: %v", err)
	}
	if _, err := runErr(append([]string{"policy", "limits", "-set", "bounty.pots=on"}, r...)...); err == nil || !strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("a mistyped name with a word value: %v", err)
	}
	lim(&rep, "bounty.max_amount=500", "bounty.budget=100")
	if len(rep.Notes) != 1 || !strings.Contains(rep.Notes[0], "over bounty.budget 100") {
		t.Fatalf("max_amount over budget: notes %q", rep.Notes)
	}
	lim(&rep, "bounty.budget=0")

	// Posting with a spending limit unset names it and the command.
	defer startNode(t, filepath.Join(root, "policies", "default"))()
	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, strings.Repeat("a1", 32)), "-serial", "5"}, r...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
	if _, err := runErr(append([]string{"bounty", "post", "-entry", built.Hash, "-strength", "8", "-amount", "1"}, r...)...); err == nil || !strings.Contains(err.Error(), "bounty.budget is not set") {
		t.Fatalf("post with no budget: %v", err)
	}
	// A node that only posts: asking and working say take is off.
	if _, err := runErr(append([]string{"bounty", "list", "-ask"}, r...)...); err == nil || !strings.Contains(err.Error(), "bounty.take=on") {
		t.Fatalf("ask on a post-only node: %v", err)
	}
	if _, err := runErr(append([]string{"bounty", "work", strings.Repeat("ab", 32), "-to", strings.Repeat("cd", 32)}, r...)...); err == nil || !strings.Contains(err.Error(), "bounty.take=on") {
		t.Fatalf("work on a post-only node: %v", err)
	}
	// Through the running node the notes come back too.
	var last rpc.LimitsReply
	lim(&last, "bounty.budget=1000")
	if len(last.Notes) != 0 || !last.Saved {
		t.Fatalf("both set: %+v", last)
	}
	var over rpc.LimitsReply
	lim(&over, "bounty.max_amount=5000")
	if len(over.Notes) != 1 || !strings.Contains(over.Notes[0], "over bounty.budget 1000") {
		t.Fatalf("through the node: notes %q", over.Notes)
	}
}
