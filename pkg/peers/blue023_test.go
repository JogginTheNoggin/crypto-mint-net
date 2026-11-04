package peers_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

var t23 = time.Unix(1_800_000_000, 0)

// add adds addr from src, or fails the test.
func add23(t *testing.T, b *peers.Book, addr string, src peers.Source, at time.Time) {
	t.Helper()
	if err := b.Add(addr, []wire.Section{series}, src, at); err != nil {
		t.Fatalf("add %s: %v", addr, err)
	}
}

// RED-065, D160: a refused add changes nothing. The book is full and the
// new address's range is full of records that do not give way; an imported
// record elsewhere must stay.
func TestRefusedAddChangesNothing(t *testing.T) {
	b, err := peers.Open("", peers.Limits{MaxRecords: 8, PerRange: 3, MaxUnlisted: 2})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := b.AddUnlisted(fmt.Sprintf("8.%d.0.1:1", i), []wire.Section{series}, t23); err != nil {
			t.Fatal(err)
		}
	}
	// A full range of records the node reached, labelled by the owner:
	// none gives way, even to the owner's own add (D162).
	for i := 0; i < 3; i++ {
		a := fmt.Sprintf("66.6.1.%d:1", i)
		add23(t, b, a, peers.FromPeerList, t23)
		b.Tried(a, true, t23)
		if _, err := b.SetLabel(a, "kept"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		add23(t, b, fmt.Sprintf("9.%d.0.1:1", i), peers.FromHand, t23)
	}
	before := b.Records()
	for _, src := range []peers.Source{peers.FromPeerList, peers.FromReply, peers.FromHand, peers.FromConfig} {
		err := b.Add("66.6.2.0:1", []wire.Section{series}, src, t23.Add(365*24*time.Hour))
		if !errors.Is(err, peers.ErrRangeFull) {
			t.Fatalf("%s: an address in a full range was not refused: %v", src, err)
		}
		if after := b.Records(); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s: a refused add changed the book (%d records before, %d after, %d imported)", src, len(before), len(after), b.Unlisted())
		}
	}
	// In a range with room, the imported record gives way as before.
	add23(t, b, "10.0.0.1:1", peers.FromPeerList, t23)
	if b.Unlisted() != 1 || len(b.Records()) != 8 {
		t.Fatalf("an add into a full book did not take an imported place: %d imported, %d records", b.Unlisted(), len(b.Records()))
	}
}

// RED-064, D160: learned candidates the node never reached give way; the
// owner's own add always finds a place among them; records the owner gave
// or the node reached keep theirs.
func TestLearnedCandidatesGiveWay(t *testing.T) {
	b, err := peers.Open("", peers.Limits{MaxRecords: 256, PerRange: 16, CandidateAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	// A hostile peer list fills the whole book from 16 ranges.
	for r := 0; r < 16; r++ {
		for i := 0; i < 16; i++ {
			add23(t, b, fmt.Sprintf("66.%d.0.%d:1", r, i), peers.FromPeerList, t23.Add(time.Duration(r*16+i)*time.Second))
		}
	}
	// A fresh learned address is refused while the junk is young and untried.
	if err := b.Add("20.0.0.1:1", []wire.Section{series}, peers.FromReply, t23.Add(time.Minute)); !errors.Is(err, peers.ErrFull) {
		t.Fatalf("a young untried candidate gave way: %v", err)
	}
	// The owner's add takes the oldest learned candidate's place at once.
	add23(t, b, "20.0.0.2:1", peers.FromHand, t23.Add(time.Minute))
	if _, held := b.Record("66.0.0.0:1"); held {
		t.Fatal("the owner's add did not take the oldest learned candidate's place")
	}
	add23(t, b, "20.0.0.3:1", peers.FromConfig, t23.Add(time.Minute))
	// A candidate whose last try failed gives way to any new address, the
	// oldest first; one that failed and then answered keeps its place.
	b.Tried("66.7.0.4:1", false, t23.Add(time.Minute))
	b.Tried("66.5.0.3:1", false, t23.Add(time.Minute))
	b.Tried("66.3.0.1:1", false, t23.Add(time.Minute))
	b.Tried("66.3.0.1:1", true, t23.Add(2*time.Minute))
	add23(t, b, "20.0.0.4:1", peers.FromPeerList, t23.Add(3*time.Minute))
	if _, held := b.Record("66.5.0.3:1"); held {
		t.Fatal("the oldest failed candidate stayed")
	}
	add23(t, b, "20.0.0.5:1", peers.FromInbound, t23.Add(3*time.Minute))
	if _, held := b.Record("66.7.0.4:1"); held {
		t.Fatal("the failed candidate stayed")
	}
	if _, held := b.Record("66.3.0.1:1"); !held {
		t.Fatal("a candidate that answered gave way")
	}
	// Past CandidateAge, an untried candidate gives way too, oldest first.
	late := t23.Add(2 * time.Hour)
	add23(t, b, "20.0.0.6:1", peers.FromReply, late)
	if _, held := b.Record("66.0.0.2:1"); held {
		t.Fatal("the oldest stale candidate stayed")
	}
	// A full range: the place comes from that range.
	add23(t, b, "66.9.1.1:1", peers.FromPeerList, late)
	if _, held := b.Record("66.9.0.0:1"); held {
		t.Fatal("the range's oldest candidate stayed")
	}
	if n := len(b.Records()); n != 256 {
		t.Fatalf("the book holds %d", n)
	}

	// Records that never give way to a learned address: a reached one, a
	// labelled one, a hand one, a configured one, a verified one. To the
	// owner's address only the labelled, hand and configured ones never
	// give way, and a peer verified now (RED-067, D162).
	k, _ := peers.Open("", peers.Limits{MaxRecords: 256, PerRange: 1, CandidateAge: time.Hour})
	add23(t, k, "30.0.0.1:1", peers.FromPeerList, t23)
	k.Tried("30.0.0.1:1", true, t23)
	k.Tried("30.0.0.1:1", false, t23) // reached once, failed since
	add23(t, k, "30.1.0.1:1", peers.FromReply, t23)
	if _, err := k.SetLabel("30.1.0.1:1", "mine"); err != nil {
		t.Fatal(err)
	}
	add23(t, k, "30.2.0.1:1", peers.FromHand, t23)
	add23(t, k, "30.3.0.1:1", peers.FromConfig, t23)
	if err := k.Verify("30.4.0.1:1", series, peers.Priority, t23); err != nil {
		t.Fatal(err)
	}
	if err := k.Verify("30.5.0.1:1", series, peers.Priority, t23); err != nil {
		t.Fatal(err)
	}
	k.Unverify("30.4.0.1:1")
	for _, a := range []string{"30.0.0.2:1", "30.1.0.2:1", "30.2.0.2:1", "30.3.0.2:1", "30.4.0.2:1", "30.5.0.2:1"} {
		if err := k.Add(a, []wire.Section{series}, peers.FromPeerList, t23.Add(1000*time.Hour)); !errors.Is(err, peers.ErrRangeFull) {
			t.Fatalf("%s: a kept record gave way to a learned address: %v", a, err)
		}
	}
	for _, a := range []string{"30.1.0.2:1", "30.2.0.2:1", "30.3.0.2:1", "30.5.0.2:1"} {
		if err := k.Add(a, []wire.Section{series}, peers.FromHand, t23.Add(1000*time.Hour)); !errors.Is(err, peers.ErrRangeFull) {
			t.Fatalf("%s: a kept record gave way to the owner: %v", a, err)
		}
	}
	for _, a := range []string{"30.0.0.2:1", "30.4.0.2:1"} {
		if err := k.Add(a, []wire.Section{series}, peers.FromConfig, t23.Add(1000*time.Hour)); err != nil {
			t.Fatalf("%s: a reached learned record did not give way to the owner: %v", a, err)
		}
	}
}

// RED-064, D160: a book kept under a larger max_book is brought down to a
// lower one by Fit, in the same order, and never below what it must keep.
func TestFitLoweredBook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	big, err := peers.Open(path, peers.Limits{MaxRecords: 1024, PerRange: 64})
	if err != nil {
		t.Fatal(err)
	}
	for r := 0; r < 16; r++ {
		for i := 0; i < 40; i++ {
			add23(t, big, fmt.Sprintf("66.%d.0.%d:1", r, i), peers.FromPeerList, t23.Add(time.Duration(r*40+i)*time.Second))
		}
	}
	for i := 0; i < 20; i++ {
		if err := big.AddUnlisted(fmt.Sprintf("8.%d.0.1:1", i), []wire.Section{series}, t23); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 300; i++ {
		a := fmt.Sprintf("77.%d.%d.1:1", i/16, i%16)
		add23(t, big, a, peers.FromReply, t23)
		big.Tried(a, true, t23)
	}
	if err := big.Save(); err != nil {
		t.Fatal(err)
	}
	if n := len(big.Records()); n != 960 {
		t.Fatalf("the big book holds %d", n)
	}

	// An open with the defaults (a command run offline) trims nothing.
	same, err := peers.Open(path, peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(same.Records()); n != 960 {
		t.Fatalf("Open trimmed the book to %d", n)
	}

	small, err := peers.Open(path, peers.Limits{MaxRecords: 256, PerRange: 16})
	if err != nil {
		t.Fatal(err)
	}
	if err := small.Add("20.0.0.1:1", []wire.Section{series}, peers.FromHand, t23); !errors.Is(err, peers.ErrFull) {
		t.Fatalf("an over-full book took an address before Fit: %v", err)
	}
	del, over := small.Fit(t23)
	// 300 reached records stay: 16 per range already (77.x), and the book
	// keeps them over its bound; every learned and imported record goes.
	if over != 300-256 || del != 960-300 || small.Unlisted() != 0 {
		t.Fatalf("Fit deleted %d, %d over, %d imported left", del, over, small.Unlisted())
	}
	for _, r := range small.Records() {
		if !r.Reached.Equal(t23) {
			t.Fatalf("Fit kept %s and dropped a reached record", r.Addr)
		}
	}

	// Without the reached ones, Fit lands on the bound, imported first.
	b3, err := peers.Open(path, peers.Limits{MaxRecords: 1024, PerRange: 64})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		b3.Delete(fmt.Sprintf("77.%d.%d.1:1", i/16, i%16))
	}
	if err := b3.Save(); err != nil {
		t.Fatal(err)
	}
	b4, err := peers.Open(path, peers.Limits{MaxRecords: 256, PerRange: 16, MaxUnlisted: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, over := b4.Fit(t23); over != 0 || len(b4.Records()) != 256 || b4.Unlisted() > 8 {
		t.Fatalf("Fit left %d records, %d over, %d imported", len(b4.Records()), over, b4.Unlisted())
	}
	counts := map[string]int{}
	for _, r := range b4.Records() {
		counts[peers.Range(r.Addr)]++
	}
	for rng, n := range counts {
		if n > 16 {
			t.Fatalf("range %s holds %d after Fit", rng, n)
		}
	}
	// The newest learned records are the ones kept.
	if _, held := b4.Record("66.15.0.39:1"); !held {
		t.Fatal("Fit dropped the newest learned record")
	}
}

// ORANGE-032's measure: a learned address into a full book of learned
// candidates at the max_book ceiling, which must scan the book for the
// one that gives way.
func BenchmarkAddFullBookLearned(b *testing.B) {
	book, _ := peers.Open("", peers.Limits{MaxRecords: 65536, PerRange: 4096, CandidateAge: time.Hour})
	for i := 0; i < 65536; i++ {
		_ = book.Add(fmt.Sprintf("66.%d.%d.%d:1", i>>12, (i>>8)&15, i&255), []wire.Section{series}, peers.FromPeerList, t23)
	}
	late := t23.Add(2 * time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = book.Add(fmt.Sprintf("20.%d.%d.%d:1", (i>>16)&255, (i>>8)&255, i&255), []wire.Section{series}, peers.FromPeerList, late)
	}
}
