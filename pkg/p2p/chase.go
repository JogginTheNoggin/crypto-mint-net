package p2p

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/chase"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// maxRounds bounds the requests of one fetch, so a peer cannot keep a node
// asking forever.
const maxRounds = 4096

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
	}
	if res.why != "" {
		h.logf("chase of %s from %s: %s: %s", pi.key, p.short(), res.out, res.why)
	}
}

// runChase runs the steps of one chase with one sender, from cheap to
// costly: confirming with a priority peer, finding the common entry from
// the sender's newest hashes, checking the work after it with provisional
// layers, and only then fetching the entries in chain order. The node
// switches only through the engine, on weight it computed itself.
func (h *Host) runChase(pi *poolInfo, p *peer, j chase.Job) result {
	before := h.ownWeight(pi)
	if !j.Sender.Stated.Greater(before) {
		return result{out: chase.Caught}
	}
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
		return h.fetchAll(pi, p, before)
	}
	// Step 3: the common entry.
	common, after, res, done := h.findCommon(pi, p)
	if done {
		if res.why == "depth limit" && h.cfg.FastCatchUp && p.catFor(pi.section) == peers.CatPriority {
			return h.fetchAll(pi, p, before)
		}
		return res
	}
	h.cq.AddHashes(j.Group.ID, after)
	if len(after) == 0 {
		// The sender's ledger ends where this node's does: it holds more
		// layers on entries both hold.
		if err := h.topUpLayers(pi, p); err != nil {
			return chaseErr(err)
		}
		return h.outcome(pi, before)
	}
	// Step 4: the work behind the branch, before any entry.
	if res, done := h.checkWork(pi, p, common, after); done {
		return res
	}
	h.cq.WorkChecked(j.Group.ID)
	// Step 5: the entries, checked in chain order against the hashes the
	// sender listed.
	if res, done := h.fetchBranch(pi, p, common, after); done {
		return res
	}
	return h.outcome(pi, before)
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
	f, err := h.fetch(pi, q, wire.ExtensionReq{RequestCommon: h.common(), From: tip, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}, nil, nil)
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

// findCommon is step 3: it asks p for its newest hashes, and pages back
// until it reaches one this node holds, up to the depth limit. It returns
// the common entry (the pool_id when the ledgers share none) and the
// sender's hashes after it, oldest first; done is set when the chase ends
// here.
func (h *Host) findCommon(pi *poolInfo, p *peer) (common hash.Hash, after []hash.Hash, res result, done bool) {
	limit := h.cfg.Limits.HashLimit
	q := wire.HashChainReq{RequestCommon: h.common(), Count: limit}
	var list []hash.Hash
	seen := map[hash.Hash]bool{}
	for page := 0; page < h.cfg.DepthLimit; page++ {
		var hs []hash.Hash
		fromStart := false
		kind := wire.TypeHashChain
		err := h.ask(p, pi, q, func(m wire.Message) error {
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
			hs = append(hs, c.Hashes...)
			return nil
		})
		if err != nil {
			if errors.Is(err, wire.ErrMalformed) {
				h.misbehave(p, err)
			}
			return common, nil, chaseErr(err), true
		}
		if kind != wire.TypeHashChain {
			return common, nil, nowhere("hash chain answered %s", kind), true
		}
		if len(hs) == 0 {
			return common, nil, nowhere("the sender holds no ledger"), true
		}
		for _, x := range hs {
			if seen[x] {
				return common, nil, falseBranch([]hash.Hash{x}, "hash %s listed twice", x.Short()), true
			}
			seen[x] = true
		}
		list = append(append([]hash.Hash(nil), hs...), list...)
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
			return hs[found], list[found+1:], result{}, false
		}
		if fromStart {
			return pi.s.PoolID(pi.key.Index), list, result{}, false
		}
		q = wire.HashChainReq{RequestCommon: h.common(), HasAnchor: true, Anchor: hs[0], Dir: wire.Before, Count: limit}
	}
	return common, nil, result{out: chase.Nowhere, why: "depth limit"}, true
}

// checkWork is step 4: it asks p for the layers on the hashes after the
// common entry and checks them provisionally, until they add up to more
// than the weight to beat. The layers are never kept, counted towards a
// ledger or forwarded; done is set when the chase ends here.
func (h *Host) checkWork(pi *poolInfo, p *peer, common hash.Hash, after []hash.Hash) (result, bool) {
	var beat weight.Weight
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
		to, _ := l.WeightTo(common)
		beat = l.Weight().Sub(to)
	})
	fn := pi.s.Hash()
	minLayer, minEntry := pi.s.MinLayerStrength(), pi.s.MinEntryStrength()
	total := weight.Zero
	for _, eh := range after {
		if eh.Strength() < minEntry {
			return falseBranch([]hash.Hash{eh}, "entry hash %s below the minimum strength", eh.Short()), true
		}
		if pi.s.EntryHashCredit() {
			total = total.Add(weight.Credit(eh.Strength()))
		}
	}
	listed := map[hash.Hash]bool{}
	for _, eh := range after {
		listed[eh] = true
	}
	counted := map[hash.Hash]bool{}
	fetched := 0
	batch := int(h.cfg.Limits.HashLimit)
	for i := 0; i < len(after) && !total.Greater(beat) && fetched < h.cfg.ProvisionalLayers; {
		end := i + batch
		if end > len(after) {
			end = len(after)
		}
		var items []wire.HeldLayers
		err := h.ask(p, pi, wire.LayersReq{RequestCommon: h.common(), Hashes: after[i:end]}, func(m wire.Message) error {
			if m.Type != wire.TypeLayers {
				return wire.DecodeEmpty(m.Type, m.Body)
			}
			c, err := wire.DecodeLayersChunk(m.Body)
			if err != nil {
				return err
			}
			items = append(items, c.Items...)
			return nil
		})
		if err != nil {
			if errors.Is(err, wire.ErrMalformed) {
				h.misbehave(p, err)
			}
			return chaseErr(err), true
		}
		if len(items) == 0 {
			return nowhere("no layers sent"), true
		}
		for _, it := range items {
			if !listed[it.Entry] {
				continue
			}
			if !it.Held {
				return nowhere("the sender no longer holds %s", it.Entry.Short()), true
			}
			for _, n := range it.Nonces {
				lh := layer.Layer{Entry: it.Entry, Nonce: n}.Hash(fn)
				if lh.Strength() < minLayer {
					return falseBranch([]hash.Hash{it.Entry}, "layer on %s below the minimum strength", it.Entry.Short()), true
				}
				if counted[lh] {
					continue
				}
				counted[lh] = true
				fetched++
				total = total.Add(weight.Credit(lh.Strength()))
			}
		}
		i += len(items)
	}
	if !total.Greater(beat) {
		return nowhere("provisional work %s does not beat %s", total, beat), true
	}
	return result{}, false
}

// fetchBranch is step 5: extensions from the common entry, each entry
// checked against the hash the sender listed for it.
func (h *Host) fetchBranch(pi *poolInfo, p *peer, common hash.Hash, after []hash.Hash) (result, bool) {
	from, idx := common, 0
	for round := 0; round < maxRounds; round++ {
		q := wire.ExtensionReq{RequestCommon: h.common(), From: from, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}
		f, err := h.fetch(pi, p, q, after, &idx)
		if err != nil {
			return chaseErr(err), true
		}
		switch f.kind {
		case wire.TypeExtension:
			if f.count == 0 {
				return result{}, false
			}
			from = f.last
			if f.more {
				continue
			}
			return result{}, false
		case wire.TypeNothingHeavier:
			return result{}, false // perhaps this node's own ledger grew
		default:
			return nowhere("extension answered %s", f.kind), true
		}
	}
	return nowhere("too many requests"), true
}

// fetchAll fetches p's whole ledger: for a node that holds nothing of the
// pool, or a fast catch-up.
func (h *Host) fetchAll(pi *poolInfo, p *peer, before weight.Weight) result {
	var q wire.Request = wire.FullLedgerReq{RequestCommon: h.common(), Variant: wire.EntriesWithLayers}
	for round := 0; round < maxRounds; round++ {
		f, err := h.fetch(pi, p, q, nil, nil)
		if err != nil {
			return chaseErr(err)
		}
		if f.kind != wire.TypeExtension || !f.more || f.count == 0 {
			break
		}
		q = wire.ExtensionReq{RequestCommon: h.common(), From: f.last, Beat: weight.Zero, Variant: wire.EntriesWithLayers}
	}
	return h.outcome(pi, before)
}

// fetched is what one entries reply brought.
type fetched struct {
	kind   wire.Type
	more   bool
	last   hash.Hash
	count  int
	stated weight.Weight
}

// fetch sends an extension or full ledger request and takes in the
// entries of the reply as they arrive, in chain order. With expect, entry
// *idx onwards must match the hashes listed there, in order.
func (h *Host) fetch(pi *poolInfo, p *peer, q wire.Request, expect []hash.Hash, idx *int) (fetched, error) {
	var f fetched
	parent := pi.s.PoolID(pi.key.Index)
	if e, ok := q.(wire.ExtensionReq); ok {
		parent = e.From
	}
	err := h.ask(p, pi, q, func(m wire.Message) error {
		f.kind, f.stated = m.Type, m.Stated
		f.more = m.More()
		if m.Type != wire.TypeExtension {
			return wire.DecodeEmpty(m.Type, m.Body)
		}
		c, err := wire.DecodeEntriesChunk(m.Body)
		if err != nil {
			return err
		}
		for _, it := range c.Items {
			eh := it.Entry.Hash(pi.s)
			if it.Entry.Parent != parent {
				return falseErr{bad: []hash.Hash{eh}, err: fmt.Errorf("entry %s does not link", eh.Short())}
			}
			if idx != nil && *idx < len(expect) {
				if eh != expect[*idx] {
					return falseErr{bad: []hash.Hash{expect[*idx], eh}, err: fmt.Errorf("entry %s is not the hash %s listed", eh.Short(), expect[*idx].Short())}
				}
				*idx++
			}
			if err := h.takeItem(pi, p, it, eh); err != nil {
				return err
			}
			parent = eh
			f.last = eh
			f.count++
		}
		return nil
	})
	if errors.Is(err, wire.ErrMalformed) {
		h.misbehave(p, err)
	}
	return f, err
}

// takeItem takes in one requested entry and its layers.
func (h *Host) takeItem(pi *poolInfo, p *peer, it wire.EntryItem, eh hash.Hash) error {
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
			return falseErr{bad: []hash.Hash{eh}, err: fmt.Errorf("layer below the minimum strength: %w", err)}
		}
	}
	return nil
}

// topUpLayers asks for the layers on the latest entries of the active
// ledger, which the peer holds too but with more layers.
func (h *Host) topUpLayers(pi *poolInfo, p *peer) error {
	var hs []hash.Hash
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
		n := l.Height()
		hs = l.Chain(n-int(h.cfg.Limits.EntryLimit)+1, n)
	})
	if len(hs) == 0 {
		return nil
	}
	q := wire.LayersReq{RequestCommon: h.common(), Hashes: hs}
	err := h.ask(p, pi, q, func(m wire.Message) error {
		if m.Type != wire.TypeLayers {
			return wire.DecodeEmpty(m.Type, m.Body)
		}
		c, err := wire.DecodeLayersChunk(m.Body)
		if err != nil {
			return err
		}
		for _, it := range c.Items {
			if it.Held {
				if err := h.takeLayers(pi, p, it.Entry, it.Nonces); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if errors.Is(err, wire.ErrMalformed) {
		h.misbehave(p, err)
	}
	return err
}
