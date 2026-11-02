package policy

import (
	"bytes"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
)

// MaxThresholdKeys is the declared cost bound of the threshold type: at most
// this many keys, so at most this many signature checks per witness.
const MaxThresholdKeys = 16

// Key is one (algorithm, public key) pair inside a policy.
type Key struct {
	Alg    AlgID
	Public []byte
}

// ThresholdParams are the params of the threshold type: K signatures from
// the listed Keys.
//
// Byte form: K as U16, then the key count as U32, then each key as its
// algorithm number (U32) and its public key (variable-length bytes).
type ThresholdParams struct {
	K    uint16
	Keys []Key
}

// Encode returns the params bytes.
func (p ThresholdParams) Encode() []byte {
	w := encoding.NewWriter(6 + len(p.Keys)*40)
	w.U16(p.K).Count(len(p.Keys))
	for _, k := range p.Keys {
		w.U32(uint32(k.Alg)).VarBytes(k.Public)
	}
	return w.Bytes()
}

// Policy returns the policy with these params.
func (p ThresholdParams) Policy() Policy {
	return Policy{Type: TypeThreshold, Params: p.Encode()}
}

// SingleKey returns the 1-of-1 policy for one key, the simplest address.
func SingleKey(alg AlgID, public []byte) Policy {
	return ThresholdParams{K: 1, Keys: []Key{{Alg: alg, Public: public}}}.Policy()
}

// DecodeThresholdParams parses and validates threshold params against the
// registry's algorithms.
func DecodeThresholdParams(reg *Registry, b []byte) (ThresholdParams, error) {
	r := encoding.NewReader(b)
	var p ThresholdParams
	p.K = r.U16()
	n := r.Count(MaxThresholdKeys, 8)
	p.Keys = make([]Key, 0, n)
	for i := 0; i < n && r.Err() == nil; i++ {
		alg := AlgID(r.U32())
		pub := r.VarBytes(1024)
		p.Keys = append(p.Keys, Key{Alg: alg, Public: pub})
	}
	if err := r.Finish(); err != nil {
		return p, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	if n == 0 || p.K == 0 || int(p.K) > n {
		return p, fmt.Errorf("%w: need 1 <= k <= n, got k=%d n=%d", ErrBadParams, p.K, n)
	}
	for i, k := range p.Keys {
		a, ok := reg.Alg(k.Alg)
		if !ok {
			return p, fmt.Errorf("%w: %d", ErrUnknownAlg, k.Alg)
		}
		if len(k.Public) != a.PublicKeySize() {
			return p, fmt.Errorf("%w: key %d has %d bytes, %s needs %d", ErrBadParams, i, len(k.Public), a.Name(), a.PublicKeySize())
		}
		for j := 0; j < i; j++ {
			if p.Keys[j].Alg == k.Alg && bytes.Equal(p.Keys[j].Public, k.Public) {
				return p, fmt.Errorf("%w: key %d repeats key %d", ErrBadParams, i, j)
			}
		}
	}
	return p, nil
}

// Sig is one signature in threshold witness data, tied to the index of the
// key that made it.
type Sig struct {
	Index     uint16
	Signature []byte
}

// ThresholdData is the witness data of the threshold type: exactly K
// signatures, with strictly increasing key indices.
//
// Byte form: the signature count as U32, then each signature as its key
// index (U16) and its bytes (variable-length).
type ThresholdData struct {
	Sigs []Sig
}

// Encode returns the data bytes.
func (d ThresholdData) Encode() []byte {
	w := encoding.NewWriter(4 + len(d.Sigs)*70)
	w.Count(len(d.Sigs))
	for _, s := range d.Sigs {
		w.U16(s.Index).VarBytes(s.Signature)
	}
	return w.Bytes()
}

// DecodeThresholdData parses threshold witness data.
func DecodeThresholdData(b []byte) (ThresholdData, error) {
	r := encoding.NewReader(b)
	n := r.Count(MaxThresholdKeys, 6)
	d := ThresholdData{Sigs: make([]Sig, 0, n)}
	for i := 0; i < n && r.Err() == nil; i++ {
		idx := r.U16()
		sig := r.VarBytes(1024)
		d.Sigs = append(d.Sigs, Sig{Index: idx, Signature: sig})
	}
	if err := r.Finish(); err != nil {
		return d, fmt.Errorf("%w: %v", ErrBadData, err)
	}
	return d, nil
}

type thresholdType struct{}

func (thresholdType) ID() TypeID   { return TypeThreshold }
func (thresholdType) Name() string { return "threshold" }

func (thresholdType) KeyAlgs(reg *Registry, params []byte) ([]AlgID, error) {
	p, err := DecodeThresholdParams(reg, params)
	if err != nil {
		return nil, err
	}
	out := make([]AlgID, len(p.Keys))
	for i, k := range p.Keys {
		out[i] = k.Alg
	}
	return out, nil
}

// Satisfy requires exactly K signatures, one per distinct key, in increasing
// key order, each valid over msg. Requiring exactly K keeps the cost bounded
// and gives every satisfying set of signers one encoding.
func (thresholdType) Satisfy(reg *Registry, params, data, msg []byte) error {
	p, err := DecodeThresholdParams(reg, params)
	if err != nil {
		return err
	}
	d, err := DecodeThresholdData(data)
	if err != nil {
		return err
	}
	if len(d.Sigs) != int(p.K) {
		return fmt.Errorf("%w: %d signatures, policy needs exactly %d", ErrBadData, len(d.Sigs), p.K)
	}
	last := -1
	for _, s := range d.Sigs {
		if int(s.Index) <= last || int(s.Index) >= len(p.Keys) {
			return fmt.Errorf("%w: key indices must be increasing and in range", ErrBadData)
		}
		last = int(s.Index)
		k := p.Keys[s.Index]
		a, _ := reg.Alg(k.Alg) // present: checked by DecodeThresholdParams
		if len(s.Signature) != a.SignatureSize() {
			return fmt.Errorf("%w: signature for key %d has wrong size", ErrBadData, s.Index)
		}
		if !a.Verify(k.Public, msg, s.Signature) {
			return fmt.Errorf("%w: signature for key %d", ErrUnsatisfied, s.Index)
		}
	}
	return nil
}
