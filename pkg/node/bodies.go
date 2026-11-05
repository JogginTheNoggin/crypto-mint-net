package node

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

// Entry bodies on disk (D172). Each pool's ledger keeps a skeleton of every
// entry it holds and drops the entry's body once the body's record is in
// the pool's log; bodies are read back by location through the node's
// one BodyCache, bounded by entry_cache_mib. The locations are built while
// the log is replayed at start and as records are appended, and rebuilt by
// each compaction, so no index file is kept (a sidecar index comes with
// snapshots, R30).

// poolBodies is a pool's BodySource: where in its log each held entry's
// record is.
//
// Lock order: the engine's lock, then mu, then the log's own. A
// compaction holds mu while it rewrites the log, so a body read waits for
// the new locations rather than failing on old ones; it never takes the
// engine's lock while holding mu.
type poolBodies struct {
	mu   sync.Mutex
	s    *series.Series
	log  *store.Log
	locs map[hash.Hash]store.Loc
	// damaged is set once a record the index points at failed to read
	// back: the log is damaged or was changed under the node. The pool's
	// snapshot is then removed and no new one written, so the next start
	// replays the log whole, which cuts it at the first damaged record,
	// instead of starting again from a snapshot that skips the damage
	// (D178).
	damaged atomic.Bool
}

var errNoBody = errors.New("node: no stored record of this entry")

func (b *poolBodies) add(h hash.Hash, loc store.Loc) {
	b.mu.Lock()
	b.locs[h] = loc
	b.mu.Unlock()
}

func (b *poolBodies) locOf(h hash.Hash) (store.Loc, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	loc, ok := b.locs[h]
	return loc, ok
}

func (b *poolBodies) has(h hash.Hash) bool {
	_, ok := b.locOf(h)
	return ok
}

// load reads entry h back from the log, and gives it only if it is an
// entry whose hash is h.
func (b *poolBodies) load(h hash.Hash) (*entry.Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	loc, ok := b.locs[h]
	if !ok || b.log == nil {
		return nil, errNoBody
	}
	e, err := b.read(h, loc)
	if err != nil {
		b.damaged.Store(true)
	}
	return e, err
}

func (b *poolBodies) read(h hash.Hash, loc store.Loc) (*entry.Entry, error) {
	r, err := b.log.ReadAt(loc)
	if err != nil {
		return nil, err
	}
	if r.Kind != store.KindEntry {
		return nil, fmt.Errorf("%w: record of kind %d", store.ErrBadRecord, r.Kind)
	}
	e, err := entry.Decode(r.Data)
	if err != nil {
		return nil, err
	}
	if e.Hash(b.s) != h {
		return nil, fmt.Errorf("%w: the record holds another entry", store.ErrBadRecord)
	}
	return e, nil
}

// source is what the engine gives each pool state it makes. It looks up
// the pool's poolBodies on each call, so a state only read and dropped
// (RED-049) costs the node nothing, and a pool whose log is opened later
// finds it.
type source struct {
	n *Node
	k engine.PoolKey
}

func (s source) Has(h hash.Hash) bool {
	b := s.n.poolBodies(s.k)
	return b != nil && b.has(h)
}

func (s source) Load(h hash.Hash) (*entry.Entry, error) {
	b := s.n.poolBodies(s.k)
	if b == nil {
		return nil, errNoBody
	}
	return b.load(h)
}

// poolBodies returns pool k's locations, or nil before its log is open.
func (n *Node) poolBodies(k engine.PoolKey) *poolBodies {
	n.bmu.Lock()
	defer n.bmu.Unlock()
	return n.bodies[k]
}

// openBodies returns pool k's locations, making them for log l.
func (n *Node) openBodies(k engine.PoolKey, l *store.Log) *poolBodies {
	s, _ := n.eng.Series(k.Series)
	n.bmu.Lock()
	defer n.bmu.Unlock()
	b := n.bodies[k]
	if b == nil {
		b = &poolBodies{s: s, locs: map[hash.Hash]store.Loc{}}
		n.bodies[k] = b
	}
	b.mu.Lock()
	b.log = l
	b.mu.Unlock()
	return b
}

// BodyCache returns the node's entry body cache.
func (n *Node) BodyCache() *ledger.BodyCache { return n.cache }

// BodyStats describes where the node's entry bodies are.
type BodyStats struct {
	ledger.BodyStats
	// Held counts the entries the node's pools hold, and InMemory those
	// whose bodies are in the pool itself (not yet written, or with no
	// record to read back).
	Held     int `json:"held"`
	InMemory int `json:"in_memory"`
	// Located counts the entry records whose places in the logs are known.
	Located int `json:"located"`
	// Cold counts the entries whose items and undo records are read back
	// from the logs when needed, outside recent_window (D175).
	Cold int `json:"cold"`
}

// Bodies reports the entry body cache and how many bodies are in memory,
// over the pools with a log.
func (n *Node) Bodies() BodyStats {
	st := BodyStats{BodyStats: n.cache.Stats()}
	n.bmu.Lock()
	keys := make([]engine.PoolKey, 0, len(n.bodies))
	bs := make([]*poolBodies, 0, len(n.bodies))
	for k, b := range n.bodies {
		keys = append(keys, k)
		bs = append(bs, b)
	}
	n.bmu.Unlock()
	for _, k := range keys {
		n.eng.Ledger(k, func(p *ledger.Pool) {
			st.Held += p.Len()
			st.InMemory += p.InMemory()
			st.Cold += p.Cold()
		})
	}
	for _, b := range bs {
		b.mu.Lock()
		st.Located += len(b.locs)
		b.mu.Unlock()
	}
	return st
}
