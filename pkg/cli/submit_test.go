package cli

import (
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
