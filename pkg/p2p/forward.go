package p2p

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
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
		switch {
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
// without being asked. An error disconnects the peer.
func (h *Host) handlePushed(p *peer, m wire.Message) error {
	pi := h.poolFor(m.Section)
	if pi == nil {
		return nil // not a pool this node follows
	}
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
		// with a spend, a full pool); none of them is the peer's fault.
		h.n.HandleTxFrom(t, engine.Pushed, p.id)
	case wire.TypeEntry:
		e, err := wire.DecodeEntry(m.Body)
		if err != nil {
			return err
		}
		_, err = h.n.HandleEntryFrom(pi.key, e, engine.Pushed, p.id)
		switch {
		case errors.Is(err, ledger.ErrInvalidEntry):
			return fmt.Errorf("sent an invalid entry: %w", err)
		case errors.Is(err, ledger.ErrUnknownParent):
			// It builds on something this node lacks: ask the sender.
			h.startChase(pi, p)
			return nil
		}
	case wire.TypeLayer:
		l, err := wire.DecodeLayer(m.Body)
		if err != nil {
			return err
		}
		_, err = h.n.HandleLayerFrom(pi.key, l, engine.Pushed, p.id)
		if errors.Is(err, ledger.ErrUnknownEntry) {
			h.startChase(pi, p)
			return nil
		}
	case wire.TypeNotice:
		if _, err := wire.DecodeNotice(m.Body); err != nil {
			return err
		}
	}
	h.checkStated(pi, p, m)
	return nil
}

// checkStated starts a catch-up when a message states a weight above the
// node's own. The stated weight is only a reason to ask.
func (h *Host) checkStated(pi *poolInfo, p *peer, m wire.Message) {
	if m.Stated.Greater(h.ownWeight(pi)) {
		h.startChase(pi, p)
	}
}
