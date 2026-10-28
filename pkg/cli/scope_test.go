package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// TestViewOnlyTable: every command the usage names is classified, as
// reading or as changing something, so a new command cannot slip through
// view-only mode unnoticed (it is refused until classified, and this test
// fails until it is).
func TestViewOnlyTable(t *testing.T) {
	for _, n := range commandNames() {
		_, read := readCommands[n]
		if read == slices.Contains(writeCommands, n) {
			t.Errorf("command %q must be in exactly one of readCommands and writeCommands", n)
		}
	}
	for _, sub := range subcommands("wallet") {
		if err := checkViewOnly([]string{"wallet", sub}); (err == nil) != readCommands["wallet"][sub] {
			t.Errorf("wallet %s: %v", sub, err)
		}
	}
}

// viewRun runs args in view-only mode.
func viewRun(args ...string) (string, error) {
	var out, errb bytes.Buffer
	err := Run(context.Background(), args, Stdio{Out: &out, Err: &errb, ViewOnly: true})
	return out.String(), err
}

// TestViewOnly runs reads and writes in view-only mode against a running
// node whose full token file is gone: reads work, so they used the view
// token; writes are refused before they run.
func TestViewOnly(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	dir := filepath.Join(root, "policies", "default")
	stop := startNode(t, dir)
	defer stop()
	t.Setenv("POO_PASSPHRASE", "view test")
	createIterations = 1
	defer func() { createIterations = 0 }()
	cli(t, nil, append([]string{"wallet", "create"}, r...)...)
	policyBefore, err := os.ReadFile(filepath.Join(dir, "node.json"))
	must(t, err)
	// Without the full token only the view token can reach the node.
	must(t, os.Remove(filepath.Join(dir, rpc.TokenFile)))
	id := testutil.ExampleSeries().ID().String()

	for _, args := range [][]string{
		{"status"}, {"pools"}, {"ledger", "-pool", "1"}, {"pending"}, {"unspent"},
		{"peers"}, {"tracking"}, {"chases"}, {"holdings", "-serial", "5"},
		{"follow"}, {"policy", "list"}, {"wallet", "addresses"}, {"wallet", "balance"},
		{"doctor", "-json"}, {"crypto"}, {"series", "example"}, {"version"},
	} {
		if out, err := viewRun(append(args, r...)...); err != nil {
			t.Errorf("view-only %v: %v\n%s", args, err, out)
		}
	}
	for _, args := range [][]string{
		{"init", "-example"}, {"node"}, {"reload"}, {"build", "-pool", "1"}, {"work", "-pool", "1"},
		{"claim", "-serial", "5"}, {"pay", "-serial", "5"}, {"peer", "block", "127.0.0.1:1"},
		{"unfollow", "-series", id}, {"follow", "-series", id, "-pool", "0"},
		{"policy", "use", "default"}, {"policy", "delete", "default"},
		{"wallet", "create"}, {"wallet", "address"}, {"wallet", "seed"}, {"wallet", "pay"},
		{"wallet", "watch", "-serial", "1"}, {"wallet", "follow"}, {"wallet", "status"},
	} {
		_, err := viewRun(append(args, r...)...)
		if err == nil || !strings.HasPrefix(err.Error(), "view-only: poo "+args[0]) {
			t.Errorf("view-only %v: %v, want refused", args, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "node.json")); !bytes.Equal(b, policyBefore) {
		t.Error("view-only changed node.json")
	}

	// The switch can also be a leading -view-only or $POO_VIEW_ONLY.
	var out bytes.Buffer
	if err := Run(context.Background(), append([]string{"-view-only", "reload"}, r...), Stdio{Out: &out, Err: &out}); err == nil {
		t.Error("-view-only reload ran")
	}
	t.Setenv(ViewOnlyEnv, "1")
	if err := Run(context.Background(), append([]string{"build", "-pool", "1"}, r...), Stdio{Out: &out, Err: &out}); err == nil {
		t.Error("POO_VIEW_ONLY=1 build ran")
	}
	if err := Run(context.Background(), append([]string{"status"}, r...), Stdio{Out: &out, Err: &out}); err != nil {
		t.Errorf("POO_VIEW_ONLY=1 status: %v", err)
	}
}

// TestSecretReader: with Stdio.Secret a passphrase is asked for through
// it, with a prompt, and never read from In.
func TestSecretReader(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example"}, r...)...)
	createIterations = 1
	defer func() { createIterations = 0 }()
	var prompts []string
	secret := func(p string) (string, error) { prompts = append(prompts, p); return "masked", nil }
	var out bytes.Buffer
	io := Stdio{In: strings.NewReader("from stdin\n"), Out: &out, Err: &out, Secret: secret}
	if err := Run(context.Background(), append([]string{"wallet", "create"}, r...), io); err != nil {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := Run(context.Background(), append([]string{"wallet", "seed"}, r...), io); err != nil {
		t.Fatalf("seed: %v\n%s", err, out.String())
	}
	if len(prompts) < 2 || strings.Contains(out.String(), "passphrase: ") {
		t.Fatalf("prompts %q, out %q", prompts, out.String())
	}
	// The wallet was sealed with what Secret returned, not with In.
	io.Secret = nil
	io.In = strings.NewReader("masked\n")
	if err := Run(context.Background(), append([]string{"wallet", "seed"}, r...), io); err != nil {
		t.Fatalf("seed with the passphrase on In: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestRestoreSeedHidden: -restore - reads the seed through Stdio.Secret,
// so it never shows on the command line or the screen (GUI-006).
func TestRestoreSeedHidden(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example"}, r...)...)
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POO_PASSPHRASE", "pw")
	cli(t, nil, append([]string{"wallet", "create"}, r...)...)
	var seed bytes.Buffer
	must(t, run(append([]string{"wallet", "seed"}, r...), &seed))
	hexSeed := strings.TrimSpace(seed.String())
	var prompts []string
	secret := func(p string) (string, error) { prompts = append(prompts, p); return hexSeed, nil }
	var out bytes.Buffer
	copyArgs := append([]string{"-wallet", filepath.Join(root, "copy.json")}, r...)
	io := Stdio{In: strings.NewReader(""), Out: &out, Err: &out, Secret: secret}
	if err := Run(context.Background(), append(append([]string{"wallet", "create"}, copyArgs...), "-restore", "-"), io); err != nil {
		t.Fatalf("restore: %v\n%s", err, out.String())
	}
	if len(prompts) != 1 || prompts[0] != "seed: " || strings.Contains(out.String(), hexSeed) {
		t.Fatalf("prompts %q, out %q", prompts, out.String())
	}
	var again bytes.Buffer
	must(t, run(append([]string{"wallet", "seed"}, copyArgs...), &again))
	if strings.TrimSpace(again.String()) != hexSeed {
		t.Fatal("the restored wallet has another seed")
	}
	if !slices.Contains(SecretFlags(), "restore") || !slices.Contains(SecretFlags(), "seed") {
		t.Fatalf("SecretFlags %q", SecretFlags())
	}
}
