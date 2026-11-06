package wire

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// Peer discovery and management messages (the design's "Peer Discovery and
// Management" document). Numbers are never reused.
const (
	TypePing          Type = 2
	TypeDroppedPolicy Type = 3

	TypeFollowReq        Type = 30
	TypePeerListReq      Type = 31
	TypeMissingChunksReq Type = 32

	TypePong        Type = 60
	TypePartialPong Type = 61
	TypeFollowYes   Type = 62
	TypePartialYes  Type = 63
	TypePeerList    Type = 64
)

func init() {
	for t, n := range map[Type]string{
		TypePing: "ping", TypeDroppedPolicy: "dropped-policy",
		TypeFollowReq: "follow-request", TypePeerListReq: "peer-list-request", TypeMissingChunksReq: "missing-chunks-request",
		TypePong: "pong", TypePartialPong: "partial-pong", TypeFollowYes: "yes", TypePartialYes: "partial-yes",
		TypePeerList: "peer-list",
	} {
		typeNames[t] = n
	}
}

// IsDiscovery reports whether t is a peer discovery message.
func (t Type) IsDiscovery() bool {
	return t == TypePing || t == TypeDroppedPolicy || (t >= TypeFollowReq && t <= TypeMissingChunksReq) ||
		(t >= TypePong && t <= TypePeerList)
}

// IsDiscoveryReply reports whether t answers a follow, peer list or ping.
func (t Type) IsDiscoveryReply() bool { return t >= TypePong && t <= TypePeerList }

// maxAddr bounds a contact address.
const maxAddr = 300

// Contact is where a node accepts connections, or nothing when it is not
// reachable. Byte form: Bool reachable ∥ [Bytes "host:port"].
type Contact struct {
	Addr string
}

// Reachable reports whether the contact names an address.
func (c Contact) Reachable() bool { return c.Addr != "" }

func (c Contact) encode(w *encoding.Writer) {
	w.Bool(c.Reachable())
	if c.Reachable() {
		w.VarBytes([]byte(c.Addr))
	}
}

func decodeContact(r *encoding.Reader) Contact {
	var c Contact
	if r.Bool() {
		c.Addr = string(r.VarBytes(maxAddr))
		if err := CheckAddr(c.Addr); err != nil {
			r.Fail(err)
		}
	}
	return c
}

// CheckAddr checks that a contact address is host:port with a port number.
func CheckAddr(a string) error {
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return err
	}
	if host == "" {
		return fmt.Errorf("address %q has no host", a)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("address %q has a bad port", a)
	}
	return nil
}

// DiscoveryLimits are the asker's reply limits on a follow or peer list
// request; 0 means none set (the replier applies its own cap), except the
// peer list limit, where 0 asks for no peers.
type DiscoveryLimits struct {
	ChunkSize     uint32
	ListLimit     uint32
	PeerListLimit uint32
}

func (l DiscoveryLimits) encode(w *encoding.Writer) {
	w.U32(l.ChunkSize).U32(l.ListLimit).U32(l.PeerListLimit)
}

func decodeDiscoveryLimits(r *encoding.Reader) DiscoveryLimits {
	return DiscoveryLimits{r.U32(), r.U32(), r.U32()}
}

// PeerVariant says which peers a follow or peer list reply adds.
type PeerVariant uint8

const (
	NoPeers   PeerVariant = 0 // follow request only: nothing beyond the yes
	FullPeers PeerVariant = 1 // peers that follow all of the mint policy (or the part the replier follows)
	AllPeers  PeerVariant = 2 // peers that follow all or part of it, each with its mint policies
)

func decodePeerVariant(r *encoding.Reader) PeerVariant {
	v := PeerVariant(r.U8())
	if v > AllPeers {
		r.Fail(fmt.Errorf("peer variant %d", v))
	}
	return v
}

// FollowReq asks "do you follow this mint policy?" (the header's section).
// Broadcast marks a request that followers may relay; a direct request is
// answered only by the node it was sent to. It is part of the body, so of
// the request id, and a relay cannot change it.
type FollowReq struct {
	Nonce     uint64
	Time      uint64
	Variant   PeerVariant
	Broadcast bool
	Contact   Contact
	Limits    DiscoveryLimits
}

// Encode returns the body.
func (q FollowReq) Encode() []byte {
	w := encoding.NewWriter(64)
	w.U64(q.Nonce).U64(q.Time).U8(uint8(q.Variant)).Bool(q.Broadcast)
	q.Contact.encode(w)
	q.Limits.encode(w)
	return w.Bytes()
}

// DecodeFollowReq reads a follow request body.
func DecodeFollowReq(b []byte) (FollowReq, error) {
	r := encoding.NewReader(b)
	q := FollowReq{Nonce: r.U64(), Time: r.U64()}
	q.Variant = decodePeerVariant(r)
	q.Broadcast = r.Bool()
	q.Contact = decodeContact(r)
	q.Limits = decodeDiscoveryLimits(r)
	return q, finish(r, "follow request")
}

// PeerListReq asks a node directly for its peers for the section.
type PeerListReq struct {
	Nonce   uint64
	Time    uint64
	Variant PeerVariant // FullPeers or AllPeers
	Limits  DiscoveryLimits
}

// Encode returns the body.
func (q PeerListReq) Encode() []byte {
	w := encoding.NewWriter(48)
	w.U64(q.Nonce).U64(q.Time).U8(uint8(q.Variant))
	q.Limits.encode(w)
	return w.Bytes()
}

// DecodePeerListReq reads a peer list request body.
func DecodePeerListReq(b []byte) (PeerListReq, error) {
	r := encoding.NewReader(b)
	q := PeerListReq{Nonce: r.U64(), Time: r.U64()}
	q.Variant = decodePeerVariant(r)
	if q.Variant == NoPeers {
		r.Fail(fmt.Errorf("a peer list request must ask for peers"))
	}
	q.Limits = decodeDiscoveryLimits(r)
	return q, finish(r, "peer list request")
}

// Ping checks that a peer runs and still follows the section.
type Ping struct {
	Nonce uint64
	Time  uint64
	// Ext holds extension fields (D186).
	Ext []Ext
}

// Encode returns the body.
func (p Ping) Encode() []byte {
	w := encoding.NewWriter(17).U64(p.Nonce).U64(p.Time)
	encodeExt(w, p.Ext)
	return w.Bytes()
}

// DecodePing reads a ping body.
func DecodePing(b []byte) (Ping, error) {
	r := encoding.NewReader(b)
	p := Ping{Nonce: r.U64(), Time: r.U64()}
	bad := extras(r, func() { p.Ext = decodeExt(r, MaxSmallExt) })
	return p, finishExtras(r, "ping", bad)
}

// MissingChunksReq asks again for chunks of a reply lost when a connection
// broke. Its header carries the original request id.
type MissingChunksReq struct {
	Positions []uint32
}

// Encode returns the body.
func (q MissingChunksReq) Encode() []byte {
	w := encoding.NewWriter(8 + 4*len(q.Positions))
	w.Count(len(q.Positions))
	for _, p := range q.Positions {
		w.U32(p)
	}
	return w.Bytes()
}

// DecodeMissingChunksReq reads a missing chunks request body.
func DecodeMissingChunksReq(b []byte) (MissingChunksReq, error) {
	r := encoding.NewReader(b)
	n := r.Count(maxItems, 4)
	var q MissingChunksReq
	for i := 0; i < n && r.Err() == nil; i++ {
		p := r.U32()
		if p == 0 {
			r.Fail(fmt.Errorf("chunk position 0"))
		}
		q.Positions = append(q.Positions, p)
	}
	return q, finish(r, "missing chunks request")
}

// PeerInfo is one listed peer: its contact details and, for the all peers
// variant, the mint policies it follows within the one asked about.
type PeerInfo struct {
	Contact  Contact
	Policies []Section
}

func encodeSections(w *encoding.Writer, ss []Section) {
	w.Count(len(ss))
	for _, s := range ss {
		s.encode(w)
	}
}

func decodeSections(r *encoding.Reader) []Section {
	n := r.Count(maxCoverage, 1)
	var out []Section
	for i := 0; i < n && r.Err() == nil; i++ {
		out = append(out, decodeSection(r))
	}
	return out
}

func encodePeers(w *encoding.Writer, ps []PeerInfo) {
	w.Count(len(ps))
	for _, p := range ps {
		p.Contact.encode(w)
		encodeSections(w, p.Policies)
	}
}

func decodePeers(r *encoding.Reader) []PeerInfo {
	n := r.Count(maxItems, 5)
	var out []PeerInfo
	for i := 0; i < n && r.Err() == nil; i++ {
		p := PeerInfo{Contact: decodeContact(r), Policies: decodeSections(r)}
		if !p.Contact.Reachable() {
			r.Fail(fmt.Errorf("a listed peer has no contact details"))
		}
		out = append(out, p)
	}
	return out
}

// FollowReply is the body of a yes or partial yes: the replier's contact
// details, for a partial yes the mint policies it follows within the one
// asked about (sorted), a marker that the list was cut short, and the
// peers the variant asks for. A relayed reply never carries peers.
type FollowReply struct {
	Contact  Contact
	Policies []Section
	More     bool
	Peers    []PeerInfo
}

// Encode returns the body.
func (f FollowReply) Encode() []byte {
	w := encoding.NewWriter(128)
	f.Contact.encode(w)
	encodeSections(w, f.Policies)
	w.Bool(f.More)
	encodePeers(w, f.Peers)
	return w.Bytes()
}

// DecodeFollowReply reads a yes or partial yes body.
func DecodeFollowReply(b []byte) (FollowReply, error) {
	r := encoding.NewReader(b)
	f := FollowReply{Contact: decodeContact(r), Policies: decodeSections(r), More: r.Bool(), Peers: decodePeers(r)}
	return f, finish(r, "follow reply")
}

// PongReply is the body of a pong or partial pong.
type PongReply struct {
	Contact  Contact
	Policies []Section // partial pong only
	// Ext holds extension fields (D186).
	Ext []Ext
}

// Encode returns the body.
func (p PongReply) Encode() []byte {
	w := encoding.NewWriter(64)
	p.Contact.encode(w)
	encodeSections(w, p.Policies)
	encodeExt(w, p.Ext)
	return w.Bytes()
}

// DecodePongReply reads a pong body.
func DecodePongReply(b []byte) (PongReply, error) {
	r := encoding.NewReader(b)
	p := PongReply{Contact: decodeContact(r), Policies: decodeSections(r)}
	bad := extras(r, func() { p.Ext = decodeExt(r, MaxSmallExt) })
	return p, finishExtras(r, "pong", bad)
}

// PeerListChunk is one chunk of a peer list reply.
type PeerListChunk struct {
	Peers []PeerInfo
}

// Encode returns the body.
func (c PeerListChunk) Encode() []byte {
	w := encoding.NewWriter(128)
	encodePeers(w, c.Peers)
	return w.Bytes()
}

// DecodePeerListChunk reads a peer list chunk.
func DecodePeerListChunk(b []byte) (PeerListChunk, error) {
	r := encoding.NewReader(b)
	c := PeerListChunk{Peers: decodePeers(r)}
	return c, finish(r, "peer list")
}

// SortSections sorts mint policies by issuer, series id, pool id and
// serial, lowest first, with a wildcard before any value.
func SortSections(ss []Section) {
	sort.Slice(ss, func(i, j int) bool { return LessSection(ss[i], ss[j]) })
}

// LessSection orders sections as SortSections does.
func LessSection(a, b Section) bool {
	for i := 1; i <= 4; i++ {
		aw, bw := int(a.Depth) < i, int(b.Depth) < i
		switch {
		case aw && bw:
			return false
		case aw:
			return true
		case bw:
			return false
		}
		if fieldsEqual(a, b, i) {
			continue
		}
		switch i {
		case 1:
			return a.Issuer.Less(b.Issuer)
		case 2:
			return a.Series.Less(b.Series)
		case 3:
			return a.Pool.Less(b.Pool)
		default:
			return a.Serial < b.Serial
		}
	}
	return false
}

// Text is the full text form of a section, as registries and the node
// policy write it: "issuer/series/pool/serial" with each hash in full hex
// and * for a wildcard, for example "<issuer hex>/<series hex>/*/*".
func (s Section) Text() string {
	f := []string{"*", "*", "*", "*"}
	if s.Depth >= 1 {
		f[0] = s.Issuer.String()
	}
	if s.Depth >= 2 {
		f[1] = s.Series.String()
	}
	if s.Depth >= 3 {
		f[2] = s.Pool.String()
	}
	if s.Depth >= 4 {
		f[3] = strconv.FormatUint(s.Serial, 10)
	}
	return strings.Join(f, "/")
}

// ParseSection reads the text form written by Text.
func ParseSection(t string) (Section, error) {
	f := strings.Split(t, "/")
	if len(f) != 4 {
		return Section{}, fmt.Errorf("section %q: want four fields", t)
	}
	var s Section
	for i, v := range f {
		if v == "*" {
			for _, rest := range f[i+1:] {
				if rest != "*" {
					return Section{}, fmt.Errorf("section %q: a field after * is set", t)
				}
			}
			break
		}
		s.Depth = uint8(i + 1)
		var err error
		switch i {
		case 0:
			s.Issuer, err = hash.Parse(v)
		case 1:
			s.Series, err = hash.Parse(v)
		case 2:
			s.Pool, err = hash.Parse(v)
		case 3:
			s.Serial, err = strconv.ParseUint(v, 10, 64)
		}
		if err != nil {
			return Section{}, fmt.Errorf("section %q: %v", t, err)
		}
	}
	return s, nil
}
