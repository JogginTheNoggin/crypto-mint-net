package p2p

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// frameRoom is what a chunk's header may take beyond its body.
const frameRoom = 512

// serve answers one ledger data request from the active ledger. An error
// disconnects the peer.
func (h *Host) serve(p *peer, m wire.Message) error {
	if wire.RequestID(m.Type, m.Section, m.Body) != m.RequestID {
		return fmt.Errorf("%w: request id does not match the request", wire.ErrMalformed)
	}
	q, err := wire.DecodeRequest(m.Type, m.Body)
	if err != nil {
		return err
	}
	r := replier{h: h, p: p, m: m}
	pi := h.poolFor(m.Section)
	if pi == nil {
		r.empty(wire.TypeNotHeld, weight.Zero)
		return nil
	}
	h.checkStated(pi, p, m.Stated, nil)
	now := time.Now()
	c := q.Common()
	at := time.Unix(int64(c.Time), 0)
	if c.Time > 1<<62 || now.Sub(at) > h.cfg.RequestWindow || at.Sub(now) > h.cfg.ClockSkew {
		r.empty(wire.TypeRefused, h.ownWeight(pi))
		return nil
	}
	if !h.reqs.add(m.RequestID, now) {
		// The same request again: refused, and counted as a repeat. It is
		// counted before the refusal goes out, so whoever sees the refusal
		// also sees the count; any action follows the refusal.
		a := h.tr.Repeat(p.key, now)
		r.empty(wire.TypeRefused, h.ownWeight(pi))
		if a != tracking.None {
			h.act(p, a)
		}
		return nil
	}
	r.lim = h.limits(c.Limits)
	r.text = pi.section.Text()
	_, r.large = q.(wire.FullLedgerReq)
	var reply func()
	err = h.n.Engine().Ledger(pi.key, func(l *ledger.Pool) {
		r.stated = l.Weight()
		reply = r.build(pi, l, q)
	})
	if err != nil {
		r.empty(wire.TypeNotHeld, weight.Zero)
		return nil
	}
	reply() // sending happens outside the engine's lock
	return nil
}

// served are the limits that apply to one reply: the asker's, capped by
// this node's. A zero asker limit means the asker set none.
type served struct {
	chunk           int
	entries, hashes int
	layers          int
}

func capTo(asked uint32, max int) int {
	if asked == 0 || int64(asked) > int64(max) {
		return max
	}
	return int(asked)
}

func (h *Host) limits(l wire.Limits) served {
	return served{
		chunk:   capTo(l.ChunkSize, wire.MaxFrame-frameRoom),
		entries: capTo(l.EntryLimit, h.cfg.ServeEntries),
		hashes:  capTo(l.HashLimit, h.cfg.ServeHashes),
		layers:  capTo(l.LayerLimit, 1<<20),
	}
}

type replier struct {
	h      *Host
	p      *peer
	m      wire.Message
	stated weight.Weight
	lim    served
	text   string // the pool, for the cost of the reply
	large  bool   // a whole ledger was asked for
}

func (r replier) header(t wire.Type, chunk uint32, flags uint8) wire.Header {
	return wire.Header{Type: t, Section: r.m.Section, RequestID: r.m.RequestID, Chunk: chunk, Flags: flags, Stated: r.stated}
}

// empty sends a one-chunk reply with no body.
func (r replier) empty(t wire.Type, stated weight.Weight) {
	r.stated = stated
	r.p.send(wire.Message{Header: r.header(t, 1, wire.FlagLast)})
}

// chunks sends bodies as the chunks of one reply.
func (r replier) chunks(t wire.Type, bodies [][]byte, more bool) {
	if len(bodies) == 0 {
		bodies = [][]byte{nil}
	}
	// The cost is counted before the reply goes out, so whoever sees the
	// reply also sees it counted.
	if r.text != "" {
		n := 0
		for _, b := range bodies {
			n += len(b)
		}
		r.h.tr.Cost(r.p.key, r.text, n, 0, r.large, time.Now())
	}
	for i, b := range bodies {
		var f uint8
		if i == len(bodies)-1 {
			f = wire.FlagLast
			if more {
				f |= wire.FlagMore
			}
		}
		r.p.send(wire.Message{Header: r.header(t, uint32(i+1), f), Body: b})
	}
}

// split cuts n items, of the given encoded sizes, into runs that fit the
// chunk size. An item larger than a chunk goes alone.
func split(sizes []int, chunk int) [][2]int {
	var out [][2]int
	start, size := 0, 0
	for i, s := range sizes {
		if i > start && size+s > chunk {
			out = append(out, [2]int{start, i})
			start, size = i, 0
		}
		size += s
	}
	if start < len(sizes) || len(out) == 0 {
		out = append(out, [2]int{start, len(sizes)})
	}
	return out
}

// item returns a held entry as a reply item.
func item(l *ledger.Pool, h hash.Hash, layers bool) wire.EntryItem {
	e, _ := l.Entry(h)
	it := wire.EntryItem{Entry: e, HasLayers: layers}
	if layers {
		for _, ly := range l.Layers(h) {
			it.Nonces = append(it.Nonces, ly.Nonce)
		}
	}
	return it
}

// build reads what the reply needs under the engine's lock and returns a
// function that sends it.
func (r replier) build(pi *poolInfo, l *ledger.Pool, q wire.Request) func() {
	switch q := q.(type) {
	case wire.ExtensionReq:
		if !l.Weight().Greater(q.Beat) {
			return func() { r.empty(wire.TypeNothingHeavier, r.stated) }
		}
		from := 0
		if q.From != pi.s.PoolID(pi.key.Index) {
			h, ok := l.HeightOf(q.From)
			if !ok {
				return func() { r.empty(wire.TypeNotFromHere, r.stated) }
			}
			from = h
		}
		return r.entries(l, from, q.Variant == wire.EntriesWithLayers)
	case wire.FullLedgerReq:
		return r.entries(l, 0, q.Variant == wire.EntriesWithLayers)
	case wire.HashChainReq:
		return r.hashChain(l, q)
	case wire.EntriesReq:
		return r.byHash(l, q)
	case wire.LayersReq:
		return r.layers(pi, l, q)
	case wire.WeightReq:
		return func() { r.empty(wire.TypeWeight, r.stated) }
	case wire.PartsReq:
		return r.parts(l, q)
	case wire.LayersFromReq:
		return r.layersFrom(pi, l, q)
	}
	return func() { r.empty(wire.TypeRefused, r.stated) }
}

// entries replies with the active ledger's entries after height from.
func (r replier) entries(l *ledger.Pool, from int, layers bool) func() {
	hs := l.Chain(from+1, from+r.lim.entries)
	more := from+len(hs) < l.Height()
	items := make([]wire.EntryItem, 0, len(hs))
	var sizes []int
	total := 0
	for _, h := range hs {
		it := item(l, h, layers)
		if layers && total+len(it.Nonces) > r.lim.layers && len(items) > 0 {
			more = true
			break
		}
		size := it.Size()
		if size > r.h.cfg.PartsOver {
			// Too large for one chunk: the reply ends before it, and the
			// asker fetches it in parts (D117).
			if len(items) == 0 {
				return r.tooLarge(h, size)
			}
			more = true
			break
		}
		total += len(it.Nonces)
		items = append(items, it)
		sizes = append(sizes, size)
	}
	return func() {
		var bodies [][]byte
		for _, run := range split(sizes, r.lim.chunk) {
			bodies = append(bodies, wire.EntriesChunk{Items: items[run[0]:run[1]]}.Encode())
		}
		r.chunks(wire.TypeExtension, bodies, more)
	}
}

// chunkRoom is what an entries chunk takes besides its items.
const chunkRoom = 16

// maxWhole is the largest entry item one frame can carry.
const maxWhole = wire.MaxFrame - frameRoom - chunkRoom - hash.Size - 1

// maxNonces is the most layer nonces of one entry a layers chunk holds.
const maxNonces = (wire.MaxFrame - frameRoom - chunkRoom - hash.Size - 5) / 8

// minPart is the least data a part carries, whatever chunk size the
// asker set.
const minPart = 1 << 10

// partsReply is the most bytes of an item one parts reply carries; the
// asker asks again for the rest.
const partsReply = 64 << 20

// tooLarge replies that the next entry, h, is larger than PartsOver.
func (r replier) tooLarge(h hash.Hash, size int) func() {
	return func() {
		r.chunks(wire.TypeLarge, [][]byte{wire.Large{Entry: h, Size: uint64(size)}.Encode()}, false)
	}
}

// parts replies with the encoded item of an entry of the active ledger
// from the asked offset, in chunks, up to partsReply bytes (D117). Under
// the engine's lock it only copies the item (the entry is immutable, the
// nonces are copied); the encoding happens when the reply is sent, once
// per item for a while (partsCache), so a fetch in many requests encodes
// it once and sees one total (RED-017, decision D120).
func (r replier) parts(l *ledger.Pool, q wire.PartsReq) func() {
	if !l.InSelected(q.Entry) {
		return func() { r.empty(wire.TypeNotHeld, r.stated) }
	}
	// A cached item needs nothing from the pool. Otherwise only the entry
	// and an unsorted copy of its nonces are taken under the lock; the sort
	// and the encoding happen outside it (RED-017, RED-020, D123).
	key := partsKey{entry: q.Entry, variant: q.Variant}
	b, _, cached := r.h.encoded.peek(key)
	var it wire.EntryItem
	if !cached {
		e, _ := l.Entry(q.Entry)
		it = wire.EntryItem{Entry: e, HasLayers: q.Variant == wire.EntriesWithLayers}
		if it.HasLayers {
			it.Nonces = l.LayerNonces(q.Entry)
		}
	}
	return func() {
		if !cached {
			slices.Sort(it.Nonces)
			b = wire.EncodeItem(it)
			r.h.encoded.put(key, b, nil)
		}
		total := uint64(len(b))
		if q.Offset >= total {
			r.empty(wire.TypeRefused, r.stated)
			return
		}
		per := r.lim.chunk - chunkRoom
		if per < minPart {
			per = minPart
		}
		var bodies [][]byte
		off := q.Offset
		for off < total && off-q.Offset < partsReply {
			end := off + uint64(per)
			if end > total {
				end = total
			}
			bodies = append(bodies, wire.Part{Total: total, Offset: off, Data: b[off:end]}.Encode())
			off = end
		}
		r.chunks(wire.TypePart, bodies, off < total)
	}
}

// partsKey names one cached item: an encoded entry item for parts
// requests, or (order set) the strongest-first order of an entry's layers
// when it held layers of them, for layers-from requests.
type partsKey struct {
	entry   hash.Hash
	variant wire.Variant
	order   bool
	layers  int
}

// partsCache keeps the encoded items and layer orders requests were last
// answered from: up to partsCacheBytes in all (always the newest one),
// each for partsCacheFor after its last use, so a fetch in many requests
// does its work once (RED-017, RED-020).
type partsCache struct {
	mu    sync.Mutex
	items map[partsKey]*encodedItem
}

type encodedItem struct {
	b    []byte
	ns   []uint64
	used time.Time
}

func (e *encodedItem) size() int { return len(e.b) + 8*len(e.ns) }

const (
	partsCacheBytes = 256 << 20
	partsCacheFor   = time.Minute
)

// peek returns what is cached under k, marking it used.
func (c *partsCache) peek(k partsKey) ([]byte, []uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[k]
	if !ok {
		return nil, nil, false
	}
	e.used = time.Now()
	return e.b, e.ns, true
}

// put caches b or ns under k and lets go of items unused for
// partsCacheFor, then of the oldest while the cache is over its size.
func (c *partsCache) put(k partsKey, b []byte, ns []uint64) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[partsKey]*encodedItem{}
	}
	c.items[k] = &encodedItem{b: b, ns: ns, used: now}
	size := 0
	for kk, e := range c.items {
		if kk != k && now.Sub(e.used) > partsCacheFor {
			delete(c.items, kk)
			continue
		}
		size += e.size()
	}
	for size > partsCacheBytes {
		var old partsKey
		var oldest *encodedItem
		for kk, e := range c.items {
			if kk != k && (oldest == nil || e.used.Before(oldest.used)) {
				old, oldest = kk, e
			}
		}
		if oldest == nil {
			break
		}
		size -= oldest.size()
		delete(c.items, old)
	}
}

// get returns the encoding of it, cached under k.
func (c *partsCache) get(k partsKey, it wire.EntryItem) []byte {
	if b, _, ok := c.peek(k); ok {
		return b
	}
	b := wire.EncodeItem(it)
	c.put(k, b, nil)
	return b
}

func (r replier) hashChain(l *ledger.Pool, q wire.HashChainReq) func() {
	height := l.Height()
	count := r.lim.hashes
	if q.Count > 0 && int64(q.Count) < int64(count) {
		count = int(q.Count)
	}
	var lo, hi int // heights, inclusive
	cut := false
	switch {
	case !q.HasAnchor && q.Count > 0:
		hi = height
		lo = height - count + 1
		cut = int64(q.Count) > int64(count) && lo > 1
	case !q.HasAnchor:
		lo, hi = 1, count
		cut = count < height
	default:
		a, ok := l.HeightOf(q.Anchor)
		if !ok {
			return func() { r.empty(wire.TypeAnchorNotHeld, r.stated) }
		}
		if q.Dir == wire.Before {
			hi = a - 1
			lo = hi - count + 1
			cut = q.Count == 0 && lo > 1
		} else {
			lo = a + 1
			hi = a + count
			cut = hi < height
		}
	}
	if lo < 1 {
		lo = 1
	}
	hs := l.Chain(lo, hi)
	fromStart := lo == 1 && len(hs) > 0
	return func() {
		per := (r.lim.chunk - 16) / hash.Size
		if per < 1 {
			per = 1
		}
		var bodies [][]byte
		for i := 0; i < len(hs) || i == 0; i += per {
			j := i + per
			if j > len(hs) {
				j = len(hs)
			}
			bodies = append(bodies, wire.HashChainChunk{FromStart: fromStart && i == 0, Hashes: hs[i:j]}.Encode())
			if j == len(hs) {
				break
			}
		}
		r.chunks(wire.TypeHashChain, bodies, cut)
	}
}

func (r replier) byHash(l *ledger.Pool, q wire.EntriesReq) func() {
	layers := q.Variant == wire.EntriesWithLayers
	var items []wire.HeldEntry
	sent, nl := 0, 0
	more := false
	for _, h := range q.Hashes {
		if !l.InSelected(h) {
			items = append(items, wire.HeldEntry{Hash: h})
			continue
		}
		it := item(l, h, layers)
		if sent == r.lim.entries || (sent > 0 && nl+len(it.Nonces) > r.lim.layers) {
			more = true
			break
		}
		if size := it.Size(); size > r.h.cfg.PartsOver {
			if len(items) == 0 {
				return r.tooLarge(h, size)
			}
			more = true
			break
		}
		sent++
		nl += len(it.Nonces)
		items = append(items, wire.HeldEntry{Hash: h, Held: true, Item: it})
	}
	return func() {
		sizes := make([]int, len(items))
		for i, it := range items {
			sizes[i] = hash.Size + 1
			if it.Held {
				sizes[i] += it.Item.Size()
			}
		}
		var bodies [][]byte
		for _, run := range split(sizes, r.lim.chunk) {
			bodies = append(bodies, wire.HeldEntriesChunk{Items: items[run[0]:run[1]]}.Encode())
		}
		r.chunks(wire.TypeEntries, bodies, more)
	}
}

// layersFrom replies with an entry's layer nonces from the asked index,
// strongest first, in chunks, up to partsReply bytes (D121). Under the
// engine's lock it counts the layers and, unless the order for that count
// is cached, copies the nonces unsorted; the hashing and the sort happen
// outside the lock, once for all the pages of a fetch (RED-020, D123).
func (r replier) layersFrom(pi *poolInfo, l *ledger.Pool, q wire.LayersFromReq) func() {
	if !l.InSelected(q.Entry) {
		return func() { r.empty(wire.TypeNotHeld, r.stated) }
	}
	n := l.LayerCount(q.Entry)
	if n >= 1<<32 || int64(q.From) > int64(n) {
		return func() { r.empty(wire.TypeRefused, r.stated) }
	}
	key := partsKey{entry: q.Entry, order: true, layers: n}
	_, order, cached := r.h.encoded.peek(key)
	if !cached {
		order = l.LayerNonces(q.Entry)
	}
	fn := pi.s.Hash()
	return func() {
		if !cached {
			strongestNonces(fn, q.Entry, order)
			r.h.encoded.put(key, nil, order)
		}
		total := uint32(len(order))
		per := max((r.lim.chunk-chunkRoom)/8, minPart/8)
		var bodies [][]byte
		i := int(q.From)
		for i < len(order) && (i-int(q.From))*8 < partsReply {
			end := min(i+per, len(order))
			bodies = append(bodies, wire.LayersFrom{Total: total, From: uint32(i), Nonces: order[i:end]}.Encode())
			i = end
		}
		if len(bodies) == 0 {
			bodies = [][]byte{wire.LayersFrom{Total: total, From: q.From}.Encode()}
		}
		r.chunks(wire.TypeLayersFrom, bodies, i < len(order))
	}
}

// strongestNonces orders the layer nonces of entry eh as a layers-from
// reply gives them: strongest first, the lower nonce first among equals.
func strongestNonces(fn hash.Func, eh hash.Hash, ns []uint64) {
	st := make(map[uint64]int, len(ns))
	for _, n := range ns {
		st[n] = layer.Layer{Entry: eh, Nonce: n}.Strength(fn)
	}
	slices.SortFunc(ns, func(a, b uint64) int {
		if st[a] != st[b] {
			return st[b] - st[a]
		}
		return cmp.Compare(a, b)
	})
}

func (r replier) layers(pi *poolInfo, l *ledger.Pool, q wire.LayersReq) func() {
	var items []wire.HeldLayers
	total := 0
	more := false
	for _, h := range q.Hashes {
		if !l.InSelected(h) {
			items = append(items, wire.HeldLayers{Entry: h})
			continue
		}
		if c := l.LayerCount(h); c > r.h.cfg.LayersWhole {
			// More layers than a reply carries whole: the reply ends
			// before the entry, and the asker pages its layers (D121).
			// Only counted, not copied, under the lock (RED-020).
			if len(items) == 0 {
				n := uint32(min(c, 1<<32-1))
				return func() {
					r.chunks(wire.TypeManyLayers, [][]byte{wire.ManyLayers{Entry: h, Count: n}.Encode()}, false)
				}
			}
			more = true
			break
		}
		lys := l.Layers(h)
		var ns []uint64
		for _, ly := range lys {
			ns = append(ns, ly.Nonce)
		}
		if total > 0 && total+len(ns) > r.lim.layers {
			more = true
			break
		}
		total += len(ns)
		items = append(items, wire.HeldLayers{Entry: h, Held: true, Nonces: ns})
	}
	return func() {
		sizes := make([]int, len(items))
		for i, it := range items {
			sizes[i] = hash.Size + 5 + 8*len(it.Nonces)
		}
		var bodies [][]byte
		for _, run := range split(sizes, r.lim.chunk) {
			bodies = append(bodies, wire.LayersChunk{Items: items[run[0]:run[1]]}.Encode())
		}
		r.chunks(wire.TypeLayers, bodies, more)
	}
}
