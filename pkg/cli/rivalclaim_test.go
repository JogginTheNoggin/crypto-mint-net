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
	// -set through the running node saves and applies the book's limit in
	// place (D168), and one that needs it reloads.
	var set struct {
		Saved, Reloading bool
		InPlace          bool `json:"in_place"`
	}
	cli(t, &set, append([]string{"policy", "limits", "-set", "p2p.max_book=8192"}, r...)...)
	if !set.Saved || set.Reloading || !set.InPlace {
		t.Fatalf("set through the node in place: %+v", set)
	}
	set.InPlace = false
	cli(t, &set, append([]string{"policy", "limits", "-set", "ties=8"}, r...)...)
	if !set.Saved || !set.Reloading || set.InPlace {
		t.Fatalf("set through the node with a reload: %+v", set)
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

// The next step of a build names the pool the person can name again, so a
// -pool build's hint works (it said -serial 0, pool 0's); a plain build
// that is not selected says to work on its entry; -bytes keeps the entry
// with -parent too; and before-claim on a claim not yet selected says
// where the claim is (GREEN-048).
func TestBuildNextSteps(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "off"}, r...)...)
	stop := startNode(t, root+"/policies/default")
	defer stop()
	alice, bob := strings.Repeat("a1", 32), strings.Repeat("b2", 32)

	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, alice), "-serial", "5"}, r...)...)
	var built struct {
		Hash     string
		Switched bool
		Next     string
	}
	cli(t, &built, append([]string{"build", "-serial", "5"}, r...)...)
	if built.Switched || !strings.Contains(built.Next, "poo work -pool 1 -entry "+built.Hash) {
		t.Fatalf("plain build: %+v", built)
	}
	// Alice's claim is in an entry no layer has selected.
	var out bytes.Buffer
	err := run(append([]string{"claim", "-seed-file", seedFile(t, bob), "-serial", "5", "-parent", "before-claim"}, r...), &out)
	if err == nil || !strings.Contains(err.Error(), "-parent root") || !strings.Contains(err.Error(), "poo pending -pool 1") {
		t.Fatalf("before-claim on an unselected claim: %v", err)
	}
	var rival struct {
		Hash  string
		Entry string
		Next  string
	}
	cli(t, &rival, append([]string{"claim", "-seed-file", seedFile(t, bob), "-serial", "6", "-parent", "root"}, r...)...)
	if !strings.Contains(rival.Next, "poo grind start "+rival.Hash+" -pool 1") || strings.Contains(rival.Next, "-serial") || rival.Entry != "" {
		t.Fatalf("claim -parent root: %+v", rival)
	}
	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, bob), "-serial", "7"}, r...)...)
	var kept struct{ Entry, Next string }
	cli(t, &kept, append([]string{"build", "-pool", "1", "-parent", "root", "-bytes"}, r...)...)
	if kept.Entry == "" || !strings.Contains(kept.Next, "-pool 1") {
		t.Fatalf("build -pool 1 -parent root -bytes: %+v", kept)
	}
}

// A -set name without its section, or misspelt, is refused with the
// limit it most likely meant (GREEN-048).
func TestLimitsSetSuggests(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-p2p", "off", "-root", root)
	for kv, want := range map[string]string{"max_book=300": "did you mean p2p.max_book?", "p2p.max_bok=300": "did you mean p2p.max_book?", "log_size=20": "did you mean tracking.log_size?"} {
		err := run([]string{"policy", "limits", "-set", kv, "-root", root}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("-set %s: %v", kv, err)
		}
	}
	if limitHint("p2p.max_book") != "" || limitHint("zzz") != "" {
		t.Error("a hint for a real limit, or for a name near none")
	}
}

// An explicitly empty -parent is refused before anything is sent, instead
// of reading as -parent left out and submitting the claim (GUI/Blue, R23).
func TestEmptyParentRefused(t *testing.T) {
	for _, args := range [][]string{
		{"claim", "-serial", "0", "-parent", ""},
		{"claim", "-serial", "0", "-parent="},
		{"build", "-parent", ""},
	} {
		var o bytes.Buffer
		err := run(append(args, "-root", t.TempDir()), &o)
		if err == nil || !strings.Contains(err.Error(), "-parent was given empty") {
			t.Fatalf("%v: %v", args, err)
		}
	}
}
