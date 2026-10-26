// Package policy implements addresses, policies, witnesses, and the public
// number lists of policy types and key algorithms.
//
// An address is the hash of a policy:
//
//	address        = H("PoO-address" ∥ encode(policy))
//	encode(policy) = len(type) ∥ type ∥ len(params) ∥ params
//
// where type is the policy type number as 4 big-endian bytes and each len is
// a 4-byte big-endian length. A witness opens the policy behind an address
// and satisfies it for one tid; it travels with a transaction but is never
// part of the tid.
//
// Checking a witness follows the cheap-first order of the design: the size
// ceiling, then one hash to compare with the address, and only then the
// policy type's satisfaction function, which checks signatures over
// "PoO-witness-v1" ∥ tid.
package policy

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// TypeID is a number from the public number list of policy types.
type TypeID uint32

// AlgID is a number from the public number list of key algorithms.
type AlgID uint32

// Known policy types.
const (
	// TypeThreshold is "k signatures from n listed keys". A single-key
	// address is a threshold policy with k = n = 1.
	TypeThreshold TypeID = 1
)

// Known key algorithms.
const (
	// AlgEd25519 is Ed25519 (RFC 8032), 32-byte keys, 64-byte signatures.
	AlgEd25519 AlgID = 1
)

// Limits that bound decoding before any policy-specific check runs.
const (
	// MaxParamsSize bounds the params of any policy.
	MaxParamsSize = 64 * 1024
	// MaxDataSize bounds the witness data of any policy.
	MaxDataSize = 64 * 1024
)

// Errors returned by witness checks. All of them make a witness invalid;
// there is no partial credit.
var (
	ErrAddressMismatch = errors.New("policy: witness policy does not hash to the address")
	ErrTypeNotAllowed  = errors.New("policy: policy type not allowed by the series")
	ErrAlgNotAllowed   = errors.New("policy: key algorithm not allowed by the series")
	ErrUnknownType     = errors.New("policy: unknown policy type")
	ErrUnknownAlg      = errors.New("policy: unknown key algorithm")
	ErrTooLarge        = errors.New("policy: witness larger than the series allows")
	ErrUnsatisfied     = errors.New("policy: witness does not satisfy the policy")
	ErrBadParams       = errors.New("policy: malformed params")
	ErrBadData         = errors.New("policy: malformed witness data")
)

// Policy is the rule an address commits to.
type Policy struct {
	Type   TypeID
	Params []byte
}

// Encode returns encode(policy), the bytes an address hashes.
func (p Policy) Encode() []byte {
	w := encoding.NewWriter(12 + len(p.Params))
	w.U32(4).U32(uint32(p.Type))
	w.VarBytes(p.Params)
	return w.Bytes()
}

// Address returns H("PoO-address" ∥ encode(policy)).
func Address(h hash.Func, p Policy) hash.Hash {
	return h(hash.TagAddress, p.Encode())
}

// DecodePolicy reads a policy written by Policy.Encode.
func DecodePolicy(r *encoding.Reader) Policy {
	if n := r.U32(); r.Err() == nil && n != 4 {
		r.Fail(fmt.Errorf("%w: type length %d", ErrBadParams, n))
	}
	t := TypeID(r.U32())
	params := r.VarBytes(MaxParamsSize)
	return Policy{Type: t, Params: params}
}

// Witness is the proof that a portion may be spent, or, on a claim, that an
// output's address accepts the claim.
type Witness struct {
	Policy Policy
	Data   []byte
}

// Encode writes the witness: the encoded policy followed by the data as
// variable-length bytes.
func (w Witness) Encode(enc *encoding.Writer) {
	enc.Raw(w.Policy.Encode())
	enc.VarBytes(w.Data)
}

// Size returns the encoded size, the quantity a series' witness ceiling
// limits.
func (w Witness) Size() int {
	return 12 + len(w.Policy.Params) + 4 + len(w.Data)
}

// DecodeWitness reads a witness written by Witness.Encode.
func DecodeWitness(r *encoding.Reader) Witness {
	p := DecodePolicy(r)
	data := r.VarBytes(MaxDataSize)
	return Witness{Policy: p, Data: data}
}

// Message returns the bytes every signature in a witness covers:
// "PoO-witness-v1" ∥ tid.
func Message(tid hash.Hash) []byte {
	m := make([]byte, 0, len(hash.TagWitness)+hash.Size)
	m = append(m, hash.TagWitness...)
	return append(m, tid[:]...)
}

// Rules is what a series allows. The series package implements it.
type Rules interface {
	AllowsPolicyType(TypeID) bool
	AllowsKeyAlg(AlgID) bool
	// MaxWitnessSize returns the largest witness allowed, or 0 for none set.
	MaxWitnessSize() int
}

// KeyAlg is one entry of the key algorithm list.
type KeyAlg interface {
	ID() AlgID
	Name() string
	PublicKeySize() int
	SignatureSize() int
	// Verify must be pure and deterministic, and accept only the canonical
	// encoding of a signature.
	Verify(pub, msg, sig []byte) bool
}

// Type is one entry of the policy type list. Implementations must be pure,
// deterministic, context-free, bounded and order-independent, as the design
// requires: two nodes holding the same witness must reach the same verdict.
type Type interface {
	ID() TypeID
	Name() string
	// KeyAlgs parses params and returns the key algorithms they name, or an
	// error if params are malformed or name an unknown algorithm.
	KeyAlgs(reg *Registry, params []byte) ([]AlgID, error)
	// Satisfy reports whether data satisfies params for the message.
	Satisfy(reg *Registry, params, data, msg []byte) error
}

// Registry holds the policy types and key algorithms this implementation
// knows. A node can only verify spends for numbers it implements.
type Registry struct {
	types map[TypeID]Type
	algs  map[AlgID]KeyAlg
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{types: map[TypeID]Type{}, algs: map[AlgID]KeyAlg{}}
}

var defaultRegistry = func() *Registry {
	r := NewRegistry()
	r.RegisterType(thresholdType{})
	r.RegisterAlg(ed25519Alg{})
	return r
}()

// Default returns the registry with every type and algorithm this
// implementation ships. It must not be modified.
func Default() *Registry { return defaultRegistry }

// RegisterType adds a policy type. Numbers are never reused.
func (r *Registry) RegisterType(t Type) {
	if _, ok := r.types[t.ID()]; ok {
		panic(fmt.Sprintf("policy: type %d registered twice", t.ID()))
	}
	r.types[t.ID()] = t
}

// RegisterAlg adds a key algorithm. Numbers are never reused.
func (r *Registry) RegisterAlg(a KeyAlg) {
	if _, ok := r.algs[a.ID()]; ok {
		panic(fmt.Sprintf("policy: algorithm %d registered twice", a.ID()))
	}
	r.algs[a.ID()] = a
}

// Type looks up a policy type.
func (r *Registry) Type(id TypeID) (Type, bool) {
	t, ok := r.types[id]
	return t, ok
}

// Alg looks up a key algorithm.
func (r *Registry) Alg(id AlgID) (KeyAlg, bool) {
	a, ok := r.algs[id]
	return a, ok
}

// HasType reports whether the registry implements a policy type.
func (r *Registry) HasType(id TypeID) bool { _, ok := r.types[id]; return ok }

// HasAlg reports whether the registry implements a key algorithm.
func (r *Registry) HasAlg(id AlgID) bool { _, ok := r.algs[id]; return ok }

// Check verifies witness w against address addr for transaction tid under
// the series rules. A nil error means the witness is valid.
func (r *Registry) Check(h hash.Func, rules Rules, w Witness, addr, tid hash.Hash) error {
	if max := rules.MaxWitnessSize(); max > 0 && w.Size() > max {
		return ErrTooLarge
	}
	if Address(h, w.Policy) != addr {
		return ErrAddressMismatch
	}
	if !rules.AllowsPolicyType(w.Policy.Type) {
		return ErrTypeNotAllowed
	}
	t, ok := r.types[w.Policy.Type]
	if !ok {
		return ErrUnknownType
	}
	algs, err := t.KeyAlgs(r, w.Policy.Params)
	if err != nil {
		return err
	}
	for _, a := range algs {
		if !rules.AllowsKeyAlg(a) {
			return ErrAlgNotAllowed
		}
	}
	return t.Satisfy(r, w.Policy.Params, w.Data, Message(tid))
}
