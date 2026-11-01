// Package wire is the byte form of the messages full nodes send each other
// over TCP: framing, the common header and every message body.
//
// A connection carries frames. A frame is a U32 length and that many bytes
// of message. A message is the header followed by the body:
//
//	U8   wire version (1)
//	U16  message type
//	     mint policy section (see Section)
//	Hash request id (zero on pushed data)
//	U32  chunk position (0 when the message is not a chunk)
//	U8   flags: bit 0 last chunk, bit 1 more marker
//	Bytes stated weight (minimal big-endian; empty on a hello)
//	U16  hop count (0; relaying comes with peer discovery)
//	     body, to the end of the frame
//
// The header lets a node decide whether a message concerns it before it
// reads the body. Stated weight is the sender's own computed weight of its
// active ledger for the pool in the section. It is a statement, never
// weight: a node only uses it to decide whether to ask for more.
//
// Package wire only encodes and decodes. What a node does with a message is
// in package p2p.
package wire

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Version is the wire version this package speaks.
const Version = 1

// MaxFrame is the largest frame any node accepts. A node's own chunk size
// may be smaller.
const MaxFrame = 16 << 20

// ErrMalformed marks a message that does not decode, or whose header and
// body do not agree. A peer that sends one is misbehaving.
var ErrMalformed = errors.New("wire: malformed message")

// Type is a message type.
type Type uint16

// Message types. Numbers are never reused.
const (
	TypeHello Type = 1

	TypeTx     Type = 10
	TypeEntry  Type = 11
	TypeLayer  Type = 12
	TypeNotice Type = 13

	TypeExtensionReq  Type = 20
	TypeHashChainReq  Type = 21
	TypeEntriesReq    Type = 22
	TypeFullLedgerReq Type = 23
	TypeLayersReq     Type = 24
	TypeWeightReq     Type = 25
	// TypePartsReq asks for an entry too large for one frame, in parts
	// (consensus audit P1, decision D117).
	TypePartsReq Type = 26
	// TypeLayersFromReq asks for the layers of one entry with more than a
	// layers reply carries whole, from an index on (D121).
	TypeLayersFromReq Type = 27

	TypeExtension      Type = 40
	TypeNothingHeavier Type = 41
	TypeNotFromHere    Type = 42
	TypeHashChain      Type = 43
	TypeAnchorNotHeld  Type = 44
	TypeEntries        Type = 45
	TypeLayers         Type = 46
	TypeWeight         Type = 47
	TypeNotHeld        Type = 48
	TypeRefused        Type = 49
	// TypeLarge says the next entry of a reply is too large for one
	// chunk: the asker fetches it with a parts request and goes on after
	// it. TypePart carries one part (D117).
	TypeLarge Type = 50
	TypePart  Type = 51
	// TypeManyLayers says the next entry of a layers reply has more layers
	// than one reply carries whole; TypeLayersFrom carries them a page at
	// a time, strongest first (D121).
	TypeManyLayers Type = 52
	TypeLayersFrom Type = 53
)

var typeNames = map[Type]string{
	TypeHello: "hello", TypeTx: "tx", TypeEntry: "entry", TypeLayer: "layer", TypeNotice: "notice",
	TypeExtensionReq: "extension-request", TypeHashChainReq: "hash-chain-request", TypeEntriesReq: "entries-request",
	TypeFullLedgerReq: "full-ledger-request", TypeLayersReq: "layers-request", TypeWeightReq: "weight-request",
	TypeExtension: "extension", TypeNothingHeavier: "nothing-heavier", TypeNotFromHere: "heavier-not-from-here",
	TypeHashChain: "hash-chain", TypeAnchorNotHeld: "anchor-not-held", TypeEntries: "entries", TypeLayers: "layers",
	TypeWeight: "weight", TypeNotHeld: "not-held", TypeRefused: "refused",
	TypePartsReq: "parts-request", TypeLarge: "large", TypePart: "part",
	TypeLayersFromReq: "layers-from-request", TypeManyLayers: "many-layers", TypeLayersFrom: "layers-from",
}

func (t Type) String() string {
	if n, ok := typeNames[t]; ok {
		return n
	}
	return fmt.Sprintf("type(%d)", uint16(t))
}

// IsRequest reports whether t is a ledger data request.
func (t Type) IsRequest() bool { return t >= TypeExtensionReq && t <= TypeLayersFromReq }

// IsReply reports whether t is a reply to a ledger data request.
func (t Type) IsReply() bool { return t >= TypeExtension && t <= TypeLayersFrom }

// Header flags.
const (
	FlagLast uint8 = 1 << 0
	FlagMore uint8 = 1 << 1
)

// Header is the part of every message before the body.
type Header struct {
	Type      Type
	Section   Section
	RequestID hash.Hash
	Chunk     uint32
	Flags     uint8
	Stated    weight.Weight
	Hops      uint16
}

// Last reports whether this is the last chunk of a reply.
func (h Header) Last() bool { return h.Flags&FlagLast != 0 }

// More reports whether the sender holds more than the asker's limits let
// it send.
func (h Header) More() bool { return h.Flags&FlagMore != 0 }

// Message is a header and its raw body.
type Message struct {
	Header
	Body []byte
}

// Encode returns the message bytes (without the frame length).
func (m Message) Encode() []byte {
	w := encoding.NewWriter(96 + len(m.Body))
	w.U8(Version).U16(uint16(m.Type))
	m.Section.encode(w)
	w.Raw(m.RequestID[:]).U32(m.Chunk).U8(m.Flags)
	w.VarBytes(m.Stated.Bytes()).U16(m.Hops)
	w.Raw(m.Body)
	return w.Bytes()
}

// Decode reads a message. The body is not checked here; Decode* functions
// for each type do that.
func Decode(b []byte) (Message, error) {
	r := encoding.NewReader(b)
	var m Message
	if v := r.U8(); r.Err() == nil && v != Version {
		return m, fmt.Errorf("%w: wire version %d", ErrMalformed, v)
	}
	m.Type = Type(r.U16())
	m.Section = decodeSection(r)
	r.Into(m.RequestID[:])
	m.Chunk = r.U32()
	m.Flags = r.U8()
	sw := r.VarBytes(64)
	m.Hops = r.U16()
	if err := r.Err(); err != nil {
		return m, fmt.Errorf("%w: header: %v", ErrMalformed, err)
	}
	if m.Flags&^(FlagLast|FlagMore) != 0 {
		return m, fmt.Errorf("%w: unknown flags %#x", ErrMalformed, m.Flags)
	}
	w, err := weight.FromBytes(sw)
	if err != nil {
		return m, fmt.Errorf("%w: stated weight: %v", ErrMalformed, err)
	}
	m.Stated = w
	m.Body = b[len(b)-r.Remaining():]
	return m, nil
}

// WriteFrame writes one framed message.
func WriteFrame(w io.Writer, m Message) error {
	b := m.Encode()
	if len(b) > MaxFrame {
		return fmt.Errorf("wire: message of %d bytes exceeds the frame limit", len(b))
	}
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	if _, err := w.Write(n[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

// ReadFrame reads one framed message, refusing frames above max (at most
// MaxFrame).
func ReadFrame(r *bufio.Reader, max int) (Message, int, error) {
	if max <= 0 || max > MaxFrame {
		max = MaxFrame
	}
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return Message{}, 0, err
	}
	size := int(binary.BigEndian.Uint32(n[:]))
	if size > max {
		return Message{}, 0, fmt.Errorf("%w: frame of %d bytes exceeds %d", ErrMalformed, size, max)
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return Message{}, 0, err
	}
	m, err := Decode(b)
	return m, 4 + size, err
}

// Section is the mint policy section: issuer, series id, pool id and mint
// serial, where Depth says how many of them are set, from the left. The
// rest are wildcards. A series that names no issuer has the zero hash as
// its issuer.
//
// Byte form: U8 depth, then each set field (hashes raw, the serial U64).
type Section struct {
	Depth  uint8
	Issuer hash.Hash
	Series hash.Hash
	Pool   hash.Hash
	Serial uint64
}

// All is the section of everything.
var All = Section{}

// IssuerSection, SeriesSection, PoolSection and MintSection build sections.
func IssuerSection(issuer hash.Hash) Section { return Section{Depth: 1, Issuer: issuer} }

func SeriesSection(issuer, series hash.Hash) Section {
	return Section{Depth: 2, Issuer: issuer, Series: series}
}

func PoolSection(issuer, series, pool hash.Hash) Section {
	return Section{Depth: 3, Issuer: issuer, Series: series, Pool: pool}
}

func MintSection(issuer, series, pool hash.Hash, serial uint64) Section {
	return Section{Depth: 4, Issuer: issuer, Series: series, Pool: pool, Serial: serial}
}

func (s Section) encode(w *encoding.Writer) {
	w.U8(s.Depth)
	if s.Depth >= 1 {
		w.Raw(s.Issuer[:])
	}
	if s.Depth >= 2 {
		w.Raw(s.Series[:])
	}
	if s.Depth >= 3 {
		w.Raw(s.Pool[:])
	}
	if s.Depth >= 4 {
		w.U64(s.Serial)
	}
}

func decodeSection(r *encoding.Reader) Section {
	var s Section
	s.Depth = r.U8()
	if s.Depth > 4 {
		r.Fail(fmt.Errorf("section depth %d", s.Depth))
		return s
	}
	if s.Depth >= 1 {
		r.Into(s.Issuer[:])
	}
	if s.Depth >= 2 {
		r.Into(s.Series[:])
	}
	if s.Depth >= 3 {
		r.Into(s.Pool[:])
	}
	if s.Depth >= 4 {
		s.Serial = r.U64()
	}
	return s
}

// Coverage is how much of a section a followed mint policy covers.
type Coverage int

const (
	None Coverage = iota
	Partial
	Full
)

// fieldsEqual reports whether a and b agree on level i (1-based).
func fieldsEqual(a, b Section, i int) bool {
	switch i {
	case 1:
		return a.Issuer == b.Issuer
	case 2:
		return a.Series == b.Series
	case 3:
		return a.Pool == b.Pool
	default:
		return a.Serial == b.Serial
	}
}

// Covers says how much of msg the followed section f covers. Full: at every
// level f's field equals msg's or is a wildcard. Partial: at every level
// the two are equal or either is a wildcard, but not full. None otherwise.
func (f Section) Covers(msg Section) Coverage {
	full := true
	for i := 1; i <= 4; i++ {
		fw, mw := int(f.Depth) < i, int(msg.Depth) < i
		switch {
		case fw:
			// f is a wildcard here: covers anything.
		case mw:
			full = false // msg asks about more than f follows
		case !fieldsEqual(f, msg, i):
			return None
		}
	}
	if full {
		return Full
	}
	return Partial
}

// Contains reports whether s includes t entirely: s covers t fully.
func (s Section) Contains(t Section) bool { return s.Covers(t) == Full }

func (s Section) String() string {
	f := []string{"*", "*", "*", "*"}
	if s.Depth >= 1 {
		f[0] = s.Issuer.Short()
	}
	if s.Depth >= 2 {
		f[1] = s.Series.Short()
	}
	if s.Depth >= 3 {
		f[2] = s.Pool.Short()
	}
	if s.Depth >= 4 {
		f[3] = fmt.Sprint(s.Serial)
	}
	return f[0] + "/" + f[1] + "/" + f[2] + "/" + f[3]
}

// tagRequest prefixes the hash that names a request.
var tagRequest = []byte("PoO-request-v1")

// RequestID returns the id of a request: SHA-256 of a tag, the type, the
// section and the body. The body holds a nonce and a time, so no two
// requests share an id. The hop count is left out; it changes at each
// relay. Wire messages always use SHA-256, whatever the series' hash.
func RequestID(t Type, s Section, body []byte) hash.Hash {
	w := encoding.NewWriter(64 + len(body))
	w.Raw(tagRequest).U16(uint16(t))
	s.encode(w)
	w.Raw(body)
	return sha256.Sum256(w.Bytes())
}
