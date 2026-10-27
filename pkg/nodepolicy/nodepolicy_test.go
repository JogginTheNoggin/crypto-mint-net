package nodepolicy_test

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func intp(i int) *int { return &i }

func TestValidate(t *testing.T) {
	id := testutil.ExampleSeries().ID()
	cases := []struct {
		name string
		edit func(*nodepolicy.Policy)
		ok   bool
	}{
		{"default", func(*nodepolicy.Policy) {}, true},
		{"format", func(p *nodepolicy.Policy) { p.Format = 2 }, false},
		{"k unlimited", func(p *nodepolicy.Policy) { p.K = intp(-1) }, true},
		{"k too low", func(p *nodepolicy.Policy) { p.K = intp(-2) }, false},
		{"negative limit", func(p *nodepolicy.Policy) { p.MaxPending = -1 }, false},
		{"follow neither", func(p *nodepolicy.Policy) { p.Follow = []nodepolicy.Follow{{}} }, false},
		{"follow both", func(p *nodepolicy.Policy) { p.Follow = []nodepolicy.Follow{{Issuer: &id, Series: &id}} }, false},
		{"issuer with pools", func(p *nodepolicy.Policy) { p.Follow = []nodepolicy.Follow{{Issuer: &id, Pools: []uint64{1}}} }, false},
		{"series pools", func(p *nodepolicy.Policy) { p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}} }, true},
		{"rpc off", func(p *nodepolicy.Policy) { p.RPC.Listen = "off" }, true},
		{"rpc localhost", func(p *nodepolicy.Policy) { p.RPC.Listen = "localhost:0" }, true},
		{"rpc ipv6 loopback", func(p *nodepolicy.Policy) { p.RPC.Listen = "[::1]:9000" }, true},
		{"rpc remote", func(p *nodepolicy.Policy) { p.RPC.Listen = "0.0.0.0:9000" }, false},
		{"rpc remote allowed", func(p *nodepolicy.Policy) { p.RPC.Listen, p.RPC.AllowRemote = "0.0.0.0:9000", true }, true},
		{"rpc bad address", func(p *nodepolicy.Policy) { p.RPC.Listen = "nonsense" }, false},
		{"p2p off", func(p *nodepolicy.Policy) { p.P2P.Listen = "off" }, true},
		{"p2p remote", func(p *nodepolicy.Policy) { p.P2P.Listen = "0.0.0.0:7000" }, true},
		{"p2p bad address", func(p *nodepolicy.Policy) { p.P2P.Listen = "nonsense" }, false},
		{"p2p peers", func(p *nodepolicy.Policy) { p.P2P.Peers = []string{"example.org:8646", "[::1]:1"} }, true},
		{"p2p bad peer", func(p *nodepolicy.Policy) { p.P2P.Peers = []string{"example.org"} }, false},
		{"p2p negative", func(p *nodepolicy.Policy) { p.P2P.MaxPeers = -1 }, false},
		{"p2p never check", func(p *nodepolicy.Policy) { p.P2P.CheckSeconds = -1 }, true},
		{"p2p check too low", func(p *nodepolicy.Policy) { p.P2P.CheckSeconds = -2 }, false},
		{"p2p registries", func(p *nodepolicy.Policy) { p.P2P.Registries = []string{"https://example.org/reg.json"} }, true},
		{"p2p empty registry", func(p *nodepolicy.Policy) { p.P2P.Registries = []string{" "} }, false},
		{"p2p negative hops", func(p *nodepolicy.Policy) { p.P2P.HopLimit = -1 }, false},
		{"p2p advertise", func(p *nodepolicy.Policy) { p.P2P.Advertise = "node.example:8646" }, true},
		{"p2p bad advertise", func(p *nodepolicy.Policy) { p.P2P.Advertise = "node.example" }, false},
		{"p2p ping never", func(p *nodepolicy.Policy) { p.P2P.PingSeconds = -1 }, true},
	}
	for _, c := range cases {
		p := nodepolicy.Default()
		c.edit(&p)
		err := p.Validate()
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if err != nil && !errors.Is(err, nodepolicy.ErrInvalid) {
			t.Errorf("%s: not ErrInvalid: %v", c.name, err)
		}
	}
}

func TestSettings(t *testing.T) {
	p := nodepolicy.Default()
	if c := p.LedgerConfig(); c.K != ledger.DefaultK || c.Ties != 0 {
		t.Fatalf("%+v", c)
	}
	p.K, p.Ties = intp(0), 3
	if c := p.LedgerConfig(); c.K != 0 || c.Ties != 3 {
		t.Fatalf("%+v", c)
	}
	if p.ListenAddr() != nodepolicy.DefaultListen {
		t.Fatal(p.ListenAddr())
	}
	p.RPC.Listen = "off"
	if p.ListenAddr() != "" {
		t.Fatal(p.ListenAddr())
	}
	if p.P2PListenAddr() != nodepolicy.DefaultP2PListen {
		t.Fatal(p.P2PListenAddr())
	}
	p.P2P.Listen = "off"
	if p.P2PListenAddr() != "" {
		t.Fatal(p.P2PListenAddr())
	}
	p.P2P.Listen = "127.0.0.1:1"
	if p.P2PListenAddr() != "127.0.0.1:1" {
		t.Fatal(p.P2PListenAddr())
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x", nodepolicy.FileName)
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: testutil.ExampleParams()}}
	id := testutil.ExampleSeries().ID()
	p.Follow = []nodepolicy.Follow{{Series: &id, Mints: []uint64{5}}}
	p.K = intp(3)
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	q, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if *q.K != 3 || *q.Follow[0].Series != id || q.Follow[0].Mints[0] != 5 {
		t.Fatalf("%+v", q)
	}
	s, _ := series.New(q.Series[0].Params)
	if s.ID() != id {
		t.Fatal("series changed in the round trip")
	}
	if err := nodepolicy.Save(path, nodepolicy.Policy{}); err == nil {
		t.Fatal("invalid policy saved")
	}
	os.WriteFile(path, []byte("{"), 0o600)
	if _, err := nodepolicy.Load(path); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

type issuer struct {
	addr   hash.Hash
	signer *policy.Ed25519Signer
}

func newIssuer(label string) issuer {
	seed := sha256.Sum256([]byte(label))
	signer, _ := policy.NewEd25519Signer(seed[:])
	s := testutil.ExampleSeries()
	return issuer{addr: s.Address(policy.SingleKey(policy.AlgEd25519, signer.Public())), signer: signer}
}

// signed returns a series published by iss, with its witness.
func (iss issuer) signed(mints uint64) nodepolicy.SeriesInfo {
	p := testutil.ExampleParams()
	p.Mints = mints
	p.Issuer = &iss.addr
	s := testutil.MustSeries(p)
	w := policy.SingleKeyWitness(iss.signer, s.IssuerMessageTID())
	return nodepolicy.SeriesInfo{Params: p, IssuerWitness: nodepolicy.EncodeWitness(w)}
}

func TestAccepted(t *testing.T) {
	a, b := newIssuer("a"), newIssuer("b")
	unsigned := nodepolicy.SeriesInfo{Params: testutil.ExampleParams()}
	noWitness := a.signed(12)
	noWitness.IssuerWitness = ""
	forged := a.signed(16)
	forged.IssuerWitness = b.signed(16).IssuerWitness
	broken := nodepolicy.SeriesInfo{Params: series.Params{}}
	garbled := a.signed(20)
	garbled.IssuerWitness = "zz"
	all := []nodepolicy.SeriesInfo{unsigned, a.signed(8), b.signed(8), noWitness, forged, broken, garbled}

	cases := []struct {
		name   string
		rule   nodepolicy.Issuers
		accept []int
	}{
		// Any still refuses a bad witness and a series that cannot be built.
		{"any", nodepolicy.Issuers{Any: true}, []int{0, 1, 2, 3}},
		{"only a", nodepolicy.Issuers{Accept: []hash.Hash{a.addr}}, []int{1}},
		{"a and b", nodepolicy.Issuers{Accept: []hash.Hash{a.addr, b.addr}}, []int{1, 2}},
		{"none", nodepolicy.Issuers{}, nil},
	}
	for _, c := range cases {
		p := nodepolicy.Default()
		p.Series, p.Issuers = all, c.rule
		acc, ref := p.Accepted()
		if len(acc)+len(ref) != len(all) {
			t.Fatalf("%s: %d accepted + %d refused", c.name, len(acc), len(ref))
		}
		if len(acc) != len(c.accept) {
			t.Fatalf("%s: accepted %d, want %d (refused %v)", c.name, len(acc), len(c.accept), ref)
		}
		for i, want := range c.accept {
			s, _ := series.New(all[want].Params)
			if acc[i].Series.ID() != s.ID() {
				t.Fatalf("%s: accepted[%d] is the wrong series", c.name, i)
			}
		}
	}
	// A verified issuer is reported.
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{a.signed(8), unsigned}
	acc, _ := p.Accepted()
	if acc[0].Issuer == nil || *acc[0].Issuer != a.addr || acc[1].Issuer != nil {
		t.Fatal("issuer not reported")
	}
}

func TestWitnessHex(t *testing.T) {
	iss := newIssuer("a")
	si := iss.signed(8)
	w, err := nodepolicy.DecodeWitness(si.IssuerWitness)
	if err != nil {
		t.Fatal(err)
	}
	if nodepolicy.EncodeWitness(w) != si.IssuerWitness {
		t.Fatal("round trip changed the witness")
	}
	if _, err := nodepolicy.DecodeWitness(si.IssuerWitness + "00"); err == nil {
		t.Fatal("trailing bytes accepted")
	}
}
