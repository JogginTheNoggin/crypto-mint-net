package series

import (
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

func base() Params {
	return Params{
		HashID:      hash.SHA256,
		Mints:       8,
		Supply:      100,
		PoolSize:    4,
		PolicyTypes: []policy.TypeID{policy.TypeThreshold},
		KeyAlgs:     []policy.AlgID{policy.AlgEd25519},
	}
}

func TestPools(t *testing.T) {
	cases := []struct{ n, p, pools, lastFirst, lastLast uint64 }{
		{100, 1, 100, 99, 99},
		{100, 10, 10, 90, 99},
		{105, 10, 11, 100, 104},
		{100, 100, 1, 0, 99},
		{100, 30, 4, 90, 99},
	}
	for _, c := range cases {
		p := base()
		p.Mints, p.PoolSize = c.n, c.p
		s, err := New(p)
		if err != nil {
			t.Fatal(err)
		}
		if s.PoolCount() != c.pools {
			t.Errorf("N=%d P=%d: %d pools, want %d", c.n, c.p, s.PoolCount(), c.pools)
		}
		f, l := s.PoolRange(c.pools - 1)
		if f != c.lastFirst || l != c.lastLast {
			t.Errorf("N=%d P=%d: last pool %d..%d", c.n, c.p, f, l)
		}
		if s.PoolOf(c.n-1) != c.pools-1 || s.PoolOf(0) != 0 {
			t.Errorf("PoolOf wrong")
		}
	}
}

func TestIDChangesWithEveryRule(t *testing.T) {
	s0, err := New(base())
	if err != nil {
		t.Fatal(err)
	}
	iss := hash.MustParse("1111111111111111111111111111111111111111111111111111111111111111")
	mutations := map[string]func(*Params){
		"mints":     func(p *Params) { p.Mints = 9 },
		"supply":    func(p *Params) { p.Supply = 101 },
		"pool size": func(p *Params) { p.PoolSize = 2 },
		"min layer": func(p *Params) { p.MinLayerStrength = 1 },
		"min entry": func(p *Params) { p.MinEntryStrength = 1 },
		"credit":    func(p *Params) { p.EntryHashCredit = true },
		"witness":   func(p *Params) { p.MaxWitness = 500 },
		"issuer":    func(p *Params) { p.Issuer = &iss },
	}
	for name, m := range mutations {
		p := base()
		m(&p)
		s, err := New(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s.ID() == s0.ID() {
			t.Errorf("%s: id did not change", name)
		}
	}
	// Same rules, same id.
	again, _ := New(base())
	if again.ID() != s0.ID() {
		t.Error("id not deterministic")
	}
}

func TestEncodeDecode(t *testing.T) {
	iss := hash.MustParse("2222222222222222222222222222222222222222222222222222222222222222")
	p := base()
	p.Issuer = &iss
	p.EntryHashCredit = true
	p.MinEntryStrength = 3
	back, err := DecodeParams(p.Encode())
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := New(p)
	s2, err := New(back)
	if err != nil {
		t.Fatal(err)
	}
	if s1.ID() != s2.ID() || *back.Issuer != iss {
		t.Fatal("round trip changed the series")
	}
	if _, err := DecodeParams(append(p.Encode(), 0)); !errors.Is(err, ErrInvalid) {
		t.Fatal("trailing bytes must be refused")
	}
	bad := p.Encode()
	bad[0] = 2
	if _, err := DecodeParams(bad); !errors.Is(err, ErrUnreadable) {
		t.Fatal("unknown format version must be unreadable")
	}
}

func TestInvalidAndUnreadable(t *testing.T) {
	invalid := map[string]func(*Params){
		"no mints":       func(p *Params) { p.Mints = 0 },
		"no supply":      func(p *Params) { p.Supply = 0 },
		"pool zero":      func(p *Params) { p.PoolSize = 0 },
		"pool too big":   func(p *Params) { p.PoolSize = 9 },
		"strength":       func(p *Params) { p.MinEntryStrength = 257 },
		"no types":       func(p *Params) { p.PolicyTypes = nil },
		"no algs":        func(p *Params) { p.KeyAlgs = nil },
		"unsorted types": func(p *Params) { p.PolicyTypes = []policy.TypeID{2, 1} },
		"repeated algs":  func(p *Params) { p.KeyAlgs = []policy.AlgID{1, 1} },
	}
	for name, m := range invalid {
		p := base()
		m(&p)
		if _, err := New(p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	unreadable := map[string]func(*Params){
		"hash": func(p *Params) { p.HashID = 99 },
		"type": func(p *Params) { p.PolicyTypes = []policy.TypeID{1, 99} },
		"alg":  func(p *Params) { p.KeyAlgs = []policy.AlgID{1, 99} },
	}
	for name, m := range unreadable {
		p := base()
		m(&p)
		if _, err := New(p); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: want ErrUnreadable, got %v", name, err)
		}
	}
}

func TestPoolIDsDiffer(t *testing.T) {
	s, _ := New(base())
	if s.PoolID(0) == s.PoolID(1) {
		t.Fatal("pool ids must differ")
	}
	p := base()
	p.Supply = 50
	other, _ := New(p)
	if other.PoolID(0) == s.PoolID(0) {
		t.Fatal("pool ids of different series must differ")
	}
}

func TestIssuerWitness(t *testing.T) {
	seed := sha256.Sum256([]byte("issuer"))
	signer, _ := policy.NewEd25519Signer(seed[:])
	pol := policy.SingleKey(policy.AlgEd25519, signer.Public())
	p := base()
	s0, _ := New(p)
	addr := s0.Address(pol)
	p.Issuer = &addr
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	w := policy.SingleKeyWitness(signer, s.IssuerMessageTID())
	if err := s.CheckIssuer(w); err != nil {
		t.Fatalf("valid issuer witness refused: %v", err)
	}
	// A witness over another series fails.
	p2 := p
	p2.Supply = 7
	s2, _ := New(p2)
	if err := s2.CheckIssuer(w); err == nil {
		t.Fatal("issuer witness must be bound to its series")
	}
	if err := s0.CheckIssuer(w); err == nil {
		t.Fatal("series without issuer must refuse")
	}
}
