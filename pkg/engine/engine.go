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
}

// DefaultConfig returns the provisional defaults.
func DefaultConfig() Config {
	return Config{Ledger: ledger.Config{K: ledger.DefaultK}}
}

type follow struct {
	all   bool
	pools map[uint64]bool
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
}

// New returns an engine that knows no series and follows nothing.
func New(cfg Config) *Engine {
	return &Engine{
		cfg:     cfg,
		series:  map[hash.Hash]*series.Series{},
		follows: map[hash.Hash]*follow{},
		pools:   map[PoolKey]*poolState{},
		seen:    NewSeen(cfg.SeenSize),
	}
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
	e.followRule(id).all = true
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
	e.followRule(id).pools[index] = true
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
	s := e.series[k.Series]
	return k.Index < s.PoolCount() && (f.all || f.pools[k.Index])
}

// pool returns the state of a followed pool, creating it on first use.
func (e *Engine) pool(k PoolKey) (*poolState, error) {
	if p, ok := e.pools[k]; ok {
		return p, nil
	}
	s, ok := e.series[k.Series]
	if !ok {
		return nil, ErrUnknownSeries
	}
	if !e.followsLocked(k) {
		return nil, ErrNotFollowed
	}
	p := &poolState{
		s:       s,
		ledger:  ledger.New(s, k.Index, e.cfg.Ledger),
		pending: mempool.New(s, e.cfg.MaxPending),
	}
	e.pools[k] = p
	return p, nil
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
			if f.all || f.pools[k] {
				out = append(out, PoolKey{Series: id, Index: k})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Series.Compare(out[j].Series); c != 0 {
			return c < 0
		}
		return out[i].Index < out[j].Index
	})
	return out
}

// KeyOf returns the pool key of a transaction.
func (e *Engine) KeyOf(t *tx.Tx) (PoolKey, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.series[t.Mint.Series]
	if !ok {
		return PoolKey{}, ErrUnknownSeries
	}
	if t.Mint.Serial >= s.Mints() {
		return PoolKey{}, tx.ErrSerialRange
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
	e.mu.Lock()
	p, err := e.pool(k)
	if err != nil {
		e.mu.Unlock()
		return
	}
	tid := t.TID(p.s.Hash())
	var owners []hash.Hash
	if !e.seen.Has(tid) {
		owners, err = p.pending.Owners(view{p.ledger}, t, tid)
	}
	e.mu.Unlock()
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
	p, err := e.pool(k)
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
	e.mu.Lock()
	p, err := e.pool(k)
	if err != nil {
		e.mu.Unlock()
		return
	}
	var items []entry.Item
	var owners [][]hash.Hash
	if !(origin.checksSeen() && e.seen.Has(en.Hash(p.s))) {
		items, owners, err = p.ledger.EntryOwners(en)
	}
	e.mu.Unlock()
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
	e.mu.Lock()
	p, err := e.pool(k)
	if err != nil {
		e.mu.Unlock()
		return
	}
	items, owners := p.ledger.PageOwners(es)
	e.mu.Unlock()
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
	p, err := e.pool(k)
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
	p, err := e.pool(k)
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
	p, err := e.pool(k)
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

func (g Grind) hasher(s *series.Series) hash.NewHasher {
	if g.New != nil {
		return g.New
	}
	return hash.GrinderFor(s.Params().HashID)
}

// BuildEntryWith is BuildEntry searching as g says.
func (e *Engine) BuildEntryWith(ctx context.Context, k PoolKey, maxTries uint64, g Grind) (*entry.Entry, error) {
	e.mu.Lock()
	p, err := e.pool(k)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	ready := p.pending.Ready()
	if len(ready) == 0 {
		e.mu.Unlock()
		return nil, ErrNothingPending
	}
	txs := make([]*tx.Tx, len(ready))
	for i, r := range ready {
		txs[i] = r.Tx
	}
	parent, _, _ := p.ledger.Tip()
	s := p.s
	e.mu.Unlock()
	en, h, err := entry.BuildWith(ctx, g.hasher(s), s, parent, txs, maxTries, g.Workers, g.Tried)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	held := p.ledger.Has(h)
	e.mu.Unlock()
	if held {
		return en, ErrAlreadyBuilt
	}
	return en, nil
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
		l, strength, err = layer.GrindSearchKeep(ctx, g.Search, entryHash, start, tries, g.Workers, g.Tried, keep)
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
	p, err := e.pool(k)
	if err != nil {
		return PoolStatus{Key: k}, err
	}
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
	p, err := e.pool(k)
	if err != nil {
		return hash.Hash{}, 0, weight.Zero, err
	}
	tip, h, w := p.ledger.Tip()
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
}

// Payment reports where a transaction stands in pool k.
func (e *Engine) Payment(k PoolKey, tid hash.Hash) (Payment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.pool(k)
	if err != nil {
		return Payment{}, err
	}
	var out Payment
	if eh, ok := p.ledger.Locate(tid); ok {
		w, _ := p.ledger.From(eh)
		out = Payment{Entry: eh, From: w, Selected: true}
		if en, ok := p.ledger.Entry(eh); ok {
			h := p.ledger.Series().Hash()
			for _, t := range en.Txs {
				if t.TID(h) == tid {
					out.Tx = t
					break
				}
			}
		}
	} else if t, ok := p.pending.Get(tid); ok {
		out = Payment{Pending: true, Tx: t}
	}
	if out.Tx != nil {
		out.Rivals = p.ledger.Rivals(out.Tx, tid)
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
	p, err := e.pool(PoolKey{Series: id, Index: s.PoolOf(serial)})
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
	p, err := e.pool(k)
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
	p, err := e.pool(k)
	if err != nil {
		return err
	}
	fn(p.ledger)
	return nil
}
