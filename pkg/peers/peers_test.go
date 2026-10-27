package peers_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

var (
	series = wire.SeriesSection(hash.Zero, hash.Hash{1})
	pool1  = wire.PoolSection(hash.Zero, hash.Hash{1}, hash.Hash{2})
	pool2  = wire.PoolSection(hash.Zero, hash.Hash{1}, hash.Hash{3})
	other  = wire.SeriesSection(hash.Zero, hash.Hash{9})
	t0     = time.Unix(1_800_000_000, 0)
)

func open(t *testing.T, lim peers.Limits) *peers.Book {
	t.Helper()
	b, err := peers.Open("", lim)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStates(t *testing.T) {
	b := open(t, peers.Limits{})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(b.Add("10.0.0.1:1", []wire.Section{series}, peers.FromRegistry, t0))
	must(b.Add("10.1.0.1:1", []wire.Section{pool1}, peers.FromPeerList, t0))
	must(b.Add("10.2.0.1:1", []wire.Section{other}, peers.FromPeerList, t0))
	if got := b.Pick(pool1, 10, nil, t0); len(got) != 2 {
		t.Fatalf("candidates for pool 1: %v", got)
	}
	must(b.Verify("10.1.0.1:1", pool1, t0))
	if got := b.VerifiedFor(series, false, nil); len(got) != 1 || got[0].Contact.Addr != "10.1.0.1:1" {
		t.Fatalf("verified for series: %v", got)
	}
	if got := b.VerifiedFor(series, true, nil); len(got) != 0 {
		t.Fatalf("pool 1 follower listed as following all of the series: %v", got)
	}
	if got := b.VerifiedFor(series, true, []wire.Section{pool1}); len(got) != 1 {
		t.Fatalf("pool 1 follower not listed for the part followed: %v", got)
	}
	if c := b.Count(wire.All); c[peers.Verified] != 1 || c[peers.Candidate] != 2 {
		t.Fatalf("counts %v", c)
	}
	b.Unverify("10.1.0.1:1")
	if got := b.VerifiedFor(series, false, nil); len(got) != 0 {
		t.Fatal("unverified peer still listed")
	}
	must(b.Verify("10.1.0.1:1", pool1, t0))
	b.Archive("10.1.0.1:1", pool1)
	if got := b.VerifiedFor(series, false, nil); len(got) != 0 {
		t.Fatal("archived peer still listed")
	}
	if b.Unarchive(pool2) != 0 || b.Unarchive(series) != 1 {
		t.Fatal("unarchive")
	}
	b.Delete("10.0.0.1:1")
	if err := b.Add("10.0.0.1:1", []wire.Section{series}, peers.FromReply, t0); !errors.Is(err, peers.ErrDeleted) {
		t.Fatalf("deleted address added again: %v", err)
	}
	if err := b.Add("nonsense", nil, peers.FromReply, t0); err == nil {
		t.Fatal("bad address added")
	}
}

func TestRangeLimits(t *testing.T) {
	b := open(t, peers.Limits{PerRange: 2, VerifiedPerRange: 1})
	b.Add("192.168.1.1:1", nil, peers.FromReply, t0)
	b.Add("192.168.2.1:1", nil, peers.FromReply, t0)
	if err := b.Add("192.168.3.1:1", nil, peers.FromReply, t0); !errors.Is(err, peers.ErrRangeFull) {
		t.Fatalf("third from one /16: %v", err)
	}
	if err := b.Add("192.169.3.1:1", nil, peers.FromReply, t0); err != nil {
		t.Fatal(err)
	}
	if err := b.Verify("192.168.1.1:1", pool1, t0); err != nil {
		t.Fatal(err)
	}
	if err := b.Verify("192.168.2.1:1", pool1, t0); !errors.Is(err, peers.ErrRangeFull) {
		t.Fatalf("second verified from one range: %v", err)
	}
	if err := b.Verify("192.168.2.1:1", pool2, t0); err != nil {
		t.Fatalf("another policy from the same range: %v", err)
	}
	for addr, want := range map[string]string{
		"1.2.3.4:5": "v4:1.2", "[2001:db8::1]:5": "v6:20010db8", "node.example:5": "host:node.example",
		"127.0.0.1:5": "loopback:127.0.0.1:5",
	} {
		if got := peers.Range(addr); got != want {
			t.Errorf("%s: %s", addr, got)
		}
	}
}

func TestPickSpreadsAndBacksOff(t *testing.T) {
	b := open(t, peers.Limits{Retry: time.Minute})
	for _, a := range []string{"10.0.0.1:1", "10.0.0.2:1", "10.9.0.1:1"} {
		b.Add(a, []wire.Section{series}, peers.FromRegistry, t0)
	}
	got := b.Pick(series, 2, nil, t0)
	if len(got) != 2 || peers.Range(got[0]) == peers.Range(got[1]) {
		t.Fatalf("not spread over ranges: %v", got)
	}
	b.Tried("10.9.0.1:1", false, t0)
	if got := b.Pick(series, 3, nil, t0.Add(30*time.Second)); len(got) != 2 {
		t.Fatalf("failed candidate retried too soon: %v", got)
	}
	if got := b.Pick(series, 3, nil, t0.Add(61*time.Second)); len(got) != 3 {
		t.Fatalf("failed candidate never retried: %v", got)
	}
	if got := b.Pick(series, 3, map[string]bool{"10.0.0.1:1": true}, t0.Add(time.Hour)); len(got) != 2 {
		t.Fatalf("skip ignored: %v", got)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), peers.FileName)
	b, err := peers.Open(path, peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	b.Add("10.0.0.1:1", []wire.Section{series}, peers.FromConfig, t0)
	b.Verify("10.0.0.2:1", pool1, t0)
	b.Add("10.0.0.3:1", []wire.Section{pool2}, peers.FromReply, t0)
	b.Archive("10.0.0.3:1")
	b.Delete("10.0.0.4:1")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	c, err := peers.Open(path, peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	recs := c.Records()
	if len(recs) != 3 {
		t.Fatalf("%d records", len(recs))
	}
	// Verification does not survive a restart; archived stays archived.
	if recs[1].Policies[0].State != peers.Candidate || recs[2].Policies[0].State != peers.Archived || recs[1].Policies[0].Section != pool1 {
		t.Fatalf("%+v", recs)
	}
	if !c.Deleted("10.0.0.4:1") {
		t.Fatal("deletion lost")
	}
}
