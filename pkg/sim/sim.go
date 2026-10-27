// Package sim is a deterministic simulator of many full nodes. Each node
// is a real engine; the network between them is simulated: links with
// latency, partitions, flooding of what each engine says to forward, and
// fetching of missing entries. Time is virtual and every random choice
// comes from one seed, so a run is repeatable to the last hash.
//
// The simulator exists to run the design's "tests to run": agreement,
// partitions, latency and sibling work, attacks with a head start, and
// the sizing of a pool. It sits on the engine, not on pkg/p2p: it models
// what reaches each node and when, not the byte form, peer ranking or
// chasing, which pkg/p2p tests over real connections (decision D71).
//
// A Sim is not safe for concurrent use.
package sim

import (
	"container/heap"
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Time is virtual time in milliseconds since the start of a run.
type Time int64

// Second is one second of virtual time.
const Second Time = 1000

// Config sets up a simulation.
type Config struct {
	// Seed makes the run: the topology, latencies and every random choice.
	Seed int64
	// Nodes is the number of full nodes.
	Nodes int
	// Degree is how many random links each node starts (so the average
	// degree is about twice this). The nodes always form a ring as well,
	// so the network is connected.
	Degree int
	// MinLatency and MaxLatency bound each link's one-way delay, drawn
	// uniformly when the link is made.
	MinLatency, MaxLatency Time
	// Series is followed in full by every node.
	Series *series.Series
	// Engine is every node's engine config; zero means engine defaults.
	Engine *engine.Config
}

// Sim is one simulated network.
type Sim struct {
	cfg   Config
	s     *series.Series
	now   Time
	seq   uint64
	q     events
	rnd   *rand.Rand
	nodes []*Node
	links map[[2]int]*link
	stats Stats
	// layers records every layer made, for the work accounting.
	layers []madeLayer
}

// Stats counts what the network carried.
type Stats struct {
	Messages  int
	Delivered int
	Dropped   int // sent over a cut link
	Fetches   int // entries asked for because a parent or entry was missing
}

type madeLayer struct {
	node   int
	pool   uint64
	entry  hash.Hash
	credit weight.Weight
	at     Time
}

type link struct {
	a, b    int
	latency Time
	cut     bool
}

func key(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

// New builds the network: every node follows the whole series, and links
// form a ring plus Degree random links per node.
func New(cfg Config) (*Sim, error) {
	if cfg.Nodes < 1 || cfg.Series == nil {
		return nil, errors.New("sim: need at least one node and a series")
	}
	if cfg.MaxLatency < cfg.MinLatency {
		return nil, errors.New("sim: MaxLatency below MinLatency")
	}
	ec := engine.DefaultConfig()
	if cfg.Engine != nil {
		ec = *cfg.Engine
	}
	s := &Sim{cfg: cfg, s: cfg.Series, rnd: rand.New(rand.NewSource(cfg.Seed)), links: map[[2]int]*link{}}
	for i := 0; i < cfg.Nodes; i++ {
		c := ec
		c.Ledger.Clock = s.clock
		e := engine.New(c)
		e.AddSeries(cfg.Series)
		if err := e.FollowSeries(cfg.Series.ID()); err != nil {
			return nil, err
		}
		s.nodes = append(s.nodes, &Node{
			ID: i, Eng: e, sim: s,
			orphans:      map[hash.Hash][]orphan{},
			orphanLayers: map[hash.Hash][]layerFrom{},
			asked:        map[hash.Hash]Time{},
			stating:      map[uint64]bool{},
			nonce:        uint64(s.rnd.Int63()), // a random start, so the lottery differs by seed
		})
	}
	if cfg.Nodes > 1 {
		for i := 0; i < cfg.Nodes; i++ {
			s.Link(i, (i+1)%cfg.Nodes)
			for d := 0; d < cfg.Degree; d++ {
				if j := s.rnd.Intn(cfg.Nodes); j != i {
					s.Link(i, j)
				}
			}
		}
	}
	return s, nil
}

// clock gives the ledgers the virtual time as their first-seen clock.
func (s *Sim) clock() timeT { return epoch.Add(durationOf(s.now)) }

// Link connects two nodes, if they are not already linked, with a latency
// drawn from the configured range.
func (s *Sim) Link(a, b int) {
	if a == b {
		return
	}
	k := key(a, b)
	if _, ok := s.links[k]; ok {
		return
	}
	lat := s.cfg.MinLatency
	if d := s.cfg.MaxLatency - s.cfg.MinLatency; d > 0 {
		lat += Time(s.rnd.Int63n(int64(d) + 1))
	}
	s.links[k] = &link{a: k[0], b: k[1], latency: lat}
	s.nodes[a].nbrs = append(s.nodes[a].nbrs, b)
	s.nodes[b].nbrs = append(s.nodes[b].nbrs, a)
}

// Series returns the simulated series.
func (s *Sim) Series() *series.Series { return s.s }

// Now returns the virtual time.
func (s *Sim) Now() Time { return s.now }

// Rand returns the run's random source, for workloads that must stay
// deterministic.
func (s *Sim) Rand() *rand.Rand { return s.rnd }

// Nodes returns the nodes.
func (s *Sim) Nodes() []*Node { return s.nodes }

// Node returns node i.
func (s *Sim) Node(i int) *Node { return s.nodes[i] }

// Stats returns the network counters so far.
func (s *Sim) Stats() Stats { return s.stats }

// Pool returns the key of pool k of the series.
func (s *Sim) Pool(k uint64) engine.PoolKey { return engine.PoolKey{Series: s.s.ID(), Index: k} }

// At runs fn at time t (or now, if t is past).
func (s *Sim) At(t Time, fn func()) {
	if t < s.now {
		t = s.now
	}
	s.seq++
	heap.Push(&s.q, &event{at: t, seq: s.seq, fn: fn})
}

// After runs fn d from now.
func (s *Sim) After(d Time, fn func()) { s.At(s.now+d, fn) }

// Every runs fn every period from now until it returns false, or until
// the run stops.
func (s *Sim) Every(period Time, fn func() bool) {
	var tick func()
	tick = func() {
		if fn() {
			s.After(period, tick)
		}
	}
	s.After(period, tick)
}

// RunUntil handles events in time order until time t; events after t
// stay queued.
func (s *Sim) RunUntil(t Time) {
	for s.q.Len() > 0 && s.q[0].at <= t {
		ev := heap.Pop(&s.q).(*event)
		s.now = ev.at
		ev.fn()
	}
	if s.now < t {
		s.now = t
	}
}

// Run handles events for d of virtual time.
func (s *Sim) Run(d Time) { s.RunUntil(s.now + d) }

// Settle runs until no message is in flight and nothing is scheduled
// before limit, or until limit. Recurring work keeps the queue busy, so
// stop it first (see Builder.Stop).
func (s *Sim) Settle(limit Time) {
	end := s.now + limit
	for s.q.Len() > 0 && s.q[0].at <= end {
		ev := heap.Pop(&s.q).(*event)
		s.now = ev.at
		ev.fn()
	}
}

// Partition cuts every link between nodes of different groups. Nodes not
// named stay linked to everyone they were linked to.
func (s *Sim) Partition(groups ...[]int) {
	side := map[int]int{}
	for g, ns := range groups {
		for _, n := range ns {
			side[n] = g + 1
		}
	}
	for _, l := range s.links {
		ga, gb := side[l.a], side[l.b]
		if ga != 0 && gb != 0 && ga != gb {
			l.cut = true
		}
	}
}

// Heal restores every cut link, and the two ends of each exchange every
// entry and layer they hold, as two nodes reconnecting would catch up.
func (s *Sim) Heal() {
	var healed []*link
	for _, l := range s.links {
		if l.cut {
			l.cut = false
			healed = append(healed, l)
		}
	}
	sort.Slice(healed, func(i, j int) bool {
		if healed[i].a != healed[j].a {
			return healed[i].a < healed[j].a
		}
		return healed[i].b < healed[j].b
	})
	for _, l := range healed {
		s.nodes[l.a].syncTo(l.b)
		s.nodes[l.b].syncTo(l.a)
	}
}

// send delivers m from a to b after the link's latency; a message over a
// cut link is lost.
func (s *Sim) send(from, to int, m msg) {
	s.stats.Messages++
	l := s.links[key(from, to)]
	if l == nil || l.cut {
		s.stats.Dropped++
		return
	}
	s.At(s.now+l.latency, func() {
		if l.cut {
			s.stats.Dropped++
			return
		}
		s.stats.Delivered++
		s.nodes[to].receive(from, m)
	})
}

// Tip returns node i's selected tip and weight in pool k.
func (s *Sim) Tip(i int, k uint64) (hash.Hash, weight.Weight) {
	st, err := s.nodes[i].Eng.Status(s.Pool(k))
	if err != nil {
		return hash.Hash{}, weight.Zero
	}
	return st.Tip, st.Weight
}

// Agree reports whether every node has the same selected tip in pool k,
// with the same weight (so the same layers counted).
func (s *Sim) Agree(k uint64) bool {
	t0, w0 := s.Tip(0, k)
	for i := range s.nodes {
		if t, w := s.Tip(i, k); t != t0 || w.Cmp(w0) != 0 {
			return false
		}
	}
	return true
}

// Tips counts the nodes on each selected tip of pool k.
func (s *Sim) Tips(k uint64) map[hash.Hash]int {
	out := map[hash.Hash]int{}
	for i := range s.nodes {
		t, _ := s.Tip(i, k)
		out[t]++
	}
	return out
}

// CheckConservation checks, at node i, that the unspent portions of every
// claimed mint add up to exactly the supply (the design's conservation
// test).
func (s *Sim) CheckConservation(i int) error {
	for serial := uint64(0); serial < s.s.Mints(); serial++ {
		hs, err := s.nodes[i].Eng.Holdings(s.s.ID(), serial)
		if err != nil {
			return err
		}
		if len(hs) == 0 {
			continue
		}
		var sum uint64
		for _, h := range hs {
			sum += h.Portion.Amount
		}
		if sum != s.s.Supply() {
			return fmt.Errorf("sim: node %d mint %d holds %d units, supply %d", i, serial, sum, s.s.Supply())
		}
	}
	return nil
}

// Work reports how the layer credit made so far splits, as node i sees
// pool k: on entries of its selected ledger, or elsewhere (on entries
// that lost, or that i never held). Sibling work is the second share.
func (s *Sim) Work(i int, k uint64) (selected, other weight.Weight) {
	in := map[hash.Hash]bool{}
	s.nodes[i].Eng.Ledger(s.Pool(k), func(p *ledger.Pool) {
		for _, l := range s.layers {
			if l.pool == k {
				if _, ok := in[l.entry]; !ok {
					in[l.entry] = p.InSelected(l.entry)
				}
			}
		}
	})
	for _, l := range s.layers {
		if l.pool != k {
			continue
		}
		if in[l.entry] {
			selected = selected.Add(l.credit)
		} else {
			other = other.Add(l.credit)
		}
	}
	return selected, other
}

// Submit hands a transaction to node i as a wallet would (it is pushed to
// the node, which forwards it if it keeps it).
func (s *Sim) Submit(i int, t *tx.Tx) error {
	res, err := s.nodes[i].Eng.HandleTx(t, engine.Pushed)
	if err != nil {
		return err
	}
	s.nodes[i].forward(-1, msg{kind: msgTx, tx: t}, res.Forward)
	return nil
}

// msg is one simulated message.
type msg struct {
	kind   msgKind
	pool   uint64
	tx     *tx.Tx
	entry  *entry.Entry
	layer  layer.Layer
	want   hash.Hash
	layers []layer.Layer // with a reply: the layers held on the entry
	hashes []hash.Hash   // entries whose layers are asked for
	weight weight.Weight // a stated weight, of the ledger ending at want
}

type msgKind int

const (
	msgTx msgKind = iota
	msgEntry
	msgLayer
	msgGet       // asks for the entry want
	msgReply     // an entry asked for, with its layers
	msgGetLayers // asks for the layers held on some entries
	msgLayers    // layers asked for
	msgWeight    // a stated weight
)

type event struct {
	at  Time
	seq uint64
	fn  func()
}

type events []*event

func (q events) Len() int { return len(q) }
func (q events) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].seq < q[j].seq
}
func (q events) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *events) Push(x any)   { *q = append(*q, x.(*event)) }
func (q *events) Pop() any {
	old := *q
	x := old[len(old)-1]
	*q = old[:len(old)-1]
	return x
}
