package ledger

import (
	"container/list"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// Entry bodies on disk (D172). A pool keeps a skeleton of every entry it
// holds (hash, parent, height, weight, layers, items and undo), which is
// all that selection, switching and checking read. With a BodySource, an
// entry's body (its transactions and witnesses, the largest part) is
// dropped from the pool once the source can give it back, and read
// through a BodyCache bounded in bytes when something asks for it:
// serving a peer, a query, a rival search, a compaction. No verdict reads
// a body that is not in memory, so the cache's size changes what a node
// costs, never what it decides.

// BodySource gives back the body of an entry from outside memory, in
// practice the pool's log on disk.
type BodySource interface {
	// Has reports whether Load can give back the body of entry h.
	Has(h hash.Hash) bool
	// Load returns the body of entry h. It must return the entry whose
	// hash is h, or an error.
	Load(h hash.Hash) (*entry.Entry, error)
}

// BodyCache holds entry bodies read back from a BodySource, the most
// recently used first, within a bound in bytes. One cache is shared by
// every pool of a node. It is safe for concurrent use.
type BodyCache struct {
	mu    sync.Mutex
	max   int64
	used  int64
	lru   *list.List // of *cached, the most recently used at the front
	items map[bodyKey]*list.Element
	stats BodyStats
}

// BodyStats describes a BodyCache.
type BodyStats struct {
	// MaxBytes and UsedBytes are the bound and what the cached bodies
	// take, by their size estimate (bodySize).
	MaxBytes  int64 `json:"max_bytes"`
	UsedBytes int64 `json:"used_bytes"`
	Bodies    int   `json:"bodies"`
	// Hits and Loads count bodies found in the cache and read back from
	// disk; Errors counts reads that failed.
	Hits   uint64 `json:"hits"`
	Loads  uint64 `json:"loads"`
	Errors uint64 `json:"errors"`
}

type bodyKey struct {
	pool hash.Hash
	h    hash.Hash
}

type cached struct {
	key  bodyKey
	e    *entry.Entry
	size int64
}

// NewBodyCache returns a cache that holds at most maxBytes of bodies. A
// body larger than the whole bound is not kept.
func NewBodyCache(maxBytes int64) *BodyCache {
	return &BodyCache{max: maxBytes, lru: list.New(), items: map[bodyKey]*list.Element{}}
}

// bodySize estimates the memory one body takes: its byte form and the
// structures that hold it.
func bodySize(e *entry.Entry) int64 { return int64(e.Size())*2 + 256 }

func (c *BodyCache) get(k bodyKey) (*entry.Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[k]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(el)
	c.stats.Hits++
	return el.Value.(*cached).e, true
}

func (c *BodyCache) put(k bodyKey, e *entry.Entry) {
	size := bodySize(e)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[k]; ok {
		c.lru.MoveToFront(el)
		return
	}
	if size > c.max {
		return
	}
	c.items[k] = c.lru.PushFront(&cached{k, e, size})
	c.used += size
	for c.used > c.max {
		el := c.lru.Back()
		x := el.Value.(*cached)
		c.lru.Remove(el)
		delete(c.items, x.key)
		c.used -= x.size
	}
}

func (c *BodyCache) count(load, failed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if load {
		c.stats.Loads++
	}
	if failed {
		c.stats.Errors++
	}
}

// SetMax changes the bound, dropping the least recently used bodies that
// no longer fit.
func (c *BodyCache) SetMax(maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.max = maxBytes
	for c.used > c.max {
		el := c.lru.Back()
		x := el.Value.(*cached)
		c.lru.Remove(el)
		delete(c.items, x.key)
		c.used -= x.size
	}
}

// Stats returns the cache's bound, use and counts.
func (c *BodyCache) Stats() BodyStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats
	s.MaxBytes, s.UsedBytes, s.Bodies = c.max, c.used, len(c.items)
	return s
}

// releaseBodies drops from memory the bodies of the entries added since
// the last call whose bodies the source can now give back, moving each
// into the cache. A body the source cannot give back yet (its record is
// not written) stays until a later call.
func (p *Pool) releaseBodies() {
	if p.cfg.Source == nil || len(p.held) == 0 {
		return
	}
	kept := p.held[:0]
	for _, n := range p.held {
		if p.nodes[n.hash] != n || n.entry == nil {
			continue // pruned, or already released
		}
		if !p.cfg.Source.Has(n.hash) {
			kept = append(kept, n)
			continue
		}
		if p.cfg.Bodies != nil {
			p.cfg.Bodies.put(bodyKey{p.id, n.hash}, n.entry)
		}
		n.entry = nil
	}
	clear(p.held[len(kept):])
	p.held = kept
}

// ReleaseBodies drops from memory every body the source can give back.
// The pool also does this on each AddEntry; a node calls it after writing
// what it took in, such as at the end of start-up.
func (p *Pool) ReleaseBodies() { p.releaseBodies() }

// body returns n's body: from memory, the cache, or the source. A failed
// read returns false; it is counted, and decides nothing, since no check
// of an entry reads a body that is not in memory.
func (p *Pool) body(n *node) (*entry.Entry, bool) {
	if n.entry != nil {
		return n.entry, true
	}
	k := bodyKey{p.id, n.hash}
	if c := p.cfg.Bodies; c != nil {
		if e, ok := c.get(k); ok {
			return e, true
		}
	}
	if p.cfg.Source == nil {
		return nil, false
	}
	e, err := p.cfg.Source.Load(n.hash)
	if c := p.cfg.Bodies; c != nil {
		c.count(true, err != nil)
		if err == nil {
			c.put(k, e)
		}
	}
	if err != nil {
		return nil, false
	}
	return e, true
}

// parentHash returns the hash of n's parent: an entry hash, or the
// pool_id for the pool's first entries.
func (p *Pool) parentHash(n *node) hash.Hash {
	if n.parent == nil {
		return p.id
	}
	return n.parent.hash
}

// InMemory counts the held entries whose bodies are in the pool itself,
// not dropped to the source (for tests and status).
func (p *Pool) InMemory() int {
	c := 0
	for _, n := range p.nodes {
		if n.entry != nil {
			c++
		}
	}
	return c
}
