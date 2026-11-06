package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDemoRuns(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"demo", "-quiet"}, &out); err != nil {
		t.Fatalf("demo failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "selected again: true") {
		t.Fatalf("unexpected summary: %s", out.String())
	}
}

func TestKeygenDeterministic(t *testing.T) {
	seed := strings.Repeat("ab", 32)
	var a, b bytes.Buffer
	if err := run([]string{"keygen", "-seed", seed}, &a); err != nil {
		t.Fatal(err)
	}
	run([]string{"keygen", "-seed", seed}, &b)
	if a.String() != b.String() {
		t.Fatal("same seed gave different keys")
	}
	var m map[string]string
	if err := json.Unmarshal(a.Bytes(), &m); err != nil || len(m["address_sha256"]) != 64 {
		t.Fatalf("bad output %s", a.String())
	}
	if err := run([]string{"keygen", "-seed", "00"}, &a); err == nil {
		t.Fatal("short seed must fail")
	}
}

func TestSeriesInfo(t *testing.T) {
	var ex bytes.Buffer
	if err := run([]string{"series", "example"}, &ex); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "series.json")
	if err := os.WriteFile(path, ex.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var info bytes.Buffer
	if err := run([]string{"series", "info", path}, &info); err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID    string `json:"series_id"`
		Pools []any  `json:"pools"`
	}
	if err := json.Unmarshal(info.Bytes(), &got); err != nil || len(got.Pools) != 2 || len(got.ID) != 64 {
		t.Fatalf("info %s", info.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	if err := run([]string{"nope"}, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
	var out bytes.Buffer
	run(nil, &out)
	if !strings.Contains(out.String(), "Usage") {
		t.Fatal("usage missing")
	}
}

func TestMainExitCode(t *testing.T) {
	var out, errOut bytes.Buffer
	s := Stdio{Out: &out, Err: &errOut}
	if code := Main(context.Background(), []string{"version"}, s); code != 0 || strings.TrimSpace(out.String()) != Version {
		t.Fatalf("version: code %d, out %q", code, out.String())
	}
	if code := Main(context.Background(), []string{"nope"}, s); code != 1 || !strings.HasPrefix(errOut.String(), `poon: unknown command "nope"`) {
		t.Fatalf("unknown command: code %d, err %q", code, errOut.String())
	}
}

// TestRunReadsIn checks a passphrase comes from the invocation's In, and
// that successive lines of one In are not lost between prompts.
func TestRunReadsIn(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	w := []string{"-root", root, "-wallet", filepath.Join(root, "w.json")}
	var out bytes.Buffer
	in := strings.NewReader("first\n")
	if err := Run(context.Background(), append([]string{"wallet", "create"}, w...), Stdio{In: in, Out: &out}); err != nil {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	in = strings.NewReader("first\nsecond\n")
	if err := Run(context.Background(), append([]string{"wallet", "passwd"}, w...), Stdio{In: in, Out: &out}); err != nil {
		t.Fatalf("passphrase: %v\n%s", err, out.String())
	}
	in = strings.NewReader("first\n")
	if err := Run(context.Background(), append([]string{"wallet", "seed"}, w...), Stdio{In: in, Out: &out}); err == nil {
		t.Fatal("old passphrase still opens the wallet")
	}
	in = strings.NewReader("second\n")
	if err := Run(context.Background(), append([]string{"wallet", "seed"}, w...), Stdio{In: in, Out: &out}); err != nil {
		t.Fatalf("new passphrase: %v\n%s", err, out.String())
	}
	// No In at all reads nothing, so the command fails rather than waits.
	if err := Run(context.Background(), append([]string{"wallet", "seed"}, w...), Stdio{Out: &out}); err == nil {
		t.Fatal("opened without a passphrase")
	}
}

// TestRunNodeStopsOnCancel checks poon node stops when its context is
// cancelled, as it does on an interrupt from a shell.
func TestRunNodeStopsOnCancel(t *testing.T) {
	dir := t.TempDir()
	cli(t, nil, "init", "-example", "-dir", dir, "-listen", "127.0.0.1:0", "-p2p", "off")
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- Run(ctx, []string{"node", "-dir", dir}, Stdio{Out: &out}) }()
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
	var st any
	cli(t, &st, "status", "-dir", dir)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("node did not stop on cancel")
	}
}

// syncBuffer is a bytes.Buffer safe for a writer and a reader at once.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// -h prints the usage and exits 0, with no "flag: help requested", for a
// command, a subcommand and a grouped command (GREEN-032).
func TestHelpFlagExitsZero(t *testing.T) {
	for _, args := range [][]string{{"pools", "-h"}, {"grind", "schedule", "set", "-h"}, {"peer", "block", "--help"}} {
		var out, errOut bytes.Buffer
		code := Main(context.Background(), args, Stdio{Out: &out, Err: &errOut})
		if code != 0 || strings.Contains(errOut.String(), "help requested") || !strings.Contains(errOut.String()+out.String(), "Usage") {
			t.Errorf("%v: exit %d, %q", args, code, errOut.String())
		}
	}
}
