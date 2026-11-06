package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewPassphraseIsConfirmed is GUI-020: a new passphrase typed through
// Stdio.Secret (a hidden terminal, the GUI's masked field) is asked for
// twice, and two that differ change nothing. A piped line is read once,
// as before (D145).
func TestNewPassphraseIsConfirmed(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	file := filepath.Join(root, "w.json")
	w := []string{"-root", root, "-wallet", file}
	typed := func(answers ...string) (Stdio, *[]string) {
		var prompts []string
		var out bytes.Buffer
		return Stdio{Out: &out, Err: &out, Secret: func(p string) (string, error) {
			prompts = append(prompts, p)
			a := answers[0]
			answers = answers[1:]
			return a, nil
		}}, &prompts
	}
	ctx := context.Background()

	io, _ := typed("one", "uno")
	if err := Run(ctx, append([]string{"wallet", "create"}, w...), io); err == nil || !strings.Contains(err.Error(), "differ") {
		t.Fatalf("create with two passphrases that differ: %v", err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("a wallet was written")
	}
	io, prompts := typed("one", "one")
	if err := Run(ctx, append([]string{"wallet", "create"}, w...), io); err != nil {
		t.Fatalf("create: %v", err)
	}
	if want := []string{"new passphrase: ", "new passphrase again: "}; strings.Join(*prompts, "|") != strings.Join(want, "|") {
		t.Fatalf("prompts %q, want %q", *prompts, want)
	}

	io, _ = typed("one", "two", "zwei")
	if err := Run(ctx, append([]string{"wallet", "passwd"}, w...), io); err == nil || !strings.Contains(err.Error(), "differ") {
		t.Fatalf("passwd with two passphrases that differ: %v", err)
	}
	io, _ = typed("one")
	if err := Run(ctx, append([]string{"wallet", "seed"}, w...), io); err != nil {
		t.Fatalf("the old passphrase no longer opens the wallet: %v", err)
	}
	io, _ = typed("one", "two", "two")
	if err := Run(ctx, append([]string{"wallet", "passwd"}, w...), io); err != nil {
		t.Fatalf("passwd: %v", err)
	}
	io, _ = typed("two")
	if err := Run(ctx, append([]string{"wallet", "seed"}, w...), io); err != nil {
		t.Fatalf("the new passphrase does not open the wallet: %v", err)
	}
}

// TestWaitingCommandKeepsAnotherChange: a key-only command that saves
// (here passwd) holds the wallet only after its prompts, and reads the
// file again then, so a change saved while it waited at a prompt (a new
// address) is kept, and it saves without ErrChanged (D147).
func TestWaitingCommandKeepsAnotherChange(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	cli(t, nil, "init", "-example", "-root", root)
	w := []string{"-root", root}
	t.Setenv("POON_PASSPHRASE", "one")
	cli(t, nil, append([]string{"wallet", "create"}, w...)...)
	ctx := context.Background()
	var out bytes.Buffer
	io := Stdio{Out: &out, Err: &out, Secret: func(p string) (string, error) {
		if strings.Contains(p, "again") {
			// Another command saves while passwd waits here.
			cli(t, nil, append([]string{"wallet", "address", "-label", "while waiting"}, w...)...)
		}
		return "two", nil
	}}
	if err := Run(ctx, append([]string{"wallet", "passwd"}, w...), io); err != nil {
		t.Fatalf("passwd after another command saved: %v\n%s", err, out.String())
	}
	t.Setenv("POON_PASSPHRASE", "two")
	var addrs []struct{ Label string }
	cli(t, &addrs, append([]string{"wallet", "addresses"}, w...)...)
	found := false
	for _, a := range addrs {
		found = found || a.Label == "while waiting"
	}
	if !found {
		t.Fatalf("the address saved while passwd waited was lost: %+v", addrs)
	}
}
