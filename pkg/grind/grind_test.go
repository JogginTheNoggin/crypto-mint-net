package grind

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func TestDefaultPlans(t *testing.T) {
	devs := Default.Detect()
	if len(devs) == 0 || devs[0].ID != "cpu" || devs[0].Threads < 1 {
		t.Fatalf("devices %+v", devs)
	}
	for _, id := range hash.IDs() {
		if !slices.Contains(devs[0].Hashes, id.Name()) || !slices.Contains(devs[0].AutoFor, id.Name()) {
			t.Fatalf("cpu lacks a fast grinder for %s: %+v", id.Name(), devs[0])
		}
		p, err := Default.Choose(devs, id, "", 0)
		if err != nil || p.Device != "cpu" || p.Fallback || p.Threads != max(1, devs[0].Threads-1) || p.Reason == "" {
			t.Fatalf("plan for %s: %+v %v", id.Name(), p, err)
		}
	}
	if _, err := Default.Choose(devs, hash.SHA256, "gpu:99", 0); err == nil || !strings.Contains(err.Error(), "auto") {
		t.Fatalf("unknown device: %v", err)
	}
	if p, _ := Default.Choose(devs, hash.SHA256, "cpu", 1<<20); p.Threads > devs[0].Threads && p.Threads != 0 {
		t.Fatalf("threads above the device's: %d", p.Threads)
	}
}

// TestEveryGrinderAgrees: the fast and the generic CPU grinders find the
// same layer as package layer's plain search, on one thread and several.
func TestEveryGrinderAgrees(t *testing.T) {
	r := &Registry{}
	r.RegisterProbe(probeCPU)
	r.Register(Grinder{Name: "cpu-generic", Kind: CPU, New: genericCPU})
	devs := r.Detect()
	var entry hash.Hash
	entry[0] = 7
	for _, id := range hash.IDs() {
		f, _ := hash.Lookup(id)
		want, ws := layer.Grind(f, entry, 100, 3000)
		gen, err := r.Choose(devs, id, "cpu", 3)
		if err != nil || !gen.Fallback {
			t.Fatalf("generic plan %+v %v", gen, err)
		}
		fast, _ := Default.Choose(Default.Detect(), id, "cpu", 3)
		for _, p := range []Plan{gen, fast} {
			got, gs, err := layer.GrindWith(context.Background(), p.newHasher, entry, 100, 3000, p.Threads, nil)
			if err != nil || got != want || gs != ws {
				t.Fatalf("%s %s: %+v/%d, want %+v/%d (%v)", id.Name(), p.Grinder, got, gs, want, ws, err)
			}
		}
	}
}

// countHasher counts the tries of a test device's grinder.
type countHasher struct {
	h    hash.Hasher
	uses *atomic.Int64
}

func (c countHasher) Hash(s []byte) hash.Hash { c.uses.Add(1); return c.h.Hash(s) }

// TestAnotherDevice: a probe and a grinder registered for another kind of
// device make auto pick it for its hash, and only for its hash; a device
// without a grinder is listed with a note and never picked.
func TestAnotherDevice(t *testing.T) {
	r := newDefault()
	r.RegisterProbe(func() []Device {
		return []Device{{ID: "gpu:0", Kind: GPU, Name: "test"}, {ID: "npu:0", Kind: "npu", Name: "other"}}
	})
	var uses atomic.Int64
	r.Register(Grinder{Name: "gpu-test", Kind: GPU, Hash: hash.SHA256, New: func(_ Device, id hash.ID) (hash.NewHasher, error) {
		return func(prefix ...[]byte) (hash.Hasher, error) {
			g, err := hash.NewGrinder(id, prefix...)
			return countHasher{g, &uses}, err
		}, nil
	}})
	devs := r.Detect()
	byID := map[string]Device{}
	for _, d := range devs {
		byID[d.ID] = d
	}
	if !slices.Equal(byID["gpu:0"].AutoFor, []string{"sha256"}) || byID["npu:0"].Note == "" || len(byID["npu:0"].AutoFor) != 0 {
		t.Fatalf("devices %+v", devs)
	}
	p, err := r.Choose(devs, hash.SHA256, Auto, 2)
	if err != nil || p.Device != "gpu:0" || p.Grinder != "gpu-test" {
		t.Fatalf("auto for sha256: %+v %v", p, err)
	}
	var entry hash.Hash
	if _, _, err := layer.GrindWith(context.Background(), p.newHasher, entry, 0, 100, p.Threads, nil); err != nil || uses.Load() != 100 {
		t.Fatalf("the device's grinder ran %d times: %v", uses.Load(), err)
	}
	if p, _ := r.Choose(devs, hash.BLAKE3, Auto, 0); p.Device != "cpu" {
		t.Fatalf("auto for blake3: %+v", p)
	}
	if _, err := r.Choose(devs, hash.BLAKE3, "gpu:0", 0); err == nil {
		t.Fatal("a device without a grinder for the hash was accepted")
	}
}

func TestProbeGPUs(t *testing.T) {
	dir := t.TempDir()
	old := sysClass
	sysClass = dir
	defer func() { sysClass = old }()
	for _, c := range []struct{ name, vendor, driver string }{{"card0", "0x10de", "nvidia"}, {"card0-HDMI-A-1", "", ""}, {"card1", "0x1002", "amdgpu"}} {
		d := filepath.Join(dir, c.name, "device")
		os.MkdirAll(d, 0o755)
		if c.vendor != "" {
			os.WriteFile(filepath.Join(d, "vendor"), []byte(c.vendor+"\n"), 0o644)
			os.WriteFile(filepath.Join(d, "uevent"), []byte("DRIVER="+c.driver+"\nPCI_ID=x\n"), 0o644)
		}
	}
	got := probeGPUs()
	if len(got) != 2 || got[0].ID != "gpu:0" || got[0].Name != "NVIDIA (nvidia)" || got[1].Name != "AMD (amdgpu)" {
		t.Fatalf("gpus %+v", got)
	}
	devs := Default.Detect()
	for _, d := range devs {
		if d.Kind == GPU && (d.Note == "" || len(d.AutoFor) != 0) {
			t.Fatalf("a gpu without a grinder: %+v", d)
		}
	}
}

func TestHours(t *testing.T) {
	for _, bad := range []string{"22", "22-22", "24-01", "a-b", "-1-3"} {
		if _, _, err := ParseHours(bad); err == nil {
			t.Fatalf("hours %q accepted", bad)
		}
	}
	from, to, err := ParseHours("22-06")
	if err != nil {
		t.Fatal(err)
	}
	at := func(h int) time.Time { return time.Date(2026, 1, 1, h, 30, 0, 0, time.Local) }
	for h, want := range map[int]bool{21: false, 22: true, 23: true, 0: true, 5: true, 6: false, 12: false} {
		if inHours(at(h), from, to) != want {
			t.Fatalf("hour %d in 22-06: %v", h, !want)
		}
	}
	if f, tt, _ := ParseHours("9-17"); !inHours(at(9), f, tt) || inHours(at(17), f, tt) {
		t.Fatal("9-17")
	}
}

func TestCheck(t *testing.T) {
	for _, bad := range []Spec{{}, {Target: "tips"}, {Target: TargetTip, UntilFrom: "5"}, {Target: TargetTip, For: "soon"}, {Target: TargetTip, Hours: "1"}, {Target: TargetPending, Threads: -1}} {
		if Check(bad) == nil {
			t.Fatalf("spec %+v accepted", bad)
		}
	}
	if err := Check(Spec{Target: hash.Hash{1}.String(), UntilFrom: "1000", For: "1m", Hours: "22-06"}); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	n *node.Node
	s *series.Series
	k *testutil.Keys
	m *Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "off"
	p.NoSync = true
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(n, Default, Defaults{Threads: 2}, nil)
	t.Cleanup(func() { m.Close(); n.Close() })
	return &fixture{n: n, s: s, k: testutil.NewKeys(s), m: m}
}

func (f *fixture) waitFor(t *testing.T, id int, cond func(Job) bool) Job {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := f.m.Get(id)
		if ok && cond(j) {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := f.m.Get(id)
	t.Fatalf("job %+v did not get there", j)
	return j
}

// TestJobs runs each kind of job on a real node: pending builds an entry,
// tip adds layers to it, an entry job stops at its from(e), stop and a
// time limit end jobs, and the events say so.
func TestJobs(t *testing.T) {
	f := newFixture(t)
	var events []Event
	evc := make(chan Event, 1024)
	f.m.OnEvent(func(e Event) {
		select {
		case evc <- e:
		default:
		}
	})
	first, _ := f.s.PoolRange(1)
	if _, err := f.n.HandleTx(f.k.Claim(first, testutil.P("alice", 100)), engine.Local); err != nil {
		t.Fatal(err)
	}
	sid := f.s.ID()
	pj, err := f.m.Start(Spec{Target: TargetPending, Series: &sid})
	if err != nil {
		t.Fatal(err)
	}
	pj = f.waitFor(t, pj.ID, func(j Job) bool { return j.Entries == 1 && j.State == StateWaiting })
	if len(pj.Plans) != 1 || pj.Plans[0].Device != "cpu" || pj.Plans[0].Threads != 2 {
		t.Fatalf("pending job %+v", pj)
	}
	f.m.Stop(pj.ID)
	key := engine.PoolKey{Series: sid, Index: 1}
	st, _ := f.n.Engine().Status(key)
	if st.Height != 0 || len(st.Branches) != 1 {
		t.Fatalf("status %+v", st)
	}
	built := st.Branches[0].Tip

	tj, err := f.m.Start(Spec{Target: TargetTip, Pools: []uint64{1}, For: "3s"})
	if err != nil {
		t.Fatal(err)
	}
	tj = f.waitFor(t, tj.ID, func(j Job) bool { return j.Done() })
	if tj.State != StateDone || tj.Layers < 1 || tj.Tries == 0 || tj.Rate == 0 {
		t.Fatalf("tip job %+v", tj)
	}
	// Its layers made the entry it built selected.
	if st, _ = f.n.Engine().Status(key); st.Height != 1 || st.Tip != built {
		t.Fatalf("status after the tip job %+v", st)
	}

	// An entry job ends once from(e) reaches its goal, which the tip job's
	// layers already passed.
	ej, err := f.m.Start(Spec{Target: st.Tip.String(), UntilFrom: "2"})
	if err != nil {
		t.Fatal(err)
	}
	ej = f.waitFor(t, ej.ID, func(j Job) bool { return j.Done() })
	if ej.State != StateDone || ej.From == "" {
		t.Fatalf("entry job %+v", ej)
	}
	// A job on an entry not held waits; stop ends it.
	wj, _ := f.m.Start(Spec{Target: hash.Hash{9}.String()})
	f.waitFor(t, wj.ID, func(j Job) bool { return j.State == StateWaiting })
	if j, err := f.m.Stop(wj.ID); err != nil || j.State != StateStopped {
		t.Fatalf("stopped %+v %v", j, err)
	}
	if _, err := f.m.Stop(999); err != ErrNoJob {
		t.Fatalf("stop of no job: %v", err)
	}
	for len(evc) > 0 {
		events = append(events, <-evc)
	}
	kinds := map[string]bool{}
	for _, e := range events {
		kinds[e.Kind] = true
	}
	for _, k := range []string{"start", "progress", "found", "waiting", "end"} {
		if !kinds[k] {
			t.Fatalf("no %s event in %+v", k, events)
		}
	}
	tries, states := f.m.Totals()
	if tries[[2]string{"cpu", "sha256"}] == 0 || states[StateDone] != 2 || states[StateStopped] != 2 {
		t.Fatalf("totals %v %v", tries, states)
	}
}

func TestJobLimits(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < MaxJobs; i++ {
		if _, err := f.m.Start(Spec{Target: hash.Hash{byte(i + 1)}.String()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.m.Start(Spec{Target: TargetTip}); err != ErrTooManyJobs {
		t.Fatalf("job %d: %v", MaxJobs+1, err)
	}
	if got := f.m.StopAll(""); len(got) != MaxJobs {
		t.Fatalf("stopped %d", len(got))
	}
	if _, err := f.m.Start(Spec{Target: TargetTip, Device: "gpu:42"}); err != nil {
		t.Fatal(err) // a missing device fails the job, not the start
	}
	f.m.Close()
	if _, err := f.m.Start(Spec{Target: TargetTip}); err != ErrClosed {
		t.Fatalf("start after close: %v", err)
	}
}

func TestBench(t *testing.T) {
	p, err := Default.Choose(Default.Detect(), hash.SHA256, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunBench(context.Background(), p, 100*time.Millisecond)
	if err != nil || b.Tries == 0 || b.Rate == 0 || b.Seconds < 0.09 {
		t.Fatalf("bench %+v %v", b, err)
	}
}
