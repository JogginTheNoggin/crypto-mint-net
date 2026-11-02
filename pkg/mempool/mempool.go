// Package mempool holds a pool's pending transactions: valid transactions a
// node has accepted but not yet seen in an entry of its selected ledger.
//
// The rules follow the design's forwarding rules for pending transactions:
//
//   - A pending transaction is valid against the selected tip.
//   - Conflicts are first come, first served: a spend whose input a pending
//     transaction already spends, or a claim of a mint with a pending claim,
//     is refused and not forwarded.
//   - A transaction that spends an output of another pending transaction is
//     refused, since no entry may hold both. The one exception is carried
//     transactions after a switch: a chain of carried payments must come back
//     one entry per link, so dependent carried transactions wait until the
//     transaction they depend on is in the selected ledger.
//
// Pool is not safe for concurrent use; the engine serializes access.
package mempool

import (
	"errors"
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// DefaultLimit is the provisional default for how many transactions a pool
// holds. The design leaves the limit to node policy.
const DefaultLimit = 10000

var (
	// ErrKnown: the transaction is already pending. Not an offence.
	ErrKnown = errors.New("mempool: already pending")
	// ErrConflict: an earlier pending transaction spends the same portion
	// or claims the same mint.
	ErrConflict = errors.New("mempool: conflicts with a pending transaction")
	// ErrDependsOnPending: an input is an output of a pending transaction.
	ErrDependsOnPending = errors.New("mempool: spends an output of a pending transaction")
	// ErrInSelected: the transaction is already in the selected ledger.
	ErrInSelected = errors.New("mempool: already in the selected ledger")
	// ErrFull: the pool is at its limit.
	ErrFull = errors.New("mempool: full")
	// ErrInvalid wraps the reason a transaction is not valid at the tip.
	ErrInvalid = errors.New("mempool: invalid at the selected tip")
)

// Ledger is what the mempool needs from the selected ledger.
type Ledger interface {
	tx.View
	// Locate reports whether a tid is in the selected ledger.
	Locate(tid hash.Hash) (hash.Hash, bool)
}

type pending struct {
	tid  hash.Hash
	tx   *tx.Tx
	seq  uint64
	wait bool // a carried transaction waiting for its parent to be selected
}

// Pool is the pending transactions of one ledger pool.
type Pool struct {
	s       *series.Series
	limit   int
	byTID   map[hash.Hash]*pending
	spentBy map[tx.Input]hash.Hash
	claims  map[uint64]hash.Hash
	seq     uint64
}

// New returns an empty mempool. limit 0 means DefaultLimit.
func New(s *series.Series, limit int) *Pool {
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &Pool{s: s, limit: limit, byTID: map[hash.Hash]*pending{}, spentBy: map[tx.Input]hash.Hash{}, claims: map[uint64]hash.Hash{}}
}

// Len returns how many transactions are held, waiting ones included.
func (p *Pool) Len() int { return len(p.byTID) }

// Has reports whether a tid is held.
func (p *Pool) Has(tid hash.Hash) bool { _, ok := p.byTID[tid]; return ok }

// Get returns a held transaction.
func (p *Pool) Get(tid hash.Hash) (*tx.Tx, bool) {
	e, ok := p.byTID[tid]
	if !ok {
		return nil, false
	}
	return e.tx, true
}

func (p *Pool) conflicts(t *tx.Tx) bool {
	if t.IsClaim() {
		_, ok := p.claims[t.Mint.Serial]
		return ok
	}
	for _, in := range t.Inputs {
		if _, ok := p.spentBy[in]; ok {
			return true
		}
	}
	return false
}

func (p *Pool) dependsOnPending(t *tx.Tx) bool {
	for _, in := range t.Inputs {
		if _, ok := p.byTID[in.TID]; ok {
			return true
		}
	}
	return false
}

func (p *Pool) insert(tid hash.Hash, t *tx.Tx, wait bool) {
	p.seq++
	p.byTID[tid] = &pending{tid: tid, tx: t, seq: p.seq, wait: wait}
	if t.IsClaim() {
		p.claims[t.Mint.Serial] = tid
	}
	for _, in := range t.Inputs {
		p.spentBy[in] = tid
	}
}

// Remove drops a transaction.
func (p *Pool) Remove(tid hash.Hash) {
	e, ok := p.byTID[tid]
	if !ok {
		return
	}
	delete(p.byTID, tid)
	if e.tx.IsClaim() && p.claims[e.tx.Mint.Serial] == tid {
		delete(p.claims, e.tx.Mint.Serial)
	}
	for _, in := range e.tx.Inputs {
		if p.spentBy[in] == tid {
			delete(p.spentBy, in)
		}
	}
}

// Add checks a transaction against the selected tip and the pending set and
// keeps it if it is acceptable. A nil error means it is now pending and
// should be forwarded. tid must be t.TID of the series hash.
func (p *Pool) Add(l Ledger, t *tx.Tx, tid hash.Hash) error {
	if _, ok := p.byTID[tid]; ok {
		return ErrKnown
	}
	if _, ok := l.Locate(tid); ok {
		return ErrInSelected
	}
	if p.conflicts(t) {
		return ErrConflict
	}
	if p.dependsOnPending(t) {
		return ErrDependsOnPending
	}
	if len(p.byTID) >= p.limit {
		return ErrFull
	}
	if err := tx.Check(p.s, l, t, tid); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	p.insert(tid, t, false)
	return nil
}

// Item is a pending transaction and its tid.
type Item struct {
	TID hash.Hash
	Tx  *tx.Tx
}

// AddCarried takes back the carried transactions of a switch, in the order
// they had in the old ledger. Each that is valid at the tip becomes pending;
// one that depends on an earlier carried transaction waits for it. Carried
// transactions take precedence over pending ones they conflict with, since
// they were already in a ledger.
func (p *Pool) AddCarried(l Ledger, carried []Item) {
	for _, c := range carried {
		if _, ok := p.byTID[c.TID]; ok {
			continue
		}
		p.evictConflicts(c.Tx)
		wait := p.dependsOnPending(c.Tx)
		if !wait {
			if err := tx.CheckAgainst(p.s, l, c.Tx, c.TID); err != nil {
				continue
			}
		}
		p.insert(c.TID, c.Tx, wait)
	}
}

func (p *Pool) evictConflicts(t *tx.Tx) {
	if t.IsClaim() {
		if other, ok := p.claims[t.Mint.Serial]; ok {
			p.removeWithDependents(other)
		}
		return
	}
	for _, in := range t.Inputs {
		if other, ok := p.spentBy[in]; ok {
			p.removeWithDependents(other)
		}
	}
}

func (p *Pool) removeWithDependents(tid hash.Hash) {
	p.Remove(tid)
	for other, e := range p.byTID {
		for _, in := range e.tx.Inputs {
			if in.TID == tid {
				p.removeWithDependents(other)
				break
			}
		}
	}
}

// Revalidate re-checks every held transaction after the selected ledger
// changed. Transactions now in the selected ledger, and those no longer
// valid at the tip, are dropped. Waiting transactions whose inputs now exist
// become pending. It returns the tids dropped as invalid.
func (p *Pool) Revalidate(l Ledger) (dropped []hash.Hash) {
	// Oldest first, so first come, first served survives revalidation and
	// a waiting transaction is checked after what it depends on.
	order := make([]*pending, 0, len(p.byTID))
	for _, e := range p.byTID {
		order = append(order, e)
	}
	sortBySeq(order)
	for _, e := range order {
		if _, ok := p.byTID[e.tid]; !ok {
			continue
		}
		if _, in := l.Locate(e.tid); in {
			p.Remove(e.tid)
			continue
		}
		if p.dependsOnPending(e.tx) {
			if !e.wait {
				// It became dependent because a parent it spends was
				// re-added as carried; let it wait.
				e.wait = true
			}
			continue
		}
		if err := tx.CheckAgainst(p.s, l, e.tx, e.tid); err != nil {
			p.Remove(e.tid)
			dropped = append(dropped, e.tid)
			continue
		}
		e.wait = false
	}
	return dropped
}

// Ready returns the pending transactions that can go into an entry on the
// selected tip, oldest first: valid at the tip, not waiting, and, by the
// pool's own rules, conflict-free and independent of one another.
func (p *Pool) Ready() []Item {
	order := make([]*pending, 0, len(p.byTID))
	for _, e := range p.byTID {
		if !e.wait {
			order = append(order, e)
		}
	}
	sortBySeq(order)
	out := make([]Item, len(order))
	for i, e := range order {
		out[i] = Item{TID: e.tid, Tx: e.tx}
	}
	return out
}

// Waiting returns how many carried transactions are waiting on another.
func (p *Pool) Waiting() int {
	n := 0
	for _, e := range p.byTID {
		if e.wait {
			n++
		}
	}
	return n
}

func sortBySeq(xs []*pending) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].seq < xs[j].seq })
}
