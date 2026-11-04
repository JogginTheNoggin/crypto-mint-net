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

// Known key algorithms (decision D81). docs/cryptography.md gives the
// sizes and standards of each.
const (
	// AlgEd25519 is Ed25519 (RFC 8032), 32-byte keys, 64-byte signatures.
	AlgEd25519 AlgID = 1
	// AlgECDSAP256 is ECDSA over P-256 with SHA-256.
	AlgECDSAP256 AlgID = 2
	// AlgECDSAP384 is ECDSA over P-384 with SHA-384.
	AlgECDSAP384 AlgID = 3
	// AlgECDSAP521 is ECDSA over P-521 with SHA-512.
	AlgECDSAP521 AlgID = 4
	// AlgECDSASecp256k1 is ECDSA over secp256k1 with SHA-256 (Bitcoin's).
	AlgECDSASecp256k1 AlgID = 5
	// AlgSchnorrSecp256k1 is BIP-340 Schnorr over secp256k1.
	AlgSchnorrSecp256k1 AlgID = 6
	// AlgMLDSA44, AlgMLDSA65 and AlgMLDSA87 are ML-DSA (FIPS 204), the
	// post-quantum lattice signature, at NIST security categories 2, 3, 5.
	AlgMLDSA44 AlgID = 7
	AlgMLDSA65 AlgID = 8
	AlgMLDSA87 AlgID = 9
	// AlgSLHDSASHA2128s is SLH-DSA-SHA2-128s (FIPS 205), the post-quantum
	// hash-based signature: 32-byte keys, 7,856-byte signatures (D168).
	AlgSLHDSASHA2128s AlgID = 10
)

// Limits that bound decoding before any policy-specific check runs.
const (
	// MaxParamsSize bounds the params of any policy.
	MaxParamsSize = 64 * 1024
	// MaxDataSize bounds the witness data of any policy.
	MaxDataSize = 64 * 1024
	// MaxKeySize and MaxSigSize bound one public key and one signature,
	// with room above the largest algorithm (ML-DSA-87: 2,592 and 4,627
	// bytes).
	MaxKeySize = 4096
	MaxSigSize = 8192
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
	p.EncodeTo(w)
	return w.Bytes()
}

// EncodeTo writes Encode's bytes to w, without a slice of its own.
func (p Policy) EncodeTo(w *encoding.Writer) {
	w.U32(4).U32(uint32(p.Type))
	w.VarBytes(p.Params)
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
	w.Policy.EncodeTo(enc)
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
	r.RegisterType(hashlockType{})
	r.RegisterType(kOfType{})
	r.RegisterAlg(ed25519Alg{})
	for _, id := range []AlgID{AlgECDSAP256, AlgECDSAP384, AlgECDSAP521} {
		r.RegisterAlg(nistAlgs[id])
	}
	r.RegisterAlg(k1Alg{})
	r.RegisterAlg(schnorrAlg{})
	for _, id := range []AlgID{AlgMLDSA44, AlgMLDSA65, AlgMLDSA87} {
		r.RegisterAlg(mldsaAlgs[id])
	}
	r.RegisterAlg(slhdsaAlgs[AlgSLHDSASHA2128s])
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

// Types returns the registered policy types, in number order.
func (r *Registry) Types() []Type {
	out := make([]Type, 0, len(r.types))
	for id := TypeID(1); len(out) < len(r.types); id++ {
		if t, ok := r.types[id]; ok {
			out = append(out, t)
		}
	}
	return out
}

// Algs returns the registered key algorithms, in number order.
func (r *Registry) Algs() []KeyAlg {
	out := make([]KeyAlg, 0, len(r.algs))
	for id := AlgID(1); len(out) < len(r.algs); id++ {
		if a, ok := r.algs[id]; ok {
			out = append(out, a)
		}
	}
	return out
}

// TypeByName and AlgByName look a number up by its name.
func (r *Registry) TypeByName(name string) (TypeID, bool) {
	for id, t := range r.types {
		if t.Name() == name {
			return id, true
		}
	}
	return 0, false
}

func (r *Registry) AlgByName(name string) (AlgID, bool) {
	for id, a := range r.algs {
		if a.Name() == name {
			return id, true
		}
	}
	return 0, false
}

// HasType reports whether the registry implements a policy type.
func (r *Registry) HasType(id TypeID) bool { _, ok := r.types[id]; return ok }

// HasAlg reports whether the registry implements a key algorithm.
func (r *Registry) HasAlg(id AlgID) bool { _, ok := r.algs[id]; return ok }

// Check verifies witness w against address addr for transaction tid under
// the series rules. A nil error means the witness is valid.
func (r *Registry) Check(h hash.Func, rules Rules, w Witness, addr, tid hash.Hash) error {
	return r.CheckCached(h, rules, w, addr, tid, nil)
}

// CheckCached is Check that skips the satisfaction function when v
// remembers it passing for the same policy, data and tid, and remembers
// each new pass. The size, address and allowed checks always run. A nil v
// remembers nothing.
func (r *Registry) CheckCached(h hash.Func, rules Rules, w Witness, addr, tid hash.Hash, v *Verified) error {
	if max := rules.MaxWitnessSize(); max > 0 && w.Size() > max {
		return ErrTooLarge
	}
	if Address(h, w.Policy) != addr {
		return ErrAddressMismatch
	}
	t, err := r.allowed(rules, w.Policy)
	if err != nil {
		return err
	}
	if v == nil {
		return t.Satisfy(r, w.Policy.Params, w.Data, Message(tid))
	}
	k := verifiedKey(w, tid)
	if v.has(k) {
		return nil
	}
	if err := t.Satisfy(r, w.Policy.Params, w.Data, Message(tid)); err != nil {
		return err
	}
	v.add(k)
	return nil
}

// Warm runs w's satisfaction function for tid and remembers a pass in v,
// without the address it must satisfy. CheckCached later still runs the
// size, address and allowed checks and finds the pass remembered: a pass
// depends only on the policy, the data and the tid, which the key covers.
// It is for data whose owners are not known yet, such as a node's own
// store on start-up (ORANGE-021). It reports whether w passed.
func (r *Registry) Warm(rules Rules, w Witness, tid hash.Hash, v *Verified) bool {
	if max := rules.MaxWitnessSize(); max > 0 && w.Size() > max {
		return false
	}
	t, err := r.allowed(rules, w.Policy)
	if err != nil {
		return false
	}
	k := verifiedKey(w, tid)
	if v.has(k) {
		return true
	}
	if t.Satisfy(r, w.Policy.Params, w.Data, Message(tid)) != nil {
		return false
	}
	v.add(k)
	return true
}

// allowed checks that the series allows policy p's type and key
// algorithms, and those of every policy inside it (decision D82).
func (r *Registry) allowed(rules Rules, p Policy) (Type, error) {
	if !rules.AllowsPolicyType(p.Type) {
		return nil, ErrTypeNotAllowed
	}
	t, ok := r.types[p.Type]
	if !ok {
		return nil, ErrUnknownType
	}
	algs, err := t.KeyAlgs(r, p.Params)
	if err != nil {
		return nil, err
	}
	for _, a := range algs {
		if !rules.AllowsKeyAlg(a) {
			return nil, ErrAlgNotAllowed
		}
	}
	if c, ok := t.(Composite); ok {
		parts, err := c.Parts(r, p.Params)
		if err != nil {
			return nil, err
		}
		for _, q := range parts {
			if _, err := r.allowed(rules, q); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

// CheckPolicy reports whether a policy is well formed and allowed by the
// rules, without a witness: what a receiver checks before giving out an
// address.
func (r *Registry) CheckPolicy(rules Rules, p Policy) error {
	_, err := r.allowed(rules, p)
	return err
}
