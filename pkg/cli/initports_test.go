package cli

import (
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// poo init writes the default ports into node.json (W-7, D187), so a later
// release that moves a default leaves this node where it is. The defaults
// are jerry's choice: 7336 for local commands, 7337 for peers.
func TestInitWritesPorts(t *testing.T) {
	if nodepolicy.DefaultListen != "127.0.0.1:7336" || nodepolicy.DefaultP2PListen != ":7337" {
		t.Fatalf("defaults %s %s", nodepolicy.DefaultListen, nodepolicy.DefaultP2PListen)
	}
	dir := t.TempDir()
	cli(t, nil, "init", "-example", "-dir", dir)
	p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if p.RPC.Listen != "127.0.0.1:7336" || p.P2P.Listen != ":7337" {
		t.Fatalf("node.json has rpc %q p2p %q", p.RPC.Listen, p.P2P.Listen)
	}
	dir2 := t.TempDir()
	cli(t, nil, "init", "-example", "-dir", dir2, "-p2p", "off", "-listen", "127.0.0.1:9100")
	p, err = nodepolicy.Load(filepath.Join(dir2, nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if p.RPC.Listen != "127.0.0.1:9100" || p.P2P.Listen != "off" {
		t.Fatalf("flags not kept: rpc %q p2p %q", p.RPC.Listen, p.P2P.Listen)
	}
}
