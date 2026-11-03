package peers_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-058, D158: addresses from a file take at most MaxUnlisted places,
// and one the node learns takes the place of the oldest of them when the
// book or its range is full; forget-imported takes them all out.
func TestImportedAddressesGiveWay(t *testing.T) {
	b, err := peers.Open("", peers.Limits{MaxRecords: 6, PerRange: 3, MaxUnlisted: 3})
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(1_800_000_000, 0)
	for i := 0; i < 3; i++ {
		if err := b.AddUnlisted(fmt.Sprintf("8.%d.0.1:1", i), []wire.Section{series}, t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.AddUnlisted("8.9.0.1:1", []wire.Section{series}, t0); !errors.Is(err, peers.ErrUnlistedFull) {
		t.Fatalf("a fourth import took a place: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := b.Add(fmt.Sprintf("9.%d.0.1:1", i), []wire.Section{series}, peers.FromPeerList, t0); err != nil {
			t.Fatal(err)
		}
	}
	// The book is full: a learned address takes the oldest imported one's place.
	if err := b.Add("10.0.0.1:1", []wire.Section{series}, peers.FromInbound, t0); err != nil {
		t.Fatalf("a full book refused a learned address: %v", err)
	}
	if _, held := b.Record("8.0.0.1:1"); held || b.Unlisted() != 2 {
		t.Fatalf("the oldest imported address stayed, or %d imported are left", b.Unlisted())
	}
	// A range full of imported addresses gives way too.
	r, err := peers.Open("", peers.Limits{MaxRecords: 100, PerRange: 2, MaxUnlisted: 10})
	if err != nil {
		t.Fatal(err)
	}
	r.AddUnlisted("7.7.0.1:1", []wire.Section{series}, t0)
	r.AddUnlisted("7.7.0.2:1", []wire.Section{series}, t0)
	if err := r.Add("7.7.0.3:1", []wire.Section{series}, peers.FromRegistry, t0); err != nil {
		t.Fatalf("a range full of imports refused a learned address: %v", err)
	}
	if err := r.AddUnlisted("7.7.0.4:1", []wire.Section{series}, t0); !errors.Is(err, peers.ErrRangeFull) {
		t.Fatalf("an import pushed out another: %v", err)
	}
	// Forget takes out only what is known from files alone.
	r.AddUnlisted("6.6.0.1:1", []wire.Section{series}, t0)
	r.PromoteImported(series, 1) // the oldest imported is now a candidate
	if n := r.ForgetImported(); n != 1 || len(r.Records()) != 2 {
		t.Fatalf("forgot %d, kept %d", n, len(r.Records()))
	}
	if r.Unlisted() != 0 {
		t.Fatal("imported records are left")
	}
}

// RED-060, D158: a host name is one range in any case and with the
// root's dot.
func TestRangeOfAHostName(t *testing.T) {
	if peers.Range("Boot.Example.com.:8646") != peers.Range("boot.example.com:1") || peers.Range("BOOT.example.COM") != "host:boot.example.com" {
		t.Fatal(peers.Range("Boot.Example.com.:8646"))
	}
}

// RED-061, D158: a label holds no direction mark or invisible character.
func TestLabelRefusesInvisibleText(t *testing.T) {
	for _, l := range []string{"a\u202eb", "a\u2028b", "a\u200bb", "\u061c", "a\x01b"} {
		if err := peers.CheckLabel(l); err == nil {
			t.Fatalf("took %q", l)
		}
	}
	if err := peers.CheckLabel("friend in T\u00f6kyo"); err != nil {
		t.Fatal(err)
	}
}
