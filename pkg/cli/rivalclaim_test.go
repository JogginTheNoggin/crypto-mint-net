package cli

import (
	"bytes"
	"strings"
	"testing"
)

// poo claim -parent before-claim builds a rival claim on the branch from
// before the selected claim, and enough work on it makes it the selected
// ledger: the mint then belongs to the rival (D161).
func TestRivalClaimCLI(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "off"}, r...)...)
	stop := startNode(t, root+"/policies/default")
	defer stop()
	alice, bob := strings.Repeat("a1", 32), strings.Repeat("b2", 32)
	var bobKey struct {
		Address string `json:"address_sha256"`
	}
	cli(t, &bobKey, "keygen", "-seed", bob)

	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, alice), "-serial", "5"}, r...)...)
	var worked struct{ Layer struct{ Switched bool } }
	cli(t, &worked, append([]string{"build", "-serial", "5", "-work", "2000"}, r...)...)
	if !worked.Layer.Switched {
		t.Fatalf("alice's claim was not selected: %+v", worked)
	}
	// A plain second claim is refused as pending.
	var out bytes.Buffer
	if err := run(append([]string{"claim", "-seed-file", seedFile(t, bob), "-serial", "5"}, r...), &out); err == nil {
		t.Fatalf("a second claim was taken as pending:\n%s", out.String())
	}
	var rival struct {
		Hash     string
		Parent   string
		Switched bool
		Next     string
	}
	cli(t, &rival, append([]string{"claim", "-seed-file", seedFile(t, bob), "-serial", "5", "-parent", "before-claim"}, r...)...)
	if rival.Switched || !strings.Contains(rival.Next, "poo grind start "+rival.Hash) {
		t.Fatalf("rival %+v", rival)
	}
	for i := 0; i < 50; i++ {
		var w struct{ Switched bool }
		cli(t, &w, append([]string{"work", "-serial", "5", "-entry", rival.Hash, "-start", itoa(i * 20000), "-tries", "20000"}, r...)...)
		if w.Switched {
			break
		}
	}
	var hs []struct{ Address string }
	cli(t, &hs, append([]string{"holdings", "-serial", "5"}, r...)...)
	if len(hs) != 1 || hs[0].Address != bobKey.Address {
		t.Fatalf("mint 5 is held by %+v, want bob %s", hs, bobKey.Address)
	}
	// -parent before-claim needs a mint; a build on a parent not held is
	// refused.
	if err := run(append([]string{"build", "-pool", "1", "-parent", "before-claim"}, r...), &out); err == nil || !strings.Contains(err.Error(), "-serial") {
		t.Fatalf("before-claim without -serial: %v", err)
	}
	if err := run(append([]string{"build", "-pool", "1", "-parent", strings.Repeat("ab", 32)}, r...), &out); err == nil || !strings.Contains(err.Error(), "not held") {
		t.Fatalf("a parent not held: %v", err)
	}
	// -set through the running node, which saves and reloads.
	var set struct {
		Saved, Reloading bool
	}
	cli(t, &set, append([]string{"policy", "limits", "-set", "p2p.max_book=8192"}, r...)...)
	if !set.Saved || !set.Reloading {
		t.Fatalf("set through the node: %+v", set)
	}
}

// poo policy limits -set changes node.json with the node stopped, checked
// whole, and a bad value changes nothing.
func TestPolicyLimitsSet(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-p2p", "off"}, r...)...)
	var rep struct {
		Saved  bool
		Limits []struct {
			Name string
			Set  int
			Used int
		}
	}
	cli(t, &rep, append([]string{"policy", "limits", "-set", "p2p.max_book=8192", "-set", "tracking.log_size=512"}, r...)...)
	used := map[string]int{}
	for _, l := range rep.Limits {
		used[l.Name] = l.Used
	}
	if !rep.Saved || used["p2p.max_book"] != 8192 || used["tracking.log_size"] != 512 {
		t.Fatalf("set: %+v", rep)
	}
	var out bytes.Buffer
	for _, bad := range []string{"p2p.max_book=10", "p2p.nope=1", "p2p.max_book"} {
		if err := run(append([]string{"policy", "limits", "-set", bad}, r...), &out); err == nil {
			t.Errorf("-set %s taken", bad)
		}
	}
	var after []struct {
		Name string
		Used int
	}
	cli(t, &after, append([]string{"policy", "limits", "-o", "json"}, r...)...)
	for _, l := range after {
		if l.Name == "p2p.max_book" && l.Used != 8192 {
			t.Fatalf("a refused -set changed max_book: %+v", l)
		}
	}
	if err := run(append([]string{"-view-only", "policy", "limits", "-set", "p2p.max_book=0"}, r...), &out); err == nil {
		t.Error("view-only -set taken")
	}
}
