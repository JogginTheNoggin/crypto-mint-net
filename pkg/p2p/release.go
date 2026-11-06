package p2p

import (
	"errors"
	"fmt"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Variable supply phase 3 (D168): nodes pass on the newest release of
// each variable series they follow, so a part's pools open at every node
// that follows the series, not only where the owner ran poon series
// add-release. A release is information: one a node has not seen yet
// only leaves the part's transactions waiting there (D166), never
// refused, so passing them on changes how soon a node knows, never what
// it accepts.

// releaseMsg is the message that passes r on.
func (h *Host) releaseMsg(r series.Release) wire.Message {
	m := wire.Message{Body: r.Encode()}
	m.Type, m.Section = wire.TypeRelease, wire.SeriesSection(h.issuers[r.Series], r.Series)
	return m
}

// followedVariable returns the series this host follows that are
// variable, by id.
func (h *Host) followedVariable(id hash.Hash) *series.Series {
	if _, ok := h.issuers[id]; !ok {
		return nil
	}
	s, ok := h.n.Engine().Series(id)
	if !ok || !s.IsVariable() {
		return nil
	}
	return s
}

// sendReleases sends a peer that just connected the newest release of
// every variable series this host follows that has one. A peer that does
// not know the message type ignores it.
func (h *Host) sendReleases(p *peer) {
	for id := range h.issuers {
		s := h.followedVariable(id)
		if s == nil {
			continue
		}
		if r, ok := s.NewestRelease(); ok {
			p.send(h.releaseMsg(r))
		}
	}
}

// Announce passes r on to the connected peers that follow any of its
// series, except the one named by skip (the sender, or "").
func (h *Host) Announce(r series.Release, skip string) {
	m := h.releaseMsg(r)
	for _, p := range h.verifiedFor(m.Section) {
		if p.id != skip {
			p.send(m)
		}
	}
}

// handleRelease takes in a release a peer passed on. A release of a
// series this node does not follow, or of a part it already knows, is
// dropped with no hashing (D165's point 4). A later one has its witness
// checked at once (one signature, no chain), and is then taken as
// takeRelease says, or held a moment so that only the highest of a burst
// is taken (RED-078). A release that does not check is an offence: it
// depends only on the series, so no honest node passes one on.
func (h *Host) handleRelease(p *peer, m wire.Message) error {
	r, err := series.DecodeRelease(m.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", wire.ErrMalformed, err)
	}
	if r.Series != m.Section.Series || m.Section.Depth != 2 {
		return fmt.Errorf("%w: a release outside the section it was sent in", wire.ErrMalformed)
	}
	s := h.followedVariable(r.Series)
	if s == nil || m.Section.Issuer != h.issuers[r.Series] {
		return nil // not a series this node follows
	}
	now := time.Now()
	switch released := s.Released(); {
	case r.Part == released:
		// The release this node holds, crossing with its own on an honest
		// link: nothing to take, and not a repeat (RED-078).
		return nil
	case r.Part < released:
		// Not checked, so not credited; counted as a repeat, so a flood of
		// them is acted on like any other.
		if a := h.tr.Repeat(p.key, now); a != tracking.None {
			return h.act(p, a)
		}
		return nil
	}
	if err := s.CheckReleaseSigned(r); err != nil {
		return invalidItem{fmt.Errorf("passed on a release that does not check: %w", err)}
	}
	// Taken in the series' worker, or held, or covered by a higher one
	// held: the read loop goes on either way (RED-082).
	h.rel.offer(r, p, now, h.workRelease)
	return nil
}

// takeRelease checks r in full and keeps it, credits p, the peer that
// passed it on, with a first delivery, and passes it on. A copy taken by
// another call at the same time is dropped: only the first is credited
// and passed on (ORANGE-036).
func (h *Host) takeRelease(r series.Release, p *peer) error {
	ctx := h.hostContext()
	res, err := h.n.AddReleaseContext(ctx, r)
	switch {
	case err != nil && ctx.Err() != nil:
		// The node is stopping: the check ended, which says nothing of
		// the release.
		return nil
	case errors.Is(err, series.ErrRelease):
		return invalidItem{fmt.Errorf("passed on a release that does not check: %w", err)}
	case err != nil:
		h.log.Warn("release not kept", "series", r.Series.Short(), "part", r.Part, "err", err)
		return nil
	case !res.Added && !res.Taken:
		return nil
	}
	h.tr.FirstDelivery(p.key, wire.SeriesSection(h.issuers[r.Series], r.Series).Text(), "release", time.Now())
	h.log.Info("release kept", "series", r.Series.Short(), "part", r.Part, "released", res.Released, "from", p.short())
	h.Released(r, p.id)
	return nil
}

// workRelease is the series' worker: it takes r unless a release since
// covered it, and acts on the peer that passed r when r does not check.
func (h *Host) workRelease(r series.Release, p *peer) {
	s := h.followedVariable(r.Series)
	if s == nil || r.Part <= s.Released() {
		return
	}
	if r.Part > 1<<20 {
		// Worked out once in a process, in time: say so, so an owner sees
		// what the node is doing (RED-082).
		h.log.Info("checking a release of a far part", "series", r.Series.Short(), "part", r.Part, "from", p.short())
	}
	if err := h.takeRelease(r, p); err != nil {
		h.misbehave(p, err)
	}
}

// Released brings the host up to date after the node took r in place
// (node.AddRelease): it indexes the part's pools and passes r on to its
// peers but skip. When the host cannot index them as it would on start,
// it asks for a reload (Config.Reload), which leaves the node as a start
// onto the same node.json would; it reports whether it applied r itself.
func (h *Host) Released(r series.Release, skip string) bool {
	ok := h.AddPools(r.Series)
	if !ok && h.cfg.Reload != nil {
		h.log.Info("reloading to index a released part's pools", "series", r.Series.Short(), "part", r.Part)
		h.cfg.Reload()
	}
	h.Announce(r, skip)
	return ok
}

// AddPools indexes the followed pools of series id that are not indexed
// yet, after a release added pools to it, as the host indexes them on
// start (New). It reports false, and changes nothing, when that would not
// give what a start gives: the series is followed by a list of pools (its
// coverage can change), or the release makes a series followed whole
// large enough to be indexed by the pools in use (D154). A series this
// host does not follow, or a large one followed whole, needs nothing.
func (h *Host) AddPools(id hash.Hash) bool {
	if _, ok := h.issuers[id]; !ok || h.big[id] != nil {
		return true
	}
	eng := h.n.Engine()
	s, ok := eng.Series(id)
	if !ok {
		return true
	}
	if !eng.FollowsWhole(id) || s.PoolCount() >= engine.ListAll {
		return false
	}
	var add []engine.PoolKey
	eng.EachFollowedPoolOf(id, func(k engine.PoolKey) { add = append(add, k) })
	h.idxMu.Lock()
	defer h.idxMu.Unlock()
	for _, k := range add {
		if h.byKey[k] == nil {
			h.indexLocked(h.makePool(s, k))
		}
	}
	return true
}
