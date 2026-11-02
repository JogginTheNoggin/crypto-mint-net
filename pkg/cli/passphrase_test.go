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
