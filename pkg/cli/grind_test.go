package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// grindLines decodes the JSON lines poo grind run printed.
func grindLines(t *testing.T, out string) []grind.Event {
	t.Helper()
	var evs []grind.Event
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev grind.Event
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatalf("line %q: %v", l, err)
		}
		evs = append(evs, ev)
	}
	return evs
}

func kindsOf(evs []grind.Event) map[string]int {
	m := map[string]int{}
	for _, e := range evs {
		m[e.Kind]++
	}
	return m
}

// TestGrindCommands drives M11 through poo against a running node: the
// devices, a pending job that builds an entry, a tip job that selects it,
// work for a time on a chosen device, start, jobs and stop, an
// interrupted run, view-only mode and the metrics.
func TestGrindCommands(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	stop := startNode(t, root+"/policies/default")
	defer stop()

	var devs devicesReply
	cli(t, &devs, append([]string{"grind", "devices"}, r...)...)
	if devs.Local || len(devs.Devices) == 0 || devs.Devices[0].ID != "cpu" || len(devs.Devices[0].AutoFor) == 0 {
		t.Fatalf("devices %+v", devs)
	}

	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, strings.Repeat("a1", 32)), "-serial", "5"}, r...)...)
	var out syncBuffer
	if err := Run(context.Background(), append([]string{"grind", "run", "pending", "-serial", "5", "-for", "2s", "-threads", "2"}, r...), Stdio{Out: &out, Err: &out}); err != nil {
		t.Fatalf("grind run pending: %v\n%s", err, out.String())
	}
	evs := grindLines(t, out.String())
	k := kindsOf(evs)
	last := evs[len(evs)-1]
	if k["start"] == 0 || k["found"] != 1 || last.Kind != "end" || last.State != grind.StateDone {
		t.Fatalf("pending run:\n%s", out.String())
	}
	for _, e := range evs {
		if e.Plan != nil && (e.Plan.Device != "cpu" || e.Plan.Threads != 2 || e.Plan.Reason == "") {
			t.Fatalf("plan %+v", e.Plan)
		}
	}

	out = syncBuffer{}
	if err := Run(context.Background(), append([]string{"grind", "run", "tip", "-serial", "5", "-for", "2s"}, r...), Stdio{Out: &out, Err: &out}); err != nil {
		t.Fatalf("grind run tip: %v\n%s", err, out.String())
	}
	if k := kindsOf(grindLines(t, out.String())); k["found"] == 0 || k["progress"] == 0 {
		t.Fatalf("tip run:\n%s", out.String())
	}

	// The entry is selected now: work on it for a time, on the CPU.
	var lr rpc.LayerReply
	cli(t, &lr, append([]string{"work", "-serial", "5", "-for", "500ms", "-device", "cpu", "-threads", "1"}, r...)...)
	if lr.Grind == nil || lr.Grind.Device != "cpu" || lr.Grind.Threads != 1 || lr.Grind.Tries == 0 || lr.Grind.Seconds < 0.4 {
		t.Fatalf("work -for %+v", lr.Grind)
	}
	if err := run(append([]string{"work", "-serial", "5", "-for", "1s", "-device", "gpu:9"}, r...), &out); err == nil || !strings.Contains(err.Error(), "gpu:9") {
		t.Fatalf("work on no device: %v", err)
	}
	if err := run(append([]string{"work", "-serial", "5", "-for", "1h"}, r...), &out); err == nil {
		t.Fatal("work for an hour accepted")
	}

	var job grind.Job
	cli(t, &job, append([]string{"grind", "start", "tip", "-serial", "5", "-hours", "0-23"}, r...)...)
	var jobs []grind.Job
	cli(t, &jobs, append([]string{"grind", "jobs"}, r...)...)
	if len(jobs) != 3 || jobs[2].ID != job.ID || jobs[2].Done() {
		t.Fatalf("jobs %+v", jobs)
	}
	var stopped grind.Job
	cli(t, &stopped, "grind", "stop", "-root", root, itoa(job.ID))
	if stopped.State != grind.StateStopped {
		t.Fatalf("stopped %+v", stopped)
	}
	for _, bad := range [][]string{{"grind", "start"}, {"grind", "start", "tips"}, {"grind", "start", "tip", "-until-from", "5"}, {"grind", "stop"}, {"grind", "stop", "1", "-all"}, {"grind", "nope"}} {
		if err := run(append(bad, r...), &out); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}

	// An interrupted run stops its job and exits cleanly.
	out = syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, append([]string{"grind", "run", "tip", "-serial", "5"}, r...), Stdio{Out: &out, Err: &out})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(out.String(), `"progress"`) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("interrupted run: %v", err)
	}
	evs = grindLines(t, out.String())
	if last := evs[len(evs)-1]; last.Kind != "end" || last.State != grind.StateStopped || last.Reason != "interrupted" {
		t.Fatalf("interrupted run:\n%s", out.String())
	}
	cli(t, &jobs, append([]string{"grind", "jobs"}, r...)...)
	for _, j := range jobs {
		if !j.Done() {
			t.Fatalf("a job still runs: %+v", j)
		}
	}

	// View-only: looking is fine, starting is not.
	view := Stdio{Out: &out, Err: &out, ViewOnly: true}
	if err := Run(context.Background(), append([]string{"grind", "jobs"}, r...), view); err != nil {
		t.Fatalf("view-only jobs: %v", err)
	}
	if err := Run(context.Background(), append([]string{"grind", "start", "tip"}, r...), view); err == nil {
		t.Fatal("view-only start accepted")
	}
	// A job with a lease nobody renews stops by itself.
	var leased grind.Job
	cli(t, &leased, append([]string{"grind", "start", "tip", "-serial", "5", "-lease", "1s"}, r...)...)
	deadline = time.Now().Add(10 * time.Second)
	for !leased.Done() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		cli(t, &jobs, append([]string{"grind", "jobs"}, r...)...)
		for _, j := range jobs {
			if j.ID == leased.ID {
				leased = j
			}
		}
	}
	if leased.State != grind.StateStopped || !strings.Contains(leased.Reason, "lease") {
		t.Fatalf("leased job %+v", leased)
	}
	if err := run(append([]string{"grind", "run", "tip", "-lease", "5s"}, r...), &out); err == nil {
		t.Fatal("run -lease accepted")
	}
	if err := run(append([]string{"grind", "start", "tip", "-device", "gpu:9"}, r...), &out); err == nil || !strings.Contains(err.Error(), "gpu:9") {
		t.Fatalf("start on no device: %v", err)
	}

	// The schedule: add jobs and defaults through the running node, which
	// starts the jobs at once; remove them again.
	var sr rpc.ScheduleReply
	cli(t, &sr, append([]string{"grind", "schedule"}, r...)...)
	if len(sr.Grind.Jobs) != 0 {
		t.Fatalf("schedule %+v", sr)
	}
	cli(t, &sr, append([]string{"grind", "schedule", "add", "tip", "-serial", "5", "-hours", "0-23"}, r...)...)
	if len(sr.Grind.Jobs) != 1 || len(sr.Grind.Jobs[0].Pools) != 1 || len(sr.Started) != 1 || sr.Started[0].Spec.Source != "policy" {
		t.Fatalf("schedule add %+v", sr)
	}
	sr = rpc.ScheduleReply{}
	cli(t, &sr, append([]string{"grind", "schedule", "set", "-threads", "1"}, r...)...)
	if sr.Grind.Threads != 1 || len(sr.Started) != 1 {
		t.Fatalf("schedule set %+v", sr)
	}
	for _, bad := range [][]string{{"grind", "schedule", "add", "tip", "-for", "1m"}, {"grind", "schedule", "add"}, {"grind", "schedule", "remove", "2"}, {"grind", "schedule", "remove"}, {"grind", "schedule", "remove", "1", "-target", "pending"}, {"grind", "schedule", "nope"}, {"grind", "schedule", "add", "tx:00"}} {
		if err := run(append(bad, r...), &out); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
	if err := Run(context.Background(), append([]string{"grind", "schedule", "remove", "-all"}, r...), view); err == nil {
		t.Fatal("view-only schedule remove accepted")
	}
	sr = rpc.ScheduleReply{}
	cli(t, &sr, append([]string{"grind", "schedule", "remove", "1", "-target", "tip"}, r...)...)
	if len(sr.Grind.Jobs) != 0 || sr.Grind.Threads != 1 {
		t.Fatalf("schedule remove %+v", sr)
	}
	cli(t, &jobs, append([]string{"grind", "jobs"}, r...)...)
	for _, j := range jobs {
		if !j.Done() {
			t.Fatalf("a job still runs after its schedule went: %+v", j)
		}
	}

	var rows []grind.Bench
	cli(t, &rows, append([]string{"grind", "bench", "-seconds", "0.05"}, r...)...)
	if len(rows) < 5 || rows[0].Rate == 0 || rows[0].Plan.Device != "cpu" {
		t.Fatalf("bench %+v", rows)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// TestGrindScheduleOffline: without a running node, poo grind schedule
// edits node.json, which the node applies when it starts.
func TestGrindScheduleOffline(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	var sr rpc.ScheduleReply
	cli(t, &sr, append([]string{"grind", "schedule", "add", "pending", "-serial", "5"}, r...)...)
	cli(t, &sr, append([]string{"grind", "schedule", "set", "-device", "cpu"}, r...)...)
	sr = rpc.ScheduleReply{}
	cli(t, &sr, append([]string{"grind", "schedule"}, r...)...)
	if sr.Grind.Device != "cpu" || len(sr.Grind.Jobs) != 1 || sr.Grind.Jobs[0].Target != "pending" || len(sr.Grind.Jobs[0].Pools) != 1 {
		t.Fatalf("schedule %+v", sr)
	}
	sr = rpc.ScheduleReply{}
	cli(t, &sr, append([]string{"grind", "schedule", "remove", "-all"}, r...)...)
	cli(t, &sr, append([]string{"grind", "schedule", "set", "-device", "auto"}, r...)...)
	sr = rpc.ScheduleReply{}
	cli(t, &sr, append([]string{"grind", "schedule"}, r...)...)
	if sr.Grind.Device != "" || len(sr.Grind.Jobs) != 0 {
		t.Fatalf("schedule after remove %+v", sr)
	}
}

// flakyRenewer fails the calls listed in fail, answers 404 from gone on,
// and counts calls.
type flakyRenewer struct {
	mu    sync.Mutex
	calls int
	fail  map[int]bool
	gone  int
}

func (f *flakyRenewer) GrindRenew(id int) (grind.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	switch {
	case f.gone > 0 && f.calls >= f.gone:
		return grind.Job{}, &rpc.Error{Message: "no job", Code: rpc.CodeNotFound}
	case f.fail[f.calls]:
		return grind.Job{}, errors.New("connection reset")
	}
	return grind.Job{ID: id, State: grind.StateRunning}, nil
}

func (f *flakyRenewer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// RED-019: a failed renewal is tried again, once warned; a job the node
// no longer knows ends the renewals.
func TestRenewSurvivesAFailure(t *testing.T) {
	f := &flakyRenewer{fail: map[int]bool{2: true, 3: true}, gone: 6}
	var warn strings.Builder
	done := make(chan struct{})
	go func() { renew(context.Background(), f, 7, time.Millisecond, &warn); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("renew did not stop at a gone job")
	}
	if f.count() != 6 {
		t.Fatalf("%d renewals, want 6 (two failures retried, then gone)", f.count())
	}
	if n := strings.Count(warn.String(), "trying again"); n != 1 {
		t.Fatalf("warned %d times: %q", n, warn.String())
	}
}
