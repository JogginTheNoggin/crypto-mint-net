package p2p

import (
	"context"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// fairMeasures are the measures a backup and a priority peer both have,
// the only fair way to compare them.
const fairMeasures = tracking.Pings | tracking.RTT | tracking.FirstDeliveries

// ranked describes p for mint policy s as the ranking rules see it.
func (h *Host) ranked(p *peer, s wire.Section, st standing, now time.Time) peers.Ranked {
	text := s.Text()
	status := h.tr.Status(p.key, now)
	return peers.Ranked{
		ID: p.id, Range: remoteRange(p), Outbound: p.outbound, Partial: st.parts != nil,
		Category: st.cat,
		Points:   h.tr.Points(p.key, text, 0, now),
		Fair:     h.tr.Points(p.key, text, fairMeasures, now),
		MayPriority: status.Priority &&
			(h.cfg.DenialLimit <= 0 || h.tr.Denials(p.key, text, now) < uint64(h.cfg.DenialLimit)),
	}
}

// rankedFor lists every connected peer verified for mint policy s, but
// skip.
func (h *Host) rankedFor(s wire.Section, skip *peer, now time.Time) []peers.Ranked {
	var out []peers.Ranked
	for _, o := range h.verifiedFor(s) {
		if o == skip {
			continue
		}
		if st, ok := o.standingFor(s); ok {
			out = append(out, h.ranked(o, s, st, now))
		}
	}
	return out
}

// place gives p, just verified for mint policy s, its category: the one
// it held before a timeout if it still fits, else priority, backup or
// candidate by the ranking rules.
func (h *Host) place(p *peer, s wire.Section, parts []wire.Section, held string, now time.Time) peers.Category {
	others := h.rankedFor(s, p, now)
	r := h.ranked(p, s, standing{cat: peers.CatCandidate, parts: parts}, now)
	cat := h.cfg.Slots.Place(others, r)
	switch held {
	case peers.CatPriority.String():
		if h.cfg.StrictReturn && cat == peers.CatPriority {
			cat = peers.CatBackup
		}
	case peers.CatBackup.String():
		if cat == peers.CatPriority {
			cat = peers.CatBackup // it returns to what it had
		}
	}
	return cat
}

// setCategory records p's category for s, in the peer, the book and the
// tracker's groups.
func (h *Host) setCategory(p *peer, s wire.Section, cat peers.Category, now time.Time) {
	p.mu.Lock()
	st := p.pols[s]
	if st == nil {
		p.mu.Unlock()
		return
	}
	st.cat = cat
	parts := st.parts
	if cat == peers.CatPriority || cat == peers.CatBackup {
		p.listed = true
	}
	p.mu.Unlock()
	h.tr.SetGroups(p.key, p.groups(), now)
	a := p.bookAddr()
	if a == "" {
		return
	}
	secs := parts
	if secs == nil {
		secs = []wire.Section{s}
	}
	for _, sec := range secs {
		if st := cat.State(); st.Listed() {
			h.book.Verify(a, sec, st, now)
		} else {
			h.book.Unverify(a, sec)
		}
	}
}

// Review runs a review of every mint policy now. Run calls it on a timer;
// tests call it directly.
func (h *Host) Review() {
	for _, s := range h.coverage {
		h.review(s)
	}
}

func (h *Host) reviewLoop(ctx context.Context) {
	defer h.wg.Done()
	t := time.NewTicker(h.cfg.ReviewEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.Review()
		}
	}
}

// review archives the peers for s not heard from for too long, then moves
// peers between priority, backup and candidate by points.
func (h *Host) review(s wire.Section) {
	now := time.Now()
	for _, p := range h.verifiedFor(s) {
		st, ok := p.standingFor(s)
		if !ok || (st.cat != peers.CatPriority && st.cat != peers.CatBackup) {
			continue
		}
		heard := h.tr.LastHeard(p.key, s.Text())
		if heard.IsZero() {
			heard = st.since
		}
		if now.Sub(heard) > h.cfg.InactiveAfter && now.Sub(st.since) > h.cfg.InactiveAfter {
			h.drop(p, s, "inactive")
		}
	}
	changes := h.cfg.Slots.Review(h.rankedFor(s, nil, now))
	if len(changes) == 0 {
		return
	}
	ids := make([]string, 0, len(changes))
	for id := range changes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	h.mu.Lock()
	byID := map[string]*peer{}
	for _, id := range ids {
		byID[id] = h.peers[id]
	}
	h.mu.Unlock()
	for _, id := range ids {
		if p := byID[id]; p != nil {
			h.setCategory(p, s, changes[id], now)
			h.log.Info("category changed", "peer", p.short(), "category", changes[id], "section", s)
		}
	}
}

// drop ends p's peering for s: it is archived for s, and a peer left with
// no mint policy is disconnected.
func (h *Host) drop(p *peer, s wire.Section, why string) {
	p.mu.Lock()
	st := p.pols[s]
	delete(p.pols, s)
	delete(p.misses, s)
	left := len(p.pols)
	p.mu.Unlock()
	if st == nil {
		return
	}
	if a := p.bookAddr(); a != "" {
		secs := st.parts
		if secs == nil {
			secs = []wire.Section{s}
		}
		h.book.Archive(a, secs...)
	}
	h.log.Warn("peer archived", "peer", p.short(), "section", s, "why", why)
	if left == 0 {
		p.close(errNoOverlap)
		return
	}
	h.tr.SetGroups(p.key, p.groups(), time.Now())
}

// peersIn returns the connected peers in category c for a mint policy
// overlapping s, best points first.
func (h *Host) peersIn(s wire.Section, c peers.Category) []*peer {
	now := time.Now()
	type scored struct {
		p   *peer
		pts float64
	}
	var out []scored
	for _, p := range h.verifiedFor(s) {
		p.mu.Lock()
		best, text := peers.CatUnlisted, ""
		for sec, st := range p.pols {
			if sec.Covers(s) != wire.None && st.cat < best {
				best, text = st.cat, sec.Text()
			}
		}
		p.mu.Unlock()
		if best == c {
			out = append(out, scored{p, h.tr.Points(p.key, text, 0, now)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].pts != out[j].pts {
			return out[i].pts > out[j].pts
		}
		if out[i].p.outbound != out[j].p.outbound {
			return out[i].p.outbound
		}
		return out[i].p.id < out[j].p.id
	})
	ps := make([]*peer, len(out))
	for i, x := range out {
		ps[i] = x.p
	}
	return ps
}

// askPeer picks a peer to send a request about s to: the best priority
// peer, or a backup only as a fallback when no priority peer is left.
func (h *Host) askPeer(s wire.Section, skip *peer) *peer {
	for _, c := range []peers.Category{peers.CatPriority, peers.CatBackup} {
		for _, p := range h.peersIn(s, c) {
			if p != skip {
				return p
			}
		}
	}
	return nil
}

// keepUnlisted decides whether an inbound connection verified for none
// of this node's mint policies may stay, as a wallet or light node would.
// When the room for them is full, the least active one is closed.
func (h *Host) keepUnlisted(p *peer) bool {
	if h.cfg.UnlistedConns < 0 || p.outbound {
		return false
	}
	h.mu.Lock()
	var unlisted []*peer
	for _, o := range h.peers {
		o.mu.Lock()
		none := len(o.pols) == 0
		o.mu.Unlock()
		if none && !o.outbound {
			unlisted = append(unlisted, o)
		}
	}
	h.mu.Unlock()
	if len(unlisted) <= h.cfg.UnlistedConns {
		return true
	}
	least := p
	for _, o := range unlisted {
		if o.active.Load() < least.active.Load() {
			least = o
		}
	}
	least.close(errUnlisted)
	return least != p
}
