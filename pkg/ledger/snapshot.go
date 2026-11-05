package ledger

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Snapshots of a pool (D178). A snapshot is the pool's skeleton as it
// stands: every held entry's hash, parent, layers, first-sighting order,
// counts, the mints it claims and, for the selected ledger, its tids, and
// the unspent portions at the selected tip. It holds no entry body: a
// restored pool reads bodies back through its BodySource, as a running
// one does (D172), and gets each recent entry's items and undo record
// from there (D175).
//
// A snapshot is node-local data written by the node itself, never
// something a peer sends, and restoring one gives the pool the node had
// when it wrote it: the same entries, weights, selected ledger and state.
// Nothing about which ledger is valid or selected changes, and every held
// entry is kept, at any depth. The node checks a snapshot's own checksum
// and that its log still holds what the snapshot covers before it uses
// one, and can always ignore it and replay its log instead (-reverify).

// snapshotVersion is the format of the records Snapshot writes.
const snapshotVersion = 1

// Record tags.
const (
	snapHeader = 'H'
	snapNode   = 'N'
	snapState  = 'S'
)

// statePerRecord is how many unspent portions one state record holds.
const statePerRecord = 4096

// portionSize is one unspent portion's encoded size.
const portionSize = hash.Size + 4 + 8 + hash.Size + 8

// ErrNotSnapshottable: some held entry's body is not in the pool's
// BodySource yet (its record is not written), or the pool has no source,
// so a restored pool could not read it back. The caller tries again
// later.
var ErrNotSnapshottable = errors.New("ledger: a held entry's body is not written yet, so the pool cannot be snapshotted now")

// ErrBadSnapshot wraps every reason Restore refuses a snapshot. The
// caller replays its log instead.
var ErrBadSnapshot = errors.New("ledger: snapshot not usable")

// snapshotOrder returns every node with each parent before its children:
// the selected ledger from the bottom, then the other entries by first
// sighting (a child is always seen after its parent).
func (p *Pool) snapshotOrder() []*node {
	out := make([]*node, 0, len(p.nodes))
	out = append(out, p.chain...)
	rest := make([]*node, 0, len(p.nodes)-len(p.chain))
	for _, n := range p.nodes {
		if !n.onChain {
			rest = append(rest, n)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].seq < rest[j].seq })
	return append(out, rest...)
}

// Snapshot writes the pool's skeleton and tip state as a sequence of
// records, each passed to emit, for Restore. It returns
// ErrNotSnapshottable when a held entry's body cannot be read back from
// the source yet. It changes nothing; the caller holds the engine's lock.
func (p *Pool) Snapshot(emit func([]byte) error) error {
	if p.cfg.Source == nil {
		return ErrNotSnapshottable
	}
	order := p.snapshotOrder()
	for _, n := range order {
		if !p.cfg.Source.Has(n.hash) {
			return ErrNotSnapshottable
		}
	}
	byNode := p.tidsByNode()
	var unspent []state.Holding
	p.st.ForEach(func(in tx.Input, po tx.Portion) {
		unspent = append(unspent, state.Holding{Input: in, Portion: po})
	})
	w := encoding.NewWriter(256)
	w.U8(snapHeader).U8(snapshotVersion).Raw(p.id[:]).U64(p.seq).
		U64(uint64(len(order))).U64(uint64(len(p.chain))).U64(uint64(len(unspent)))
	var newest hash.Hash
	if p.newest != nil && p.nodes[p.newest.hash] == p.newest {
		newest = p.newest.hash
	}
	w.Raw(newest[:])
	if err := emit(w.Bytes()); err != nil {
		return err
	}
	for _, n := range order {
		w := encoding.NewWriter(128 + 40*len(n.layers))
		ph := p.parentHash(n)
		w.U8(snapNode).Raw(n.hash[:]).Raw(ph[:]).U64(n.seq).U64(uint64(n.seen.UnixNano())).
			U32(uint32(n.nitems)).U32(uint32(n.nspent)).Bool(n.onChain)
		lhs := make([]hash.Hash, 0, len(n.layers))
		for lh := range n.layers {
			lhs = append(lhs, lh)
		}
		slices.SortFunc(lhs, func(a, b hash.Hash) int { return bytes.Compare(a[:], b[:]) })
		w.Count(len(lhs))
		for _, lh := range lhs {
			w.Raw(lh[:]).U64(n.layers[lh].Nonce)
		}
		w.Count(len(n.claims))
		for _, c := range n.claims {
			w.U64(c)
		}
		if n.onChain {
			tids := byNode[n]
			w.Count(len(tids))
			for _, t := range tids {
				w.Raw(t[:])
			}
		}
		if err := emit(w.Bytes()); err != nil {
			return err
		}
	}
	for i := 0; i < len(unspent); i += statePerRecord {
		part := unspent[i:min(len(unspent), i+statePerRecord)]
		w := encoding.NewWriter(5 + portionSize*len(part))
		w.U8(snapState).Count(len(part))
		for _, u := range part {
			w.Raw(u.Input.TID[:]).U32(u.Input.Index).U64(u.Portion.Serial).Raw(u.Portion.Address[:]).U64(u.Portion.Amount)
		}
		if err := emit(w.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

// tidsByNode returns the tids of each selected entry's transactions,
// sorted, from the tids index, which holds them for cold entries too.
// Restore needs the set (it is the index), not the order.
func (p *Pool) tidsByNode() map[*node][]hash.Hash {
	out := make(map[*node][]hash.Hash, len(p.chain))
	for tid, n := range p.tids {
		out[n] = append(out[n], tid)
	}
	for _, ts := range out {
		slices.SortFunc(ts, func(a, b hash.Hash) int { return bytes.Compare(a[:], b[:]) })
	}
	return out
}

// Restore builds a pool from the records Snapshot wrote, read one at a
// time from next (which returns nil at the end). cfg is the pool's
// configuration now; it must have a Source holding every entry's body.
// Recent entries of the selected ledger, and every entry off it, get
// their items read back from the source, and the selected ones their undo
// records recomputed (D175); older ones are left cold. Any inconsistency
// gives an error wrapping ErrBadSnapshot, and the pool is not used.
func Restore(s *series.Series, index uint64, cfg Config, next func() ([]byte, error)) (pl *Pool, err error) {
	if cfg.Source == nil {
		return nil, fmt.Errorf("%w: no body source", ErrBadSnapshot)
	}
	// A body that cannot be read back panics with a ReadFault inside
	// warm; here it only means the snapshot is not usable.
	defer func() {
		if v := recover(); v != nil {
			f, ok := v.(ReadFault)
			if !ok {
				panic(v)
			}
			pl, err = nil, fmt.Errorf("%w: %v", ErrBadSnapshot, f)
		}
	}()
	p := New(s, index, cfg)
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrBadSnapshot, fmt.Sprintf(format, a...))
	}
	rec, err := next()
	if err != nil {
		return nil, err
	}
	r := encoding.NewReader(rec)
	if r.U8() != snapHeader || r.U8() != snapshotVersion {
		return nil, bad("not a version %d pool snapshot", snapshotVersion)
	}
	var id, newest hash.Hash
	r.Into(id[:])
	p.seq = r.U64()
	nNodes, nChain, nUnspent := r.U64(), r.U64(), r.U64()
	r.Into(newest[:])
	if err := r.Finish(); err != nil {
		return nil, bad("header: %v", err)
	}
	if id != p.id {
		return nil, bad("snapshot of pool %s, not %s", id.Short(), p.id.Short())
	}
	if nChain > nNodes {
		return nil, bad("%d selected entries of %d", nChain, nNodes)
	}
	credit := s.EntryHashCredit()
	var order []*node
	for i := uint64(0); i < nNodes; i++ {
		rec, err := next()
		if err != nil {
			return nil, err
		}
		if rec == nil {
			return nil, bad("ends after %d of %d entries", i, nNodes)
		}
		r := encoding.NewReader(rec)
		if r.U8() != snapNode {
			return nil, bad("record %d is not an entry", i)
		}
		n := &node{layers: map[hash.Hash]layer.Layer{}}
		var ph hash.Hash
		r.Into(n.hash[:])
		r.Into(ph[:])
		n.seq = r.U64()
		n.seen = time.Unix(0, int64(r.U64()))
		n.nitems, n.nspent = int(r.U32()), int(r.U32())
		n.onChain = r.Bool()
		if credit {
			n.own = weight.Credit(n.hash.Strength())
		}
		for range r.Count(0, hash.Size+8) {
			var lh hash.Hash
			r.Into(lh[:])
			l := layer.Layer{Entry: n.hash, Nonce: r.U64()}
			if r.Err() != nil {
				break
			}
			if _, dup := n.layers[lh]; dup || lh.Strength() < s.MinLayerStrength() {
				r.Fail(fmt.Errorf("layer %s repeated or too weak", lh.Short()))
				break
			}
			n.layers[lh] = l
			n.own = n.own.Add(weight.Credit(lh.Strength()))
		}
		for range r.Count(0, 8) {
			n.claims = append(n.claims, r.U64())
		}
		var tids []hash.Hash
		if n.onChain {
			for range r.Count(0, hash.Size) {
				var t hash.Hash
				r.Into(t[:])
				tids = append(tids, t)
			}
			if r.Err() == nil && len(tids) != n.nitems {
				r.Fail(fmt.Errorf("%d tids for %d transactions", len(tids), n.nitems))
			}
		}
		if err := r.Finish(); err != nil {
			return nil, bad("entry %d: %v", i, err)
		}
		if _, dup := p.nodes[n.hash]; dup {
			return nil, bad("entry %s twice", n.hash.Short())
		}
		if ph != p.id {
			parent, ok := p.nodes[ph]
			if !ok {
				return nil, bad("entry %s comes before its parent", n.hash.Short())
			}
			n.parent = parent
			n.height = parent.height + 1
			parent.children = append(parent.children, n)
		} else {
			n.height = 1
			p.roots = append(p.roots, n)
		}
		if n.seq == 0 || n.seq > p.seq {
			return nil, bad("entry %s seen at %d of %d", n.hash.Short(), n.seq, p.seq)
		}
		if !cfg.Source.Has(n.hash) {
			return nil, bad("entry %s has no body in the store", n.hash.Short())
		}
		p.nodes[n.hash] = n
		order = append(order, n)
		if uint64(len(p.chain)) < nChain {
			if !n.onChain || n.height != len(p.chain)+1 {
				return nil, bad("selected ledger broken at height %d", len(p.chain)+1)
			}
			p.chain = append(p.chain, n)
			p.units = append(p.units, p.units[len(p.units)-1]+unitsOf(n))
			p.bit.push(n.own)
			for _, t := range tids {
				if _, dup := p.tids[t]; dup {
					return nil, bad("tid %s in two selected entries", t.Short())
				}
				p.tids[t] = n
			}
		} else if n.onChain {
			return nil, bad("entry %s marked selected above the selected tip", n.hash.Short())
		}
	}
	// The tip state: the unspent portions as written, and the mints the
	// selected entries claim.
	var got uint64
	for got < nUnspent {
		rec, err := next()
		if err != nil {
			return nil, err
		}
		if rec == nil {
			return nil, bad("ends after %d of %d unspent portions", got, nUnspent)
		}
		r := encoding.NewReader(rec)
		if r.U8() != snapState {
			return nil, bad("record is not state")
		}
		c := r.Count(statePerRecord, portionSize)
		for range c {
			var in tx.Input
			var po tx.Portion
			r.Into(in.TID[:])
			in.Index = r.U32()
			po.Serial = r.U64()
			r.Into(po.Address[:])
			po.Amount = r.U64()
			p.st.Restore(in, po)
		}
		if err := r.Finish(); err != nil {
			return nil, bad("state: %v", err)
		}
		got += uint64(c)
	}
	if got != nUnspent || uint64(p.st.Len()) != nUnspent {
		return nil, bad("%d unspent portions, header says %d", p.st.Len(), nUnspent)
	}
	if rec, err := next(); err != nil {
		return nil, err
	} else if rec != nil {
		return nil, bad("records after the end")
	}
	for _, n := range p.chain {
		for _, c := range n.claims {
			p.st.SetClaimed(c)
		}
	}
	// Off-chain bookkeeping: anchors, relative weights, leaves.
	for _, n := range order {
		if n.onChain {
			continue
		}
		p.off[n] = struct{}{}
		if len(n.children) == 0 {
			p.leaves[n] = struct{}{}
		}
		switch {
		case n.parent == nil:
			n.rel, n.anchor = n.own, 0
		case n.parent.onChain:
			n.rel, n.anchor = n.own, n.parent.height
		default:
			n.rel, n.anchor = n.parent.rel.Add(n.own), n.parent.anchor
		}
	}
	if newest != (hash.Hash{}) {
		if p.newest = p.nodes[newest]; p.newest == nil {
			return nil, bad("newest entry %s not held", newest.Short())
		}
	}
	// Items and undo: the selected entries below the recent window stay
	// cold; the rest are read back (D175). With no window every one is.
	cold := 0
	if w := cfg.RecentWindow; w > 0 {
		cold = max(0, len(p.chain)-w)
	}
	for _, n := range p.chain {
		n.cold = true
	}
	p.warm(p.chain[cold:])
	p.cooled = cold
	for _, n := range p.chain[cold:] {
		if len(n.items) != n.nitems {
			return nil, bad("entry %s reads back %d transactions, not %d", n.hash.Short(), len(n.items), n.nitems)
		}
		for _, it := range n.items {
			if p.tids[it.TID] != n {
				return nil, bad("entry %s's transaction %s is not indexed to it", n.hash.Short(), it.TID.Short())
			}
		}
	}
	for _, n := range order {
		if n.onChain {
			continue
		}
		items, err := p.loadItems(n)
		if err != nil {
			return nil, bad("entry %s: %v", n.hash.Short(), err)
		}
		n.items = items
	}
	return p, nil
}

// Renumber gives the held entries the first-sighting order a replay of
// ReplayOrder gives them: the selected ledger first, then the others as
// they were. A node calls it when it rewrites its log in that order, so
// the pool it runs is the pool a restart of the new log builds.
func (p *Pool) Renumber() {
	order := p.snapshotOrder()
	for i, n := range order {
		n.seq = uint64(i + 1)
	}
	p.seq = uint64(len(order))
}

// Fingerprint returns a digest of everything about the pool that
// selection, checking and serving read: every held entry with its parent,
// layers, counts, claims and whether it is selected, in first-sighting
// order (the order ties are broken by, not the numbers), the selected
// ledger's tids, and the unspent portions at the tip. Two pools with the
// same fingerprint decide the same way on every item. It leaves out what
// is local and decides nothing: when each entry was seen, which bodies
// are in memory, and which entries are cold. For tests and -reverify.
func (p *Pool) Fingerprint() hash.Hash {
	all := make([]*node, 0, len(p.nodes))
	for _, n := range p.nodes {
		all = append(all, n)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })
	h := sha256.New()
	w := encoding.NewWriter(256)
	put := func() {
		h.Write(w.Bytes())
		w.Reset()
	}
	w.Raw(p.id[:]).U64(uint64(len(all))).U64(uint64(len(p.chain)))
	put()
	for _, n := range all {
		ph := p.parentHash(n)
		w.Raw(n.hash[:]).Raw(ph[:]).Bool(n.onChain).VarBytes(n.own.Bytes()).U32(uint32(n.nitems)).U32(uint32(n.nspent))
		lhs := make([]hash.Hash, 0, len(n.layers))
		for lh := range n.layers {
			lhs = append(lhs, lh)
		}
		slices.SortFunc(lhs, func(a, b hash.Hash) int { return bytes.Compare(a[:], b[:]) })
		w.Count(len(lhs))
		for _, lh := range lhs {
			w.Raw(lh[:])
		}
		w.Count(len(n.claims))
		for _, c := range n.claims {
			w.U64(c)
		}
		put()
	}
	byNode := p.tidsByNode()
	for _, n := range p.chain {
		w.Raw(n.hash[:])
		tids := byNode[n]
		w.Count(len(tids))
		for _, t := range tids {
			w.Raw(t[:])
		}
		put()
	}
	var unspent []state.Holding
	p.st.ForEach(func(in tx.Input, po tx.Portion) {
		unspent = append(unspent, state.Holding{Input: in, Portion: po})
	})
	sort.Slice(unspent, func(i, j int) bool {
		a, b := unspent[i].Input, unspent[j].Input
		if c := bytes.Compare(a.TID[:], b.TID[:]); c != 0 {
			return c < 0
		}
		return a.Index < b.Index
	})
	w.U64(uint64(len(unspent)))
	for _, u := range unspent {
		w.Raw(u.Input.TID[:]).U32(u.Input.Index).U64(u.Portion.Serial).Raw(u.Portion.Address[:]).U64(u.Portion.Amount)
	}
	put()
	var out hash.Hash
	copy(out[:], h.Sum(nil))
	return out
}
