package policy

import (
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

type rules struct {
	types map[TypeID]bool
	algs  map[AlgID]bool
	max   int
}

func (r rules) AllowsPolicyType(t TypeID) bool { return r.types[t] }
func (r rules) AllowsKeyAlg(a AlgID) bool      { return r.algs[a] }
func (r rules) MaxWitnessSize() int            { return r.max }

var allowAll = rules{types: map[TypeID]bool{TypeThreshold: true}, algs: map[AlgID]bool{AlgEd25519: true}}

func signer(t *testing.T, label string) *Ed25519Signer {
	t.Helper()
	seed := sha256.Sum256([]byte(label))
	s, err := NewEd25519Signer(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func sha(t *testing.T) hash.Func {
	f, err := hash.Lookup(hash.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSingleKeyWitness(t *testing.T) {
	h := sha(t)
	alice := signer(t, "alice")
	pol := SingleKey(AlgEd25519, alice.Public())
	addr := Address(h, pol)
	tid := h([]byte("some tid"))

	w := SingleKeyWitness(alice, tid)
	if err := Default().Check(h, allowAll, w, addr, tid); err != nil {
		t.Fatalf("valid witness refused: %v", err)
	}

	// The same witness does not authorize a different tid (no replay).
	other := h([]byte("other tid"))
	if err := Default().Check(h, allowAll, w, addr, other); !errors.Is(err, ErrUnsatisfied) {
		t.Fatalf("replay: want ErrUnsatisfied, got %v", err)
	}

	// A witness for a different key does not open the address.
	bob := signer(t, "bob")
	if err := Default().Check(h, allowAll, SingleKeyWitness(bob, tid), addr, tid); !errors.Is(err, ErrAddressMismatch) {
		t.Fatalf("want ErrAddressMismatch, got %v", err)
	}
}

func TestAddressDependsOnPolicy(t *testing.T) {
	h := sha(t)
	a := signer(t, "a")
	if Address(h, SingleKey(AlgEd25519, a.Public())) == Address(h, SingleKey(AlgID(2), a.Public())) {
		t.Fatal("the algorithm number must be inside the address")
	}
}

func TestThreshold2of3(t *testing.T) {
	h := sha(t)
	a, b, c := signer(t, "a"), signer(t, "b"), signer(t, "c")
	p := ThresholdParams{K: 2, Keys: []Key{
		{AlgEd25519, a.Public()}, {AlgEd25519, b.Public()}, {AlgEd25519, c.Public()},
	}}
	addr := Address(h, p.Policy())
	tid := h([]byte("tx"))

	w, err := SignThreshold(p, map[uint16]Signer{0: a, 2: c}, tid)
	if err != nil {
		t.Fatal(err)
	}
	if err := Default().Check(h, allowAll, w, addr, tid); err != nil {
		t.Fatalf("2 of 3 refused: %v", err)
	}

	// One signature is not enough.
	d := ThresholdData{Sigs: []Sig{{0, a.Sign(Message(tid))}}}
	if err := Default().Check(h, allowAll, Witness{p.Policy(), d.Encode()}, addr, tid); !errors.Is(err, ErrBadData) {
		t.Fatalf("want ErrBadData, got %v", err)
	}

	// Three signatures are refused too: exactly K keeps one encoding per set.
	all, _ := SignThreshold(ThresholdParams{K: 3, Keys: p.Keys}, map[uint16]Signer{0: a, 1: b, 2: c}, tid)
	if err := Default().Check(h, allowAll, Witness{p.Policy(), all.Data}, addr, tid); !errors.Is(err, ErrBadData) {
		t.Fatalf("want ErrBadData for extra signatures, got %v", err)
	}

	// Indices out of order are refused.
	msg := Message(tid)
	rev := ThresholdData{Sigs: []Sig{{2, c.Sign(msg)}, {0, a.Sign(msg)}}}
	if err := Default().Check(h, allowAll, Witness{p.Policy(), rev.Encode()}, addr, tid); !errors.Is(err, ErrBadData) {
		t.Fatalf("want ErrBadData for unordered indices, got %v", err)
	}

	// A signature by the wrong key for its index fails.
	wrong := ThresholdData{Sigs: []Sig{{0, a.Sign(msg)}, {1, c.Sign(msg)}}}
	if err := Default().Check(h, allowAll, Witness{p.Policy(), wrong.Encode()}, addr, tid); !errors.Is(err, ErrUnsatisfied) {
		t.Fatalf("want ErrUnsatisfied, got %v", err)
	}
}

func TestSeriesRules(t *testing.T) {
	h := sha(t)
	a := signer(t, "a")
	pol := SingleKey(AlgEd25519, a.Public())
	addr := Address(h, pol)
	tid := h([]byte("tx"))
	w := SingleKeyWitness(a, tid)

	noType := rules{types: map[TypeID]bool{}, algs: allowAll.algs}
	if err := Default().Check(h, noType, w, addr, tid); !errors.Is(err, ErrTypeNotAllowed) {
		t.Fatalf("want ErrTypeNotAllowed, got %v", err)
	}
	noAlg := rules{types: allowAll.types, algs: map[AlgID]bool{}}
	if err := Default().Check(h, noAlg, w, addr, tid); !errors.Is(err, ErrAlgNotAllowed) {
		t.Fatalf("want ErrAlgNotAllowed, got %v", err)
	}
	small := rules{types: allowAll.types, algs: allowAll.algs, max: w.Size() - 1}
	if err := Default().Check(h, small, w, addr, tid); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	exact := rules{types: allowAll.types, algs: allowAll.algs, max: w.Size()}
	if err := Default().Check(h, exact, w, addr, tid); err != nil {
		t.Fatalf("witness at the ceiling refused: %v", err)
	}
}

func TestUnknownTypeAndAlg(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tx"))
	unknownType := Policy{Type: 77, Params: []byte{1}}
	open := rules{types: map[TypeID]bool{77: true, TypeThreshold: true}, algs: map[AlgID]bool{AlgEd25519: true, 9: true}}
	if err := Default().Check(h, open, Witness{Policy: unknownType}, Address(h, unknownType), tid); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("want ErrUnknownType, got %v", err)
	}
	unknownAlg := SingleKey(9, make([]byte, 32))
	if err := Default().Check(h, open, Witness{Policy: unknownAlg}, Address(h, unknownAlg), tid); !errors.Is(err, ErrUnknownAlg) {
		t.Fatalf("want ErrUnknownAlg, got %v", err)
	}
}

func TestBadParams(t *testing.T) {
	cases := map[string]ThresholdParams{
		"k zero":       {K: 0, Keys: []Key{{AlgEd25519, make([]byte, 32)}}},
		"k above n":    {K: 2, Keys: []Key{{AlgEd25519, make([]byte, 32)}}},
		"no keys":      {K: 1},
		"short key":    {K: 1, Keys: []Key{{AlgEd25519, make([]byte, 31)}}},
		"repeated key": {K: 1, Keys: []Key{{AlgEd25519, make([]byte, 32)}, {AlgEd25519, make([]byte, 32)}}},
	}
	for name, p := range cases {
		if _, err := DecodeThresholdParams(Default(), p.Encode()); !errors.Is(err, ErrBadParams) {
			t.Errorf("%s: want ErrBadParams, got %v", name, err)
		}
	}
	// Trailing bytes are refused.
	b := append(SingleKey(AlgEd25519, make([]byte, 32)).Params, 0)
	if _, err := DecodeThresholdParams(Default(), b); !errors.Is(err, ErrBadParams) {
		t.Error("trailing bytes must be refused")
	}
}

func TestWitnessEncodingRoundTrip(t *testing.T) {
	h := sha(t)
	w := SingleKeyWitness(signer(t, "a"), h([]byte("x")))
	enc := encoding.NewWriter(0)
	w.Encode(enc)
	if enc.Len() != w.Size() {
		t.Fatalf("Size %d != encoded %d", w.Size(), enc.Len())
	}
	r := encoding.NewReader(enc.Bytes())
	back := DecodeWitness(r)
	if err := r.Finish(); err != nil {
		t.Fatal(err)
	}
	if back.Policy.Type != w.Policy.Type || string(back.Policy.Params) != string(w.Policy.Params) || string(back.Data) != string(w.Data) {
		t.Fatal("round trip mismatch")
	}
}

func TestSignThresholdErrors(t *testing.T) {
	a, b := signer(t, "a"), signer(t, "b")
	p := ThresholdParams{K: 1, Keys: []Key{{AlgEd25519, a.Public()}}}
	if _, err := SignThreshold(p, map[uint16]Signer{0: b}, hash.Zero); err == nil {
		t.Fatal("mismatched signer must fail")
	}
	if _, err := SignThreshold(p, map[uint16]Signer{5: a}, hash.Zero); err == nil {
		t.Fatal("out of range index must fail")
	}
	if _, err := SignThreshold(p, map[uint16]Signer{}, hash.Zero); err == nil {
		t.Fatal("too few signers must fail")
	}
	if _, err := NewEd25519Signer([]byte{1}); err == nil {
		t.Fatal("short seed must fail")
	}
}
