package p2p

import (
	"errors"
	"fmt"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// onKept runs, under the node's lock, for every item the node keeps. It
// only queues messages: it must not block or call the node.
func (h *Host) onKept(ev node.Event) {
	pi := h.byKey[ev.Key]
	if pi == nil {
		return
	}
	hdr := wire.Header{Section: pi.section, Stated: ev.Weight}
	if ev.Forward {
		m := wire.Message{Header: hdr}
		// The size first, so an item too large to push is never encoded
		// here (ORANGE-017).
		switch {
		case ev.Tx != nil && ev.Tx.Size() > h.cfg.PartsOver:
			// A pending transaction this large travels only inside an
			// entry (D117).
			return
		case ev.Entry != nil && ev.Entry.Size() > h.cfg.PartsOver:
			// Too large to push whole: an entry is announced by a notice,
			// and peers fetch it in parts when they chase it (D117).
			m.Type, m.Body = wire.TypeNotice, wire.Notice{Tip: ev.Entry.Hash(pi.s)}.Encode()
		case ev.Tx != nil:
			m.Type, m.Body = wire.TypeTx, ev.Tx.Encode()
		case ev.Entry != nil:
			m.Type, m.Body = wire.TypeEntry, ev.Entry.Encode()
		case ev.Layer != nil:
			m.Type, m.Body = wire.TypeLayer, ev.Layer.Encode()
		default:
			return
		}
		for _, p := range h.peersFor(pi.section, ev.From) {
			p.send(m)
		}
	}
	// Data the node asked for is never forwarded. When it moved the node to
	// another ledger, peers hear of it through a notice instead; so do they
	// after a forwarded item displaced entries they may hold.
	if ev.Switch != nil && !h.cfg.NoNotices && (!ev.Forward || !ev.Switch.Extension()) {
		m := wire.Message{Header: hdr, Body: wire.Notice{Tip: ev.Switch.To}.Encode()}
		m.Type = wire.TypeNotice
		for _, p := range h.peersFor(pi.section, ev.From) {
			p.send(m)
		}
	}
}

// poolFor returns the followed pool a message's section names, or nil.
func (h *Host) poolFor(s wire.Section) *poolInfo {
	if s.Depth < 3 {
		return nil
	}
	pi := h.byPoolID[s.Pool]
	if pi == nil || pi.section.Issuer != s.Issuer || pi.section.Series != s.Series {
		return nil
	}
	return pi
}

// handlePushed takes in a transaction, entry, layer or notice a peer sent
// without being asked. An invalid item is an offence; a malformed message
// disconnects the peer. A new valid item credits the peer with a first
// delivery; one already held counts as a repeat.
func (h *Host) handlePushed(p *peer, m wire.Message) error {
	pi := h.poolFor(m.Section)
	if pi == nil {
		return nil // not a pool this node follows
	}
	now := time.Now()
	text := pi.section.Text()
	var at *hash.Hash // the branch the message names, for the chase queue
	switch m.Type {
	case wire.TypeTx:
		t, err := wire.DecodeTx(m.Body)
		if err != nil {
			return err
		}
		k, err := h.n.Engine().KeyOf(t)
		if err != nil || k != pi.key {
			return fmt.Errorf("%w: transaction outside the section it was sent in", wire.ErrMalformed)
		}
		// A transaction can be refused for many honest reasons (a race
		// with a spend, a full pool); none of them is the peer's fault. A
		// witness that does not satisfy its address is: no honest node
		// forwards one (RED-009).
		_, err = h.n.HandleTxFrom(t, engine.Pushed, p.id)
		if errors.Is(err, tx.ErrWitness) {
			return invalidItem{fmt.Errorf("sent a transaction with a bad witness: %w", err)}
		}
		h.credit(p, text, "tx", err, now)
	case wire.TypeEntry:
		e, err := wire.DecodeEntry(m.Body)
		if err != nil {
			return err
		}
		eh := e.Hash(pi.s)
		if h.cq.IsBad(eh, now) || h.cq.IsBad(e.Parent, now) {
			return nil // a branch shown to be false: dropped at once
		}
		// The selected entries the node reverts to check it are the
		// sender's cost, drawn from its revert budget (RED-011, D111).
		// Over it the entry is not dropped for good: the branch goes to
		// the chase queue, which checks its work before it fetches
		// anything (D116).
		if err := h.spendRevert(pi, p, e); errors.Is(err, errRevertBudget) {
			h.offer(pi, p, m.Stated, &eh)
			return nil
		} else if err != nil {
			return err
		}
		_, err = h.n.HandleEntryFrom(pi.key, e, engine.Pushed, p.id)
		switch {
		case errors.Is(err, ledger.ErrInvalidEntry):
			return invalidItem{fmt.Errorf("sent an invalid entry: %w", err)}
		case errors.Is(err, ledger.ErrUnknownParent):
			// It builds on something this node lacks: the branch to ask
			// the sender about, if it states a heavier ledger.
			h.offer(pi, p, m.Stated, &eh)
			return nil
		}
		h.credit(p, text, "entry", err, now)
		at = &eh
	case wire.TypeLayer:
		l, err := wire.DecodeLayer(m.Body)
		if err != nil {
			return err
		}
		if h.cq.IsBad(l.Entry, now) {
			return nil
		}
		_, err = h.n.HandleLayerFrom(pi.key, l, engine.Pushed, p.id)
		switch {
		case errors.Is(err, ledger.ErrWeakLayer):
			return invalidItem{fmt.Errorf("sent a layer below the minimum strength: %w", err)}
		case errors.Is(err, ledger.ErrUnknownEntry):
			// Not kept or forwarded, but its stated weight can start a
			// chase for the entry it is on.
			h.offer(pi, p, m.Stated, &l.Entry)
			return nil
		case errors.Is(err, ledger.ErrKnownLayer):
			err = engine.ErrSeen
		}
		h.credit(p, text, "layer", err, now)
		at = &l.Entry
	case wire.TypeNotice:
		n, err := wire.DecodeNotice(m.Body)
		if err != nil {
			return err
		}
		if h.cq.IsBad(n.Tip, now) {
			return nil
		}
		at = &n.Tip
	}
	h.checkStated(pi, p, m.Stated, at)
	return nil
}

// credit records the outcome of a pushed item: a first delivery when it
// was new and kept, a repeat when it was held already.
func (h *Host) credit(p *peer, section, kind string, err error, now time.Time) {
	switch {
	case err == nil:
		h.tr.FirstDelivery(p.key, section, kind, now)
	case errors.Is(err, engine.ErrSeen) || errors.Is(err, ledger.ErrKnownEntry):
		h.tr.Item(p.key, section, kind, now)
		if a := h.tr.Repeat(p.key, now); a != tracking.None {
			h.act(p, a)
		}
	}
}

// checkStated sends a stated weight above the node's own to the chase
// queue. The stated weight is only a reason to ask, and never orders the
// queue.
func (h *Host) checkStated(pi *poolInfo, p *peer, stated weight.Weight, at *hash.Hash) {
	h.offer(pi, p, stated, at)
}
