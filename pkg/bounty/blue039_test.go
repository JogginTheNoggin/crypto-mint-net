package bounty

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// BLUE-039 (RED-118, RED-119).

// claimed posts a one-layer offer and has peer claim it.
func claimed(t *testing.T, b *Book, peer string, payTo hash.Hash) Mine {
	t.Helper()
	m, err := b.Post(testOffer(20, 1))
	if err != nil {
		t.Fatal(err)
	}
	b.Take(peer, Take{Offer: m.ID, Rate: 1 << 20, PayTo: payTo})
	if a := b.Claim(peer, Claim{Offer: m.ID, Nonce: 1, PayTo: payTo}, 30, false); a.Outcome != Accepted {
		t.Fatal(OutcomeName(a.Outcome))
	}
	return m
}

func doneLines(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, DoneFileName))
	if err != nil {
		t.Fatal(err)
	}
	return strings.SplitAfter(string(raw), "\n")
}

// RED-118: a done file trimmed after bounties.json was written is read in
// full, so an offer archived just before a crash stays left out, and the
// change is reported.
func TestBlue039TrimmedDoneFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	b, _ := payerBook(t, path)
	a := claimed(t, b, "alice", hash.Hash{0xa})
	if _, _, err := b.Paid(a.ID, 1, []hash.Hash{{1}}); err != nil {
		t.Fatal(err)
	}
	x := claimed(t, b, "carol", hash.Hash{0xc})
	before, _ := os.ReadFile(path)
	if _, _, err := b.Paid(x.ID, 1, []hash.Hash{{2}}); err != nil {
		t.Fatal(err)
	}
	// The crash: bounties.json from before X's payment comes back. Then
	// the first record is trimmed from the done file.
	_ = os.WriteFile(path, before, 0o600)
	lines := doneLines(t, dir)
	if len(lines) != 4 || lines[3] != "" { // header, A, X, ""
		t.Fatalf("done file %q", lines)
	}
	_ = os.WriteFile(filepath.Join(dir, DoneFileName), []byte(lines[0]+lines[2]), 0o600)
	b2, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b2.Get(x.ID); ok {
		t.Fatal("an offer archived before the crash came back live with its unpaid claim")
	}
	if d := strings.Join(b2.Dropped(), "\n"); !strings.Contains(d, "changed since") || !strings.Contains(d, x.ID.Short()) {
		t.Fatalf("dropped: %s", d)
	}

	// Cut part way into a line, the same.
	_ = os.WriteFile(path, before, 0o600)
	_ = os.WriteFile(filepath.Join(dir, DoneFileName), []byte(lines[0]+lines[1][100:]+lines[2]), 0o600)
	if b3, err := Open(b.Config(), path); err != nil {
		t.Fatal(err)
	} else if _, ok := b3.Get(x.ID); ok {
		t.Fatal("an offer archived before the crash came back live (mid-line offset)")
	}
}

// RED-118: a done file whose last line was cut is reported at the start,
// and the next record is written on a line of its own, so its id is read.
func TestBlue039TornDoneFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	b, _ := payerBook(t, path)
	a := claimed(t, b, "alice", hash.Hash{0xa})
	if _, _, err := b.Paid(a.ID, 1, []hash.Hash{{1}}); err != nil {
		t.Fatal(err)
	}
	donePath := filepath.Join(dir, DoneFileName)
	raw, _ := os.ReadFile(donePath)
	_ = os.WriteFile(donePath, raw[:len(raw)-40], 0o600)
	b2, err := Open(b.Config(), path)
	if err != nil {
		t.Fatal(err)
	}
	if d := strings.Join(b2.Dropped(), "\n"); !strings.Contains(d, "cut short") {
		t.Fatalf("dropped: %s", d)
	}
	b2.SetClock(b.now)
	x := claimed(t, b2, "carol", hash.Hash{0xc})
	if _, _, err := b2.Paid(x.ID, 1, []hash.Hash{{2}}); err != nil {
		t.Fatal(err)
	}
	lines := doneLines(t, dir)
	if len(lines) != 4 || !strings.HasPrefix(lines[2], doneIDPrefix+x.ID.String()) {
		t.Fatalf("the record after a cut line is not a line of its own: %q", lines)
	}
	scan, err := doneIDs(donePath, 0, hash.Zero)
	if err != nil || !scan.ids[a.ID] || !scan.ids[x.ID] || scan.torn {
		t.Fatalf("ids read %v torn %v err %v", scan.ids, scan.torn, err)
	}
}

// RED-119: a payment whose save fails is not kept in memory either: this
// run and a restart agree the claim is unpaid, and paying it again (with
// the wallet's tids) records it.
func TestBlue039PaidSaveFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	b, _ := payerBook(t, path)
	m := claimed(t, b, "alice", hash.Hash{0xa})
	donePath := filepath.Join(dir, DoneFileName)
	if err := os.Mkdir(donePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Paid(m.ID, 1, []hash.Hash{{1}}); err == nil {
		t.Fatal("a payment that was not written was reported recorded")
	}
	if got, ok := b.Get(m.ID); !ok || len(got.Claims[0].Paid) != 0 {
		t.Fatalf("memory says paid while the file says unpaid: %+v", got.Claims)
	}
	if err := os.Remove(donePath); err != nil {
		t.Fatal(err)
	}
	if b2, err := Open(b.Config(), path); err != nil {
		t.Fatal(err)
	} else if got, ok := b2.Get(m.ID); !ok || len(got.Claims[0].Paid) != 0 {
		t.Fatalf("after a restart: %+v", got.Claims)
	}
	if _, _, err := b.Paid(m.ID, 1, []hash.Hash{{1}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Get(m.ID); ok {
		t.Fatal("the recorded payment did not archive the offer")
	}
	if l := doneLines(t, dir); len(l) != 3 || !bytes.Contains([]byte(l[1]), []byte(m.ID.String())) {
		t.Fatalf("done file %q", l)
	}
}
