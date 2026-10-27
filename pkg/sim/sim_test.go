package sim_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/sim"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

func newSim(t *testing.T, seed int64, nodes int) *sim.Sim {
	t.Helper()
	s, err := sim.New(sim.Config{Seed: seed, Nodes: nodes, Degree: 2, MinLatency: 20, MaxLatency: 200, Series: testutil.ExampleSeries()})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestAgreement runs a busy network and checks that every node ends on
// the same ledger, that supply is conserved, and that a run is repeatable.
func TestAgreement(t *testing.T) {
	run := func() (map[uint64]string, sim.Stats) {
		s := newSim(t, 1, 12)
		w := sim.NewWallets(s, 6)
		if err := w.ClaimAll(); err != nil {
			t.Fatal(err)
		}
		var bs []*sim.Builder
		for i := 0; i < 12; i += 3 {
			for k := uint64(0); k < 2; k++ {
				bs = append(bs, s.StartBuilder(i, k, 2*sim.Second, 500, 64))
			}
		}
		stop := false
		w.RandomPayments(300, &stop)
		s.Run(60 * sim.Second)
		stop = true
		for _, b := range bs {
			b.Stop()
		}
		s.Settle(30 * sim.Second)
		tips := map[uint64]string{}
		for k := uint64(0); k < 2; k++ {
			if !s.Agree(k) {
				t.Fatalf("pool %d: nodes disagree: %v", k, s.Tips(k))
			}
			tip, wt := s.Tip(0, k)
			tips[k] = tip.String() + "/" + wt.String()
		}
		for i := range s.Nodes() {
			if err := s.CheckConservation(i); err != nil {
				t.Fatal(err)
			}
		}
		sel, other := s.Work(0, 1)
		t.Logf("sent %d refused %d; pool 1 work selected %s other %s; %+v", w.Sent, w.Refused, sel, other, s.Stats())
		return tips, s.Stats()
	}
	a, sa := run()
	b, sb := run()
	if a[0] != b[0] || a[1] != b[1] || sa != sb {
		t.Fatalf("not repeatable: %v %v / %+v %+v", a, b, sa, sb)
	}
}

func TestLoadScenario(t *testing.T) {
	r, err := sim.Load(3, sim.DefaultNet(), testutil.ExampleSeries(), 300, 60*sim.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", r)
	if !r.Agree || !r.Conserve || r.Settled == 0 {
		t.Fatalf("load: %+v", r)
	}
}

func TestPartitionScenario(t *testing.T) {
	for seed := int64(0); seed < 3; seed++ {
		r, err := sim.Partition(seed, sim.DefaultNet(), testutil.ExampleSeries(), 30*sim.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%+v", r)
		if !r.SidesDiffered || !r.Agree || !r.Conserve || r.Winner != r.HeavierSide || r.Lost != 0 || r.Carried == 0 {
			t.Fatalf("partition: %+v", r)
		}
	}
}

func TestAttackScenario(t *testing.T) {
	c := sim.AttackConfig{Net: sim.DefaultNet(), Share: 0.3, Threshold: weight.FromUint64(2000), HeadStart: 2 * sim.Second, GiveUp: 60 * sim.Second}
	r, err := sim.Attack(1, c, testutil.ExampleSeries())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Released {
		t.Fatalf("the receiver never released: %+v", r)
	}
	if again, _ := sim.Attack(1, c, testutil.ExampleSeries()); again != r {
		t.Fatalf("not repeatable: %+v %+v", r, again)
	}
	if _, err := sim.Attack(1, sim.AttackConfig{Net: sim.DefaultNet(), Share: 1}, testutil.ExampleSeries()); err == nil {
		t.Fatal("a share of 1 was accepted")
	}
}

// TestThresholdProtects checks the receiver's rule on the simulated
// network: a higher threshold leaves a weaker attacker fewer wins.
func TestThresholdProtects(t *testing.T) {
	if testing.Short() {
		t.Skip("runs 40 attacks")
	}
	s := testutil.ExampleSeries()
	c := sim.AttackConfig{Net: sim.DefaultNet(), Share: 0.2, HeadStart: 2 * sim.Second, GiveUp: 30 * sim.Second}
	c.Threshold = weight.FromUint64(1000)
	low, _, _, err := sim.AttackSweep(c, s, 20)
	if err != nil {
		t.Fatal(err)
	}
	c.Threshold = weight.FromUint64(200000)
	high, _, _, err := sim.AttackSweep(c, s, 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("attacker with 20%% of the work wins %.2f at threshold 1000, %.2f at 200000", low, high)
	if high >= low {
		t.Fatalf("a higher threshold did not help: %.2f >= %.2f", high, low)
	}
}

func TestSizing(t *testing.T) {
	rows, err := sim.Sizing(1, sim.DefaultNet(), testutil.ExampleParams(), []uint64{1, 8}, 500, 20*sim.Second, sim.AttackConfig{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Pools != 8 || rows[1].Pools != 1 || rows[0].Settled == 0 || rows[1].Settled == 0 {
		t.Fatalf("sizing %+v", rows)
	}
}

func TestConfigErrors(t *testing.T) {
	if _, err := sim.New(sim.Config{}); err == nil {
		t.Fatal("no nodes accepted")
	}
	if _, err := sim.New(sim.Config{Nodes: 2, Series: testutil.ExampleSeries(), MinLatency: 10, MaxLatency: 5}); err == nil {
		t.Fatal("inverted latency accepted")
	}
}

// TestPrivateNodeAndRelease checks the attacker's tools: a private node
// keeps what it makes until Release.
func TestPrivateNodeAndRelease(t *testing.T) {
	s := newSim(t, 2, 4)
	w := sim.NewWallets(s, 2)
	n := s.Node(3)
	n.Private = true
	if _, err := w.Claim(3, 4); err != nil {
		t.Fatal(err)
	}
	s.Run(5 * sim.Second)
	if st, _ := s.Node(0).Eng.Status(s.Pool(1)); st.Pending != 0 {
		t.Fatal("a private node's transaction reached the others")
	}
	n.Release()
	s.Run(5 * sim.Second)
	if st, _ := s.Node(0).Eng.Status(s.Pool(1)); st.Pending != 1 {
		t.Fatalf("released transaction did not arrive: %+v", st)
	}
}
