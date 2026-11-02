package p2p

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// disk: the chase waits for disk space for a large entry.
	disk bool
}

// WaitDisk is a chase group's Waiting while its chase waits for disk
// space for a large entry's parts: poo chases, poo doctor and the metrics
// show it, so a heavier ledger waiting on disk does not look like a
// healthy node (Black's RED-022 condition, D124).
const WaitDisk = "waiting for disk space for a large entry"

// partsDir is where large entries' parts go.
func (h *Host) partsDir() string {
	if h.cfg.PartsDir != "" {
		return h.cfg.PartsDir
	}
	return os.TempDir()
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
	if p.closed() && (res.out == chase.Nowhere || res.out == chase.Stalled || res.out == chase.Slowed) {
		res.out = chase.Gone
	}
	now := time.Now()
	if res.out.Keeps() {
		h.cq.Note(j.Group.ID, res.why, now)
	}
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
	case chase.Yield, chase.Stalled, chase.Slowed:
		// Not an outcome: the chase goes on later with what it took in.
	}
	switch {
	case res.disk:
		// A heavier ledger waiting on disk space must not look like a
		// healthy node (Black's RED-022 condition, D124).
		h.log.Warn("chase waits for disk space", "pool", pi.key.String(), "peer", p.short(), "dir", h.partsDir(), "why", res.why)
	case res.why != "":
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
		m0 := st.mark()
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
		res = slowedIf(res, st, m0)
		if res.out.Keeps() {
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
	m0 := st.mark()
	var res result
	if st.topping {
		done := false
		if res, done = h.topUpLayers(pi, p, st, j.Sender.Stated, deadline); !done {
			res = h.outcome(pi, before)
		}
	} else {
		res = h.stepsFrom(pi, p, j, st, before, deadline)
	}
	res = slowedIf(res, st, m0)
	if res.out.Keeps() {
		h.saveProgress(k, st)
	} else {
		st.drop()
	}
	return res
}

// progressMark is a snapshot of how far a chase has come, to tell
// whether a chase took something in before it stalled.
type progressMark struct {
	listed, pending, idx, top int
	total                     string
	from, topAt               hash.Hash
	partOff                   uint64
	manyFrom                  uint32
	worked                    bool
}

func (st *progress) mark() progressMark {
	m := progressMark{listed: st.listed, pending: len(st.pending), idx: st.idx, top: len(st.top),
		total: st.total.String(), from: st.from, topAt: st.topAt, manyFrom: st.manyFrom, worked: st.worked}
	if st.part != nil {
		m.partOff = st.part.off
	}
	return m
}

// slowedIf turns a stall into chase.Slowed when the chase took something
// in before it, so the sender's backoff does not double (Black's C-6,
// D129).
func slowedIf(res result, st *progress, m0 progressMark) result {
	if res.out == chase.Stalled && st.mark() != m0 {
		res.out = chase.Slowed
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
	case errors.Is(err, errPartsSpace):
		// Not refused: it goes on once the disk has room (RED-022).
		return result{out: chase.Yield, why: WaitDisk, disk: true}
	case errors.Is(err, errTimeout):
		// Not refused either: the sender is backed off, and a later chase
		// goes on from what this one took in (Black's B2, D124).
		return result{out: chase.Stalled, why: err.Error()}
	case errors.Is(err, errLayersYield):
		return result{out: chase.Yield, why: "time slice used up paging an entry's layers"}
	case errors.Is(err, errLayersPaced):
		return result{out: chase.Yield, why: "requests to the sender paced"}
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
	total weight.Weight // provisional work counted so far
	// counted holds the entries whose layers were counted, not each
	// layer: the layers of one entry are told apart in the reply that
	// carries them, and an entry's many layers by their order, so the
	// progress holds nothing for each layer (RED-026, D131).
	counted map[hash.Hash]bool
	marks   map[int]hash.Hash // listed hashes with layers, by index after common
	worked  bool              // total beat the node's own weight after common

	// Step 5.
	from   hash.Hash
	idx    int
	branch map[hash.Hash]bool // marked and fetched hashes
	part   *partial           // a large entry, as far as it has arrived

	// Kept by saveProgress: when it was saved, and since when its sender
	// has not been connected (zero while it is), for sweepProgress;
	// compacted once the sweep has cut it down to its common entry.
	saved, away time.Time
	compacted   bool
	// partsRate is the rate, in bytes a second, at which the last chase
	// that asked this sender for parts received them, over the time its
	// requests took, lost ones included, measured over a whole chase and
	// never a burst (fetchParts, Black's B6, D140); partsAt is when it
	// was measured (takeRoom, D139).
	partsRate float64
	partsAt   time.Time

	// Topping up layers on entries both hold (topUpLayers).
	topping  bool
	top      []hash.Hash // the sender's hashes whose layers to ask for next
	topAt    hash.Hash   // the oldest of them, the anchor for the page before
	topStart bool        // top reaches the sender's first entry

	// An entry's many layers, paged with layers-from requests: the entry,
	// the next index, and the last and the furthest layer read, so paging
	// goes on from there after a yield or a return to step 4, and the
	// order is checked across pages in constant memory (Black's B3 and
	// C-5, D124).
	many              hash.Hash
	manyOn            bool
	manyFrom          uint32
	manyRead          bool
	manyCur, manyHigh layerAt
	manyTotal         uint32 // the total the last page stated
	manySlack         uint64 // read layers a page may still repeat
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
	now := time.Now()
	on := h.connectedIDs()
	h.progMu.Lock()
	defer h.progMu.Unlock()
	if h.prog == nil {
		h.prog = map[progressKey]*progress{}
	}
	if _, ok := h.prog[k]; !ok {
		h.progOrder = append(h.progOrder, k)
	}
	st.saved, st.away, st.compacted = now, time.Time{}, false
	h.prog[k] = st
	h.sweepLocked(now, on)
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

// connectedIDs returns the ids of the connected peers.
func (h *Host) connectedIDs() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	on := make(map[string]bool, len(h.peers))
	for id := range h.peers {
		on[id] = true
	}
	return on
}

// sweepProgress removes the parts files of senders gone for longer than
// ProgressGrace, and forgets any progress not taken up for ProgressTTL
// after its sender's backoff ends, so a sender that leaves holds no disk
// for long and no memory for ever (RED-023, D128), while a slow sender's
// stalled chase keeps its progress until its next turn (C-6, D129).
func (h *Host) sweepProgress(now time.Time) {
	on := h.connectedIDs()
	h.progMu.Lock()
	defer h.progMu.Unlock()
	h.sweepLocked(now, on)
}

func (h *Host) sweepLocked(now time.Time, on map[string]bool) {
	kept := h.progOrder[:0]
	for _, k := range h.progOrder {
		st := h.prog[k]
		switch {
		case on[k.sender]:
			st.away = time.Time{}
		case st.away.IsZero():
			st.away = now
		}
		if !st.away.IsZero() && now.Sub(st.away) > h.cfg.ProgressGrace && !st.compacted {
			// A sender gone past the grace loses its parts file and all
			// it showed but the common entry: what was counted and
			// fetched is rebuilt from the sender if it comes back. The
			// progress itself is kept to the time to live (Black's C-6,
			// D129; RED-026, D131).
			st.drop()
			if !st.topping {
				// Topping up holds only a page of the sender's hashes.
				c := newProgress(st.common, nil, false)
				c.saved, c.away = st.saved, st.away
				h.prog[k] = c
				st = c
			}
			st.compacted = true
		}
		// The time to live runs from when the progress was saved or, if
		// later, when its sender's backoff ends, so a stalled chase's
		// progress always outlasts the wait before its next chase
		// (Black's C-6, D129).
		from := st.saved
		if h.cq != nil {
			if until, _ := h.cq.BackedOff(k.sender, now); until.After(from) {
				from = until
			}
		}
		if now.Sub(from) > h.cfg.ProgressTTL {
			st.drop()
			delete(h.prog, k)
			continue
		}
		kept = append(kept, k)
	}
	clear(h.progOrder[len(kept):])
	h.progOrder = kept
}

// takeRoom makes room for n more bytes of the chase of st, which is
// receiving its parts at rate (bytes a second, over a whole chase: see
// fetchParts),
// by cutting bytes off the end of other kept parts files until room says
// there is room. It reports whether it did. Bytes are taken only as the
// waiting chase receives them, so whoever takes room has paid for it
// byte for byte, and only from a file it is not behind (takeFrom). The
// file cut the most is the one whose sender left, then one whose last
// chase took nothing or that has not asked for a while, then the one
// whose sender is slowest. A file cut to nothing goes (D139; rates, not
// times to finish, since Black's B6, D140).
//
// This replaces the ranks of D131, D132, D136 and D137 with a measure no
// claim alone can game: a sender that claims a size and trickles has a
// low rate, so it finishes late and is cut; one that claims a size just
// under the honest entry's must also deliver as fast to stay ahead, and
// then finish, after which the hash decides; a fresh sender is judged by
// what it delivers, and takes only what it delivers. Between two honest
// fetches the one expected to finish first keeps the room, and each cut
// widens its lead, so they never take turns (ORANGE-022), and a fetch that waits is never
// held for good: once the holder stops delivering, it is cut (Black's
// B4, D132; B5, D137; RED-031 to RED-033).
func (h *Host) takeRoom(st *progress, rate float64, n uint64, room func() bool) bool {
	on := h.connectedIDs()
	now := time.Now()
	h.progMu.Lock()
	defer h.progMu.Unlock()
	for {
		var low *progress
		lowPri, lowEst := 0, 0.0
		for k, o := range h.prog {
			pri, est, ok := h.takeFrom(o, !on[k.sender], st, rate, now)
			if !ok {
				continue
			}
			if low == nil || pri < lowPri || pri == lowPri && est > lowEst {
				low, lowPri, lowEst = o, pri, est
			}
		}
		if low == nil {
			return false
		}
		low.part.cut(n)
		if low.part.off == 0 {
			low.drop()
		}
		if room() {
			return true
		}
	}
}

// takeFrom reports whether the waiting chase st, receiving at rate, may
// cut kept file o, with its priority (lower first) and how slow o's
// sender is (the larger, the sooner it is cut). A file whose sender left
// is cut first (0); then one whose last chase that asked for parts took
// nothing, or that has not asked for partsStale (1); then one whose
// sender delivers at least partsMargin times more slowly than st's (2);
// then, between senders within partsMargin of each other, one with more
// still to come (3).
// A claim, and how much is still to come, decide only between senders
// about as fast as each other: a holder keeps the room against a much
// faster fetch only by delivering about as fast, for as long as it holds
// it (Black's B6, D140). With cuts by bytes neither file starts over.
func (h *Host) takeFrom(o *progress, away bool, st *progress, rate float64, now time.Time) (int, float64, bool) {
	if o == st || o.part == nil || o.part.off == 0 {
		return 0, 0, false
	}
	switch {
	case away:
		return 0, 0, true
	case o.partsRate <= 0 || now.Sub(o.partsAt) > h.partsStale():
		return 1, 0, true
	case rate > 0 && rate >= partsMargin*o.partsRate:
		return 2, 1 / o.partsRate, true
	case rate > 0 && rate*partsMargin >= o.partsRate && st.part != nil && st.partsToCome() < o.partsToCome():
		// About as fast: the one nearer its end keeps the room. Each cut
		// puts the file cut further behind and the one cutting further
		// ahead, so the two never take turns (ORANGE-022), and a claim
		// near its end must still be delivered at about the honest rate
		// and then finished, after which the hash decides.
		return 3, 1 / o.partsRate, true
	}
	return 0, 0, false
}

// partsMargin is how many times faster than a kept file's sender a fetch
// must deliver to cut it, so two fetches at about the same rate do not
// trade bytes back and forth (Black's B6, D140).
const partsMargin = 1.5

// partsStale is how long a kept parts file's measure lasts without its
// chase asking for parts again: two of the sender's first backoff steps,
// or two time slices if longer, so a holder that stops being chased does
// not keep the room on an old measure (Black's B5, D137).
func (h *Host) partsStale() time.Duration {
	b := h.cfg.Chase.Backoff
	if b <= 0 {
		b = chase.DefaultConfig().Backoff
	}
	return 2 * max(b, h.cfg.ChaseSlice)
}

// partsToCome is how many bytes st's large entry still needs.
func (st *progress) partsToCome() uint64 {
	if st.part == nil {
		return 0
	}
	return st.part.size - st.part.off
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
		// Each answer, and each page of an entry's many layers, is counted
		// as it arrives; once the work beats the node's, nothing more is
		// read (RED-021, D123).
		before := st.total
		var stop *result
		answered, enough, err := h.askLayers(pi, p, st, page, deadline, func(it wire.HeldLayers) (bool, error) {
			i := at[it.Entry]
			if !it.Held {
				r := nowhere("the sender no longer holds %s", it.Entry.Short())
				stop = &r
				return true, nil
			}
			paging := st.manyOn && st.many == it.Entry
			if st.counted[it.Entry] && !paging {
				// Listed again: its layers were counted where it was
				// listed first.
				return st.total.Greater(beat), nil
			}
			st.counted[it.Entry] = true
			// Within one reply an entry's layers are told apart here; the
			// pages of its many layers come without repeats (layersFrom).
			var seen map[uint64]bool
			if !paging {
				seen = make(map[uint64]bool, len(it.Nonces))
			}
			for _, n := range it.Nonces {
				lh := layer.Layer{Entry: it.Entry, Nonce: n}.Hash(fn)
				if lh.Strength() < minLayer {
					// The layer is false, not the entry under it (P5).
					r := falseBranch(nil, "layer on %s below the minimum strength", it.Entry.Short())
					stop = &r
					return true, nil
				}
				if seen != nil {
					if seen[n] {
						continue
					}
					seen[n] = true
				}
				st.total = st.total.Add(weight.Credit(lh.Strength()))
				st.marks[i] = it.Entry
				st.branch[it.Entry] = true
			}
			return st.total.Greater(beat), nil
		})
		if err != nil {
			return chaseErr(err), true
		}
		if stop != nil {
			return *stop, true
		}
		if answered == 0 && !enough {
			return nowhere("no layers sent"), true
		}
		if st.total.Greater(before) {
			bare = 0
		} else {
			bare++
		}
		st.pending = st.pending[min(answered, end):]
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

// askLayers asks p for the layers on hashes and gives each answered entry
// to take as it arrives, in order. An entry with more layers than a reply
// carries whole comes as a many-layers reply, and its layers are then
// paged with layers-from requests, each page given to take as one item,
// so no entry holds too many layers to be counted (D121) and none is held
// whole. take returns enough once what it counted is all the caller needs
// (RED-021, decision D123). It returns n, how many of hashes, from the
// first, were taken whole: an entry that take stopped in the middle of is
// not among them, nor is any after it, so a caller that comes back moves
// on only past what it counted (Black's C-5, D124). An entry whose layers
// were being paged when a chase stopped, st.many, goes on from st.manyFrom
// when it is first in hashes. enough reports that take said enough.
func (h *Host) askLayers(pi *poolInfo, p *peer, st *progress, hashes []hash.Hash, deadline time.Time, take func(wire.HeldLayers) (enough bool, err error)) (n int, enough bool, err error) {
	if len(hashes) > 0 && st.manyOn && hashes[0] == st.many {
		whole, enough, err := h.layersFrom(pi, p, st, hashes[0], deadline, take)
		if whole {
			n = 1
		}
		return n, enough, err
	}
	var many *wire.ManyLayers
	err = h.ask(p, pi, wire.LayersReq{RequestCommon: h.common(), Hashes: hashes}, func(m wire.Message) error {
		switch m.Type {
		case wire.TypeLayers:
			c, err := wire.DecodeLayersChunk(m.Body)
			if err != nil {
				return err
			}
			for _, it := range c.Items {
				if enough {
					return nil // not counted: a later chase asks again
				}
				if n >= len(hashes) || it.Entry != hashes[n] {
					return falseErr{err: fmt.Errorf("layers named %s, not the hash asked about next", it.Entry.Short())}
				}
				if enough, err = take(it); err != nil {
					return err
				}
				n++
			}
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
		st.startMany(many.Entry)
		var whole bool
		whole, enough, err = h.layersFrom(pi, p, st, many.Entry, deadline, take)
		if whole {
			n = 1
		}
	}
	if errors.Is(err, wire.ErrMalformed) {
		h.misbehave(p, err)
	}
	return n, enough, err
}

// errLayersYield and errLayersPaced end a slice while an entry's many
// layers are paged; the chase goes on from st.manyFrom (Black's B3).
var (
	errLayersYield = errors.New("time slice used up paging an entry's layers")
	errLayersPaced = errors.New("requests to the sender paced while paging an entry's layers")
)

// layerAt is one layer's place in the strongest-first order of an entry's
// layers: the stronger first, the lower nonce first among equals.
type layerAt struct {
	strength int
	nonce    uint64
}

// after reports whether a comes after b in that order.
func (a layerAt) after(b layerAt) bool {
	if a.strength != b.strength {
		return a.strength < b.strength
	}
	return a.nonce > b.nonce
}

// startMany starts paging entry eh's layers from the first.
func (st *progress) startMany(eh hash.Hash) {
	st.many, st.manyOn, st.manyFrom, st.manyRead = eh, true, 0, false
	st.manyTotal, st.manySlack = 0, 0
}

// layersFrom pages the layer nonces of entry eh from p, strongest first,
// from st.manyFrom, giving each page to take until it says enough or the
// layers end. whole reports that the layers ended. Before each page after
// the first it checks the deadline and the pace, and yields with the
// entry and its offset kept in st, so a sender cannot hold the chase here
// (Black's B3, D124). Each page must start where the last ended; a total
// that changes is followed, since layers keep arriving (D121).
//
// The order is checked across pages with what st keeps, the last layer
// read, the furthest, and the total stated: within a page each layer
// comes after the one before it. A page may start with layers read
// already, since layers that arrive meanwhile push read ones later, but
// no more of them than the total grew by since, and such a run must
// reach the furthest layer read before any layer past it. A sender that
// skips, reorders or repeats otherwise, or whose total shrinks, is false.
func (h *Host) layersFrom(pi *poolInfo, p *peer, st *progress, eh hash.Hash, deadline time.Time, take func(wire.HeldLayers) (bool, error)) (whole, enough bool, err error) {
	fn := pi.s.Hash()
	for pages := 0; ; pages++ {
		if pages > 0 && time.Now().After(deadline) {
			return false, false, errLayersYield
		}
		if pages > 0 && h.paced(p) {
			return false, false, errLayersPaced
		}
		more, got := false, false
		var kind wire.Type
		err := h.ask(p, pi, wire.LayersFromReq{RequestCommon: h.common(), Entry: eh, From: st.manyFrom}, func(m wire.Message) error {
			kind, more = m.Type, m.More()
			if m.Type != wire.TypeLayersFrom {
				return wire.DecodeEmpty(m.Type, m.Body)
			}
			c, err := wire.DecodeLayersFrom(m.Body)
			if err != nil {
				return err
			}
			if c.From != st.manyFrom {
				return falseErr{err: fmt.Errorf("layers of %s from %d, want %d", eh.Short(), c.From, st.manyFrom)}
			}
			if enough || len(c.Nonces) == 0 {
				return nil
			}
			if uint64(c.From)+uint64(len(c.Nonces)) > uint64(c.Total) || (st.manyRead && c.Total < st.manyTotal) {
				return falseErr{err: fmt.Errorf("layers of %s: %d from %d of %d, after %d", eh.Short(), len(c.Nonces), c.From, c.Total, st.manyTotal)}
			}
			if st.manyRead {
				// Layers that arrived since the last page may push that
				// many read ones into this one, and no more.
				st.manySlack += uint64(c.Total - st.manyTotal)
			}
			st.manyTotal = c.Total
			// Only layers not read before go on to take: a page may
			// repeat read ones (below), and they were counted then, so
			// nothing needs to remember each layer (RED-026, D131).
			fresh := make([]uint64, 0, len(c.Nonces))
			for i, n := range c.Nonces {
				at := layerAt{layer.Layer{Entry: eh, Nonce: n}.Strength(fn), n}
				if st.manyRead {
					switch {
					case i > 0 && !at.after(st.manyCur):
						return falseErr{err: fmt.Errorf("layers of %s out of order at %d", eh.Short(), int(c.From)+i)}
					case at.after(st.manyHigh) && st.manyCur != st.manyHigh:
						return falseErr{err: fmt.Errorf("layers of %s skip past the last read at %d", eh.Short(), int(c.From)+i)}
					case !at.after(st.manyHigh):
						if st.manySlack == 0 {
							return falseErr{err: fmt.Errorf("layers of %s read again at %d with no new layers", eh.Short(), int(c.From)+i)}
						}
						st.manySlack--
						st.manyCur = at
						continue
					}
				}
				fresh = append(fresh, n)
				st.manyCur = at
				if !st.manyRead || at.after(st.manyHigh) {
					st.manyHigh = at
				}
				st.manyRead = true
			}
			got = true
			st.manyFrom += uint32(len(c.Nonces))
			if len(fresh) == 0 {
				return nil
			}
			enough, err = take(wire.HeldLayers{Entry: eh, Held: true, Nonces: fresh})
			return err
		})
		if err != nil {
			return false, enough, err
		}
		if kind != wire.TypeLayersFrom {
			return false, false, fmt.Errorf("layers of %s answered %s", eh.Short(), kind)
		}
		if !more {
			st.manyOn = false
			return true, enough, nil
		}
		if enough {
			return false, true, nil
		}
		if !got {
			return false, false, falseErr{err: fmt.Errorf("layers of %s: more said, none sent", eh.Short())}
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
		if len(c.Items) > 1 && c.Items[0].Entry.Parent == parent {
			// The chunk's witnesses on every core first, so each entry
			// taken in order finds them verified (ORANGE-020).
			es := make([]*entry.Entry, len(c.Items))
			for i, it := range c.Items {
				es[i] = it.Entry
			}
			h.n.Engine().PreverifyPage(pi.key, es)
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

// cut takes up to n bytes off the end of the parts, which its chase then
// asks for again (takeRoom, D139).
func (pt *partial) cut(n uint64) {
	off := pt.off - min(n, pt.off)
	if err := pt.f.Truncate(int64(off)); err != nil {
		return
	}
	if _, err := pt.f.Seek(int64(off), io.SeekStart); err != nil {
		return
	}
	pt.off = off
}

func (pt *partial) close() {
	pt.f.Close()
	os.Remove(pt.f.Name())
}

// errPartsYield ends a slice in the middle of a large entry; st keeps the
// parts, and the next slice goes on from them.
var errPartsYield = errors.New("time slice used up fetching a large entry")

// errPartsSpace ends a slice when the parts' disk is down to its reserve;
// st keeps the parts, and a later slice goes on once there is room.
var errPartsSpace = errors.New("the disk for large entries is down to its reserve")

// diskSpace is statDisk, a seam for tests.
var diskSpace = statDisk

// partsRoom reports whether the disk of the parts directory has room for
// n more bytes above its reserve: the larger of Config.PartsReserve and
// 2% of the disk, so a sender's claim never takes the room the store
// needs. When the space cannot be read, it has room (RED-022, D123).
func (h *Host) partsRoom(f *os.File, n uint64) bool {
	free, total, err := diskSpace(filepath.Dir(f.Name()))
	if err != nil {
		return true
	}
	reserve := max(h.cfg.PartsReserve, total/50)
	return free >= reserve && free-reserve >= n
}

// fetchParts fetches the encoded item of an entry too large for one
// chunk, part by part, and decodes it once whole. Each part must start
// where the last ended, hold at least a byte and name the size the sender
// stated, and the first must start with the expected parent (D117,
// RED-016). The parts go to a temporary file; with st, a fetch past the
// deadline, or one whose disk is down to its reserve, yields and a later
// slice goes on from them (RED-022). No size is
// refused: the sender pays for every byte it claims by sending it.
func (h *Host) fetchParts(pi *poolInfo, p *peer, l wire.Large, parent hash.Hash, st *progress, deadline time.Time) (_ wire.EntryItem, err error) {
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
		// The parts stay with the progress whenever the chase keeps it: a
		// yield, and a stall such as a timeout, go on from their offset
		// (RED-024, D128). A false or finished entry lets them go, and so
		// does one waiting for room with nothing in it yet (D139).
		if pt.off == 0 && errors.Is(err, errPartsSpace) {
			pt.close()
			if st != nil && st.part == pt {
				st.part = nil
			}
			return
		}
		if !keep && (st == nil || err == nil || !chaseErr(err).out.Keeps()) {
			pt.close()
			if st != nil && st.part == pt {
				st.part = nil
			}
		}
	}()
	// What this call measures of the sender: the bytes of parts received,
	// kept or not, over the time its requests took, a lost one's wait
	// included. It is a measure of a whole chase, never a burst, once a
	// request was lost, a full parts reply's worth (or the rest of the
	// entry) arrived, or the slice ended; only then is it saved, and only
	// then does it decide a cut (Black's B6, D140).
	began := time.Now()
	var seen uint64
	var dur time.Duration
	lost := false
	measured := func() bool {
		return lost || seen >= min(partsReply, l.Size-pt.off) ||
			(!deadline.IsZero() && time.Now().After(deadline)) || time.Since(began) >= h.cfg.ChaseSlice
	}
	rate := func(inFlight time.Duration) float64 {
		if d := (dur + inFlight).Seconds(); d > 0 {
			return float64(seen) / d
		}
		return 0
	}
	defer func() {
		if st != nil && (seen > 0 || lost) && measured() {
			st.partsRate, st.partsAt = rate(0), time.Now()
		}
	}()
	// probing: no room, and no measure yet that could take any. The chase
	// goes on asking for the part at its offset and throws it away until
	// it has measured the sender, then takes room by that measure or
	// waits (D140).
	probing := false
	asked := false
	for pt.off < l.Size {
		if asked && st != nil && !deadline.IsZero() && time.Now().After(deadline) {
			keep = true
			return wire.EntryItem{}, errPartsYield
		}
		if st != nil && (probing || !h.partsRoom(pt.f, min(l.Size-pt.off, partsReply))) && !h.mayTake(st) {
			// Free space is waited for, never a reason to refuse: a real
			// large entry goes on from its offset once there is room
			// (RED-022). Room is taken from another file only as parts
			// arrive (takeRoom); with none it could be taken from, the
			// chase does not ask. A byte of room is not room: the chase
			// asks with no measure that could take room only when a
			// whole part fits (Black's B6, D140).
			keep = true
			return wire.EntryItem{}, errPartsSpace
		}
		asked = true
		start := time.Now()
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
			n := uint64(len(part.Data))
			seen += n
			if st != nil && !h.partsRoom(pt.f, n) {
				// Room for what has arrived, paid for by its arrival
				// (D139), by this chase's measure once it has one, else by
				// the last whole chase's (D140).
				// A departed, idle or stale holder is cut by any.
				r := st.partsRate
				if measured() {
					r = rate(time.Since(start))
				}
				switch {
				case h.takeRoom(st, r, n, func() bool { return h.partsRoom(pt.f, n) }):
					probing = false
				case !measured() && h.mayTake(st):
					probing = true // thrown away, and asked for again
					return nil
				default:
					return errPartsSpace
				}
			}
			if _, err := pt.f.Write(part.Data); err != nil {
				return err
			}
			pt.off += n
			return nil
		})
		dur += time.Since(start)
		if errors.Is(err, errTimeout) {
			lost = true
		}
		if errors.Is(err, errPartsSpace) {
			keep = true
		}
		if err != nil {
			return wire.EntryItem{}, err
		}
		if kind != wire.TypePart {
			return wire.EntryItem{}, fmt.Errorf("parts of entry %s answered %s", l.Entry.Short(), kind)
		}
		if pt.off < l.Size && !more && !probing {
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
		for len(st.top) > 0 {
			if asked && time.Now().After(deadline) {
				return result{out: chase.Yield, why: "time slice used up taking layers"}, true
			}
			if h.paced(p) {
				return result{out: chase.Yield, why: "requests to the sender paced"}, true
			}
			asked = true
			// Each page of layers goes in as it arrives, so an entry's
			// many layers are never held whole (RED-021).
			answered, _, err := h.askLayers(pi, p, st, st.top, deadline, func(it wire.HeldLayers) (bool, error) {
				if it.Held {
					return false, h.takeLayers(pi, p, it.Entry, it.Nonces)
				}
				return false, nil
			})
			if err != nil {
				return chaseErr(err), true
			}
			if answered == 0 {
				return nowhere("no layers sent"), true
			}
			st.top = st.top[min(answered, len(st.top)):]
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

// mayTake reports whether a chase waiting for room could take any: some
// other kept parts file holds bytes, and the chase has no measure yet or
// one by which it would take from that file (takeFrom). A chase that
// could take none does not ask, so a waiting sender's replies are not
// thrown away chase after chase (D139).
func (h *Host) mayTake(st *progress) bool {
	on := h.connectedIDs()
	now := time.Now()
	h.progMu.Lock()
	defer h.progMu.Unlock()
	for k, o := range h.prog {
		if o == st || o.part == nil || o.part.off == 0 {
			continue
		}
		if st.partsRate <= 0 || now.Sub(st.partsAt) > h.partsStale() {
			// No measure, or an old one: the chase asks, to measure the
			// sender again over a whole chase (Black's B5 and B6, D140).
			return true
		}
		if _, _, ok := h.takeFrom(o, !on[k.sender], st, st.partsRate, now); ok {
			return true
		}
	}
	return false
}
