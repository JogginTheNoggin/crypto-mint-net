package nodepolicy_test

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
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
		{"grind auto", func(p *nodepolicy.Policy) {
			p.Grind = &nodepolicy.Grind{Device: "auto", Jobs: []nodepolicy.GrindJob{{Target: "tip", Hours: "22-06"}, {Target: "pending", Series: &id}, {Target: id.String(), UntilFrom: "1000"}}}
		}, true},
		{"grind bad target", func(p *nodepolicy.Policy) { p.Grind = &nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "tips"}}} }, false},
		{"grind bad hours", func(p *nodepolicy.Policy) {
			p.Grind = &nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "tip", Hours: "25-1"}}}
		}, false},
		{"grind until on tip", func(p *nodepolicy.Policy) {
			p.Grind = &nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "tip", UntilFrom: "5"}}}
		}, false},
		{"grind threads", func(p *nodepolicy.Policy) { p.Grind = &nodepolicy.Grind{Threads: -1} }, false},
		{"grind layer floor", func(p *nodepolicy.Policy) { p.Grind = &nodepolicy.Grind{LayerFloor: 20, LayerRate: 50} }, true},
		{"grind layer rate", func(p *nodepolicy.Policy) {
			p.Grind = &nodepolicy.Grind{LayerFloor: 20, LayerRate: nodepolicy.MaxLayerRate + 1}
		}, false},
		{"grind too many", func(p *nodepolicy.Policy) {
			p.Grind = &nodepolicy.Grind{Jobs: make([]nodepolicy.GrindJob, nodepolicy.MaxGrindJobs+1)}
		}, false},
		{"p2p negative", func(p *nodepolicy.Policy) { p.P2P.MaxPeers = -1 }, false},
		{"p2p never check", func(p *nodepolicy.Policy) { p.P2P.CheckSeconds = -1 }, true},
		{"p2p check too low", func(p *nodepolicy.Policy) { p.P2P.CheckSeconds = -2 }, false},
		{"p2p registries", func(p *nodepolicy.Policy) { p.P2P.Registries = []string{"https://example.org/reg.json"} }, true},
		{"p2p empty registry", func(p *nodepolicy.Policy) { p.P2P.Registries = []string{" "} }, false},
		{"p2p negative hops", func(p *nodepolicy.Policy) { p.P2P.HopLimit = -1 }, false},
		{"p2p advertise", func(p *nodepolicy.Policy) { p.P2P.Advertise = "node.example:8646" }, true},
		{"p2p bad advertise", func(p *nodepolicy.Policy) { p.P2P.Advertise = "node.example" }, false},
		{"p2p ping never", func(p *nodepolicy.Policy) { p.P2P.PingSeconds = -1 }, true},
		{"p2p slots", func(p *nodepolicy.Policy) { p.P2P.PrioritySlots, p.P2P.BackupSlots = 8, 2 }, true},
		{"p2p negative slots", func(p *nodepolicy.Policy) { p.P2P.BackupSlots = -1 }, false},
		{"p2p no unlisted", func(p *nodepolicy.Policy) { p.P2P.UnlistedConns = -1 }, true},
		{"p2p review never", func(p *nodepolicy.Policy) { p.P2P.ReviewSeconds = -1 }, true},
		{"tracking detailed", func(p *nodepolicy.Policy) { p.Tracking.Template = "detailed" }, true},
		{"tracking unknown template", func(p *nodepolicy.Policy) { p.Tracking.Template = "nope" }, false},
		{"tracking custom", func(p *nodepolicy.Policy) {
			p.Tracking.Custom = map[string]tracking.Custom{"lean": {From: "basic", Off: []string{"rtt"}}}
			p.Tracking.Groups = map[string]string{"candidate": "lean"}
		}, true},
		{"tracking custom cycle", func(p *nodepolicy.Policy) {
			p.Tracking.Custom = map[string]tracking.Custom{"x": {From: "y"}, "y": {From: "x"}}
		}, false},
		{"tracking unknown group", func(p *nodepolicy.Policy) { p.Tracking.Groups = map[string]string{"friends": "basic"} }, false},
		{"tracking negative limit", func(p *nodepolicy.Policy) { p.Tracking.BlockLimit = -1 }, false},
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

// The tracking section fills a tracker's settings, the defaults standing
// for whatever it leaves out.
func TestTrackingConfig(t *testing.T) {
	p := nodepolicy.Default()
	d := p.TrackingConfig()
	if def := tracking.DefaultConfig(); d.Limits != def.Limits || d.EscalateAt != def.EscalateAt {
		t.Fatalf("defaults %+v", d)
	}
	p.Tracking = nodepolicy.Tracking{Template: "detailed", EscalateAt: -1, BlockLimit: 50, TimeoutSeconds: 60}
	c := p.TrackingConfig()
	if c.Default != "detailed" || c.EscalateAt != 0 || c.Limits.Block != 50 || c.Limits.TimeOutFor != time.Minute ||
		c.Limits.TimeOut != d.Limits.TimeOut {
		t.Fatalf("config %+v", c)
	}
	if _, err := tracking.New(c, ""); err != nil {
		t.Fatal(err)
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

// A variable series, signed by its issuer, is refused as unreadable at
// this node until its transactions are built (D163, phase 1); the fixed
// series beside it is accepted as before.
func TestVariableNotFollowedYet(t *testing.T) {
	a := newIssuer("a")
	p := testutil.ExampleParams()
	p.Mints, p.PoolSize, p.Issuer = 8, 4, &a.addr
	p.SupplyType, p.PartSize = series.Variable, 4
	s := testutil.MustSeries(p)
	v := nodepolicy.SeriesInfo{Params: p, IssuerWitness: nodepolicy.EncodeWitness(policy.SingleKeyWitness(a.signer, s.IssuerMessageTID()))}
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{v, a.signed(8)}
	acc, ref := pol.Accepted()
	if len(acc) != 1 || acc[0].Series.IsVariable() || len(ref) != 1 || ref[0].Index != 0 {
		t.Fatalf("accepted %d, refused %v", len(acc), ref)
	}
	if !errors.Is(ref[0].Reason, nodepolicy.ErrVariableNotFollowed) || !errors.Is(ref[0].Reason, series.ErrUnreadable) {
		t.Fatalf("refused as %v", ref[0].Reason)
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

func TestLogSection(t *testing.T) {
	p := nodepolicy.Default()
	if l, f := p.LogSettings(); l != "" || f != "" {
		t.Fatal(l, f)
	}
	p.Log = &nodepolicy.Log{Level: "debug", Format: "json"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []nodepolicy.Log{{Level: "loud"}, {Format: "xml"}} {
		p.Log = &bad
		if err := p.Validate(); !errors.Is(err, nodepolicy.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

// RED-013: concurrent saves of documents of different lengths always
// leave a file that loads, and no temporary files.
func TestConcurrentSavesNeverTear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, nodepolicy.FileName)
	short := nodepolicy.Default()
	long := nodepolicy.Default()
	for i := 0; i < 40; i++ {
		long.Grind = &nodepolicy.Grind{}
		long.Grind.Jobs = append(long.Grind.Jobs, nodepolicy.GrindJob{Target: "tip", Hours: "0-23"})
	}
	for round := 0; round < 200; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			p := short
			if i%2 == 1 {
				p = long
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := nodepolicy.Save(path, p); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if _, err := nodepolicy.Load(path); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 {
		t.Fatalf("%d files left in the directory", len(es))
	}
}

// RED-025: a node.json written while a bound was higher still opens; the
// value is brought down to the bound and noted, and a write of it is
// still refused.
func TestLoadLowersAValueOverALoweredBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), nodepolicy.FileName)
	p := nodepolicy.Default()
	p.Grind = &nodepolicy.Grind{LayerFloor: 8, LayerRate: 10}
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), `"layer_rate": 10`, `"layer_rate": 200`, 1))
	if !strings.Contains(string(b), `"layer_rate": 200`) {
		t.Fatalf("setup: %s", b)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	q, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatalf("an older valid node.json is refused: %v", err)
	}
	if q.Grind.LayerRate != nodepolicy.MaxLayerRate || len(q.Lowered) != 1 {
		t.Fatalf("layer_rate %d, notes %q", q.Grind.LayerRate, q.Lowered)
	}
	q.Grind.LayerRate = 200
	if err := nodepolicy.Save(path, q); err == nil {
		t.Fatal("a layer_rate over the most was written")
	}
}

// SameApplied ignores the grind section, which a running node applies at
// once, and sees any other change (GREEN-037).
func TestSameApplied(t *testing.T) {
	a := nodepolicy.Policy{Format: 1}
	b := a
	b.Grind = &nodepolicy.Grind{Threads: 2}
	if !a.SameApplied(b) {
		t.Fatal("a grind change needs a reload")
	}
	k := 3
	b.K = &k
	if a.SameApplied(b) {
		t.Fatal("a change of K is applied without a reload")
	}
}
