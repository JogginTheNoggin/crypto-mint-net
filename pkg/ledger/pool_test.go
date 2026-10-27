package ledger_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

// layerOf grinds a layer of exactly the given strength on an entry hash.
// nonceBase keeps layers on the same entry distinct.
func layerOf(t *testing.T, s *series.Series, e hash.Hash, strength int, nonceBase uint64) layer.Layer {
	t.Helper()
	for n := nonceBase; ; n++ {
		l := layer.Layer{Entry: e, Nonce: n}
		if l.Strength(s.Hash()) == strength {
			return l
		}
	}
}

type fixture struct {
	t     *testing.T
	s     *series.Series
	k     *testutil.Keys
	pool  *ledger.Pool
	nonce uint64
}

func newFixture(t *testing.T, cfg ledger.Config) *fixture {
	s := testutil.ExampleSeries()
	return &fixture{t: t, s: s, k: testutil.NewKeys(s), pool: ledger.New(s, 1, cfg)}
}

func (f *fixture) build(parent hash.Hash, txs ...*tx.Tx) *entry.Entry {
	f.t.Helper()
	e, _, err := entry.Build(f.s, parent, txs, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

func (f *fixture) add(e *entry.Entry) ledger.AddResult {
	f.t.Helper()
	res, err := f.pool.AddEntry(e)
	if err != nil {
		f.t.Fatalf("AddEntry: %v", err)
	}
	return res
}

func (f *fixture) layers(e hash.Hash, strengths ...int) *ledger.Switch {
	f.t.Helper()
	var sw *ledger.Switch
	for _, st := range strengths {
		f.nonce += 1 << 32
		res, err := f.pool.AddLayer(layerOf(f.t, f.s, e, st, f.nonce))
		if err != nil {
			f.t.Fatalf("AddLayer: %v", err)
		}
		if res.Switch != nil {
			sw = res.Switch
		}
	}
	return sw
}

func (f *fixture) tid(x *tx.Tx) hash.Hash { return x.TID(f.s.Hash()) }

// appendixA builds the ledger of the design's worked example A.1 with the
// layers of A.2 and returns its transactions by name.
func appendixA(f *fixture) (map[string]*tx.Tx, []hash.Hash) {
	k := f.k
	x := map[string]*tx.Tx{}
	x["c4"] = k.Claim(4, P("a1", 100))
	x["c5"] = k.Claim(5, P("b1", 100))
	x["c6"] = k.Claim(6, P("carol", 100))
	e1 := f.build(f.pool.ID(), x["c4"], x["c5"], x["c6"])

	x["alice-carol"] = k.Spend(4, []testutil.In{k.Out(x["c4"], 0, "a1")}, P("carol", 30), P("a2", 70))
	x["bob-dave"] = k.Spend(5, []testutil.In{k.Out(x["c5"], 0, "b1")}, P("d1", 40), P("b2", 60))
	x["carol-erin"] = k.Spend(6, []testutil.In{k.Out(x["c6"], 0, "carol")}, P("e1", 100))
	e2 := f.build(e1.Hash(f.s), x["alice-carol"], x["bob-dave"], x["carol-erin"])

	x["alice-erin"] = k.Spend(4, []testutil.In{k.Out(x["alice-carol"], 1, "a2")}, P("e2", 25), P("a3", 45))
	x["carol-erin30"] = k.Spend(4, []testutil.In{k.Out(x["alice-carol"], 0, "carol")}, P("e3", 30))
	x["dave-frank"] = k.Spend(5, []testutil.In{k.Out(x["bob-dave"], 0, "d1")}, P("frank", 15), P("d2", 25))
	e3 := f.build(e2.Hash(f.s), x["alice-erin"], x["carol-erin30"], x["dave-frank"])

	x["erin-merge"] = k.Spend(4, []testutil.In{k.Out(x["alice-erin"], 0, "e2"), k.Out(x["carol-erin30"], 0, "e3")}, P("e1", 55))
	x["frank-gina"] = k.Spend(5, []testutil.In{k.Out(x["dave-frank"], 0, "frank")}, P("gina", 15))
	e4 := f.build(e3.Hash(f.s), x["erin-merge"], x["frank-gina"])

	var hs []hash.Hash
	for _, e := range []*entry.Entry{e1, e2, e3, e4} {
		f.add(e)
		hs = append(hs, e.Hash(f.s))
	}
	f.layers(hs[0], 12)
	f.layers(hs[1], 12, 10)
	f.layers(hs[2], 12)
	f.layers(hs[3], 10, 12)
	return x, hs
}

func TestAppendixA2Weights(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	_, hs := appendixA(f)
	if w := f.pool.Weight(); w.String() != "18432" {
		t.Fatalf("ledger weight %s, want 18432", w)
	}
	want := []string{"18432", "14336", "9216", "5120"}
	for i, h := range hs {
		got, err := f.pool.From(h)
		if err != nil || got.String() != want[i] {
			t.Errorf("from(E%d) = %s, want %s", i+1, got, want[i])
		}
	}
	// WeightTo is the rest: the weight up to and including an entry.
	for i, w := range []string{"4096", "9216", "13312", "18432"} {
		if got, ok := f.pool.WeightTo(hs[i]); !ok || got.String() != w {
			t.Errorf("to(E%d) = %s, want %s", i+1, got, w)
		}
	}
	if w, ok := f.pool.WeightTo(f.pool.ID()); !ok || !w.IsZero() {
		t.Error("the pool_id carries weight")
	}
	if _, ok := f.pool.WeightTo(hash.Hash{1}); ok {
		t.Error("an unknown entry has a weight")
	}
	tip, height, _ := f.pool.Tip()
	if tip != hs[3] || height != 4 {
		t.Fatal("tip wrong")
	}
	// Each mint adds up to the supply.
	for serial, total := range f.pool.State().Totals() {
		if total != 100 {
			t.Errorf("mint %d totals %d", serial, total)
		}
	}
}

func TestAppendixA3RivalLedger(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	x, hs := appendixA(f)
	k := f.k

	// Alice signs a second spend of the portion she claimed.
	rivalTx := k.Spend(4, []testutil.In{k.Out(x["c4"], 0, "a1")}, P("eve", 100))
	r2 := f.build(hs[0], rivalTx)
	if res := f.add(r2); res.Switch != nil {
		t.Fatal("a rival with no layers must not be selected")
	}
	r2h := r2.Hash(f.s)

	// Case A: 4,096 + 8,192 + 1,024 = 13,312 against 14,336.
	if sw := f.layers(r2h, 12, 13, 10); sw != nil {
		t.Fatal("case A must not switch")
	}
	if tip, _, _ := f.pool.Tip(); tip != hs[3] {
		t.Fatal("honest ledger must stay selected")
	}
	info, _ := f.pool.Info(r2h)
	if info.Ledger.String() != "17408" { // 4,096 shared + 13,312
		t.Fatalf("rival ledger weight %s", info.Ledger)
	}
	// Carol, paid in E2, sees the rival: 13,312 after the fork against
	// from(E2) = 14,336, a margin of 1,024 (the design's section 7.3).
	rs := f.pool.Rivals(x["alice-carol"], f.tid(x["alice-carol"]))
	if len(rs) != 1 || rs[0].Conflict != f.tid(rivalTx) || rs[0].Entry != r2h || rs[0].Fork != hs[0] || rs[0].After.String() != "13312" {
		t.Fatalf("rivals of alice-carol: %+v", rs)
	}
	// Bob's payment conflicts with nothing in the rival.
	if rs := f.pool.Rivals(x["bob-dave"], f.tid(x["bob-dave"])); len(rs) != 0 {
		t.Fatalf("rivals of bob-dave: %+v", rs)
	}

	// Case B: 2,048 and a lucky 16,384 more: 31,744 after E1.
	sw := f.layers(r2h, 11, 14)
	if sw == nil {
		t.Fatal("case B must switch")
	}
	if sw.Fork != hs[0] || sw.ForkHeight != 1 || len(sw.Reverted) != 3 || len(sw.Applied) != 1 {
		t.Fatalf("switch shape wrong: %+v", sw)
	}
	names := map[hash.Hash]string{}
	for n, t := range x {
		names[f.tid(t)] = n
	}
	got := func(ds []ledger.Displaced) []string {
		var out []string
		for _, d := range ds {
			out = append(out, names[d.TID])
		}
		return out
	}
	wantVoid := map[string]bool{"alice-carol": true, "alice-erin": true, "carol-erin30": true, "erin-merge": true}
	wantCarried := map[string]bool{"bob-dave": true, "carol-erin": true, "dave-frank": true, "frank-gina": true}
	for _, n := range got(sw.Void) {
		if !wantVoid[n] {
			t.Errorf("%s should not be void", n)
		}
		delete(wantVoid, n)
	}
	for _, n := range got(sw.Carried) {
		if !wantCarried[n] {
			t.Errorf("%s should not be carried", n)
		}
		delete(wantCarried, n)
	}
	if len(wantVoid) != 0 || len(wantCarried) != 0 || len(sw.Unchanged) != 0 {
		t.Fatalf("missing: void %v carried %v", wantVoid, wantCarried)
	}
	// The displaced entries were buried under the honest weight.
	if sw.Void[0].Weight.String() != "14336" {
		t.Errorf("buried weight %s, want 14336", sw.Void[0].Weight)
	}

	// Mint 4 is now one portion of 100 at Eve's address.
	ps := f.pool.State().Portions(4)
	if len(ps) != 1 || ps[0].Portion.Address != k.Addr("eve") || ps[0].Portion.Amount != 100 {
		t.Fatalf("mint 4 after switch: %+v", ps)
	}
	// The old ledger is inactive but kept, and its layers stay with it.
	if !f.pool.Has(hs[3]) || f.pool.InSelected(hs[3]) {
		t.Fatal("old ledger must be kept and inactive")
	}

	// Putting the carried transactions back takes one entry per link.
	r3 := f.build(r2h, x["bob-dave"], x["carol-erin"])
	f.add(r3)
	f.layers(r3.Hash(f.s), 1)
	r4 := f.build(r3.Hash(f.s), x["dave-frank"])
	f.add(r4)
	f.layers(r4.Hash(f.s), 1)
	r5 := f.build(r4.Hash(f.s), x["frank-gina"])
	f.add(r5)
	f.layers(r5.Hash(f.s), 1)
	if tip, h, _ := f.pool.Tip(); tip != r5.Hash(f.s) || h != 5 {
		t.Fatal("carried chain not selected")
	}
	// Frank's payment to Gina cannot share an entry with the payment that
	// created its input.
	bad := f.build(r2h, x["dave-frank"], x["frank-gina"])
	if _, err := f.pool.AddEntry(bad); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Fatalf("want ErrInvalidEntry, got %v", err)
	}
	m5 := f.pool.State().Portions(5)
	if len(m5) != 3 {
		t.Fatalf("mint 5 after R5: %+v", m5)
	}

	// Restoring the honest ledger takes only more layers on it.
	if sw := f.layers(hs[3], 16); sw == nil || sw.To != hs[3] {
		t.Fatal("honest ledger should return with more layers")
	}
	if ps := f.pool.State().Portions(4); len(ps) != 2 {
		t.Fatalf("mint 4 after restore: %+v", ps)
	}
}

func TestAppendixA4Siblings(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	x, hs := appendixA(f)
	k := f.k
	a3ToFrank := k.Spend(4, []testutil.In{k.Out(x["alice-erin"], 1, "a3")}, P("frank", 45))
	erinToGina := k.Spend(6, []testutil.In{k.Out(x["carol-erin"], 0, "e1")}, P("gx", 100))
	bobToHank := k.Spend(5, []testutil.In{k.Out(x["bob-dave"], 1, "b2")}, P("hank", 60))

	e5x := f.build(hs[3], erinToGina, a3ToFrank) // any order gives the canonical one
	e5y := f.build(hs[3], bobToHank)
	f.add(e5x)
	f.add(e5y)
	// Builder Z makes the same entry as X.
	if _, err := f.pool.AddEntry(f.build(hs[3], a3ToFrank, erinToGina)); !errors.Is(err, ledger.ErrKnownEntry) {
		t.Fatal("same bundle on the same parent must be the same entry")
	}
	f.layers(e5x.Hash(f.s), 11, 10, 14)
	f.layers(e5y.Hash(f.s), 14)
	if tip, _, w := f.pool.Tip(); tip != e5x.Hash(f.s) || w.String() != "37888" {
		t.Fatalf("E5x should be selected at 37,888, got %s", w)
	}
	info, _ := f.pool.Info(e5y.Hash(f.s))
	if info.Ledger.String() != "34816" || info.Selected {
		t.Fatalf("E5y ledger %s", info.Ledger)
	}
	// E5y's transaction is still valid after E5x and goes into E6.
	e6 := f.build(e5x.Hash(f.s), bobToHank)
	f.add(e6)
	f.layers(e6.Hash(f.s), 12)
	if _, _, w := f.pool.Tip(); w.String() != "41984" {
		t.Fatalf("weight after E6 %s, want 41984", w)
	}
	if loc, ok := f.pool.Locate(f.tid(bobToHank)); !ok || loc != e6.Hash(f.s) {
		t.Fatal("Locate should find the payment in E6")
	}
}

func TestTiesKeepFirstSeen(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	a := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	b := f.build(f.pool.ID(), f.k.Claim(4, P("bob", 100)))
	f.add(a)
	f.layers(a.Hash(f.s), 5)
	f.add(b)
	if sw := f.layers(b.Hash(f.s), 5); sw != nil {
		t.Fatal("an exact tie must keep the ledger selected first")
	}
	if tip, _, _ := f.pool.Tip(); tip != a.Hash(f.s) {
		t.Fatal("first seen should stay selected")
	}
	if sw := f.layers(b.Hash(f.s), 0); sw == nil || sw.To != b.Hash(f.s) {
		t.Fatal("one more layer should break the tie")
	}
	// The displaced claim is void: the mint is claimed in the new ledger.
	sw := f.layers(a.Hash(f.s), 2)
	if sw == nil || len(sw.Void) != 1 {
		t.Fatalf("switch back: %+v", sw)
	}
}

func TestZeroCreditExtension(t *testing.T) {
	f := newFixture(t, ledger.Config{K: 0})
	e1 := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	if res := f.add(e1); res.Switch != nil {
		t.Fatal("an entry with no credit ties with the empty ledger and is not selected")
	}
	if f.pool.Height() != 0 || !f.pool.Has(e1.Hash(f.s)) {
		t.Fatal("the entry must be kept while it ties, even with K = 0")
	}
	sw := f.layers(e1.Hash(f.s), 3)
	if sw == nil || !sw.Extension() || f.pool.Height() != 1 {
		t.Fatal("a layer should make it selected as an extension")
	}
	e2 := f.build(e1.Hash(f.s), f.k.Claim(5, P("bob", 100)))
	f.add(e2)
	if f.pool.Height() != 1 {
		t.Fatal("zero-credit child must wait")
	}
	f.layers(e2.Hash(f.s), 0)
	if f.pool.Height() != 2 {
		t.Fatal("child with credit should be selected")
	}
}

func TestEntryHashCredit(t *testing.T) {
	p := testutil.ExampleParams()
	p.EntryHashCredit = true
	p.MinEntryStrength = 4
	s := testutil.MustSeries(p)
	k := testutil.NewKeys(s)
	pool := ledger.New(s, 1, ledger.Config{K: 2})
	e, h, err := entry.Build(s, pool.ID(), []*tx.Tx{k.Claim(4, P("a", 100))}, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pool.AddEntry(e)
	if err != nil {
		t.Fatal(err)
	}
	if res.Switch == nil {
		t.Fatal("an entry whose hash earns credit is selected at once")
	}
	if pool.Weight().String() != weightString(1<<h.Strength()) {
		t.Fatalf("weight %s for strength %d", pool.Weight(), h.Strength())
	}
}

func weightString(n uint64) string {
	return func() string {
		var b []byte
		if n == 0 {
			return "0"
		}
		for n > 0 {
			b = append([]byte{byte('0' + n%10)}, b...)
			n /= 10
		}
		return string(b)
	}()
}

func TestErrors(t *testing.T) {
	f := newFixture(t, ledger.Config{K: 4})
	e1 := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	f.add(e1)
	if _, err := f.pool.AddEntry(e1); !errors.Is(err, ledger.ErrKnownEntry) {
		t.Errorf("want ErrKnownEntry, got %v", err)
	}
	orphan := f.build(hash.MustParse("ab00000000000000000000000000000000000000000000000000000000000000"))
	if _, err := f.pool.AddEntry(orphan); !errors.Is(err, ledger.ErrUnknownParent) {
		t.Errorf("want ErrUnknownParent, got %v", err)
	}
	wrongPool := f.build(f.pool.ID(), f.k.Claim(0, P("alice", 100)))
	if _, err := f.pool.AddEntry(wrongPool); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Errorf("want ErrInvalidEntry, got %v", err)
	}
	// A second claim of mint 4 after E1 is invalid.
	again := f.build(e1.Hash(f.s), f.k.Claim(4, P("bob", 100)))
	if _, err := f.pool.AddEntry(again); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Errorf("want ErrInvalidEntry, got %v", err)
	}
	if f.pool.Has(again.Hash(f.s)) {
		t.Error("an invalid entry must not be kept")
	}
	if _, err := f.pool.AddLayer(layer.Layer{Entry: orphan.Hash(f.s)}); !errors.Is(err, ledger.ErrUnknownEntry) {
		t.Errorf("want ErrUnknownEntry, got %v", err)
	}
	l := layerOf(t, f.s, e1.Hash(f.s), 2, 0)
	if _, err := f.pool.AddLayer(l); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.AddLayer(l); !errors.Is(err, ledger.ErrKnownLayer) {
		t.Errorf("want ErrKnownLayer, got %v", err)
	}
	if _, err := f.pool.From(orphan.Hash(f.s)); !errors.Is(err, ledger.ErrNotSelected) {
		t.Errorf("want ErrNotSelected, got %v", err)
	}

	p := testutil.ExampleParams()
	p.MinLayerStrength = 30
	s := testutil.MustSeries(p)
	pool := ledger.New(s, 1, ledger.Config{})
	e, _, _ := entry.Build(s, pool.ID(), nil, 0)
	if _, err := pool.AddEntry(e); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.AddLayer(layer.Layer{Entry: e.Hash(s)}); !errors.Is(err, ledger.ErrWeakLayer) {
		t.Errorf("want ErrWeakLayer, got %v", err)
	}
}

func TestPruneKeepsTopK(t *testing.T) {
	f := newFixture(t, ledger.Config{K: 2})
	e1 := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	f.add(e1)
	f.layers(e1.Hash(f.s), 10)
	// Two rival first entries, kept as the top 2 inactive branches.
	r1 := f.build(f.pool.ID(), f.k.Claim(4, P("r1", 100)))
	f.add(r1)
	f.layers(r1.Hash(f.s), 2)
	r2 := f.build(f.pool.ID(), f.k.Claim(4, P("r2", 100)))
	f.add(r2)
	f.layers(r2.Hash(f.s), 3)
	// A third arrives with no layers. It is the lightest branch, beyond
	// K, but the newest entry is kept until another arrives (D79).
	r3 := f.build(f.pool.ID(), f.k.Claim(4, P("r3", 100)))
	if res := f.add(r3); len(res.Pruned) != 0 || !f.pool.Has(r3.Hash(f.s)) {
		t.Fatalf("the newest entry should be kept, pruned %v", res.Pruned)
	}
	// A fourth, on the selected tip, takes its place as the newest, and
	// the third is dropped.
	e2 := f.build(e1.Hash(f.s), f.k.Claim(5, P("alice", 100)))
	res := f.add(e2)
	if len(res.Pruned) != 1 || res.Pruned[0] != r3.Hash(f.s) || f.pool.Has(r3.Hash(f.s)) {
		t.Fatalf("lightest branch beyond K should be dropped, pruned %v", res.Pruned)
	}
	// The fourth ties with the selected ledger and is kept with the ties.
	if b := f.pool.Branches(); len(b) != 3 || b[0].Tip != e2.Hash(f.s) || b[1].Tip != r2.Hash(f.s) || b[2].Tip != r1.Hash(f.s) {
		t.Fatalf("branches %+v", b)
	}
	// Its layers are thrown out until it is held again.
	if _, err := f.pool.AddLayer(layerOf(t, f.s, r3.Hash(f.s), 1, 0)); !errors.Is(err, ledger.ErrUnknownEntry) {
		t.Fatal("layer on a dropped entry must be thrown out")
	}
	// Lowering K drops the lighter of the two.
	f.pool.SetK(1)
	if f.pool.Has(r1.Hash(f.s)) || !f.pool.Has(r2.Hash(f.s)) {
		t.Fatal("SetK(1) should keep only the heaviest inactive branch")
	}
	// A dropped entry is forgotten, not lost: it can be given again, and
	// shared entries are never dropped with a branch.
	f.pool.SetK(ledger.Unlimited)
	if _, err := f.pool.AddEntry(r1); err != nil {
		t.Fatalf("re-adding a dropped entry: %v", err)
	}
	if !f.pool.Has(e1.Hash(f.s)) {
		t.Fatal("the selected ledger is never pruned")
	}
}

func TestTransferLayers(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	e1 := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	f.add(e1)
	f.layers(e1.Hash(f.s), 8)
	a := f.build(e1.Hash(f.s), f.k.Claim(5, P("bob", 100)))
	b := f.build(e1.Hash(f.s), f.k.Claim(5, P("carol", 100)))
	f.add(a)
	f.add(b)
	f.layers(a.Hash(f.s), 3)
	f.layers(b.Hash(f.s), 4)
	// The shared E1 counts in both ledgers: 256 + 16.
	if tip, _, w := f.pool.Tip(); tip != b.Hash(f.s) || w.String() != "272" {
		t.Fatalf("tip weight %s", w)
	}
	// Layers on E1 still count after switching back and forth.
	f.layers(a.Hash(f.s), 4) // 8 + 16 = 24 > 16
	if tip, _, w := f.pool.Tip(); tip != a.Hash(f.s) || w.String() != "280" {
		t.Fatalf("after switch weight %s", w)
	}
	f.layers(e1.Hash(f.s), 9) // shared: both ledgers gain 512
	if _, _, w := f.pool.Tip(); w.String() != "792" {
		t.Fatalf("after shared layer weight %s", w)
	}
	info, _ := f.pool.Info(b.Hash(f.s))
	if info.Ledger.String() != "784" {
		t.Fatalf("inactive branch weight %s, want 784", info.Ledger)
	}
}

// A heavier rival fetched entry by entry, each followed by its layers, as
// a node catching up does, must not be pruned before its layers arrive,
// even when K inactive branches outweigh its first entry (decision D79).
func TestRivalFetchedEntryByEntryIsKept(t *testing.T) {
	f := newFixture(t, ledger.Config{K: 2})
	e1 := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	f.add(e1)
	f.layers(e1.Hash(f.s), 10)
	// Three siblings on e1: one selected, two inactive and heavy.
	for i, o := range []string{"s1", "s2", "s3"} {
		s := f.build(e1.Hash(f.s), f.k.Claim(5, P(o, 100)))
		f.add(s)
		f.layers(s.Hash(f.s), 2+i)
	}
	// The rival's first entry arrives with no credit yet: the lightest
	// branch, beyond K.
	r1 := f.build(f.pool.ID(), f.k.Claim(4, P("rival", 100)))
	f.add(r1)
	if !f.pool.Has(r1.Hash(f.s)) {
		t.Fatal("the newest entry was pruned before its layers could arrive")
	}
	f.layers(r1.Hash(f.s), 11)
	r2 := f.build(r1.Hash(f.s), f.k.Claim(5, P("rival", 100)))
	f.add(r2)
	f.layers(r2.Hash(f.s), 3)
	if tip, _, _ := f.pool.Tip(); tip != r2.Hash(f.s) {
		t.Fatalf("tip %s, want the rival's %s", tip.Short(), r2.Hash(f.s).Short())
	}
}
