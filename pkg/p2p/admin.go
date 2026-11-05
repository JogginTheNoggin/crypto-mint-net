package p2p

import (
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// The owner's controls over peers, for the node's local interface. Each
// takes an address as the owner gives it; the tracker keys it the way it
// keys connections (see tracking.Tracker.Key).

// connectedAs returns the connected peers matching f.
func (h *Host) connectedAs(f func(*peer) bool) []*peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*peer
	for _, p := range h.peers {
		if f(p) {
			out = append(out, p)
		}
	}
	return out
}

func (h *Host) byKeyOf(addr string) []*peer {
	key := h.tr.Key(addr)
	return h.connectedAs(func(p *peer) bool { return p.key == key })
}

// TimeOut pauses addr for d (0: the configured length). Its connections
// close, keeping their categories for when it returns. It returns how many
// connections closed.
func (h *Host) TimeOut(addr string, d time.Duration) int {
	h.tr.TimeOutFor(addr, d, time.Now())
	ps := h.byKeyOf(addr)
	if len(ps) > 0 {
		h.act(ps[0], tracking.TimeOut)
	}
	return len(ps)
}

// Block refuses addr for d (0: the configured length; negative: until
// lifted), or its whole network range when wholeRange is set. Its
// connections close. It returns how many closed.
func (h *Host) Block(addr string, d time.Duration, wholeRange bool) int {
	now := time.Now()
	if wholeRange {
		rng := h.tr.BlockRange(addr, d, now)
		ps := h.connectedAs(func(p *peer) bool { return remoteRange(p) == rng })
		for _, p := range ps {
			p.close(errBlocked)
		}
		return len(ps)
	}
	h.tr.BlockFor(addr, d, now)
	ps := h.byKeyOf(addr)
	if len(ps) > 0 {
		h.act(ps[0], tracking.Block)
	}
	return len(ps)
}

// Unblock ends a timeout or block of addr and its range. It returns as a
// candidate, when it connects again or is dialled.
func (h *Host) Unblock(addr string) { h.tr.Lift(addr) }

// DeletePeer takes addr out of the peer book and drops what the tracker
// kept about it, closing its connections. Nothing stops it returning as a
// new candidate; block it for that. It reports whether the book held it.
func (h *Host) DeletePeer(addr string) bool {
	held := h.book.Delete(addr)
	h.tr.Clear(addr, "all")
	for _, p := range h.byKeyOf(addr) {
		p.close(errDeleted)
	}
	return held
}

// AddPeer puts addr in the peer book by the owner's hand, as a candidate
// for every mint policy this node follows, with label (D157). The
// connection manager dials it when a policy is short of peers;
// ConnectNow dials it at once. A blocked address is refused, and so is a
// local one unless the node allows local peers or lists it.
func (h *Host) AddPeer(addr, label string) error {
	if err := peers.CheckLabel(label); err != nil {
		return err
	}
	if !h.learnable(addr) && !h.configured[addr] {
		return errNotPublic
	}
	if err := h.book.Add(addr, h.coverage, peers.FromHand, time.Now()); err != nil {
		return err
	}
	if label != "" {
		if _, err := h.book.SetLabel(addr, label); err != nil {
			return err
		}
	}
	return h.book.Save()
}

// LabelPeer sets addr's label in the peer book ("" clears it). It reports
// whether the book holds addr.
func (h *Host) LabelPeer(addr, label string) (bool, error) {
	ok, err := h.book.SetLabel(addr, label)
	if err == nil && ok {
		err = h.book.Save()
	}
	return ok, err
}

// ConnectImported makes every unlisted address from an imported peer
// file a candidate and runs the connection manager, which dials them as
// policies have room (D157). It returns how many it made candidates.
func (h *Host) ConnectImported() int {
	n := h.book.PromoteImported(wire.All, 0)
	if n > 0 {
		h.spawn(h.Manage)
	}
	return n
}

// Listed returns the addresses the node policy lists (p2p.peers) and the
// ones found to lead back to this node: an imported block list never
// blocks them (D157).
func (h *Host) Listed() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := append([]string(nil), h.cfg.Peers...)
	for a := range h.selfAddrs {
		out = append(out, a)
	}
	if a := h.advertised(); a != "" {
		out = append(out, a)
	}
	return out
}

// AllowsLocalPeers reports the node policy's p2p.allow_local_peers.
func (h *Host) AllowsLocalPeers() bool { return h.cfg.AllowLocalPeers }

// SetPeerTemplate chooses addr's own tracking template ("" for its groups
// and the default).
func (h *Host) SetPeerTemplate(addr, name string) error {
	return h.tr.SetTemplate(addr, name, time.Now())
}

// ClearPeer clears addr's flags, measures or all of its record.
func (h *Host) ClearPeer(addr, what string) error { return h.tr.Clear(addr, what) }

// TrackerReport describes addr as the tracker sees it.
func (h *Host) TrackerReport(addr string) (tracking.Report, bool) {
	return h.tr.Report(addr, time.Now())
}

// Tracked lists every tracked key with its status, and the blocked ranges.
func (h *Host) Tracked() (map[string]tracking.Status, map[string]time.Time) {
	now := time.Now()
	out := map[string]tracking.Status{}
	for _, k := range h.tr.Keys() {
		out[k] = h.tr.Status(k, now)
	}
	return out, h.tr.Ranges()
}
