// Package series implements the published set of rules a group of mints
// follows: the hash, the number of mints, the supply of each mint, the pool
// size, the strength minimums, entry hash credit, the allowed policy types
// and key algorithms, the witness ceiling, and an optional issuer.
//
// A series is identified by series_id = H("PoO-series" ∥ encode(params)).
// Its byte form starts with the format version and the hash number, so a
// reader always knows which hash computes the id. Changing any rule gives a
// new series_id and so a new series with new mints.
//
// The package also derives the pools of a series: the pool of serial s is
// s / P, and pool k starts its ledger at pool_id = H("PoO-pool" ∥ series_id
// ∥ k), with k written as 8 bytes.
package series

import (
	"errors"
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// FormatVersion is the only series byte-form version this implementation
// reads. A series carries a format version; consensus itself has none.
const FormatVersion = 1

// MaxStrength is the largest strength a 32-byte hash can have.
const MaxStrength = hash.Size * 8

var (
	// ErrInvalid marks a series whose settings break the rules every series
	// must obey, such as a pool size larger than the number of mints.
	ErrInvalid = errors.New("series: invalid")
	// ErrUnreadable marks a valid series that names a number this node does
	// not implement. The node cannot verify its transactions, so it does not
	// select its entries or count its layers.
	ErrUnreadable = errors.New("series: unreadable at this node")
)

// Params are the settings a series fixes. They are plain data so they can
// be published as JSON and read by people; the id is computed from their
// canonical byte form.
type Params struct {
	// HashID names the hash used for everything in the series.
	HashID hash.ID `json:"hash"`
	// Mints is N; serials run from 0 to N-1.
	Mints uint64 `json:"mints"`
	// Supply is S, the units in every mint.
	Supply uint64 `json:"supply"`
	// PoolSize is P, the number of consecutive mints sharing one ledger.
	PoolSize uint64 `json:"pool_size"`
	// MinLayerStrength: layers below it are ignored.
	MinLayerStrength uint16 `json:"min_layer_strength"`
	// MinEntryStrength: entries whose hash is below it are invalid. Zero
	// means none, and then entries carry no nonce.
	MinEntryStrength uint16 `json:"min_entry_strength"`
	// EntryHashCredit: whether an entry's own hash adds its credit.
	EntryHashCredit bool `json:"entry_hash_credit"`
	// PolicyTypes are the allowed address policy types, sorted, no repeats.
	PolicyTypes []policy.TypeID `json:"policy_types"`
	// KeyAlgs are the allowed key algorithms, sorted, no repeats.
	KeyAlgs []policy.AlgID `json:"key_algs"`
	// MaxWitness is the largest witness in bytes, or 0 for not set.
	MaxWitness uint32 `json:"max_witness"`
	// Issuer is the optional address of the publisher, whose witness may sit
	// beside the series. Selection never reads it.
	Issuer *hash.Hash `json:"issuer,omitempty"`
}

// Encode returns the canonical byte form of the params.
func (p Params) Encode() []byte {
	w := encoding.NewWriter(128)
	w.U8(FormatVersion).U32(uint32(p.HashID))
	w.U64(p.Mints).U64(p.Supply).U64(p.PoolSize)
	w.U16(p.MinLayerStrength).U16(p.MinEntryStrength).Bool(p.EntryHashCredit)
	w.Count(len(p.PolicyTypes))
	for _, t := range p.PolicyTypes {
		w.U32(uint32(t))
	}
	w.Count(len(p.KeyAlgs))
	for _, a := range p.KeyAlgs {
		w.U32(uint32(a))
	}
	w.U32(p.MaxWitness)
	w.Bool(p.Issuer != nil)
	if p.Issuer != nil {
		w.Raw(p.Issuer[:])
	}
	return w.Bytes()
}

// DecodeParams reads the canonical byte form.
func DecodeParams(b []byte) (Params, error) {
	r := encoding.NewReader(b)
	var p Params
	if v := r.U8(); r.Err() == nil && v != FormatVersion {
		return p, fmt.Errorf("%w: format version %d", ErrUnreadable, v)
	}
	p.HashID = hash.ID(r.U32())
	p.Mints, p.Supply, p.PoolSize = r.U64(), r.U64(), r.U64()
	p.MinLayerStrength, p.MinEntryStrength = r.U16(), r.U16()
	p.EntryHashCredit = r.Bool()
	n := r.Count(1024, 4)
	for i := 0; i < n; i++ {
		p.PolicyTypes = append(p.PolicyTypes, policy.TypeID(r.U32()))
	}
	n = r.Count(1024, 4)
	for i := 0; i < n; i++ {
		p.KeyAlgs = append(p.KeyAlgs, policy.AlgID(r.U32()))
	}
	p.MaxWitness = r.U32()
	if r.Bool() {
		var iss hash.Hash
		r.Into(iss[:])
		p.Issuer = &iss
	}
	if err := r.Finish(); err != nil {
		return p, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return p, nil
}

// Series is a validated, readable series with its id and hash function.
type Series struct {
	params Params
	id     hash.Hash
	h      hash.Func
	types  map[policy.TypeID]bool
	algs   map[policy.AlgID]bool
	reg    *policy.Registry
	// verified remembers witnesses already satisfied, a local cache that
	// never changes a result (see policy.Verified).
	verified *policy.Verified
}

// New validates params against the default policy registry and returns the
// series. It returns an error wrapping ErrInvalid or ErrUnreadable.
func New(p Params) (*Series, error) {
	return NewWithRegistry(p, policy.Default())
}

// NewWithRegistry is New with a chosen policy registry.
func NewWithRegistry(p Params, reg *policy.Registry) (*Series, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	h, err := hash.Lookup(p.HashID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	s := &Series{params: p, h: h, reg: reg, verified: policy.NewVerified(0), types: map[policy.TypeID]bool{}, algs: map[policy.AlgID]bool{}}
	for _, t := range p.PolicyTypes {
		if !reg.HasType(t) {
			return nil, fmt.Errorf("%w: policy type %d", ErrUnreadable, t)
		}
		s.types[t] = true
	}
	for _, a := range p.KeyAlgs {
		if !reg.HasAlg(a) {
			return nil, fmt.Errorf("%w: key algorithm %d", ErrUnreadable, a)
		}
		s.algs[a] = true
	}
	s.id = h(hash.TagSeries, p.Encode())
	return s, nil
}

func (p Params) validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	switch {
	case p.Mints == 0:
		return bad("number of mints must be at least 1")
	case p.Supply == 0:
		return bad("supply must be at least 1")
	case p.PoolSize == 0 || p.PoolSize > p.Mints:
		return bad("pool size must be from 1 to the number of mints")
	case p.MinLayerStrength > MaxStrength || p.MinEntryStrength > MaxStrength:
		return bad("strength minimums cannot exceed %d", MaxStrength)
	case len(p.PolicyTypes) == 0:
		return bad("at least one policy type must be allowed")
	case len(p.KeyAlgs) == 0:
		return bad("at least one key algorithm must be allowed")
	}
	if !sort.SliceIsSorted(p.PolicyTypes, func(i, j int) bool { return p.PolicyTypes[i] < p.PolicyTypes[j] }) || hasRepeat(p.PolicyTypes) {
		return bad("policy types must be sorted with no repeats")
	}
	if !sort.SliceIsSorted(p.KeyAlgs, func(i, j int) bool { return p.KeyAlgs[i] < p.KeyAlgs[j] }) || hasRepeat(p.KeyAlgs) {
		return bad("key algorithms must be sorted with no repeats")
	}
	return nil
}

func hasRepeat[T comparable](xs []T) bool {
	for i := 1; i < len(xs); i++ {
		if xs[i] == xs[i-1] {
			return true
		}
	}
	return false
}

// ID returns the series_id.
func (s *Series) ID() hash.Hash { return s.id }

// Params returns a copy of the settings.
func (s *Series) Params() Params { return s.params }

// Hash returns the series' hash function.
func (s *Series) Hash() hash.Func { return s.h }

// Registry returns the policy registry the series was checked against.
func (s *Series) Registry() *policy.Registry { return s.reg }

// Mints returns N.
func (s *Series) Mints() uint64 { return s.params.Mints }

// Supply returns S.
func (s *Series) Supply() uint64 { return s.params.Supply }

// PoolSize returns P.
func (s *Series) PoolSize() uint64 { return s.params.PoolSize }

// MinLayerStrength returns the minimum layer strength.
func (s *Series) MinLayerStrength() int { return int(s.params.MinLayerStrength) }

// MinEntryStrength returns the minimum entry strength, 0 for none.
func (s *Series) MinEntryStrength() int { return int(s.params.MinEntryStrength) }

// EntriesHaveNonce reports whether entries carry a nonce, which they do
// exactly when the series sets a minimum entry strength.
func (s *Series) EntriesHaveNonce() bool { return s.params.MinEntryStrength > 0 }

// EntryHashCredit reports whether an entry's own hash adds credit.
func (s *Series) EntryHashCredit() bool { return s.params.EntryHashCredit }

// AllowsPolicyType implements policy.Rules.
func (s *Series) AllowsPolicyType(t policy.TypeID) bool { return s.types[t] }

// AllowsKeyAlg implements policy.Rules.
func (s *Series) AllowsKeyAlg(a policy.AlgID) bool { return s.algs[a] }

// MaxWitnessSize implements policy.Rules.
func (s *Series) MaxWitnessSize() int { return int(s.params.MaxWitness) }

// CheckWitness checks a witness against an address for a tid under this
// series' hash and rules.
func (s *Series) CheckWitness(w policy.Witness, addr, tid hash.Hash) error {
	return s.reg.CheckCached(s.h, s, w, addr, tid, s.verified)
}

// Preverify verifies the signatures of a witness for tid ahead of a full
// check and remembers a pass, so the full check, which may run under a
// lock, does not verify them again. It never changes what CheckWitness
// returns.
func (s *Series) Preverify(w policy.Witness, tid hash.Hash) {
	s.reg.Preverify(s, w, tid, s.verified)
}

// Remembered returns how many satisfied witnesses the series remembers
// (ORANGE-002), for tests and metrics.
func (s *Series) Remembered() int { return s.verified.Len() }

// Address returns the address of a policy under this series' hash.
func (s *Series) Address(p policy.Policy) hash.Hash { return policy.Address(s.h, p) }

// PoolCount returns the number of pools, N / P rounded up.
func (s *Series) PoolCount() uint64 {
	return (s.params.Mints + s.params.PoolSize - 1) / s.params.PoolSize
}

// PoolOf returns the pool index of a serial. The serial must be below N.
func (s *Series) PoolOf(serial uint64) uint64 { return serial / s.params.PoolSize }

// PoolRange returns the first and last serial of pool k.
func (s *Series) PoolRange(k uint64) (first, last uint64) {
	first = k * s.params.PoolSize
	last = first + s.params.PoolSize - 1
	if last >= s.params.Mints {
		last = s.params.Mints - 1
	}
	return first, last
}

// PoolID returns H("PoO-pool" ∥ series_id ∥ k), the parent of the first
// entry of pool k.
func (s *Series) PoolID(k uint64) hash.Hash {
	w := encoding.NewWriter(8).U64(k)
	return s.h(hash.TagPool, s.id[:], w.Bytes())
}

// Mint returns the id of the mint with the given serial.
func (s *Series) Mint(serial uint64) MintID { return MintID{Series: s.id, Serial: serial} }

// IssuerMessageTID returns H("PoO-issuer-v1" ∥ series_id), the value an
// issuer witness signs in place of a tid. The tag keeps it from ever equal
// to a real tid, so an issuer signature can never authorize a transaction.
func (s *Series) IssuerMessageTID() hash.Hash {
	return s.h(hash.TagIssuer, s.id[:])
}

// CheckIssuer verifies an issuer witness. The issuer witness sits beside the
// series, not inside the series_id, so signing never changes the id. It
// proves only who published the series; a node may use it when deciding whom
// to follow.
func (s *Series) CheckIssuer(w policy.Witness) error {
	if s.params.Issuer == nil {
		return fmt.Errorf("%w: series names no issuer", ErrInvalid)
	}
	return s.reg.Check(s.h, s, w, *s.params.Issuer, s.IssuerMessageTID())
}

// MintID names one mint: the series_id and a serial from 0 to N-1.
type MintID struct {
	Series hash.Hash `json:"series"`
	Serial uint64    `json:"serial"`
}

// String returns "<series short>/<serial>".
func (m MintID) String() string { return fmt.Sprintf("%s/%d", m.Series.Short(), m.Serial) }
