package p2p

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// The bounty market (D196). The host carries its messages between peers
// that list the "bounty" feature; the book (package bounty) decides. A
// node whose policy neither posts nor takes has no book, does not list
// the feature, and ignores the messages. Nothing here touches what any
// node accepts into a ledger: an accepted claim's layer is handed to the
// engine like any layer a peer sends.

// BountyWait bounds how long a take or claim waits for the payer's
// answer.
var BountyWait = 10 * time.Second

// bountyWaits are the takes and claims waiting for their answers.
type bountyWaits struct {
	mu     sync.Mutex
	grants map[hash.Hash]chan bounty.Grant
	takes  map[hash.Hash]bounty.Take
	acks   map[bountyAckKey]chan bounty.Ack
}

type bountyAckKey struct {
	offer hash.Hash
	nonce uint64
}

// ErrNoBounty is a market call on a node whose policy has the market off.
var ErrNoBounty = errors.New("the bounty market is off: poo policy limits -set bounty.take=1 takes peers' offers; -set bounty.post=1 with bounty.max_amount and bounty.budget posts them")

// ErrNoTake is asking for or working peers' offers on a node that only
// posts: it keeps no peer's offer, so the answers would be dropped
// (GREEN-062).
var ErrNoTake = fmt.Errorf("%w: this node does not take peers' offers; poo policy limits -set bounty.take=1 turns it on", bounty.ErrOff)

// ErrPayerGone is a take or claim whose payer is not connected.
var ErrPayerGone = errors.New("the peer that sent the offer is not connected")

// Bounty returns the node's bounty book, or nil when the market is off.
func (h *Host) Bounty() *bounty.Book { return h.cfg.Bounty }

// features is what this node lists in its hello: its build's features,
// and "bounty" when its policy takes part in the market.
func (h *Host) features() []wire.Feature {
	if h.cfg.Bounty == nil {
		return wire.OwnFeatures
	}
	fs := append([]wire.Feature{{Name: wire.FeatureBounty, Ver: 1}}, wire.OwnFeatures...)
	slices.SortFunc(fs, func(a, b wire.Feature) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}
		return 0
	})
	return fs
}

// bountyPeers returns the connected peers whose hellos list the feature.
func (h *Host) bountyPeers() []*peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*peer
	for _, p := range h.peers {
		if wire.Has(p.hello.Features, wire.FeatureBounty, 1) {
			out = append(out, p)
		}
	}
	return out
}

func (h *Host) bountyPeer(id string) *peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.peers[id]
	if p == nil || !wire.Has(p.hello.Features, wire.FeatureBounty, 1) {
		return nil
	}
	return p
}

func bountyMsg(t wire.Type, body []byte) wire.Message {
	m := wire.Message{Body: body}
	m.Type = t
	return m
}

// PostBounty posts an offer and sends it to every connected peer that
// takes part. It returns the offer and how many peers it went to.
func (h *Host) PostBounty(o bounty.Offer) (bounty.Mine, int, error) {
	b := h.cfg.Bounty
	if b == nil {
		return bounty.Mine{}, 0, ErrNoBounty
	}
	if _, ok := h.n.Engine().Series(o.Series); !ok {
		return bounty.Mine{}, 0, fmt.Errorf("this node does not follow series %s", o.Series.Short())
	}
	if h.poolByKey(engine.PoolKey{Series: o.Series, Index: o.Pool}) == nil {
		return bounty.Mine{}, 0, fmt.Errorf("this node does not follow pool %d of series %s", o.Pool, o.Series.Short())
	}
	if !h.holdsEntry(engine.PoolKey{Series: o.Series, Index: o.Pool}, o.Entry) {
		// A layer on an entry the pool does not hold is not kept, yet an
		// accepted claim for it is still owed (GREEN-061).
		hint := ""
		if ks := h.BountyPools(o.Series, o.Entry); len(ks) > 0 {
			hint = fmt.Sprintf("; it is in pool %d", ks[0])
		}
		return bounty.Mine{}, 0, fmt.Errorf("pool %d of series %s does not hold entry %s, so a layer on it would be paid for and not kept%s", o.Pool, o.Series.Short(), o.Entry.Short(), hint)
	}
	if s, _ := h.n.Engine().Series(o.Series); int(o.Strength) < int(s.Params().MinLayerStrength) {
		return bounty.Mine{}, 0, fmt.Errorf("strength %d is under the series' layer floor %d, so no node would keep the layer", o.Strength, s.Params().MinLayerStrength)
	}
	m, err := b.Post(o)
	if err != nil {
		return bounty.Mine{}, 0, err
	}
	sent := 0
	for _, p := range h.bountyPeers() {
		p.send(bountyMsg(wire.TypeBountyOffer, m.Offer.Encode()))
		sent++
	}
	return m, sent, nil
}

// holdsEntry reports whether the pool k holds the entry e.
func (h *Host) holdsEntry(k engine.PoolKey, e hash.Hash) bool {
	held := false
	h.n.Engine().Ledger(k, func(l *ledger.Pool) { held = l.Has(e) })
	return held
}

// holdsLayer reports whether the pool k holds the layer of nonce on the
// entry e.
func (h *Host) holdsLayer(k engine.PoolKey, e hash.Hash, nonce uint64) bool {
	held := false
	h.n.Engine().Ledger(k, func(l *ledger.Pool) { held = l.HasLayer(e, nonce) })
	return held
}

// hostOf is the key the market limits a network address by: addr's IPv4
// address, its IPv6 /64 (one site may hold every address in one,
// RED-116), or its host name when it is not an address.
func hostOf(addr string) string {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return (&net.IPNet{IP: ip.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}).String()
}

// BountyPools returns, in order, the followed pools of series that hold
// the entry e: what poo bounty post offers on when no pool is named
// (GREEN-061).
func (h *Host) BountyPools(series, e hash.Hash) []uint64 {
	var ks []uint64
	for _, pi := range h.sortedPools() {
		if pi.key.Series == series && h.holdsEntry(pi.key, e) {
			ks = append(ks, pi.key.Index)
		}
	}
	return ks
}

// AskBounties asks every connected peer that takes part for its open
// offers in series (all when zero); the answers arrive as they come. It
// returns how many peers were asked.
func (h *Host) AskBounties(series hash.Hash) (int, error) {
	if h.cfg.Bounty == nil {
		return 0, ErrNoBounty
	}
	if !h.cfg.Bounty.Config().Take {
		return 0, ErrNoTake
	}
	n := 0
	for _, p := range h.bountyPeers() {
		p.send(bountyMsg(wire.TypeBountyListReq, bounty.ListReq{Series: series}.Encode()))
		n++
	}
	return n, nil
}

// TakeBounty asks the payer of an offer this node holds for a lease, and
// waits for the answer.
func (h *Host) TakeBounty(ctx context.Context, id hash.Hash, rate uint64, payTo hash.Hash) (bounty.Grant, error) {
	b := h.cfg.Bounty
	if b == nil {
		return bounty.Grant{}, ErrNoBounty
	}
	if !b.Config().Take {
		return bounty.Grant{}, ErrNoTake
	}
	held, ok := b.HeldOffer(id)
	if !ok {
		return bounty.Grant{}, errors.New("no offer with that id was received; poo bounty list -ask asks peers for theirs")
	}
	p := h.bountyPeer(held.From)
	if p == nil {
		return bounty.Grant{}, ErrPayerGone
	}
	ch := make(chan bounty.Grant, 1)
	t := bounty.Take{Offer: id, Rate: rate, PayTo: payTo}
	w := &h.bwait
	w.mu.Lock()
	if w.grants == nil {
		w.grants, w.takes = map[hash.Hash]chan bounty.Grant{}, map[hash.Hash]bounty.Take{}
	}
	w.grants[id], w.takes[id] = ch, t
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		if w.grants[id] == ch {
			delete(w.grants, id)
			delete(w.takes, id)
		}
		w.mu.Unlock()
	}()
	p.send(bountyMsg(wire.TypeBountyTake, t.Encode()))
	ctx, cancel := context.WithTimeout(ctx, BountyWait)
	defer cancel()
	select {
	case g := <-ch:
		return g, nil
	case <-ctx.Done():
		return bounty.Grant{}, fmt.Errorf("no answer from the payer: %w", ctx.Err())
	}
}

// ClaimBounty hands the payer a layer for an offer this node holds, and
// waits for the answer.
func (h *Host) ClaimBounty(ctx context.Context, id hash.Hash, nonce uint64, payTo hash.Hash) (bounty.Ack, error) {
	b := h.cfg.Bounty
	if b == nil {
		return bounty.Ack{}, ErrNoBounty
	}
	held, ok := b.HeldOffer(id)
	if !ok {
		return bounty.Ack{}, errors.New("no offer with that id was received")
	}
	p := h.bountyPeer(held.From)
	if p == nil {
		return bounty.Ack{}, ErrPayerGone
	}
	key := bountyAckKey{id, nonce}
	ch := make(chan bounty.Ack, 1)
	w := &h.bwait
	w.mu.Lock()
	if w.acks == nil {
		w.acks = map[bountyAckKey]chan bounty.Ack{}
	}
	w.acks[key] = ch
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		if w.acks[key] == ch {
			delete(w.acks, key)
		}
		w.mu.Unlock()
	}()
	p.send(bountyMsg(wire.TypeBountyClaim, bounty.Claim{Offer: id, Nonce: nonce, PayTo: payTo}.Encode()))
	ctx, cancel := context.WithTimeout(ctx, BountyWait)
	defer cancel()
	select {
	case a := <-ch:
		return a, nil
	case <-ctx.Done():
		return bounty.Ack{}, fmt.Errorf("no answer from the payer: %w", ctx.Err())
	}
}

// PaidBounty records the payment of an accepted claim and tells its
// grinder, if it is connected. It reports whether the grinder was told.
func (h *Host) PaidBounty(id hash.Hash, nonce uint64, tids []hash.Hash) (bool, error) {
	b := h.cfg.Bounty
	if b == nil {
		return false, ErrNoBounty
	}
	a, who, err := b.Paid(id, nonce, tids)
	if err != nil {
		return false, err
	}
	p := h.bountyPeer(who)
	if p == nil {
		return false, nil
	}
	p.send(bountyMsg(wire.TypeBountyAck, a.Encode()))
	return true, nil
}

// handleBounty takes in one bounty message. A message that does not
// decode is malformed; a claim whose layer is under the series' floor is
// counted as an invalid item, since the grinder could check it before
// sending.
func (h *Host) handleBounty(p *peer, m wire.Message) error {
	b := h.cfg.Bounty
	if b == nil {
		return nil
	}
	bad := func(err error) error { return fmt.Errorf("%w: %v", wire.ErrMalformed, err) }
	switch m.Type {
	case wire.TypeBountyOffer:
		o, err := bounty.DecodeOffer(m.Body)
		if err != nil {
			return bad(err)
		}
		if _, ok := h.n.Engine().Series(o.Series); ok {
			b.ReceiveFrom(p.id, hostOf(p.addr), o)
		}
	case wire.TypeBountyListReq:
		q, err := bounty.DecodeListReq(m.Body)
		if err != nil {
			return bad(err)
		}
		if b.Config().Post {
			p.send(bountyMsg(wire.TypeBountyList, bounty.EncodeList(b.Listable(q.Series))))
		}
	case wire.TypeBountyList:
		os, err := bounty.DecodeList(m.Body)
		if err != nil {
			return bad(err)
		}
		for _, o := range os {
			if _, ok := h.n.Engine().Series(o.Series); ok {
				b.ReceiveFrom(p.id, hostOf(p.addr), o)
			}
		}
	case wire.TypeBountyTake:
		t, err := bounty.DecodeTake(m.Body)
		if err != nil {
			return bad(err)
		}
		p.send(bountyMsg(wire.TypeBountyGrant, b.TakeFrom(p.id, hostOf(p.addr), t).Encode()))
	case wire.TypeBountyGrant:
		g, err := bounty.DecodeGrant(m.Body)
		if err != nil {
			return bad(err)
		}
		h.bwait.mu.Lock()
		ch, t := h.bwait.grants[g.Offer], h.bwait.takes[g.Offer]
		h.bwait.mu.Unlock()
		b.Granted(p.id, g, t.PayTo)
		// Only the offer's payer answers the waiting call (RED-111).
		if from, ok := b.HeldFrom(g.Offer); ch != nil && ok && from == p.id {
			select {
			case ch <- g:
			default:
			}
		}
	case wire.TypeBountyClaim:
		return h.handleBountyClaim(p, m)
	case wire.TypeBountyAck:
		a, err := bounty.DecodeAck(m.Body)
		if err != nil {
			return bad(err)
		}
		b.Acked(p.id, a)
		h.bwait.mu.Lock()
		ch := h.bwait.acks[bountyAckKey{a.Offer, a.Nonce}]
		h.bwait.mu.Unlock()
		if from, ok := b.HeldFrom(a.Offer); ch != nil && ok && from == p.id {
			select {
			case ch <- a:
			default:
			}
		}
	}
	return nil
}

func (h *Host) handleBountyClaim(p *peer, m wire.Message) error {
	b := h.cfg.Bounty
	c, err := bounty.DecodeClaim(m.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", wire.ErrMalformed, err)
	}
	mine, ok := b.Get(c.Offer)
	strength, floor := -1, 0
	known := false
	var l layer.Layer
	if ok {
		if s, sok := h.n.Engine().Series(mine.Offer.Series); sok {
			l = layer.Layer{Entry: mine.Offer.Entry, Nonce: c.Nonce}
			strength = l.Strength(s.Hash())
			floor = int(s.Params().MinLayerStrength)
			// A layer the pool holds already is not owed (RED-110).
			known = h.holdsLayer(engine.PoolKey{Series: mine.Offer.Series, Index: mine.Offer.Pool}, mine.Offer.Entry, c.Nonce)
		}
	}
	a := b.Claim(p.id, c, strength, known)
	p.send(bountyMsg(wire.TypeBountyAck, a.Encode()))
	if strength < 0 {
		return nil
	}
	// A layer at or above the series' floor is valid weight whatever the
	// market made of it, so it goes to the engine like any layer a peer
	// sends; one the pool holds already need not (Black, Round 36).
	if strength >= floor && !known {
		k := engine.PoolKey{Series: mine.Offer.Series, Index: mine.Offer.Pool}
		if _, err := h.n.HandleLayerFrom(k, l, engine.Pushed, p.id); err != nil {
			h.log.Debug("bounty layer not kept", "peer", p.short(), "err", err)
		}
	}
	// Only a layer under the series' floor, which no node keeps and the
	// grinder could check before sending, is an offence. One that meets
	// the floor but not the offer is ordinary weight, answered "too weak"
	// (Black, Round 36).
	if strength < floor {
		return invalidItem{fmt.Errorf("bounty claim: a layer of strength %d, under the series' floor %d", strength, floor)}
	}
	return nil
}
