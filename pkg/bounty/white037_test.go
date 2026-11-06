package bounty

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// An offer with fields a later release added after Salt keeps its id and
// its bytes through a decode, a list, a book and the JSON of bounties.json
// (W-27).
func TestW27OfferIDIsTheBytesReceived(t *testing.T) {
	o := testOffer(24, 2)
	o.Salt = 9
	later := append(o.Encode(), 1, 2, 3, 4, 5, 6, 7, 8)
	got, err := DecodeOffer(later)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != sha256.Sum256(later) || got.ID() == o.ID() {
		t.Fatalf("id %s, want the hash of the 8 trailing bytes too", got.ID().Short())
	}
	if !bytes.Equal(got.Encode(), later) {
		t.Fatal("the offer is not passed on as received")
	}
	list, err := DecodeList(EncodeList([]Offer{got}))
	if err != nil || len(list) != 1 || list[0].ID() != got.ID() {
		t.Fatalf("list %+v %v", list, err)
	}
	raw, _ := json.Marshal(got)
	var back Offer
	if err := json.Unmarshal(raw, &back); err != nil || back.ID() != got.ID() {
		t.Fatalf("json %s: %v", raw, err)
	}
	b, _ := Open(Config{Take: true}, "")
	if !b.Receive("p", got) {
		t.Fatal("not kept")
	}
	if h, ok := b.HeldOffer(sha256.Sum256(later)); !ok || !bytes.Equal(h.Offer.Encode(), later) {
		t.Fatal("the held offer lost its bytes")
	}
}

// bounties.json keeps what a later release wrote, at the top, in an offer
// and in a claim, and a later format is refused with a way out (W-28).
func TestW28FileKeepsLaterFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	b, _ := payerBook(t, path)
	m, _ := b.Post(testOffer(20, 2))
	b.Take("alice", Take{Offer: m.ID, Rate: 1 << 20, PayTo: hash.Hash{0xa}})
	b.Claim("alice", Claim{Offer: m.ID, Nonce: 1, PayTo: hash.Hash{0xa}}, 30, false)
	raw, _ := os.ReadFile(path)
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	f["later_top"] = 1
	offer := f["offers"].([]any)[0].(map[string]any)
	offer["later_offer"] = "x"
	offer["claims"].([]any)[0].(map[string]any)["later_claim"] = []any{1.0}
	raw, _ = json.Marshal(f)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	b2, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b2.Cancel(m.ID); err != nil { // a save
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	for _, k := range []string{`"later_top":1`, `"later_offer":"x"`, `"later_claim":[1]`} {
		if !strings.Contains(string(compactJSON(t, out)), k) {
			t.Errorf("lost %s:\n%s", k, out)
		}
	}
	f["format"] = 2
	raw, _ = json.Marshal(f)
	_ = os.WriteFile(path, raw, 0o600)
	if _, err := Open(b.Config(), path); err == nil || !strings.Contains(err.Error(), "later release") || !strings.Contains(err.Error(), "move the file aside") {
		t.Fatalf("format 2: %v", err)
	}
}

func compactJSON(t *testing.T, b []byte) []byte {
	var c bytes.Buffer
	if err := json.Compact(&c, b); err != nil {
		t.Fatal(err)
	}
	return c.Bytes()
}

// An offer that needs nothing more moves to the done file, appended
// before it leaves bounties.json; one found in both is not loaded twice;
// a lease's save waits on its timer and Close writes it (ORANGE-046 P1,
// P2).
func TestOrange046DoneFileAndLaterSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	b, _ := payerBook(t, path)
	m, _ := b.Post(testOffer(20, 1))
	keep, _ := b.Post(testOffer(20, 1))
	b.Take("alice", Take{Offer: m.ID, Rate: 1 << 20, PayTo: hash.Hash{0xa}})
	if a := b.Claim("alice", Claim{Offer: m.ID, Nonce: 1, PayTo: hash.Hash{0xa}}, 30, false); a.Outcome != Accepted {
		t.Fatal(OutcomeName(a.Outcome))
	}
	before, _ := os.ReadFile(path)
	if _, _, err := b.Paid(m.ID, 1, []hash.Hash{{1}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Get(m.ID); ok {
		t.Fatal("a paid, filled offer is still in the book")
	}
	done, _ := os.ReadFile(filepath.Join(dir, DoneFileName))
	lines := strings.Split(strings.TrimSpace(string(done)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], DoneFileKind) || !strings.Contains(lines[1], m.ID.String()) {
		t.Fatalf("done file:\n%s", done)
	}
	// A crash after the append and before the save leaves the offer in
	// both files: it is loaded from neither twice.
	_ = os.WriteFile(path, before, 0o600)
	b2, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b2.Get(m.ID); ok {
		t.Fatal("an offer in the done file was loaded again")
	}
	if _, ok := b2.Get(keep.ID); !ok {
		t.Fatal("the open offer was lost")
	}

	// A take is saved later, in one write for a burst, and Close writes
	// what still waits.
	saveDelay = time.Hour
	defer func() { saveDelay = 200 * time.Millisecond }()
	b2.Take("bob", Take{Offer: keep.ID, Rate: 1 << 20, PayTo: hash.Hash{0xb}})
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), `"bob"`) {
		t.Fatal("a take was written at once")
	}
	if err := b2.Close(); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), `"bob"`) {
		t.Fatal("Close did not write the waiting save")
	}
}

// Offers say their state and when they close, and acks name their
// outcome (GUI-GREEN-062).
func TestGUIGreen062StateAndNames(t *testing.T) {
	b, now := payerBook(t, "")
	m, _ := b.Post(testOffer(20, 1))
	if got, _ := b.Get(m.ID); got.State != StateOpen || got.Closes == nil || !got.Closes.Equal(now.Add(time.Hour)) {
		t.Fatalf("open: %s %v", got.State, got.Closes)
	}
	b.Take("alice", Take{Offer: m.ID, Rate: 1 << 20, PayTo: hash.Hash{0xa}})
	b.Claim("alice", Claim{Offer: m.ID, Nonce: 1, PayTo: hash.Hash{0xa}}, 30, false)
	if got, _ := b.Get(m.ID); got.State != StateFilled {
		t.Fatalf("filled: %s", got.State)
	}
	idle, _ := b.Post(testOffer(20, 1))
	*now = now.Add(2 * time.Hour)
	// A filled offer stays filled when its time runs out; an idle one is
	// expired (GUI-GREEN-063).
	if got, _ := b.Get(m.ID); got.State != StateFilled {
		t.Fatalf("filled, past its time: %s", got.State)
	}
	if got, _ := b.Get(idle.ID); got.State != StateExpired {
		t.Fatalf("expired: %s", got.State)
	}

	g, _ := Open(Config{Take: true}, "")
	o := testOffer(20, 1)
	o.Salt = 3
	g.Receive("payer", o)
	id := o.ID()
	g.Granted("payer", Grant{Offer: id, Seconds: 60}, hash.Hash{0xa})
	if h, _ := g.HeldOffer(id); h.State != StateLeased {
		t.Fatalf("leased: %s", h.State)
	}
	g.Acked("payer", Ack{Offer: id, Nonce: 1, Outcome: Filled})
	h, _ := g.HeldOffer(id)
	if h.State != StateFilled {
		t.Fatalf("filled: %s", h.State)
	}
	raw, _ := json.Marshal(h.Acks[0])
	if !strings.Contains(string(raw), `"outcome_name":"filled"`) {
		t.Fatalf("ack %s", raw)
	}
}

// The done file refuses a later format, reads an id followed by fields a
// later release added, and an offer that finished while the node was
// stopped moves there at start (W-31, GUI-GREEN-063, D200).
func TestW31DoneFileFormatAndStartMove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	done := filepath.Join(dir, DoneFileName)
	b, now := payerBook(t, path)
	m, _ := b.Post(testOffer(20, 1))
	_ = b.Close()

	// Past its time with nothing handed in, it is done: a new start moves
	// it, as its time ran out while the node was stopped.
	later := now.Add(2 * time.Hour)
	b2, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	b2.SetClock(func() time.Time { return later })
	b2.mu.Lock()
	b2.expire()
	b2.mu.Unlock()
	if err := b2.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := b2.Get(m.ID); ok {
		t.Fatal("an expired, empty offer stayed in the book")
	}
	raw, _ := os.ReadFile(done)
	if !strings.Contains(string(raw), m.ID.String()) {
		t.Fatalf("done file:\n%s", raw)
	}

	// A later release's line: the id first, then fields this one does not
	// know. The offer in both files is still left out.
	o := testOffer(20, 1)
	o.Salt = 77
	o.Raw = o.Encode()
	id := o.ID()
	line := `{"id":"` + id.String() + `","later":[1,2],"offer":{}}` + "\n"
	if err := os.WriteFile(done, append(raw, line...), 0o600); err != nil {
		t.Fatal(err)
	}
	f := file{Kind: FileKind, Format: FileFormat, Offers: []*Mine{{ID: id, Offer: o, Posted: time.Now()}}}
	fb, _ := json.Marshal(f)
	_ = os.WriteFile(path, fb, 0o600)
	b3, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b3.Get(id); ok {
		t.Fatal("an offer in the done file, after a field this release does not know, was loaded")
	}

	// A later format is refused, saying what to do.
	head := strings.Replace(string(raw), `"format":1`, `"format":2`, 1)
	_ = os.WriteFile(done, []byte(head), 0o600)
	if _, err := Open(b.Config(), path); err == nil || !strings.Contains(err.Error(), "later release") {
		t.Fatalf("format 2: %v", err)
	}
}

// A grinder states the strongest offer it works: every list request and
// take carries it, the payer lists and leases only offers at or under
// it, and the grinder keeps none stronger (jerry, D202).
func TestMaxStrengthChoosesOffers(t *testing.T) {
	for _, b := range [][]byte{ListReq{Series: hash.Hash{1}}.Encode(), Take{Offer: hash.Hash{2}}.Encode()} {
		if b[len(b)-1] != AnyStrength {
			t.Fatalf("no max strength written as any: %x", b)
		}
	}
	if _, err := DecodeListReq(make([]byte, 32)); err == nil {
		t.Fatal("a list request with no max strength decoded")
	}
	if _, err := DecodeTake(Take{}.Encode()[:72]); err == nil {
		t.Fatal("a take with no max strength decoded")
	}
	if _, err := DecodeListReq(append(make([]byte, 32), 0)); err == nil {
		t.Fatal("a list request with max strength 0 decoded")
	}
	if q, err := DecodeListReq(ListReq{MaxStrength: 24}.Encode()); err != nil || q.MaxStrength != 24 {
		t.Fatalf("list request %+v %v", q, err)
	}

	b, _ := payerBook(t, "")
	b.cfg.Budget = 1000
	easy, _ := b.Post(testOffer(20, 1))
	hard, _ := b.Post(testOffer(30, 1))
	if got := b.Listable(hash.Hash{}, 24); len(got) != 1 || got[0].ID() != easy.ID {
		t.Fatalf("listed for 24 bits: %d offers", len(got))
	}
	if n := len(b.Listable(hash.Hash{}, AnyStrength)); n != 2 {
		t.Fatalf("listed for any: %d", n)
	}
	if g := b.Take("slow", Take{Offer: hard.ID, Rate: 1 << 30, PayTo: hash.Hash{1}, MaxStrength: 24}); g.Seconds != 0 || !strings.Contains(g.Reason, "over the 24") {
		t.Fatalf("a lease over the grinder's max: %+v", g)
	}
	if g := b.Take("fast", Take{Offer: hard.ID, Rate: 1 << 30, PayTo: hash.Hash{1}, MaxStrength: 32}); g.Seconds == 0 {
		t.Fatalf("a fast grinder refused: %+v", g)
	}

	g, _ := Open(Config{Take: true, MaxStrength: 24}, "")
	if g.Receive("payer", hard.Offer) || !g.Receive("payer", easy.Offer) {
		t.Fatal("the grinder kept an offer over its max, or dropped one under it")
	}
}
