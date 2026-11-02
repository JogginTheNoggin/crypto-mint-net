package wire

import (
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Bounds on lists in bodies. They only stop a hostile count from causing a
// large allocation; the frame size is the real limit.
const (
	maxCoverage = 4096
	maxHashes   = 1 << 16
	maxItems    = 1 << 16
	maxLayers   = 1 << 20
	maxString   = 256
)

func finish(r *encoding.Reader, what string) error {
	if err := r.Finish(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrMalformed, what, err)
	}
	return nil
}

// Hello opens a connection. Each side sends one first and nothing else
// until it has the other's.
type Hello struct {
	// Protocol is the highest protocol version the sender speaks.
	Protocol uint16
	// NodeID is random per run, so a node can tell it connected to itself.
	NodeID hash.Hash
	// Listen is the address where the sender accepts connections, or empty.
	Listen string
	// Agent names the software, for logs only.
	Agent string
	// Coverage lists the mint policies the sender follows.
	Coverage []Section
}

// Encode returns the body.
func (h Hello) Encode() []byte {
	w := encoding.NewWriter(128 + 140*len(h.Coverage))
	w.U16(h.Protocol).Raw(h.NodeID[:]).VarBytes([]byte(h.Listen)).VarBytes([]byte(h.Agent))
	w.Count(len(h.Coverage))
	for _, s := range h.Coverage {
		s.encode(w)
	}
	return w.Bytes()
}

// DecodeHello reads a hello body.
func DecodeHello(b []byte) (Hello, error) {
	r := encoding.NewReader(b)
	var h Hello
	h.Protocol = r.U16()
	r.Into(h.NodeID[:])
	h.Listen = string(r.VarBytes(maxString))
	h.Agent = string(r.VarBytes(maxString))
	n := r.Count(maxCoverage, 1)
	for i := 0; i < n && r.Err() == nil; i++ {
		h.Coverage = append(h.Coverage, decodeSection(r))
	}
	return h, finish(r, "hello")
}

// DecodeTx reads a pushed transaction body.
func DecodeTx(b []byte) (*tx.Tx, error) {
	t, err := tx.Decode(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return t, nil
}

// DecodeEntry reads a pushed entry body.
func DecodeEntry(b []byte) (*entry.Entry, error) {
	e, err := entry.Decode(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return e, nil
}

// DecodeLayer reads a pushed layer body.
func DecodeLayer(b []byte) (layer.Layer, error) {
	l, err := layer.Decode(b)
	if err != nil {
		return l, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return l, nil
}

// Notice tells peers a node switched to a heavier ledger. The stated weight
// is in the header.
type Notice struct {
	Tip hash.Hash
}

// Encode returns the body.
func (n Notice) Encode() []byte { return append([]byte(nil), n.Tip[:]...) }

// DecodeNotice reads a notice body.
func DecodeNotice(b []byte) (Notice, error) {
	r := encoding.NewReader(b)
	var n Notice
	r.Into(n.Tip[:])
	return n, finish(r, "notice")
}

// Limits are the asker's reply limits; 0 means no limit.
type Limits struct {
	ChunkSize  uint32
	EntryLimit uint32
	HashLimit  uint32
	LayerLimit uint32
}

func (l Limits) encode(w *encoding.Writer) {
	w.U32(l.ChunkSize).U32(l.EntryLimit).U32(l.HashLimit).U32(l.LayerLimit)
}

func decodeLimits(r *encoding.Reader) Limits {
	return Limits{r.U32(), r.U32(), r.U32(), r.U32()}
}

// RequestCommon starts every request body: a random nonce, the time the
// request was made (Unix seconds) and the asker's limits.
type RequestCommon struct {
	Nonce  uint64
	Time   uint64
	Limits Limits
}

func (c RequestCommon) encode(w *encoding.Writer) {
	w.U64(c.Nonce).U64(c.Time)
	c.Limits.encode(w)
}

func decodeCommon(r *encoding.Reader) RequestCommon {
	return RequestCommon{Nonce: r.U64(), Time: r.U64(), Limits: decodeLimits(r)}
}

// Variant says whether a reply with entries also carries their layers.
type Variant uint8

const (
	EntriesOnly       Variant = 0
	EntriesWithLayers Variant = 1
)

func decodeVariant(r *encoding.Reader) Variant {
	v := Variant(r.U8())
	if v > EntriesWithLayers {
		r.Fail(fmt.Errorf("variant %d", v))
	}
	return v
}

// Request is any ledger data request body.
type Request interface {
	Type() Type
	Common() RequestCommon
	Encode() []byte
}

// ExtensionReq asks for the entries after From, if the peer's active
// ledger is heavier than Beat.
type ExtensionReq struct {
	RequestCommon
	From    hash.Hash
	Beat    weight.Weight
	Variant Variant
}

func (ExtensionReq) Type() Type               { return TypeExtensionReq }
func (q ExtensionReq) Common() RequestCommon  { return q.RequestCommon }
func (q HashChainReq) Common() RequestCommon  { return q.RequestCommon }
func (q EntriesReq) Common() RequestCommon    { return q.RequestCommon }
func (q FullLedgerReq) Common() RequestCommon { return q.RequestCommon }
func (q LayersReq) Common() RequestCommon     { return q.RequestCommon }
func (q WeightReq) Common() RequestCommon     { return q.RequestCommon }
func (HashChainReq) Type() Type               { return TypeHashChainReq }
func (EntriesReq) Type() Type                 { return TypeEntriesReq }
func (FullLedgerReq) Type() Type              { return TypeFullLedgerReq }
func (LayersReq) Type() Type                  { return TypeLayersReq }
func (WeightReq) Type() Type                  { return TypeWeightReq }
func (q ExtensionReq) Encode() []byte         { return enc(func(w *encoding.Writer) { q.encode(w) }) }
func (q HashChainReq) Encode() []byte         { return enc(func(w *encoding.Writer) { q.encode(w) }) }
func (q EntriesReq) Encode() []byte           { return enc(func(w *encoding.Writer) { q.encode(w) }) }
func (q FullLedgerReq) Encode() []byte        { return enc(func(w *encoding.Writer) { q.encode(w) }) }
func (q LayersReq) Encode() []byte            { return enc(func(w *encoding.Writer) { q.encode(w) }) }
func (q WeightReq) Encode() []byte {
	return enc(func(w *encoding.Writer) { q.RequestCommon.encode(w) })
}
func enc(f func(*encoding.Writer)) []byte { w := encoding.NewWriter(64); f(w); return w.Bytes() }
func (q ExtensionReq) encode(w *encoding.Writer) {
	q.RequestCommon.encode(w)
	w.Raw(q.From[:]).VarBytes(q.Beat.Bytes()).U8(uint8(q.Variant))
}

// Direction of a hash chain anchor.
type Direction uint8

const (
	Before Direction = 0
	After  Direction = 1
)

// HashChainReq asks for entry hashes of the peer's active ledger.
type HashChainReq struct {
	RequestCommon
	HasAnchor bool
	Anchor    hash.Hash
	Dir       Direction
	// Count is how many hashes, or 0 for no count.
	Count uint32
}

func (q HashChainReq) encode(w *encoding.Writer) {
	q.RequestCommon.encode(w)
	w.Bool(q.HasAnchor)
	if q.HasAnchor {
		w.Raw(q.Anchor[:]).U8(uint8(q.Dir))
	}
	w.U32(q.Count)
}

// EntriesReq asks for entries by hashes the same peer sent.
type EntriesReq struct {
	RequestCommon
	Variant Variant
	Hashes  []hash.Hash
}

func (q EntriesReq) encode(w *encoding.Writer) {
	q.RequestCommon.encode(w)
	w.U8(uint8(q.Variant)).Count(len(q.Hashes))
	for _, h := range q.Hashes {
		w.Raw(h[:])
	}
}

// FullLedgerReq asks for the peer's whole active ledger.
type FullLedgerReq struct {
	RequestCommon
	Variant Variant
}

func (q FullLedgerReq) encode(w *encoding.Writer) {
	q.RequestCommon.encode(w)
	w.U8(uint8(q.Variant))
}

// LayersReq asks for the layers on entries.
type LayersReq struct {
	RequestCommon
	Hashes []hash.Hash
}

func (q LayersReq) encode(w *encoding.Writer) {
	q.RequestCommon.encode(w)
	w.Count(len(q.Hashes))
	for _, h := range q.Hashes {
		w.Raw(h[:])
	}
}

// WeightReq asks only for the peer's stated weight.
type WeightReq struct {
	RequestCommon
}

func readHashes(r *encoding.Reader) []hash.Hash {
	n := r.Count(maxHashes, hash.Size)
	out := make([]hash.Hash, 0, n)
	for i := 0; i < n && r.Err() == nil; i++ {
		var h hash.Hash
		r.Into(h[:])
		out = append(out, h)
	}
	return out
}

// DecodeRequest reads the body of a request of type t.
func DecodeRequest(t Type, b []byte) (Request, error) {
	r := encoding.NewReader(b)
	c := decodeCommon(r)
	var q Request
	switch t {
	case TypeExtensionReq:
		x := ExtensionReq{RequestCommon: c}
		r.Into(x.From[:])
		bw := r.VarBytes(64)
		x.Variant = decodeVariant(r)
		if r.Err() == nil {
			w, err := weight.FromBytes(bw)
			if err != nil {
				r.Fail(err)
			}
			x.Beat = w
		}
		q = x
	case TypeHashChainReq:
		x := HashChainReq{RequestCommon: c}
		x.HasAnchor = r.Bool()
		if x.HasAnchor {
			r.Into(x.Anchor[:])
			x.Dir = Direction(r.U8())
			if x.Dir > After {
				r.Fail(fmt.Errorf("direction %d", x.Dir))
			}
		}
		x.Count = r.U32()
		q = x
	case TypeEntriesReq:
		x := EntriesReq{RequestCommon: c}
		x.Variant = decodeVariant(r)
		x.Hashes = readHashes(r)
		q = x
	case TypeFullLedgerReq:
		q = FullLedgerReq{RequestCommon: c, Variant: decodeVariant(r)}
	case TypeLayersReq:
		q = LayersReq{RequestCommon: c, Hashes: readHashes(r)}
	case TypeWeightReq:
		q = WeightReq{RequestCommon: c}
	default:
		return nil, fmt.Errorf("%w: %s is not a request", ErrMalformed, t)
	}
	return q, finish(r, t.String())
}

// EntryItem is one entry in a reply, with its layers' nonces when the
// variant asks for them. The layers name the entry's hash, so only their
// nonces travel.
type EntryItem struct {
	Entry     *entry.Entry
	HasLayers bool
	Nonces    []uint64
}

func (it EntryItem) encode(w *encoding.Writer) {
	it.Entry.EncodeTo(w)
	w.Bool(it.HasLayers)
	if it.HasLayers {
		w.Count(len(it.Nonces))
		for _, n := range it.Nonces {
			w.U64(n)
		}
	}
}

func decodeEntryItem(r *encoding.Reader) EntryItem {
	var it EntryItem
	it.Entry = entry.DecodeFrom(r)
	it.HasLayers = r.Bool()
	if it.HasLayers {
		n := r.Count(maxLayers, 8)
		it.Nonces = make([]uint64, 0, n)
		for i := 0; i < n && r.Err() == nil; i++ {
			it.Nonces = append(it.Nonces, r.U64())
		}
	}
	return it
}

// Size returns the encoded size of the item.
func (it EntryItem) Size() int {
	n := len(it.Entry.Encode()) + 1
	if it.HasLayers {
		n += 4 + 8*len(it.Nonces)
	}
	return n
}

// EntriesChunk is the body of an extension or full ledger reply chunk:
// entries in chain order.
type EntriesChunk struct {
	Items []EntryItem
}

// Encode returns the body.
func (c EntriesChunk) Encode() []byte {
	w := encoding.NewWriter(1024)
	w.Count(len(c.Items))
	for _, it := range c.Items {
		it.encode(w)
	}
	return w.Bytes()
}

// DecodeEntriesChunk reads an extension reply chunk.
func DecodeEntriesChunk(b []byte) (EntriesChunk, error) {
	r := encoding.NewReader(b)
	n := r.Count(maxItems, 45)
	var c EntriesChunk
	for i := 0; i < n && r.Err() == nil; i++ {
		c.Items = append(c.Items, decodeEntryItem(r))
	}
	return c, finish(r, "entries chunk")
}

// HashChainChunk is the body of a hash chain reply chunk.
type HashChainChunk struct {
	// FromStart: the first hash of the whole reply is the pool's first
	// entry. Set on the first chunk.
	FromStart bool
	Hashes    []hash.Hash
}

// Encode returns the body.
func (c HashChainChunk) Encode() []byte {
	w := encoding.NewWriter(8 + 32*len(c.Hashes))
	w.Bool(c.FromStart).Count(len(c.Hashes))
	for _, h := range c.Hashes {
		w.Raw(h[:])
	}
	return w.Bytes()
}

// DecodeHashChainChunk reads a hash chain reply chunk.
func DecodeHashChainChunk(b []byte) (HashChainChunk, error) {
	r := encoding.NewReader(b)
	var c HashChainChunk
	c.FromStart = r.Bool()
	c.Hashes = readHashes(r)
	return c, finish(r, "hash chain chunk")
}

// HeldEntry is one answer of an entries-by-hash reply.
type HeldEntry struct {
	Hash hash.Hash
	Held bool
	Item EntryItem
}

// HeldEntriesChunk is the body of an entries-by-hash reply chunk.
type HeldEntriesChunk struct {
	Items []HeldEntry
}

// Encode returns the body.
func (c HeldEntriesChunk) Encode() []byte {
	w := encoding.NewWriter(1024)
	w.Count(len(c.Items))
	for _, it := range c.Items {
		w.Raw(it.Hash[:]).Bool(it.Held)
		if it.Held {
			it.Item.encode(w)
		}
	}
	return w.Bytes()
}

// DecodeHeldEntriesChunk reads an entries-by-hash reply chunk.
func DecodeHeldEntriesChunk(b []byte) (HeldEntriesChunk, error) {
	r := encoding.NewReader(b)
	n := r.Count(maxItems, 33)
	var c HeldEntriesChunk
	for i := 0; i < n && r.Err() == nil; i++ {
		var it HeldEntry
		r.Into(it.Hash[:])
		it.Held = r.Bool()
		if it.Held {
			it.Item = decodeEntryItem(r)
		}
		c.Items = append(c.Items, it)
	}
	return c, finish(r, "entries chunk")
}

// HeldLayers is the layers on one entry in a layers reply.
type HeldLayers struct {
	Entry  hash.Hash
	Held   bool
	Nonces []uint64
}

// LayersChunk is the body of a layers reply chunk.
type LayersChunk struct {
	Items []HeldLayers
}

// Encode returns the body.
func (c LayersChunk) Encode() []byte {
	w := encoding.NewWriter(1024)
	w.Count(len(c.Items))
	for _, it := range c.Items {
		w.Raw(it.Entry[:]).Bool(it.Held)
		if it.Held {
			w.Count(len(it.Nonces))
			for _, n := range it.Nonces {
				w.U64(n)
			}
		}
	}
	return w.Bytes()
}

// DecodeLayersChunk reads a layers reply chunk.
func DecodeLayersChunk(b []byte) (LayersChunk, error) {
	r := encoding.NewReader(b)
	n := r.Count(maxItems, 33)
	var c LayersChunk
	for i := 0; i < n && r.Err() == nil; i++ {
		var it HeldLayers
		r.Into(it.Entry[:])
		it.Held = r.Bool()
		if it.Held {
			m := r.Count(maxLayers, 8)
			it.Nonces = make([]uint64, 0, m)
			for j := 0; j < m && r.Err() == nil; j++ {
				it.Nonces = append(it.Nonces, r.U64())
			}
		}
		c.Items = append(c.Items, it)
	}
	return c, finish(r, "layers chunk")
}

// DecodeEmpty checks that a reply that carries nothing has an empty body.
func DecodeEmpty(t Type, b []byte) error {
	if len(b) != 0 {
		return fmt.Errorf("%w: %s carries a body", ErrMalformed, t)
	}
	return nil
}
