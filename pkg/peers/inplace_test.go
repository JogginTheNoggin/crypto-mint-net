package peers_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// A book given new limits in place (SetLimits) holds what a node opening
// the same file with them holds after its start (Open, then Fit), lowered
// and raised, at the floors and the ceilings of node policy (ORANGE-035's
// in-place rule, D168).
func TestSetLimitsMatchesReopen(t *testing.T) {
	src := filepath.Join(t.TempDir(), "peers.json")
	start := peers.Limits{MaxRecords: 1024, PerRange: 64, CandidateAge: time.Hour}
	b, err := peers.Open(src, start)
	if err != nil {
		t.Fatal(err)
	}
	for r := 0; r < 16; r++ {
		for i := 0; i < 40; i++ {
			add23(t, b, fmt.Sprintf("66.%d.0.%d:1", r, i), peers.FromPeerList, t23.Add(time.Duration(r*40+i)*time.Second))
		}
	}
	for i := 0; i < 20; i++ {
		if err := b.AddUnlisted(fmt.Sprintf("8.%d.0.1:1", i), []wire.Section{series}, t23); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 200; i++ {
		a := fmt.Sprintf("77.%d.%d.1:1", i/16, i%16)
		add23(t, b, a, peers.FromReply, t23)
		b.Tried(a, true, t23)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	now := t23.Add(2 * time.Hour)
	addrs := func(b *peers.Book) []string {
		var out []string
		for _, r := range b.Records() {
			out = append(out, r.Addr)
		}
		slices.Sort(out)
		return out
	}
	for _, l := range []peers.Limits{
		{MaxRecords: 256, PerRange: 1},      // the floors
		{MaxRecords: 256, PerRange: 16},     // lowered
		{MaxRecords: 2048, PerRange: 128},   // raised
		{MaxRecords: 65536, PerRange: 4096}, // the ceilings
		{MaxRecords: 300, PerRange: 8, CandidateAge: time.Minute, Retry: time.Second},
	} {
		l.MaxUnlisted = max(1, l.MaxRecords/4)
		if l.CandidateAge == 0 {
			l.CandidateAge = 24 * time.Hour
		}
		if l.Retry == 0 {
			l.Retry = time.Minute
		}
		l.VerifiedPerRange = 4
		dir := t.TempDir()
		a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")
		for _, p := range []string{a, b} {
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		running, err := peers.Open(a, start)
		if err != nil {
			t.Fatal(err)
		}
		d1, o1 := running.SetLimits(l, now)
		reopened, err := peers.Open(b, l)
		if err != nil {
			t.Fatal(err)
		}
		d2, o2 := reopened.Fit(now)
		if d1 != d2 || o1 != o2 || !slices.Equal(addrs(running), addrs(reopened)) || running.Limits() != reopened.Limits() {
			t.Fatalf("%+v: in place deleted %d (%d over), %d records; a reopen deleted %d (%d over), %d records",
				l, d1, o1, len(running.Records()), d2, o2, len(reopened.Records()))
		}
		if running.Unlisted() != reopened.Unlisted() {
			t.Fatalf("%+v: imported %d in place, %d reopened", l, running.Unlisted(), reopened.Unlisted())
		}
	}
}
