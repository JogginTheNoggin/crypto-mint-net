package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestDoctorNoPolicy(t *testing.T) {
	r := doctor(t.TempDir())
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
	r := doctor(dir)
	wantResults(t, r, map[string]string{"policy": checkFail})
}

func TestDoctorStoppedNode(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o700) // as poo init makes it under -root
	cli(t, nil, "init", "-example", "-dir", dir, "-p2p", "off")
	r := doctor(dir)
	if !r.OK {
		t.Fatalf("a stopped node is a warning, not a failure: %+v", r.Checks)
	}
	wantResults(t, r, map[string]string{"policy": checkOK, "files": checkOK, "series": checkOK, "rpc": checkWarn, "p2p": checkSkip})

	// A leftover rpc.addr from a crash: nothing answers there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	os.WriteFile(filepath.Join(dir, "rpc.addr"), []byte(addr+"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "rpc.token"), []byte(strings.Repeat("ab", 32)+"\n"), 0o600)
	r = doctor(dir)
	wantResults(t, r, map[string]string{"rpc": checkFail})
	if !strings.Contains(r.Checks[3].Detail, "left over") {
		t.Errorf("stale rpc.addr: %+v", r.Checks[3])
	}

	if runtime.GOOS != "windows" {
		os.Chmod(filepath.Join(dir, "rpc.token"), 0o644)
		wantResults(t, doctor(dir), map[string]string{"files": checkWarn})
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
	if d := r.Checks[4].Detail; !strings.Contains(d, "accepting on") {
		t.Errorf("p2p detail %q", d)
	}

	// The text form says the same.
	b.Reset()
	if err := run([]string{"doctor", "-dir", dir}, &b); err != nil || !strings.Contains(b.String(), "ok    rpc") {
		t.Fatalf("doctor text: %v\n%s", err, b.String())
	}
}
