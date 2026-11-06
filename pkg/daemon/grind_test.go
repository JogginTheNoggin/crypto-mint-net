package daemon_test

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// TestPolicyGrindJobs: a node with a grind section runs its jobs by
// itself, with the section's threads, and they build and select an entry
// for a pushed transaction (pending, then tip); their events are on the
// watch stream; a job the policy lists out of hours waits.
func TestPolicyGrindJobs(t *testing.T) {
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "127.0.0.1:0"
	p.P2P.Listen = "off"
	p.NoSync = true
	now := time.Now().Hour()
	p.Grind = &nodepolicy.Grind{Threads: 1, Jobs: []nodepolicy.GrindJob{
		{Target: "pending"},
		{Target: "tip", Series: &id},
		{Target: "tip", Hours: hoursWithout(now)},
	}}
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	var log logBuf
	go func() { done <- daemon.Run(ctx, dir, daemon.Options{Log: &log, Ready: func(a string) { ready <- a }}) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("node stopped: %v\n%s", err, log.String())
	case <-time.After(10 * time.Second):
		t.Fatal("node not up")
	}
	c, err := rpc.Dial(dir)
	must(t, err)
	found := make(chan grind.Event, 64)
	go c.Watch(ctx, filter.Filter{Kinds: []filter.Kind{filter.KindGrind}}, func(ev watch.Event) error {
		if ev.Grind != nil && ev.Grind.Kind == "found" {
			select {
			case found <- *ev.Grind:
			default:
			}
		}
		return nil
	})
	first, _ := s.PoolRange(1)
	time.Sleep(100 * time.Millisecond) // the watcher connects
	_, err = c.SubmitTx(testutil.NewKeys(s).Claim(first, testutil.P("alice", 100)))
	must(t, err)
	deadline := time.Now().Add(20 * time.Second)
	var st rpc.StatusReply
	for time.Now().Before(deadline) {
		st, err = c.Status()
		must(t, err)
		if len(st.Pools) == 1 && st.Pools[0].Height == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(st.Pools) != 1 || st.Pools[0].Height != 1 {
		t.Fatalf("the jobs did not select an entry: %+v\n%s", st, log.String())
	}
	select {
	case <-found:
	case <-time.After(5 * time.Second):
		t.Fatal("no found event on the watch stream")
	}
	// The tip job may reach the entry a moment after the pending job's
	// layers selected it; wait for its first layer.
	var jobs []grind.Job
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		jobs, err = c.GrindJobs()
		must(t, err)
		if len(jobs) != 3 || jobs[1].Layers > 0 || time.Now().After(end) {
			break
		}
	}
	if len(jobs) != 3 || jobs[0].Spec.Source != "policy" || jobs[0].Entries != 1 || jobs[1].Layers == 0 || jobs[2].State != grind.StateWaiting {
		t.Fatalf("jobs %+v", jobs)
	}
	for _, j := range jobs[:2] {
		if len(j.Plans) == 0 || j.Plans[0].Threads != 1 {
			t.Fatalf("job %d plans %+v", j.ID, j.Plans)
		}
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// hoursWithout returns a two-hour window that does not hold hour h.
func hoursWithout(h int) string {
	from := (h + 2) % 24
	to := (h + 4) % 24
	return itoa(from) + "-" + itoa(to)
}

func itoa(n int) string { return strconv.Itoa(n) }
