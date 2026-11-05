package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// An unknown setting inside a list entry cannot be kept by Save, so Load
// names it and doctor warns; one inside a section is kept, and doctor
// notes it (GREEN-057).
func TestUnknownInsideListEntriesNamed(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-example", "-listen", "127.0.0.1:0", "-p2p", "off", "-root", root)
	path := filepath.Join(root, "policies", "default", nodepolicy.FileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["follow"].([]any)[0].(map[string]any)["future_rule"] = 1
	m["series"].([]any)[0].(map[string]any)["future_series"] = "x"
	m["rpc"].(map[string]any)["future_rpc"] = true
	b, _ = json.Marshal(m)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := strings.Join(p.UnknownDropped(), " "); d != "follow[0].future_rule series[0].future_series" {
		t.Fatalf("dropped %q", d)
	}
	cs := unknownSettingsChecks(p)
	if len(cs) != 2 || cs[0].Result != checkOK || !strings.Contains(cs[0].Detail, "rpc.future_rpc") ||
		cs[1].Result != checkWarn || !strings.Contains(cs[1].Detail, "2 settings this release does not know sit inside list entries") {
		t.Fatalf("checks %+v", cs)
	}
}

// Doctor compares the running node's release with this poo's (GREEN-057).
func TestReleaseCheck(t *testing.T) {
	mine := node.RunningRelease()
	if c := releaseCheck(rpc.StatusReply{Release: mine}); c.Result != checkOK {
		t.Fatalf("same: %+v", c)
	}
	if c := releaseCheck(rpc.StatusReply{Release: node.Release{Version: "0.0.9"}}); c.Result != checkWarn || !strings.Contains(c.Detail, "the node runs 0.0.9") || !strings.Contains(c.Fix, "restart the node") {
		t.Fatalf("older: %+v", c)
	}
	if c := releaseCheck(rpc.StatusReply{}); c.Result != checkWarn || !strings.Contains(c.Detail, "does not say its release") {
		t.Fatalf("unsaid: %+v", c)
	}
}
