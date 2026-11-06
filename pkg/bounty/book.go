package bounty

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// Config is the node policy of the market (nodepolicy's "bounty"
// section). The zero value posts nothing and takes nothing.
type Config struct {
	// Post lets this node post offers; Take lets it hold others' offers
	// and claim them.
	Post bool
	Take bool
	// MaxOpen bounds this node's open offers.
	MaxOpen int
	// MaxAmount bounds what one offer may pay in all (amount × count);
	// Budget bounds what all open offers and unpaid claims may owe
	// together. Both must be set before anything is posted.
	MaxAmount uint64
	Budget    uint64
	// MaxLeasesPerGrinder bounds the leases one grinder holds on this
	// node's offers at once; a grinder starts at one and earns one more
	// for each layer it hands in, up to this.
	MaxLeasesPerGrinder int
	// LeaseFloor and LeaseCeiling bound a lease. A lease is three times
	// the expected time to find the layer at the rate the grinder states.
	LeaseFloor   time.Duration
	LeaseCeiling time.Duration
	// Held bounds the offers from others this node keeps, the oldest
	// dropped first; PerPeer bounds those from one peer.
	Held    int
	PerPeer int
	// MinPrice: offers paying less per layer are not kept.
	MinPrice uint64
}

// Defaults for the zero fields of Config.
const (
	DefaultMaxOpen             = 16
	DefaultMaxLeasesPerGrinder = 4
	DefaultLeaseFloor          = time.Minute
	DefaultLeaseCeiling        = time.Hour
	DefaultHeld                = 256
	DefaultPerPeer             = MaxListed
)

func (c *Config) fill() {
	if c.MaxOpen <= 0 {
		c.MaxOpen = DefaultMaxOpen
	}
	if c.MaxLeasesPerGrinder <= 0 {
		c.MaxLeasesPerGrinder = DefaultMaxLeasesPerGrinder
	}
	if c.LeaseFloor <= 0 {
		c.LeaseFloor = DefaultLeaseFloor
	}
	if c.LeaseCeiling <= 0 {
		c.LeaseCeiling = DefaultLeaseCeiling
	}
	if c.LeaseCeiling < c.LeaseFloor {
		c.LeaseCeiling = c.LeaseFloor
	}
	if c.Held <= 0 {
		c.Held = DefaultHeld
	}
	if c.PerPeer <= 0 {
		c.PerPeer = DefaultPerPeer
	}
}

// Lease is an offer held by one grinder until Until.
type Lease struct {
	Peer     string    `json:"peer"`
	PayTo    hash.Hash `json:"pay_to"`
	Rate     uint64    `json:"rate"`
	Until    time.Time `json:"until"`
	Extended bool      `json:"extended,omitempty"`
}

// ClaimRecord is a layer handed in and accepted.
type ClaimRecord struct {
	Peer     string      `json:"peer"`
	Nonce    uint64      `json:"nonce"`
	PayTo    hash.Hash   `json:"pay_to"`
	Strength int         `json:"strength"`
	At       time.Time   `json:"at"`
	Paid     []hash.Hash `json:"paid,omitempty"`
}

// Mine is an offer this node posted.
type Mine struct {
	ID     hash.Hash     `json:"id"`
	Offer  Offer         `json:"offer"`
	Posted time.Time     `json:"posted"`
	Closed bool          `json:"closed,omitempty"`
	Leases []Lease       `json:"leases,omitempty"`
	Claims []ClaimRecord `json:"claims,omitempty"`
}

// Owed is what the offer may still have to pay: the unpaid accepted
// layers, and while it is open the layers not yet handed in.
func (m *Mine) Owed() uint64 {
	n := uint64(0)
	for _, c := range m.Claims {
		if len(c.Paid) == 0 {
			n++
		}
	}
	if !m.Closed {
		n += uint64(m.Offer.Count) - uint64(min(len(m.Claims), int(m.Offer.Count)))
	}
	return n * m.Offer.Amount
}

func (m *Mine) free() int { return int(m.Offer.Count) - len(m.Claims) - len(m.Leases) }

func (m *Mine) lease(peer string) int {
	return slices.IndexFunc(m.Leases, func(l Lease) bool { return l.Peer == peer })
}

func (m *Mine) expired(now time.Time) bool {
	return m.Offer.OpenSeconds > 0 && now.After(m.Posted.Add(time.Duration(m.Offer.OpenSeconds)*time.Second))
}

// Held is an offer from a peer this node keeps, with what it did with it.
type Held struct {
	ID       hash.Hash `json:"id"`
	Offer    Offer     `json:"offer"`
	From     string    `json:"from"`
	Received time.Time `json:"received"`
	// Lease is what the payer granted, if anything.
	Lease *HeldLease `json:"lease,omitempty"`
	// Acks are the payer's answers to this node's claims.
	Acks []Ack `json:"acks,omitempty"`
}

// HeldLease is a lease this node holds on a peer's offer.
type HeldLease struct {
	PayTo  hash.Hash `json:"pay_to"`
	Until  time.Time `json:"until"`
	Reason string    `json:"reason,omitempty"`
}

// PayerRecord is what this node saw of one payer: layers it accepted, and
// how many of those it paid.
type PayerRecord struct {
	Accepted int `json:"accepted"`
	Paid     int `json:"paid"`
}

// grinder is what this node saw of one grinder on its own offers.
type grinder struct {
	filled, expired int
}

// Book is a node's market: the offers it posted, with their leases and
// claims, and the offers peers sent it.
type Book struct {
	mu       sync.Mutex
	cfg      Config
	now      func() time.Time
	path     string
	mine     map[hash.Hash]*Mine
	order    []hash.Hash
	held     map[hash.Hash]*Held
	heldOrd  []hash.Hash
	grinders map[string]*grinder
	payers   map[string]*PayerRecord
}

// FileName is the file, in the node's data directory, that keeps the
// offers a node posted, so a restart keeps every accepted claim to pay.
const FileName = "bounties.json"

// FileKind and FileFormat name that file.
const (
	FileKind   = brand.KindPrefix + "bounties"
	FileFormat = 1
)

type file struct {
	Kind   string  `json:"kind"`
	Format int     `json:"format"`
	Offers []*Mine `json:"offers"`
}

// Open returns the book whose posted offers are kept in path ("" keeps
// them in memory only).
func Open(cfg Config, path string) (*Book, error) {
	cfg.fill()
	b := &Book{cfg: cfg, now: time.Now, path: path, mine: map[hash.Hash]*Mine{}, held: map[hash.Hash]*Held{},
		grinders: map[string]*grinder{}, payers: map[string]*PayerRecord{}}
	if path == "" {
		return b, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Kind != FileKind || f.Format != FileFormat {
		return nil, fmt.Errorf("%s: not a %s file of format %d", path, FileKind, FileFormat)
	}
	for _, m := range f.Offers {
		if m == nil || m.ID != m.Offer.ID() {
			continue
		}
		b.mine[m.ID] = m
		b.order = append(b.order, m.ID)
	}
	return b, nil
}

// SetClock replaces the clock; tests use it.
func (b *Book) SetClock(now func() time.Time) { b.mu.Lock(); b.now = now; b.mu.Unlock() }

// Config returns the book's settings.
func (b *Book) Config() Config { return b.cfg }

// save writes the posted offers. Called with mu held.
func (b *Book) save() error {
	if b.path == "" {
		return nil
	}
	f := file{Kind: FileKind, Format: FileFormat}
	for _, id := range b.order {
		f.Offers = append(f.Offers, b.mine[id])
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFile(b.path, append(raw, '\n'), 0o600)
}

// expire drops leases that ran out and peers' offers past their time.
// Called with mu held.
func (b *Book) expire() {
	now := b.now()
	for _, m := range b.mine {
		m.Leases = slices.DeleteFunc(m.Leases, func(l Lease) bool {
			if now.After(l.Until) {
				b.grinderOf(l.Peer).expired++
				return true
			}
			return false
		})
		if !m.Closed && m.expired(now) {
			m.Closed, m.Leases = true, nil
		}
	}
	b.heldOrd = slices.DeleteFunc(b.heldOrd, func(id hash.Hash) bool {
		h := b.held[id]
		if h.Offer.OpenSeconds > 0 && now.After(h.Received.Add(time.Duration(h.Offer.OpenSeconds)*time.Second)) {
			delete(b.held, id)
			return true
		}
		return false
	})
}

func (b *Book) grinderOf(peer string) *grinder {
	g := b.grinders[peer]
	if g == nil {
		g = &grinder{}
		b.grinders[peer] = g
	}
	return g
}

// ErrOff is a market action this node's policy does not allow.
var ErrOff = errors.New("bounty: off in this node's policy")

// Post adds an offer this node will pay for. A zero Salt is filled with a
// random one, so the same request twice makes two offers.
func (b *Book) Post(o Offer) (Mine, error) {
	if err := o.Check(); err != nil {
		return Mine{}, err
	}
	if o.Salt == 0 {
		var s [8]byte
		_, _ = rand.Read(s[:])
		o.Salt = binary.BigEndian.Uint64(s[:]) | 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.cfg.Post {
		return Mine{}, fmt.Errorf("%w: set bounty.post to post offers", ErrOff)
	}
	b.expire()
	total, over := mulCheck(o.Amount, uint64(o.Count))
	if over || total > b.cfg.MaxAmount {
		return Mine{}, fmt.Errorf("bounty: the offer pays %d × %d, over bounty.max_amount %d", o.Amount, o.Count, b.cfg.MaxAmount)
	}
	open, owed := 0, uint64(0)
	for _, m := range b.mine {
		if !m.Closed {
			open++
		}
		owed += m.Owed()
	}
	if open >= b.cfg.MaxOpen {
		return Mine{}, fmt.Errorf("bounty: %d offers open, the most bounty.max_open allows", open)
	}
	if owed+total > b.cfg.Budget || owed+total < owed {
		return Mine{}, fmt.Errorf("bounty: open offers and unpaid layers owe %d; this one would take it over bounty.budget %d", owed, b.cfg.Budget)
	}
	m := &Mine{ID: o.ID(), Offer: o, Posted: b.now()}
	if _, dup := b.mine[m.ID]; dup {
		return Mine{}, errors.New("bounty: that offer is posted already")
	}
	b.mine[m.ID] = m
	b.order = append(b.order, m.ID)
	if err := b.save(); err != nil {
		return Mine{}, err
	}
	return clone(m), nil
}

func mulCheck(a, b uint64) (uint64, bool) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, true
	}
	return a * b, false
}

// Cancel closes an offer: no new lease or claim. Layers it accepted are
// still owed.
func (b *Book) Cancel(id hash.Hash) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.mine[id]
	if m == nil {
		return errors.New("bounty: no such offer of this node")
	}
	m.Closed, m.Leases = true, nil
	return b.save()
}

// Mine returns this node's offers, oldest first.
func (b *Book) Mine() []Mine {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	out := make([]Mine, 0, len(b.order))
	for _, id := range b.order {
		out = append(out, clone(b.mine[id]))
	}
	return out
}

// Get returns one of this node's offers.
func (b *Book) Get(id hash.Hash) (Mine, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	m := b.mine[id]
	if m == nil {
		return Mine{}, false
	}
	return clone(m), true
}

func clone(m *Mine) Mine {
	c := *m
	c.Leases = slices.Clone(m.Leases)
	c.Claims = slices.Clone(m.Claims)
	for i := range c.Claims {
		c.Claims[i].Paid = slices.Clone(c.Claims[i].Paid)
	}
	return c
}

// Listable returns the offers a peer may take now, in series (all when
// zero): open, with a layer neither handed in nor leased. A leased offer
// is not listed, so only its grinder works on it.
func (b *Book) Listable(series hash.Hash) []Offer {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	var out []Offer
	for _, id := range b.order {
		m := b.mine[id]
		if m.Closed || m.free() <= 0 || (series != hash.Hash{} && m.Offer.Series != series) {
			continue
		}
		out = append(out, m.Offer)
		if len(out) == MaxListed {
			break
		}
	}
	return out
}

// ExpectedSeconds is the expected time to find a layer of strength bits
// at rate tries a second.
func ExpectedSeconds(strength uint8, rate uint64) float64 {
	if rate == 0 {
		return math.Inf(1)
	}
	return math.Ldexp(1, int(strength)) / float64(rate)
}

// Take answers a grinder's take: a new lease, the one extension of a
// lease it holds, or a refusal with its reason.
func (b *Book) Take(peer string, t Take) Grant {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	g := Grant{Offer: t.Offer}
	refuse := func(why string) Grant { g.Reason = why; return g }
	m := b.mine[t.Offer]
	switch {
	case m == nil || m.Closed:
		return refuse("no such open offer")
	case t.Rate == 0:
		return refuse("no rate stated")
	}
	exp := ExpectedSeconds(m.Offer.Strength, t.Rate)
	dur := time.Duration(math.Min(3*exp, b.cfg.LeaseCeiling.Seconds()) * float64(time.Second))
	dur = max(dur, b.cfg.LeaseFloor)
	if i := m.lease(peer); i >= 0 {
		l := &m.Leases[i]
		if l.Extended {
			return refuse("the lease was extended once already")
		}
		l.Until, l.Extended = l.Until.Add(dur), true
		g.Seconds = uint32(max(0, l.Until.Sub(b.now())).Seconds())
		_ = b.save()
		return g
	}
	if m.free() <= 0 {
		return refuse("leased or filled")
	}
	held, work := 0, exp
	for _, o := range b.mine {
		if i := o.lease(peer); i >= 0 {
			held++
			work += ExpectedSeconds(o.Offer.Strength, o.Leases[i].Rate)
		}
	}
	allowed := min(b.cfg.MaxLeasesPerGrinder, 1+b.grinderOf(peer).filled)
	if held >= allowed {
		return refuse(fmt.Sprintf("lease limit %d reached; each layer handed in earns one more, up to %d", allowed, b.cfg.MaxLeasesPerGrinder))
	}
	if work > b.cfg.LeaseCeiling.Seconds() {
		return refuse("the stated rate does not cover another lease")
	}
	m.Leases = append(m.Leases, Lease{Peer: peer, PayTo: t.PayTo, Rate: t.Rate, Until: b.now().Add(dur)})
	_ = b.save()
	g.Seconds = uint32(dur.Seconds())
	return g
}

// Claim answers a grinder's claim. strength is the claimed layer's
// strength on the offer's entry, worked out by the caller with the
// series' hash. An accepted claim ends the grinder's lease and is kept
// until it is paid.
func (b *Book) Claim(peer string, c Claim, strength int) Ack {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	a := Ack{Offer: c.Offer, Nonce: c.Nonce}
	m := b.mine[c.Offer]
	if m == nil {
		a.Outcome = Closed
		return a
	}
	if slices.ContainsFunc(m.Claims, func(r ClaimRecord) bool { return r.Nonce == c.Nonce }) {
		a.Outcome = Repeat
		return a
	}
	i := m.lease(peer)
	switch {
	case m.Closed:
		a.Outcome = Closed
	case len(m.Claims) >= int(m.Offer.Count):
		a.Outcome = Filled
	case i >= 0 && m.Leases[i].PayTo != c.PayTo:
		a.Outcome = Mismatch
	case i < 0 && m.free() <= 0:
		a.Outcome = Reserved
	case strength < int(m.Offer.Strength):
		a.Outcome = TooWeak
	default:
		a.Outcome = Accepted
		if i >= 0 {
			m.Leases = slices.Delete(m.Leases, i, i+1)
		}
		m.Claims = append(m.Claims, ClaimRecord{Peer: peer, Nonce: c.Nonce, PayTo: c.PayTo, Strength: strength, At: b.now()})
		b.grinderOf(peer).filled++
		_ = b.save()
	}
	return a
}

// Paid records the payment of an accepted claim and returns the ack to
// send its grinder, and the grinder's peer.
func (b *Book) Paid(id hash.Hash, nonce uint64, tids []hash.Hash) (Ack, string, error) {
	if len(tids) == 0 || len(tids) > MaxTIDs {
		return Ack{}, "", fmt.Errorf("bounty: a payment has 1 to %d legs", MaxTIDs)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.mine[id]
	if m == nil {
		return Ack{}, "", errors.New("bounty: no such offer of this node")
	}
	i := slices.IndexFunc(m.Claims, func(r ClaimRecord) bool { return r.Nonce == nonce })
	if i < 0 {
		return Ack{}, "", errors.New("bounty: no accepted claim with that nonce")
	}
	if len(m.Claims[i].Paid) > 0 {
		return Ack{}, "", errors.New("bounty: that claim is paid already")
	}
	m.Claims[i].Paid = slices.Clone(tids)
	if err := b.save(); err != nil {
		return Ack{}, "", err
	}
	return Ack{Offer: id, Nonce: nonce, Outcome: Paid, TIDs: slices.Clone(tids)}, m.Claims[i].Peer, nil
}

// Receive keeps an offer a peer sent, if this node takes offers and the
// offer pays at least MinPrice. It reports whether the offer was kept.
func (b *Book) Receive(peer string, o Offer) bool {
	if o.Check() != nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.cfg.Take || o.Amount < b.cfg.MinPrice {
		return false
	}
	b.expire()
	id := o.ID()
	if h, ok := b.held[id]; ok {
		h.From = peer
		return true
	}
	if _, mine := b.mine[id]; mine {
		return false
	}
	from := 0
	oldest := -1
	for i, hid := range b.heldOrd {
		if b.held[hid].From == peer {
			from++
			if oldest < 0 {
				oldest = i
			}
		}
	}
	if from >= b.cfg.PerPeer {
		b.dropHeld(oldest)
	}
	if len(b.heldOrd) >= b.cfg.Held {
		b.dropHeld(0)
	}
	b.held[id] = &Held{ID: id, Offer: o, From: peer, Received: b.now()}
	b.heldOrd = append(b.heldOrd, id)
	return true
}

func (b *Book) dropHeld(i int) {
	delete(b.held, b.heldOrd[i])
	b.heldOrd = slices.Delete(b.heldOrd, i, i+1)
}

// Granted records the payer's answer to this node's take.
func (b *Book) Granted(peer string, g Grant, payTo hash.Hash) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.held[g.Offer]
	if h == nil || h.From != peer {
		return
	}
	if g.Seconds == 0 {
		h.Lease = &HeldLease{Reason: g.Reason}
		return
	}
	h.Lease = &HeldLease{PayTo: payTo, Until: b.now().Add(time.Duration(g.Seconds) * time.Second)}
}

// Acked records the payer's answer to one of this node's claims.
func (b *Book) Acked(peer string, a Ack) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.held[a.Offer]
	if h == nil || h.From != peer {
		return
	}
	p := b.payers[peer]
	if p == nil {
		p = &PayerRecord{}
		b.payers[peer] = p
	}
	switch a.Outcome {
	case Accepted:
		p.Accepted++
	case Paid:
		p.Paid++
	}
	h.Acks = append(h.Acks, a)
	if a.Outcome == Accepted {
		h.Lease = nil
	}
}

// HeldOffers returns the offers peers sent, oldest first.
func (b *Book) HeldOffers() []Held {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	out := make([]Held, 0, len(b.heldOrd))
	for _, id := range b.heldOrd {
		h := *b.held[id]
		if h.Lease != nil {
			l := *h.Lease
			h.Lease = &l
		}
		h.Acks = slices.Clone(h.Acks)
		out = append(out, h)
	}
	return out
}

// HeldOffer returns one offer a peer sent.
func (b *Book) HeldOffer(id hash.Hash) (Held, bool) {
	for _, h := range b.HeldOffers() {
		if h.ID == id {
			return h, true
		}
	}
	return Held{}, false
}

// Payers returns, by peer, what this node saw each payer accept and pay.
func (b *Book) Payers() map[string]PayerRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]PayerRecord{}
	for k, v := range b.payers {
		out[k] = *v
	}
	return out
}
