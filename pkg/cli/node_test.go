package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// startNode runs the node of root's current policy until the returned stop
// function is called.
func startNode(t *testing.T, dir string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	var log bytes.Buffer
	go func() { done <- runNode(ctx, dir, &log, func(string) { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("node did not start: %v\n%s", err, log.String())
	case <-time.After(10 * time.Second):
		t.Fatal("node did not start")
	}
	return func() {
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("node: %v\n%s", err, log.String())
		}
	}
}

func cli(t *testing.T, v any, args ...string) {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("poo %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	if v != nil {
		if err := json.Unmarshal(out.Bytes(), v); err != nil {
			t.Fatalf("poo %s: %v\n%s", strings.Join(args, " "), err, out.String())
		}
	}
}

func TestNodeEndToEnd(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	dir := root + "/policies/default"
	stop := startNode(t, dir)

	alice := strings.Repeat("a1", 32)
	bob := strings.Repeat("b2", 32)
	var bobKey struct {
		Address string `json:"address_sha256"`
	}
	cli(t, &bobKey, "keygen", "-seed", bob)

	var claim struct{ TID string }
	cli(t, &claim, append([]string{"claim", "-seed", alice, "-serial", "5"}, r...)...)
	var built struct {
		Hash     string
		Switched bool
	}
	cli(t, &built, append([]string{"build", "-pool", "1"}, r...)...)
	if built.Switched {
		t.Fatal("an entry with no credit was selected")
	}
	var worked struct {
		Strength int
		Switched bool
	}
	cli(t, &worked, append([]string{"work", "-pool", "1", "-entry", built.Hash, "-tries", "4000"}, r...)...)
	if !worked.Switched || worked.Strength < 6 {
		t.Fatalf("work %+v", worked)
	}
	// More work on the selected tip, the default target.
	cli(t, &worked, append([]string{"work", "-pool", "1", "-start", "4000", "-tries", "100"}, r...)...)
	var pay struct{ TID string }
	cli(t, &pay, append([]string{"pay", "-seed", alice, "-serial", "5", "-to", bobKey.Address, "-amount", "30"}, r...)...)
	var bw struct {
		Layer struct{ Switched bool }
	}
	cli(t, &bw, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
	if !bw.Layer.Switched {
		t.Fatalf("build -work did not select the new entry: %+v", bw)
	}
	var payment struct {
		Selected bool
		From     string
	}
	cli(t, &payment, append([]string{"payment", "-serial", "5", "-tid", pay.TID}, r...)...)
	if !payment.Selected || payment.From == "0" {
		t.Fatalf("payment %+v", payment)
	}
	stop()

	// Restart: everything comes back from the store.
	stop = startNode(t, dir)
	defer stop()
	var hs []struct {
		Address string
		Amount  uint64
	}
	cli(t, &hs, append([]string{"holdings", "-serial", "5"}, r...)...)
	got := map[string]uint64{}
	for _, h := range hs {
		got[h.Address] = h.Amount
	}
	if len(hs) != 2 || got[bobKey.Address] != 30 {
		t.Fatalf("holdings after restart %+v", hs)
	}
	var st struct {
		Pools []struct {
			Pool   uint64
			Height int
		}
		Replay struct{ Records, Kept int }
	}
	cli(t, &st, append([]string{"status"}, r...)...)
	if len(st.Pools) != 2 || st.Pools[1].Height != 2 || st.Replay.Records == 0 {
		t.Fatalf("status %+v", st)
	}
	// Paying more than the key holds fails before anything is sent.
	var out bytes.Buffer
	if err := run(append([]string{"pay", "-seed", alice, "-serial", "5", "-to", bobKey.Address, "-amount", "71"}, r...), &out); err == nil {
		t.Fatal("overspend accepted")
	}
}

func TestPolicyCommands(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-root", root, "-policy", "home", "-listen", "off")
	cli(t, nil, "init", "-root", root, "-policy", "shop", "-listen", "off")
	var out bytes.Buffer
	if err := run([]string{"init", "-root", root, "-policy", "shop"}, &out); err == nil {
		t.Fatal("init over an existing policy without -force")
	}
	list := func() string {
		var b bytes.Buffer
		if err := run([]string{"policy", "list", "-root", root}, &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	if l := list(); !strings.Contains(l, "* shop") || !strings.Contains(l, "  home") {
		t.Fatalf("list:\n%s", l)
	}
	for _, args := range [][]string{
		{"policy", "use", "-root", root, "home"},
		{"policy", "archive", "-root", root, "shop"},
		{"policy", "unarchive", "-root", root, "-fresh", "shop"},
		{"policy", "delete", "-root", root, "shop"},
	} {
		cli(t, nil, args...)
	}
	if l := list(); !strings.Contains(l, "* home") || strings.Contains(l, "shop") {
		t.Fatalf("list:\n%s", l)
	}
	for _, args := range [][]string{
		{"policy"},
		{"policy", "use", "-root", root},
		{"policy", "frobnicate", "-root", root, "x"},
		{"policy", "delete", "-root", root, "nope"},
		{"status", "-root", root},
		{"claim", "-root", root, "-seed", "00"},
	} {
		if err := run(args, &out); err == nil {
			t.Errorf("poo %v succeeded", args)
		}
	}
}

// Two nodes run by the command: the second lists the first as a peer and
// receives what the first builds.
func TestTwoNodes(t *testing.T) {
	root := t.TempDir()
	one := []string{"-root", root, "-policy", "one"}
	two := []string{"-root", root, "-policy", "two"}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, one...)...)
	defer startNode(t, root+"/policies/one")()
	var peers struct {
		NodeID string `json:"node_id"`
		Listen string
		Peers  []struct{ ID string }
	}
	cli(t, &peers, append([]string{"peers"}, one...)...)
	if peers.Listen == "" || len(peers.Peers) != 0 {
		t.Fatalf("peers of one: %+v", peers)
	}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "off", "-peer", peers.Listen}, two...)...)
	stopTwo := startNode(t, root+"/policies/two")
	defer func() {
		stopTwo()
		// The peer book is saved with the node.
		b, err := os.ReadFile(root + "/policies/two/peers.json")
		if err != nil || !strings.Contains(string(b), peers.Listen) {
			t.Errorf("peer book: %v\n%s", err, b)
		}
	}()
	firstID := peers.NodeID
	waitFor(t, func() bool {
		cli(t, &peers, append([]string{"peers"}, two...)...)
		return len(peers.Peers) == 1 && peers.Peers[0].ID == firstID
	})

	alice := strings.Repeat("a1", 32)
	cli(t, nil, append([]string{"claim", "-seed", alice, "-serial", "5"}, one...)...)
	cli(t, nil, append([]string{"build", "-pool", "1", "-work", "4000"}, one...)...)
	type status struct {
		Pools []struct {
			Tip    string
			Height int
			Weight string
		}
	}
	var s1, s2 status
	cli(t, &s1, append([]string{"status"}, one...)...)
	waitFor(t, func() bool {
		cli(t, &s2, append([]string{"status"}, two...)...)
		return s2.Pools[1] == s1.Pools[1]
	})
	if s2.Pools[1].Height != 1 {
		t.Fatalf("two: %+v", s2.Pools[1])
	}

	// The owner's view of the tracker and the chase queue, and acting on
	// a peer: two times one out, which closes the connection.
	var tracked struct{ Tracked map[string]any }
	cli(t, &tracked, append([]string{"tracking"}, two...)...)
	if _, ok := tracked.Tracked[peers.Listen]; !ok {
		t.Fatalf("one is not tracked: %+v", tracked)
	}
	var chases struct{ Groups []any }
	cli(t, &chases, append([]string{"chases"}, two...)...)
	var acted struct {
		Closed int
		Status struct {
			TimedOut bool `json:"timed_out"`
		} `json:"status"`
	}
	cli(t, &acted, append(append([]string{"peer", "timeout"}, two...), "-seconds", "60", peers.Listen)...)
	if acted.Closed != 1 || !acted.Status.TimedOut {
		t.Fatalf("timeout: %+v", acted)
	}
	waitFor(t, func() bool {
		cli(t, &peers, append([]string{"peers"}, two...)...)
		return len(peers.Peers) == 0
	})
	acted.Status.TimedOut = false // omitted when false
	cli(t, &acted, append(append([]string{"peer", "unblock"}, two...), peers.Listen)...)
	if acted.Status.TimedOut {
		t.Fatalf("unblock: %+v", acted)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
