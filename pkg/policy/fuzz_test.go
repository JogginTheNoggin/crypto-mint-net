package policy

import (
	"bytes"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// anyRules allows every registered type and algorithm, with no ceiling.
type anyRules struct{}

func (anyRules) AllowsPolicyType(TypeID) bool { return true }
func (anyRules) AllowsKeyAlg(AlgID) bool      { return true }
func (anyRules) MaxWitnessSize() int          { return 0 }

func fuzzSigner(f *testing.F) *Ed25519Signer {
	s, err := NewEd25519Signer(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		f.Fatal(err)
	}
	return s
}

// FuzzWitness decodes a witness and checks it against an address: neither
// may panic, and what decodes re-encodes to the same bytes.
func FuzzWitness(f *testing.F) {
	s := fuzzSigner(f)
	tid := hash.Hash{1}
	w := SingleKeyWitness(s, tid)
	enc := encoding.NewWriter(0)
	w.Encode(enc)
	f.Add(enc.Bytes())
	f.Add([]byte{0, 0, 0, 4})
	reg := Default()
	h, _ := hash.Lookup(hash.SHA256)
	addr := Address(h, w.Policy)
	f.Fuzz(func(t *testing.T, b []byte) {
		r := encoding.NewReader(b)
		got := DecodeWitness(r)
		if r.Finish() != nil {
			return
		}
		out := encoding.NewWriter(0)
		got.Encode(out)
		if !bytes.Equal(out.Bytes(), b) {
			t.Fatal("re-encoding differs")
		}
		reg.Check(h, anyRules{}, got, addr, tid)
	})
}

// FuzzThreshold decodes threshold params and data.
func FuzzThreshold(f *testing.F) {
	s := fuzzSigner(f)
	p := ThresholdParams{K: 1, Keys: []Key{{Alg: AlgEd25519, Public: s.Public()}}}
	f.Add(p.Encode(), ThresholdData{Sigs: []Sig{{Index: 0, Signature: make([]byte, 64)}}}.Encode())
	reg := Default()
	f.Fuzz(func(t *testing.T, params, data []byte) {
		if p, err := DecodeThresholdParams(reg, params); err == nil && !bytes.Equal(p.Encode(), params) {
			t.Fatal("params re-encoding differs")
		}
		if d, err := DecodeThresholdData(data); err == nil && !bytes.Equal(d.Encode(), data) {
			t.Fatal("data re-encoding differs")
		}
	})
}
