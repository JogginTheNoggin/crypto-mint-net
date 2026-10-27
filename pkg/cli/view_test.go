package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestViewFollowAndWatch(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	stop := startNode(t, root+"/policies/default")
	defer stop()
	id := testutil.ExampleSeries().ID().String()

	// A watcher of mint 5's transactions, stopped by cancelling it.
	ctx, cancel := context.WithCancel(context.Background())
	var events syncBuf
	wdone := make(chan error, 1)
	go func() {
		wdone <- Run(ctx, append([]string{"watch", "-notices", "-serial", "5", "-kind", "tx,selected"}, r...), Stdio{Out: &events, Err: &events})
	}()
	// seen lists the kinds and tids printed so far, pings left out.
	seen := func() (out []string) {
		for _, l := range strings.Split(strings.TrimSpace(events.String()), "\n") {
			var ev struct{ Kind, TID string }
			if json.Unmarshal([]byte(l), &ev) == nil && ev.Kind != "ping" {
				out = append(out, ev.Kind+" "+ev.TID)
			}
		}
		return out
	}
	waitFor := func(n int) []string {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && len(seen()) < n {
			time.Sleep(20 * time.Millisecond)
		}
		got := seen()
		if len(got) != n {
			t.Fatalf("watch printed %d lines, want %d:\n%s", len(got), n, events.String())
		}
		return got
	}
	waitFor(1) // hello: connected

	alice := strings.Repeat("a1", 32)
	var claim struct{ TID string }
	cli(t, &claim, append([]string{"claim", "-seed-file", seedFile(t, alice), "-serial", "5"}, r...)...)
	var pend []struct{ TID string }
	cli(t, &pend, append([]string{"pending", "-serial", "5"}, r...)...)
	if len(pend) != 1 || pend[0].TID != claim.TID {
		t.Fatalf("pending %+v", pend)
	}
	cli(t, &pend, append([]string{"pending", "-pool", "0"}, r...)...)
	if len(pend) != 0 {
		t.Fatalf("pending in pool 0 %+v", pend)
	}
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1"}, r...)...)
	cli(t, nil, append([]string{"work", "-pool", "1", "-entry", built.Hash, "-tries", "4000"}, r...)...)

	var lp struct {
		Height  int
		Entries []struct {
			Hash string
			Txs  []struct{ TID string }
		}
	}
	cli(t, &lp, append([]string{"ledger", "-pool", "1"}, r...)...)
	if lp.Height != 1 || lp.Entries[0].Hash != built.Hash || lp.Entries[0].Txs[0].TID != claim.TID {
		t.Fatalf("ledger %+v", lp)
	}
	var ts struct {
		Selected bool
		Tx       struct{ Outputs []struct{ Address string } }
	}
	cli(t, &ts, append(append([]string{"tx"}, r...), claim.TID)...)
	if !ts.Selected {
		t.Fatalf("tx %+v", ts)
	}
	addr := ts.Tx.Outputs[0].Address
	var a struct{ Balance uint64 }
	cli(t, &a, append(append([]string{"address"}, r...), addr)...)
	if a.Balance != 100 {
		t.Fatalf("address %+v", a)
	}
	var un []struct{ Amount uint64 }
	cli(t, &un, append([]string{"unspent", "-address", addr}, r...)...)
	if len(un) != 1 {
		t.Fatalf("unspent %+v", un)
	}

	// The watcher saw the claim arrive and be selected.
	got := waitFor(3)
	if got[1] != "tx "+claim.TID || got[2] != "selected "+claim.TID {
		t.Fatalf("watch printed %v", got)
	}

	// Unfollow pool 0 on the running node; the watcher reconnects.
	var fr struct {
		Pools []struct{ Pool uint64 }
	}
	cli(t, &fr, append([]string{"unfollow", "-series", id, "-pool", "0"}, r...)...)
	if len(fr.Pools) != 1 || fr.Pools[0].Pool != 1 {
		t.Fatalf("after unfollow %+v", fr)
	}
	cli(t, &fr, append([]string{"follow"}, r...)...)
	if len(fr.Pools) != 1 {
		t.Fatalf("follow shows %+v", fr)
	}
	var b bytes.Buffer
	if err := run(append([]string{"claim", "-seed-file", seedFile(t, alice), "-serial", "1"}, r...), &b); err == nil {
		t.Fatal("claim in an unfollowed pool accepted")
	}
	// The stream ended for the reload and came back: end, hello. Events
	// while the node reloaded are not replayed; nothing of mint 5 happened.
	got = waitFor(5)
	if got[3] != "end " || got[4] != "hello " {
		t.Fatalf("around the reload: %v", got)
	}
	var pay struct{ TID string }
	cli(t, &pay, append([]string{"pay", "-seed-file", seedFile(t, alice), "-serial", "5", "-to", addr, "-amount", "10"}, r...)...)
	if got = waitFor(6); got[5] != "tx "+pay.TID {
		t.Fatalf("after the reload: %v", got)
	}
	cancel()
	if err := <-wdone; err != nil {
		t.Fatalf("watch: %v", err)
	}
	cli(t, nil, append([]string{"reload"}, r...)...)
	for _, bad := range [][]string{{"ledger"}, {"tx"}, {"unfollow"}, {"pending", "-kind", "x"}, {"follow", "-series", "zz"}} {
		if err := run(append(bad, r...), &b); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}
