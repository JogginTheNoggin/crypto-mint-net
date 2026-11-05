package p2p

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/registry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// pathTTL is how long a relaying node remembers where a request came from,
// to pass replies back.
const pathTTL = 30 * time.Second

// sentTTL is how long a peer list reply is kept for missing chunk
// requests.
const sentTTL = 2 * time.Minute

// Limits this node sets on its own discovery requests.
const (
	ownListLimit     = 256
	ownPeerListLimit = 64
)

type returnPath struct {
	p     *peer
	until time.Time
}

type sentReply struct {
	chunks []wire.Message
	until  time.Time
}

// answer says how much of s this node follows: all of it, or the parts
// of it listed (sorted), or nothing.
func (h *Host) answer(s wire.Section) (full bool, parts []wire.Section) {
	for _, c := range h.coverage {
		if c.Contains(s) {
			return true, nil
		}
		if s.Contains(c) {
			parts = append(parts, c)
		}
	}
	wire.SortSections(parts)
	return false, parts
}

// contact is this node's offered contact details.
func (h *Host) contact() wire.Contact { return wire.Contact{Addr: h.advertised()} }

// isSelf reports whether addr is one of this node's own addresses.
func (h *Host) isSelf(addr string) bool {
	if addr == "" {
		return false
	}
	if addr == h.advertised() || addr == h.Addr() {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.selfAddrs[addr]
}

// stated is this node's stated weight for the section, when it names a
// followed pool, else zero.
func (h *Host) stated(s wire.Section) weight.Weight {
	if pi := h.poolFor(s); pi != nil {
		return h.ownWeight(pi)
	}
	return weight.Zero
}

// admit runs the cheap checks on a discovery request: its id matches, it
// is new, its time is inside the window, and the peer is within its rate.
// A false result means drop it silently; an error disconnects.
func (h *Host) admit(p *peer, m wire.Message, at uint64) (bool, error) {
	if wire.RequestID(m.Type, m.Section, m.Body) != m.RequestID {
		return false, fmt.Errorf("%w: request id does not match the request", wire.ErrMalformed)
	}
	now := time.Now()
	t := time.Unix(int64(at), 0)
	if at > 1<<62 || now.Sub(t) > h.cfg.RequestWindow || t.Sub(now) > h.cfg.ClockSkew {
		return false, nil
	}
	if !p.limiter.take(now) {
		return false, nil
	}
	return h.reqs.add(m.RequestID, now), nil
}

// discovery handles one peer discovery message. An error disconnects.
func (h *Host) discovery(p *peer, m wire.Message) error {
	switch m.Type {
	case wire.TypeFollowReq:
		return h.handleFollow(p, m)
	case wire.TypePeerListReq:
		return h.handlePeerList(p, m)
	case wire.TypePing:
		return h.handlePing(p, m)
	case wire.TypeMissingChunksReq:
		return h.handleMissing(p, m)
	case wire.TypeDroppedPolicy:
		return h.handleDropped(p, m)
	}
	return h.routeReply(p, m)
}

func (h *Host) handleFollow(p *peer, m wire.Message) error {
	q, err := wire.DecodeFollowReq(m.Body)
	if err != nil {
		return err
	}
	if ok, err := h.admit(p, m, q.Time); !ok {
		return err
	}
	full, parts := h.answer(m.Section)
	if !full && len(parts) == 0 {
		return nil // silence, and no relaying by a node that follows none of it
	}
	if q.Contact.Reachable() && !h.isSelf(q.Contact.Addr) {
		// The asker asks only about what it follows: a candidate.
		h.learn(q.Contact.Addr, []wire.Section{m.Section}, peers.FromReply, time.Now())
	}
	h.replyFollow(p, m, q, full, parts)
	if q.Broadcast && !h.cfg.NoRelay && int(m.Hops) < h.cfg.HopLimit {
		h.relay(p, m)
	}
	return nil
}

// replyFollow sends a yes or partial yes. A direct reply may carry peers
// and come in chunks; a relayed one is a single message without peers.
func (h *Host) replyFollow(p *peer, m wire.Message, q wire.FollowReq, full bool, parts []wire.Section) {
	t := wire.TypeFollowYes
	rep := wire.FollowReply{Contact: h.contact()}
	if !full {
		t = wire.TypePartialYes
		lim := capTo(q.Limits.ListLimit, len(parts))
		rep.Policies = parts[:lim]
		rep.More = lim < len(parts)
	}
	chunk := capTo(q.Limits.ChunkSize, wire.MaxFrame-frameRoom)
	hdr := func(i, n int, more bool) wire.Header {
		var f uint8
		if i == n {
			f = wire.FlagLast
			if more {
				f |= wire.FlagMore
			}
		}
		return wire.Header{Type: t, Section: m.Section, RequestID: m.RequestID, Chunk: uint32(i), Flags: f, Stated: h.stated(m.Section)}
	}
	if m.Hops > 0 {
		// Relayed: one message that fits, cutting the list if it must.
		for len(rep.Encode()) > chunk && len(rep.Policies) > 0 {
			rep.Policies = rep.Policies[:len(rep.Policies)-1]
			rep.More = true
		}
		p.send(wire.Message{Header: hdr(1, 1, false), Body: rep.Encode()})
		return
	}
	var ps []wire.PeerInfo
	if q.Variant != wire.NoPeers && q.Limits.PeerListLimit > 0 {
		ps = h.peerList(m.Section, q.Variant, parts, q.Contact.Addr)
		if len(ps) > int(q.Limits.PeerListLimit) {
			ps = ps[:q.Limits.PeerListLimit]
		}
	}
	bodies := peerChunks(rep, ps, chunk)
	for i, b := range bodies {
		p.send(wire.Message{Header: hdr(i+1, len(bodies), false), Body: b})
	}
}

// peerChunks splits a follow reply's peers over chunks: the first chunk
// holds the reply itself, later ones more peers only.
func peerChunks(first wire.FollowReply, ps []wire.PeerInfo, chunk int) [][]byte {
	var out [][]byte
	cur := first
	for _, pe := range ps {
		next := cur
		next.Peers = append(append([]wire.PeerInfo(nil), cur.Peers...), pe)
		if len(next.Encode()) > chunk && (len(cur.Peers) > 0 || len(out) > 0 || cur.Contact.Reachable() || len(cur.Policies) > 0) {
			out = append(out, cur.Encode())
			cur = wire.FollowReply{Peers: []wire.PeerInfo{pe}}
			continue
		}
		cur = next
	}
	return append(out, cur.Encode())
}

// peerList lists verified peers for s, for the variant, leaving out the
// asker's own address.
func (h *Host) peerList(s wire.Section, v wire.PeerVariant, parts []wire.Section, asker string) []wire.PeerInfo {
	var out []wire.PeerInfo
	for _, pe := range h.book.VerifiedFor(s, v == wire.FullPeers, parts) {
		if pe.Contact.Addr == asker || h.isSelf(pe.Contact.Addr) {
			continue
		}
		if v == wire.FullPeers {
			pe.Policies = nil
		}
		out = append(out, pe)
	}
	return out
}

// relay passes a broadcast follow request on to this node's peers for its
// mint policy, remembering where it came from so replies can go back.
func (h *Host) relay(from *peer, m wire.Message) {
	targets := h.peersFor(m.Section, from.id)
	if len(targets) == 0 {
		return
	}
	h.mu.Lock()
	h.paths[m.RequestID] = returnPath{p: from, until: time.Now().Add(pathTTL)}
	h.mu.Unlock()
	m.Hops++
	for _, t := range targets {
		t.send(m)
	}
}

// routeReply delivers a discovery reply: to a request of this node's own,
// or back along a relay path. Anything else is dropped.
func (h *Host) routeReply(p *peer, m wire.Message) error {
	p.mu.Lock()
	ch, mine := p.calls[m.RequestID]
	p.mu.Unlock()
	if mine {
		select {
		case ch <- m:
		default:
			return errors.New("reply chunks arrive faster than they are read")
		}
		return nil
	}
	now := time.Now()
	h.mu.Lock()
	bc := h.bcast[m.RequestID]
	path, relayed := h.paths[m.RequestID]
	if relayed && now.After(path.until) {
		delete(h.paths, m.RequestID)
		relayed = false
	}
	h.mu.Unlock()
	switch {
	case bc != nil:
		select {
		case bc <- m:
		default: // more replies than this node reads; the rest are dropped
		}
	case relayed && (m.Type == wire.TypeFollowYes || m.Type == wire.TypePartialYes):
		// A relayed reply is one message and carries no peers.
		if m.Chunk != 1 || !m.Last() {
			return nil
		}
		if r, err := wire.DecodeFollowReply(m.Body); err != nil || len(r.Peers) > 0 {
			return nil
		}
		m.Hops++
		path.p.send(m)
	}
	return nil
}

func (h *Host) handlePeerList(p *peer, m wire.Message) error {
	q, err := wire.DecodePeerListReq(m.Body)
	if err != nil {
		return err
	}
	if ok, err := h.admit(p, m, q.Time); !ok {
		return err
	}
	full, parts := h.answer(m.Section)
	if !full && len(parts) == 0 {
		return nil
	}
	ps := h.peerList(m.Section, q.Variant, parts, "")
	if len(ps) > int(q.Limits.PeerListLimit) {
		ps = ps[:q.Limits.PeerListLimit]
	}
	chunk := capTo(q.Limits.ChunkSize, wire.MaxFrame-frameRoom)
	var bodies [][]byte
	var cur wire.PeerListChunk
	for _, pe := range ps {
		next := wire.PeerListChunk{Peers: append(append([]wire.PeerInfo(nil), cur.Peers...), pe)}
		if len(next.Encode()) > chunk && len(cur.Peers) > 0 {
			bodies = append(bodies, cur.Encode())
			cur = wire.PeerListChunk{Peers: []wire.PeerInfo{pe}}
			continue
		}
		cur = next
	}
	bodies = append(bodies, cur.Encode())
	var msgs []wire.Message
	for i, b := range bodies {
		var f uint8
		if i == len(bodies)-1 {
			f = wire.FlagLast
		}
		msgs = append(msgs, wire.Message{Header: wire.Header{
			Type: wire.TypePeerList, Section: m.Section, RequestID: m.RequestID, Chunk: uint32(i + 1), Flags: f,
			Stated: h.stated(m.Section),
		}, Body: b})
	}
	now := time.Now()
	h.mu.Lock()
	for id, s := range h.sent {
		if now.After(s.until) {
			delete(h.sent, id)
		}
	}
	h.sent[m.RequestID] = sentReply{chunks: msgs, until: now.Add(sentTTL)}
	h.mu.Unlock()
	for _, msg := range msgs {
		p.send(msg)
	}
	return nil
}

func (h *Host) handleMissing(p *peer, m wire.Message) error {
	q, err := wire.DecodeMissingChunksReq(m.Body)
	if err != nil {
		return err
	}
	if !p.limiter.take(time.Now()) {
		return nil
	}
	h.mu.Lock()
	s, ok := h.sent[m.RequestID]
	h.mu.Unlock()
	if !ok || time.Now().After(s.until) {
		return nil // no longer held: silence
	}
	// Each chunk at most once per request, whatever the list repeats; a
	// repeat counts against the peer, and what is resent is its cost
	// (RED-002).
	now := time.Now()
	resent := make(map[uint32]bool, len(s.chunks))
	repeated, bytes := false, 0
	for _, pos := range q.Positions {
		switch {
		case resent[pos]:
			repeated = true
		case int(pos) <= len(s.chunks):
			resent[pos] = true
			bytes += len(s.chunks[pos-1].Body)
			p.send(s.chunks[pos-1])
		}
	}
	if bytes > 0 {
		h.tr.Cost(p.key, m.Section.Text(), bytes, 0, false, now)
	}
	if repeated {
		if a := h.tr.Repeat(p.key, now); a != tracking.None {
			h.act(p, a)
		}
	}
	return nil
}

func (h *Host) handlePing(p *peer, m wire.Message) error {
	q, err := wire.DecodePing(m.Body)
	if err != nil {
		return err
	}
	if pi := h.poolFor(m.Section); pi != nil {
		h.checkStated(pi, p, m.Stated, nil)
	}
	if ok, err := h.admit(p, m, q.Time); !ok {
		return err
	}
	full, parts := h.answer(m.Section)
	rep := wire.PongReply{Contact: h.contact()}
	t := wire.TypePong
	switch {
	case full:
	case len(parts) > 0:
		t, rep.Policies = wire.TypePartialPong, parts
	default:
		return nil
	}
	p.send(wire.Message{Header: wire.Header{
		Type: t, Section: m.Section, RequestID: m.RequestID, Chunk: 1, Flags: wire.FlagLast, Stated: h.stated(m.Section),
	}, Body: rep.Encode()})
	return nil
}

// handleDropped takes a peer's notice that it stopped following a mint
// policy. A verified policy that holds the dropped one but is wider stays
// verified: sections cannot express "all but this part", and a later ping
// shows what the peer still follows.
func (h *Host) handleDropped(p *peer, m wire.Message) error {
	if err := wire.DecodeEmpty(m.Type, m.Body); err != nil {
		return err
	}
	for _, s := range p.policies() {
		if m.Section.Contains(s) {
			h.drop(p, s, "dropped policy update")
			continue
		}
		// A dropped part of a partly followed policy goes; the rest stays.
		p.mu.Lock()
		if st := p.pols[s]; st != nil && st.parts != nil {
			keep := st.parts[:0]
			for _, part := range st.parts {
				if !m.Section.Contains(part) {
					keep = append(keep, part)
				}
			}
			st.parts = keep
			if len(keep) == 0 {
				p.mu.Unlock()
				h.drop(p, s, "dropped policy update")
				continue
			}
		}
		p.mu.Unlock()
	}
	if a := p.bookAddr(); a != "" {
		h.book.Archive(a, m.Section)
	}
	return nil
}

// request sends a discovery request to p and passes each reply chunk to
// f until the last. It reports false when the peer stayed silent for
// timeout, which is how a node says no.
func (h *Host) request(p *peer, m wire.Message, timeout time.Duration, f func(wire.Message) error) (bool, error) {
	ch := make(chan wire.Message, callBuffer)
	p.mu.Lock()
	p.calls[m.RequestID] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.calls, m.RequestID)
		p.mu.Unlock()
	}()
	p.send(m)
	t := time.NewTimer(timeout)
	defer t.Stop()
	pos := uint32(0)
	for {
		select {
		case r, ok := <-ch:
			if !ok {
				return false, errClosed
			}
			pos++
			if r.Chunk != pos {
				err := fmt.Errorf("%w: %s chunk %d out of order", wire.ErrMalformed, r.Type, r.Chunk)
				p.close(err)
				return false, err
			}
			if err := f(r); err != nil {
				return true, err
			}
			if r.Last() {
				return true, nil
			}
			t.Reset(timeout)
		case <-t.C:
			return pos > 0, nil
		case <-p.done:
			return false, errClosed
		}
	}
}

// discoveryMsg builds a discovery request message.
func discoveryMsg(t wire.Type, s wire.Section, body []byte, stated weight.Weight) wire.Message {
	return wire.Message{Header: wire.Header{Type: t, Section: s, RequestID: wire.RequestID(t, s, body), Stated: stated}, Body: body}
}

// followResult is what a follow request learned.
type followResult struct {
	answered bool
	policies []wire.Section // verified: s itself, or the parts listed
	contact  string
	peers    []wire.PeerInfo
}

// askFollow sends p a direct follow request about s.
func (h *Host) askFollow(p *peer, s wire.Section, v wire.PeerVariant) (followResult, error) {
	q := wire.FollowReq{Nonce: nonce(), Time: uint64(time.Now().Unix()), Variant: v, Contact: h.contact(),
		Limits: wire.DiscoveryLimits{ChunkSize: h.cfg.Limits.ChunkSize, ListLimit: ownListLimit}}
	if v != wire.NoPeers {
		q.Limits.PeerListLimit = ownPeerListLimit
	}
	var res followResult
	m := discoveryMsg(wire.TypeFollowReq, s, q.Encode(), h.stated(s))
	h.reqs.add(m.RequestID, time.Now())
	answered, err := h.request(p, m, h.cfg.FollowTimeout, func(r wire.Message) error {
		if r.Type != wire.TypeFollowYes && r.Type != wire.TypePartialYes {
			return fmt.Errorf("%w: %s in reply to a follow request", wire.ErrMalformed, r.Type)
		}
		rep, err := wire.DecodeFollowReply(r.Body)
		if err != nil {
			return err
		}
		if r.Chunk == 1 {
			res.contact = rep.Contact.Addr
			if r.Type == wire.TypeFollowYes {
				res.policies = []wire.Section{s}
			}
			for _, ps := range rep.Policies {
				if !s.Contains(ps) || r.Type == wire.TypeFollowYes {
					return fmt.Errorf("%w: reply lists a mint policy outside the one asked about", wire.ErrMalformed)
				}
				res.policies = append(res.policies, ps)
			}
		}
		res.peers = append(res.peers, rep.Peers...)
		return nil
	})
	res.answered = answered && len(res.policies) > 0
	if err != nil && errors.Is(err, wire.ErrMalformed) {
		p.close(err)
	}
	return res, err
}

// verifyPeer asks a new connection, for each mint policy this node
// follows, whether it follows it. The answers place it in a category for
// those policies. An outbound connection that follows none of them, or
// finds no slot, is closed; an inbound one may stay as an unlisted node,
// such as a wallet.
func (h *Host) verifyPeer(p *peer) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	results := make([]followResult, len(h.coverage))
	for i, s := range h.coverage {
		wg.Add(1)
		go func(i int, s wire.Section) {
			defer wg.Done()
			res, err := h.askFollow(p, s, wire.NoPeers)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			results[i] = res
		}(i, s)
	}
	wg.Wait()
	if firstErr != nil && (errors.Is(firstErr, wire.ErrMalformed) || errors.Is(firstErr, errClosed)) {
		return firstErr
	}
	now := time.Now()
	held := h.tr.Held(p.key, h.cfg.PingEvery, now)
	for i, s := range h.coverage {
		if results[i].answered {
			h.peerFollows(p, s, results[i], held[s.Text()], now)
		}
	}
	h.tr.SetGroups(p.key, p.groups(), now)
	if len(p.policies()) == 0 {
		if h.keepUnlisted(p) {
			return nil
		}
		return errNoOverlap
	}
	if p.outbound && !p.covers(wire.All, true) {
		if a := p.bookAddr(); a != "" {
			h.book.Tried(a, false, now) // no slot: try it again later
		}
		return errNoSlot
	}
	// Ask the new peer how heavy its ledgers are; a heavier one is chased.
	var mine []*poolInfo
	for _, pi := range h.sortedPools() {
		if p.covers(pi.section, false) {
			mine = append(mine, pi)
		}
	}
	h.sweepWeights(p, mine)
	return nil
}

// peerFollows records what a peer answered for mint policy s and places
// it in a category.
func (h *Host) peerFollows(p *peer, s wire.Section, res followResult, held string, now time.Time) {
	var parts []wire.Section
	if len(res.policies) != 1 || res.policies[0] != s {
		parts = append(parts, res.policies...)
		wire.SortSections(parts)
	}
	cat := h.place(p, s, parts, held, now)
	p.mu.Lock()
	p.pols[s] = &standing{cat: peers.CatCandidate, parts: parts, since: now}
	p.mu.Unlock()
	h.setCategory(p, s, cat, now)
	h.tr.Heard(p.key, s.Text(), now)
	if res.contact != "" && !h.isSelf(res.contact) {
		p.mu.Lock()
		p.contact = res.contact
		p.mu.Unlock()
		if !p.outbound {
			h.learn(res.contact, res.policies, peers.FromReply, now)
		}
	}
}

// remoteRange is the network range of a peer's connection.
func remoteRange(p *peer) string { return peers.Range(p.c.RemoteAddr().String()) }

// pingLoop pings priority peers every PingEvery and backup peers every
// BackupPingEvery, for each mint policy. Candidates and unlisted nodes
// are not pinged.
func (h *Host) pingLoop(ctx context.Context) {
	defer h.wg.Done()
	t := time.NewTicker(h.cfg.PingEvery)
	defer t.Stop()
	last := map[string]time.Time{} // peer and policy → last backup ping
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now()
			h.mu.Lock()
			ps := make([]*peer, 0, len(h.peers))
			for _, p := range h.peers {
				ps = append(ps, p)
			}
			h.mu.Unlock()
			for _, p := range ps {
				for _, s := range p.policies() {
					st, _ := p.standingFor(s)
					switch st.cat {
					case peers.CatPriority:
					case peers.CatBackup:
						k := p.id + " " + s.Text()
						if now.Sub(last[k]) < h.cfg.BackupPingEvery-h.cfg.PingEvery/2 {
							continue
						}
						last[k] = now
					default:
						continue
					}
					h.spawn(func() {
						h.ping(p, s)
					})
				}
			}
		}
	}
}

// Ping pings every priority and backup peer for each mint policy now and
// waits for the answers. Tests call it directly.
func (h *Host) Ping() {
	var wg sync.WaitGroup
	h.mu.Lock()
	ps := make([]*peer, 0, len(h.peers))
	for _, p := range h.peers {
		ps = append(ps, p)
	}
	h.mu.Unlock()
	for _, p := range ps {
		for _, s := range p.policies() {
			if st, _ := p.standingFor(s); st.cat == peers.CatPriority || st.cat == peers.CatBackup {
				wg.Add(1)
				go func(p *peer, s wire.Section) {
					defer wg.Done()
					h.ping(p, s)
				}(p, s)
			}
		}
	}
	wg.Wait()
}

// ping checks that p still runs and follows mint policy s, and ends the
// peering for s after MaxMisses silences in a row.
func (h *Host) ping(p *peer, s wire.Section) {
	q := wire.Ping{Nonce: nonce(), Time: uint64(time.Now().Unix())}
	m := discoveryMsg(wire.TypePing, s, q.Encode(), h.stated(s))
	h.reqs.add(m.RequestID, time.Now())
	var narrowed []wire.Section
	partial := false
	start := time.Now()
	answered, err := h.request(p, m, h.cfg.FollowTimeout, func(r wire.Message) error {
		if pi := h.poolFor(s); pi != nil {
			h.checkStated(pi, p, r.Stated, nil)
		}
		switch r.Type {
		case wire.TypePong:
		case wire.TypePartialPong:
			rep, err := wire.DecodePongReply(r.Body)
			if err != nil {
				return err
			}
			partial = true
			for _, ps := range rep.Policies {
				if s.Contains(ps) {
					narrowed = append(narrowed, ps)
				}
			}
		default:
			return fmt.Errorf("%w: %s in reply to a ping", wire.ErrMalformed, r.Type)
		}
		return nil
	})
	if errors.Is(err, errClosed) {
		return
	}
	if err != nil {
		h.misbehave(p, err)
		return
	}
	now := time.Now()
	h.tr.Ping(p.key, s.Text(), answered, now.Sub(start), now)
	gone := false
	p.mu.Lock()
	if st := p.pols[s]; st != nil {
		if answered {
			p.misses[s] = 0
			if partial {
				if len(narrowed) == 0 {
					gone = true
				} else {
					st.parts = narrowed
				}
			} else {
				st.parts = nil
			}
		} else {
			p.misses[s]++
			gone = p.misses[s] >= h.cfg.MaxMisses
		}
	}
	p.mu.Unlock()
	if gone {
		h.drop(p, s, "no longer answers pings for it")
		return
	}
	if a := p.bookAddr(); a != "" && answered {
		h.book.Heard(a, s, now)
	}
}

// manageLoop keeps enough verified peers for each followed mint policy.
func (h *Host) manageLoop(ctx context.Context) {
	defer h.wg.Done()
	t := time.NewTicker(h.cfg.ManageEvery)
	defer t.Stop()
	for {
		h.manage(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Manage runs the connection manager once: for each mint policy short of
// peers it dials candidates, searches the archive, asks its peers and
// loads the registries. Run calls it on a timer; tests call it directly.
func (h *Host) Manage() { h.manage(h.ctx) }

func (h *Host) manage(ctx context.Context) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	defer func() {
		if err := h.book.Save(); err != nil {
			h.log.Error("saving the peer book", "err", err)
		}
	}()
	now := time.Now()
	for _, s := range h.coverage {
		have := h.peersFor(s, "")
		need := h.cfg.Slots.Priority + h.cfg.Slots.Backup - len(have)
		if need <= 0 {
			continue
		}
		skip, room := h.dialState()
		if need > room {
			need = room
		}
		cands := h.book.Pick(s, need, skip, now)
		if len(cands) == 0 && h.book.Unarchive(s) > 0 {
			cands = h.book.Pick(s, need, skip, now)
		}
		// Then what an imported peer file gave, a few at a time (D157).
		if len(cands) < need && !h.cfg.NoDialImported && h.book.PromoteImported(s, need-len(cands)) > 0 {
			cands = h.book.Pick(s, need, skip, now)
		}
		for _, a := range cands {
			h.dialCandidate(ctx, a)
		}
		if len(cands) >= need {
			continue
		}
		// Short of candidates: ask around, at most once per period.
		h.mu.Lock()
		askNow := now.Sub(h.lastBcast[s]) >= h.cfg.BroadcastEvery
		if askNow {
			h.lastBcast[s] = now
		}
		regNow := len(h.cfg.Registries) > 0 && now.Sub(h.lastReg) >= h.cfg.RegistryEvery
		if regNow {
			h.lastReg = now
		}
		h.mu.Unlock()
		if p := h.askPeer(s, nil); askNow && p != nil {
			h.spawn(func() {
				h.askPeerList(p, s)
				h.Broadcast(s)
			})
		}
		if regNow {
			h.spawn(func() {
				h.LoadRegistries(ctx)
			})
		}
	}
}

// dialState returns the addresses not to dial (connected, being dialled,
// this node's own) and how many more outbound connections may start.
func (h *Host) dialState() (map[string]bool, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	skip := map[string]bool{}
	out := 0
	for a := range h.dialing {
		skip[a] = true
		out++
	}
	for _, p := range h.peers {
		skip[p.addr] = true
		p.mu.Lock()
		if p.contact != "" {
			skip[p.contact] = true
		}
		p.mu.Unlock()
		if p.outbound && !h.dialing[p.addr] {
			out++
		}
	}
	for a := range h.selfAddrs {
		skip[a] = true
	}
	if a := h.advertised(); a != "" {
		skip[a] = true
	}
	if h.ln != nil {
		skip[h.ln.Addr().String()] = true
	}
	room := h.cfg.MaxOutbound - out
	if room < 0 {
		room = 0
	}
	return skip, room
}

// dialCandidate connects to a candidate in the background and runs the
// connection; the follow requests decide whether it becomes a peer.
func (h *Host) dialCandidate(ctx context.Context, addr string) {
	h.mu.Lock()
	if h.dialing[addr] {
		h.mu.Unlock()
		return
	}
	h.dialing[addr] = true
	h.mu.Unlock()
	h.spawn(func() {
		defer func() {
			h.mu.Lock()
			delete(h.dialing, addr)
			h.mu.Unlock()
		}()
		if !h.tr.Status(addr, time.Now()).Allowed() {
			return // timed out or blocked: not until it ends
		}
		c, err := h.candidateDialer(addr).DialContext(ctx, "tcp", addr)
		if err != nil {
			h.book.Tried(addr, false, time.Now())
			return
		}
		ready := make(chan error, 1)
		remote := c.RemoteAddr().String()
		go func() {
			err := <-ready
			h.book.Tried(addr, err == nil, time.Now())
			if err == nil {
				// A host name counts in the range it led to (RED-067).
				if errors.Is(h.book.Resolved(addr, remote, time.Now()), peers.ErrSameMachine) {
					h.log.Info("peer name leads to a full network range; forgotten", "addr", addr, "via", remote)
				}
			}
			if errors.Is(err, errSelf) {
				h.mu.Lock()
				h.selfAddrs[addr] = true
				h.mu.Unlock()
				h.book.Archive(addr)
			}
		}()
		h.runConnNotify(c, true, addr, ready)
	})
}

// Broadcast sends a follow request about s over every open connection,
// for followers to answer and relay, and adds the repliers that offer
// contact details to the book as candidates. It waits twice the follow
// timeout for replies and returns how many candidates it added.
func (h *Host) Broadcast(s wire.Section) int {
	q := wire.FollowReq{Nonce: nonce(), Time: uint64(time.Now().Unix()), Broadcast: true, Contact: h.contact(),
		Limits: wire.DiscoveryLimits{ChunkSize: h.cfg.Limits.ChunkSize, ListLimit: ownListLimit}}
	m := discoveryMsg(wire.TypeFollowReq, s, q.Encode(), h.stated(s))
	h.reqs.add(m.RequestID, time.Now())
	ch := make(chan wire.Message, 256)
	h.mu.Lock()
	h.bcast[m.RequestID] = ch
	ps := make([]*peer, 0, len(h.peers))
	for _, p := range h.peers {
		ps = append(ps, p)
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.bcast, m.RequestID)
		h.mu.Unlock()
	}()
	for _, p := range ps {
		p.send(m)
	}
	added := 0
	t := time.NewTimer(2 * h.cfg.FollowTimeout)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			return added
		case r := <-ch:
			if r.Type != wire.TypeFollowYes && r.Type != wire.TypePartialYes {
				continue
			}
			rep, err := wire.DecodeFollowReply(r.Body)
			if err != nil || !rep.Contact.Reachable() || h.isSelf(rep.Contact.Addr) {
				continue
			}
			pols := []wire.Section{s}
			if r.Type == wire.TypePartialYes {
				pols = nil
				for _, ps := range rep.Policies {
					if s.Contains(ps) {
						pols = append(pols, ps)
					}
				}
			}
			if len(pols) > 0 && h.learn(rep.Contact.Addr, pols, peers.FromReply, time.Now()) == nil {
				added++
			}
		}
	}
}

// askPeerList asks p directly for its peers for s and adds them to the
// book as candidates.
func (h *Host) askPeerList(p *peer, s wire.Section) int {
	q := wire.PeerListReq{Nonce: nonce(), Time: uint64(time.Now().Unix()), Variant: wire.AllPeers,
		Limits: wire.DiscoveryLimits{ChunkSize: h.cfg.Limits.ChunkSize, PeerListLimit: ownPeerListLimit}}
	m := discoveryMsg(wire.TypePeerListReq, s, q.Encode(), h.stated(s))
	h.reqs.add(m.RequestID, time.Now())
	added := 0
	h.request(p, m, h.cfg.FollowTimeout, func(r wire.Message) error {
		if r.Type != wire.TypePeerList {
			return fmt.Errorf("%w: %s in reply to a peer list request", wire.ErrMalformed, r.Type)
		}
		c, err := wire.DecodePeerListChunk(r.Body)
		if err != nil {
			return err
		}
		for _, pe := range c.Peers {
			if h.isSelf(pe.Contact.Addr) {
				continue
			}
			pols := []wire.Section{s}
			if len(pe.Policies) > 0 {
				pols = nil
				for _, ps := range pe.Policies {
					if ps.Covers(s) != wire.None {
						pols = append(pols, ps)
					}
				}
			}
			if len(pols) > 0 && h.learn(pe.Contact.Addr, pols, peers.FromPeerList, time.Now()) == nil {
				added++
			}
		}
		return nil
	})
	return added
}

// registryClient returns the client that fetches the registry at loc.
// Unless the policy allows local peers, it refuses, after name resolution
// and on every redirect, to connect to an address that is not public, as
// a book candidate's dialer does (RED-043, D150): a registry location
// never makes the node send a request into its own network, such as a
// cloud metadata address or a service on loopback. A registry reached
// through a proxy the environment names is the proxy's to resolve.
func (h *Host) registryClient(loc string) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	switch {
	case h.cfg.AllowLocalPeers:
	case viaProxy(tr, loc):
		// The proxy resolves the name; a literal address is checked here.
		if u, err := url.Parse(loc); err == nil {
			if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicIP(ip) {
				return nil, errNotPublic
			}
		}
	default:
		tr.Proxy, tr.DialContext = nil, publicDialer().DialContext
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: tr}, nil
}

// viaProxy reports whether tr sends a request for loc through a proxy.
func viaProxy(tr *http.Transport, loc string) bool {
	if tr.Proxy == nil {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, loc, nil)
	if err != nil {
		return false
	}
	u, err := tr.Proxy(req)
	return err == nil && u != nil
}

// LoadRegistries reads every configured registry and adds the nodes listed
// for this node's mint policies to the book as candidates.
func (h *Host) LoadRegistries(ctx context.Context) int {
	added := 0
	for _, loc := range h.cfg.Registries {
		client, err := h.registryClient(loc)
		var es []registry.Entry
		if err == nil {
			es, err = registry.Fetch(ctx, client, loc)
		}
		if err != nil {
			h.log.Warn("registry failed", "location", loc, "err", err)
			continue
		}
		for _, e := range es {
			if h.isSelf(e.Addr) {
				continue
			}
			var pols []wire.Section
			for _, f := range e.Follows {
				for _, c := range h.coverage {
					if f.Covers(c) != wire.None {
						pols = append(pols, c)
					}
				}
			}
			if len(pols) > 0 && h.learn(e.Addr, pols, peers.FromRegistry, time.Now()) == nil {
				added++
			}
		}
	}
	return added
}

// DropPolicy tells this node's peers for s that it no longer follows it.
// Nothing changes locally: what a node follows is its node policy.
func (h *Host) DropPolicy(s wire.Section) {
	for _, p := range h.peersFor(s, "") {
		p.send(wire.Message{Header: wire.Header{Type: wire.TypeDroppedPolicy, Section: s}})
	}
}
