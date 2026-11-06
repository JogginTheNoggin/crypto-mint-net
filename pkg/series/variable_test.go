package series

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// encodeV1 is the series byte form exactly as it was before variable
// supply (D163), frozen here so a change to Encode that touches a fixed
// series fails this test.
func encodeV1(p Params) []byte {
	w := encoding.NewWriter(128)
	w.U8(1).U32(uint32(p.HashID))
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

// A fixed series keeps its exact bytes, its series_id, its pools and its
// JSON: variable supply adds nothing to it (D163).
func TestFixedSeriesUnchanged(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 2000; i++ {
		p := base()
		p.Mints = 1 + r.Uint64N(1<<40)
		p.PoolSize = 1 + r.Uint64N(p.Mints)
		p.Supply = 1 + r.Uint64N(math.MaxUint64)
		p.MinLayerStrength, p.MinEntryStrength = uint16(r.IntN(257)), uint16(r.IntN(257))
		p.EntryHashCredit = r.IntN(2) == 0
		p.MaxWitness = r.Uint32()
		if r.IntN(2) == 0 {
			iss := hash.Hash(sha256.Sum256([]byte{byte(i)}))
			p.Issuer = &iss
		}
		got := p.Encode()
		if want := encodeV1(p); !bytes.Equal(got, want) {
			t.Fatalf("fixed series %d: byte form changed\n got %x\nwant %x", i, got, want)
		}
		s, err := New(p)
		if err != nil {
			t.Fatal(err)
		}
		if s.ID() != sha256Func()(hash.TagSeries, encodeV1(p)) {
			t.Fatalf("fixed series %d: series_id changed", i)
		}
		back, err := DecodeParams(got)
		if err != nil || back.SupplyType != Fixed || back.PartSize != 0 || back.MaxParts != 0 {
			t.Fatalf("fixed series %d read back as %+v: %v", i, back, err)
		}
		if s.IsVariable() {
			t.Fatal("a fixed series reads as variable")
		}
		if _, err := s.PartID(1); !errors.Is(err, ErrFixed) {
			t.Fatalf("a fixed series has parts: %v", err)
		}
		j, _ := json.Marshal(p)
		for _, k := range []string{"supply_type", "part_size", "max_parts"} {
			if strings.Contains(string(j), k) {
				t.Fatalf("a fixed series' JSON gained %s: %s", k, j)
			}
		}
	}
}

// The pinned series_id of the base params, from before variable supply.
func TestFixedSeriesIDPinned(t *testing.T) {
	s, err := New(base())
	if err != nil {
		t.Fatal(err)
	}
	const want = "9d7f095da805d59ec8aa667a24ce07d618dd3dbdd95a8a7d97318f5bcfbbe3f1"
	if got := s.ID().String(); got != want {
		t.Fatalf("series_id %s, want %s", got, want)
	}
}

type issuerKey struct {
	signer policy.Signer
	addr   hash.Hash
}

func newIssuer(t *testing.T, label string) issuerKey {
	t.Helper()
	seed := sha256.Sum256([]byte("PoON-test-key:" + label))
	sg, err := policy.NewEd25519Signer(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	p := policy.ThresholdParams{K: 1, Keys: []policy.Key{{Alg: sg.Alg(), Public: sg.Public()}}}.Policy()
	return issuerKey{sg, policy.Address(sha256Func(), p)}
}

// docParams is the example of the variable supply document: N = 50,000,
// M = 10,000, S = 1, P = 1,000.
func docParams(iss hash.Hash) Params {
	p := base()
	p.Mints, p.Supply, p.PoolSize = 50_000, 1, 1_000
	p.SupplyType, p.PartSize, p.Issuer = Variable, 10_000, &iss
	return p
}

func TestVariableByteForm(t *testing.T) {
	iss := newIssuer(t, "issuer")
	p := docParams(iss.addr)
	b := p.Encode()
	if b[0] != FormatVariable {
		t.Fatalf("format version %d", b[0])
	}
	back, err := DecodeParams(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.SupplyType != Variable || back.PartSize != 10_000 || back.MaxParts != 0 || !bytes.Equal(back.Encode(), b) {
		t.Fatalf("round trip: %+v", back)
	}
	p.MaxParts = 7
	if back, err = DecodeParams(p.Encode()); err != nil || back.MaxParts != 7 {
		t.Fatalf("max parts round trip: %+v %v", back, err)
	}
	// Every supply field is inside the series_id.
	s0, _ := New(docParams(iss.addr))
	for name, f := range map[string]func(*Params){
		"part size": func(p *Params) { p.PartSize = 20_000 },
		"max parts": func(p *Params) { p.MaxParts = 3 },
		"issuer":    func(p *Params) { o := newIssuer(t, "other").addr; p.Issuer = &o },
	} {
		q := docParams(iss.addr)
		f(&q)
		s, err := New(q)
		if err != nil {
			t.Fatal(name, err)
		}
		if s.ID() == s0.ID() {
			t.Errorf("%s does not change the series_id", name)
		}
	}
	// The same rules as a fixed series give another id.
	q := docParams(iss.addr)
	q.SupplyType, q.PartSize = Fixed, 0
	f, _ := New(q)
	if f.ID() == s0.ID() {
		t.Fatal("a variable series has its fixed twin's id")
	}
	// A wrong supply type under version 2 is refused, so there is one form.
	b = docParams(iss.addr).Encode()
	b[len(b)-17] = 0
	if _, err := DecodeParams(b); !errors.Is(err, ErrInvalid) {
		t.Fatalf("version 2 with a fixed supply type: %v", err)
	}
	j, _ := json.Marshal(docParams(iss.addr))
	if !strings.Contains(string(j), `"supply_type":"variable"`) || !strings.Contains(string(j), `"part_size":10000`) {
		t.Fatalf("JSON: %s", j)
	}
	var jp Params
	if err := json.Unmarshal([]byte(`{"supply_type":"growing"}`), &jp); err == nil {
		t.Fatal("an unknown supply type name was read")
	}
}

func TestVariableRules(t *testing.T) {
	iss := newIssuer(t, "issuer").addr
	for name, f := range map[string]func(*Params){
		"no issuer":            func(p *Params) { p.Issuer = nil },
		"no part size":         func(p *Params) { p.PartSize = 0 },
		"N not a multiple":     func(p *Params) { p.Mints = 50_500 },
		"M not a multiple":     func(p *Params) { p.PartSize = 10_500 },
		"cap overflows":        func(p *Params) { p.MaxParts = math.MaxUint64 },
		"unknown supply type":  func(p *Params) { p.SupplyType = 2 },
		"fixed with part size": func(p *Params) { p.SupplyType = Fixed },
		"fixed with max parts": func(p *Params) { p.SupplyType, p.PartSize, p.MaxParts = Fixed, 0, 2 },
	} {
		p := docParams(iss)
		f(&p)
		if _, err := New(p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	p := docParams(iss)
	p.MaxParts = 1
	if _, err := New(p); err != nil {
		t.Fatalf("a cap of one part: %v", err)
	}
}

// The document's table: parts, serials and pools.
func TestPartRanges(t *testing.T) {
	s, err := New(docParams(newIssuer(t, "issuer").addr))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ k, first, last, pool0, pool1 uint64 }{
		{1, 0, 49_999, 0, 49},
		{2, 50_000, 59_999, 50, 59},
		{3, 60_000, 69_999, 60, 69},
	} {
		f, l, err := s.PartRange(c.k)
		if err != nil || f != c.first || l != c.last {
			t.Fatalf("part %d: %d..%d %v", c.k, f, l, err)
		}
		if s.PoolOf(f) != c.pool0 || s.PoolOf(l) != c.pool1 {
			t.Fatalf("part %d pools %d..%d", c.k, s.PoolOf(f), s.PoolOf(l))
		}
		for _, serial := range []uint64{f, l, (f + l) / 2} {
			if k, err := s.PartOf(serial); err != nil || k != c.k {
				t.Fatalf("serial %d in part %d: %d %v", serial, c.k, k, err)
			}
		}
		if n, _ := s.MintsThrough(c.k); n != c.last+1 {
			t.Fatalf("MintsThrough(%d) = %d", c.k, n)
		}
	}
	if _, _, err := s.PartRange(0); !errors.Is(err, ErrNoPart) {
		t.Fatal("part 0")
	}
	capped := docParams(newIssuer(t, "issuer").addr)
	capped.MaxParts = 3
	c, _ := New(capped)
	if _, err := c.PartOf(70_000); !errors.Is(err, ErrNoPart) {
		t.Fatalf("a serial past the cap: %v", err)
	}
	if _, err := c.PartID(4); !errors.Is(err, ErrNoPart) {
		t.Fatalf("a part past the cap: %v", err)
	}
	// Without a cap, a part whose serials pass 64 bits never exists.
	if _, err := s.PartOf(math.MaxUint64 - 20_000); err != nil {
		t.Fatalf("a serial near the top: %v", err)
	}
	if _, err := s.PartOf(math.MaxUint64); !errors.Is(err, ErrNoPart) {
		t.Fatalf("a serial whose part would pass 64 bits: %v", err)
	}
	if _, err := s.MintsThrough(math.MaxUint64); !errors.Is(err, ErrNoPart) {
		t.Fatalf("an overflowing part: %v", err)
	}
}

// part_id(1) = H("PoON-part" ∥ series_id ∥ 1 ∥ series_id), part_id(k) =
// H("PoON-part" ∥ series_id ∥ k ∥ part_id(k − 1)).
func TestPartIDs(t *testing.T) {
	s, _ := New(docParams(newIssuer(t, "issuer").addr))
	h := sha256Func()
	id := s.ID()
	prev := id
	for k := uint64(1); k <= 5; k++ {
		w := encoding.NewWriter(40).U64(k).Raw(prev[:])
		want := h(hash.TagPart, id[:], w.Bytes())
		got, err := s.PartID(k)
		if err != nil || got != want {
			t.Fatalf("part %d: %s, want %s (%v)", k, got, want, err)
		}
		prev = want
	}
	// Worked out in another order, from a fresh series, the same.
	s2, _ := New(docParams(newIssuer(t, "issuer").addr))
	p5, _ := s2.PartID(5)
	p3, _ := s2.PartID(3)
	if q5, _ := s.PartID(5); q5 != p5 {
		t.Fatal("part 5 differs")
	}
	if q3, _ := s.PartID(3); q3 != p3 {
		t.Fatal("part 3 differs")
	}
}

func sign(t *testing.T, s *Series, k issuerKey, r Release) Release {
	t.Helper()
	r.Witness = policy.SingleKeyWitness(k.signer, s.ReleaseMessage(r.ID(s.Hash())))
	return r
}

func TestRelease(t *testing.T) {
	iss := newIssuer(t, "issuer")
	s, _ := New(docParams(iss.addr))
	u, err := s.Unsigned(2)
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := s.PartID(1)
	p2, _ := s.PartID(2)
	if u.Previous != p1 || u.ID(s.Hash()) != p2 {
		t.Fatalf("unsigned part 2: %+v", u)
	}
	r := sign(t, s, iss, u)
	if err := s.CheckRelease(r); err != nil {
		t.Fatal(err)
	}
	// Byte form and JSON round trips.
	back, err := DecodeRelease(r.Encode())
	if err != nil || !bytes.Equal(back.Encode(), r.Encode()) {
		t.Fatalf("byte form: %v", err)
	}
	j, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var jr Release
	if err := json.Unmarshal(j, &jr); err != nil || s.CheckRelease(jr) != nil || !bytes.Equal(jr.Encode(), r.Encode()) {
		t.Fatalf("JSON: %s %v", j, err)
	}
	// A release of part 3 is checked against the chain.
	r3 := sign(t, s, iss, mustUnsigned(t, s, 3))
	if err := s.CheckRelease(r3); err != nil {
		t.Fatal(err)
	}
	other := newIssuer(t, "other")
	for name, bad := range map[string]Release{
		"part 1":          sign(t, s, iss, Release{Series: s.ID(), Part: 1, Previous: s.ID()}),
		"part 0":          sign(t, s, iss, Release{Series: s.ID(), Part: 0, Previous: s.ID()}),
		"wrong previous":  sign(t, s, iss, Release{Series: s.ID(), Part: 3, Previous: p1}),
		"another signer":  sign(t, s, other, u),
		"another series":  sign(t, s, iss, Release{Series: hash.Hash{1}, Part: 2, Previous: p1}),
		"signed as a tid": {Series: u.Series, Part: 2, Previous: p1, Witness: policy.SingleKeyWitness(iss.signer, p2)},
		"issuer message":  {Series: u.Series, Part: 2, Previous: p1, Witness: policy.SingleKeyWitness(iss.signer, s.IssuerMessageTID())},
		"unsigned":        u,
	} {
		if err := s.CheckRelease(bad); !errors.Is(err, ErrRelease) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Re-signing gives another witness but the same part, and the next
	// part names the part id, never a signature.
	again := sign(t, s, iss, u)
	if again.ID(s.Hash()) != r.ID(s.Hash()) || s.CheckRelease(again) != nil {
		t.Fatal("a re-signed part 2 is another part")
	}
	// A release witness is never valid as the series' issuer witness.
	if err := s.CheckIssuer(r.Witness); err == nil {
		t.Fatal("a release witness passed as the issuer witness")
	}
	// Past the cap there is no part to release.
	cp := docParams(iss.addr)
	cp.MaxParts = 2
	c, _ := New(cp)
	if _, err := c.Unsigned(3); !errors.Is(err, ErrNoPart) {
		t.Fatalf("unsigned past the cap: %v", err)
	}
	r3c := sign(t, c, iss, Release{Series: c.ID(), Part: 3, Previous: mustPart(t, c, 2)})
	if err := c.CheckRelease(r3c); !errors.Is(err, ErrRelease) {
		t.Fatalf("a release past the cap: %v", err)
	}
	// A fixed series has no releases.
	f, _ := New(base())
	if err := f.CheckRelease(r); !errors.Is(err, ErrFixed) {
		t.Fatalf("a fixed series took a release: %v", err)
	}
}

func mustUnsigned(t *testing.T, s *Series, k uint64) Release {
	t.Helper()
	u, err := s.Unsigned(k)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustPart(t *testing.T, s *Series, k uint64) hash.Hash {
	t.Helper()
	id, err := s.PartID(k)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func sha256Func() hash.Func {
	h, err := hash.Lookup(hash.SHA256)
	if err != nil {
		panic(err)
	}
	return h
}
