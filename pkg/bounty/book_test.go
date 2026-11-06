package bounty

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

func testOffer(strength uint8, count uint16) Offer {
	return Offer{Kind: KindLayer, Series: hash.Hash{1}, Pool: 3, Entry: hash.Hash{2}, Strength: strength, Amount: 10, Count: count, OpenSeconds: 3600}
}

func TestCodecRoundTrip(t *testing.T) {
	o := testOffer(24, 2)
	o.Salt = 9
	if got, err := DecodeOffer(o.Encode()); err != nil || !got.Same(o) || got.ID() != o.ID() || got.Salt != 9 {
		t.Fatalf("offer %+v %v", got, err)
	}
	if got, err := DecodeList(EncodeList([]Offer{o, o})); err != nil || len(got) != 2 || !got[1].Same(o) {
		t.Fatalf("list %+v %v", got, err)
	}
	tk := Take{Offer: o.ID(), Rate: 5, PayTo: hash.Hash{7}, MaxStrength: 30}
	if got, err := DecodeTake(tk.Encode()); err != nil || got != tk {
		t.Fatalf("take %+v %v", got, err)
	}
	g := Grant{Offer: o.ID(), Seconds: 60, Reason: "x"}
	if got, err := DecodeGrant(g.Encode()); err != nil || got != g {
		t.Fatalf("grant %+v %v", got, err)
	}
	c := Claim{Offer: o.ID(), Nonce: 4, PayTo: hash.Hash{7}}
	if got, err := DecodeClaim(c.Encode()); err != nil || got != c {
		t.Fatalf("claim %+v %v", got, err)
	}
	a := Ack{Offer: o.ID(), Nonce: 4, Outcome: Paid, TIDs: []hash.Hash{{8}, {9}}}
	if got, err := DecodeAck(a.Encode()); err != nil || got.Outcome != Paid || len(got.TIDs) != 2 || got.TIDs[1] != a.TIDs[1] {
		t.Fatalf("ack %+v %v", got, err)
	}
	for _, short := range [][]byte{nil, {1}, o.Encode()[:20]} {
		if _, err := DecodeOffer(short); err == nil {
			t.Fatalf("short offer %x decoded", short)
		}
	}
}

func payerBook(t *testing.T, path string) (*Book, *time.Time) {
	t.Helper()
	b, err := Open(Config{Post: true, MaxAmount: 1000, Budget: 100, LeaseFloor: time.Minute, LeaseCeiling: time.Hour}, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	b.SetClock(func() time.Time { return now })
	return b, &now
}

// D196: posting is bounded by the budget; a lease goes to one grinder
// at a time; a new grinder holds one lease until it fills one; the claim
// must name the lease's address; an accepted claim waits to be paid and
// survives a restart.
func TestBookLeasesAndClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	b, now := payerBook(t, path)
	if _, err := b.Post(Offer{Kind: KindLayer, Strength: 24, Amount: 10, Count: 11}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("over budget: %v", err)
	}
	m1, err := b.Post(testOffer(20, 1))
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := b.Post(testOffer(20, 1))
	if m1.ID == m2.ID {
		t.Fatal("two posts made one offer")
	}
	if n := len(b.Listable(hash.Hash{}, 0)); n != 2 {
		t.Fatalf("listable %d", n)
	}
	// A rate of 2^20 a second expects one second: the lease is the floor.
	g := b.Take("alice", Take{Offer: m1.ID, Rate: 1 << 20, PayTo: hash.Hash{0xa}})
	if g.Seconds != 60 {
		t.Fatalf("grant %+v", g)
	}
	if n := len(b.Listable(hash.Hash{}, 0)); n != 1 {
		t.Fatalf("a leased offer is still listed: %d", n)
	}
	if g := b.Take("bob", Take{Offer: m1.ID, Rate: 1 << 20}); g.Seconds != 0 {
		t.Fatalf("a second grinder leased the same offer: %+v", g)
	}
	if g := b.Take("alice", Take{Offer: m2.ID, Rate: 1 << 20}); g.Seconds != 0 || !strings.Contains(g.Reason, "lease limit 1") {
		t.Fatalf("a new grinder took a second lease: %+v", g)
	}
	// Claims: another grinder is reserved out, the wrong address and a
	// weak layer are refused, the right one is accepted.
	if a := b.Claim("bob", Claim{Offer: m1.ID, Nonce: 1}, 30, false); a.Outcome != Reserved {
		t.Fatalf("bob's claim %s", OutcomeName(a.Outcome))
	}
	if a := b.Claim("alice", Claim{Offer: m1.ID, Nonce: 1, PayTo: hash.Hash{0xb}}, 30, false); a.Outcome != Mismatch {
		t.Fatalf("wrong address %s", OutcomeName(a.Outcome))
	}
	if a := b.Claim("alice", Claim{Offer: m1.ID, Nonce: 1, PayTo: hash.Hash{0xa}}, 19, false); a.Outcome != TooWeak {
		t.Fatalf("weak %s", OutcomeName(a.Outcome))
	}
	if a := b.Claim("alice", Claim{Offer: m1.ID, Nonce: 2, PayTo: hash.Hash{0xa}}, 21, false); a.Outcome != Accepted {
		t.Fatalf("good %s", OutcomeName(a.Outcome))
	}
	if a := b.Claim("alice", Claim{Offer: m1.ID, Nonce: 2, PayTo: hash.Hash{0xa}}, 21, false); a.Outcome != Repeat {
		t.Fatalf("again %s", OutcomeName(a.Outcome))
	}
	// Filling one earns a second lease.
	if g := b.Take("alice", Take{Offer: m2.ID, Rate: 1 << 20}); g.Seconds == 0 {
		t.Fatalf("no second lease after a fill: %+v", g)
	}
	// The lease runs out: the offer is listed again.
	*now = now.Add(2 * time.Minute)
	if n := len(b.Listable(hash.Hash{}, 0)); n != 1 {
		t.Fatalf("after the lease ran out: %d listable", n)
	}
	// Restart: the accepted claim is still owed, then paid.
	b2, _ := payerBook(t, path)
	got, ok := b2.Get(m1.ID)
	if !ok || len(got.Claims) != 1 || got.Owed() != 10 {
		t.Fatalf("after restart %+v", got)
	}
	ack, peer, err := b2.Paid(m1.ID, 2, []hash.Hash{{5}})
	if err != nil || peer != "alice" || ack.Outcome != Paid {
		t.Fatalf("paid %+v %s %v", ack, peer, err)
	}
	if _, _, err := b2.Paid(m1.ID, 2, []hash.Hash{{5}}); err == nil {
		t.Fatal("paid twice")
	}
}

// A slow grinder's rate makes a long lease, capped at the ceiling, and a
// rate that cannot cover more work is refused another.
func TestBookLeaseFollowsRate(t *testing.T) {
	b, _ := payerBook(t, "")
	m, _ := b.Post(testOffer(30, 1))
	// 2^30 at 2^20 a second: about 1024 s expected, 3072 s leased.
	g := b.Take("carol", Take{Offer: m.ID, Rate: 1 << 20})
	if g.Seconds < 3000 || g.Seconds > 3100 {
		t.Fatalf("lease %d s", g.Seconds)
	}
	m2, _ := b.Post(testOffer(40, 1))
	b.grinderOf("carol").filled = 3
	if g := b.Take("carol", Take{Offer: m2.ID, Rate: 1 << 20}); g.Seconds != 0 || !strings.Contains(g.Reason, "rate") {
		t.Fatalf("over the rate: %+v", g)
	}
	// The one extension.
	if g := b.Take("carol", Take{Offer: m.ID, Rate: 1 << 20}); g.Seconds == 0 {
		t.Fatalf("extension refused: %+v", g)
	}
	if g := b.Take("carol", Take{Offer: m.ID, Rate: 1 << 20}); g.Seconds != 0 {
		t.Fatalf("a second extension: %+v", g)
	}
}

// The grinder's side: offers are kept only when taking is on and they pay
// enough, a peer's offers are capped, and the payer's answers are kept.
func TestBookHeldOffers(t *testing.T) {
	b, err := Open(Config{Take: true, MinPrice: 5, PerPeer: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	cheap := testOffer(20, 1)
	cheap.Amount = 4
	if b.Receive("p", cheap) {
		t.Fatal("kept an offer under min_price")
	}
	for i := uint64(1); i <= 3; i++ {
		o := testOffer(20, 1)
		o.Salt = i
		if !b.Receive("p", o) {
			t.Fatal("not kept")
		}
	}
	held := b.HeldOffers()
	if len(held) != 2 || held[0].Offer.Salt != 2 {
		t.Fatalf("per-peer cap: %+v", held)
	}
	b.Granted("p", Grant{Offer: held[0].ID, Seconds: 60}, hash.Hash{0xa})
	b.Acked("p", Ack{Offer: held[0].ID, Nonce: 1, Outcome: Accepted})
	b.Acked("p", Ack{Offer: held[0].ID, Nonce: 1, Outcome: Paid, TIDs: []hash.Hash{{1}}})
	if p := b.Payers()["p"]; p.Accepted != 1 || p.Paid != 1 {
		t.Fatalf("payer record %+v", p)
	}
	off, _ := Open(Config{}, "")
	if off.Receive("p", testOffer(20, 1)) {
		t.Fatal("kept an offer with take off")
	}
	if _, err := off.Post(testOffer(20, 1)); err == nil {
		t.Fatal("posted with post off")
	}
}
