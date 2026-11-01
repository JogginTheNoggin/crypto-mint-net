package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// results maps each check's name to its result.
func results(r DoctorReport) map[string]string {
	m := map[string]string{}
	for _, c := range r.Checks {
		m[c.Name] = c.Result
	}
	return m
}

func wantResults(t *testing.T, r DoctorReport, want map[string]string) {
	t.Helper()
	got := results(r)
	for n, w := range want {
		if got[n] != w {
			t.Errorf("%s: %s, want %s\n%+v", n, got[n], w, r.Checks)
		}
	}
}

func checkNamed(r DoctorReport, name string) Check {
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	return Check{}
}

func TestDoctorNoPolicy(t *testing.T) {
	r := doctor(io.Discard, t.TempDir())
	if r.OK {
		t.Fatal("ok without a policy")
	}
	wantResults(t, r, map[string]string{"policy": checkFail, "rpc": checkSkip, "p2p": checkSkip})
	if !strings.Contains(r.Checks[0].Fix, "poo init") {
		t.Errorf("fix %q", r.Checks[0].Fix)
	}
}

func TestDoctorBadPolicy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "node.json"), []byte(`{"format": 99}`), 0o600)
	r := doctor(io.Discard, dir)
	wantResults(t, r, map[string]string{"policy": checkFail})
}

func TestDoctorStoppedNode(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o700) // as poo init makes it under -root
	cli(t, nil, "init", "-example", "-dir", dir, "-p2p", "off")
	r := doctor(io.Discard, dir)
	if !r.OK {
		t.Fatalf("a stopped node is a warning, not a failure: %+v", r.Checks)
	}
	wantResults(t, r, map[string]string{"policy": checkOK, "files": checkOK, "series": checkOK, "grind": checkOK, "rpc": checkWarn, "p2p": checkSkip})
	if c := checkNamed(r, "rpc"); len(c.FixArgs) != 1 || c.FixArgs[0] != "node" {
		t.Errorf("rpc fix_args %+v", c)
	}

	// A leftover rpc.addr from a crash: nothing answers there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	os.WriteFile(filepath.Join(dir, "rpc.addr"), []byte(addr+"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "rpc.token"), []byte(strings.Repeat("ab", 32)+"\n"), 0o600)
	r = doctor(io.Discard, dir)
	wantResults(t, r, map[string]string{"rpc": checkFail})
	if c := checkNamed(r, "rpc"); !strings.Contains(c.Detail, "left over") {
		t.Errorf("stale rpc.addr: %+v", c)
	}

	if runtime.GOOS != "windows" {
		os.Chmod(filepath.Join(dir, "rpc.token"), 0o644)
		wantResults(t, doctor(io.Discard, dir), map[string]string{"files": checkWarn})
	}
}

func TestDoctorRunningNode(t *testing.T) {
	dir := t.TempDir()
	cli(t, nil, "init", "-example", "-dir", dir, "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- Run(ctx, []string{"node", "-dir", dir}, Stdio{Out: &out}) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "rpc.addr")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("node did not start\n%s", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	var b bytes.Buffer
	if err := run([]string{"doctor", "-dir", dir, "-json"}, &b); err != nil {
		t.Fatalf("doctor: %v\n%s", err, b.String())
	}
	var r DoctorReport
	if err := json.Unmarshal(b.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	// p2p listens, but no peers or registries are set: a warning.
	wantResults(t, r, map[string]string{"policy": checkOK, "rpc": checkOK, "p2p": checkWarn})
	if d := checkNamed(r, "p2p").Detail; !strings.Contains(d, "accepting on") {
		t.Errorf("p2p detail %q", d)
	}

	// The text form says the same.
	b.Reset()
	if err := run([]string{"doctor", "-dir", dir}, &b); err != nil || !strings.Contains(b.String(), "ok    rpc") {
		t.Fatalf("doctor text: %v\n%s", err, b.String())
	}
}

// TestDoctorOutputAndGrind: -o json is -json, -o table works, fix_args
// names the fix's command, and a grind device that is not there fails.
func TestDoctorOutputAndGrind(t *testing.T) {
	var j1, j2 DoctorReport
	dir := t.TempDir()
	os.Chmod(dir, 0o700)
	_ = run([]string{"doctor", "-json", "-dir", dir}, jsonInto(t, &j1))
	_ = run([]string{"doctor", "-o", "json", "-dir", dir}, jsonInto(t, &j2))
	if len(j1.Checks) == 0 || len(j1.Checks) != len(j2.Checks) || len(j1.Checks[0].FixArgs) != 1 || j1.Checks[0].FixArgs[0] != "init" {
		t.Fatalf("-json %+v, -o json %+v", j1, j2)
	}
	var tb bytes.Buffer
	_ = run([]string{"doctor", "-o", "table", "-dir", dir}, &tb)
	if !strings.Contains(tb.String(), "policy") || strings.HasPrefix(strings.TrimSpace(tb.String()), "{") {
		t.Fatalf("table:\n%s", tb.String())
	}
	if err := run([]string{"doctor", "-o", "yaml", "-dir", dir}, &tb); err == nil {
		t.Fatal("-o yaml accepted")
	}
	cli(t, nil, "init", "-example", "-dir", dir, "-p2p", "off")
	p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	must(t, err)
	p.Grind = &nodepolicy.Grind{Device: "gpu:42"}
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))
	if c := checkNamed(doctor(io.Discard, dir), "grind"); c.Result != checkFail || !strings.Contains(c.Fix, "cpu") {
		t.Fatalf("grind check %+v", c)
	}
}

// jsonInto is a writer that decodes what is written into v at the end.
func jsonInto(t *testing.T, v any) *jsonWriter { return &jsonWriter{t: t, v: v} }

type jsonWriter struct {
	t *testing.T
	v any
}

func (w *jsonWriter) Write(p []byte) (int, error) {
	if err := json.Unmarshal(p, w.v); err != nil {
		w.t.Fatalf("%v: %s", err, p)
	}
	return len(p), nil
}

// GREEN-035 and the GUI's ask: a value Load lowered is a warning whose
// fix_args writes the value used, and running them clears it.
func TestDoctorLoweredHasFixArgs(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o700)
	cli(t, nil, "init", "-example", "-dir", dir, "-p2p", "off")
	path := filepath.Join(dir, "node.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	raw["grind"] = map[string]any{"layer_rate": 1000}
	b, _ = json.Marshal(raw)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	var lowered Check
	for _, c := range doctor(io.Discard, dir).Checks {
		if c.Name == "policy" && c.Result == checkWarn {
			lowered = c
		}
	}
	want := []string{"grind", "schedule", "set", "-layer-rate", "50"}
	if strings.Join(lowered.FixArgs, " ") != strings.Join(want, " ") || !strings.Contains(lowered.Detail, "layer_rate 1000") {
		t.Fatalf("lowered check %+v", lowered)
	}
	cli(t, nil, append(lowered.FixArgs, "-dir", dir)...)
	for _, c := range doctor(io.Discard, dir).Checks {
		if c.Name == "policy" && c.Result == checkWarn {
			t.Fatalf("still lowered after the fix: %+v", c)
		}
	}
}
