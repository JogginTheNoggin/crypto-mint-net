package sim

import (
	"errors"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

type timeT = time.Time

// epoch is virtual time zero, for the ledgers' first-seen clock.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func durationOf(t Time) time.Duration { return time.Duration(t) * time.Millisecond }

// Node is one simulated full node.
type Node struct {
	ID  int
	Eng *engine.Engine
	// Private nodes keep what they make to themselves until Release: an
	// attacker building a rival ledger in secret.
	Private bool

	sim  *Sim
	nbrs []int
	// orphans wait for their parent; orphanLayers for their entry.
	orphans      map[hash.Hash][]orphan
	orphanLayers map[hash.Hash][]layerFrom
	// asked records when each missing entry was last asked for; a lost
	// request is asked again after askAgain.
	asked    map[hash.Hash]Time
	nonce    uint64
	stating  map[uint64]bool
	withheld []msg
}

// askAgain is how long a node waits for an entry it asked for before it
// asks again.
const askAgain = 2 * Second

type orphan struct {
	from   int
	m      msg
	origin engine.Origin
}

type layerFrom struct {
	from int
	pool uint64
	l    layer.Layer
}

// Neighbours returns the nodes linked to n.
func (n *Node) Neighbours() []int { return append([]int(nil), n.nbrs...) }

// forward sends m to every neighbour but skip, when the engine said to.
// A private node holds it back instead.
func (n *Node) forward(skip int, m msg, ok bool) {
	if !ok {
		return
	}
	if n.Private {
		n.withheld = append(n.withheld, m)
		return
	}
	for _, b := range n.nbrs {
		if b != skip {
			n.sim.send(n.ID, b, m)
		}
	}
}

// Release makes a private node public and sends everything it held back.
func (n *Node) Release() {
	n.Private = false
	w := n.withheld
	n.withheld = nil
	for _, m := range w {
		n.forward(-1, m, true)
	}
}

// receive handles one message from a neighbour.
func (n *Node) receive(from int, m msg) {
	switch m.kind {
	case msgTx:
		res, err := n.Eng.HandleTx(m.tx, engine.Pushed)
		if err == nil {
			n.forward(from, m, res.Forward)
		}
	case msgEntry:
		n.takeEntry(from, m, engine.Pushed)
	case msgLayer:
		n.takeLayer(from, m.pool, m.layer, engine.Pushed)
	case msgGet:
		n.answer(from, m.pool, m.want)
	case msgReply:
		n.takeEntry(from, m, engine.Requested)
	case msgGetLayers:
		var ls []layer.Layer
		n.Eng.Ledger(n.sim.Pool(m.pool), func(p *ledger.Pool) {
			for _, h := range m.hashes {
				ls = append(ls, p.Layers(h)...)
			}
		})
		if len(ls) > 0 {
			n.sim.send(n.ID, from, msg{kind: msgLayers, pool: m.pool, layers: ls})
		}
	case msgWeight:
		n.heard(from, m)
	case msgLayers:
		for _, l := range m.layers {
			n.takeLayer(from, m.pool, l, engine.Requested)
		}
	}
}

// takeEntry hands an entry to the engine, then the layers that came with
// it (a reply). A missing parent is asked for from the sender and the
// entry waits for it, with its layers. It reports whether the entry is
// now held.
func (n *Node) takeEntry(from int, m msg, origin engine.Origin) bool {
	if n.addEntry(from, m, origin) {
		for _, l := range m.layers {
			n.takeLayer(from, m.pool, l, engine.Requested)
		}
		return true
	}
	return false
}

func (n *Node) addEntry(from int, m msg, origin engine.Origin) bool {
	k := n.sim.Pool(m.pool)
	res, err := n.Eng.HandleEntry(k, m.entry, origin)
	switch {
	case errors.Is(err, ledger.ErrUnknownParent):
		p := m.entry.Parent
		n.orphans[p] = append(n.orphans[p], orphan{from, m, origin})
		n.ask(from, m.pool, p)
		return false
	case errors.Is(err, engine.ErrSeen):
		return true
	case err != nil:
		return false
	}
	n.forward(from, msg{kind: msgEntry, pool: m.pool, entry: m.entry}, res.Forward)
	n.switched(from, m.pool, res.Switch != nil && !res.Forward)
	n.adopt(res.Hash)
	return true
}

// adopt handles what waited for entry h.
func (n *Node) adopt(h hash.Hash) {
	delete(n.asked, h)
	if os := n.orphans[h]; len(os) > 0 {
		delete(n.orphans, h)
		for _, o := range os {
			n.takeEntry(o.from, o.m, o.origin)
		}
	}
	if ls := n.orphanLayers[h]; len(ls) > 0 {
		delete(n.orphanLayers, h)
		for _, l := range ls {
			n.takeLayer(l.from, l.pool, l.l, engine.Pushed)
		}
	}
}

func (n *Node) takeLayer(from int, pool uint64, l layer.Layer, origin engine.Origin) {
	res, err := n.Eng.HandleLayer(n.sim.Pool(pool), l, origin)
	if errors.Is(err, ledger.ErrUnknownEntry) {
		n.orphanLayers[l.Entry] = append(n.orphanLayers[l.Entry], layerFrom{from, pool, l})
		n.ask(from, pool, l.Entry)
		return
	}
	if err != nil {
		return
	}
	n.forward(from, msg{kind: msgLayer, pool: pool, layer: l}, res.Forward)
	n.switched(from, pool, res.Switch != nil && !res.Forward)
	if !res.Forward && res.Switch == nil {
		n.stateWeight(pool)
	}
}

// stateWeight tells the neighbours, shortly, the weight of n's selected
// ledger after requested work raised it without a switch: work that is
// never forwarded, which the neighbours would otherwise not learn of
// until a regular weight check (decision D59). A neighbour lighter on the
// same tip asks for the layers; one on another tip asks for the entry.
func (n *Node) stateWeight(pool uint64) {
	if n.Private || n.stating[pool] {
		return
	}
	n.stating[pool] = true
	n.sim.After(200, func() {
		n.stating[pool] = false
		tip, w := n.sim.Tip(n.ID, pool)
		for _, b := range n.nbrs {
			n.sim.send(n.ID, b, msg{kind: msgWeight, pool: pool, want: tip, weight: w})
		}
	})
}

// heard handles a neighbour's stated weight.
func (n *Node) heard(from int, m msg) {
	tip, w := n.sim.Tip(n.ID, m.pool)
	if !m.weight.Greater(w) {
		return
	}
	if tip == m.want {
		var chain []hash.Hash
		n.Eng.Ledger(n.sim.Pool(m.pool), func(p *ledger.Pool) { chain = p.Chain(0, p.Height()) })
		n.sim.send(n.ID, from, msg{kind: msgGetLayers, pool: m.pool, hashes: chain})
		return
	}
	n.ask(from, m.pool, m.want)
}

// switched follows a switch made on requested data: the node asks the
// sender for the layers it holds on the new selected ledger (work on
// shared entries that never reached this node), and announces the tip.
func (n *Node) switched(from int, pool uint64, sw bool) {
	if !sw {
		return
	}
	if from >= 0 {
		var chain []hash.Hash
		n.Eng.Ledger(n.sim.Pool(pool), func(p *ledger.Pool) { chain = p.Chain(0, p.Height()) })
		n.sim.send(n.ID, from, msg{kind: msgGetLayers, pool: pool, hashes: chain})
	}
	n.announce(pool, true)
}

// announce tells the neighbours about a new selected tip reached through
// requested data, which is never forwarded: the design's notice after a
// switch (decision D36). Neighbours that lack it fetch what they miss.
func (n *Node) announce(pool uint64, switched bool) {
	if !switched || n.Private {
		return
	}
	var tip hash.Hash
	var e *entry.Entry
	n.Eng.Ledger(n.sim.Pool(pool), func(p *ledger.Pool) {
		tip, _, _ = p.Tip()
		e, _ = p.Entry(tip)
	})
	if e == nil {
		return
	}
	for _, b := range n.nbrs {
		n.sim.send(n.ID, b, msg{kind: msgReply, pool: pool, entry: e, layers: n.layersOn(pool, tip)})
	}
}

func (n *Node) ask(from int, pool uint64, h hash.Hash) {
	if t, ok := n.asked[h]; from < 0 || ok && n.sim.now-t < askAgain {
		return
	}
	n.asked[h] = n.sim.now
	n.sim.stats.Fetches++
	n.sim.send(n.ID, from, msg{kind: msgGet, pool: pool, want: h})
}

func (n *Node) layersOn(pool uint64, h hash.Hash) []layer.Layer {
	var ls []layer.Layer
	n.Eng.Ledger(n.sim.Pool(pool), func(p *ledger.Pool) { ls = p.Layers(h) })
	return ls
}

// answer replies to a request for entry h with the entry and its layers.
// A private node answers too: it is asked only for what it has shown.
func (n *Node) answer(to int, pool uint64, h hash.Hash) {
	var e *entry.Entry
	n.Eng.Ledger(n.sim.Pool(pool), func(p *ledger.Pool) { e, _ = p.Entry(h) })
	if e == nil {
		return
	}
	n.sim.send(n.ID, to, msg{kind: msgReply, pool: pool, entry: e, layers: n.layersOn(pool, h)})
}

// syncTo sends node b every entry and layer n holds, in an order that
// rebuilds n's tree: two nodes catching up after a partition.
func (n *Node) syncTo(b int) {
	if n.Private {
		return
	}
	for k := uint64(0); k < n.sim.s.PoolCount(); k++ {
		var es []*entry.Entry
		var ls [][]layer.Layer
		n.Eng.Ledger(n.sim.Pool(k), func(p *ledger.Pool) {
			for _, h := range p.ReplayOrder() {
				e, _ := p.Entry(h)
				es = append(es, e)
				ls = append(ls, p.Layers(h))
			}
		})
		for i, e := range es {
			n.sim.send(n.ID, b, msg{kind: msgReply, pool: k, entry: e, layers: ls[i]})
		}
	}
}

// Build makes an entry from n's pending transactions of pool k on its
// selected tip and sends it out. It returns the entry, or nil when there
// was nothing to build.
func (n *Node) Build(k uint64) *entry.Entry {
	e, res, err := n.Eng.Build(n.sim.Pool(k), 0)
	if err != nil {
		return nil
	}
	n.forward(-1, msg{kind: msgEntry, pool: k, entry: e}, res.Forward)
	return e
}

// BuildOn makes an entry of the given transactions on parent, outside the
// pending pool: a rival's entry. It is kept and, unless n is private, sent.
func (n *Node) BuildOn(k uint64, parent hash.Hash, txs []*tx.Tx) (*entry.Entry, error) {
	e, _, err := entry.Build(n.sim.s, parent, txs, 0)
	if err != nil {
		return nil, err
	}
	res, err := n.Eng.HandleEntry(n.sim.Pool(k), e, engine.Local)
	if err != nil {
		return nil, err
	}
	n.forward(-1, msg{kind: msgEntry, pool: k, entry: e}, res.Forward)
	return e, nil
}

// WorkOn tries tries nonces on entry h of pool k, adds the strongest
// layer and sends it out. It returns the layer's credit.
func (n *Node) WorkOn(k uint64, h hash.Hash, tries uint64) (weight.Weight, bool) {
	l, res, err := n.Eng.Work(n.sim.Pool(k), h, n.nonce, tries)
	n.nonce += tries
	if err != nil {
		return weight.Weight{}, false
	}
	n.sim.layers = append(n.sim.layers, madeLayer{node: n.ID, pool: k, entry: h, credit: res.Credit, at: n.sim.now})
	n.forward(-1, msg{kind: msgLayer, pool: k, layer: l}, res.Forward)
	return res.Credit, true
}

// Work adds work where an honest builder would: on an entry that extends
// n's selected tip and ties with it (a new entry with no credit yet, the
// first seen), else on the selected tip itself.
func (n *Node) Work(k uint64, tries uint64) (weight.Weight, bool) {
	target, ok := n.target(k)
	if !ok {
		return weight.Weight{}, false
	}
	return n.WorkOn(k, target, tries)
}

func (n *Node) target(k uint64) (hash.Hash, bool) {
	st, err := n.Eng.Status(n.sim.Pool(k))
	if err != nil {
		return hash.Hash{}, false
	}
	for _, b := range st.Branches {
		if b.Fork == st.Tip && b.Weight.Cmp(st.Weight) == 0 {
			return b.Tip, true
		}
	}
	if st.Tip == n.sim.s.PoolID(k) {
		return hash.Hash{}, false // no entry yet
	}
	return st.Tip, true
}
