package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// Flags may follow a command's word, as they may for poo series parts FILE
// -o table: poo tx TID -o table, poo address ADDR -limit 1, poo tracking
// ADDR -o json and poo policy use NAME -root DIR all work (GREEN-054).
func TestFlagsAfterTheWord(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "off"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	alice := strings.Repeat("a1", 32)
	var claim struct{ TID string }
	cli(t, &claim, append([]string{"claim", "-seed-file", seedFile(t, alice), "-serial", "5"}, r...)...)
	var key struct{ Address string }
	cli(t, &key, "keygen", "-seed", alice, "-root", t.TempDir())

	if out := runOut(t, append([]string{"tx", claim.TID, "-o", "table"}, r...)...); !strings.Contains(out, "pending") {
		t.Errorf("poo tx TID -o table:\n%s", out)
	}
	if out := runOut(t, append([]string{"address", key.Address, "-o", "table", "-limit", "1"}, r...)...); !strings.Contains(out, "balance") {
		t.Errorf("poo address ADDR -o table:\n%s", out)
	}
	if _, err := runErr(append([]string{"tracking", "127.0.0.1:1", "-o", "json"}, r...)...); err != nil && strings.Contains(err.Error(), "usage") {
		t.Errorf("poo tracking ADDR -o json: %v", err)
	}
	if _, err := runErr("tx", claim.TID, "-o", "table", "extra", "-root", root); err == nil || !strings.Contains(err.Error(), "usage: poo tx") {
		t.Errorf("two words: %v", err)
	}
	if _, err := runErr("policy", "use", "default", "-root", root); err != nil {
		t.Errorf("poo policy use NAME -root DIR: %v", err)
	}
}
