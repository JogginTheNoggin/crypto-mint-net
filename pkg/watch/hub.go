package watch

import (
	"slices"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// DefaultBuffer is how many events a subscriber may fall behind by before
// events are dropped for it.
const DefaultBuffer = 1024

// Event is one line of the watch stream. Kind says which of the optional
// fields are set.
type Event struct {
	Seq    uint64      `json:"seq"`
	Time   time.Time   `json:"time"`
	Kind   filter.Kind `json:"kind"`
	Series hash.Hash   `json:"series"`
	Pool   uint64      `json:"pool"`
	// Weight is the pool's selected ledger weight after the item.
	Weight weight.Weight `json:"weight"`
	// Origin is how the item came: pushed, requested, local or stored.
	Origin string `json:"origin,omitempty"`
	// From names the peer that sent the item.
	From string `json:"from,omitempty"`
	// Tx: for tx, selected, reverted and dropped (when the node still has
	// the transaction).
	Tx  *Tx        `json:"tx,omitempty"`
	TID *hash.Hash `json:"tid,omitempty"`
	// Entry: for entry, and the entry that now holds a selected
	// transaction or held a reverted one.
	Entry *EventEntry `json:"entry,omitempty"`
	Layer *EventLayer `json:"layer,omitempty"`
	// Switch: for switch.
	Switch *EventSwitch `json:"switch,omitempty"`
	// Void: for reverted, whether the transaction can no longer apply
	// (else it is pending again).
	Void bool `json:"void,omitempty"`
	// Missed: for missed, the stream's own notice that this many events
	// were dropped because the reader fell behind.
	Missed uint64 `json:"missed,omitempty"`
	// Grind: for grind, what a grind job did.
	Grind *grind.Event `json:"grind,omitempty"`
}

// EventEntry describes an entry in an event.
type EventEntry struct {
	Hash     hash.Hash   `json:"hash"`
	Parent   hash.Hash   `json:"parent,omitempty"`
	Height   int         `json:"height,omitempty"`
	TIDs     []hash.Hash `json:"tids,omitempty"`
	Selected bool        `json:"selected"`
}

// EventLayer describes a layer in an event.
type EventLayer struct {
	Entry    hash.Hash     `json:"entry"`
	Hash     hash.Hash     `json:"hash"`
	Nonce    uint64        `json:"nonce"`
	Strength int           `json:"strength"`
	Credit   weight.Weight `json:"credit"`
}

// EventSwitch describes a change of selected ledger that replaced entries.
type EventSwitch struct {
	From       hash.Hash   `json:"from"`
	To         hash.Hash   `json:"to"`
	Fork       hash.Hash   `json:"fork"`
	ForkHeight int         `json:"fork_height"`
	Reverted   []hash.Hash `json:"reverted"`
	Applied    []hash.Hash `json:"applied"`
	Carried    []hash.Hash `json:"carried"`
	Void       []hash.Hash `json:"void"`
}

// Hub fans a node's kept items out to subscribers as events. It is safe
// for concurrent use.
type Hub struct {
	eng  *engine.Engine
	mu   sync.Mutex
	seq  uint64
	subs map[*Sub]bool
	// closed: Close ran, so a later subscriber is closed at once rather
	// than left waiting for events that will never come.
	closed bool
}

// Sub is one subscriber. Its events arrive on C until Close.
type Sub struct {
	C      <-chan Event
	c      chan Event
	f      filter.Filter
	hub    *Hub
	missed uint64
	closed bool
}

// NewHub returns a hub fed by n. Observing is cheap while nobody is
// subscribed.
func NewHub(n *node.Node) *Hub {
	h := &Hub{eng: n.Engine(), subs: map[*Sub]bool{}}
	n.Observe(h.observe)
	return h
}

// Subscribe returns a subscriber that receives the events f selects. A
// subscriber that falls more than buffer events behind (0: DefaultBuffer)
// loses events; Missed reports how many.
func (h *Hub) Subscribe(f filter.Filter, buffer int) *Sub {
	if buffer <= 0 {
		buffer = DefaultBuffer
	}
	c := make(chan Event, buffer)
	s := &Sub{C: c, c: c, f: f, hub: h}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		s.closed = true
		close(c)
		return s
	}
	h.subs[s] = true
	return s
}

// Missed returns how many events were dropped because the subscriber was
// behind, and resets the count.
func (s *Sub) Missed() uint64 {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	n := s.missed
	s.missed = 0
	return n
}

// Close stops the subscriber and closes C.
func (s *Sub) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if !s.closed {
		s.closed = true
		delete(s.hub.subs, s)
		close(s.c)
	}
}

// Close closes every subscriber, as when the node stops, and every one
// that subscribes after.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	subs := make([]*Sub, 0, len(h.subs))
	for s := range h.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()
	for _, s := range subs {
		s.Close()
	}
}

// PublishGrind sends a grind job's event to the subscribers whose filter
// selects it: by kind, and by series and pool when the event names a pool
// (job-wide events, such as a job's end, name none and pass any series or
// pool filter only when it is empty). It never blocks.
func (h *Hub) PublishGrind(ev grind.Event) {
	if !h.active() {
		return
	}
	e := Event{Time: time.Now().UTC(), Kind: filter.KindGrind, Series: ev.Series, Pool: ev.Pool, Grind: &ev}
	if !ev.JobWide() {
		h.deliver([]sized{{e: e, sub: filter.Subject{Kind: filter.KindGrind, Series: ev.Series, Pool: ev.Pool, HasPool: true}}})
		return
	}
	// A job-wide event (start, waiting, end) has no pool: it passes a
	// filter on series or pools when the job can work on one of them, so
	// a watcher of one series sees its jobs start and end (GREEN-017).
	h.deliver([]sized{{e: e, match: func(f filter.Filter) bool { return jobMatches(f, ev.Scope) }}})
}

// jobMatches reports whether filter f selects a job-wide grind event of a
// job with spec s.
func jobMatches(f filter.Filter, s *grind.Spec) bool {
	if !f.WantsKind(filter.KindGrind) || len(f.Serials) > 0 || len(f.Addresses) > 0 || len(f.Policies) > 0 || len(f.TIDs) > 0 {
		return false
	}
	if s == nil {
		return f.Empty() || (len(f.Series) == 0 && len(f.Pools) == 0)
	}
	if len(f.Series) > 0 && s.Series != nil && !slices.Contains(f.Series, *s.Series) {
		return false
	}
	if len(f.Pools) > 0 && len(s.Pools) > 0 && !slices.ContainsFunc(f.Pools, func(p uint64) bool { return slices.Contains(s.Pools, p) }) {
		return false
	}
	return true
}

func (h *Hub) active() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs) > 0
}

// observe runs under the node's lock, so it does the work of turning the
// item into events only when someone listens, and never blocks.
func (h *Hub) observe(ev node.Event) {
	if !h.active() {
		return
	}
	s, ok := h.eng.Series(ev.Key.Series)
	if !ok {
		return
	}
	base := Event{Time: time.Now().UTC(), Series: ev.Key.Series, Pool: ev.Key.Index, Weight: ev.Weight, Origin: originName(ev.Origin), From: ev.From}
	var out []sized
	switch {
	case ev.Tx != nil:
		v := TxOf(s, ev.Tx)
		e := base
		e.Kind, e.Tx, e.TID = filter.KindTx, &v, &v.TID
		out = append(out, sized{e: e, sub: v.Subject(s, filter.KindTx)})
	case ev.Entry != nil:
		txs := txsOf(s, ev.Entry)
		e := base
		e.Kind = filter.KindEntry
		e.Entry = &EventEntry{Hash: ev.Entry.Hash(s), Parent: ev.Entry.Parent, TIDs: ev.Entry.TIDs(s.Hash())}
		h.eng.Ledger(ev.Key, func(p *ledger.Pool) {
			if info, ok := p.Info(e.Entry.Hash); ok {
				e.Entry.Height, e.Entry.Selected = info.Height, info.Selected
			}
		})
		out = append(out, sized{e: e, sub: subjectOf(s, ev.Key.Index, filter.KindEntry, txs, nil)})
	case ev.Layer != nil:
		e := base
		e.Kind = filter.KindLayer
		e.Layer = &EventLayer{Entry: ev.Layer.Entry, Hash: ev.Layer.Hash(s.Hash()), Nonce: ev.Layer.Nonce,
			Strength: ev.Layer.Strength(s.Hash()), Credit: ev.Layer.Credit(s.Hash())}
		// A layer matters to whoever watches the transactions of its entry.
		var txs []Tx
		h.eng.Ledger(ev.Key, func(p *ledger.Pool) {
			if en, err := p.Entry(ev.Layer.Entry); err == nil {
				txs = txsOf(s, en)
			}
		})
		out = append(out, sized{e: e, sub: subjectOf(s, ev.Key.Index, filter.KindLayer, txs, nil)})
	}
	if ev.Switch != nil {
		out = append(out, h.switchEvents(s, ev.Key, base, ev.Switch)...)
	}
	for _, tid := range ev.Dropped {
		e := base
		t := tid
		e.Kind, e.TID = filter.KindDropped, &t
		out = append(out, sized{e: e, sub: filter.Subject{Series: s.ID(), Hash: s.Hash(), Pool: ev.Key.Index, HasPool: true, TIDs: []hash.Hash{tid}, Kind: filter.KindDropped}})
	}
	h.deliver(out)
}

// sized pairs an event with what filters match it against.
type sized struct {
	e   Event
	sub filter.Subject
	// match, when set, decides instead of matching sub.
	match func(filter.Filter) bool
}

func (h *Hub) switchEvents(s *series.Series, k engine.PoolKey, base Event, sw *ledger.Switch) []sized {
	var out []sized
	var applied []Tx
	h.eng.Ledger(k, func(p *ledger.Pool) {
		for _, eh := range sw.Applied {
			// A body that cannot be read back is a damaged log, which
			// poon doctor reports; the switch's events leave it out
			// (RED-084, D173).
			en, err := p.Entry(eh)
			if err != nil {
				continue
			}
			height, _ := p.HeightOf(eh)
			for _, t := range txsOf(s, en) {
				e := base
				v := t
				e.Kind, e.Tx, e.TID = filter.KindSelected, &v, &v.TID
				e.Entry = &EventEntry{Hash: eh, Height: height, Selected: true}
				out = append(out, sized{e: e, sub: v.Subject(s, filter.KindSelected)})
				applied = append(applied, t)
			}
		}
	})
	if sw.Extension() {
		return out
	}
	es := &EventSwitch{From: sw.From, To: sw.To, Fork: sw.Fork, ForkHeight: sw.ForkHeight,
		Reverted: nonNil(sw.Reverted), Applied: nonNil(sw.Applied), Carried: []hash.Hash{}, Void: []hash.Hash{}}
	var displaced []Tx
	var displacedTIDs []hash.Hash
	add := func(d ledger.Displaced, void bool) {
		e := base
		tid := d.TID
		e.Kind, e.TID, e.Void = filter.KindReverted, &tid, void
		e.Entry = &EventEntry{Hash: d.Entry}
		sub := filter.Subject{Series: s.ID(), Hash: s.Hash(), Pool: k.Index, HasPool: true, TIDs: []hash.Hash{tid}, Kind: filter.KindReverted}
		if d.Tx != nil {
			v := TxOf(s, d.Tx)
			e.Tx = &v
			sub = v.Subject(s, filter.KindReverted)
			displaced = append(displaced, v)
		} else {
			displacedTIDs = append(displacedTIDs, tid)
		}
		out = append(out, sized{e: e, sub: sub})
	}
	for _, d := range sw.Carried {
		es.Carried = append(es.Carried, d.TID)
		add(d, false)
	}
	for _, d := range sw.Void {
		es.Void = append(es.Void, d.TID)
		add(d, true)
	}
	e := base
	e.Kind, e.Switch = filter.KindSwitch, es
	all := append(append([]Tx(nil), applied...), displaced...)
	out = append(out, sized{e: e, sub: subjectOf(s, k.Index, filter.KindSwitch, all, displacedTIDs)})
	return out
}

func (h *Hub) deliver(evs []sized) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, x := range evs {
		h.seq++
		x.e.Seq = h.seq
		for s := range h.subs {
			if x.match != nil && !x.match(s.f) || x.match == nil && !s.f.Match(x.sub) {
				continue
			}
			select {
			case s.c <- x.e:
			default:
				s.missed++
			}
		}
	}
}

func originName(o engine.Origin) string {
	switch o {
	case engine.Pushed:
		return "pushed"
	case engine.Requested:
		return "requested"
	case engine.Local:
		return "local"
	case engine.Stored:
		return "stored"
	}
	return ""
}

func nonNil(l []hash.Hash) []hash.Hash {
	if l == nil {
		return []hash.Hash{}
	}
	return l
}
