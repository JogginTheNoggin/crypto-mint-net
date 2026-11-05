package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A start past the tip is refused rather than written as an empty file
// that claims a range, and an import the node cannot take says which
// series and pool the file is for (GREEN-058).
func TestLedgerExportPastTipAndImportHints(t *testing.T) {
	src, other := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-listen", "127.0.0.1:0", "-p2p", "off")
	cli(t, nil, "init", "-root", other, "-listen", "127.0.0.1:0", "-p2p", "off")
	for _, root := range []string{src, other} {
		stop := startNode(t, root+"/policies/default")
		defer stop()
	}
	r := []string{"-root", src}
	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, strings.Repeat("a1", 32)), "-serial", "1"}, r...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "0"}, r...)...)
	cli(t, nil, append([]string{"work", "-pool", "0", "-entry", built.Hash, "-tries", "4000"}, r...)...)

	file := filepath.Join(t.TempDir(), "pool0.json")
	if out := runOut(t, "ledger", "export", "-pool", "0", "-root", src, "-out", file); !strings.Contains(out, "wrote 1 entries to") {
		t.Fatalf("export -pool 0: %s", out)
	}
	if _, err := runErr("ledger", "export", "-pool", "0", "-from", "5", "-root", src); err == nil || !strings.Contains(err.Error(), "-from 5 is past the tip") {
		t.Fatalf("-from past the tip: %v", err)
	}

	if _, err := runErr("ledger", "import", file, "-root", other); err == nil || !strings.Contains(err.Error(), "which this node does not know") {
		t.Fatalf("an unknown series: %v", err)
	}
	var m map[string]any
	b, _ := os.ReadFile(file)
	json.Unmarshal(b, &m)
	m["pool"] = 7
	b, _ = json.Marshal(m)
	seven := filepath.Join(t.TempDir(), "pool7.json")
	os.WriteFile(seven, b, 0o600)
	if _, err := runErr("ledger", "import", seven, "-root", src); err == nil || !strings.Contains(err.Error(), "poo follow -series") || !strings.Contains(err.Error(), "-pool 7") {
		t.Fatalf("a pool not followed: %v", err)
	}
}
