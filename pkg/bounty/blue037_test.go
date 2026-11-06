package bounty

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// BLUE-037 (RED-110..114).

// RED-110: a claim needs a lease, and a layer the node holds is not owed.
func TestBlue037ClaimNeedsLeaseAndNewLayer(t *testing.T) {
	b, _ := payerBook(t, "")
	m, _ := b.Post(testOffer(20, 2))
	if a := b.Claim("mallory", Claim{Offer: m.ID, Nonce: 1}, 30, false); a.Outcome != NoLease {
		t.Fatalf("no lease: %s", OutcomeName(a.Outcome))
	}
	b.Take("alice", Take{Offer: m.ID, Rate: 1 << 20, PayTo: hash.Hash{0xa}})
	if a := b.Claim("alice", Claim{Offer: m.ID, Nonce: 1, PayTo: hash.Hash{0xa}}, 30, true); a.Outcome != Known {
		t.Fatalf("known layer: %s", OutcomeName(a.Outcome))
	}
	if got, _ := b.Get(m.ID); len(got.Claims) != 0 {
		t.Fatalf("a known layer was recorded: %+v", got.Claims)
	}
	if a := b.Claim("alice", Claim{Offer: m.ID, Nonce: 2, PayTo: hash.Hash{0xa}}, 30, false); a.Outcome != Accepted {
		t.Fatalf("new layer: %s", OutcomeName(a.Outcome))
	}
}

// RED-112: the leases from one network address are limited as one
// grinder's are, whatever node ids it uses.
func TestBlue037LeasesPerAddress(t *testing.T) {
	b, _ := payerBook(t, "")
	b.cfg.Budget = 1000
	var granted int
	for i := range 8 {
		m, err := b.Post(testOffer(20, 1))
		if err != nil {
			t.Fatal(err)
		}
		if g := b.TakeFrom(string(rune('a'+i)), "192.0.2.7", Take{Offer: m.ID, Rate: 1 << 20}); g.Seconds > 0 {
			granted++
		}
	}
	if granted != b.cfg.MaxLeasesPerGrinder {
		t.Fatalf("one address took %d leases, want %d", granted, b.cfg.MaxLeasesPerGrinder)
	}
	if n := len(b.Listable(hash.Hash{})); n != 8-granted {
		t.Fatalf("listable %d", n)
	}
}

// RED-111 (b): an offer sent again by another peer keeps its payer.
// RED-113: acks are bounded per offer, and peer records overall.
func TestBlue037HeldOfferKeepsPayerAndBoundsAcks(t *testing.T) {
	b, _ := Open(Config{Take: true}, "")
	o := testOffer(20, 1)
	b.Receive("payer", o)
	b.Receive("mallory", o)
	if from, _ := b.HeldFrom(o.ID()); from != "payer" {
		t.Fatalf("the offer moved to %q", from)
	}
	for i := range 3 * MaxAcks {
		b.Acked("payer", Ack{Offer: o.ID(), Nonce: uint64(i), Outcome: TooWeak})
	}
	h, _ := b.HeldOffer(o.ID())
	if len(h.Acks) != MaxAcks || h.Acks[MaxAcks-1].Nonce != 3*MaxAcks-1 {
		t.Fatalf("%d acks kept, last %+v", len(h.Acks), h.Acks[len(h.Acks)-1])
	}
	for i := range maxPeerRecords + 100 {
		b.grinderOf(string(rune(0x1000 + i)))
	}
	if len(b.grinders) > maxPeerRecords {
		t.Fatalf("%d grinder records", len(b.grinders))
	}
}

// RED-114: an offer or claim in bounties.json that the posting or
// claiming checks would refuse is left out and reported, and what is
// owed never wraps.
func TestBlue037FileChecked(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	good := testOffer(20, 1)
	zero := testOffer(20, 1)
	zero.Strength = 0
	huge := testOffer(20, 4)
	huge.Amount = 1 << 62
	weak := testOffer(20, 1)
	weak.Salt = 5
	f := file{Kind: FileKind, Format: FileFormat, Offers: []*Mine{
		{ID: good.ID(), Offer: good, Posted: time.Now()},
		{ID: zero.ID(), Offer: zero},
		{ID: huge.ID(), Offer: huge},
		{ID: weak.ID(), Offer: weak, Claims: []ClaimRecord{{Nonce: 1, Strength: 3}}},
	}}
	raw, _ := json.Marshal(f)
	os.WriteFile(path, raw, 0o600)
	b, err := Open(Config{Post: true}, path)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(b.Mine()); n != 1 {
		t.Fatalf("%d offers kept, want 1: %v", n, b.Dropped())
	}
	if d := b.Dropped(); len(d) != 3 || !strings.Contains(strings.Join(d, "\n"), "strength 3") {
		t.Fatalf("dropped %q", d)
	}
	m := &Mine{Offer: huge}
	if m.Owed() != math.MaxUint64 {
		t.Fatalf("owed wrapped to %d", m.Owed())
	}
}
