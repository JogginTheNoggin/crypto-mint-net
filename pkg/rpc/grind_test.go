package rpc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// TestGrindEndpoints covers /v1/grind and the grind options of build and
// work (M11): devices, the plan and rate in replies, a job's life, bad
// requests, the view token, and the metrics.
func TestGrindEndpoints(t *testing.T) {
	dir := t.TempDir()
	s := writePolicy(t, dir, "off")
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	tok, _ := rpc.LoadOrCreateToken(dir)
	view, _ := rpc.LoadOrCreateViewToken(dir)
	h := rpc.NewHandler(n, tok)
	h.SetViewToken(view)
	m := grind.NewManager(n, grind.Default, grind.Defaults{Threads: 2}, nil)
	h.SetGrind(m)
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); m.Close(); n.Close() })
	addr := strings.TrimPrefix(srv.URL, "http://")
	c := rpc.NewClient(addr, tok)
	pool := rpc.PoolRef{Series: s.ID(), Pool: 1}

	d, err := c.GrindDevices()
	if err != nil || len(d.Devices) == 0 || d.Devices[0].ID != "cpu" {
		t.Fatalf("devices %+v %v", d, err)
	}
	first, _ := s.PoolRange(1)
	_, err = c.SubmitTx(testutil.NewKeys(s).Claim(first, P("alice", 100)))
	must(t, err)
	er, err := c.BuildWith(pool, 0, rpc.GrindOptions{For: "1s", Threads: 1})
	if err != nil || er.Grind == nil || er.Grind.Device != "cpu" || er.Grind.Threads != 1 {
		t.Fatalf("build %+v %v", er, err)
	}
	lr, err := c.WorkWith(pool, er.Hash, 0, 0, rpc.GrindOptions{For: "300ms"})
	if err != nil || lr.Grind == nil || lr.Grind.Tries == 0 || lr.Grind.Threads != 2 || lr.Grind.Rate == 0 {
		t.Fatalf("work %+v %v", lr, err)
	}
	for _, bad := range []rpc.GrindOptions{{Device: "gpu:9", For: "1s"}, {For: "1h"}, {For: "soon"}, {Threads: -1, For: "1s"}} {
		if _, err := c.WorkWith(pool, er.Hash, 0, 0, bad); code(err) != rpc.CodeBadRequest {
			t.Fatalf("work %+v: %v", bad, err)
		}
	}
	if _, err := c.WorkWith(pool, er.Hash, 0, 0, rpc.GrindOptions{}); code(err) != rpc.CodeBadRequest {
		t.Fatalf("work with no tries and no for: %v", err)
	}

	j, err := c.GrindStart(grind.Spec{Target: grind.TargetTip, For: "500ms"})
	must(t, err)
	deadline := time.Now().Add(10 * time.Second)
	for !j.Done() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		j, err = c.GrindJob(j.ID)
		must(t, err)
	}
	if j.State != grind.StateDone || j.Layers == 0 {
		t.Fatalf("job %+v", j)
	}
	j2, err := c.GrindStart(grind.Spec{Target: grind.TargetTip})
	must(t, err)
	if js, err := c.GrindJobs(); err != nil || len(js) != 2 {
		t.Fatalf("jobs %+v %v", js, err)
	}
	if j2, err = c.GrindStop(j2.ID); err != nil || j2.State != grind.StateStopped {
		t.Fatalf("stop %+v %v", j2, err)
	}
	if _, err := c.GrindStop(99); code(err) != rpc.CodeNotFound {
		t.Fatalf("stop of no job: %v", err)
	}
	if _, err := c.GrindJob(99); code(err) != rpc.CodeNotFound {
		t.Fatalf("no job: %v", err)
	}
	for _, bad := range []grind.Spec{{Target: "tips"}, {Target: grind.TargetTip, Source: "policy"}, {Target: grind.TargetTip, Source: "mine"}} {
		if _, err := c.GrindStart(bad); code(err) != rpc.CodeBadRequest {
			t.Fatalf("start %+v: %v", bad, err)
		}
	}
	if js, err := c.GrindStopAll("user"); err != nil || len(js) != 0 {
		t.Fatalf("stop all %+v %v", js, err)
	}

	// The view token reads jobs and devices but cannot start or stop.
	vc := rpc.NewClient(addr, view)
	if _, err := vc.GrindJobs(); err != nil {
		t.Fatalf("view token jobs: %v", err)
	}
	if _, err := vc.GrindStart(grind.Spec{Target: grind.TargetTip}); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token start: %v", err)
	}
	if _, err := vc.GrindStopAll(""); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token stop: %v", err)
	}

	// Leases and the schedule.
	lj, err := c.GrindStart(grind.Spec{Target: grind.TargetTip, Lease: "1s"})
	must(t, err)
	if lj, err = c.GrindRenew(lj.ID); err != nil || lj.Done() {
		t.Fatalf("renew %+v %v", lj, err)
	}
	if _, err := c.GrindRenew(99); code(err) != rpc.CodeNotFound {
		t.Fatalf("renew of no job: %v", err)
	}
	if _, err := vc.GrindRenew(lj.ID); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token renew: %v", err)
	}
	if sr, err := vc.GrindSchedule(); err != nil || len(sr.Grind.Jobs) != 0 {
		t.Fatalf("schedule %+v %v", sr, err)
	}
	sr, err := c.GrindSetSchedule(nodepolicy.Grind{Threads: 1, Jobs: []nodepolicy.GrindJob{{Target: "tip", Hours: "0-23"}, {Target: "pending", Device: "gpu:9"}}})
	if err != nil || len(sr.Started) != 1 || sr.Failed[1] == "" || sr.Started[0].Spec.Source != "policy" {
		t.Fatalf("set schedule %+v %v", sr, err)
	}
	if sr, err = c.GrindSchedule(); err != nil || len(sr.Grind.Jobs) != 2 || sr.Grind.Threads != 1 {
		t.Fatalf("schedule after set %+v %v", sr, err)
	}
	if _, err := c.GrindSetSchedule(nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "tips"}}}); code(err) != rpc.CodeInvalid {
		t.Fatalf("bad schedule: %v", err)
	}
	// A section of only layer_floor is saved and checked, not dropped
	// (GREEN-032).
	if _, err := c.GrindSetSchedule(nodepolicy.Grind{LayerFloor: 300}); code(err) != rpc.CodeInvalid {
		t.Fatalf("layer_floor 300: %v", err)
	}
	if _, err := c.GrindSetSchedule(nodepolicy.Grind{LayerFloor: 12}); err != nil {
		t.Fatal(err)
	}
	if sr, err = c.GrindSchedule(); err != nil || sr.Grind.LayerFloor != 12 {
		t.Fatalf("layer_floor after set %+v %v", sr, err)
	}
	// So is a section of only layer_rate (D122).
	if _, err := c.GrindSetSchedule(nodepolicy.Grind{LayerRate: 5}); err != nil {
		t.Fatal(err)
	}
	if sr, err = c.GrindSchedule(); err != nil || sr.Grind.LayerRate != 5 {
		t.Fatalf("layer_rate after set %+v %v", sr, err)
	}
	if _, err := vc.GrindSetSchedule(nodepolicy.Grind{}); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token schedule: %v", err)
	}
	if sr, err = c.GrindSetSchedule(nodepolicy.Grind{}); err != nil || len(sr.Started) != 0 {
		t.Fatalf("clear schedule %+v %v", sr, err)
	}
	c.GrindStopAll("")

	req, _ := http.NewRequest("GET", srv.URL+"/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{`poo_grind_tries_total{device="cpu",hash="sha256"}`, `poo_grind_jobs{state="done"} 1`, `poo_grind_jobs{state="stopped"}`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("metrics lack %s:\n%s", want, body)
		}
	}

	// RED-013: two schedules set at once leave node.json and the running
	// policy jobs on the same one.
	one := nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "tip"}}}
	three := nodepolicy.Grind{Threads: 1, Jobs: []nodepolicy.GrindJob{{Target: "tip"}, {Target: "tip"}, {Target: "tip"}}}
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		for _, g := range []nodepolicy.Grind{one, three} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := c.GrindSetSchedule(g); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		sr, err := c.GrindSchedule()
		must(t, err)
		jobs, err := c.GrindJobs()
		must(t, err)
		running := 0
		for _, j := range jobs {
			if !j.Done() && j.Spec.Source == "policy" {
				running++
			}
		}
		if running != len(sr.Grind.Jobs) {
			t.Fatalf("round %d: node.json has %d jobs, %d running", round, len(sr.Grind.Jobs), running)
		}
	}
	c.GrindSetSchedule(nodepolicy.Grind{})
	c.GrindStopAll("")

	// GUI-014 (D125): an add and a remove made at once against the same
	// reading both take effect, a stale removal is refused, and a whole
	// replacement can be tied to the version it was made from.
	tip, night := nodepolicy.GrindJob{Target: "tip"}, nodepolicy.GrindJob{Target: "tip", Hours: "0-23"}
	if _, err := c.GrindEditSchedule(nodepolicy.GrindEdit{Add: []nodepolicy.GrindJob{tip, night}}); err != nil {
		t.Fatal(err)
	}
	read, err := c.GrindSchedule()
	must(t, err)
	var wg sync.WaitGroup
	for _, e := range []nodepolicy.GrindEdit{
		{Remove: []nodepolicy.GrindRemove{{N: 1, Job: tip}}},
		{Add: []nodepolicy.GrindJob{{Target: "pending"}}},
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.GrindEditSchedule(e); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	sr, err = c.GrindSchedule()
	if err != nil || len(sr.Grind.Jobs) != 2 || sr.Grind.Jobs[0].Hours != "0-23" || sr.Grind.Jobs[1].Target != "pending" {
		t.Fatalf("after edits at once %+v %v", sr, err)
	}
	if _, err := c.GrindEditSchedule(nodepolicy.GrindEdit{Remove: []nodepolicy.GrindRemove{{N: 1, Job: tip}}}); code(err) != rpc.CodeConflict {
		t.Fatalf("stale remove: %v", err)
	}
	if _, err := c.GrindReplaceSchedule(nodepolicy.Grind{}, read.Version); code(err) != rpc.CodeConflict {
		t.Fatalf("replace of an old version: %v", err)
	}
	if _, err := c.GrindReplaceSchedule(nodepolicy.Grind{}, sr.Version); err != nil {
		t.Fatalf("replace of the current version: %v", err)
	}
	if _, err := vc.GrindEditSchedule(nodepolicy.GrindEdit{RemoveAll: true}); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token edit: %v", err)
	}
	c.GrindStopAll("")
}

// TestGrindWithoutManager: an interface with no grind manager still
// lists devices and grinds build and work, and says it runs no jobs.
func TestGrindWithoutManager(t *testing.T) {
	f := newFixture(t)
	if d, err := f.c.GrindDevices(); err != nil || len(d.Devices) == 0 {
		t.Fatalf("devices %+v %v", d, err)
	}
	if _, err := f.c.GrindJobs(); code(err) != rpc.CodeNotFound {
		t.Fatalf("jobs: %v", err)
	}
}
