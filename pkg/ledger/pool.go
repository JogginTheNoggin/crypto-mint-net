// Package ledger implements one pool's tree of entries and the consensus
// rule that selects a ledger from it.
//
// A Pool holds every valid entry it has been given whose parent it holds,
// the distinct layers on each, the selected ledger (the valid ledger with
// the greatest weight), the pool state at the selected tip, and up to K
// inactive branches. It is the full node's implementation of the design's
// sections on weight, selection, switching and displaced transactions:
//
//   - own(e) is the credit of the distinct layers on e, plus the credit of
//     e's own hash where the series counts it.
//   - weight(ledger) is the sum of own(e) over its entries.
//   - The selected ledger is the heaviest. A ledger replaces the selected one
//     only when it is strictly heavier, so an exact tie keeps what was
//     selected first. There is no depth limit on switching.
//   - Layers are kept by entry hash, so a switch keeps every layer on the
//     entries the new ledger shares with the old (transfer layers).
//   - On a switch, the transactions of the replaced entries are displaced,
//     and each is unchanged, carried or void.
//
// A Pool is not safe for concurrent use; the engine serializes access.
package ledger

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Errors returned by Pool methods.
var (
	// ErrKnownEntry: the entry is already held. Not an offence.
	ErrKnownEntry = errors.New("ledger: entry already held")
	// ErrUnknownParent: the entry's parent is not held, so it cannot be
	// checked and is not kept. Its stated weight may be worth chasing.
	ErrUnknownParent = errors.New("ledger: parent not held")
	// ErrInvalidEntry: the entry breaks a rule. It is thrown out together
	// with its hash.
	ErrInvalidEntry = errors.New("ledger: invalid entry")
	// ErrUnknownEntry: a layer names an entry hash the pool does not hold.
	// The layer is thrown out, forgotten rather than lost.
	ErrUnknownEntry = errors.New("ledger: layer on an entry not held")
	// ErrWeakLayer: the layer is below the series' minimum layer strength.
	ErrWeakLayer = errors.New("ledger: layer below the minimum layer strength")
	// ErrKnownLayer: the layer is already counted. Not an offence.
	ErrKnownLayer = errors.New("ledger: layer already counted")
	// ErrNotSelected: the entry is not in the selected ledger.
	ErrNotSelected = errors.New("ledger: entry not in the selected ledger")
	// ErrNoEntry: Entry was asked for an entry the pool does not hold.
	ErrNoEntry = errors.New("ledger: entry not held")
	// ErrUnreadable: the pool holds the entry, but its body is not in
	// memory and could not be read back (a damaged or changed log, a disk
	// error). For the caller it is not held right now; it decides nothing,
	// since no check reads a body that is not in memory (RED-084, D173).
	ErrUnreadable = errors.New("ledger: entry body cannot be read back")
)

// Unlimited, as K, keeps every inactive branch.
const Unlimited = -1

// Provisional defaults. The design leaves default values open until tests
// show what works; see docs/decisions.md.
const (
	DefaultK    = 8
	DefaultTies = 64
)

type node struct {
	hash     hash.Hash
	entry    *entry.Entry
	items    []state.Item
	parent   *node // nil when the parent is the pool_id
	children []*node
	height   int // 1 for an entry whose parent is the pool_id
	own      weight.Weight
	layers   map[hash.Hash]layer.Layer
	undo     state.Undo
	// nitems counts the entry's transactions and claims the mints it
	// claims, kept when a cold entry drops its items (D175).
	nitems  int
	claims  []uint64
	cold    bool
	seq     uint64
	seen    time.Time
	onChain bool
	rel     weight.Weight // off chain: own weight from the anchor (exclusive) to here
	anchor  int           // off chain: the anchor's height, 0 for the pool_id
}

// Config holds a pool's node policy settings.
type Config struct {
	// K is how many inactive branches to keep, or Unlimited. A branch is
	// an entry outside the selected ledger with no children, together with
	// the entries leading to it.
	K int
	// Ties is how many off-chain entries whose ledger ties with the
	// selected one are kept, in addition to K, earliest seen first. An
	// entry that earns no credit ties with its parent's ledger; it must be
	// kept so the layers that will make it selected can be counted. 0
	// means DefaultTies.
	Ties int
	// Source, when set, gives back entry bodies from outside memory, so
	// the pool drops each body once Source has it (D172). Nil keeps every
	// body in memory, as before.
	Source BodySource
	// Bodies caches the bodies read back from Source; one cache may serve
	// many pools. Nil caches none.
	Bodies *BodyCache
	// RecentWindow is how many entries below the selected tip keep their
	// items and undo records in memory once their bodies are in Source
	// (D175); deeper ones are read back when a switch or a view needs
	// them. 0 keeps every one in memory.
	RecentWindow int
	// Clock gives the local time recorded when an entry is first seen. It
	// is never read by selection. Defaults to time.Now.
	Clock func() time.Time
}

// Pool is the tree of entries of one pool.
type Pool struct {
	s     *series.Series
	index uint64
	id    hash.Hash
	cfg   Config

	nodes map[hash.Hash]*node
	roots []*node
	chain []*node // the selected ledger, chain[h-1] at height h
	// units[h] is the view cost (unitsOf) of chain[:h], so the cost of a
	// revert to any height is one subtraction (RED-012).
	units []int
	bit   *fenwick
	st    *state.State // pool state at the selected tip
	tids  map[hash.Hash]*node
	// off holds every node outside the selected ledger, so ranking the
	// inactive branches looks only at them, not at every entry held.
	off map[*node]struct{}
	// leaves holds the nodes of off with no children, so ranking the
	// branches looks only at the branch tips, not at every entry of a long
	// branch (RED-012).
	leaves map[*node]struct{}
	seq    uint64
	// newest is the entry added last. It is never pruned while it is a
	// leaf, so a branch fetched parent first can take its layers and its
	// next entry before it is ranked (decision D79).
	newest *node
	// view is the state after viewOf, kept from the last AddEntry so the
	// next entry on it (every entry of a branch taken in order) starts
	// there instead of re-applying the branch (RED-012, decision D114).
	// Any change to the selected chain or to viewOf drops it.
	view   *state.State
	viewOf *node
	// held are the entries added since their bodies were last released
	// to the source (releaseBodies, D172).
	held []*node
	// cooled is how many entries of the selected ledger, from the bottom,
	// cool has passed: chain[:cooled] may be cold (D175).
	cooled int
}

// New returns an empty pool: no entries, weight 0, nothing claimed.
func New(s *series.Series, index uint64, cfg Config) *Pool {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Ties == 0 {
		cfg.Ties = DefaultTies
	}
	return &Pool{
		s: s, index: index, id: s.PoolID(index), cfg: cfg,
		nodes:  map[hash.Hash]*node{},
		bit:    newFenwick(),
		units:  []int{0},
		st:     state.New(),
		tids:   map[hash.Hash]*node{},
		off:    map[*node]struct{}{},
		leaves: map[*node]struct{}{},
	}
}

// Series returns the pool's series.
func (p *Pool) Series() *series.Series { return p.s }

// Index returns the pool index.
func (p *Pool) Index() uint64 { return p.index }

// ID returns the pool_id, the parent of the pool's first entries.
func (p *Pool) ID() hash.Hash { return p.id }

// SetK changes how many inactive branches are kept and prunes at once.
func (p *Pool) SetK(k int) {
	p.cfg.K = k
	p.prune()
}

func (p *Pool) tip() *node {
	if len(p.chain) == 0 {
		return nil
	}
	return p.chain[len(p.chain)-1]
}

// Tip returns the newest entry hash of the selected ledger (the pool_id
// when the ledger is empty), its height, and the ledger's weight.
func (p *Pool) Tip() (hash.Hash, int, weight.Weight) {
	if t := p.tip(); t != nil {
		return t.hash, t.height, p.bit.prefix(t.height)
	}
	return p.id, 0, weight.Zero
}

// Weight returns the weight of the selected ledger.
func (p *Pool) Weight() weight.Weight { return p.bit.prefix(len(p.chain)) }

// Height returns the number of entries in the selected ledger.
func (p *Pool) Height() int { return len(p.chain) }

// Len returns how many entries the pool holds, on every branch.
func (p *Pool) Len() int { return len(p.nodes) }

// State returns the pool state at the selected tip. Callers must not
// modify it.
func (p *Pool) State() *state.State { return p.st }

// Has reports whether the entry is held, on any branch.
func (p *Pool) Has(h hash.Hash) bool { _, ok := p.nodes[h]; return ok }

// Entry returns a held entry. It returns ErrNoEntry for an entry the
// pool does not hold and ErrUnreadable for one whose body cannot be read
// back, and a nil entry only with an error (RED-084, D173).
func (p *Pool) Entry(h hash.Hash) (*entry.Entry, error) {
	n, ok := p.nodes[h]
	if !ok {
		return nil, ErrNoEntry
	}
	if e, ok := p.body(n); ok {
		return e, nil
	}
	return nil, ErrUnreadable
}

// Tx returns transaction tid of held entry h from the entry's skeleton,
// which is always in memory. Its witnesses are gone once the entry's body
// was released, so it serves what reads inputs and outputs only (D173).
func (p *Pool) Tx(h, tid hash.Hash) (*tx.Tx, bool) {
	n, ok := p.nodes[h]
	if !ok {
		return nil, false
	}
	items, err := p.itemsOf(n)
	if err != nil {
		return nil, false
	}
	for _, it := range items {
		if it.TID == tid {
			return it.Tx, true
		}
	}
	return nil, false
}

// Info describes a held entry.
type Info struct {
	Hash      hash.Hash
	Parent    hash.Hash
	Height    int
	Own       weight.Weight
	Ledger    weight.Weight // weight of the ledger that ends at this entry
	Layers    int
	Selected  bool
	FirstSeen time.Time
}

// Info returns what the pool knows about a held entry.
func (p *Pool) Info(h hash.Hash) (Info, bool) {
	n, ok := p.nodes[h]
	if !ok {
		return Info{}, false
	}
	return Info{
		Hash: n.hash, Parent: p.parentHash(n), Height: n.height, Own: n.own,
		Ledger: p.weightOf(n), Layers: len(n.layers), Selected: n.onChain, FirstSeen: n.seen,
	}, true
}

// Layers returns the layers counted on a held entry, sorted by nonce.
func (p *Pool) Layers(h hash.Hash) []layer.Layer {
	n, ok := p.nodes[h]
	if !ok {
		return nil
	}
	out := make([]layer.Layer, 0, len(n.layers))
	for _, l := range n.layers {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nonce < out[j].Nonce })
	return out
}

// LayerCount returns how many layers the entry holds.
func (p *Pool) LayerCount(h hash.Hash) int {
	if n, ok := p.nodes[h]; ok {
		return len(n.layers)
	}
	return 0
}

// LayerNonces returns the nonces of the entry's layers, in no order: a
// copy for a caller that sorts outside the engine's lock (RED-020).
func (p *Pool) LayerNonces(h hash.Hash) []uint64 {
	n, ok := p.nodes[h]
	if !ok {
		return nil
	}
	out := make([]uint64, 0, len(n.layers))
	for _, l := range n.layers {
		out = append(out, l.Nonce)
	}
	return out
}

// InSelected reports whether the entry is in the selected ledger.
func (p *Pool) InSelected(h hash.Hash) bool {
	n, ok := p.nodes[h]
	return ok && n.onChain
}

// From returns from(e): the own weight of e plus the own weight of every
// later entry of the selected ledger. It is the number a receiver reads.
func (p *Pool) From(h hash.Hash) (weight.Weight, error) {
	n, ok := p.nodes[h]
	if !ok || !n.onChain {
		return weight.Zero, ErrNotSelected
	}
	return p.Weight().Sub(p.bit.prefix(n.height - 1)), nil
}

// WeightTo returns the weight of the ledger that ends at a held entry: its
// own weight and that of every entry before it. The pool_id has none. A
// chase uses it for the weight to beat after a common entry.
func (p *Pool) WeightTo(h hash.Hash) (weight.Weight, bool) {
	if h == p.id {
		return weight.Zero, true
	}
	n, ok := p.nodes[h]
	if !ok {
		return weight.Zero, false
	}
	return p.weightOf(n), true
}

// Locate returns the entry of the selected ledger that holds a tid.
func (p *Pool) Locate(tid hash.Hash) (hash.Hash, bool) {
	n, ok := p.tids[tid]
	if !ok {
		return hash.Zero, false
	}
	return n.hash, true
}

// Chain returns the entry hashes of the selected ledger at heights
// from..to inclusive, clamped to the ledger.
func (p *Pool) Chain(from, to int) []hash.Hash {
	if from < 1 {
		from = 1
	}
	if to > len(p.chain) {
		to = len(p.chain)
	}
	var out []hash.Hash
	for h := from; h <= to; h++ {
		out = append(out, p.chain[h-1].hash)
	}
	return out
}

// HeightOf returns the height of an entry in the selected ledger.
func (p *Pool) HeightOf(h hash.Hash) (int, bool) {
	n, ok := p.nodes[h]
	if !ok || !n.onChain {
		return 0, false
	}
	return n.height, true
}

func (p *Pool) weightOf(n *node) weight.Weight {
	if n.onChain {
		return p.bit.prefix(n.height)
	}
	a := p.anchorHeight(n)
	return p.bit.prefixPlus(a, n.rel)
}

// anchorHeight returns the height of the nearest selected ancestor of an
// off-chain node, or 0 when it is the pool_id.
func (p *Pool) anchorHeight(n *node) int { return n.anchor }

// AddResult reports what adding an entry or a layer did.
type AddResult struct {
	Hash hash.Hash
	// Switch is set when the selected ledger changed.
	Switch *Switch
	// Pruned lists entries dropped to keep within K.
	Pruned []hash.Hash
}

// AddEntry checks an entry and, if it is valid and its parent is held,
// keeps it. It returns ErrKnownEntry, ErrUnknownParent, or an error wrapping
// ErrInvalidEntry when it does not keep it.
func (p *Pool) AddEntry(e *entry.Entry) (AddResult, error) {
	// The cheap shape and the parent come before any signature: an entry
	// on a parent not held costs no signature check (RED-009).
	p.releaseBodies()
	h, items, err := entry.CheckShape(p.s, p.index, e)
	if err != nil {
		return AddResult{}, fmt.Errorf("%w: %v", ErrInvalidEntry, err)
	}
	if _, ok := p.nodes[h]; ok {
		return AddResult{Hash: h}, ErrKnownEntry
	}
	parent, ok := p.parentOf(e)
	if !ok {
		return AddResult{Hash: h}, ErrUnknownParent
	}
	for _, it := range items {
		if err := tx.CheckStateless(p.s, it.Tx, it.TID); err != nil {
			return AddResult{Hash: h}, fmt.Errorf("%w: %w %s: %v", ErrInvalidEntry, entry.ErrInvalidTx, it.TID.Short(), err)
		}
	}
	view := p.viewAfter(parent)
	if err := entry.CheckAgainst(p.s, view, items); err != nil {
		return AddResult{Hash: h}, fmt.Errorf("%w: %v", ErrInvalidEntry, err)
	}
	stItems := toStateItems(items)
	undo := view.Apply(stItems)
	p.view, p.viewOf = nil, nil

	p.seq++
	n := &node{
		hash: h, entry: e, items: stItems, parent: parent, undo: undo, nitems: len(stItems),
		layers: map[hash.Hash]layer.Layer{}, seq: p.seq, seen: p.cfg.Clock(),
	}
	for _, it := range stItems {
		if it.Tx.IsClaim() {
			n.claims = append(n.claims, it.Tx.Mint.Serial)
		}
	}
	if p.s.EntryHashCredit() {
		n.own = weight.Credit(h.Strength())
	}
	if parent == nil {
		n.height = 1
		p.roots = append(p.roots, n)
		n.rel = n.own
	} else {
		n.height = parent.height + 1
		parent.children = append(parent.children, n)
		delete(p.leaves, parent)
		if parent.onChain {
			n.rel, n.anchor = n.own, parent.height
		} else {
			n.rel, n.anchor = parent.rel.Add(n.own), parent.anchor
		}
	}
	p.nodes[h] = n
	if p.cfg.Source != nil {
		p.held = append(p.held, n)
	}
	p.off[n] = struct{}{}
	p.leaves[n] = struct{}{}
	p.newest = n

	res := AddResult{Hash: h}
	if p.weightOf(n).Greater(p.Weight()) {
		res.Switch = p.switchTo(n)
	} else {
		p.view, p.viewOf = view, n
	}
	res.Pruned = p.prune()
	p.cool()
	return res, nil
}

// parentOf returns the node of e's parent, nil for the pool's first
// entry, and whether it is held.
func (p *Pool) parentOf(e *entry.Entry) (*node, bool) {
	if e.Parent == p.id {
		return nil, true
	}
	n, ok := p.nodes[e.Parent]
	return n, ok
}

// EntryOwners runs AddEntry's checks short of every witness signature and
// returns e's items with, for each, the addresses its witnesses must
// satisfy against the state after e's parent (entry.Owners), or the error
// AddEntry would give. It changes nothing.
func (p *Pool) EntryOwners(e *entry.Entry) ([]entry.Item, [][]hash.Hash, error) {
	h, items, err := entry.CheckShape(p.s, p.index, e)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidEntry, err)
	}
	if _, ok := p.nodes[h]; ok {
		return nil, nil, ErrKnownEntry
	}
	parent, ok := p.parentOf(e)
	if !ok {
		return nil, nil, ErrUnknownParent
	}
	// The state after a deep parent costs a revert of the selected chain
	// down to it; AddEntry pays that under the lock anyway, so it is not
	// paid twice here (RED-011, decision D111).
	if p.viewCost(parent, MaxPreviewCost) > MaxPreviewCost {
		return nil, nil, ErrDeepParent
	}
	owners, err := entry.Owners(p.s, p.viewAfter(parent), items)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidEntry, err)
	}
	return items, owners, nil
}

// PageOwners is EntryOwners for a run of entries, each the parent of the
// next, as a chase receives them: each entry's owners are found against
// the state after the entries before it in the run, applied to a scratch
// overlay. It returns the items and owners of the entries it got through,
// in order, and stops before the first it cannot preview: one that fails
// AddEntry's checks short of the witnesses, does not follow the one before
// it, is held already, or whose first parent is too far from the selected
// tip. It changes nothing (ORANGE-020).
func (p *Pool) PageOwners(es []*entry.Entry) ([][]entry.Item, [][][]hash.Hash) {
	if len(es) == 0 {
		return nil, nil
	}
	parent, ok := p.parentOf(es[0])
	if !ok || p.viewCost(parent, MaxPreviewCost) > MaxPreviewCost {
		return nil, nil
	}
	v := p.viewAfter(parent).Overlay()
	var items [][]entry.Item
	var owners [][][]hash.Hash
	var prev hash.Hash
	for i, e := range es {
		h, its, err := entry.CheckShape(p.s, p.index, e)
		if err != nil || (i > 0 && e.Parent != prev) {
			break
		}
		if _, held := p.nodes[h]; held {
			break
		}
		o, err := entry.Owners(p.s, v, its)
		if err != nil {
			break
		}
		v.Apply(toStateItems(its))
		items, owners = append(items, its), append(owners, o)
		prev = h
	}
	return items, owners
}

// MaxPreviewCost is the most EntryOwners spends, in view cost units
// (ViewCost), to build the state after an entry's parent. An honest entry
// builds on the tip or close to it.
const MaxPreviewCost = 256

// ErrDeepParent: EntryOwners will not build the state after a parent that
// far from the selected tip. Not an offence, and AddEntry still takes the
// entry.
var ErrDeepParent = errors.New("ledger: parent too far from the selected tip to preview")

// ViewCost is what checking e against the state after its parent costs:
// one unit for each entry reverted or applied and one for each of its
// transactions, counting no further than about limit; 0 for a parent this
// pool does not hold, the tip, or the entry the last AddEntry kept the
// state after. It changes nothing (RED-011, RED-012).
func (p *Pool) ViewCost(e *entry.Entry, limit int) int {
	parent, ok := p.parentOf(e)
	if !ok {
		return 0
	}
	return p.viewCost(parent, limit)
}

// viewCost is the cost of viewAfter(n), counting no further than about
// limit.
func (p *Pool) viewCost(n *node, limit int) int {
	if n != nil && (n == p.viewOf || n.onChain && n.height == len(p.chain)) {
		return 0
	}
	cost := 0
	x := n
	for x != nil && !x.onChain {
		if cost += unitsOf(x); cost > limit {
			return cost
		}
		x = x.parent
	}
	anchor := 0
	if x != nil {
		anchor = x.height
	}
	return cost + p.units[len(p.chain)] - p.units[anchor]
}

// unitsOf is the view cost of reverting or applying one entry.
func unitsOf(n *node) int { return 1 + n.nitems }

func toStateItems(items []entry.Item) []state.Item {
	out := make([]state.Item, len(items))
	for i, it := range items {
		out[i] = state.Item{TID: it.TID, Tx: it.Tx}
	}
	return out
}

// viewAfter returns a scratch state equal to the pool state after entry n
// (nil for the pool_id), as an overlay on the tip state. The caller may
// change it only after it has dropped the kept view (AddEntry).
func (p *Pool) viewAfter(n *node) *state.State {
	if n != nil && n == p.viewOf {
		return p.view
	}
	v := p.st.Overlay()
	if n != nil && n.onChain && n.height == len(p.chain) {
		return v
	}
	// Find the anchor and the off-chain path down to n.
	var path []*node
	x := n
	for x != nil && !x.onChain {
		path = append(path, x)
		x = x.parent
	}
	anchor := 0
	if x != nil {
		anchor = x.height
	}
	memo := map[*node][]state.Item{}
	for h := len(p.chain); h > anchor; h-- {
		c := p.chain[h-1]
		items, undo := p.warmed(c, memo)
		v.Revert(items, undo)
	}
	for i := len(path) - 1; i >= 0; i-- {
		v.Apply(path[i].items)
	}
	return v
}

// AddLayer counts a layer on a held entry. It returns ErrUnknownEntry,
// ErrWeakLayer or ErrKnownLayer when it does not count it.
func (p *Pool) AddLayer(l layer.Layer) (AddResult, error) {
	return p.AddLayerHashed(l, l.Hash(p.s.Hash()))
}

// AddLayerHashed is AddLayer for a caller that has already hashed the
// layer: lh must be l.Hash under the series' hash (ORANGE-009).
func (p *Pool) AddLayerHashed(l layer.Layer, lh hash.Hash) (AddResult, error) {
	n, ok := p.nodes[l.Entry]
	if !ok {
		return AddResult{}, ErrUnknownEntry
	}
	if lh.Strength() < p.s.MinLayerStrength() {
		return AddResult{Hash: lh}, ErrWeakLayer
	}
	if _, dup := n.layers[lh]; dup {
		return AddResult{Hash: lh}, ErrKnownLayer
	}
	n.layers[lh] = l
	c := weight.Credit(lh.Strength())
	n.own = n.own.Add(c)
	res := AddResult{Hash: lh}
	if n.onChain {
		// Every ledger through n gains c, the selected one included, so
		// no other ledger can overtake it.
		p.bit.add(n.height, c)
		return res, nil
	}
	// Off chain: n and its subtree gain c. Only they can overtake.
	var best *node
	var walk func(*node)
	walk = func(x *node) {
		x.rel = x.rel.Add(c)
		if best == nil || p.better(x, best) {
			best = x
		}
		for _, ch := range x.children {
			walk(ch)
		}
	}
	walk(n)
	if p.weightOf(best).Greater(p.Weight()) {
		res.Switch = p.switchTo(best)
	}
	res.Pruned = p.prune()
	p.cool()
	return res, nil
}

// better reports whether a is preferred to b as a selection candidate: a
// greater weight, or an equal weight and seen first.
func (p *Pool) better(a, b *node) bool {
	switch p.weightOf(a).Cmp(p.weightOf(b)) {
	case 1:
		return true
	case -1:
		return false
	}
	return a.seq < b.seq
}

// Switch describes a change of the selected ledger.
type Switch struct {
	From, To   hash.Hash // old and new newest entries (pool_id if empty)
	Fork       hash.Hash // last shared entry (pool_id if none)
	ForkHeight int
	// Reverted are the old ledger's entries after the fork, newest first.
	Reverted []hash.Hash
	// Applied are the new ledger's entries after the fork, oldest first.
	Applied []hash.Hash
	// Displaced transactions, in the order they had in the old ledger.
	Unchanged []Displaced // already in the new ledger after the fork
	Carried   []Displaced // still apply; to be bundled again
	Void      []Displaced // cannot apply in any ledger extending the new one
}

// Extension reports whether the switch only added entries on top of the
// old ledger, displacing nothing.
func (s *Switch) Extension() bool { return len(s.Reverted) == 0 }

// Displaced is a transaction from a replaced entry.
type Displaced struct {
	TID hash.Hash
	Tx  *tx.Tx
	// Entry is the replaced entry that held it.
	Entry hash.Hash
	// Weight was from(Entry) in the old ledger just before the switch: how
	// buried the transaction was when it was displaced.
	Weight weight.Weight
}

func (p *Pool) switchTo(n *node) *Switch {
	oldTip := p.tip()
	sw := &Switch{From: p.id, To: n.hash, Fork: p.id}
	if oldTip != nil {
		sw.From = oldTip.hash
	}
	// The new segment: off-chain ancestors of n, then n.
	var seg []*node
	f := n
	for f != nil && !f.onChain {
		seg = append(seg, f)
		f = f.parent
	}
	forkHeight := 0
	if f != nil {
		forkHeight = f.height
		sw.Fork = f.hash
	}
	sw.ForkHeight = forkHeight
	for i, j := 0, len(seg)-1; i < j; i, j = i+1, j-1 {
		seg[i], seg[j] = seg[j], seg[i]
	}

	// Record how buried each displaced entry was before anything changes.
	oldWeight := p.Weight()
	old := append([]*node(nil), p.chain[forkHeight:]...)
	// Read back the items and undo of every cold entry to revert, before
	// anything changes (D175).
	if forkHeight < p.cooled {
		p.warm(old)
		p.cooled = forkHeight
	}
	buried := make([]weight.Weight, len(old))
	for i, o := range old {
		buried[i] = oldWeight.Sub(p.bit.prefix(o.height - 1))
	}

	// Undo the old segment, newest first.
	for i := len(old) - 1; i >= 0; i-- {
		o := old[i]
		p.st.Revert(o.items, o.undo)
		o.onChain = false
		p.off[o] = struct{}{}
		if len(o.children) == 0 {
			p.leaves[o] = struct{}{}
		}
		for _, it := range o.items {
			delete(p.tids, it.TID)
		}
		sw.Reverted = append(sw.Reverted, o.hash)
	}
	p.chain = p.chain[:forkHeight]
	p.units = p.units[:forkHeight+1]
	p.bit.truncate(forkHeight)
	p.view, p.viewOf = nil, nil

	// Apply the new segment, oldest first.
	newTIDs := map[hash.Hash]struct{}{}
	for _, x := range seg {
		x.undo = p.st.Apply(x.items)
		x.onChain = true
		delete(p.off, x)
		delete(p.leaves, x)
		p.chain = append(p.chain, x)
		p.units = append(p.units, p.units[len(p.units)-1]+unitsOf(x))
		p.bit.push(x.own)
		for _, it := range x.items {
			p.tids[it.TID] = x
			newTIDs[it.TID] = struct{}{}
		}
		sw.Applied = append(sw.Applied, x.hash)
	}

	// Off-chain nodes below the fork have new anchors: recompute rel.
	var reanchor func(x *node)
	reanchor = func(x *node) {
		if !x.onChain {
			switch {
			case x.parent == nil:
				x.rel, x.anchor = x.own, 0
			case x.parent.onChain:
				x.rel, x.anchor = x.own, x.parent.height
			default:
				x.rel, x.anchor = x.parent.rel.Add(x.own), x.parent.anchor
			}
		}
		for _, c := range x.children {
			reanchor(c)
		}
	}
	if f == nil {
		for _, r := range p.roots {
			reanchor(r)
		}
	} else {
		for _, c := range f.children {
			reanchor(c)
		}
	}

	// Sort the displaced transactions. One replay on an overlay of the new
	// tip state finds both reasons a transaction is void: a portion it
	// spends is already spent in the new ledger, or was created by a void
	// transaction.
	//
	// An entry whose body was released has items without witnesses
	// (stripWitnesses). Its transactions are sorted by the same rules
	// short of the witnesses (tx.Owners): the witnesses satisfied these
	// owners when the entry was checked, and an input's owner is fixed by
	// the transaction that made it, so the answer is the one CheckAgainst
	// gives. The body is read back once, so every displaced transaction
	// is reported whole, as before (the watch hub finds spenders by their
	// witnesses). If it cannot be read, the transactions are reported
	// without witnesses and none is carried, since a carried one goes back
	// to the pending transactions; that only leaves it for a peer to send
	// again (ORANGE-038).
	replay := p.st.Overlay()
	for i, o := range old {
		bare := stripped(o)
		var whole []*tx.Tx
		if bare {
			if e, ok := p.body(o); ok && len(e.Txs) == len(o.items) {
				whole = e.Txs
			}
		}
		for j, it := range o.items {
			d := Displaced{TID: it.TID, Tx: it.Tx, Entry: o.hash, Weight: buried[i]}
			if whole != nil {
				d.Tx = whole[j]
			}
			if _, ok := newTIDs[it.TID]; ok {
				sw.Unchanged = append(sw.Unchanged, d)
				continue
			}
			var err error
			if bare {
				_, err = tx.Owners(replay, it.Tx)
			} else {
				err = tx.CheckAgainst(p.s, replay, it.Tx, it.TID)
			}
			if err != nil {
				sw.Void = append(sw.Void, d)
				continue
			}
			replay.ApplyTx(it, nil)
			if bare && whole == nil {
				continue
			}
			sw.Carried = append(sw.Carried, d)
		}
	}
	return sw
}

// Branch describes one inactive branch: an off-chain entry with no
// children.
type Branch struct {
	Tip    hash.Hash
	Height int
	Weight weight.Weight
	Fork   hash.Hash // last entry it shares with the selected ledger
}

// Branches returns the inactive branches, heaviest first.
func (p *Pool) Branches() []Branch {
	leaves := p.offChainLeaves()
	out := make([]Branch, 0, len(leaves))
	for _, lf := range leaves {
		l := lf.n
		b := Branch{Tip: l.hash, Height: l.height, Weight: lf.w, Fork: p.id}
		for x := l.parent; x != nil; x = x.parent {
			if x.onChain {
				b.Fork = x.hash
				break
			}
		}
		out = append(out, b)
	}
	return out
}

// Rival is an inactive branch holding a transaction that spends one of
// the same portions as a payment: a known rival of the payment.
type Rival struct {
	Branch
	// Conflict is the rival transaction's tid, and Entry the entry that
	// holds it.
	Conflict hash.Hash
	Entry    hash.Hash
	// After is the branch's weight after its fork point: what it has to
	// set against the payment's from(e).
	After weight.Weight
}

// Rivals returns the inactive branches that hold a spend of any input of
// t other than t itself, heaviest first. Only the entries after each
// branch's fork point are searched: below it the branch is the selected
// ledger. A claim has no inputs and so no rivals here.
func (p *Pool) Rivals(t *tx.Tx, tid hash.Hash) []Rival {
	if len(t.Inputs) == 0 {
		return nil
	}
	spends := make(map[tx.Input]bool, len(t.Inputs))
	for _, in := range t.Inputs {
		spends[in] = true
	}
	var out []Rival
	for _, b := range p.Branches() {
		for x := p.nodes[b.Tip]; x != nil && !x.onChain; x = x.parent {
			found := false
			// The skeleton's items carry every input, so the search reads
			// no body and cannot miss a rival whose body is unreadable
			// (RED-084, D173).
			for _, it := range x.items {
				o, otid := it.Tx, it.TID
				if o.IsClaim() {
					continue
				}
				if otid == tid {
					continue
				}
				for _, in := range o.Inputs {
					if spends[in] {
						to, _ := p.WeightTo(b.Fork)
						out = append(out, Rival{Branch: b, Conflict: otid, Entry: x.hash, After: b.Weight.Sub(to)})
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}
	}
	return out
}

// ReplayOrder returns every held entry in an order that, given with its
// layers to an empty pool with the same configuration, rebuilds the same
// tree and the same selected ledger: the selected ledger from the pool_id
// up, then every other entry by first sighting. A store uses it to compact
// its log. Putting the selected ledger first matters for ties: replayed
// first, it is selected first, so a branch of equal weight does not take
// its place.
func (p *Pool) ReplayOrder() []hash.Hash {
	out := make([]hash.Hash, 0, len(p.nodes))
	for _, n := range p.chain {
		out = append(out, n.hash)
	}
	rest := make([]*node, 0, len(p.nodes)-len(p.chain))
	for _, n := range p.nodes {
		if !n.onChain {
			rest = append(rest, n)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].seq < rest[j].seq })
	for _, n := range rest {
		out = append(out, n.hash)
	}
	return out
}

// leaf is an off-chain node with no children and its ledger's weight.
type leaf struct {
	n *node
	w weight.Weight
}

// offChainLeaves returns the off-chain nodes with no children, best first:
// a greater weight, or an equal weight and seen first, as better ranks
// them. Each weight is worked out once, not at every comparison.
func (p *Pool) offChainLeaves() []leaf {
	var leaves []leaf
	for n := range p.leaves {
		leaves = append(leaves, leaf{n, p.weightOf(n)})
	}
	sort.Slice(leaves, func(i, j int) bool {
		switch leaves[i].w.Cmp(leaves[j].w) {
		case 1:
			return true
		case -1:
			return false
		}
		return leaves[i].n.seq < leaves[j].n.seq
	})
	return leaves
}

// prune drops inactive branches beyond the top K, ranked by weight and then
// by first sighting. Leaves whose ledger ties with the selected one are kept
// separately, up to Ties of them, earliest first. The newest entry is kept
// while it is a leaf, beyond those limits (D79). Entries shared with a kept
// branch or the selected ledger stay. A dropped entry is forgotten, not
// lost: it can be given again.
func (p *Pool) prune() []hash.Hash {
	if p.cfg.K < 0 {
		return nil
	}
	top := p.Weight()
	var ties, rest []*node
	for _, l := range p.offChainLeaves() {
		if l.w.Cmp(top) >= 0 {
			ties = append(ties, l.n)
		} else {
			rest = append(rest, l.n)
		}
	}
	var drop []*node
	if len(ties) > p.cfg.Ties {
		sort.Slice(ties, func(i, j int) bool { return ties[i].seq < ties[j].seq })
		drop = append(drop, ties[p.cfg.Ties:]...)
	}
	if len(rest) > p.cfg.K {
		drop = append(drop, rest[p.cfg.K:]...)
	}
	var dropped []hash.Hash
	for _, l := range drop {
		if l == p.newest {
			continue
		}
		// Remove l and its ancestors up to the first one that is selected
		// or still has other children.
		x := l
		for x != nil && !x.onChain && len(x.children) == 0 {
			parent := x.parent
			delete(p.nodes, x.hash)
			delete(p.off, x)
			delete(p.leaves, x)
			if x == p.viewOf {
				p.view, p.viewOf = nil, nil
			}
			dropped = append(dropped, x.hash)
			if parent == nil {
				p.roots = removeNode(p.roots, x)
			} else {
				parent.children = removeNode(parent.children, x)
			}
			x = parent
		}
	}
	return dropped
}

func removeNode(list []*node, x *node) []*node {
	for i, c := range list {
		if c == x {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}
