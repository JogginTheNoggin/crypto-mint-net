// Package p2p connects full nodes over TCP. It implements forwarding (the
// design's "Forwarding" document) and ledger data requests ("Ledger
// Data"): a node pushes every item it keeps to its peers that cover the
// pool, serves requests from its active ledger, and catches up when a peer
// states a heavier weight than its own.
//
// The rules it follows:
//   - an item is forwarded only after the node checked and kept it, only to
//     peers whose coverage includes it, and never back to its sender;
//   - data the node asked for is never forwarded; after a switch the node
//     may send a notice (its new tip and stated weight) instead;
//   - every message carries the sender's stated weight, which is only ever
//     a reason to ask, never weight;
//   - requests are answered from the active ledger only, within the
//     asker's limits, in chunks of whole items;
//   - a node asks only by entries it holds or hashes the same peer sent,
//     and switches only on weight it computed itself.
//
// It also finds peers (the design's "Peer Discovery and Management" and
// "Address Registries"): a connection is only a peer for the mint policies
// this node checked with its own follow request; broadcast follow requests
// are relayed by followers with a return path; peers are pinged; and a
// connection manager keeps enough verified peers per mint policy, picking
// candidates from the peer book, peer lists, follow replies, the archive
// and address registries.
//
// Peer ranking, tracking and the full chasing rules are milestone 5. Until
// then a peer that sends a malformed message or an invalid entry is
// disconnected.
package p2p

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Protocol is the protocol version this package speaks in its hello.
const Protocol = 1

// Config holds the p2p settings, all node policy. Zero values mean the
// provisional defaults in DefaultConfig.
type Config struct {
	// Listen is the TCP address for inbound connections; empty means none.
	Listen string
	// Advertise is the address peers are told to connect to; empty means
	// none is given.
	Advertise string
	// Peers are addresses to keep connected to.
	Peers []string
	// MaxPeers caps open connections, inbound and outbound together.
	MaxPeers int
	// Limits are the reply limits this node sets on its own requests.
	Limits wire.Limits
	// ServeEntries and ServeHashes cap what one reply from this node holds,
	// whatever the asker allows.
	ServeEntries int
	ServeHashes  int
	// NoNotices turns off notices: by default a node tells its peers after
	// switching to a ledger they would not hear of through forwarding.
	NoNotices bool
	// CheckEvery is how often to ask a peer for an extension of each pool
	// (negative: never).
	CheckEvery time.Duration
	// RequestTimeout bounds the wait for a whole reply.
	RequestTimeout time.Duration
	// RequestWindow is how long request ids are remembered and how old a
	// request's time may be; ClockSkew is how far ahead it may be.
	RequestWindow time.Duration
	ClockSkew     time.Duration
	// Redial is the first wait before reconnecting to a configured peer;
	// it doubles up to MaxRedial.
	Redial    time.Duration
	MaxRedial time.Duration
	// QueueSize is how many messages may wait to be sent to one peer. A
	// peer that falls further behind is disconnected.
	QueueSize int
	// Agent names this software in hellos.
	Agent string

	// Book is the peer book; nil keeps one in memory.
	Book *peers.Book
	// Registries are address registry locations (URLs or files).
	Registries []string
	// PeersPerPolicy is how many verified peers to keep for each followed
	// mint policy; the connection manager dials candidates below it.
	PeersPerPolicy int
	// MaxOutbound caps connections this node dials itself.
	MaxOutbound int
	// ManageEvery is how often the connection manager runs (negative:
	// never; then only listed peers are dialled).
	ManageEvery time.Duration
	// BroadcastEvery is the least time between two broadcast follow
	// requests for one mint policy; RegistryEvery between two loads of the
	// registries.
	BroadcastEvery time.Duration
	RegistryEvery  time.Duration
	// FollowTimeout is how long to wait for answers to a follow request.
	FollowTimeout time.Duration
	// NoRelay stops this node relaying broadcast follow requests.
	NoRelay bool
	// HopLimit is the hop count at which this node stops relaying.
	HopLimit int
	// RequestRate caps the follow, peer list and ping requests a peer may
	// send per minute; more are dropped.
	RequestRate int
	// PingEvery is how often to ping each peer for each verified mint
	// policy (negative: never); MaxMisses unanswered pings in a row end
	// the peering for that policy.
	PingEvery time.Duration
	MaxMisses int
	// VerifiedPerRange caps the peers for one mint policy from one network
	// range (loopback addresses are exempt).
	VerifiedPerRange int
	// Log receives connection events. Nil discards them.
	Log io.Writer
}

// DefaultConfig returns the provisional defaults.
func DefaultConfig() Config {
	return Config{
		MaxPeers:       32,
		Limits:         wire.Limits{ChunkSize: 1 << 20, EntryLimit: 512, HashLimit: 256, LayerLimit: 1 << 16},
		ServeEntries:   1024,
		ServeHashes:    4096,
		CheckEvery:     time.Minute,
		RequestTimeout: 30 * time.Second,
		RequestWindow:  10 * time.Minute,
		ClockSkew:      2 * time.Minute,
		Redial:         2 * time.Second,
		MaxRedial:      5 * time.Minute,
		QueueSize:      4096,
		Agent:          "poo",

		PeersPerPolicy:   4,
		MaxOutbound:      8,
		ManageEvery:      30 * time.Second,
		BroadcastEvery:   5 * time.Minute,
		RegistryEvery:    time.Hour,
		FollowTimeout:    5 * time.Second,
		HopLimit:         8,
		RequestRate:      120,
		PingEvery:        2 * time.Minute,
		MaxMisses:        3,
		VerifiedPerRange: 4,
	}
}

func (c *Config) fill() {
	d := DefaultConfig()
	if c.MaxPeers == 0 {
		c.MaxPeers = d.MaxPeers
	}
	if c.Limits == (wire.Limits{}) {
		c.Limits = d.Limits
	}
	if c.ServeEntries == 0 {
		c.ServeEntries = d.ServeEntries
	}
	if c.ServeHashes == 0 {
		c.ServeHashes = d.ServeHashes
	}
	if c.CheckEvery == 0 {
		c.CheckEvery = d.CheckEvery
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = d.RequestTimeout
	}
	if c.RequestWindow == 0 {
		c.RequestWindow = d.RequestWindow
	}
	if c.ClockSkew == 0 {
		c.ClockSkew = d.ClockSkew
	}
	if c.Redial == 0 {
		c.Redial = d.Redial
	}
	if c.MaxRedial == 0 {
		c.MaxRedial = d.MaxRedial
	}
	if c.QueueSize == 0 {
		c.QueueSize = d.QueueSize
	}
	if c.Agent == "" {
		c.Agent = d.Agent
	}
	if c.Log == nil {
		c.Log = io.Discard
	}
	if c.PeersPerPolicy == 0 {
		c.PeersPerPolicy = d.PeersPerPolicy
	}
	if c.MaxOutbound == 0 {
		c.MaxOutbound = d.MaxOutbound
	}
	if c.ManageEvery == 0 {
		c.ManageEvery = d.ManageEvery
	}
	if c.BroadcastEvery == 0 {
		c.BroadcastEvery = d.BroadcastEvery
	}
	if c.RegistryEvery == 0 {
		c.RegistryEvery = d.RegistryEvery
	}
	if c.FollowTimeout == 0 {
		c.FollowTimeout = d.FollowTimeout
	}
	if c.HopLimit == 0 {
		c.HopLimit = d.HopLimit
	}
	if c.RequestRate == 0 {
		c.RequestRate = d.RequestRate
	}
	if c.PingEvery == 0 {
		c.PingEvery = d.PingEvery
	}
	if c.MaxMisses == 0 {
		c.MaxMisses = d.MaxMisses
	}
	if c.VerifiedPerRange == 0 {
		c.VerifiedPerRange = d.VerifiedPerRange
	}
}

// poolInfo is what the host knows about one followed pool.
type poolInfo struct {
	key     engine.PoolKey
	s       *series.Series
	section wire.Section
}

// Host runs the p2p side of one node.
type Host struct {
	n   *node.Node
	cfg Config
	id  hash.Hash

	coverage []wire.Section
	byPoolID map[hash.Hash]*poolInfo
	byKey    map[engine.PoolKey]*poolInfo
	issuers  map[hash.Hash]hash.Hash // series id → issuer (zero if none)

	mu      sync.Mutex
	peers   map[string]*peer
	nodeIDs map[hash.Hash]string
	seq     int
	chasing map[engine.PoolKey]bool
	again   map[engine.PoolKey]*peer
	reqs    *idCache
	ln      net.Listener
	ctx     context.Context
	wg      sync.WaitGroup

	book      *peers.Book
	dialing   map[string]bool                 // addresses being dialled
	paths     map[hash.Hash]returnPath        // relayed requests: where replies go
	bcast     map[hash.Hash]chan wire.Message // this node's broadcasts awaiting replies
	sent      map[hash.Hash]sentReply         // peer list replies kept for missing chunks
	lastBcast map[wire.Section]time.Time
	lastReg   time.Time
	selfAddrs map[string]bool // addresses found to lead back to this node
}

// New prepares a host for n. Run starts it.
func New(n *node.Node, cfg Config) (*Host, error) {
	cfg.fill()
	h := &Host{
		n: n, cfg: cfg,
		byPoolID: map[hash.Hash]*poolInfo{},
		byKey:    map[engine.PoolKey]*poolInfo{},
		issuers:  map[hash.Hash]hash.Hash{},
		peers:    map[string]*peer{},
		nodeIDs:  map[hash.Hash]string{},
		chasing:  map[engine.PoolKey]bool{},
		again:    map[engine.PoolKey]*peer{},
		reqs:     newIDCache(cfg.RequestWindow),
		book:     cfg.Book,
		dialing:  map[string]bool{},
		paths:    map[hash.Hash]returnPath{},
		bcast:    map[hash.Hash]chan wire.Message{},
		sent:     map[hash.Hash]sentReply{},

		lastBcast: map[wire.Section]time.Time{},
		selfAddrs: map[string]bool{},
	}
	if h.book == nil {
		h.book, _ = peers.Open("", peers.Limits{})
	}
	if _, err := rand.Read(h.id[:]); err != nil {
		return nil, err
	}
	eng := n.Engine()
	perSeries := map[hash.Hash]int{}
	for _, k := range eng.FollowedPools() {
		s, _ := eng.Series(k.Series)
		var iss hash.Hash
		if p := s.Params().Issuer; p != nil {
			iss = *p
		}
		h.issuers[s.ID()] = iss
		pi := &poolInfo{key: k, s: s, section: wire.PoolSection(iss, s.ID(), s.PoolID(k.Index))}
		h.byPoolID[s.PoolID(k.Index)] = pi
		h.byKey[k] = pi
		perSeries[k.Series]++
	}
	// Coverage: a whole series where every pool is followed, else pools.
	ids := make([]hash.Hash, 0, len(perSeries))
	for id := range perSeries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].Less(ids[j]) })
	for _, id := range ids {
		s, _ := eng.Series(id)
		if uint64(perSeries[id]) == s.PoolCount() {
			h.coverage = append(h.coverage, wire.SeriesSection(h.issuers[id], id))
			continue
		}
		for _, pi := range h.sortedPools() {
			if pi.key.Series == id {
				h.coverage = append(h.coverage, pi.section)
			}
		}
	}
	if len(h.coverage) > 4096 {
		return nil, fmt.Errorf("p2p: following %d separate pools; follow whole series instead", len(h.coverage))
	}
	for _, a := range cfg.Peers {
		h.book.Add(a, h.coverage, peers.FromConfig, time.Now())
	}
	n.Observe(h.onKept)
	return h, nil
}

func (h *Host) sortedPools() []*poolInfo {
	out := make([]*poolInfo, 0, len(h.byKey))
	for _, pi := range h.byKey {
		out = append(out, pi)
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].key.Series.Compare(out[j].key.Series); c != 0 {
			return c < 0
		}
		return out[i].key.Index < out[j].key.Index
	})
	return out
}

func (h *Host) logf(format string, args ...any) {
	fmt.Fprintf(h.cfg.Log, "p2p: "+format+"\n", args...)
}

// ID returns this run's random node id.
func (h *Host) ID() hash.Hash { return h.id }

// Addr returns the address the host listens on, once Run has started, or
// "".
func (h *Host) Addr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ln == nil {
		return ""
	}
	return h.ln.Addr().String()
}

// Run listens, keeps the configured peers connected and checks pools
// until ctx is done. ready, if set, is called once listening has started.
func (h *Host) Run(ctx context.Context, ready func()) error {
	h.mu.Lock()
	h.ctx = ctx
	h.mu.Unlock()
	if h.cfg.Listen != "" {
		ln, err := net.Listen("tcp", h.cfg.Listen)
		if err != nil {
			return err
		}
		h.mu.Lock()
		h.ln = ln
		h.mu.Unlock()
		h.logf("listening on %s", ln.Addr())
		h.wg.Add(1)
		go h.acceptLoop(ln)
	}
	for _, addr := range h.cfg.Peers {
		h.wg.Add(1)
		go h.dialLoop(ctx, addr)
	}
	if h.cfg.CheckEvery > 0 {
		h.wg.Add(1)
		go h.checkLoop(ctx)
	}
	if h.cfg.ManageEvery > 0 {
		h.wg.Add(1)
		go h.manageLoop(ctx)
	}
	if h.cfg.PingEvery > 0 {
		h.wg.Add(1)
		go h.pingLoop(ctx)
	}
	if ready != nil {
		ready()
	}
	<-ctx.Done()
	h.mu.Lock()
	if h.ln != nil {
		h.ln.Close()
	}
	for _, p := range h.peers {
		p.close(errors.New("shutting down"))
	}
	h.mu.Unlock()
	h.wg.Wait()
	return h.book.Save()
}

func (h *Host) acceptLoop(ln net.Listener) {
	defer h.wg.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		h.mu.Lock()
		full := len(h.peers) >= h.cfg.MaxPeers
		h.mu.Unlock()
		if full {
			c.Close()
			continue
		}
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.runConn(c, false, c.RemoteAddr().String())
		}()
	}
}

// dialLoop keeps a listed peer connected, whatever the connection
// manager does.
func (h *Host) dialLoop(ctx context.Context, addr string) {
	defer h.wg.Done()
	h.mu.Lock()
	h.dialing[addr] = true
	h.mu.Unlock()
	wait := h.cfg.Redial
	for ctx.Err() == nil {
		start := time.Now()
		err := h.dial(ctx, addr)
		if err != nil {
			h.logf("peer %s: %v", addr, err)
		}
		if time.Since(start) > h.cfg.MaxRedial {
			wait = h.cfg.Redial // it was up for a good while; retry soon
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > h.cfg.MaxRedial {
			wait = h.cfg.MaxRedial
		}
	}
}

// dial connects to addr and runs the connection until it closes.
func (h *Host) dial(ctx context.Context, addr string) error {
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return h.runConn(c, true, addr)
}

// Connect dials addr once and runs the connection in the background. It
// returns once the peer has answered the follow requests (or failed to).
func (h *Host) Connect(addr string) error {
	h.mu.Lock()
	ctx := h.ctx
	h.mu.Unlock()
	if ctx == nil {
		return errors.New("p2p: host not running")
	}
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	ready := make(chan error, 1)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.runConnNotify(c, true, addr, ready)
	}()
	return <-ready
}

func (h *Host) checkLoop(ctx context.Context) {
	defer h.wg.Done()
	t := time.NewTicker(h.cfg.CheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, pi := range h.sortedPools() {
				if p := h.anyPeerFor(pi.section); p != nil {
					h.startChase(pi, p)
				}
			}
		}
	}
}

// PeerInfo describes a connected peer.
type PeerInfo struct {
	ID       string    `json:"id"`
	Addr     string    `json:"addr"`
	Outbound bool      `json:"outbound"`
	Agent    string    `json:"agent"`
	Listen   string    `json:"listen,omitempty"`
	Coverage []string  `json:"coverage"`
	Since    time.Time `json:"since"`
	Sent     uint64    `json:"sent"`
	Received uint64    `json:"received"`
}

// Peers lists the connected peers.
func (h *Host) Peers() []PeerInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]PeerInfo, 0, len(h.peers))
	for _, p := range h.peers {
		out = append(out, p.info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// peersFor returns the connected peers whose coverage includes s, except
// the one named skip.
func (h *Host) peersFor(s wire.Section, skip string) []*peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*peer
	for id, p := range h.peers {
		if id != skip && p.covers(s) {
			out = append(out, p)
		}
	}
	return out
}

// anyPeerFor picks a peer to ask about s, preferring one this node dialled
// itself: those are harder for an attacker to place.
func (h *Host) anyPeerFor(s wire.Section) *peer {
	ps := h.peersFor(s, "")
	if len(ps) == 0 {
		return nil
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].outbound != ps[j].outbound {
			return ps[i].outbound
		}
		return ps[i].id < ps[j].id
	})
	return ps[0]
}

// Book returns the peer book.
func (h *Host) Book() *peers.Book { return h.book }

// idCache remembers request ids for a window.
type idCache struct {
	mu     sync.Mutex
	window time.Duration
	seen   map[hash.Hash]time.Time
	last   time.Time
}

func newIDCache(window time.Duration) *idCache {
	return &idCache{window: window, seen: map[hash.Hash]time.Time{}}
}

// add records id and reports whether it was new.
func (c *idCache) add(id hash.Hash, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Sub(c.last) > c.window/4 {
		for k, t := range c.seen {
			if now.Sub(t) > c.window {
				delete(c.seen, k)
			}
		}
		c.last = now
	}
	if _, ok := c.seen[id]; ok {
		return false
	}
	c.seen[id] = now
	return true
}
