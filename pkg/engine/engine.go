// Package engine is the transport-independent core of a full node.
//
// The engine knows which series exist (from outside the network) and which
// pools the node follows. For every followed pool it keeps the tree of
// entries with the selected ledger (package ledger) and the pending
// transactions (package mempool), and a node-wide seen list. It takes in
// transactions, entries and layers, checks them, and says for each whether it
// was kept and must be forwarded. It also builds entries and adds layers for
// the node's own owner.
//
// The engine never touches the network. A transport (or a test, or a
// simulation) feeds it items and acts on its answers, which keeps the
// consensus-relevant logic in one place that is easy to test.
//
// The forwarding rules it applies:
//   - only items for followed pools are taken in;
//   - an item already on the seen list, or already held, is dropped;
//   - an item is listed as seen only once it has passed every check;
//   - data the node asked for (Requested) or read back from its own store
//     (Stored) is kept but never forwarded;
//   - entries and layers are forwarded whatever branch they are on.
//
// An Engine is safe for concurrent use.
package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/mempool"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Errors returned by the engine, beyond those of the ledger and mempool,
// which it passes through wrapped.
var (
	ErrUnknownSeries   = errors.New("engine: series not known")
	ErrNotFollowed     = errors.New("engine: pool not followed")
	ErrSeen            = errors.New("engine: already seen")
	ErrNothingPending  = errors.New("engine: no pending transactions to build an entry from")
	ErrNoStrongerLayer = errors.New("engine: no layer found")
	// ErrAlreadyBuilt: the entry Build would make is already held, so there
	// is nothing new to send. Its transactions stay pending until it, or an
	// entry after it, earns credit and is selected.
	ErrAlreadyBuilt = errors.New("engine: entry already built")
)

// Origin says how an item reached the node. It decides forwarding.
type Origin int

const (
	// Pushed: forwarded by a peer or sent by an unlisted node such as a
	// wallet. Kept items are forwarded.
	Pushed Origin = iota
	// Requested: received because the node asked for it. Never forwarded.
	Requested
	// Local: made by this node. Kept items are forwarded.
	Local
	// Stored: read back from this node's own store on start-up. Like
	// Requested, it skips the seen list and is never forwarded.
	Stored
)

// forwards reports whether kept items of this origin are forwarded.
func (o Origin) forwards() bool { return o == Pushed || o == Local }

// checksSeen reports whether entries and layers of this origin are dropped
// when already on the seen list. Data the node asked for, or reads back
// from its store, may have been seen and then dropped.
func (o Origin) checksSeen() bool { return o == Pushed || o == Local }

// String names the origin.
func (o Origin) String() string {
	switch o {
	case Pushed:
		return "pushed"
	case Requested:
		return "requested"
	case Local:
		return "local"
	case Stored:
		return "stored"
	}
	return fmt.Sprintf("origin(%d)", int(o))
}

// PoolKey names one pool of one series.
type PoolKey struct {
	Series hash.Hash
	Index  uint64
}

// String returns "<series short>#<index>".
func (k PoolKey) String() string { return fmt.Sprintf("%s#%d", k.Series.Short(), k.Index) }

// Config holds the engine's node policy settings.
type Config struct {
	// Ledger is applied to every pool.
	Ledger ledger.Config
	// MaxPending is the pending transaction limit per pool (0: default).
	MaxPending int
	// SeenSize is the size of the seen list (0: default).
	SeenSize int
	// Source, if set, gives each pool the BodySource its ledger drops
	// entry bodies to (D172); Ledger.Bodies is the cache they are read
	// back through. It is called for every pool state made, including
	// ones only read and not kept, so it should be cheap.
	Source func(PoolKey) ledger.BodySource
}

// DefaultConfig returns the provisional defaults.
func DefaultConfig() Config {
	return Config{Ledger: ledger.Config{K: ledger.DefaultK}}
}

type follow struct {
	all   bool
	pools map[uint64]bool
	// except, with all, are the pools left out of the series (RED-050).
	except map[uint64]bool
}

// has reports whether the rule follows pool k of a series of n pools.
func (f *follow) has(k, n uint64) bool {
	return k < n && (f.pools[k] || (f.all && !f.except[k]))
}

type poolState struct {
	s       *series.Series
	ledger  *ledger.Pool
	pending *mempool.Pool
}

// view adapts a ledger pool to mempool.Ledger.
type view struct{ p *ledger.Pool }

func (v view) Unspent(in tx.Input) (tx.Portion, bool) { return v.p.State().Unspent(in) }
func (v view) Claimed(serial uint64) bool             { return v.p.State().Claimed(serial) }
func (v view) Locate(tid hash.Hash) (hash.Hash, bool) { return v.p.Locate(tid) }

// Engine is the core of a full node.
type Engine struct {
	mu      sync.Mutex
	cfg     Config
	series  map[hash.Hash]*series.Series
	follows map[hash.Hash]*follow
	pools   map[PoolKey]*poolState
	seen    *Seen
	// failed is set, under mu, when a panic unwound out of a method that
	// changes a pool (D174); stopped is closed then.
	failed  error
	stopped chan struct{}
}

// ErrFailed is returned by every method that changes a pool once one of
// them panicked part way: the pool may be half changed, so the engine
// takes nothing more and the node must stop and rebuild from its stores,
// as it did when such a panic ended the process (Black's Round 28
// condition, D174).
var ErrFailed = errors.New("engine: stopped after an internal error while changing a pool; restart the node to rebuild it from its stores")

// New returns an engine that knows no series and follows nothing.
func New(cfg Config) *Engine {
	return &Engine{
		cfg:     cfg,
		series:  map[hash.Hash]*series.Series{},
		follows: map[hash.Hash]*follow{},
		pools:   map[PoolKey]*poolState{},
		seen:    NewSeen(cfg.SeenSize),
		stopped: make(chan struct{}),
	}
}

// Failed is closed once a method that changes a pool panicked part way
// (ErrFailed), or a read of a pool under the lock panicked, such as a
// ledger.ReadFault on a cold entry (RED-085, D177). A node stops then,
// without compacting its stores.
func (e *Engine) Failed() <-chan struct{} { return e.stopped }

// failLocked fails the engine with panic value v, once. e.mu is held.
func (e *Engine) failLocked(v any) {
	if e.failed == nil {
		e.failed = fmt.Errorf("%w (%v)", ErrFailed, v)
		close(e.stopped)
	}
}

// under runs fn holding e.mu, and always releases it. A panic in fn (a
// ledger.ReadFault on a cold entry) fails the engine as handling does,
// releases the lock, and goes on up, so no caller that recovers it, such
// as the p2p work loop or the HTTP server, leaves the engine locked with
// Failed still open (RED-085, D177). With the engine failed, fn does not
// run and under returns false.
func (e *Engine) under(fn func()) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return false
	}
	defer func() {
		if v := recover(); v != nil {
			e.failLocked(v)
			panic(v)
		}
	}()
	fn()
	return true
}

// Err returns ErrFailed, with the panic, once the engine has failed.
func (e *Engine) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.failed
}

// AddSeries makes a series known. Series information comes from outside
// the network, never from peers.
func (e *Engine) AddSeries(s *series.Series) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.series[s.ID()] = s
}

// Series returns a known series.
func (e *Engine) Series(id hash.Hash) (*series.Series, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.series[id]
	return s, ok
}

// FollowSeries follows every pool of a known series.
func (e *Engine) FollowSeries(id hash.Hash) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.series[id]; !ok {
		return ErrUnknownSeries
	}
	f := e.followRule(id)
	f.all, f.except = true, nil
	return nil
}

// FollowsWhole reports whether a series is followed whole, perhaps but
// for some pools (FollowSeriesExcept).
func (e *Engine) FollowsWhole(id hash.Hash) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	f, ok := e.follows[id]
	return ok && f.all
}

// FollowSeriesExcept follows every pool of a known series but those in
// except. Followed with the rest, a pool is followed if any rule follows
// it: a whole follow without exceptions keeps every pool, and a pool
// followed by its index stays followed (RED-050, D154).
func (e *Engine) FollowSeriesExcept(id hash.Hash, except []uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.series[id]
	if !ok {
		return ErrUnknownSeries
	}
	f := e.followRule(id)
	ex := map[uint64]bool{}
	for _, k := range except {
		if k >= s.PoolCount() {
			return fmt.Errorf("engine: pool %d out of range", k)
		}
		if !f.pools[k] && (!f.all || f.except[k]) {
			ex[k] = true
		}
	}
	f.all, f.except = true, ex
	return nil
}

// FollowPool follows one pool of a known series.
func (e *Engine) FollowPool(id hash.Hash, index uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.series[id]
	if !ok {
		return ErrUnknownSeries
	}
	if index >= s.PoolCount() {
		return fmt.Errorf("engine: pool %d out of range", index)
	}
	f := e.followRule(id)
	f.pools[index] = true
	delete(f.except, index)
	return nil
}

// FollowMint follows one mint, which means following its whole pool, since
// the mint cannot be checked without it.
func (e *Engine) FollowMint(id hash.Hash, serial uint64) error {
	e.mu.Lock()
	s, ok := e.series[id]
	e.mu.Unlock()
	if !ok {
		return ErrUnknownSeries
	}
	if serial >= s.Mints() {
		return fmt.Errorf("engine: serial %d out of range", serial)
	}
	return e.FollowPool(id, s.PoolOf(serial))
}

func (e *Engine) followRule(id hash.Hash) *follow {
	f, ok := e.follows[id]
	if !ok {
		f = &follow{pools: map[uint64]bool{}}
		e.follows[id] = f
	}
	return f
}

// Follows reports whether the node follows a pool.
func (e *Engine) Follows(k PoolKey) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.followsLocked(k)
}

func (e *Engine) followsLocked(k PoolKey) bool {
	f, ok := e.follows[k.Series]
	if !ok {
		return false
	}
	return f.has(k.Index, e.series[k.Series].PoolCount())
}

// pool returns the state of a followed pool, creating and keeping it on
// first use.
func (e *Engine) pool(k PoolKey) (*poolState, error) {
	p, created, err := e.poolFor(k)
	if created {
		e.pools[k] = p
	}
	return p, err
}

// poolFor returns the state of a followed pool and whether it is new: a
// new state is not kept yet. Every pool's state starts the same (its
// series and index decide it), so a state made and dropped again is the
// state made the next time (RED-049, D154).
func (e *Engine) poolFor(k PoolKey) (*poolState, bool, error) {
	if p, ok := e.pools[k]; ok {
		return p, false, nil
	}
	s, ok := e.series[k.Series]
	if !ok {
		return nil, false, ErrUnknownSeries
	}
	if !e.followsLocked(k) {
		return nil, false, ErrNotFollowed
	}
	lc := e.cfg.Ledger
	if e.cfg.Source != nil {
		lc.Source = e.cfg.Source(k)
	}
	return &poolState{
		s:       s,
		ledger:  ledger.New(s, k.Index, lc),
		pending: mempool.New(s, e.cfg.MaxPending),
	}, true, nil
}

// peek returns the state of a followed pool for reading: a pool with no
// state yet is read from a fresh one, which is not kept, so a query or a
// check of a pool never in use costs it nothing (RED-049).
func (e *Engine) peek(k PoolKey) (*poolState, error) {
	p, _, err := e.poolFor(k)
	return p, err
}

// unstated reports whether pool k is followed but has no state, and then
// its pool id: what an empty ledger reads as its tip. Status and Tip
// answer such a pool from it rather than from a fresh state (peek), which
// cost about 1.7 KB and 1 µs a pool on every call, so a status, metrics
// scrape or query of a series of up to ListAll pools followed whole made
// about 100 MiB of garbage each time (ORANGE-030). The answer is the one a
// fresh state gives (TestUnstatedReadsAsFresh). An error is peek's: the
// series is unknown or the pool not followed.
func (e *Engine) unstated(k PoolKey) (hash.Hash, bool, error) {
	if _, ok := e.pools[k]; ok {
		return hash.Hash{}, false, nil
	}
	s, ok := e.series[k.Series]
	if !ok {
		return hash.Hash{}, false, ErrUnknownSeries
	}
	if !e.followsLocked(k) {
		return hash.Hash{}, false, ErrNotFollowed
	}
	return s.PoolID(k.Index), true, nil
}

// handling returns the state of pool k for an item. A new state is kept
// only if the item left something in it (an entry, a pending or waiting
// transaction) by the time done is called, so an item that is refused
// leaves no state behind (RED-049, D154). What is accepted does not
// change: the item meets the same state either way.
func (e *Engine) handling(k PoolKey) (p *poolState, done func(), err error) {
	if e.failed != nil {
		return nil, func() {}, e.failed
	}
	p, created, err := e.poolFor(k)
	if err != nil {
		return p, func() {}, err
	}
	if created {
		e.pools[k] = p
	}
	// done runs deferred under e.mu, after the change. A panic during the
	// change fails the engine before it unwinds further (D174): what the
	// pool holds may be half changed, so nothing may build on it.
	return p, func() {
		if v := recover(); v != nil {
			e.failLocked(v)
			panic(v)
		}
		if created && p.ledger.Len() == 0 && p.pending.Len() == 0 && p.pending.Waiting() == 0 {
			delete(e.pools, k)
		}
	}, nil
}

// FollowedPools returns every pool the node follows, by series id and then
// pool index.
func (e *Engine) FollowedPools() []PoolKey {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []PoolKey
	for id, f := range e.follows {
		s := e.series[id]
		for k := uint64(0); k < s.PoolCount(); k++ {
			if f.has(k, s.PoolCount()) {
				out = append(out, PoolKey{Series: id, Index: k})
			}
		}
	}
	sortKeys(out)
	return out
}

// ListAll is the number of pools below which a series followed whole has
// every pool listed by ListedPools; from it on, only the pools the node
// holds state for are (RED-046, D152). It limits nothing the node
// follows or accepts: a pool not listed is still followed, and its state
// is made when its first item or request arrives.
const ListAll = 1 << 16

// ListedPools returns the pools to list, query or report: every pool
// followed by its index, every pool of a series followed whole with
// fewer than ListAll pools, and of a larger one the pools the node holds
// state for. Reporting and queries use it, so a series of very many
// pools followed whole costs only the pools in use. Sorted by series id
// and then pool index.
func (e *Engine) ListedPools() []PoolKey {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []PoolKey
	for id, f := range e.follows {
		s := e.series[id]
		switch {
		case f.all && s.PoolCount() < ListAll:
			for k := uint64(0); k < s.PoolCount(); k++ {
				if f.has(k, s.PoolCount()) {
					out = append(out, PoolKey{Series: id, Index: k})
				}
			}
		default:
			for k := range f.pools {
				if k < s.PoolCount() {
					out = append(out, PoolKey{Series: id, Index: k})
				}
			}
			if f.all {
				for k := range e.pools {
					if k.Series == id && !f.pools[k.Index] && f.has(k.Index, s.PoolCount()) {
						out = append(out, k)
					}
				}
			}
		}
	}
	sortKeys(out)
	return out
}

// EachFollowedPoolOf calls fn for every pool of series id the node
// follows, in no set order, without listing them first. fn must not call
// the engine.
func (e *Engine) EachFollowedPoolOf(id hash.Hash, fn func(PoolKey)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f, ok := e.follows[id]
	if !ok {
		return
	}
	s := e.series[id]
	if f.all {
		for k := uint64(0); k < s.PoolCount(); k++ {
			if f.has(k, s.PoolCount()) {
				fn(PoolKey{Series: id, Index: k})
			}
		}
		return
	}
	for k := range f.pools {
		if k < s.PoolCount() {
			fn(PoolKey{Series: id, Index: k})
		}
	}
}

// FollowedCount returns how many pools the node follows, without listing
// them.
func (e *Engine) FollowedCount() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var n uint64
	for id, f := range e.follows {
		s := e.series[id]
		if f.all {
			n += s.PoolCount() - uint64(len(f.except))
			continue
		}
		for k := range f.pools {
			if k < s.PoolCount() {
				n++
			}
		}
	}
	return n
}

// FollowedSeries returns the ids of the series the node follows a pool
// of, sorted.
func (e *Engine) FollowedSeries() []hash.Hash {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []hash.Hash
	for id, f := range e.follows {
		if f.all || len(f.pools) > 0 {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) < 0 })
	return out
}

func sortKeys(out []PoolKey) {
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Series.Compare(out[j].Series); c != 0 {
			return c < 0
		}
		return out[i].Index < out[j].Index
	})
}

// KeyOf returns the pool key of a transaction.
func (e *Engine) KeyOf(t *tx.Tx) (PoolKey, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.series[t.Mint.Series]
	if !ok {
		return PoolKey{}, ErrUnknownSeries
	}
	// A wrong part or a serial no part holds is invalid; a part this node
	// has not seen released waits (tx.ErrWaiting). Neither is kept on the
	// seen list, which HandleTx adds to only once a transaction is taken.
	if err := tx.CheckMint(s, t.Mint); err != nil {
		return PoolKey{}, err
	}
	return PoolKey{Series: s.ID(), Index: s.PoolOf(t.Mint.Serial)}, nil
}

// PreverifyTx verifies the witness signatures of t ahead of HandleTx,
// holding the engine's lock only for the checks HandleTx runs before any
// signature (a followed pool, not seen, not pending or in the selected
// ledger, no conflict, every input unspent at the tip, the amounts) and to
// find the address each witness must satisfy. A caller that serializes
// items under its own lock calls it first, so items from several peers are
// checked on several cores and the checks under the lock find them
// remembered (ORANGE-004). A transaction HandleTx would refuse before its
// signatures costs none here, and one with a bad witness costs one
// (RED-009, decision D107). It changes no result.
func (e *Engine) PreverifyTx(t *tx.Tx) {
	k, err := e.KeyOf(t)
	if err != nil {
		return
	}
	var p *poolState
	var tid hash.Hash
	var owners []hash.Hash
	e.under(func() {
		if p, err = e.peek(k); err != nil {
			return
		}
		tid = t.TID(p.s.Hash())
		if !e.seen.Has(tid) {
			owners, err = p.pending.Owners(view{p.ledger}, t, tid)
		}
	})
	if owners != nil && err == nil {
		tx.Preverify(p.s, t, tid, owners)
	}
}

// MaxCountedCost is the most EntryCost counts.
const MaxCountedCost = 1 << 20

// EntryCost is how many entries HandleEntry would revert and apply to
// check en against the state after its parent (ledger.Pool.ViewCost), at
// most about MaxCountedCost; 0 when pool k or the parent is not held. The
// network charges it to the sender (RED-011, decision D111).
func (e *Engine) EntryCost(k PoolKey, en *entry.Entry) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.peek(k)
	if err != nil {
		return 0
	}
	return p.ledger.ViewCost(en, MaxCountedCost)
}

// PreverifyEntry is PreverifyTx for the transactions of an entry of pool
// k. It verifies signatures only for an entry that passes every check
// HandleEntry runs before a signature: not held or seen, its shape and
// strength, its parent held, and every transaction valid but for its
// witnesses against the state after that parent. It stops at the first
// transaction with a bad witness (RED-009, decisions D105, D107). It does
// nothing for a parent far from the selected tip (ledger.ErrDeepParent):
// that state costs a revert, which HandleEntry pays once (RED-011, D111).
func (e *Engine) PreverifyEntry(k PoolKey, en *entry.Entry, origin Origin) {
	var p *poolState
	var items []entry.Item
	var owners [][]hash.Hash
	err := ErrFailed
	e.under(func() {
		if p, err = e.peek(k); err != nil {
			return
		}
		if !(origin.checksSeen() && e.seen.Has(en.Hash(p.s))) {
			items, owners, err = p.ledger.EntryOwners(en)
		}
	})
	if err != nil {
		return
	}
	for i, it := range items {
		if tx.Preverify(p.s, it.Tx, it.TID, owners[i]) != nil {
			return
		}
	}
}

// PreverifyPage is PreverifyEntry for a run of entries, each the parent
// of the next, as a chase receives them in one reply. It finds every
// entry's owners under the lock, against the state after the entries
// before it (Pool.PageOwners), then checks all their witnesses outside
// the lock on every core, so taking the entries in order afterwards finds
// each witness verified. Entries it cannot preview, and every entry after
// them, are left to PreverifyEntry. Like PreverifyEntry it only fills the
// list of verified witnesses and never changes what any check returns.
// Witnesses are only checked once their addresses match (RED-009); the
// workers stop at the first that fails, so a bad one costs at most about
// one check per core (ORANGE-014, ORANGE-020).
func (e *Engine) PreverifyPage(k PoolKey, es []*entry.Entry) {
	if len(es) < 2 {
		return
	}
	var p *poolState
	var items [][]entry.Item
	var owners [][][]hash.Hash
	err := ErrFailed
	e.under(func() {
		if p, err = e.peek(k); err == nil {
			items, owners = p.ledger.PageOwners(es)
		}
	})
	if err != nil {
		return
	}
	type job struct {
		it     entry.Item
		owners []hash.Hash
	}
	var jobs []job
	for i := range items {
		for j, it := range items[i] {
			jobs = append(jobs, job{it, owners[i][j]})
		}
	}
	workers := min(runtime.GOMAXPROCS(0), len(jobs))
	if workers < 2 {
		for _, j := range jobs {
			if tx.Preverify(p.s, j.it.Tx, j.it.TID, j.owners) != nil {
				return
			}
		}
		return
	}
	var next atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !failed.Load() {
				i := int(next.Add(1) - 1)
				if i >= len(jobs) {
					return
				}
				if tx.Preverify(p.s, jobs[i].it.Tx, jobs[i].it.TID, jobs[i].owners) != nil {
					failed.Store(true)
				}
			}
		}()
	}
	wg.Wait()
}

// MaxWarm is the most witnesses WarmStored checks in one call, well under
// the series' list of verified witnesses (policy.DefaultVerifiedSize), so
// what it remembers is still there when the records are taken in.
const MaxWarm = policy.DefaultVerifiedSize / 2

// WarmStored checks the witnesses of transactions read back from this
// node's own store, on every core, and remembers each that passes, so
// replaying them in order (HandleTx, HandleEntry) finds them verified. It
// needs no owners, so it works ahead of the state the records build; each
// witness's address is still checked when the record is replayed, and a
// witness that fails is simply not remembered, so a tampered store is
// refused as before. It checks at most MaxWarm witnesses and never changes
// what any check returns (ORANGE-021).
func (e *Engine) WarmStored(txs []*tx.Tx) {
	type job struct {
		s   *series.Series
		w   policy.Witness
		tid hash.Hash
	}
	var jobs []job
	seen := map[hash.Hash]bool{} // a stored transaction and the entry holding it
	e.mu.Lock()
	for _, t := range txs {
		s, ok := e.series[t.Mint.Series]
		if !ok || tx.CheckMint(s, t.Mint) != nil {
			continue
		}
		if _, err := e.peek(PoolKey{Series: s.ID(), Index: s.PoolOf(t.Mint.Serial)}); err != nil {
			continue
		}
		tid := t.TID(s.Hash())
		if seen[tid] {
			continue
		}
		seen[tid] = true
		for _, w := range t.Witnesses {
			if len(jobs) == MaxWarm {
				break
			}
			jobs = append(jobs, job{s, w, tid})
		}
	}
	e.mu.Unlock()
	workers := min(runtime.GOMAXPROCS(0), len(jobs))
	if workers < 2 {
		return // on one core, replaying checks them anyway
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(jobs) {
					return
				}
				jobs[i].s.WarmWitness(jobs[i].w, jobs[i].tid)
			}
		}()
	}
	wg.Wait()
}

// TxResult reports what HandleTx did.
type TxResult struct {
	Key     PoolKey
	TID     hash.Hash
	Forward bool
}

// HandleTx takes in a transaction. A nil error means it is now pending.
func (e *Engine) HandleTx(t *tx.Tx, origin Origin) (TxResult, error) {
	k, err := e.KeyOf(t)
	if err != nil {
		return TxResult{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	p, done, err := e.handling(k)
	defer done()
	if err != nil {
		return TxResult{Key: k}, err
	}
	tid := t.TID(p.s.Hash())
	res := TxResult{Key: k, TID: tid}
	if e.seen.Has(tid) {
		return res, ErrSeen
	}
	if err := p.pending.Add(view{p.ledger}, t, tid); err != nil {
		return res, err
	}
	e.seen.Add(tid)
	res.Forward = origin.forwards()
	return res, nil
}

// EntryResult reports what HandleEntry did.
type EntryResult struct {
	Key     PoolKey
	Hash    hash.Hash
	Forward bool
	// Switch is set when the selected ledger changed.
	Switch *ledger.Switch
	// Pruned lists entries dropped to stay within K.
	Pruned []hash.Hash
	// Dropped lists pending transactions dropped as no longer valid.
	Dropped []hash.Hash
}

// HandleEntry takes in an entry of pool k. It returns ledger.ErrUnknownParent
// when the parent is not held (the entry is not kept), and an error wrapping
// ledger.ErrInvalidEntry when the entry breaks a rule.
func (e *Engine) HandleEntry(k PoolKey, en *entry.Entry, origin Origin) (EntryResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, done, err := e.handling(k)
	defer done()
	if err != nil {
		return EntryResult{Key: k}, err
	}
	h := en.Hash(p.s)
	res := EntryResult{Key: k, Hash: h}
	// Requested data skips the seen list, so a node can fetch again an
	// entry it dropped; it is still never forwarded.
	if p.ledger.Has(h) || (origin.checksSeen() && e.seen.Has(h)) {
		return res, ErrSeen
	}
	ar, err := p.ledger.AddEntry(en)
	if err != nil {
		return res, err
	}
	e.seen.Add(h)
	res.Forward = origin.forwards()
	res.Pruned = ar.Pruned
	res.Switch = ar.Switch
	res.Dropped = e.afterSwitch(p, ar.Switch)
	return res, nil
}

// LayerResult reports what HandleLayer did.
type LayerResult struct {
	Key     PoolKey
	Hash    hash.Hash // the layer hash
	Credit  weight.Weight
	Forward bool
	Switch  *ledger.Switch
	Pruned  []hash.Hash
	Dropped []hash.Hash
}

// HandleLayer takes in a layer on an entry of pool k. It returns
// ledger.ErrUnknownEntry when the entry is not held: the layer is thrown out
// and not forwarded.
func (e *Engine) HandleLayer(k PoolKey, l layer.Layer, origin Origin) (LayerResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, done, err := e.handling(k)
	defer done()
	if err != nil {
		return LayerResult{Key: k}, err
	}
	lh := l.Hash(p.s.Hash())
	res := LayerResult{Key: k, Hash: lh, Credit: weight.Credit(lh.Strength())}
	if origin.checksSeen() && e.seen.Has(lh) {
		return res, ErrSeen
	}
	ar, err := p.ledger.AddLayerHashed(l, lh)
	if err != nil {
		return res, err
	}
	e.seen.Add(lh)
	res.Forward = origin.forwards()
	res.Switch = ar.Switch
	res.Pruned = ar.Pruned
	res.Dropped = e.afterSwitch(p, ar.Switch)
	return res, nil
}

// afterSwitch brings the pending transactions in line with a new selected
// ledger: carried transactions come back, and anything now included or no
// longer valid is dropped.
func (e *Engine) afterSwitch(p *poolState, sw *ledger.Switch) []hash.Hash {
	if sw == nil {
		return nil
	}
	v := view{p.ledger}
	if len(sw.Carried) > 0 {
		items := make([]mempool.Item, len(sw.Carried))
		for i, d := range sw.Carried {
			items[i] = mempool.Item{TID: d.TID, Tx: d.Tx}
		}
		p.pending.AddCarried(v, items)
	}
	return p.pending.Revalidate(v)
}

// Build makes an entry on the selected tip of pool k from its ready pending
// transactions, keeps it, and returns it for forwarding. Where the series
// sets a minimum entry strength it tries up to maxTries nonces (0: no
// limit).
func (e *Engine) Build(k PoolKey, maxTries uint64) (*entry.Entry, EntryResult, error) {
	en, err := e.BuildEntry(context.Background(), k, maxTries)
	if err != nil {
		return en, EntryResult{Key: k}, err
	}
	res, err := e.HandleEntry(k, en, Local)
	return en, res, err
}

// BuildEntry makes the entry Build would make but does not take it in.
// Only choosing the transactions and the tip holds the engine's lock: the
// search for a nonce runs without it, so a long search never stalls the
// engine, and it stops with ctx's error once ctx is done (RED-006). The
// caller hands the entry to HandleEntry; if the tip moved meanwhile it is
// kept as a branch like any other entry.
func (e *Engine) BuildEntry(ctx context.Context, k PoolKey, maxTries uint64) (*entry.Entry, error) {
	return e.BuildEntryWith(ctx, k, maxTries, Grind{})
}

// Grind says how a build or a layer searches: with which hasher, on how
// many goroutines, counting tries where. Package grind fills it from a
// device and the pool's hash (decision D109); the zero value is the CPU
// grinder for the series' hash on one goroutine.
type Grind struct {
	New     hash.NewHasher
	Workers int
	Tried   *atomic.Uint64
	// Search, when set, searches layers a range of nonces at a time
	// instead of hashing each with New (ORANGE-012).
	Search layer.Search
	// Chunk, when above 0, is how many nonces Search takes at a time
	// (a GPU's batch, D208); 0 is the CPU's.
	Chunk uint64
	// EntryWorkers, when above 0, is how many goroutines grind an entry
	// instead of Workers: a GPU searches layers, and entries are still
	// ground on the CPU (D208).
	EntryWorkers int
	// Floor, when above 0, also hands Keep the other layers found at or
	// above it, and at or above the series' minimum layer strength
	// (ORANGE-010, D118). Keep runs on the search's workers at once.
	Floor int
	Keep  func(layer.Layer, int)
	// KeepMax, when above 0, is how many of those layers the node takes
	// in after a search, strongest first (ORANGE-016).
	KeepMax int
	// KeptCount, if not nil, counts the layers besides the strongest that
	// the node took in (node.Node.WorkWith).
	KeptCount *atomic.Uint64
}

// entryWorkers is EntryWorkers, else Workers.
func (g Grind) entryWorkers() int {
	if g.EntryWorkers > 0 {
		return g.EntryWorkers
	}
	return g.Workers
}

func (g Grind) hasher(s *series.Series) hash.NewHasher {
	if g.New != nil {
		return g.New
	}
	return hash.GrinderFor(s.Params().HashID)
}

// BuildEntryWith is BuildEntry searching as g says.
func (e *Engine) BuildEntryWith(ctx context.Context, k PoolKey, maxTries uint64, g Grind) (*entry.Entry, error) {
	en, _, err := e.BuildEntryOn(ctx, k, maxTries, g, BuildOn{})
	return en, err
}

// BuildOn says where an entry is built and from what (D161). The zero
// value builds on the selected tip from the ready pending transactions.
type BuildOn struct {
	// Parent, when set, is the entry to build on: any held entry of the
	// pool, or the pool_id for a first entry, so a user can build a rival
	// branch, such as another claim of a mint the selected ledger has
	// claimed. The entry is then kept as a branch like any other, and
	// weight decides between them.
	Parent *hash.Hash
	// Txs, when set, are the transactions to build from instead of the
	// pending ones. They need not be pending: a claim of a mint the
	// selected ledger has claimed is refused by the pending pool, but is
	// valid on a parent from before that claim.
	Txs []*tx.Tx
}

// BuildEntryOn is BuildEntryWith built as o says. The transactions are
// checked against the state after the parent and those that do not fit
// are left out, with their errors in the result; with none left it is
// ErrNothingPending. The checks are AddEntry's, which takes the entry in
// after.
func (e *Engine) BuildEntryOn(ctx context.Context, k PoolKey, maxTries uint64, g Grind, o BuildOn) (*entry.Entry, []ledger.Refused, error) {
	var p *poolState
	var parent hash.Hash
	var refused []ledger.Refused
	txs := o.Txs
	err := ErrFailed
	if !e.under(func() {
		if p, err = e.pool(k); err != nil {
			return
		}
		pending := txs == nil
		if pending {
			for _, r := range p.pending.Ready() {
				txs = append(txs, r.Tx)
			}
		}
		parent, _, _ = p.ledger.Tip()
		if o.Parent != nil || o.Txs != nil {
			if o.Parent != nil {
				parent = *o.Parent
			}
			check := p.ledger.CheckOn
			if pending {
				// The pending pool checked their signatures (RED-068).
				check = p.ledger.CheckPendingOn
			}
			txs, refused, err = check(parent, txs)
		}
	}) {
		return nil, nil, e.Err()
	}
	if err != nil {
		return nil, nil, err
	}
	if len(txs) == 0 {
		return nil, refused, ErrNothingPending
	}
	s := p.s
	en, h, err := entry.BuildWith(ctx, g.hasher(s), s, parent, txs, maxTries, g.entryWorkers(), g.Tried)
	if err != nil {
		return nil, refused, err
	}
	var held bool
	e.under(func() { held = p.ledger.Has(h) })
	if held {
		return en, refused, ErrAlreadyBuilt
	}
	return en, refused, nil
}

// ClaimParent returns the parent of the entry of the selected ledger of
// pool k that holds mint serial's claim: the place to build a rival claim
// on (D161).
func (e *Engine) ClaimParent(k PoolKey, serial uint64) (hash.Hash, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.pool(k)
	if err != nil {
		return hash.Zero, err
	}
	eh, _, err := p.ledger.ClaimEntry(serial)
	if err != nil {
		return hash.Zero, err
	}
	parent, _ := p.ledger.ParentOf(eh)
	return parent, nil
}

// PoolID returns the pool_id of pool k: the parent of its first entry.
func (e *Engine) PoolID(k PoolKey) (hash.Hash, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.pool(k)
	if err != nil {
		return hash.Zero, err
	}
	return p.ledger.ID(), nil
}

// Work tries tries nonces on an entry hash of pool k, starting at start,
// and adds the strongest layer found.
func (e *Engine) Work(k PoolKey, entryHash hash.Hash, start, tries uint64) (layer.Layer, LayerResult, error) {
	l, err := e.GrindLayer(k, entryHash, start, tries)
	if err != nil {
		return l, LayerResult{Key: k}, err
	}
	res, err := e.HandleLayer(k, l, Local)
	return l, res, err
}

// GrindLayer finds the layer Work would add but does not take it in. The
// grinding runs without the engine's lock.
func (e *Engine) GrindLayer(k PoolKey, entryHash hash.Hash, start, tries uint64) (layer.Layer, error) {
	if tries == 0 {
		return layer.Layer{}, ErrNoStrongerLayer
	}
	return e.GrindLayerWith(context.Background(), k, entryHash, start, tries, Grind{})
}

// GrindLayerWith is GrindLayer searching as g says. tries 0 grinds until
// ctx is done. When ctx ends the search early, the strongest layer found
// so far is returned without error, so a search bounded by time (a
// deadline on ctx) or stopped by its user keeps its work; only when no
// nonce was tried is it ErrNoStrongerLayer.
func (e *Engine) GrindLayerWith(ctx context.Context, k PoolKey, entryHash hash.Hash, start, tries uint64, g Grind) (layer.Layer, error) {
	e.mu.Lock()
	p, err := e.pool(k)
	if err != nil {
		e.mu.Unlock()
		return layer.Layer{}, err
	}
	s := p.s
	held := p.ledger.Has(entryHash)
	e.mu.Unlock()
	if !held {
		return layer.Layer{}, ledger.ErrUnknownEntry
	}
	var keep *layer.Keep
	if g.Floor > 0 && g.Keep != nil {
		keep = &layer.Keep{Floor: max(g.Floor, s.MinLayerStrength()), Func: g.Keep}
	}
	var l layer.Layer
	var strength int
	if g.Search != nil {
		l, strength, err = layer.GrindSearchChunk(ctx, g.Search, entryHash, start, tries, g.Workers, g.Tried, keep, g.Chunk)
	} else {
		l, strength, err = layer.GrindWithKeep(ctx, g.hasher(s), entryHash, start, tries, g.Workers, g.Tried, keep)
	}
	if err != nil && ctx.Err() == nil {
		return l, err
	}
	if strength < 0 {
		return l, ErrNoStrongerLayer
	}
	return l, nil
}

// PoolStatus summarizes one pool.
type PoolStatus struct {
	Key      PoolKey
	Tip      hash.Hash
	Height   int
	Weight   weight.Weight
	Entries  int
	Branches []ledger.Branch
	Pending  int
	Waiting  int
}

// Status returns the status of a followed pool.
func (e *Engine) Status(k PoolKey) (PoolStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if id, ok, err := e.unstated(k); err != nil {
		return PoolStatus{Key: k}, err
	} else if ok {
		// What a fresh state reads, without making one (ORANGE-030).
		return PoolStatus{Key: k, Tip: id, Branches: []ledger.Branch{}}, nil
	}
	p := e.pools[k]
	tip, h, w := p.ledger.Tip()
	return PoolStatus{
		Key: k, Tip: tip, Height: h, Weight: w, Entries: p.ledger.Len(),
		Branches: p.ledger.Branches(), Pending: p.pending.Len(), Waiting: p.pending.Waiting(),
	}, nil
}

// Tip returns pool k's selected tip, height and weight: Status without
// the inactive branches, whose listing walks every off-chain branch back
// to the selected ledger (ORANGE-015).
func (e *Engine) Tip(k PoolKey) (hash.Hash, int, weight.Weight, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if id, ok, err := e.unstated(k); err != nil {
		return hash.Hash{}, 0, weight.Zero, err
	} else if ok {
		return id, 0, weight.Zero, nil
	}
	tip, h, w := e.pools[k].ledger.Tip()
	return tip, h, w, nil
}

// Payment is what a receiver reads about one transaction.
type Payment struct {
	// Pending: held as a pending transaction, not in the selected ledger.
	Pending bool
	// Entry holds the transaction in the selected ledger, if it is there.
	Entry hash.Hash
	// From is from(Entry): the weight a challenger must beat, after the
	// entry's parent, to reverse the payment. Zero when not selected.
	From     weight.Weight
	Selected bool
	// Tx is the transaction, when the node holds it (selected or pending).
	Tx *tx.Tx
	// Rivals are the inactive branches this node holds that spend one of
	// the same portions, heaviest first. Against each, the margin is From
	// minus the rival's weight after its fork point.
	Rivals []ledger.Rival
	// Unreadable is set when the transaction is in the selected ledger
	// but its entry's body cannot be read back from the log: Tx is then
	// nil, and Rivals were still found, from the entry's skeleton
	// (RED-084, D173).
	Unreadable bool
}

// Payment reports where a transaction stands in pool k.
func (e *Engine) Payment(k PoolKey, tid hash.Hash) (Payment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.peek(k)
	if err != nil {
		return Payment{}, err
	}
	var out Payment
	if eh, ok := p.ledger.Locate(tid); ok {
		w, _ := p.ledger.From(eh)
		out = Payment{Entry: eh, From: w, Selected: true}
		if en, err := p.ledger.Entry(eh); err == nil {
			h := p.ledger.Series().Hash()
			for _, t := range en.Txs {
				if t.TID(h) == tid {
					out.Tx = t
					break
				}
			}
		} else {
			out.Unreadable = true
		}
	} else if t, ok := p.pending.Get(tid); ok {
		out = Payment{Pending: true, Tx: t}
	}
	// Rivals need only the inputs, which the skeleton keeps.
	if spend := out.Tx; spend != nil || out.Unreadable {
		if spend == nil {
			spend, _ = p.ledger.Tx(out.Entry, tid)
		}
		if spend != nil {
			out.Rivals = p.ledger.Rivals(spend, tid)
		}
	}
	return out, nil
}

// Holdings returns the unspent portions of one mint at the selected tip.
func (e *Engine) Holdings(id hash.Hash, serial uint64) ([]state.Holding, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.series[id]
	if !ok {
		return nil, ErrUnknownSeries
	}
	p, err := e.peek(PoolKey{Series: id, Index: s.PoolOf(serial)})
	if err != nil {
		return nil, err
	}
	return p.ledger.State().Portions(serial), nil
}

// Pending returns the pending transactions of pool k that are ready to go
// into an entry, oldest first.
func (e *Engine) Pending(k PoolKey) ([]mempool.Item, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.peek(k)
	if err != nil {
		return nil, err
	}
	return p.pending.Ready(), nil
}

// Ledger runs fn with the ledger of a followed pool while holding the
// engine's lock, for read-only queries such as serving ledger requests.
func (e *Engine) Ledger(k PoolKey, fn func(*ledger.Pool)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.peek(k)
	if err != nil {
		return err
	}
	fn(p.ledger)
	return nil
}
