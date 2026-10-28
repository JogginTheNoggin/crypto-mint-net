package rpc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
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
	for _, bad := range []grind.Spec{{Target: "tips"}, {Target: grind.TargetTip, Source: "policy"}} {
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

	req, _ := http.NewRequest("GET", srv.URL+"/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{`poo_grind_tries_total{device="cpu",hash="sha256"}`, `poo_grind_jobs{state="done"} 1`, `poo_grind_jobs{state="stopped"} 1`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("metrics lack %s:\n%s", want, body)
		}
	}
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
