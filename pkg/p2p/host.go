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
// And it manages them (the design's "Peer Categories and Ranking", "Peer
// Tracking and Telemetry" and "Chasing Heavier Ledgers"): each verified
// peer is a priority or backup peer for a mint policy, or waits as a
// candidate, and moves between them at reviews by points from package
// tracking; requests and data are handled in queues by the sender's
// category; offences become flags that can time out or block an address;
// and a stated weight above the node's own goes to the chase queue of
// package chase, ordered by evidence, and is chased from cheap to costly:
// hashes, then provisional layers, then entries.
package p2p

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/chase"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
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
	// CheckEvery is how often to ask the priority peers of each pool for
	// their stated weight (negative: never), so a node hears it is behind
	// in a quiet pool.
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
	// MaxOutbound caps connections this node dials itself.
	MaxOutbound int
	// AllowLocalPeers lets addresses learned from peers and registries be
	// loopback, private, link-local or otherwise not public, and lets the
	// connection manager dial them. Off by default, so another party cannot
	// make the node connect into its own network (decision D96). Addresses
	// in Peers are always dialled. Local test networks turn it on.
	AllowLocalPeers bool
	// MaxInbound caps connections other nodes open to this one. By
	// default it is MaxPeers less MaxOutbound, so that inbound connections
	// cannot take the slots the node keeps for peers it chose itself
	// (decision D75). Negative: no inbound cap besides MaxPeers.
	MaxInbound int
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
	// PingEvery is how often to ping each priority peer for each mint
	// policy (negative: never), and BackupPingEvery each backup peer;
	// MaxMisses unanswered pings in a row end the peering for that policy.
	PingEvery       time.Duration
	BackupPingEvery time.Duration
	MaxMisses       int

	// Slots are the priority and backup slots per mint policy and the
	// ranking rules; the connection manager dials candidates while they
	// are not full.
	Slots peers.Slots
	// ReviewEvery is how often points are compared and peers moved
	// between priority and backup (negative: never).
	ReviewEvery time.Duration
	// InactiveAfter archives a priority or backup peer not heard from for
	// that long.
	InactiveAfter time.Duration
	// DenialLimit denials in the activity window bar a peer from priority.
	DenialLimit int
	// StrictReturn returns a timed out peer as a backup, so it must earn
	// priority again; by default it returns to the categories it had.
	StrictReturn bool
	// UnlistedConns caps inbound connections in no category (wallets,
	// light nodes, unverified full nodes); negative: none.
	UnlistedConns int
	// QueueShares are how many messages each queue (priority, backup,
	// candidate, unlisted) may take in one round, so lower queues are not
	// starved; QueueLimit is how many may wait in each.
	QueueShares [4]int
	QueueLimit  int

	// Tracker records what peers do; nil keeps records in memory with the
	// default template.
	Tracker *tracking.Tracker
	// Chase holds the chase queue's limits.
	Chase chase.Config
	// ChaseSlice is how long one chase runs before it yields its turn in
	// the chase queue. A yielded chase keeps everything it fetched and is
	// taken up again later; it is never refused for its length, since a
	// heavier ledger must always be reachable (audit items G1 to G3,
	// docs/consensus-audit.md).
	ChaseSlice time.Duration
	// WeightlessPages is how many pages of hashes in a row that bring no
	// work a chase checks before it yields its turn; it goes on later from
	// where it stopped, so a listing is paced, never refused (RED-015,
	// D120).
	WeightlessPages int
	// RequestPace is how many requests a chase sends one peer in
	// RequestPaceWindow before it yields its turn and goes on later from
	// where it stopped. It is under the request budget peers keep by
	// default (tracking budget "request", 600 a minute): over that budget
	// a peer drops requests unanswered, the chase times out and loses what
	// it took in, and a long enough branch is never reached (D115,
	// ORANGE-018). Negative: no pacing.
	RequestPace int
	// RequestPaceWindow is the window RequestPace counts over: the
	// default request budget's window (one minute).
	RequestPaceWindow time.Duration
	// PartsDir is where large entries are put together from their parts
	// ("": the system's temporary directory; RED-016).
	PartsDir string
	// PartsReserve is the free space a large entry's parts never take
	// from their disk; the reserve is the larger of it and 2% of the
	// disk. At the reserve a fetch waits and goes on later (RED-022,
	// D123). 0: 1 GiB.
	PartsReserve uint64
	// ProgressGrace is how long a yielded chase's progress, its parts
	// file included, is kept after its sender disconnects, so a sender
	// that comes back goes on from it; ProgressTTL is how long any
	// progress is kept with no chase taking it up (RED-023, D128).
	// 0: 2 minutes and 1 hour.
	ProgressGrace, ProgressTTL time.Duration
	// LayersWhole is the most layers of one entry a layers reply carries;
	// an entry with more has them paged (D121). It is at most, and by
	// default, what one frame can hold.
	LayersWhole int
	// PartsOver is the largest entry item a reply carries whole; a larger
	// one is sent in parts on request (consensus audit P1, D117). It is at
	// most, and by default, what one frame can hold.
	PartsOver int
	// NoConfirm stops asking a priority peer to confirm a stated weight
	// from another sender.
	NoConfirm bool

	// Log receives connection events. Nil discards them.
	Log io.Writer
	// Logger, if set, is used instead of a plain logger on Log; the
	// daemon passes the node's (node.Logger), which follows the node
	// policy's log section.
	Logger *slog.Logger
}

// DefaultConfig returns the provisional defaults.
func DefaultConfig() Config {
	return Config{
		MaxPeers:       32,
		Limits:         wire.Limits{ChunkSize: 1 << 20, EntryLimit: 512, HashLimit: 4096, LayerLimit: 1 << 16},
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

		MaxOutbound:     8,
		ManageEvery:     30 * time.Second,
		BroadcastEvery:  5 * time.Minute,
		RegistryEvery:   time.Hour,
		FollowTimeout:   5 * time.Second,
		HopLimit:        8,
		RequestRate:     120,
		PingEvery:       2 * time.Minute,
		BackupPingEvery: 6 * time.Minute,
		MaxMisses:       3,

		Slots:             peers.DefaultSlots(),
		ReviewEvery:       10 * time.Minute,
		InactiveAfter:     30 * time.Minute,
		DenialLimit:       3,
		UnlistedConns:     8,
		QueueShares:       [4]int{8, 4, 2, 1},
		QueueLimit:        1024,
		ChaseSlice:        2 * time.Minute,
		WeightlessPages:   64,
		RequestPace:       480,
		RequestPaceWindow: time.Minute,
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
	if c.MaxOutbound == 0 {
		c.MaxOutbound = d.MaxOutbound
	}
	if c.MaxInbound == 0 {
		c.MaxInbound = c.MaxPeers - c.MaxOutbound
	}
	if c.MaxInbound <= 0 || c.MaxInbound > c.MaxPeers {
		c.MaxInbound = c.MaxPeers
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
	if c.BackupPingEvery == 0 {
		c.BackupPingEvery = 3 * c.PingEvery
	}
	if c.MaxMisses == 0 {
		c.MaxMisses = d.MaxMisses
	}
	if c.Slots == (peers.Slots{}) {
		c.Slots = d.Slots
	}
	if c.ReviewEvery == 0 {
		c.ReviewEvery = d.ReviewEvery
	}
	if c.InactiveAfter == 0 {
		c.InactiveAfter = d.InactiveAfter
	}
	if c.DenialLimit == 0 {
		c.DenialLimit = d.DenialLimit
	}
	if c.UnlistedConns == 0 {
		c.UnlistedConns = d.UnlistedConns
	}
	if c.QueueShares == ([4]int{}) {
		c.QueueShares = d.QueueShares
	}
	if c.QueueLimit == 0 {
		c.QueueLimit = d.QueueLimit
	}
	if c.ChaseSlice == 0 {
		c.ChaseSlice = d.ChaseSlice
	}
	if c.RequestPace == 0 {
		c.RequestPace = d.RequestPace
	}
	if c.RequestPaceWindow < paceBuckets {
		c.RequestPaceWindow = d.RequestPaceWindow
	}
	if c.WeightlessPages <= 0 {
		c.WeightlessPages = d.WeightlessPages
	}
	if c.LayersWhole <= 0 || c.LayersWhole > maxNonces {
		c.LayersWhole = maxNonces
	}
	if c.PartsOver <= 0 || c.PartsOver > maxWhole {
		c.PartsOver = maxWhole
	}
	if c.PartsReserve == 0 {
		c.PartsReserve = 1 << 30
	}
	if c.ProgressGrace <= 0 {
		c.ProgressGrace = 2 * time.Minute
	}
	if c.ProgressTTL <= 0 {
		c.ProgressTTL = time.Hour
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
	// prog holds what yielded chases learned (see progress).
	progMu    sync.Mutex
	prog      map[progressKey]*progress
	progOrder []progressKey
	// encoded keeps the last large items encoded for parts requests
	// (RED-017).
	encoded partsCache

	n   *node.Node
	cfg Config
	id  hash.Hash
	log *slog.Logger
	// recvLines caps the debug line for each received message.
	recvLines lineCap

	coverage []wire.Section
	byPoolID map[hash.Hash]*poolInfo
	byKey    map[engine.PoolKey]*poolInfo
	byName   map[string]*poolInfo    // pool section text → pool
	issuers  map[hash.Hash]hash.Hash // series id → issuer (zero if none)

	tr    *tracking.Tracker
	cq    *chase.Queue
	inbox *inbox
	wake  chan struct{} // the chase loop has work

	mu      sync.Mutex
	peers   map[string]*peer
	nodeIDs map[hash.Hash]string
	reqs    *idCache
	ln      net.Listener
	ctx     context.Context
	wg      sync.WaitGroup
	// wgMu guards stopping, set once Run waits for its goroutines, so no
	// goroutine is added while it waits (see spawn).
	wgMu     sync.Mutex
	stopping bool

	book      *peers.Book
	dialing   map[string]bool                 // addresses being dialled
	paths     map[hash.Hash]returnPath        // relayed requests: where replies go
	bcast     map[hash.Hash]chan wire.Message // this node's broadcasts awaiting replies
	sent      map[hash.Hash]sentReply         // peer list replies kept for missing chunks
	lastBcast map[wire.Section]time.Time
	lastReg   time.Time
	selfAddrs map[string]bool // addresses found to lead back to this node
	// configured are the addresses in cfg.Peers, dialled whatever their
	// range; read-only after New.
	configured map[string]bool
}

// New prepares a host for n. Run starts it.
func New(n *node.Node, cfg Config) (*Host, error) {
	cfg.fill()
	h := &Host{
		n: n, cfg: cfg,
		byPoolID: map[hash.Hash]*poolInfo{},
		byKey:    map[engine.PoolKey]*poolInfo{},
		byName:   map[string]*poolInfo{},
		issuers:  map[hash.Hash]hash.Hash{},
		tr:       cfg.Tracker,
		cq:       chase.New(cfg.Chase),
		wake:     make(chan struct{}, 1),
		peers:    map[string]*peer{},
		nodeIDs:  map[hash.Hash]string{},
		reqs:     newIDCache(cfg.RequestWindow),
		book:     cfg.Book,
		dialing:  map[string]bool{},
		paths:    map[hash.Hash]returnPath{},
		bcast:    map[hash.Hash]chan wire.Message{},
		sent:     map[hash.Hash]sentReply{},

		lastBcast: map[wire.Section]time.Time{},
		selfAddrs: map[string]bool{},

		configured: map[string]bool{},
	}
	h.log = cfg.Logger
	if h.log == nil {
		h.log, _ = logging.New(cfg.Log, "", "")
	}
	h.log = h.log.With(logging.Component, "p2p")
	if h.book == nil {
		h.book, _ = peers.Open("", peers.Limits{})
	}
	if h.tr == nil {
		h.tr, _ = tracking.New(tracking.Config{}, "")
	}
	h.inbox = newInbox(cfg.QueueShares, cfg.QueueLimit)
	// Blocked addresses never come back as candidates.
	h.book.SetRefuse(func(addr string) bool { return h.tr.Status(addr, time.Now()).Blocked })
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
		h.byName[pi.section.Text()] = pi
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
		h.configured[a] = true
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
		h.log.Info("listening", "addr", ln.Addr().String())
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
	if h.cfg.ReviewEvery > 0 {
		h.wg.Add(1)
		go h.reviewLoop(ctx)
	}
	h.wg.Add(2)
	go h.workLoop(ctx)
	go h.chaseLoop(ctx)
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
	h.inbox.close()
	h.wgMu.Lock()
	h.stopping = true
	h.wgMu.Unlock()
	h.wg.Wait()
	// Parts of large entries fetched so far are not kept past a run.
	h.progMu.Lock()
	for _, st := range h.prog {
		st.drop()
	}
	h.prog, h.progOrder = nil, nil
	h.progMu.Unlock()
	err := h.book.Save()
	if terr := h.tr.Save(); err == nil {
		err = terr
	}
	return err
}

// spawn runs f in a goroutine Run waits for, unless Run is already
// waiting: a goroutine started from outside one Run tracks (Connect, from
// the RPC) must not be added once the count may have reached zero. It
// reports whether f was started.
func (h *Host) spawn(f func()) bool {
	h.wgMu.Lock()
	defer h.wgMu.Unlock()
	if h.stopping {
		return false
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		f()
	}()
	return true
}

func (h *Host) acceptLoop(ln net.Listener) {
	defer h.wg.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		h.mu.Lock()
		full := h.fullLocked(false)
		h.mu.Unlock()
		remote := c.RemoteAddr().String()
		if full || !h.tr.Status(remote, time.Now()).Allowed() {
			h.tr.Connection(remote, "refused", time.Now())
			c.Close()
			continue
		}
		if !h.spawn(func() { h.runConn(c, false, c.RemoteAddr().String()) }) {
			c.Close()
			return
		}
	}
}

// fullLocked reports whether a new connection, inbound or outbound, would
// pass a cap. h.mu is held.
func (h *Host) fullLocked(outbound bool) bool {
	if len(h.peers) >= h.cfg.MaxPeers {
		return true
	}
	if outbound {
		return false
	}
	in := 0
	for _, p := range h.peers {
		if !p.outbound {
			in++
		}
	}
	return in >= h.cfg.MaxInbound
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
			h.log.Info("dial failed", "addr", addr, "err", err)
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
	if st := h.tr.Status(addr, time.Now()); !st.Allowed() {
		return fmt.Errorf("address is timed out or blocked")
	}
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
	if st := h.tr.Status(addr, time.Now()); !st.Allowed() {
		return errBlocked
	}
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	ready := make(chan error, 1)
	if !h.spawn(func() { h.runConnNotify(c, true, addr, ready) }) {
		c.Close()
		return errors.New("p2p: host shutting down")
	}
	return <-ready
}

// checkLoop asks the priority peers of each pool for their stated weight;
// a heavier one goes to the chase queue like any other.
func (h *Host) checkLoop(ctx context.Context) {
	defer h.wg.Done()
	t := time.NewTicker(h.cfg.CheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.sweepProgress(time.Now())
			for _, pi := range h.sortedPools() {
				for _, p := range h.peersIn(pi.section, peers.CatPriority) {
					h.spawn(func() {
						h.askWeight(pi, p)
					})
				}
			}
		}
	}
}

// PeerInfo describes a connected peer.
type PeerInfo struct {
	ID       string `json:"id"`
	Addr     string `json:"addr"`
	Key      string `json:"key"`
	Outbound bool   `json:"outbound"`
	Agent    string `json:"agent"`
	Listen   string `json:"listen,omitempty"`
	// Coverage lists what the peer was verified to follow.
	Coverage []string `json:"coverage"`
	// Categories maps each of this node's mint policies the peer was
	// verified for, in full text form, to its category and points.
	Categories map[string]Standing `json:"categories,omitempty"`
	Since      time.Time           `json:"since"`
	Sent       uint64              `json:"sent"`
	Received   uint64              `json:"received"`
}

// Standing is a peer's category and points for one mint policy.
type Standing struct {
	Category string  `json:"category"`
	Points   float64 `json:"points"`
	Partial  bool    `json:"partial,omitempty"`
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

// peersFor returns the connected priority and backup peers whose verified
// coverage includes s, except the one named skip: the peers forwarded to.
func (h *Host) peersFor(s wire.Section, skip string) []*peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*peer
	for id, p := range h.peers {
		if id != skip && p.covers(s, true) {
			out = append(out, p)
		}
	}
	return out
}

// verifiedFor returns the connected peers verified for any part of s, in
// any category.
func (h *Host) verifiedFor(s wire.Section) []*peer {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*peer
	for _, p := range h.peers {
		if p.covers(s, false) {
			out = append(out, p)
		}
	}
	return out
}

// Book returns the peer book.
func (h *Host) Book() *peers.Book { return h.book }

// Tracker returns the tracker.
func (h *Host) Tracker() *tracking.Tracker { return h.tr }

// ChaseQueue returns the chase queue.
func (h *Host) ChaseQueue() *chase.Queue { return h.cq }

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

// lineCap bounds how many lines of one kind a host logs per second, so
// a debug line written for every message a peer sends cannot let a flood
// of messages, or a slow log sink, hold up every peer's read loop. Lines
// over the cap are counted, and the count is logged when the next second
// begins.
type lineCap struct {
	mu      sync.Mutex
	second  int64
	n       int
	dropped int
}

// debugReceivedPerSecond caps the debug line for received messages.
const debugReceivedPerSecond = 200

// allow reports whether one more line may be logged now, and how many
// were dropped in the second before, if this is the first line of a new
// second.
func (c *lineCap) allow(now time.Time, max int) (ok bool, dropped int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := now.Unix(); s != c.second {
		c.second, c.n, dropped, c.dropped = s, 0, c.dropped, 0
	}
	if c.n >= max {
		c.dropped++
		return false, dropped
	}
	c.n++
	return true, dropped
}
