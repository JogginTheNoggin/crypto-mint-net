package ledger_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
)

// snapshotRecords writes p's snapshot and returns its records.
func snapshotRecords(t *testing.T, p *ledger.Pool) [][]byte {
	t.Helper()
	var recs [][]byte
	if err := p.Snapshot(func(b []byte) error {
		recs = append(recs, append([]byte(nil), b...))
		return nil
	}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return recs
}

func reader(recs [][]byte) func() ([]byte, error) {
	i := 0
	return func() ([]byte, error) {
		if i == len(recs) {
			return nil, nil
		}
		i++
		return recs[i-1], nil
	}
}

// restore replaces the twin's pool with one restored from its snapshot,
// after checking that the restored pool's fingerprint is the twin's and
// the in-memory pool's (D178). The random history then goes on against
// the restored pool, so every later step checks that it decides as the
// pool it was snapshotted from would have.
func (tw *twin) restore(t *testing.T, step int, mem *ledger.Pool) {
	t.Helper()
	tw.pool.ReleaseBodies()
	want := tw.pool.Fingerprint()
	if got := mem.Fingerprint(); got != want {
		t.Fatalf("step %d: twin's fingerprint differs from the in-memory pool's", step)
	}
	cfg := tw.cfg
	g, err := ledger.Restore(tw.pool.Series(), tw.pool.Index(), cfg, reader(snapshotRecords(t, tw.pool)))
	if err != nil {
		t.Fatalf("step %d: restore: %v", step, err)
	}
	if got := g.Fingerprint(); got != want {
		t.Fatalf("step %d: restored pool's fingerprint differs", step)
	}
	if err := ledger.CheckOff(g); err != nil {
		t.Fatalf("step %d: restored: %v", step, err)
	}
	tw.compare(t, step, g)
	tw.pool = g
	tw.restores++
}

// TestRestoreRefuses: a snapshot that is cut short, has records after its
// end, belongs to another pool, or names an entry the store does not hold
// is refused with ErrBadSnapshot, never restored part way.
func TestRestoreRefuses(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	tw := newTwin(f.s, ledger.Config{K: ledger.Unlimited})
	e1 := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	tw.addEntry(t, 0, e1, nil)
	e2 := f.build(e1.Hash(f.s), f.k.Claim(5, P("bob", 100)))
	tw.addEntry(t, 1, e2, nil)
	tw.pool.AddLayer(layerOf(t, f.s, e2.Hash(f.s), 3, 1))
	tw.pool.ReleaseBodies()
	recs := snapshotRecords(t, tw.pool)
	if _, err := ledger.Restore(f.s, 1, tw.cfg, reader(recs)); err != nil {
		t.Fatalf("whole snapshot: %v", err)
	}
	for name, rs := range map[string][][]byte{
		"cut short":       recs[:len(recs)-1],
		"extra record":    append(append([][]byte(nil), recs...), recs[1]),
		"entries swapped": {recs[0], recs[2], recs[1], recs[3]},
		"empty":           {},
	} {
		if _, err := ledger.Restore(f.s, 1, tw.cfg, reader(rs)); !errors.Is(err, ledger.ErrBadSnapshot) {
			t.Errorf("%s: %v, want ErrBadSnapshot", name, err)
		}
	}
	if _, err := ledger.Restore(f.s, 2, tw.cfg, reader(recs)); !errors.Is(err, ledger.ErrBadSnapshot) {
		t.Errorf("another pool: %v", err)
	}
	// A body the store no longer holds.
	delete(tw.src.bytes, e1.Hash(f.s))
	if _, err := ledger.Restore(f.s, 1, tw.cfg, reader(recs)); !errors.Is(err, ledger.ErrBadSnapshot) {
		t.Errorf("missing body: %v", err)
	}
	// A pool with an unwritten body cannot be snapshotted yet.
	e3 := f.build(e2.Hash(f.s), f.k.Claim(6, P("carol", 100)))
	if _, err := tw.pool.AddEntry(e3); err != nil {
		t.Fatal(err)
	}
	if err := tw.pool.Snapshot(func([]byte) error { return nil }); !errors.Is(err, ledger.ErrNotSnapshottable) {
		t.Errorf("unwritten body: %v", err)
	}
}

// TestRestoreAnyWindow: a snapshot restores to the same pool whatever
// recent_window the restoring node runs, including none.
func TestRestoreAnyWindow(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	tw := newTwin(f.s, ledger.Config{K: ledger.Unlimited})
	parent := f.pool.ID()
	for i := uint64(0); i < 4; i++ {
		e := f.build(parent, f.k.Claim(4+i, P("alice", 100)))
		tw.addEntry(t, int(i), e, nil)
		tw.pool.AddLayer(layerOf(t, f.s, e.Hash(f.s), 2, i))
		parent = e.Hash(f.s)
	}
	tw.pool.ReleaseBodies()
	want := tw.pool.Fingerprint()
	recs := snapshotRecords(t, tw.pool)
	for _, w := range []int{0, 1, 2, 3, 100} {
		cfg := tw.cfg
		cfg.RecentWindow = w
		g, err := ledger.Restore(f.s, 1, cfg, reader(recs))
		if err != nil {
			t.Fatalf("window %d: %v", w, err)
		}
		if g.Fingerprint() != want {
			t.Fatalf("window %d: fingerprint differs", w)
		}
		if wantCold := max(0, 4-w); w == 0 && g.Cold() != 0 || w > 0 && g.Cold() != wantCold {
			t.Fatalf("window %d: %d cold, want %d", w, g.Cold(), wantCold)
		}
	}
}
