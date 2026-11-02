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
	errTooMany   = errors.New("too many peers")
	errTimeout   = errors.New("request timed out")
	errClosed    = errors.New("connection closed")
)

// peer is one open connection after a successful hello.
type peer struct {
	h        *Host
	c        net.Conn
	id       string // the peer's node id, hex
	nodeID   hash.Hash
	addr     string
	outbound bool
	hello    wire.Hello
	since    time.Time
	limiter  *bucket

	sendq     chan wire.Message
	done      chan struct{}
	closeOnce sync.Once
	errMu     sync.Mutex
	err       error

	sent, received atomic.Uint64

	mu    sync.Mutex
	calls map[hash.Hash]chan wire.Message
	// verified are the mint policies this node checked, with its own
	// follow request or ping, that the peer follows.
	verified map[wire.Section]bool
	// contact is the address the peer offered in its replies, if any.
	contact string
	misses  map[wire.Section]int
}

// covers reports whether the peer is verified for any part of s.
func (p *peer) covers(s wire.Section) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for c := range p.verified {
		if c.Covers(s) != wire.None {
			return true
		}
	}
	return false
}

// verifiedList returns the peer's verified mint policies, sorted.
func (p *peer) verifiedList() []wire.Section {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]wire.Section, 0, len(p.verified))
	for s := range p.verified {
		out = append(out, s)
	}
	wire.SortSections(out)
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
	p.mu.Unlock()
	return PeerInfo{
		ID: p.id, Addr: p.addr, Outbound: p.outbound, Agent: p.hello.Agent,
		Listen: contact, Coverage: cov, Since: p.since,
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
	p.mu.Unlock()
	h.logf("disconnected from %s: %v", p.short(), p.closeErr())
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
		addr: addr, outbound: outbound, hello: theirs, since: time.Now(),
		sendq: make(chan wire.Message, h.cfg.QueueSize), done: make(chan struct{}),
		calls: map[hash.Hash]chan wire.Message{}, verified: map[wire.Section]bool{},
		misses: map[wire.Section]int{}, limiter: newBucket(h.cfg.RequestRate, time.Now()),
	}
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
		m, _, err := wire.ReadFrame(r, wire.MaxFrame)
		if err != nil {
			if p.closed() {
				return p.closeErr()
			}
			return err
		}
		p.received.Add(1)
		if err := h.dispatch(p, m); err != nil {
			return err
		}
	}
}

// dispatch handles one message. An error disconnects the peer.
func (h *Host) dispatch(p *peer, m wire.Message) error {
	switch {
	case m.Type == wire.TypeHello:
		return fmt.Errorf("%w: second hello", wire.ErrMalformed)
	case m.Type == wire.TypeTx || m.Type == wire.TypeEntry || m.Type == wire.TypeLayer || m.Type == wire.TypeNotice:
		return h.handlePushed(p, m)
	case m.Type.IsDiscovery():
		return h.discovery(p, m)
	case m.Type.IsRequest():
		return h.serve(p, m)
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
	}
	// Unknown types are ignored, so later versions can add messages.
	return nil
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
func (h *Host) ask(p *peer, pi *poolInfo, q wire.Request, f func(wire.Message) error) error {
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
			p.close(err)
			return err
		}
		first = m.Type
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
