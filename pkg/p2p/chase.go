package p2p

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// maxRounds bounds the requests of one catch-up, so a peer cannot keep a
// node asking forever.
const maxRounds = 4096

// maxRechase bounds how many catch-ups run back to back for one pool when
// new reasons to ask keep arriving during one.
const maxRechase = 8

// errStop ends a catch-up quietly: the peer has nothing more to give.
var errStop = errors.New("stop")

// startChase starts a catch-up of pool pi from peer p unless one is
// running for the pool; then it runs again from p when that one ends.
func (h *Host) startChase(pi *poolInfo, p *peer) {
	h.mu.Lock()
	if h.ctx == nil || h.ctx.Err() != nil || p.closed() {
		h.mu.Unlock()
		return
	}
	if h.chasing[pi.key] {
		h.again[pi.key] = p
		h.mu.Unlock()
		return
	}
	h.chasing[pi.key] = true
	h.wg.Add(1)
	h.mu.Unlock()
	go func() {
		defer h.wg.Done()
		for i := 0; ; i++ {
			if err := h.chase(pi, p); err != nil && !errors.Is(err, errStop) {
				h.logf("catch-up of %s from %s: %v", pi.key, p.short(), err)
			}
			h.mu.Lock()
			next := h.again[pi.key]
			delete(h.again, pi.key)
			if next == nil || i+1 >= maxRechase || next.closed() || h.ctx.Err() != nil {
				delete(h.chasing, pi.key)
				h.mu.Unlock()
				return
			}
			h.mu.Unlock()
			p = next
		}
	}()
}

// tip reads the node's active ledger for pi.
func (h *Host) tip(pi *poolInfo) (hash.Hash, int, weight.Weight) {
	var t hash.Hash
	var ht int
	var w weight.Weight
	h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) { t, ht, w = l.Tip() })
	return t, ht, w
}

// fetched is what one entries reply brought.
type fetched struct {
	kind   wire.Type
	more   bool
	last   hash.Hash
	count  int
	stated weight.Weight
}

// chase catches up pool pi from p: a full ledger when the node holds
// nothing, otherwise extensions from its latest entry, finding the split
// with hash chains when the peer's heavier ledger does not hold it. It
// switches only through the engine, on weight it computed.
func (h *Host) chase(pi *poolInfo, p *peer) error {
	tip, height, _ := h.tip(pi)
	var q wire.Request = wire.ExtensionReq{RequestCommon: h.common(), From: tip, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}
	if height == 0 {
		q = wire.FullLedgerReq{RequestCommon: h.common(), Variant: wire.EntriesWithLayers}
	}
	searched := false
	for round := 0; round < maxRounds; round++ {
		f, err := h.fetch(pi, p, q)
		if err != nil {
			return err
		}
		switch f.kind {
		case wire.TypeExtension:
			if f.more {
				q = wire.ExtensionReq{RequestCommon: h.common(), From: f.last, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}
				continue
			}
			// The peer stated more weight than the entries brought: it
			// holds layers this node lacks.
			if f.stated.Greater(h.ownWeight(pi)) {
				return h.topUpLayers(pi, p)
			}
			return nil
		case wire.TypeNotFromHere:
			if searched {
				return errors.New("peer's ledger does not hold the split it sent")
			}
			split, err := h.findSplit(pi, p)
			if err != nil {
				return err
			}
			searched = true
			q = wire.ExtensionReq{RequestCommon: h.common(), From: split, Beat: h.ownWeight(pi), Variant: wire.EntriesWithLayers}
		default:
			return nil // nothing heavier, not held or refused
		}
	}
	return errors.New("catch-up took too many requests")
}

// fetch sends an extension or full ledger request and takes in the
// entries of the reply as they arrive, in chain order.
func (h *Host) fetch(pi *poolInfo, p *peer, q wire.Request) (fetched, error) {
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
			p.close(err)
			return err
		}
		for _, it := range c.Items {
			if it.Entry.Parent != parent {
				err := fmt.Errorf("%w: entries out of chain order", wire.ErrMalformed)
				p.close(err)
				return err
			}
			eh, err := h.takeItem(pi, p, it)
			if err != nil {
				return err
			}
			parent = eh
			f.last = eh
			f.count++
		}
		return nil
	})
	return f, err
}

// takeItem takes in one requested entry and its layers.
func (h *Host) takeItem(pi *poolInfo, p *peer, it wire.EntryItem) (hash.Hash, error) {
	eh := it.Entry.Hash(pi.s)
	_, err := h.n.HandleEntryFrom(pi.key, it.Entry, engine.Requested, p.id)
	switch {
	case err == nil, errors.Is(err, engine.ErrSeen):
	case errors.Is(err, ledger.ErrInvalidEntry):
		err = fmt.Errorf("sent an invalid entry: %w", err)
		p.close(err)
		return eh, err
	default:
		// The first entry that cannot be kept ends what is kept from the
		// reply; the ones before it stay.
		return eh, fmt.Errorf("entry %s: %w", eh.Short(), err)
	}
	return eh, h.takeLayers(pi, p, eh, it.Nonces)
}

func (h *Host) takeLayers(pi *poolInfo, p *peer, eh hash.Hash, nonces []uint64) error {
	for _, n := range nonces {
		_, err := h.n.HandleLayerFrom(pi.key, layer.Layer{Entry: eh, Nonce: n}, engine.Requested, p.id)
		if errors.Is(err, ledger.ErrWeakLayer) {
			err = fmt.Errorf("sent a layer below the minimum strength: %w", err)
			p.close(err)
			return err
		}
	}
	return nil
}

// findSplit asks p for hash chains, newest first, until it finds the
// newest hash this node holds. It returns the pool_id when the two ledgers
// share no entry.
func (h *Host) findSplit(pi *poolInfo, p *peer) (hash.Hash, error) {
	limit := h.cfg.Limits.HashLimit
	q := wire.HashChainReq{RequestCommon: h.common(), Count: limit}
	for round := 0; round < maxRounds; round++ {
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
				p.close(err)
				return err
			}
			if m.Chunk == 1 {
				fromStart = c.FromStart
			}
			hs = append(hs, c.Hashes...)
			return nil
		})
		if err != nil {
			return hash.Zero, err
		}
		if kind != wire.TypeHashChain {
			return hash.Zero, errStop // the peer switched or will not say; ask again later
		}
		if len(hs) == 0 {
			return pi.s.PoolID(pi.key.Index), nil
		}
		split, found := hash.Zero, false
		h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
			for i := len(hs) - 1; i >= 0; i-- {
				if l.Has(hs[i]) {
					split, found = hs[i], true
					return
				}
			}
		})
		if found {
			return split, nil
		}
		if fromStart {
			return pi.s.PoolID(pi.key.Index), nil
		}
		q = wire.HashChainReq{RequestCommon: h.common(), HasAnchor: true, Anchor: hs[0], Dir: wire.Before, Count: limit}
	}
	return hash.Zero, errors.New("split not found")
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
	return h.ask(p, pi, q, func(m wire.Message) error {
		if m.Type != wire.TypeLayers {
			return wire.DecodeEmpty(m.Type, m.Body)
		}
		c, err := wire.DecodeLayersChunk(m.Body)
		if err != nil {
			p.close(err)
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
}
