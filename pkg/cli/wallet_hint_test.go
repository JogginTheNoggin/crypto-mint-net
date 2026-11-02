package cli

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// A wallet command with no wallet says how to make one, and the error is
// still a missing file (GREEN-040).
func TestNoWalletSaysWhatToDo(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-example", "-p2p", "off", "-root", root)
	err := run([]string{"wallet", "balance", "-root", root}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "poo wallet create") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wallet balance with no wallet: %v", err)
	}
}
