package policy

import (
	"bytes"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// Policy types beyond the threshold type (decision D82).
const (
	// TypeHashlock is "show a preimage of this digest": the data is any
	// bytes whose hash, under a named hash function, is the digest. On
	// its own anyone who has seen the preimage can spend, so it belongs
	// inside a k-of-policies policy beside a key.
	TypeHashlock TypeID = 2
	// TypeKOfPolicies is "satisfy K of these policies": all of them when
	// K is their number, any one when K is 1. It nests, up to
	// MaxPolicyDepth.
	TypeKOfPolicies TypeID = 3
)

// Bounds of the types in this file.
const (
	// MaxPreimageSize bounds a hashlock preimage.
	MaxPreimageSize = 1024
	// MaxSubPolicies bounds the policies inside one k-of-policies policy.
	MaxSubPolicies = 16
	// MaxPolicyDepth bounds nesting: a k-of-policies policy inside another
	// counts one level more, and the outermost is level 1.
	MaxPolicyDepth = 4
)

// Composite is a policy type made of other policies. Registry.Check
// applies the series' rules to every policy inside one, at every depth.
type Composite interface {
	Parts(reg *Registry, params []byte) ([]Policy, error)
}

// HashlockParams are the params of a hashlock: the hash function, by its
// number in the hash list, and the digest a preimage must hash to.
//
// Byte form: the hash number as U32, then the 32-byte digest.
type HashlockParams struct {
	Hash   hash.ID
	Digest hash.Hash
}

// Encode returns the params bytes.
func (p HashlockParams) Encode() []byte {
	w := encoding.NewWriter(36)
	w.U32(uint32(p.Hash)).Raw(p.Digest[:])
	return w.Bytes()
}

// Policy returns the policy with these params.
func (p HashlockParams) Policy() Policy { return Policy{Type: TypeHashlock, Params: p.Encode()} }

// NewHashlock returns the hashlock for preimage under hash function id.
func NewHashlock(id hash.ID, preimage []byte) (HashlockParams, error) {
	f, err := hash.Lookup(id)
	if err != nil {
		return HashlockParams{}, err
	}
	return HashlockParams{Hash: id, Digest: f(preimage)}, nil
}

// DecodeHashlockParams parses hashlock params.
func DecodeHashlockParams(b []byte) (HashlockParams, hash.Func, error) {
	r := encoding.NewReader(b)
	p := HashlockParams{Hash: hash.ID(r.U32())}
	r.Into(p.Digest[:])
	if err := r.Finish(); err != nil {
		return p, nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	f, err := hash.Lookup(p.Hash)
	if err != nil {
		return p, nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	return p, f, nil
}

// HashlockData returns the witness data that opens a hashlock: the
// preimage as variable-length bytes.
func HashlockData(preimage []byte) []byte {
	w := encoding.NewWriter(4 + len(preimage))
	w.VarBytes(preimage)
	return w.Bytes()
}

type hashlockType struct{}

func (hashlockType) ID() TypeID   { return TypeHashlock }
func (hashlockType) Name() string { return "hashlock" }

func (hashlockType) KeyAlgs(_ *Registry, params []byte) ([]AlgID, error) {
	_, _, err := DecodeHashlockParams(params)
	return nil, err
}

// Satisfy hashes the preimage once. It does not depend on msg: the
// transaction is bound by whatever else the spending policy requires.
func (hashlockType) Satisfy(_ *Registry, params, data, _ []byte) error {
	p, f, err := DecodeHashlockParams(params)
	if err != nil {
		return err
	}
	r := encoding.NewReader(data)
	pre := r.VarBytes(MaxPreimageSize)
	if err := r.Finish(); err != nil {
		return fmt.Errorf("%w: %v", ErrBadData, err)
	}
	if f(pre) != p.Digest {
		return fmt.Errorf("%w: preimage does not match", ErrUnsatisfied)
	}
	return nil
}

// KOfPoliciesParams are the params of a k-of-policies policy.
//
// Byte form: K as U16, then the policy count as U32, then each policy as
// Policy.Encode writes it.
type KOfPoliciesParams struct {
	K        uint16
	Policies []Policy
}

// Encode returns the params bytes.
func (p KOfPoliciesParams) Encode() []byte {
	w := encoding.NewWriter(64)
	w.U16(p.K).Count(len(p.Policies))
	for _, q := range p.Policies {
		w.Raw(q.Encode())
	}
	return w.Bytes()
}

// Policy returns the policy with these params.
func (p KOfPoliciesParams) Policy() Policy {
	return Policy{Type: TypeKOfPolicies, Params: p.Encode()}
}

// AllOf and AnyOf are the two common cases.
func AllOf(ps ...Policy) Policy {
	return KOfPoliciesParams{K: uint16(len(ps)), Policies: ps}.Policy()
}
func AnyOf(ps ...Policy) Policy { return KOfPoliciesParams{K: 1, Policies: ps}.Policy() }

// DecodeKOfPoliciesParams parses k-of-policies params. It checks the
// shape (1 <= K <= n <= MaxSubPolicies, no repeated policy) and the
// nesting depth, not the policies inside.
func DecodeKOfPoliciesParams(b []byte) (KOfPoliciesParams, error) {
	return decodeKOf(b, 1)
}

func decodeKOf(b []byte, depth int) (KOfPoliciesParams, error) {
	if depth > MaxPolicyDepth {
		return KOfPoliciesParams{}, fmt.Errorf("%w: nested deeper than %d", ErrBadParams, MaxPolicyDepth)
	}
	r := encoding.NewReader(b)
	var p KOfPoliciesParams
	p.K = r.U16()
	n := r.Count(MaxSubPolicies, 12)
	for i := 0; i < n && r.Err() == nil; i++ {
		p.Policies = append(p.Policies, DecodePolicy(r))
	}
	if err := r.Finish(); err != nil {
		return p, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	if n == 0 || p.K == 0 || int(p.K) > n {
		return p, fmt.Errorf("%w: need 1 <= k <= n, got k=%d n=%d", ErrBadParams, p.K, n)
	}
	for i, q := range p.Policies {
		for j := 0; j < i; j++ {
			if q.Type == p.Policies[j].Type && bytes.Equal(q.Params, p.Policies[j].Params) {
				return p, fmt.Errorf("%w: policy %d repeats policy %d", ErrBadParams, i, j)
			}
		}
		if q.Type == TypeKOfPolicies {
			if _, err := decodeKOf(q.Params, depth+1); err != nil {
				return p, err
			}
		}
	}
	return p, nil
}

// Part is one satisfied policy in k-of-policies witness data.
type Part struct {
	Index uint16
	Data  []byte
}

// KOfPoliciesData is the witness data of a k-of-policies policy: exactly
// K parts, with strictly increasing indices, each the witness data for
// the policy at its index.
//
// Byte form: the part count as U32, then each part as its index (U16)
// and its data (variable-length bytes).
type KOfPoliciesData struct {
	Parts []Part
}

// Encode returns the data bytes.
func (d KOfPoliciesData) Encode() []byte {
	w := encoding.NewWriter(64)
	w.Count(len(d.Parts))
	for _, p := range d.Parts {
		w.U16(p.Index).VarBytes(p.Data)
	}
	return w.Bytes()
}

// DecodeKOfPoliciesData parses k-of-policies witness data.
func DecodeKOfPoliciesData(b []byte) (KOfPoliciesData, error) {
	r := encoding.NewReader(b)
	n := r.Count(MaxSubPolicies, 6)
	var d KOfPoliciesData
	for i := 0; i < n && r.Err() == nil; i++ {
		idx := r.U16()
		d.Parts = append(d.Parts, Part{Index: idx, Data: r.VarBytes(MaxDataSize)})
	}
	if err := r.Finish(); err != nil {
		return d, fmt.Errorf("%w: %v", ErrBadData, err)
	}
	return d, nil
}

type kOfType struct{}

func (kOfType) ID() TypeID   { return TypeKOfPolicies }
func (kOfType) Name() string { return "k-of-policies" }

// KeyAlgs names no algorithms itself; Registry.Check walks the parts.
func (kOfType) KeyAlgs(_ *Registry, params []byte) ([]AlgID, error) {
	_, err := DecodeKOfPoliciesParams(params)
	return nil, err
}

func (kOfType) Parts(_ *Registry, params []byte) ([]Policy, error) {
	p, err := DecodeKOfPoliciesParams(params)
	return p.Policies, err
}

// Satisfy requires exactly K parts, in increasing index order, each
// satisfying its policy. Each policy is checked at most once, so the cost
// is bounded by what the params list.
func (kOfType) Satisfy(reg *Registry, params, data, msg []byte) error {
	p, err := DecodeKOfPoliciesParams(params)
	if err != nil {
		return err
	}
	d, err := DecodeKOfPoliciesData(data)
	if err != nil {
		return err
	}
	if len(d.Parts) != int(p.K) {
		return fmt.Errorf("%w: %d parts, policy needs exactly %d", ErrBadData, len(d.Parts), p.K)
	}
	last := -1
	for _, part := range d.Parts {
		if int(part.Index) <= last || int(part.Index) >= len(p.Policies) {
			return fmt.Errorf("%w: part indices must be increasing and in range", ErrBadData)
		}
		last = int(part.Index)
		q := p.Policies[part.Index]
		t, ok := reg.Type(q.Type)
		if !ok {
			return fmt.Errorf("%w: %d", ErrUnknownType, q.Type)
		}
		if err := t.Satisfy(reg, q.Params, part.Data, msg); err != nil {
			return fmt.Errorf("part %d: %w", part.Index, err)
		}
	}
	return nil
}
