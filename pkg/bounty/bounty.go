// Package bounty is the bounty market (D196): a node posts an offer to pay
// for a layer of a chosen strength on one of its entries; a peer leases
// the offer, grinds the layer and claims it with an address to pay; the
// payer checks the layer and pays with an ordinary payment.
//
// Nothing here is part of the ledger. An offer, lease, claim or ack is
// never inside an entry, layer or transaction, so no setting of this
// package can make any ledger valid or invalid. The messages travel only
// between peers whose hellos list the "bounty" feature.
//
// The package holds the message forms and the book of offers; it does no
// networking. pkg/p2p carries the messages and pkg/rpc the commands.
package bounty

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// Kinds of work an offer pays for. Only layers exist; entries may follow,
// and a node ignores an offer of a kind it does not know.
const (
	KindLayer uint8 = 1
)

// ErrMalformed is a message that does not decode.
var ErrMalformed = errors.New("bounty: malformed")

// Bounds on what one message may carry.
const (
	// MaxReason bounds the text of a grant or ack.
	MaxReason = 200
	// MaxListed bounds the offers in one list reply.
	MaxListed = 64
	// MaxOfferBytes bounds one offer's bytes, alone or in a list: room
	// for many later fields, while a held offer stays small (RED-115).
	MaxOfferBytes = 1024
	// MaxTIDs bounds the payment ids in one ack: one per leg.
	MaxTIDs = 16
)

// Offer asks for Count layers of at least Strength bits on entry Entry of
// pool Pool of series Series, paying Amount units of that series for each.
type Offer struct {
	Kind     uint8     `json:"kind"`
	Series   hash.Hash `json:"series"`
	Pool     uint64    `json:"pool"`
	Entry    hash.Hash `json:"entry"`
	Strength uint8     `json:"strength"`
	Amount   uint64    `json:"amount"`
	Count    uint16    `json:"count"`
	// OpenSeconds is how long the offer stays open after it is posted; it
	// is advisory: a node drops an offer it holds once this passes.
	OpenSeconds uint32 `json:"open_seconds"`
	// Salt makes two otherwise equal offers different offers.
	Salt uint64 `json:"salt"`
	// Raw is the offer's bytes as received or posted, with any fields a
	// later release added after Salt. The id is the hash of these bytes,
	// and they are what is passed on, so nodes of every release name an
	// offer alike (W-27). Empty for an offer built from its fields.
	Raw Bytes `json:"raw,omitempty"`
}

// Bytes are bytes written in JSON as hex.
type Bytes []byte

// MarshalText writes b as hex.
func (b Bytes) MarshalText() ([]byte, error) { return []byte(hex.EncodeToString(b)), nil }

// UnmarshalText reads hex.
func (b *Bytes) UnmarshalText(t []byte) error {
	v, err := hex.DecodeString(string(t))
	if err != nil {
		return err
	}
	*b = v
	return nil
}

// Encode returns the offer's bytes: Raw when it is set, else its fields.
func (o Offer) Encode() []byte {
	if len(o.Raw) > 0 {
		return slices.Clone([]byte(o.Raw))
	}
	w := encoding.NewWriter(96)
	o.encode(w)
	return w.Bytes()
}

func (o Offer) encode(w *encoding.Writer) {
	w.U8(o.Kind).Raw(o.Series[:]).U64(o.Pool).Raw(o.Entry[:]).U8(o.Strength)
	w.U64(o.Amount).U16(o.Count).U32(o.OpenSeconds).U64(o.Salt)
}

func decodeOffer(r *encoding.Reader) Offer {
	var o Offer
	o.Kind = r.U8()
	r.Into(o.Series[:])
	o.Pool = r.U64()
	r.Into(o.Entry[:])
	o.Strength = r.U8()
	o.Amount = r.U64()
	o.Count = r.U16()
	o.OpenSeconds = r.U32()
	o.Salt = r.U64()
	return o
}

// ID is the offer's id: SHA-256 of its bytes, as received when it was
// received (W-27).
func (o Offer) ID() hash.Hash {
	if len(o.Raw) > 0 {
		return sha256.Sum256(o.Raw)
	}
	return sha256.Sum256(o.Encode())
}

// Same reports whether o and p are the same offer: the same bytes.
func (o Offer) Same(p Offer) bool { return bytes.Equal(o.Encode(), p.Encode()) }

// Check refuses an offer no node should act on.
func (o Offer) Check() error {
	switch {
	case o.Kind != KindLayer:
		return fmt.Errorf("bounty: unknown kind %d", o.Kind)
	case o.Strength == 0:
		return errors.New("bounty: no target strength")
	case o.Amount == 0:
		return errors.New("bounty: no amount")
	case o.Count == 0:
		return errors.New("bounty: no layers wanted")
	}
	return nil
}

// DecodeOffer reads one offer. Fields a later release adds after it are
// not read, but kept in Raw, so the id and what is passed on stay those of
// the bytes received (W-27).
func DecodeOffer(b []byte) (Offer, error) {
	if len(b) > MaxOfferBytes {
		return Offer{}, fmt.Errorf("%w: offer: %d bytes, over %d", ErrMalformed, len(b), MaxOfferBytes)
	}
	r := encoding.NewReader(b)
	o := decodeOffer(r)
	if err := r.Err(); err != nil {
		return Offer{}, fmt.Errorf("%w: offer: %v", ErrMalformed, err)
	}
	o.Raw = slices.Clone(b)
	return o, nil
}

// ListReq asks a peer for its open offers, in one series (or all, when
// Series is zero).
type ListReq struct {
	Series hash.Hash
	// MaxStrength is the strongest offer the asker will work: only offers
	// asking at most this many bits are listed (jerry, D202). Every list
	// request carries it; 255 takes any, and 0 here is written as 255.
	MaxStrength uint8
}

// AnyStrength is the max strength of a grinder that takes any offer.
const AnyStrength = 255

func orAny(s uint8) uint8 {
	if s == 0 {
		return AnyStrength
	}
	return s
}

// Encode returns the request's bytes.
func (q ListReq) Encode() []byte {
	return encoding.NewWriter(33).Raw(q.Series[:]).U8(orAny(q.MaxStrength)).Bytes()
}

// DecodeListReq reads a list request.
func DecodeListReq(b []byte) (ListReq, error) {
	r := encoding.NewReader(b)
	var q ListReq
	r.Into(q.Series[:])
	q.MaxStrength = r.U8()
	if err := r.Err(); err != nil {
		return ListReq{}, fmt.Errorf("%w: list request: %v", ErrMalformed, err)
	}
	if q.MaxStrength == 0 {
		return ListReq{}, fmt.Errorf("%w: list request: max strength 0", ErrMalformed)
	}
	return q, nil
}

// EncodeList returns the bytes of a list reply.
func EncodeList(offers []Offer) []byte {
	w := encoding.NewWriter(8 + 96*len(offers))
	w.Count(len(offers))
	for _, o := range offers {
		w.VarBytes(o.Encode())
	}
	return w.Bytes()
}

// DecodeList reads a list reply of at most MaxListed offers.
func DecodeList(b []byte) ([]Offer, error) {
	r := encoding.NewReader(b)
	n := r.Count(MaxListed, 1)
	out := make([]Offer, 0, n)
	for i := 0; i < n && r.Err() == nil; i++ {
		ob := r.VarBytes(MaxOfferBytes)
		if r.Err() != nil {
			break
		}
		o, err := DecodeOffer(ob)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("%w: list: %v", ErrMalformed, err)
	}
	return out, nil
}

// Take asks the payer to lease an offer to this grinder: it states how
// many tries a second it makes and the address it wants paid. Sent again
// for an offer it holds, it asks once for more time.
type Take struct {
	Offer hash.Hash `json:"offer"`
	Rate  uint64    `json:"rate"`
	PayTo hash.Hash `json:"pay_to"`
	// MaxStrength is the strongest offer the grinder will work; the payer
	// refuses a lease on an offer above it (jerry, D202). Every take
	// carries it; 255 takes any, and 0 here is written as 255.
	MaxStrength uint8 `json:"max_strength,omitempty"`
}

// Encode returns the take's bytes.
func (t Take) Encode() []byte {
	return encoding.NewWriter(73).Raw(t.Offer[:]).U64(t.Rate).Raw(t.PayTo[:]).U8(orAny(t.MaxStrength)).Bytes()
}

// DecodeTake reads a take.
func DecodeTake(b []byte) (Take, error) {
	r := encoding.NewReader(b)
	var t Take
	r.Into(t.Offer[:])
	t.Rate = r.U64()
	r.Into(t.PayTo[:])
	t.MaxStrength = r.U8()
	if err := r.Err(); err != nil {
		return Take{}, fmt.Errorf("%w: take: %v", ErrMalformed, err)
	}
	if t.MaxStrength == 0 {
		return Take{}, fmt.Errorf("%w: take: max strength 0", ErrMalformed)
	}
	return t, nil
}

// Grant answers a take: a lease of Seconds, or 0 and the reason it was
// refused.
type Grant struct {
	Offer   hash.Hash `json:"offer"`
	Seconds uint32    `json:"seconds"`
	Reason  string    `json:"reason,omitempty"`
}

// Encode returns the grant's bytes.
func (g Grant) Encode() []byte {
	return encoding.NewWriter(48 + len(g.Reason)).Raw(g.Offer[:]).U32(g.Seconds).VarBytes([]byte(g.Reason)).Bytes()
}

// DecodeGrant reads a grant.
func DecodeGrant(b []byte) (Grant, error) {
	r := encoding.NewReader(b)
	var g Grant
	r.Into(g.Offer[:])
	g.Seconds = r.U32()
	g.Reason = string(r.VarBytes(MaxReason))
	if err := r.Err(); err != nil {
		return Grant{}, fmt.Errorf("%w: grant: %v", ErrMalformed, err)
	}
	return g, nil
}

// Claim hands in a layer: its nonce on the offer's entry, and the address
// to pay, which must be the one the lease named.
type Claim struct {
	Offer hash.Hash `json:"offer"`
	Nonce uint64    `json:"nonce"`
	PayTo hash.Hash `json:"pay_to"`
}

// Encode returns the claim's bytes.
func (c Claim) Encode() []byte {
	return encoding.NewWriter(72).Raw(c.Offer[:]).U64(c.Nonce).Raw(c.PayTo[:]).Bytes()
}

// DecodeClaim reads a claim.
func DecodeClaim(b []byte) (Claim, error) {
	r := encoding.NewReader(b)
	var c Claim
	r.Into(c.Offer[:])
	c.Nonce = r.U64()
	r.Into(c.PayTo[:])
	if err := r.Err(); err != nil {
		return Claim{}, fmt.Errorf("%w: claim: %v", ErrMalformed, err)
	}
	return c, nil
}

// Outcomes of a claim.
const (
	Accepted uint8 = 1  // the layer is good; payment follows
	Paid     uint8 = 2  // paid: TIDs name the payment's legs
	TooWeak  uint8 = 3  // below the target, or not on the entry
	Filled   uint8 = 4  // the offer has all the layers it wanted
	Closed   uint8 = 5  // no such open offer
	Reserved uint8 = 6  // leased to another grinder
	Mismatch uint8 = 7  // the address is not the one the lease named
	Repeat   uint8 = 8  // this layer was claimed already
	Known    uint8 = 9  // the payer's node holds this layer already (RED-110)
	NoLease  uint8 = 10 // the grinder holds no lease on the offer (RED-110)
)

// OutcomeName names an outcome for output.
func OutcomeName(o uint8) string {
	switch o {
	case Accepted:
		return "accepted"
	case Paid:
		return "paid"
	case TooWeak:
		return "too weak"
	case Filled:
		return "filled"
	case Closed:
		return "closed"
	case Reserved:
		return "reserved"
	case Mismatch:
		return "address mismatch"
	case Repeat:
		return "repeat"
	case Known:
		return "known"
	case NoLease:
		return "no lease"
	}
	return fmt.Sprintf("outcome %d", o)
}

// Ack answers a claim.
type Ack struct {
	Offer   hash.Hash   `json:"offer"`
	Nonce   uint64      `json:"nonce"`
	Outcome uint8       `json:"outcome"`
	TIDs    []hash.Hash `json:"tids,omitempty"`
}

// MarshalJSON writes the ack with its outcome's name beside its number
// (GUI-GREEN-062).
func (a Ack) MarshalJSON() ([]byte, error) {
	type plain Ack
	return json.Marshal(struct {
		plain
		Name string `json:"outcome_name"`
	}{plain(a), OutcomeName(a.Outcome)})
}

// Encode returns the ack's bytes.
func (a Ack) Encode() []byte {
	w := encoding.NewWriter(48 + 32*len(a.TIDs))
	w.Raw(a.Offer[:]).U64(a.Nonce).U8(a.Outcome).Count(len(a.TIDs))
	for _, t := range a.TIDs {
		w.Raw(t[:])
	}
	return w.Bytes()
}

// DecodeAck reads an ack.
func DecodeAck(b []byte) (Ack, error) {
	r := encoding.NewReader(b)
	var a Ack
	r.Into(a.Offer[:])
	a.Nonce = r.U64()
	a.Outcome = r.U8()
	n := r.Count(MaxTIDs, hash.Size)
	for i := 0; i < n && r.Err() == nil; i++ {
		var t hash.Hash
		r.Into(t[:])
		a.TIDs = append(a.TIDs, t)
	}
	if err := r.Err(); err != nil {
		return Ack{}, fmt.Errorf("%w: ack: %v", ErrMalformed, err)
	}
	return a, nil
}
