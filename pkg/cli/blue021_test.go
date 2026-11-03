package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// writeSettings writes a settings bundle of p to a file and returns its
// path. p is not validated, as a hostile file is not.
func writeSettings(t *testing.T, p nodepolicy.Policy) string {
	t.Helper()
	b := nodepolicy.NewBundle(nodepolicy.ScopeSettings, false, time.Now())
	b.Policies = []nodepolicy.BundlePolicy{{Policy: p}}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// RED-053, D156: a registry with escape sequences never reaches the
// terminal: the import is refused, and the error shows it escaped.
func TestImportRefusesTerminalEscapes(t *testing.T) {
	dst := t.TempDir()
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	p := nodepolicy.Default()
	p.P2P.MaxPeers = 100000
	p.P2P.Registries = []string{"https://reg.example/x\x1b[8A\x1b[J"}
	file := writeSettings(t, p)
	var out bytes.Buffer
	err := run([]string{"settings", "import", file, "-prefer-file", "-root", dst}, &out)
	if err == nil || !strings.Contains(err.Error(), `\x1b[8A`) {
		t.Fatalf("took it: %v\n%s", err, out.String())
	}
	if strings.ContainsRune(err.Error()+out.String(), 0x1b) {
		t.Fatalf("an escape reached the output: %q %q", err, out.String())
	}
}

// RED-053, D156: termSafe escapes what could move the cursor or start a
// line, and leaves other text alone.
func TestTermSafe(t *testing.T) {
	for in, want := range map[string]string{
		"https://reg.example/a.json": "https://reg.example/a.json",
		"x\x1b[8A\x1b[J":             `x\x1b[8A\x1b[J`,
		"a\nnote: fake line":         `a\nnote: fake line`,
		"a\rb\bc":                    `a\rb\bc`,
		"a\u202eb":                   `a\u202eb`,
		"grüße":                      "grüße",
	} {
		if got := termSafe(in); got != want {
			t.Errorf("termSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

// RED-054, D156: -prefer-file -apply saves a conflict that took the file's
// value, also when it is the only difference.
func TestPreferFileAppliesALoneConflict(t *testing.T) {
	dst := t.TempDir()
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	path := filepath.Join(dst, "policies", "default", nodepolicy.FileName)
	local, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	local.Issuers.Any = false
	if err := nodepolicy.Save(path, local); err != nil {
		t.Fatal(err)
	}
	file := writeSettings(t, nodepolicy.ExportSettings(func() nodepolicy.Policy { p := local; p.Issuers.Any = true; return p }(), false))
	out := runOut(t, "settings", "import", file, "-prefer-file", "-root", dst)
	if strings.Contains(out, "nothing to change") || !strings.Contains(out, "dry run") || !strings.Contains(out, "import digest") {
		t.Fatalf("preview:\n%s", out)
	}
	runOut(t, "settings", "import", file, "-prefer-file", "-apply", "-root", dst)
	after, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Issuers.Any {
		t.Fatal("took the file's value and saved nothing")
	}
}
