package nodepolicy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// A setting a newer release wrote survives an older release loading,
// changing and saving node.json (White's W-1, D184): at the top level,
// inside a section, and in a section this release does not know.
func TestUnknownSettingsSurviveSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, nodepolicy.FileName)
	must(t, nodepolicy.Save(path, nodepolicy.Default()))
	var m map[string]any
	b, err := os.ReadFile(path)
	must(t, err)
	must(t, json.Unmarshal(b, &m))
	m["future_top"] = map[string]any{"a": 1.5, "b": []any{"x"}}
	m["p2p"] = map[string]any{"max_peers": 9, "future_p2p": "keep me"}
	m["rpc"] = map[string]any{"future_rpc": true}
	b, err = json.Marshal(m)
	must(t, err)
	must(t, os.WriteFile(path, b, 0o600))

	p, err := nodepolicy.Load(path)
	must(t, err)
	if u := p.UnknownSettings(); strings.Join(u, " ") != "future_top p2p.future_p2p rpc.future_rpc" {
		t.Fatalf("unknown %v", u)
	}
	p.P2P.MaxPeers = 12 // a change the older release makes
	must(t, nodepolicy.Save(path, p))

	b, err = os.ReadFile(path)
	must(t, err)
	var got map[string]any
	must(t, json.Unmarshal(b, &got))
	p2p := got["p2p"].(map[string]any)
	if p2p["future_p2p"] != "keep me" || p2p["max_peers"] != 12.0 {
		t.Fatalf("p2p %v", p2p)
	}
	if got["rpc"].(map[string]any)["future_rpc"] != true {
		t.Fatalf("rpc %v", got["rpc"])
	}
	top := got["future_top"].(map[string]any)
	if top["a"] != 1.5 || top["b"].([]any)[0] != "x" {
		t.Fatalf("top %v", top)
	}
	if !strings.HasSuffix(string(b), "}\n") || !strings.Contains(string(b), "\n  \"p2p\"") {
		t.Fatalf("not indented:\n%s", b)
	}
	// It loads again, and a second save keeps them still.
	p, err = nodepolicy.Load(path)
	must(t, err)
	must(t, nodepolicy.Save(path, p))
	b2, err := os.ReadFile(path)
	must(t, err)
	if string(b2) != string(b) {
		t.Fatalf("a second save changed the file:\n%s\n%s", b, b2)
	}
	// An export carries only what this release knows.
	e, err := json.Marshal(nodepolicy.ExportPolicy(p, true))
	must(t, err)
	if strings.Contains(string(e), "future_") {
		t.Fatalf("export carried unknown settings: %s", e)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
