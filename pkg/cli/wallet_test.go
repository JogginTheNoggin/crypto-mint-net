package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

func TestWalletEndToEnd(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POON_PASSPHRASE", "correct horse")
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	dir := filepath.Join(root, "policies", "default")
	stop := startNode(t, dir)
	defer stop()

	aw := append([]string{"-wallet", filepath.Join(root, "alice.json")}, r...)
	bw := append([]string{"-wallet", filepath.Join(root, "bob.json")}, r...)
	wcmd := func(v any, sub string, who []string, args ...string) {
		t.Helper()
		cli(t, v, append(append([]string{"wallet", sub}, who...), args...)...)
	}
	wcmd(nil, "create", aw)
	wcmd(nil, "create", bw)
	var out bytes.Buffer
	if err := run(append([]string{"wallet", "create"}, aw...), &out); err == nil {
		t.Fatal("create overwrote a wallet")
	}

	// Alice claims mint 5 and works the claim into the selected ledger.
	var watched struct{ Mints, Pools []uint64 }
	wcmd(&watched, "watch", aw, "-serial", "5")
	if len(watched.Pools) != 1 || watched.Pools[0] != 1 {
		t.Fatalf("watch %+v", watched)
	}
	wcmd(nil, "watch", bw, "-pool", "1")
	wcmd(nil, "claim", aw, "-serial", "5")
	work := func() {
		t.Helper()
		var b struct{ Layer struct{ Switched bool } }
		cli(t, &b, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
		if !b.Layer.Switched {
			t.Fatalf("build -work did not select the new entry: %+v", b)
		}
	}
	work()
	var bal struct {
		Mints map[string]uint64
		Total uint64
	}
	wcmd(&bal, "balance", aw)
	if bal.Total != 100 || bal.Mints["5"] != 100 {
		t.Fatalf("balance %+v", bal)
	}

	// Bob asks for 30; Alice pays the request.
	req := filepath.Join(root, "req.json")
	wcmd(nil, "request", bw, "-amount", "30", "-memo", "lunch", "-out", req)
	var sent struct {
		Legs []struct {
			Serial uint64
			TID    string
			Change uint64
		}
	}
	wcmd(&sent, "pay", aw, "-request", req)
	if len(sent.Legs) != 1 || sent.Legs[0].Change != 70 {
		t.Fatalf("pay %+v", sent)
	}
	var rc struct {
		Received uint64
		Complete bool
		Settled  bool
		Legs     []struct{ State string }
	}
	wcmd(&rc, "receive", bw, "-request", req, "-serial", "5", "-tid", sent.Legs[0].TID)
	if rc.Received != 30 || rc.Settled || rc.Legs[0].State != "pending" {
		t.Fatalf("receive before work %+v", rc)
	}
	work()
	wcmd(&rc, "receive", bw, "-request", req)
	if !rc.Complete || !rc.Settled || rc.Legs[0].State != "selected" {
		t.Fatalf("receive after work %+v", rc)
	}
	var st []struct {
		Claim   bool
		Settled bool
	}
	wcmd(&st, "status", aw)
	if len(st) != 2 || !st[0].Claim || !st[1].Settled {
		t.Fatalf("status %+v", st)
	}
	wcmd(&st, "status", aw, "-threshold", "1"+strings.Repeat("0", 60))
	if st[1].Settled {
		t.Fatal("settled below the threshold")
	}
	wcmd(&bal, "balance", bw)
	if bal.Total != 30 {
		t.Fatalf("bob's balance %+v", bal)
	}

	// Restoring from the seed finds the same addresses.
	var seed bytes.Buffer
	if err := run(append([]string{"wallet", "seed"}, aw...), &seed); err != nil {
		t.Fatal(err)
	}
	cw := append([]string{"-wallet", filepath.Join(root, "copy.json")}, r...)
	wcmd(nil, "create", cw, "-restore", strings.TrimSpace(seed.String()))
	wcmd(nil, "watch", cw, "-serial", "5")
	wcmd(&bal, "balance", cw)
	if bal.Total != 70 {
		t.Fatalf("restored balance %+v", bal)
	}

	// A wrong passphrase is refused.
	t.Setenv("POON_PASSPHRASE", "wrong")
	if err := run(append([]string{"wallet", "address"}, aw...), &out); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	t.Setenv("POON_PASSPHRASE", "correct horse")

	// A 2-of-2 policy with bob: a payment from it waits for his signature.
	var bobKey struct{ Public string }
	wcmd(&bobKey, "address", bw, "-label", "joint with alice")
	var ap struct {
		Policy  struct{ Keys []struct{ Public string } }
		Address string
	}
	wcmd(&ap, "policy", aw, "-name", "joint", "-k", "2", "-key", "self", "-key", bobKey.Public)
	if len(ap.Policy.Keys) != 2 || ap.Policy.Keys[1].Public != bobKey.Public {
		t.Fatalf("policy %+v", ap)
	}
	wcmd(nil, "pay", aw, "-to", ap.Address, "-amount", "20")
	work()
	unsigned := filepath.Join(root, "unsigned.json")
	out.Reset()
	if err := run(append(append([]string{"wallet", "pay"}, aw...), "-to", ap.Address, "-amount", "15", "-out", unsigned), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "signatures missing") {
		t.Fatalf("a shared payment was sent without bob: %s", out.String())
	}
	if err := run(append(append([]string{"wallet", "submit"}, aw...), unsigned), &out); err == nil {
		t.Fatal("submitted with a signature missing")
	}
	out.Reset()
	if err := run(append(append([]string{"wallet", "cosign"}, bw...), unsigned), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "added 1 signatures") {
		t.Fatalf("cosign: %s", out.String())
	}
	var joint struct{ Legs []struct{ State string } }
	wcmd(&joint, "submit", aw, unsigned)
	if len(joint.Legs) != 1 || joint.Legs[0].State != "pending" {
		t.Fatalf("submit %+v", joint)
	}
	work()
	wcmd(&st, "status", aw)
	if len(st) != 4 || !st[3].Settled {
		t.Fatalf("status after the joint payment %+v", st)
	}

	// Light mode: the node policy follows only the wallet's pools.
	wcmd(nil, "follow", aw)
	p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Follow) != 1 || len(p.Follow[0].Pools) != 1 || p.Follow[0].Pools[0] != 1 {
		t.Fatalf("follow %+v", p.Follow)
	}
	if _, err := os.Stat(filepath.Join(root, "alice.json")); err != nil {
		t.Fatal(err)
	}
}
