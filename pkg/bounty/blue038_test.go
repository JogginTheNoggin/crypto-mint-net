package bounty

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// BLUE-038 (RED-115..117).

// RED-115: an offer over MaxOfferBytes is refused, alone or in a list.
func TestBlue038OfferBytesBounded(t *testing.T) {
	o := testOffer(20, 1)
	raw := o.Encode()
	if _, err := DecodeOffer(append(raw, make([]byte, MaxOfferBytes-len(raw))...)); err != nil {
		t.Fatalf("an offer at the bound: %v", err)
	}
	if _, err := DecodeOffer(append(raw, make([]byte, 8<<20)...)); err == nil {
		t.Fatal("an 8 MiB offer was kept")
	}
}

// RED-116: the offers held from one network address are bounded as one
// peer's, whatever node ids it uses; an offer this node holds a lease on
// is not the one pushed out; the grinder record that goes is the one
// that earned least.
func TestBlue038HeldPerAddress(t *testing.T) {
	b, _ := Open(Config{Take: true, Held: 8, PerPeer: 4}, "")
	now := time.Unix(1_800_000_000, 0)
	b.SetClock(func() time.Time { return now })
	own := testOffer(20, 1)
	own.Salt = 999
	b.ReceiveFrom("payer", "198.51.100.1", own)
	b.Granted("payer", Grant{Offer: own.ID(), Seconds: 600}, hash.Hash{0xa})
	for i := range 32 {
		o := testOffer(20, 1)
		o.Salt = uint64(i + 1)
		b.ReceiveFrom(string(rune('a'+i%4)), "192.0.2.7", o)
	}
	from := 0
	for _, h := range b.HeldOffers() {
		if h.Addr == "192.0.2.7" {
			from++
		}
	}
	if from > 4 {
		t.Fatalf("one address holds %d offers, over its 4", from)
	}
	if _, ok := b.HeldOffer(own.ID()); !ok {
		t.Fatal("the offer this node leased was pushed out")
	}

	g, _ := payerBook(t, "")
	for i := range maxPeerRecords - 1 {
		g.grinderOf(string(rune(0x1000 + i)))
	}
	g.grinderOf("earner").filled = 3
	g.grinderOf("new")
	if g.filled("earner") != 3 {
		t.Fatal("the grinder that earned leases was forgotten")
	}
}

// RED-117: the done file is read a line's start at a time, so a huge line
// costs nothing, a line that is not ours is counted, and an offer left
// out for being done is said.
func TestBlue038DoneFileBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	b, _ := payerBook(t, path)
	m, _ := b.Post(testOffer(20, 1))
	keep, _ := b.Post(testOffer(20, 1))
	before, _ := os.ReadFile(path)
	b.Take("alice", Take{Offer: m.ID, Rate: 1 << 20, PayTo: hash.Hash{0xa}})
	b.Claim("alice", Claim{Offer: m.ID, Nonce: 1, PayTo: hash.Hash{0xa}}, 30, false)
	b.Paid(m.ID, 1, []hash.Hash{{1}})
	f, _ := os.OpenFile(filepath.Join(dir, DoneFileName), os.O_APPEND|os.O_WRONLY, 0o600)
	f.Write([]byte(`{"id":"` + strings.Repeat("ab", 32) + `","x":"`))
	f.Write(bytes.Repeat([]byte{'z'}, 64<<20))
	f.Write([]byte("\"}\nnot ours\n"))
	f.Close()
	os.WriteFile(path, before, 0o600) // a crash before the save
	var ms0, ms1 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	b2, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&ms1)
	if got := ms1.TotalAlloc - ms0.TotalAlloc; got > 16<<20 {
		t.Fatalf("reading a 64 MiB line allocated %d bytes", got)
	}
	if _, ok := b2.Get(m.ID); ok {
		t.Fatal("a finished offer was loaded again")
	}
	if _, ok := b2.Get(keep.ID); !ok {
		t.Fatal("the open offer was lost")
	}
	d := strings.Join(b2.Dropped(), "\n")
	if !strings.Contains(d, "already, finished") || !strings.Contains(d, "1 lines") {
		t.Fatalf("dropped:\n%s", d)
	}
}
