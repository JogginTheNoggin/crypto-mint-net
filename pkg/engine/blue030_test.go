package engine_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// failingSource keeps entries as written and fails every load once fail
// is set, standing in for a pool log damaged under the node.
type failingSource struct {
	mu     sync.Mutex
	s      *series.Series
	bodies map[hash.Hash]*entry.Entry
	fail   bool
}

func (f *failingSource) put(e *entry.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[e.Hash(f.s)] = e
}

func (f *failingSource) Has(h hash.Hash) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.bodies[h]
	return ok
}

func (f *failingSource) Load(h hash.Hash) (*entry.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.bodies[h]; ok && !f.fail {
		return e, nil
	}
	return nil, errors.New("injected read failure")
}

// coldEngine returns an engine holding a chain of n entries, almost all
// of them cold (a window of 2, no cache), with its source set to fail.
func coldEngine(t *testing.T, n int) (*engine.Engine, engine.PoolKey, []*entry.Entry, *series.Series, *testutil.Keys) {
	t.Helper()
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	src := &failingSource{s: s, bodies: map[hash.Hash]*entry.Entry{}}
	e := engine.New(engine.Config{
		Ledger: ledger.Config{K: ledger.Unlimited, RecentWindow: 2, Bodies: ledger.NewBodyCache(0)},
		Source: func(engine.PoolKey) ledger.BodySource { return src },
	})
	e.AddSeries(s)
	if err := e.FollowSeries(s.ID()); err != nil {
		t.Fatal(err)
	}
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	es, _ := chainOf(t, s, k, n)
	for _, en := range es {
		src.put(en) // the record is written as the pool takes the entry
		if _, err := e.HandleEntry(key, en, engine.Pushed); err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.Work(key, en.Hash(s), 0, 64); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := e.Status(key); st.Height != n {
		t.Fatalf("height %d, want %d", st.Height, n)
	}
	e.Ledger(key, func(p *ledger.Pool) { p.ReleaseBodies() })
	if c := cold(e, key); c < n/2 {
		t.Fatalf("%d cold entries of %d", c, n)
	}
	src.mu.Lock()
	src.fail = true
	src.mu.Unlock()
	return e, key, es, s, k
}

func cold(e *engine.Engine, key engine.PoolKey) (c int) {
	e.Ledger(key, func(p *ledger.Pool) { c = p.Cold() })
	return c
}

// failsAndAnswers calls f, which must panic with a ledger.ReadFault, then
// checks that the engine failed and still answers: its lock was released.
func failsAndAnswers(t *testing.T, e *engine.Engine, key engine.PoolKey, f func()) {
	t.Helper()
	func() {
		defer func() {
			if _, ok := recover().(ledger.ReadFault); !ok {
				t.Fatal("no ReadFault reached the caller")
			}
		}()
		f()
	}()
	done := make(chan error, 1)
	go func() {
		_, err := e.Status(key)
		if err == nil {
			err = e.Err()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, engine.ErrFailed) {
			t.Fatalf("Err after the fault: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the engine's lock is still held after the fault")
	}
	select {
	case <-e.Failed():
	default:
		t.Fatal("Failed not closed after a read fault")
	}
}

// RED-085: a read fault on a cold entry in the paths that take the lock
// outside handling (BuildEntryOn, PreverifyEntry, PreverifyPage) fails the
// engine and releases its lock, so the node answers and can stop.
func TestColdReadFaultReleasesTheLock(t *testing.T) {
	t.Run("BuildEntryOn", func(t *testing.T) {
		e, key, es, s, k := coldEngine(t, 12)
		parent := es[0].Hash(s)
		first, _ := s.PoolRange(1)
		failsAndAnswers(t, e, key, func() {
			e.BuildEntryOn(context.Background(), key, 0, engine.Grind{}, engine.BuildOn{Parent: &parent, Txs: []*tx.Tx{k.Claim(first+1, P("zed", 100))}})
		})
		if _, _, err := e.BuildEntryOn(context.Background(), key, 0, engine.Grind{}, engine.BuildOn{}); !errors.Is(err, engine.ErrFailed) {
			t.Fatalf("a build after the failure: %v", err)
		}
	})
	t.Run("PreverifyEntry", func(t *testing.T) {
		e, key, es, s, k := coldEngine(t, 12)
		first, _ := s.PoolRange(1)
		rival, _, err := entry.Build(s, es[0].Hash(s), []*tx.Tx{k.Claim(first+1, P("zed", 100))}, 0)
		if err != nil {
			t.Fatal(err)
		}
		failsAndAnswers(t, e, key, func() { e.PreverifyEntry(key, rival, engine.Pushed) })
	})
	t.Run("PreverifyPage", func(t *testing.T) {
		e, key, es, s, k := coldEngine(t, 12)
		first, _ := s.PoolRange(1)
		r1, _, _ := entry.Build(s, es[0].Hash(s), []*tx.Tx{k.Claim(first+1, P("zed", 100))}, 0)
		r2, _, _ := entry.Build(s, r1.Hash(s), []*tx.Tx{k.Claim(first+2, P("zed", 100))}, 0)
		failsAndAnswers(t, e, key, func() { e.PreverifyPage(key, []*entry.Entry{r1, r2}) })
	})
}
