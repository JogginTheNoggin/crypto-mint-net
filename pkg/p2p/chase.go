package p2p

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/chase"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// offer is step 0 of a chase: a stated weight above the node's own goes to
// the chase queue, grouped with others that name the same branch. A sender
// has one open stated weight per pool, and the ones that start something
// new count against its budget.
func (h *Host) offer(pi *poolInfo, p *peer, stated weight.Weight, at *hash.Hash) {
	if !stated.Greater(h.ownWeight(pi)) {
		return
	}
	now := time.Now()
	text := pi.section.Text()
	h.tr.Stated(p.key, text, stated, now)
	s := chase.Sender{ID: p.id, Range: remoteRange(p), Cat: p.catFor(pi.section), Stated: stated}
	id, isNew, ok := h.cq.Offer(text, s, at, now)
	if !ok {
		return
	}
	if isNew {
		if within, act := h.tr.StartChase(p.key, now); !within {
			h.cq.Leave(id, p.id)
			h.act(p, act)
			return
		}
	}
	h.kick()
}

// kick wakes the chase loop.
func (h *Host) kick() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// chaseLoop starts the next chases whenever the queue has room, within
// the chase share the queue's limits set.
func (h *Host) chaseLoop(ctx context.Context) {
	defer h.wg.Done()
	for {
		for ctx.Err() == nil {
			j, ok := h.cq.Next(time.Now())
			if !ok {
				break
			}
			h.spawn(func() {
				h.runJob(j)
			})
		}
		wait := h.cq.Wait(time.Now())
		if wait <= 0 || wait > time.Second {
			wait = time.Second
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-h.wake:
		case <-t.C:
		}
		t.Stop()
	}
}

func (h *Host) peerByID(id string) *peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.peers[id]
}

// result is how one chase ended.
type result struct {
	out chase.Outcome
	bad []hash.Hash // entry hashes shown to be false
	why string
}

func nowhere(format string, args ...any) result {
	return result{out: chase.Nowhere, why: fmt.Sprintf(format, args...)}
}

func falseBranch(bad []hash.Hash, format string, args ...any) result {
	return result{out: chase.False, bad: bad, why: fmt.Sprintf(format, args...)}
}

// runJob runs one chase and records how it ended: in the queue (backoff,
// known-bad list), in the sender's measures, and as a flag when the branch
// was provably false.
func (h *Host) runJob(j chase.Job) {
	defer h.kick()
	pi := h.byName[j.Group.Pool]
	p := h.peerByID(j.Sender.ID)
	if pi == nil || p == nil || p.closed() || h.ctx.Err() != nil {
		h.cq.Finish(j.Group.ID, chase.Gone, nil, time.Now())
		return
	}
	res := h.runChase(pi, p, j)
	if p.closed() && res.out == chase.Nowhere {
		res.out = chase.Gone
	}
	now := time.Now()
	g := h.cq.Finish(j.Group.ID, res.out, res.bad, now)
	text := pi.section.Text()
	switch res.out {
	case chase.Switched:
		h.tr.ChaseOutcome(p.key, text, true, now)
		// Priority peers that said they saw nothing heavier were wrong.
		for _, id := range g.Denied {
			if d := h.peerByID(id); d != nil {
				h.tr.Denial(d.key, text, now)
			}
		}
	case chase.Nowhere:
		h.tr.ChaseOutcome(p.key, text, false, now)
	case chase.False:
		h.tr.ChaseOutcome(p.key, text, false, now)
		h.act(p, h.tr.Offend(p.key, tracking.FalseBranch, res.why, now))
	case chase.Yield:
		// Not an outcome: the chase goes on later with what it took in.
	}
	if res.why != "" {
		h.log.Info("chase", "pool", pi.key.String(), "peer", p.short(), "result", res.out, "why", res.why)
	}
}

// runChase runs the steps of one chase with one sender, from cheap to
// costly: confirming with a priority peer, finding the common entry,
// checking the work after it with provisional layers, and only then
// fetching the entries in chain order. The node switches only through the
// engine, on weight it computed itself. Nothing here refuses a heavier
// ledger for good: there is no depth limit and no limit on its length,
// and a chase that runs past its time slice yields and goes on later with
// what it took in kept (consensus audit P2 to P4, D116).
func (h *Host) runChase(pi *poolInfo, p *peer, j chase.Job) result {
	before := h.ownWeight(pi)
	if !j.Sender.Stated.Greater(before) {
		return result{out: chase.Caught}
	}
	deadline := time.Now().Add(h.cfg.ChaseSlice)
	// Step 1: a group whose best sender is not a priority peer is put to
	// one priority peer first.
	if !h.cfg.NoConfirm && j.Sender.Cat != peers.CatPriority && !j.Group.Asked {
		if q := h.askPeerIn(pi.section, peers.CatPriority, p); q != nil {
			heavier, stated := h.confirm(pi, q)
			h.cq.Confirm(j.Group.ID, chase.Sender{ID: q.id, Range: remoteRange(q), Cat: peers.CatPriority, Stated: stated}, heavier)
			if heavier {
				return result{out: chase.Requeue}
			}
			if !j.Sender.Stated.Greater(h.ownWeight(pi)) {
				return result{out: chase.Caught}
			}
		}
	}
	if _, height, _ := h.tip(pi); height == 0 {
		// Nothing held: there is nothing to beat, and the whole ledger is
		// the branch.
		k := progressKey{j.Group.ID, p.id}
		st := h.takeProgress(k)
		if st == nil {
			st = newProgress(pi.s.PoolID(pi.key.Index), nil, false)
		}
		// The sender shows some work before any entry is taken in: any
		// heavier ledger has some, against this node's none (RED-016,
		// decision D120). It is asked for, never refused.
		res := result{}
		done := false
		if !st.worked {
			res, done = h.checkWork(pi, p, st, deadline)
			if !done {
				h.cq.WorkChecked(j.Group.ID)
			}
		}
		if !done {
			res = h.fetchAll(pi, p, st, before, deadline)
		}
		if res.out == chase.Yield {
			h.saveProgress(k, st)
		} else {
			st.drop()
		}
		return res
	}
	// Step 3: the common entry, unless a chase that yielded found it.
	k := progressKey{j.Group.ID, p.id}
	st := h.takeProgress(k)
	if st != nil && !h.holdsSelected(pi, st.common) {
		st.drop()
		st = nil // this node moved off it meanwhile: start again
	}
	if st == nil {
		common, after, complete, page, pageStart, res, done := h.findCommon(pi, p)
		if done {
			return res
		}
		h.cq.AddHashes(j.Group.ID, after)
		if complete && len(after) == 0 {
			// The sender's newest hash is on this node's selected ledger:
			// it holds those entries with more layers.
			st = newProgress(common, nil, true)
			st.topping = true
			st.top, st.topAt, st.topStart = page, page[0], pageStart
		}
		if st == nil {
			st = newProgress(common, after, complete)
		}
	}
	var res result
	if st.topping {
		done := false
		if res, done = h.topUpLayers(pi, p, st, j.Sender.Stated, deadline); !done {
			res = h.outcome(pi, before)
		}
	} else {
		res = h.stepsFrom(pi, p, j, st, before, deadline)
	}
	if res.out == chase.Yield {
		h.saveProgress(k, st)
	} else {
		st.drop()
	}
	return res
}

// stepsFrom runs steps 4 and 5 from what st holds.
func (h *Host) stepsFrom(pi *poolInfo, p *peer, j chase.Job, st *progress, before weight.Weight, deadline time.Time) result {
	for {
		// Step 4: the work behind the branch, before any entry.
		if !st.worked {
			if res, done := h.checkWork(pi, p, st, deadline); done {
				return res
			}
			h.cq.WorkChecked(j.Group.ID)
		}
		// Step 5: the entries, checked in chain order against the hashes
		// the sender listed. It hands back to step 4 when this node's own
		// ledger outgrew the work counted (C-1).
		res, done := h.fetchBranch(pi, p, st, deadline)
		if !done {
			return h.outcome(pi, before)
		}
		if st.worked {
			return res
		}
	}
}

// holdsSelected reports whether eh is the pool_id or an entry of the
// node's selected ledger.
func (h *Host) holdsSelected(pi *poolInfo, eh hash.Hash) bool {
	if eh == pi.s.PoolID(pi.key.Index) {
		return true
	}
	ok := false
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) { _, ok = l.HeightOf(eh) })
	return ok
}

// outcome reports a switch if the node's computed weight grew.
func (h *Host) outcome(pi *poolInfo, before weight.Weight) result {
	if h.ownWeight(pi).Greater(before) {
		return result{out: chase.Switched}
	}
	return nowhere("not heavier once checked")
}

// chaseErr turns an error during a chase into its outcome.
func chaseErr(err error) result {
	var f falseErr
	switch {
	case errors.As(err, &f):
		return falseBranch(f.bad, "%v", f.err)
	case errors.Is(err, errClosed):
		return result{out: chase.Gone}
	case errors.Is(err, errRevertBudget):
		// Not refused: it goes on once the budget has room again (P2).
		return result{out: chase.Yield, why: "the revert budget is spent for now"}
	case errors.Is(err, errPartsYield):
		// A large entry is fetched over several slices, from where the
		// last one stopped (RED-016).
		return result{out: chase.Yield, why: "time slice used up fetching a large entry"}
	}
	return nowhere("%v", err)
}

// falseErr is data in a chase that breaks the rules or contradicts itself.
type falseErr struct {
	bad []hash.Hash
	err error
}

func (e falseErr) Error() string { return e.err.Error() }

// confirm asks a priority peer for an extension from this node's latest
// entry: an extension or "not from here" means it sees something heavier.
// Whatever entries it sends are taken in like any requested data.
func (h *Host) confirm(pi *poolInfo, q *peer) (bool, weight.Weight) {
	tip, _, _ := h.tip(pi)
	f, err := h.fetch(pi, q, wire.ExtensionReq{RequestCommon: h.common(), From: tip, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}, nil, nil, nil, true, time.Time{})
	if err != nil {
		return false, weight.Zero
	}
	return f.kind == wire.TypeExtension || f.kind == wire.TypeNotFromHere, f.stated
}

// askPeerIn returns the best peer in category c for s, other than skip.
func (h *Host) askPeerIn(s wire.Section, c peers.Category, skip *peer) *peer {
	for _, p := range h.peersIn(s, c) {
		if p != skip {
			return p
		}
	}
	return nil
}

// askWeight asks p for its stated weight for pi; a heavier one is offered
// to the chase queue.
func (h *Host) askWeight(pi *poolInfo, p *peer) {
	h.ask(p, pi, wire.WeightReq{RequestCommon: h.common()}, func(m wire.Message) error {
		if m.Type == wire.TypeWeight {
			h.checkStated(pi, p, m.Stated, nil)
		}
		return nil
	})
}

// tip reads the node's active ledger for pi.
func (h *Host) tip(pi *poolInfo) (hash.Hash, int, weight.Weight) {
	var t hash.Hash
	var ht int
	var w weight.Weight
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) { t, ht, w = l.Tip() })
	return t, ht, w
}

// hashPage asks p for one page of hash chain.
func (h *Host) hashPage(pi *poolInfo, p *peer, q wire.HashChainReq) (hs []hash.Hash, fromStart, more bool, kind wire.Type, err error) {
	kind = wire.TypeHashChain
	err = h.ask(p, pi, q, func(m wire.Message) error {
		kind = m.Type
		if m.Type != wire.TypeHashChain {
			return wire.DecodeEmpty(m.Type, m.Body)
		}
		c, err := wire.DecodeHashChainChunk(m.Body)
		if err != nil {
			return err
		}
		if m.Chunk == 1 {
			fromStart = c.FromStart
		}
		more = m.More()
		hs = append(hs, c.Hashes...)
		return nil
	})
	if errors.Is(err, wire.ErrMalformed) {
		h.misbehave(p, err)
	}
	return hs, fromStart, more, kind, err
}

// findCommon is step 3. It asks p for its newest hashes; if one is held
// here, that is the common entry and the hashes after it are the whole
// branch (complete). Otherwise the split is deeper, and it finds the
// common entry by halving over this node's selected ledger: p is asked
// whether it holds the entry at a height, which takes about log2(height)
// requests at any depth, anchored only on entries this node holds, so a
// sender cannot make the search longer with made-up hashes (P4). The
// hashes after the common entry are then paged by checkWork. There is no
// depth limit (the owner, G3; D116).
func (h *Host) findCommon(pi *poolInfo, p *peer) (common hash.Hash, after []hash.Hash, complete bool, page []hash.Hash, pageStart bool, res result, done bool) {
	poolID := pi.s.PoolID(pi.key.Index)
	hs, fromStart, _, kind, err := h.hashPage(pi, p, wire.HashChainReq{RequestCommon: h.common(), Count: h.cfg.Limits.HashLimit})
	if err != nil {
		return common, nil, false, hs, fromStart, chaseErr(err), true
	}
	if kind != wire.TypeHashChain {
		return common, nil, false, hs, fromStart, nowhere("hash chain answered %s", kind), true
	}
	if len(hs) == 0 {
		return common, nil, false, hs, fromStart, nowhere("the sender holds no ledger"), true
	}
	if dup := firstRepeat(hs, nil); dup != nil {
		// A lie about the order, which says nothing against the entry with
		// that hash: nothing goes on the known-bad list (P5).
		return common, nil, false, hs, fromStart, falseBranch(nil, "hash %s listed twice", dup.Short()), true
	}
	found := -1
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
		for i := len(hs) - 1; i >= 0; i-- {
			if l.Has(hs[i]) {
				found = i
				return
			}
		}
	})
	if found >= 0 {
		// The newest hash this node holds. On its selected ledger it is
		// the common entry. Off it, the node holds that branch already,
		// perhaps without the layers that make it heavier: the common
		// entry is where the branch leaves the selected ledger, found by
		// walking back through entries held here, with no request, and
		// the branch's held hashes go first in the listing (Black's B1,
		// D121).
		at := hs[found]
		var back []hash.Hash
		h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
			for at != poolID {
				if _, ok := l.HeightOf(at); ok {
					return
				}
				e, ok := l.Entry(at)
				if !ok {
					return
				}
				back = append(back, at)
				at = e.Parent
			}
		})
		slices.Reverse(back)
		if len(back) > 0 {
			return at, append(back, hs[found+1:]...), true, hs, fromStart, result{}, false
		}
		return hs[found], hs[found+1:], true, hs, fromStart, result{}, false
	}
	if fromStart {
		return poolID, hs, true, hs, fromStart, result{}, false
	}
	// Halve: lo is a height p holds (0, the pool_id, always), hi one it
	// does not or that is past this node's tip.
	_, height, _ := h.tip(pi)
	lo, hi := 0, height+1
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		var at hash.Hash
		ok := false
		h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
			if mid <= l.Height() {
				at, ok = l.Chain(mid, mid)[0], true
			}
		})
		if !ok {
			hi = mid
			continue
		}
		_, _, _, kind, err := h.hashPage(pi, p, wire.HashChainReq{RequestCommon: h.common(), HasAnchor: true, Anchor: at, Dir: wire.After, Count: 1})
		switch {
		case err != nil:
			return common, nil, false, hs, fromStart, chaseErr(err), true
		case kind == wire.TypeHashChain:
			lo, common = mid, at
		case kind == wire.TypeAnchorNotHeld:
			hi = mid
		default:
			return common, nil, false, hs, fromStart, nowhere("hash chain answered %s", kind), true
		}
	}
	if lo == 0 {
		common = poolID
	}
	return common, nil, false, hs, fromStart, result{}, false
}

// firstRepeat returns a hash of hs listed twice, or listed in seen.
func firstRepeat(hs []hash.Hash, seen map[hash.Hash]bool) *hash.Hash {
	if seen == nil {
		seen = map[hash.Hash]bool{}
	}
	for i, x := range hs {
		if seen[x] {
			return &hs[i]
		}
		seen[x] = true
	}
	return nil
}

// progress is what a chase has learned about one sender's branch. A
// chase that yields leaves it with the host, and the next chase of the
// same group with the same sender goes on from it, so a long branch is
// reached over several time slices instead of starting again each time
// (consensus audit P2 to P4, D116). It holds only what work backs: the
// hashes still to check (a page at most), the hashes that carried layers
// and the entries taken in, never every hash listed, so a sender listing
// hashes with no work makes the node hold nothing more (RED-015, D120).
type progress struct {
	common   hash.Hash
	pending  []hash.Hash // listed, layers not counted yet, in order
	vetted   bool        // pending has passed list's checks
	listed   int         // hashes listed after common so far
	last     hash.Hash   // the last one listed: the next page goes on from it
	complete bool        // the listing reaches the sender's tip

	// Step 4.
	total   weight.Weight // provisional work counted so far
	counted map[hash.Hash]bool
	marks   map[int]hash.Hash // listed hashes with layers, by index after common
	worked  bool              // total beat the node's own weight after common

	// Step 5.
	from   hash.Hash
	idx    int
	branch map[hash.Hash]bool // marked and fetched hashes
	part   *partial           // a large entry, as far as it has arrived

	// Topping up layers on entries both hold (topUpLayers).
	topping  bool
	top      []hash.Hash // the sender's hashes whose layers to ask for next
	topAt    hash.Hash   // the oldest of them, the anchor for the page before
	topStart bool        // top reaches the sender's first entry
}

func newProgress(common hash.Hash, after []hash.Hash, complete bool) *progress {
	st := &progress{common: common, pending: after, listed: len(after), complete: complete, from: common,
		counted: map[hash.Hash]bool{}, marks: map[int]hash.Hash{}, branch: map[hash.Hash]bool{}}
	if len(after) > 0 {
		st.last = after[len(after)-1]
	}
	return st
}

// want is the hash listed at index i after common, when it carried work.
func (st *progress) want(i int) (hash.Hash, bool) {
	h, ok := st.marks[i]
	return h, ok
}

// drop lets go of what st holds outside memory.
func (st *progress) drop() {
	if st != nil && st.part != nil {
		st.part.close()
		st.part = nil
	}
}

// progressKey names a chase of one group with one sender.
type progressKey struct {
	group  uint64
	sender string
}

// maxProgress is how many yielded chases the host remembers; past it the
// oldest is forgotten and starts again when its turn comes.
const maxProgress = 256

func (h *Host) saveProgress(k progressKey, st *progress) {
	h.progMu.Lock()
	defer h.progMu.Unlock()
	if h.prog == nil {
		h.prog = map[progressKey]*progress{}
	}
	if _, ok := h.prog[k]; !ok {
		h.progOrder = append(h.progOrder, k)
	}
	h.prog[k] = st
	for len(h.progOrder) > maxProgress {
		// Forget the chase with the least shown: one whose work check has
		// not passed before one whose has, then the least work counted,
		// then the oldest. A worked chase is never forgotten for one that
		// is not (Black's C-2, D121).
		v := 0
		for i, o := range h.progOrder {
			if lessShown(h.prog[o], h.prog[h.progOrder[v]]) {
				v = i
			}
		}
		h.prog[h.progOrder[v]].drop()
		delete(h.prog, h.progOrder[v])
		h.progOrder = append(h.progOrder[:v], h.progOrder[v+1:]...)
	}
}

// lessShown reports whether a has shown less than b: not worked where b
// has, or less work counted.
func lessShown(a, b *progress) bool {
	if a.worked != b.worked {
		return !a.worked
	}
	return b.total.Greater(a.total)
}

// takeProgress returns and forgets what a yielded chase left, or nil.
func (h *Host) takeProgress(k progressKey) *progress {
	h.progMu.Lock()
	defer h.progMu.Unlock()
	st := h.prog[k]
	if st != nil {
		delete(h.prog, k)
		for i, o := range h.progOrder {
			if o == k {
				h.progOrder = append(h.progOrder[:i], h.progOrder[i+1:]...)
				break
			}
		}
	}
	return st
}

// checkWork is step 4: it asks p for the layers on the hashes after the
// common entry and checks them provisionally, until they add up to more
// than the weight to beat. When the listing is not complete it pages p's
// hashes on from the common entry as it goes. The layers are never kept,
// counted towards a ledger or forwarded. done is set when the chase ends
// here. There is no cap on the layers looked at, nor on the hashes: real
// work is what a heavier ledger is made of, and it may sit on any of its
// entries. A hash with no layers is let go once checked, and after
// WeightlessPages pages in a row that bring no work the chase yields, to
// go on later from where it stopped: a listing with no work is paced,
// never refused (RED-015, decision D120). Past the deadline it yields too,
// but only once it has made one request, so every slice makes progress.
func (h *Host) checkWork(pi *poolInfo, p *peer, st *progress, deadline time.Time) (result, bool) {
	var beat weight.Weight
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
		to, _ := l.WeightTo(st.common)
		beat = l.Weight().Sub(to)
	})
	fn := pi.s.Hash()
	minLayer, minEntry := pi.s.MinLayerStrength(), pi.s.MinEntryStrength()
	// list vets listed hashes: each must be able to be an entry's.
	list := func(hs []hash.Hash) (result, bool) {
		if dup := firstRepeat(hs, nil); dup != nil {
			return falseBranch(nil, "hash %s listed twice", dup.Short()), true
		}
		for _, eh := range hs {
			if eh.Strength() < minEntry {
				// No valid entry has this hash: it is shown false itself.
				return falseBranch([]hash.Hash{eh}, "entry hash %s below the minimum strength", eh.Short()), true
			}
			if pi.s.EntryHashCredit() {
				st.total = st.total.Add(weight.Credit(eh.Strength()))
			}
		}
		return result{}, false
	}
	if !st.vetted {
		if res, done := list(st.pending); done {
			return res, true
		}
		st.vetted = true
	}
	batch := int(h.cfg.Limits.HashLimit)
	asked := false
	bare := 0 // pages in a row that brought no work
	for !st.total.Greater(beat) {
		if asked && time.Now().After(deadline) {
			return result{out: chase.Yield, why: "time slice used up checking the work"}, true
		}
		if bare >= h.cfg.WeightlessPages {
			return result{out: chase.Yield, why: fmt.Sprintf("%d pages of hashes with no work: paced", bare)}, true
		}
		if h.paced(p) {
			return result{out: chase.Yield, why: "requests to the sender paced"}, true
		}
		asked = true
		if len(st.pending) == 0 {
			if st.complete {
				break
			}
			// The first page after the pool_id is the sender's first
			// hashes (no anchor and no count); every other page goes on
			// after an anchor.
			q := wire.HashChainReq{RequestCommon: h.common()}
			if st.listed > 0 || st.common != pi.s.PoolID(pi.key.Index) {
				anchor := st.common
				if st.listed > 0 {
					anchor = st.last
				}
				q.HasAnchor, q.Anchor, q.Dir, q.Count = true, anchor, wire.After, h.cfg.Limits.HashLimit
			}
			hs, _, more, kind, err := h.hashPage(pi, p, q)
			switch {
			case err != nil:
				return chaseErr(err), true
			case kind == wire.TypeAnchorNotHeld:
				return nowhere("the sender no longer holds %s", q.Anchor.Short()), true
			case kind != wire.TypeHashChain:
				return nowhere("hash chain answered %s", kind), true
			}
			before := st.total
			if res, done := list(hs); done {
				return res, true
			}
			if st.total.Greater(before) {
				bare = 0
			}
			st.pending = hs
			st.listed += len(hs)
			if len(hs) > 0 {
				st.last = hs[len(hs)-1]
			}
			st.complete = !more || len(hs) == 0
			continue
		}
		end := len(st.pending)
		if batch > 0 {
			end = min(batch, end)
		}
		page := st.pending[:end]
		base := st.listed - len(st.pending)
		at := make(map[hash.Hash]int, len(page))
		for i, eh := range page {
			at[eh] = base + i
		}
		items, err := h.askLayers(pi, p, page)
		if err != nil {
			return chaseErr(err), true
		}
		if len(items) == 0 {
			return nowhere("no layers sent"), true
		}
		before := st.total
		for _, it := range items {
			i, ok := at[it.Entry]
			if !ok {
				continue
			}
			if !it.Held {
				return nowhere("the sender no longer holds %s", it.Entry.Short()), true
			}
			for _, n := range it.Nonces {
				lh := layer.Layer{Entry: it.Entry, Nonce: n}.Hash(fn)
				if lh.Strength() < minLayer {
					// The layer is false, not the entry under it (P5).
					return falseBranch(nil, "layer on %s below the minimum strength", it.Entry.Short()), true
				}
				if st.counted[lh] {
					continue
				}
				st.counted[lh] = true
				st.total = st.total.Add(weight.Credit(lh.Strength()))
				st.marks[i] = it.Entry
				st.branch[it.Entry] = true
			}
		}
		if st.total.Greater(before) {
			bare = 0
		} else {
			bare++
		}
		st.pending = st.pending[min(len(items), end):]
		if len(st.pending) == 0 {
			st.pending = nil
		}
	}
	if !st.total.Greater(beat) {
		return nowhere("provisional work %s does not beat %s", st.total, beat), true
	}
	st.worked = true
	return result{}, false
}

// askLayers asks p for the layers on hashes. An entry with more layers
// than a reply carries whole comes as a many-layers reply, and its layers
// are then paged with layers-from requests until all have come, so no
// entry holds too many layers to be counted (D121). It returns one item
// per entry answered, in order; with many layers, only that entry.
func (h *Host) askLayers(pi *poolInfo, p *peer, hashes []hash.Hash) ([]wire.HeldLayers, error) {
	var items []wire.HeldLayers
	var many *wire.ManyLayers
	err := h.ask(p, pi, wire.LayersReq{RequestCommon: h.common(), Hashes: hashes}, func(m wire.Message) error {
		switch m.Type {
		case wire.TypeLayers:
			c, err := wire.DecodeLayersChunk(m.Body)
			if err != nil {
				return err
			}
			items = append(items, c.Items...)
			return nil
		case wire.TypeManyLayers:
			ml, err := wire.DecodeManyLayers(m.Body)
			if err != nil {
				return err
			}
			if len(hashes) == 0 || ml.Entry != hashes[0] {
				return falseErr{err: fmt.Errorf("many layers named %s, not the first hash asked about", ml.Entry.Short())}
			}
			many = &ml
			return nil
		}
		return wire.DecodeEmpty(m.Type, m.Body)
	})
	if err == nil && many != nil {
		var ns []uint64
		ns, err = h.layersFrom(pi, p, many.Entry)
		items = []wire.HeldLayers{{Entry: many.Entry, Held: err == nil, Nonces: ns}}
	}
	if errors.Is(err, wire.ErrMalformed) {
		h.misbehave(p, err)
	}
	return items, err
}

// layersFrom pages every layer nonce of one entry from p, strongest
// first. Each page must start where the last ended; a total that changes
// is followed, since layers keep arriving (D121).
func (h *Host) layersFrom(pi *poolInfo, p *peer, eh hash.Hash) ([]uint64, error) {
	var ns []uint64
	for {
		from := uint32(len(ns))
		more, got := false, false
		var kind wire.Type
		err := h.ask(p, pi, wire.LayersFromReq{RequestCommon: h.common(), Entry: eh, From: from}, func(m wire.Message) error {
			kind, more = m.Type, m.More()
			if m.Type != wire.TypeLayersFrom {
				return wire.DecodeEmpty(m.Type, m.Body)
			}
			c, err := wire.DecodeLayersFrom(m.Body)
			if err != nil {
				return err
			}
			if c.From != uint32(len(ns)) {
				return falseErr{err: fmt.Errorf("layers of %s from %d, want %d", eh.Short(), c.From, len(ns))}
			}
			got = got || len(c.Nonces) > 0
			ns = append(ns, c.Nonces...)
			return nil
		})
		if err != nil {
			return nil, err
		}
		if kind != wire.TypeLayersFrom {
			return nil, fmt.Errorf("layers of %s answered %s", eh.Short(), kind)
		}
		if !more {
			return ns, nil
		}
		if !got {
			return nil, falseErr{err: fmt.Errorf("layers of %s: more said, none sent", eh.Short())}
		}
	}
}

// fetchBranch is step 5: extensions from the common entry, each entry
// checked against the hash the sender listed for it. Its entries are not
// charged to the revert budget, since the sender has shown real work
// greater than this node's ledger after the fork (P2). Before each
// request it checks the branch is still worth it: once a ledger off the
// branch and heavier than its provisional work has been selected
// meanwhile, the chase ends as not heavier (P3). Like checkWork it
// yields past the deadline once it has made a request.
func (h *Host) fetchBranch(pi *poolInfo, p *peer, st *progress, deadline time.Time) (result, bool) {
	asked := false
	for {
		if asked && time.Now().After(deadline) {
			return result{out: chase.Yield, why: "time slice used up fetching"}, true
		}
		if h.paced(p) {
			return result{out: chase.Yield, why: "requests to the sender paced"}, true
		}
		asked = true
		var beat weight.Weight
		onBranch := false
		h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
			tip, _, w := l.Tip()
			onBranch = st.branch[tip]
			to, _ := l.WeightTo(st.common)
			beat = w.Sub(to)
		})
		if !onBranch && !st.total.Greater(beat) {
			// This node's own ledger grew past the work counted so far,
			// which beat it by as little as one page. That says nothing
			// against the rest of the branch: count more (Black's C-1,
			// D121). Only a branch counted to its end can be not heavier.
			if st.complete && len(st.pending) == 0 {
				return nowhere("a ledger heavier than the whole branch was selected meanwhile"), true
			}
			st.worked = false
			return result{}, true
		}
		q := wire.ExtensionReq{RequestCommon: h.common(), From: st.from, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}
		f, err := h.fetch(pi, p, q, st, st.want, &st.idx, false, deadline)
		for _, eh := range f.hashes {
			st.branch[eh] = true
		}
		if f.count > 0 {
			st.from = f.last
		}
		if err != nil {
			return chaseErr(err), true
		}
		switch f.kind {
		case wire.TypeExtension:
			if f.count > 0 && f.more {
				continue
			}
			return result{}, false
		case wire.TypeNothingHeavier:
			return result{}, false // perhaps this node's own ledger grew
		default:
			return nowhere("extension answered %s", f.kind), true
		}
	}
}

// fetchAll fetches p's whole ledger, for a node that holds nothing of the
// pool. Its entries are charged to the revert budget, which waits for room
// rather than refusing (P2). Past the time slice it yields, once it has
// made a request, and the next chase goes on from st.from.
func (h *Host) fetchAll(pi *poolInfo, p *peer, st *progress, before weight.Weight, deadline time.Time) result {
	asked := false
	for {
		if asked && time.Now().After(deadline) {
			return result{out: chase.Yield, why: "time slice used up fetching the ledger"}
		}
		if h.paced(p) {
			return result{out: chase.Yield, why: "requests to the sender paced"}
		}
		asked = true
		var q wire.Request = wire.FullLedgerReq{RequestCommon: h.common(), Variant: wire.EntriesWithLayers}
		if st.from != st.common {
			q = wire.ExtensionReq{RequestCommon: h.common(), From: st.from, Beat: weight.Zero, Variant: wire.EntriesWithLayers}
		}
		f, err := h.fetch(pi, p, q, st, nil, nil, true, deadline)
		if f.count > 0 {
			st.from = f.last
		}
		if err != nil {
			return chaseErr(err)
		}
		if f.kind != wire.TypeExtension || !f.more || f.count == 0 {
			break
		}
	}
	return h.outcome(pi, before)
}

// fetched is what one entries reply brought.
type fetched struct {
	kind   wire.Type
	more   bool
	last   hash.Hash
	hashes []hash.Hash // of the entries taken in, in order
	count  int
	stated weight.Weight
}

// fetch sends an extension or full ledger request and takes in the
// entries of the reply as they arrive, in chain order. With expect, entry
// *idx onwards must match the hashes it gives for their indexes (those
// that carried work in step 4). charge says whether its entries draw on
// the sender's revert budget. st, when set, keeps a large entry's parts
// across slices, and deadline ends a slice.
func (h *Host) fetch(pi *poolInfo, p *peer, q wire.Request, st *progress, expect func(int) (hash.Hash, bool), idx *int, charge bool, deadline time.Time) (fetched, error) {
	var f fetched
	parent := pi.s.PoolID(pi.key.Index)
	if e, ok := q.(wire.ExtensionReq); ok {
		parent = e.From
	}
	take := func(it wire.EntryItem) error {
		eh := it.Entry.Hash(pi.s)
		if it.Entry.Parent != parent {
			// Out of order: the reply is false, the entry may well be
			// valid, so nothing goes on the known-bad list (P5).
			return falseErr{err: fmt.Errorf("entry %s does not link", eh.Short())}
		}
		if idx != nil {
			if want, ok := expect(*idx); ok && eh != want {
				// The sender lied about its listing; the listed hash may be a
				// valid entry's it copied, so nothing is bad-listed, and the
				// sender is flagged (G4, Black's Round 7 note; D121).
				return falseErr{err: fmt.Errorf("entry %s is not the hash %s listed", eh.Short(), want.Short())}
			}
			*idx++
		}
		if err := h.takeItem(pi, p, it, eh, charge); err != nil {
			return err
		}
		parent = eh
		f.last = eh
		f.hashes = append(f.hashes, eh)
		f.count++
		return nil
	}
	var large *wire.Large
	err := h.ask(p, pi, q, func(m wire.Message) error {
		f.kind, f.stated = m.Type, m.Stated
		f.more = m.More()
		if m.Type == wire.TypeLarge {
			l, err := wire.DecodeLarge(m.Body)
			if err != nil {
				return err
			}
			large = &l
			return nil
		}
		if m.Type != wire.TypeExtension {
			return wire.DecodeEmpty(m.Type, m.Body)
		}
		c, err := wire.DecodeEntriesChunk(m.Body)
		if err != nil {
			return err
		}
		for _, it := range c.Items {
			if err := take(it); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && large != nil {
		// The next entry is too large for one chunk: it comes in parts,
		// and the reply goes on after it (P1, D117).
		var it wire.EntryItem
		it, err = h.fetchParts(pi, p, *large, parent, st, deadline)
		if err == nil {
			err = take(it)
		}
		f.kind, f.more = wire.TypeExtension, true
	}
	if errors.Is(err, wire.ErrMalformed) {
		h.misbehave(p, err)
	}
	return f, err
}

// partial is a large entry's encoded item as far as it has arrived, in a
// temporary file, so what a sender claims costs the node disk as the bytes
// arrive, not memory (RED-016, decision D120).
type partial struct {
	entry hash.Hash
	size  uint64
	f     *os.File
	off   uint64
}

func (pt *partial) close() {
	pt.f.Close()
	os.Remove(pt.f.Name())
}

// errPartsYield ends a slice in the middle of a large entry; st keeps the
// parts, and the next slice goes on from them.
var errPartsYield = errors.New("time slice used up fetching a large entry")

// fetchParts fetches the encoded item of an entry too large for one
// chunk, part by part, and decodes it once whole. Each part must start
// where the last ended, hold at least a byte and name the size the sender
// stated, and the first must start with the expected parent (D117,
// RED-016). The parts go to a temporary file; with st, a fetch past the
// deadline yields and the next slice goes on from them. No size is
// refused: the sender pays for every byte it claims by sending it.
func (h *Host) fetchParts(pi *poolInfo, p *peer, l wire.Large, parent hash.Hash, st *progress, deadline time.Time) (wire.EntryItem, error) {
	var pt *partial
	if st != nil && st.part != nil && st.part.entry == l.Entry && st.part.size == l.Size {
		pt = st.part
	} else {
		st.drop()
		f, err := os.CreateTemp(h.cfg.PartsDir, "poo-parts-*")
		if err != nil {
			return wire.EntryItem{}, err
		}
		pt = &partial{entry: l.Entry, size: l.Size, f: f}
		if st != nil {
			st.part = pt
		}
	}
	keep := false
	defer func() {
		if !keep {
			pt.close()
			if st != nil && st.part == pt {
				st.part = nil
			}
		}
	}()
	asked := false
	for pt.off < l.Size {
		if asked && st != nil && !deadline.IsZero() && time.Now().After(deadline) {
			keep = true
			return wire.EntryItem{}, errPartsYield
		}
		asked = true
		q := wire.PartsReq{RequestCommon: h.common(), Entry: l.Entry, Variant: wire.EntriesWithLayers, Offset: pt.off}
		var kind wire.Type
		more := false
		err := h.ask(p, pi, q, func(m wire.Message) error {
			kind, more = m.Type, m.More()
			if m.Type != wire.TypePart {
				return wire.DecodeEmpty(m.Type, m.Body)
			}
			part, err := wire.DecodePart(m.Body)
			if err != nil {
				return err
			}
			if part.Total != l.Size || part.Offset != pt.off || len(part.Data) == 0 || uint64(len(part.Data)) > l.Size-pt.off {
				return falseErr{err: fmt.Errorf("part %d+%d of %d for entry %s, want %d of %d", part.Offset, len(part.Data), part.Total, l.Entry.Short(), pt.off, l.Size)}
			}
			if pt.off == 0 && (len(part.Data) < len(parent) || !bytes.Equal(part.Data[:len(parent)], parent[:])) {
				// An entry item starts with its parent: this one is not
				// the next entry of the reply (RED-016).
				return falseErr{err: fmt.Errorf("the parts of entry %s do not start with its parent %s", l.Entry.Short(), parent.Short())}
			}
			if _, err := pt.f.Write(part.Data); err != nil {
				return err
			}
			pt.off += uint64(len(part.Data))
			return nil
		})
		if err != nil {
			return wire.EntryItem{}, err
		}
		if kind != wire.TypePart {
			return wire.EntryItem{}, fmt.Errorf("parts of entry %s answered %s", l.Entry.Short(), kind)
		}
		if pt.off < l.Size && !more {
			return wire.EntryItem{}, falseErr{err: fmt.Errorf("entry %s ended at %d of %d bytes", l.Entry.Short(), pt.off, l.Size)}
		}
	}
	// Decoding needs the item whole, as the pool will hold it (D117).
	buf, err := os.ReadFile(pt.f.Name())
	if err != nil {
		return wire.EntryItem{}, err
	}
	it, err := wire.DecodeItem(buf)
	if err != nil {
		return wire.EntryItem{}, err
	}
	if eh := it.Entry.Hash(pi.s); eh != l.Entry {
		// The sender named one hash and sent another entry.
		return wire.EntryItem{}, falseErr{err: fmt.Errorf("parts of %s make entry %s", l.Entry.Short(), eh.Short())}
	}
	return it, nil
}

// errRevertBudget ends a fetch when the sender's revert budget is spent.
var errRevertBudget = errors.New("the revert budget is spent")

// spendRevert draws what checking e costs (Engine.EntryCost) from the
// sender's revert budget: pushed entries, and requested ones whose work
// was not checked first (RED-011, RED-012, D111, D114, D116). Over it the
// entry waits; it is never refused for good.
func (h *Host) spendRevert(pi *poolInfo, p *peer, e *entry.Entry) error {
	c := h.n.Engine().EntryCost(pi.key, e)
	if c == 0 {
		return nil
	}
	ok, act := h.tr.Spend(p.key, tracking.KindRevert, uint64(c), time.Now())
	if err := h.act(p, act); err != nil {
		return err
	}
	if !ok {
		return errRevertBudget
	}
	return nil
}

// takeItem takes in one requested entry and its layers, charging the
// revert budget when charge is set.
func (h *Host) takeItem(pi *poolInfo, p *peer, it wire.EntryItem, eh hash.Hash, charge bool) error {
	if charge {
		if err := h.spendRevert(pi, p, it.Entry); err != nil {
			return err
		}
	}
	_, err := h.n.HandleEntryFrom(pi.key, it.Entry, engine.Requested, p.id)
	switch {
	case err == nil, errors.Is(err, engine.ErrSeen):
	case errors.Is(err, ledger.ErrInvalidEntry):
		return falseErr{bad: []hash.Hash{eh}, err: fmt.Errorf("invalid entry: %w", err)}
	default:
		// The first entry that cannot be kept ends what is kept from the
		// reply; the ones before it stay.
		return fmt.Errorf("entry %s: %w", eh.Short(), err)
	}
	return h.takeLayers(pi, p, eh, it.Nonces)
}

func (h *Host) takeLayers(pi *poolInfo, p *peer, eh hash.Hash, nonces []uint64) error {
	for _, n := range nonces {
		_, err := h.n.HandleLayerFrom(pi.key, layer.Layer{Entry: eh, Nonce: n}, engine.Requested, p.id)
		if errors.Is(err, ledger.ErrWeakLayer) {
			// A layer is not an entry: the sender is flagged, and nothing
			// goes on the known-bad list (P5).
			return falseErr{err: fmt.Errorf("layer below the minimum strength: %w", err)}
		}
	}
	return nil
}

// topUpLayers is the chase when the sender's newest hashes end on an
// entry of this node's selected ledger: the sender holds those entries
// with more layers. It asks for the layers on the sender's own hashes,
// from its tip back, page by page, and takes them in, until this node's
// ledger is as heavy as the sender stated or the sender's chain is done
// (Black's B1, D121). It used to ask about this node's latest entries,
// which a sender on another branch does not hold. Past the deadline it
// yields, once it has made a request; st.top is the page to go on from.
func (h *Host) topUpLayers(pi *poolInfo, p *peer, st *progress, stated weight.Weight, deadline time.Time) (result, bool) {
	asked := false
	for len(st.top) > 0 {
		if asked && time.Now().After(deadline) {
			return result{out: chase.Yield, why: "time slice used up taking layers"}, true
		}
		if h.paced(p) {
			return result{out: chase.Yield, why: "requests to the sender paced"}, true
		}
		asked = true
		for len(st.top) > 0 {
			items, err := h.askLayers(pi, p, st.top)
			if err != nil {
				return chaseErr(err), true
			}
			if len(items) == 0 {
				return nowhere("no layers sent"), true
			}
			for _, it := range items {
				if it.Held {
					if err := h.takeLayers(pi, p, it.Entry, it.Nonces); err != nil {
						return chaseErr(err), true
					}
				}
			}
			st.top = st.top[min(len(items), len(st.top)):]
		}
		if !stated.Greater(h.ownWeight(pi)) || st.topStart {
			break
		}
		hs, fromStart, _, kind, err := h.hashPage(pi, p, wire.HashChainReq{RequestCommon: h.common(), HasAnchor: true, Anchor: st.topAt, Dir: wire.Before, Count: h.cfg.Limits.HashLimit})
		switch {
		case err != nil:
			return chaseErr(err), true
		case kind == wire.TypeAnchorNotHeld:
			return nowhere("the sender no longer holds %s", st.topAt.Short()), true
		case kind != wire.TypeHashChain:
			return nowhere("hash chain answered %s", kind), true
		}
		if len(hs) == 0 {
			break
		}
		st.top, st.topAt, st.topStart = hs, hs[0], fromStart
	}
	return result{}, false
}
