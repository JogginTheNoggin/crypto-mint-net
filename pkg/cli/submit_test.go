package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// TestSubmitEntryAndLayer: an entry and a layer made by one node, in the
// hex its API prints, are handed to another with poo submit, which
// selects them as a peer's would be (GREEN-038, D143).
func TestSubmitEntryAndLayer(t *testing.T) {
	rootA, rootB := t.TempDir(), t.TempDir()
	for _, root := range []string{rootA, rootB} {
		cli(t, nil, "init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0", "-root", root)
	}
	dirA, dirB := rootA+"/policies/default", rootB+"/policies/default"
	stopA := startNode(t, dirA)
	defer stopA()
	stopB := startNode(t, dirB)
	defer stopB()

	alice := strings.Repeat("a1", 32)
	cli(t, nil, "claim", "-seed-file", seedFile(t, alice), "-serial", "5", "-root", rootA)
	a, err := rpc.Dial(dirA)
	if err != nil {
		t.Fatal(err)
	}
	known, err := a.Series()
	if err != nil || len(known) != 1 {
		t.Fatalf("series %v %v", known, err)
	}
	ref := rpc.PoolRef{Series: known[0].ID, Pool: 1}
	built, err := a.Build(ref, 0)
	if err != nil || built.Entry == "" {
		t.Fatalf("build %+v %v", built, err)
	}
	worked, err := a.Work(ref, built.Hash, 0, 4000)
	if err != nil || worked.Layer == "" {
		t.Fatalf("work %+v %v", worked, err)
	}

	var got struct {
		Hash  string
		Entry string
	}
	cli(t, &got, "submit", "entry", "-pool", "1", "-root", rootB, built.Entry)
	if got.Hash != built.Hash.String() || got.Entry != "" {
		t.Fatalf("submit entry %+v, want hash %s and no bytes", got, built.Hash)
	}
	var lay struct {
		Hash     string
		Strength int
		Switched bool
	}
	cli(t, &lay, "submit", "layer", "-serial", "5", "-root", rootB, worked.Layer)
	if lay.Hash != worked.Hash.String() || !lay.Switched {
		t.Fatalf("submit layer %+v, want %+v", lay, worked)
	}

	// The same with poo alone: build -bytes -work prints the entry and its
	// layer for poo submit on the other node (GREEN-039).
	var both struct {
		Hash  string
		Entry string
		Layer struct {
			Hash  string
			Layer string
		}
	}
	cli(t, nil, "pay", "-seed-file", seedFile(t, alice), "-serial", "5", "-to", strings.Repeat("b2", 32), "-amount", "1", "-root", rootA)
	cli(t, &both, "build", "-pool", "1", "-bytes", "-work", "4000", "-root", rootA)
	if both.Entry == "" || both.Layer.Layer == "" {
		t.Fatalf("build -bytes -work %+v", both)
	}
	got.Hash = ""
	cli(t, &got, "submit", "entry", "-pool", "1", "-root", rootB, both.Entry)
	cli(t, &lay, "submit", "layer", "-pool", "1", "-root", rootB, both.Layer.Layer)
	if got.Hash != both.Hash || lay.Hash != both.Layer.Hash {
		t.Fatalf("submitted %+v and %+v, built %+v", got, lay, both)
	}
	var plain struct{ Entry string }
	cli(t, nil, "pay", "-seed-file", seedFile(t, alice), "-serial", "5", "-to", strings.Repeat("b2", 32), "-amount", "2", "-root", rootA)
	cli(t, &plain, "build", "-pool", "1", "-root", rootA)
	if plain.Entry != "" {
		t.Fatal("build without -bytes printed the entry's bytes")
	}
	// A new node C lacks the parent of the entry built with -bytes, and
	// the error names it.
	rootC := t.TempDir()
	cli(t, nil, "init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0", "-root", rootC)
	stopC := startNode(t, rootC+"/policies/default")
	defer stopC()
	var sb strings.Builder
	if err := run([]string{"submit", "entry", "-pool", "1", "-root", rootC, both.Entry}, &sb); err == nil || !strings.Contains(err.Error(), "submit its parent") {
		t.Fatalf("an entry whose parent C lacks: %v", err)
	}

	// RED-038 (D144): the hex from a file (-f) or standard input (-), as
	// one argument is at most 128 KiB on Linux.
	// A fresh node D takes the first entry, then the one built on it.
	rootD := t.TempDir()
	cli(t, nil, "init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0", "-root", rootD)
	stopD := startNode(t, rootD+"/policies/default")
	defer stopD()
	for _, how := range []string{"stdin", "file"} {
		e := struct{ Hash, Entry string }{built.Hash.String(), built.Entry}
		if how == "file" {
			e.Hash, e.Entry = both.Hash, both.Entry
		}
		var sub struct{ Hash string }
		if how == "file" {
			path := t.TempDir() + "/entry.hex"
			if err := os.WriteFile(path, []byte(e.Entry+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cli(t, &sub, "submit", "entry", "-pool", "1", "-root", rootD, "-f", path)
		} else {
			var buf bytes.Buffer
			if err := Run(context.Background(), []string{"submit", "entry", "-pool", "1", "-root", rootD, "-"}, Stdio{In: strings.NewReader(e.Entry + "\n"), Out: &buf, Err: &buf}); err != nil {
				t.Fatalf("submit from standard input: %v", err)
			}
			if err := json.Unmarshal(buf.Bytes(), &sub); err != nil {
				t.Fatalf("%v: %s", err, buf.String())
			}
		}
		if sub.Hash != e.Hash {
			t.Fatalf("submit from %s: %+v, built %s", how, sub, e.Hash)
		}
	}
	// 300 KB of hex on standard input is read whole and decoded (junk, so
	// refused as an entry); -f with an argument is refused.
	err = Run(context.Background(), []string{"submit", "entry", "-pool", "1", "-root", rootB, "-"}, Stdio{In: strings.NewReader(strings.Repeat("00", 150_000)), Out: io.Discard, Err: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "poo submit entry:") || strings.Contains(err.Error(), "usage") {
		t.Fatalf("300 KB of hex on standard input: %v", err)
	}
	var sink strings.Builder
	if err := run([]string{"submit", "entry", "-pool", "1", "-root", rootB, "-f", "x", "00"}, &sink); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("-f and an argument: %v", err)
	}

	var out strings.Builder
	if err := run([]string{"submit"}, &out); err == nil {
		t.Fatal("poo submit accepted")
	}
	if err := run([]string{"submit", "tx"}, &out); err == nil {
		t.Fatal("poo submit tx accepted")
	}
	for _, bad := range [][]string{
		{"entry", "-pool", "1"}, {"entry", "-pool", "1", "zz"}, {"entry", "-pool", "1", "00"},
		{"layer", "-pool", "1", "00"}, {"entry", "-pool", "1", built.Entry, built.Entry},
	} {
		args := append([]string{"submit", bad[0], "-root", rootB}, bad[1:]...)
		if err := run(args, &out); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}
