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

// FuzzComposite decodes hashlock and k-of-policies params and data, and
// tries to satisfy them: nothing may panic, and what decodes re-encodes to
// the same bytes.
func FuzzComposite(f *testing.F) {
	s := fuzzSigner(f)
	tid := hash.Hash{1}
	kw := SingleKeyWitness(s, tid)
	hl, _ := NewHashlock(hash.BLAKE3, []byte("secret"))
	both := KOfPoliciesParams{K: 2, Policies: []Policy{kw.Policy, hl.Policy()}}
	f.Add(uint8(TypeKOfPolicies), both.Encode(), KOfPoliciesData{Parts: []Part{{0, kw.Data}, {1, HashlockData([]byte("secret"))}}}.Encode())
	f.Add(uint8(TypeKOfPolicies), AnyOf(both.Policy(), kw.Policy).Params, []byte{0, 0, 0, 1, 0, 1})
	f.Add(uint8(TypeHashlock), hl.Encode(), HashlockData([]byte("secret")))
	reg := Default()
	msg := Message(tid)
	f.Fuzz(func(t *testing.T, typ uint8, params, data []byte) {
		switch TypeID(typ) {
		case TypeHashlock:
			if p, _, err := DecodeHashlockParams(params); err == nil && !bytes.Equal(p.Encode(), params) {
				t.Fatal("hashlock params re-encoding differs")
			}
		case TypeKOfPolicies:
			if p, err := DecodeKOfPoliciesParams(params); err == nil && !bytes.Equal(p.Encode(), params) {
				t.Fatal("k-of-policies params re-encoding differs")
			}
			if d, err := DecodeKOfPoliciesData(data); err == nil && !bytes.Equal(d.Encode(), data) {
				t.Fatal("k-of-policies data re-encoding differs")
			}
		default:
			return
		}
		ty, _ := reg.Type(TypeID(typ))
		reg.CheckPolicy(anyRules{}, Policy{Type: TypeID(typ), Params: params})
		ty.Satisfy(reg, params, data, msg)
	})
}

// FuzzVerify gives every algorithm arbitrary keys and signatures: none may
// panic or accept one. The seeds sign another message, so a mutation that
// undoes the flipped bit gives a real signature, but not over msg.
func FuzzVerify(f *testing.F) {
	msg := Message(hash.Hash{1})
	for _, a := range Default().Algs() {
		s, err := NewSigner(a.ID(), bytes.Repeat([]byte{7}, SeedSize))
		if err != nil {
			f.Fatal(err)
		}
		sig := s.Sign(Message(hash.Hash{2}))
		sig[0] ^= 1
		f.Add(uint8(a.ID()), s.Public(), sig)
	}
	reg := Default()
	f.Fuzz(func(t *testing.T, alg uint8, pub, sig []byte) {
		a, ok := reg.Alg(AlgID(alg))
		if !ok {
			return
		}
		if a.Verify(pub, msg, sig) {
			t.Fatal("arbitrary signature accepted")
		}
	})
}
