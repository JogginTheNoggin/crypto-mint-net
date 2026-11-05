package ledger_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// mapSource is a BodySource that keeps entries as bytes, standing in for a
// pool log. Written entries become loadable; fail makes every load fail.
type mapSource struct {
	mu    sync.Mutex
	s     *series.Series
	bytes map[hash.Hash][]byte
	loads int
	fail  bool
}

func newMapSource(s *series.Series) *mapSource {
	return &mapSource{s: s, bytes: map[hash.Hash][]byte{}}
}

func (m *mapSource) write(e *entry.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bytes[e.Hash(m.s)] = e.Encode()
}

func (m *mapSource) Has(h hash.Hash) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.bytes[h]
	return ok
}

func (m *mapSource) Load(h hash.Hash) (*entry.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	b, ok := m.bytes[h]
	if !ok || m.fail {
		return nil, errors.New("not loadable")
	}
	e, err := entry.Decode(b)
	if err != nil {
		return nil, err
	}
	if e.Hash(m.s) != h {
		return nil, errors.New("wrong entry")
	}
	return e, nil
}

// twin is a pool with bodies on a source and a tiny cache, fed the same
// entries and layers as the pool under test, and compared with it after
// every step (D172: the cache changes what a node costs, never what it
// decides).
type twin struct {
	src   *mapSource
	cache *ledger.BodyCache
	pool  *ledger.Pool
}

func newTwin(s *series.Series, cfg ledger.Config) *twin {
	tw := &twin{src: newMapSource(s), cache: ledger.NewBodyCache(1024)}
	cfg.Source, cfg.Bodies = tw.src, tw.cache
	tw.pool = ledger.New(s, 1, cfg)
	return tw
}

func (tw *twin) addEntry(t *testing.T, step int, e *entry.Entry, wantErr error) *ledger.Switch {
	t.Helper()
	res, err := tw.pool.AddEntry(e)
	if (err == nil) != (wantErr == nil) {
		t.Fatalf("step %d: twin AddEntry %v, pool %v", step, err, wantErr)
	}
	if err == nil {
		tw.src.write(e) // the node writes the record after the pool takes it
	}
	return res.Switch
}

// sameSwitch checks that the twin, whose bodies are on its source and
// whose items have no witnesses, sorts a switch's displaced transactions
// as the pool does, and carries each with its witnesses (ORANGE-038).
func sameSwitch(t *testing.T, step int, want, got *ledger.Switch) {
	t.Helper()
	if (want == nil) != (got == nil) {
		t.Fatalf("step %d: switch %v, twin %v", step, want != nil, got != nil)
	}
	if want == nil {
		return
	}
	for _, c := range []struct {
		name      string
		want, got []ledger.Displaced
	}{{"unchanged", want.Unchanged, got.Unchanged}, {"carried", want.Carried, got.Carried}, {"void", want.Void, got.Void}} {
		if len(c.want) != len(c.got) {
			t.Fatalf("step %d: %d %s, twin %d", step, len(c.want), c.name, len(c.got))
		}
		for i := range c.want {
			if c.want[i].TID != c.got[i].TID || c.want[i].Entry != c.got[i].Entry {
				t.Fatalf("step %d: %s %d differs", step, c.name, i)
			}
		}
	}
	for _, c := range []struct {
		name      string
		want, got []ledger.Displaced
	}{{"unchanged", want.Unchanged, got.Unchanged}, {"carried", want.Carried, got.Carried}, {"void", want.Void, got.Void}} {
		for i, d := range c.got {
			if !bytes.Equal(d.Tx.Encode(), c.want[i].Tx.Encode()) {
				t.Fatalf("step %d: %s %s comes back without its witnesses", step, c.name, d.TID.Short())
			}
		}
	}
}

func (tw *twin) compare(t *testing.T, step int, p *ledger.Pool) {
	t.Helper()
	t1, h1, w1 := p.Tip()
	t2, h2, w2 := tw.pool.Tip()
	if t1 != t2 || h1 != h2 || w1.Cmp(w2) != 0 || p.Len() != tw.pool.Len() {
		t.Fatalf("step %d: twin selects %s (%d, %s, %d held), pool %s (%d, %s, %d held)", step, t2.Short(), h2, w2, tw.pool.Len(), t1.Short(), h1, w1, p.Len())
	}
	if snapshotOf(p.State()) != snapshotOf(tw.pool.State()) {
		t.Fatalf("step %d: twin's tip state differs", step)
	}
	if err := ledger.CheckOff(tw.pool); err != nil {
		t.Fatalf("step %d: twin: %v", step, err)
	}
}

// compareBodies checks that every held entry's body reads back the same
// and that the twin keeps at most the latest body in memory.
func (tw *twin) compareBodies(t *testing.T, p *ledger.Pool, held []hash.Hash) {
	t.Helper()
	tw.pool.ReleaseBodies()
	if n := tw.pool.InMemory(); n != 0 {
		t.Fatalf("twin keeps %d bodies in memory after ReleaseBodies", n)
	}
	for _, h := range held {
		a, err1 := p.Entry(h)
		b, err2 := tw.pool.Entry(h)
		ok1, ok2 := err1 == nil, err2 == nil
		if ok1 != ok2 {
			t.Fatalf("entry %s: held by pool %v, twin %v", h.Short(), ok1, ok2)
		}
		if ok1 && !bytes.Equal(a.Encode(), b.Encode()) {
			t.Fatalf("entry %s: twin's body differs", h.Short())
		}
		if ok1 {
			pa, _ := p.ParentOf(h)
			pb, _ := tw.pool.ParentOf(h)
			if pa != pb {
				t.Fatalf("entry %s: parent differs", h.Short())
			}
		}
	}
	if st := tw.cache.Stats(); st.UsedBytes > st.MaxBytes {
		t.Fatalf("cache uses %d bytes over its bound %d", st.UsedBytes, st.MaxBytes)
	}
}

func TestBodyCacheBound(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	tw := newTwin(f.s, ledger.Config{K: ledger.Unlimited})
	parent := f.pool.ID()
	var hs []hash.Hash
	for i := 0; i < 8; i++ {
		e := f.build(parent, f.k.Claim(uint64(4+i%4), P("o1", 100)))
		if i >= 4 {
			e = f.build(parent)
		}
		f.add(e)
		tw.addEntry(t, i, e, nil)
		parent = e.Hash(f.s)
		hs = append(hs, parent)
	}
	tw.compareBodies(t, f.pool, hs)
	st := tw.cache.Stats()
	if st.Loads == 0 || st.UsedBytes > 1024 {
		t.Fatalf("stats %+v: want loads and at most 1024 bytes", st)
	}
	// A cache of no bytes keeps nothing and still serves every body.
	tw.cache.SetMax(0)
	if st := tw.cache.Stats(); st.Bodies != 0 || st.UsedBytes != 0 {
		t.Fatalf("after SetMax(0): %+v", st)
	}
	tw.compareBodies(t, f.pool, hs)
	// A failed read hides the body and decides nothing.
	tw.src.fail = true
	if e, err := tw.pool.Entry(hs[0]); !errors.Is(err, ledger.ErrUnreadable) || e != nil {
		t.Fatalf("a failed read: %v, %v", e, err)
	}
	if tw.cache.Stats().Errors == 0 {
		t.Fatal("a failed read was not counted")
	}
	e := f.build(parent)
	f.add(e)
	tw.addEntry(t, 99, e, nil)
	tw.compare(t, 99, f.pool)
}
