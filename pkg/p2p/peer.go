package p2p

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// helloTimeout bounds the hello exchange.
const helloTimeout = 10 * time.Second

// writeTimeout bounds writing one frame.
const writeTimeout = 30 * time.Second

// callBuffer is how many reply chunks may wait for the goroutine that
// asked. A peer that sends more without the asker keeping up is
// disconnected.
const callBuffer = 1024

var (
	errQueueFull = errors.New("send queue full")
	errSelf      = errors.New("connected to self")
	errDuplicate = errors.New("already connected to this node")
	errNoOverlap = errors.New("no mint policy in common was verified")
	errNoSlot    = errors.New("verified, but no slot is free")
	errTooMany   = errors.New("too many peers")
	errTimeout   = errors.New("request timed out")
	errClosed    = errors.New("connection closed")
	errPaused    = errors.New("address timed out")
	errBlocked   = errors.New("address blocked")
	errUnlisted  = errors.New("room for unlisted connections is full")
)

// invalidItem marks an error as an invalid item: an offence, but not one
// that leaves the connection in doubt.
type invalidItem struct{ err error }

func (e invalidItem) Error() string { return e.err.Error() }
func (e invalidItem) Unwrap() error { return e.err }

// standing is a peer's place for one of this node's mint policies.
type standing struct {
	cat peers.Category
	// parts is what the peer follows of the mint policy; nil: all of it.
	parts []wire.Section
	since time.Time
}

// peer is one open connection after a successful hello.
type peer struct {
	h        *Host
	c        net.Conn
	id       string // the peer's node id, hex
	nodeID   hash.Hash
	addr     string
	key      string // the address it is tracked by
	outbound bool
	hello    wire.Hello
	since    time.Time
	limiter  *bucket
	active   atomic.Int64 // unix nanos of the last message received

	sendq     chan wire.Message
	done      chan struct{}
	closeOnce sync.Once
	errMu     sync.Mutex
	err       error

	sent, received atomic.Uint64

	mu    sync.Mutex
	calls map[hash.Hash]chan wire.Message
	// pols are this node's mint policies the peer was verified for, with
	// its own follow request or ping, and its category for each.
	pols map[wire.Section]*standing
	// contact is the address the peer offered in its replies, if any.
	contact string
	misses  map[wire.Section]int
	// listed is set once the peer has held a priority or backup place.
	listed bool
}

// covers reports whether the peer is verified for any part of s; with
// listedOnly, as a priority or backup peer.
func (p *peer) covers(s wire.Section, listedOnly bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for c, st := range p.pols {
		if listedOnly && st.cat != peers.CatPriority && st.cat != peers.CatBackup {
			continue
		}
		if st.parts == nil {
			if c.Covers(s) != wire.None {
				return true
			}
			continue
		}
		for _, part := range st.parts {
			if part.Covers(s) != wire.None {
				return true
			}
		}
	}
	return false
}

// catFor returns the best category the peer has for a mint policy that
// overlaps s, or unlisted.
func (p *peer) catFor(s wire.Section) peers.Category {
	p.mu.Lock()
	defer p.mu.Unlock()
	best := peers.CatUnlisted
	for c, st := range p.pols {
		if c.Covers(s) != wire.None && st.cat < best {
			best = st.cat
		}
	}
	return best
}

// standingFor returns a copy of the peer's place for mint policy s.
func (p *peer) standingFor(s wire.Section) (standing, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.pols[s]
	if !ok {
		return standing{}, false
	}
	return *st, true
}

// policies returns this node's mint policies the peer was verified for,
// sorted.
func (p *peer) policies() []wire.Section {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]wire.Section, 0, len(p.pols))
	for s := range p.pols {
		out = append(out, s)
	}
	wire.SortSections(out)
	return out
}

// verifiedList returns what the peer was verified to follow, sorted: each
// mint policy, or the parts of it the peer follows.
func (p *peer) verifiedList() []wire.Section {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []wire.Section
	for s, st := range p.pols {
		if st.parts == nil {
			out = append(out, s)
		} else {
			out = append(out, st.parts...)
		}
	}
	wire.SortSections(out)
	return out
}

// groups names the peer groups the peer is in, for choosing its tracking
// template.
func (p *peer) groups() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := map[peers.Category]bool{}
	var out []string
	for _, st := range p.pols {
		if !seen[st.cat] {
			seen[st.cat] = true
			out = append(out, st.cat.String())
		}
	}
	if len(p.pols) == 0 {
		out = append(out, peers.CatUnlisted.String())
	}
	if p.outbound {
		out = append(out, "outbound")
	} else {
		out = append(out, "inbound")
	}
	return out
}

// bookAddr is the address the peer is known by in the book: the one
// dialled for an outbound connection, else none (an inbound peer's offered
// contact is only a candidate).
func (p *peer) bookAddr() string {
	if p.outbound {
		return p.addr
	}
	return ""
}

func (p *peer) info() PeerInfo {
	vs := p.verifiedList()
	cov := make([]string, len(vs))
	for i, s := range vs {
		cov[i] = s.String()
	}
	p.mu.Lock()
	contact := p.contact
	cats := map[string]Standing{}
	var secs []wire.Section
	for s, st := range p.pols {
		cats[s.String()] = Standing{Category: st.cat.String(), Partial: st.parts != nil}
		secs = append(secs, s)
	}
	p.mu.Unlock()
	now := time.Now()
	for _, s := range secs {
		c := cats[s.String()]
		c.Points = p.h.tr.Points(p.key, s.Text(), 0, now)
		cats[s.String()] = c
	}
	return PeerInfo{
		ID: p.id, Addr: p.addr, Key: p.key, Outbound: p.outbound, Agent: p.hello.Agent,
		Listen: contact, Coverage: cov, Categories: cats, Since: p.since,
		Sent: p.sent.Load(), Received: p.received.Load(),
	}
}

func (p *peer) short() string { return p.nodeID.Short() }

// close ends the connection. It never takes the host's lock, so it may be
// called with it held.
func (p *peer) close(err error) {
	p.closeOnce.Do(func() {
		p.errMu.Lock()
		p.err = err
		p.errMu.Unlock()
		close(p.done)
		p.c.Close()
	})
}

func (p *peer) closeErr() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}

func (p *peer) closed() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// send queues m without waiting. A peer whose queue is full is too far
// behind and is disconnected.
func (p *peer) send(m wire.Message) {
	select {
	case <-p.done:
	case p.sendq <- m:
	default:
		p.close(errQueueFull)
	}
}

func (p *peer) writeLoop() {
	w := bufio.NewWriter(p.c)
	for {
		var m wire.Message
		select {
		case <-p.done:
			return
		case m = <-p.sendq:
		}
		p.c.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := wire.WriteFrame(w, m); err != nil {
			p.close(fmt.Errorf("write: %w", err))
			return
		}
		p.sent.Add(1)
		if len(p.sendq) == 0 {
			if err := w.Flush(); err != nil {
				p.close(fmt.Errorf("write: %w", err))
				return
			}
		}
	}
}

// runConn runs a connection until it closes.
func (h *Host) runConn(c net.Conn, outbound bool, addr string) error {
	return h.runConnNotify(c, outbound, addr, nil)
}

// runConnNotify runs a connection until it closes, reporting the hello's
// outcome on ready if it is set.
func (h *Host) runConnNotify(c net.Conn, outbound bool, addr string, ready chan error) error {
	var once sync.Once
	notify := func(err error) {
		once.Do(func() {
			if ready != nil {
				ready <- err
			}
		})
	}
	p, err := h.handshake(c, outbound, addr)
	if err != nil {
		c.Close()
		notify(err)
		return err
	}
	h.logf("connected to %s at %s (outbound %v)", p.short(), addr, outbound)
	go p.writeLoop()
	// The hello proves nothing about what the peer follows: ask it.
	go func() {
		err := h.verifyPeer(p)
		if err != nil {
			p.close(err)
		}
		notify(err)
	}()
	err = h.readLoop(p)
	notify(err)
	p.close(err)
	if p.outbound {
		h.book.Unverify(p.addr)
	}
	h.mu.Lock()
	if h.peers[p.id] == p {
		delete(h.peers, p.id)
	}
	h.mu.Unlock()
	p.mu.Lock()
	for id, ch := range p.calls {
		close(ch)
		delete(p.calls, id)
	}
	freed := make([]wire.Section, 0, len(p.pols))
	for s, st := range p.pols {
		if st.cat == peers.CatPriority || st.cat == peers.CatBackup {
			freed = append(freed, s)
		}
	}
	p.mu.Unlock()
	h.cq.Drop(p.id)
	h.tr.Connection(p.key, "closed", time.Now())
	h.logf("disconnected from %s: %v", p.short(), p.closeErr())
	// A freed slot is filled at once from the peers waiting as candidates.
	if len(freed) > 0 && h.ctx.Err() == nil {
		for _, s := range freed {
			h.review(s)
		}
	}
	return p.closeErr()
}

// handshake exchanges hellos and registers the peer.
func (h *Host) handshake(c net.Conn, outbound bool, addr string) (*peer, error) {
	c.SetDeadline(time.Now().Add(helloTimeout))
	mine := wire.Hello{Protocol: Protocol, NodeID: h.id, Agent: h.cfg.Agent}
	// Write and read at the same time, so neither side waits on the other.
	werr := make(chan error, 1)
	go func() {
		werr <- wire.WriteFrame(c, wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: mine.Encode()})
	}()
	r := bufio.NewReader(c)
	m, _, err := wire.ReadFrame(r, wire.MaxFrame)
	if err != nil {
		return nil, fmt.Errorf("hello: %w", err)
	}
	if err := <-werr; err != nil {
		return nil, fmt.Errorf("hello: %w", err)
	}
	if m.Type != wire.TypeHello {
		return nil, fmt.Errorf("%w: expected a hello, got %s", wire.ErrMalformed, m.Type)
	}
	theirs, err := wire.DecodeHello(m.Body)
	if err != nil {
		return nil, err
	}
	if theirs.Protocol < 1 {
		return nil, fmt.Errorf("protocol %d not spoken", theirs.Protocol)
	}
	if theirs.NodeID == h.id {
		return nil, errSelf
	}
	c.SetDeadline(time.Time{})
	p := &peer{
		h: h, c: &bufConn{Conn: c, r: r}, id: theirs.NodeID.String(), nodeID: theirs.NodeID,
		addr: addr, key: h.tr.Key(c.RemoteAddr().String()), outbound: outbound, hello: theirs, since: time.Now(),
		sendq: make(chan wire.Message, h.cfg.QueueSize), done: make(chan struct{}),
		calls: map[hash.Hash]chan wire.Message{}, pols: map[wire.Section]*standing{},
		misses: map[wire.Section]int{}, limiter: newBucket(h.cfg.RequestRate, time.Now()),
	}
	p.active.Store(time.Now().UnixNano())
	h.tr.Connection(p.key, "opened", time.Now())
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.peers[p.id]; ok {
		// Both ends connected to each other at once. Both keep the
		// connection dialled by the node with the lower id, so they agree.
		if keepNew(h.id, p, old) {
			old.close(errDuplicate)
		} else {
			return nil, errDuplicate
		}
	} else if len(h.peers) >= h.cfg.MaxPeers {
		return nil, errTooMany
	}
	h.peers[p.id] = p
	return p, nil
}

// keepNew decides between two connections to the same node.
func keepNew(self hash.Hash, nw, old *peer) bool {
	dialer := func(p *peer) hash.Hash {
		if p.outbound {
			return self
		}
		return p.nodeID
	}
	dn, do := dialer(nw), dialer(old)
	if dn == do {
		return true // the same side redialled; the old one is stale
	}
	low := self
	if p := nw.nodeID; p.Less(low) {
		low = p
	}
	return dn == low
}

// bufConn reads through the reader that read the hello, so no bytes that
// arrived with it are lost.
type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufConn) Read(p []byte) (int, error) { return b.r.Read(p) }

func (h *Host) readLoop(p *peer) error {
	r := bufio.NewReader(p.c)
	for {
		m, n, err := wire.ReadFrame(r, wire.MaxFrame)
		if err != nil {
			if p.closed() {
				return p.closeErr()
			}
			return err
		}
		p.received.Add(1)
		p.active.Store(time.Now().UnixNano())
		if err := h.dispatch(p, m, n); err != nil {
			h.misbehave(p, err)
			return p.closeErr()
		}
	}
}

// budgetKind is the budget a message counts against.
func budgetKind(t wire.Type) string {
	switch {
	case t == wire.TypeTx:
		return tracking.KindTx
	case t == wire.TypeEntry || t == wire.TypeNotice:
		return tracking.KindEntry
	case t == wire.TypeLayer:
		return tracking.KindLayer
	case t.IsDiscovery():
		return tracking.KindDiscovery
	}
	return tracking.KindRequest
}

// dispatch runs the cheap checks on one message and queues it by the
// sender's category; replies to this node's own requests go straight to
// the request. An error disconnects the peer.
func (h *Host) dispatch(p *peer, m wire.Message, size int) error {
	now := time.Now()
	h.tr.Message(p.key, m.Type.String(), size, now)
	switch {
	case m.Type == wire.TypeHello:
		return fmt.Errorf("%w: second hello", wire.ErrMalformed)
	case m.Type.IsDiscoveryReply():
		return h.routeReply(p, m)
	case m.Type.IsReply():
		p.mu.Lock()
		ch, ok := p.calls[m.RequestID]
		p.mu.Unlock()
		if !ok {
			return nil // a late reply to a request that timed out
		}
		select {
		case ch <- m:
		default:
			return errors.New("reply chunks arrive faster than they are read")
		}
		return nil
	case !m.Type.IsRequest() && !m.Type.IsDiscovery() && !isPushed(m.Type):
		// Unknown types are ignored, so later versions can add messages.
		return nil
	}
	ok, act := h.tr.Budget(p.key, budgetKind(m.Type), now)
	if err := h.act(p, act); err != nil {
		return err
	}
	if !ok {
		return nil // over budget: dropped
	}
	h.inbox.put(p.catFor(m.Section), job{p: p, m: m, size: size})
	return nil
}

func isPushed(t wire.Type) bool {
	return t == wire.TypeTx || t == wire.TypeEntry || t == wire.TypeLayer || t == wire.TypeNotice
}

// handle takes in one queued message. An error is an offence or
// disconnects the peer (see misbehave).
func (h *Host) handle(p *peer, m wire.Message, size int) error {
	switch {
	case isPushed(m.Type):
		if pi := h.poolFor(m.Section); pi != nil && size > 0 {
			h.tr.Cost(p.key, pi.section.Text(), 0, size, false, time.Now())
		}
		return h.handlePushed(p, m)
	case m.Type.IsDiscovery():
		return h.discovery(p, m)
	case m.Type.IsRequest():
		return h.serve(p, m)
	}
	return nil
}

// misbehave turns an error from a peer's message into what happens to the
// peer: an invalid item is a flag, a malformed message a flag and the end
// of the connection, anything else the end of the connection.
func (h *Host) misbehave(p *peer, err error) {
	now := time.Now()
	var inv invalidItem
	switch {
	case errors.As(err, &inv):
		h.logf("%s: %v", p.short(), err)
		if e := h.act(p, h.tr.Offend(p.key, tracking.InvalidItem, err.Error(), now)); e != nil {
			p.close(e)
		}
		return
	case errors.Is(err, wire.ErrMalformed):
		if e := h.act(p, h.tr.Offend(p.key, tracking.InvalidMessage, err.Error(), now)); e != nil {
			err = e
		}
	}
	p.close(err)
}

// act carries out an offence's action on every connection from the
// peer's address, and returns the error that closes them, or nil.
func (h *Host) act(p *peer, a tracking.Action) error {
	var err error
	switch a {
	case tracking.None:
		return nil
	case tracking.TimeOut:
		err = errPaused
	case tracking.Block:
		err = errBlocked
	}
	h.mu.Lock()
	var same []*peer
	for _, o := range h.peers {
		if o.key == p.key {
			same = append(same, o)
		}
	}
	h.mu.Unlock()
	if a == tracking.TimeOut {
		// The peer keeps its categories while it is paused.
		held := map[string]string{}
		for _, o := range same {
			o.mu.Lock()
			for s, st := range o.pols {
				held[s.Text()] = st.cat.String()
			}
			o.mu.Unlock()
		}
		h.tr.Hold(p.key, held, time.Now())
	}
	h.logf("%s at %s: %v", p.short(), p.key, err)
	for _, o := range same {
		o.close(err)
	}
	return err
}

// nonce returns a random request nonce.
func nonce() uint64 {
	var b [8]byte
	rand.Read(b[:])
	return binary.BigEndian.Uint64(b[:])
}

// ask sends a request to p about pool pi and passes each reply chunk to f,
// in order, until the last one. It fails if a chunk does not arrive within
// the request timeout.
func (h *Host) ask(p *peer, pi *poolInfo, q wire.Request, f func(wire.Message) error) (err error) {
	start := time.Now()
	outcome, bytes := tracking.TimedOut, 0
	defer func() {
		if errors.Is(err, errClosed) {
			return
		}
		if errors.Is(err, errTimeout) {
			outcome = tracking.TimedOut
		}
		now := time.Now()
		h.tr.Request(p.key, pi.section.Text(), outcome, now.Sub(start), bytes, now)
		if bytes > 0 {
			h.tr.Cost(p.key, pi.section.Text(), 0, bytes, false, now)
		}
	}()
	body := q.Encode()
	id := wire.RequestID(q.Type(), pi.section, body)
	ch := make(chan wire.Message, callBuffer)
	p.mu.Lock()
	p.calls[id] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.calls, id)
		p.mu.Unlock()
	}()
	p.send(wire.Message{Header: wire.Header{
		Type: q.Type(), Section: pi.section, RequestID: id, Stated: h.ownWeight(pi),
	}, Body: body})
	var first wire.Type
	pos := uint32(0)
	t := time.NewTimer(h.cfg.RequestTimeout)
	defer t.Stop()
	for {
		var m wire.Message
		var ok bool
		select {
		case m, ok = <-ch:
			if !ok {
				return errClosed
			}
		case <-t.C:
			return errTimeout
		case <-p.done:
			return errClosed
		}
		pos++
		if !replyFits(q.Type(), m.Type) || (pos > 1 && m.Type != first) || m.Chunk != pos {
			err := fmt.Errorf("%w: %s chunk %d in reply to %s", wire.ErrMalformed, m.Type, m.Chunk, q.Type())
			h.misbehave(p, err)
			return err
		}
		first = m.Type
		bytes += len(m.Body)
		switch m.Type {
		case wire.TypeRefused:
			outcome = tracking.Refused
		case wire.TypeNotHeld:
			outcome = tracking.NotHeld
		default:
			outcome = tracking.Answered
		}
		if err := f(m); err != nil {
			return err
		}
		if m.Last() {
			return nil
		}
		t.Reset(h.cfg.RequestTimeout)
	}
}

// replyFits reports whether r may answer a request of type q.
func replyFits(q, r wire.Type) bool {
	if r == wire.TypeNotHeld || r == wire.TypeRefused {
		return true
	}
	switch q {
	case wire.TypeExtensionReq:
		return r == wire.TypeExtension || r == wire.TypeNothingHeavier || r == wire.TypeNotFromHere
	case wire.TypeFullLedgerReq:
		return r == wire.TypeExtension
	case wire.TypeHashChainReq:
		return r == wire.TypeHashChain || r == wire.TypeAnchorNotHeld
	case wire.TypeEntriesReq:
		return r == wire.TypeEntries
	case wire.TypeLayersReq:
		return r == wire.TypeLayers
	case wire.TypeWeightReq:
		return r == wire.TypeWeight
	}
	return false
}

// common returns the start of a new request body.
func (h *Host) common() wire.RequestCommon {
	return wire.RequestCommon{Nonce: nonce(), Time: uint64(time.Now().Unix()), Limits: h.cfg.Limits}
}

// ownWeight is the node's computed weight of its active ledger for pi.
func (h *Host) ownWeight(pi *poolInfo) weight.Weight {
	st, err := h.n.Engine().Status(pi.key)
	if err != nil {
		return weight.Zero
	}
	return st.Weight
}

// bucket is a token bucket: rate requests a minute, bursting to rate.
type bucket struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	last   time.Time
}

func newBucket(perMinute int, now time.Time) *bucket {
	return &bucket{rate: float64(perMinute), tokens: float64(perMinute), last: now}
}

// take spends a token, reporting false when there is none.
func (b *bucket) take(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens += now.Sub(b.last).Minutes() * b.rate
	if b.tokens > b.rate {
		b.tokens = b.rate
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
