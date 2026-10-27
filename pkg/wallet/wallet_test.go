package wallet_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

var exS = testutil.ExampleSeries()

// engineNode is a wallet.Node over an engine, standing in for a full node.
type engineNode struct {
	e    *engine.Engine
	s    *series.Series
	hide map[hash.Hash]bool // tids to report as not held
}

func newNode(t *testing.T) *engineNode { return newNodeFor(t, exS) }

func newNodeFor(t *testing.T, s *series.Series) *engineNode {
	t.Helper()
	e := engine.New(engine.Config{Ledger: ledger.Config{K: ledger.Unlimited}})
	e.AddSeries(s)
	must(t, e.FollowSeries(s.ID()))
	return &engineNode{e: e, s: s, hide: map[hash.Hash]bool{}}
}

func (n *engineNode) Holdings(s hash.Hash, serial uint64) ([]wallet.Portion, error) {
	hs, err := n.e.Holdings(s, serial)
	var out []wallet.Portion
	for _, h := range hs {
		out = append(out, wallet.Portion{Input: h.Input, Address: h.Portion.Address, Amount: h.Portion.Amount})
	}
	return out, err
}

func (n *engineNode) Payment(s hash.Hash, serial uint64, tid hash.Hash) (wallet.Status, error) {
	if n.hide[tid] {
		return wallet.Status{}, nil
	}
	sr, _ := n.e.Series(s)
	p, err := n.e.Payment(engine.PoolKey{Series: s, Index: sr.PoolOf(serial)}, tid)
	st := wallet.Status{Pending: p.Pending, Selected: p.Selected, Entry: p.Entry, From: p.From, Tx: p.Tx}
	for _, r := range p.Rivals {
		st.Rivals = append(st.Rivals, wallet.Rival{Conflict: r.Conflict, Entry: r.Entry, After: r.After})
	}
	return st, err
}

func (n *engineNode) Submit(t *tx.Tx) error {
	delete(n.hide, t.TID(n.s.Hash()))
	_, err := n.e.HandleTx(t, engine.Local)
	if errors.Is(err, engine.ErrSeen) {
		return nil // already held
	}
	return err
}

// mine builds an entry of pool from the pending transactions and adds a
// layer.
func (n *engineNode) mine(t *testing.T, pool uint64) hash.Hash {
	t.Helper()
	k := engine.PoolKey{Series: n.s.ID(), Index: pool}
	_, res, err := n.e.Build(k, 0)
	must(t, err)
	_, _, err = n.e.Work(k, res.Hash, 0, 4000)
	must(t, err)
	return res.Hash
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newWallet(t *testing.T, name string, o wallet.Options) *wallet.Wallet {
	t.Helper()
	if o.Iterations == 0 {
		o.Iterations = 1
	}
	w, err := wallet.Create(filepath.Join(t.TempDir(), name+".json"), "pass "+name, o)
	must(t, err)
	return w
}

func TestCreateOpenUnlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "w.json")
	seed := make([]byte, wallet.SeedSize)
	seed[0] = 7
	w, err := wallet.Create(path, "secret", wallet.Options{Seed: seed, Iterations: 10, Gap: 3})
	must(t, err)
	if _, err := wallet.Create(path, "secret", wallet.Options{}); !errors.Is(err, wallet.ErrExists) {
		t.Fatalf("overwrote a wallet: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if len(w.Keys()) != 3 {
		t.Fatalf("%d keys, gap 3", len(w.Keys()))
	}
	a, err := w.NewAddress(exS, "shop")
	must(t, err)
	must(t, w.Save())
	b, _ := os.ReadFile(path)
	var raw map[string]any
	must(t, json.Unmarshal(b, &raw))
	if _, ok := raw["seed"]; ok {
		t.Fatal("the seed is in the clear")
	}

	w2, err := wallet.Open(path)
	must(t, err)
	if w2.Unlocked() {
		t.Fatal("opened unlocked")
	}
	if _, err := w2.Seed(); !errors.Is(err, wallet.ErrLocked) {
		t.Fatal("seed readable while locked")
	}
	if err := w2.Unlock("wrong"); !errors.Is(err, wallet.ErrPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	must(t, w2.Unlock("secret"))
	got, _ := w2.Seed()
	if string(got) != string(seed) {
		t.Fatal("seed changed")
	}
	// The first address, the gap topped up past it.
	if o := w2.Owners(exS)[a.Address]; o.Key == nil || *o.Key != 0 || o.Label != "shop" || len(w2.Keys()) != 4 {
		t.Fatalf("owner %+v, %d keys", o, len(w2.Keys()))
	}
	// A wallet restored from the seed derives the same keys.
	r := newWallet(t, "restored", wallet.Options{Seed: seed, Gap: 3})
	if string(r.Keys()[0].Public) != string(w.Keys()[0].Public) {
		t.Fatal("restored keys differ")
	}
	// A new passphrase.
	must(t, w2.Rekey("other"))
	must(t, w2.Save())
	w3, _ := wallet.Open(path)
	must(t, w3.Unlock("other"))
}

func TestLockedWalletGivesOutTheGap(t *testing.T) {
	w := newWallet(t, "a", wallet.Options{Gap: 2})
	w.Lock()
	for i := 0; i < 2; i++ {
		if _, err := w.NewAddress(exS, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.NewAddress(exS, ""); !errors.Is(err, wallet.ErrLocked) {
		t.Fatalf("beyond the gap while locked: %v", err)
	}
}

// Alice claims a mint. Bob asks for 30 and Alice pays it with
// change to a fresh address; Bob reads the weight from the entry until it
// reaches his threshold.
func TestClaimPayAndReceive(t *testing.T) {
	n := newNode(t)
	alice, bob := newWallet(t, "alice", wallet.Options{}), newWallet(t, "bob", wallet.Options{})
	c, err := alice.Claim(exS, n, 4)
	must(t, err)
	n.mine(t, 1)
	ls, err := alice.Refresh(exS, n, c)
	must(t, err)
	if ls[0].State != wallet.StateSelected || ls[0].From.IsZero() {
		t.Fatalf("claim %+v", ls)
	}
	bal, _ := alice.Balance(exS, n)
	if bal[4] != 100 {
		t.Fatalf("balance %v", bal)
	}

	must(t, bob.WatchMints(exS, 4))
	req, err := bob.Request(exS, 30, "coffee")
	must(t, err)
	if _, err := req.Check(exS); err != nil {
		t.Fatal(err)
	}
	p, u, err := alice.PayRequest(exS, n, req)
	must(t, err)
	if u != nil || len(p.Legs) != 1 || p.Legs[0].Change != 70 {
		t.Fatalf("payment %+v %+v", p, u)
	}
	// The spent portion is reserved until the payment settles or fails.
	if bal, _ := alice.Balance(exS, n); bal[4] != 0 {
		t.Fatalf("balance while pending %v", bal)
	}
	e, _ := bob.Find(req.Address)
	e.Tell(wallet.TxRef{Serial: 4, TID: p.Legs[0].TID})
	rc, err := bob.Receive(exS, n, e)
	must(t, err)
	if rc.Received != 30 || !rc.Complete || rc.Legs[0].State != wallet.StatePending || rc.Settled(weight.Zero) {
		t.Fatalf("receipt while pending %+v", rc)
	}

	eh := n.mine(t, 1)
	rc, err = bob.Receive(exS, n, e)
	must(t, err)
	if rc.Legs[0].State != wallet.StateSelected || *rc.Legs[0].Entry != eh {
		t.Fatalf("receipt %+v", rc)
	}
	from := rc.Legs[0].From
	if !rc.Settled(from) || rc.Settled(from.Add(weight.FromUint64(1))) {
		t.Fatalf("settled at from(e) = %s", from)
	}
	if bal, _ := alice.Balance(exS, n); bal[4] != 70 {
		t.Fatalf("change %v", bal)
	}
	// Without being told the tid, Bob still finds the selected payment.
	r2, err := bob.Request(exS, 5, "")
	must(t, err)
	p2, _, err := alice.PayRequest(exS, n, r2)
	must(t, err)
	n.mine(t, 1)
	e2, _ := bob.Find(r2.Address)
	rc2, err := bob.Receive(exS, n, e2)
	must(t, err)
	if rc2.Received != 5 || len(rc2.Legs) != 1 || rc2.Legs[0].TID != p2.Legs[0].TID {
		t.Fatalf("found receipt %+v", rc2)
	}
}

// Wallets of every key algorithm claim, pay and receive, in series of
// every hash, each series allowing only the algorithms in use.
func TestEveryKeyAlgorithm(t *testing.T) {
	algs := policy.Default().Algs()
	ids := hash.IDs()
	for i, a := range algs {
		payee := algs[(i+1)%len(algs)].ID()
		t.Run(a.Name(), func(t *testing.T) {
			p := testutil.ExampleParams()
			p.HashID = ids[i%len(ids)]
			p.KeyAlgs = []policy.AlgID{a.ID(), payee}
			if payee < a.ID() {
				p.KeyAlgs[0], p.KeyAlgs[1] = payee, a.ID()
			}
			s := testutil.MustSeries(p)
			n := newNodeFor(t, s)
			alice := newWallet(t, "alice", wallet.Options{Alg: a.ID()})
			bob := newWallet(t, "bob", wallet.Options{Alg: payee})
			if alice.KeyAlg() != a.ID() {
				t.Fatalf("key algorithm %d", alice.KeyAlg())
			}
			_, err := alice.Claim(s, n, 4)
			must(t, err)
			n.mine(t, 1)
			must(t, bob.WatchMints(s, 4))
			req, err := bob.Request(s, 30, "")
			must(t, err)
			_, _, err = alice.PayRequest(s, n, req)
			must(t, err)
			n.mine(t, 1)
			e, _ := bob.Find(req.Address)
			rc, err := bob.Receive(s, n, e)
			must(t, err)
			if rc.Received != 30 || rc.Legs[0].State != wallet.StateSelected {
				t.Fatalf("receipt %+v", rc)
			}
			// Reopened, the wallet derives the same keys.
			again, err := wallet.Open(alice.Path())
			must(t, err)
			must(t, again.Unlock("pass alice"))
			if again.KeyAlg() != a.ID() || string(again.Keys()[0].Public) != string(alice.Keys()[0].Public) {
				t.Fatal("reopened wallet differs")
			}
		})
	}
	// A series that does not allow the wallet's algorithm refuses its
	// addresses.
	w := newWallet(t, "k1", wallet.Options{Alg: policy.AlgECDSASecp256k1})
	if _, err := w.Request(exS, 1, ""); err == nil {
		t.Fatal("request for an algorithm the series does not allow")
	}
	if _, err := wallet.Create(filepath.Join(t.TempDir(), "x.json"), "p", wallet.Options{Alg: 999, Iterations: 1}); err == nil {
		t.Fatal("unknown algorithm accepted")
	}
}

// A payment larger than any one mint holds takes a leg from each of
// several mints of one pool.
func TestPaymentAcrossMints(t *testing.T) {
	n := newNode(t)
	alice, bob := newWallet(t, "alice", wallet.Options{}), newWallet(t, "bob", wallet.Options{})
	for _, m := range []uint64{4, 5, 2} {
		_, err := alice.Claim(exS, n, m)
		must(t, err)
	}
	n.mine(t, 1)
	n.mine(t, 0)
	to, _ := bob.NewAddress(exS, "")
	p, _, err := alice.Pay(exS, n, to.Address, 150, wallet.PayOptions{})
	must(t, err)
	if len(p.Legs) != 2 || exS.PoolOf(p.Legs[0].Serial) != 1 || exS.PoolOf(p.Legs[1].Serial) != 1 ||
		p.Legs[0].Amount+p.Legs[1].Amount != 150 {
		t.Fatalf("legs %+v", p.Legs)
	}
	// Restricted to mint 2, too little.
	if _, _, err := alice.Pay(exS, n, to.Address, 150, wallet.PayOptions{Mints: []uint64{2}}); !errors.Is(err, wallet.ErrNoFunds) {
		t.Fatalf("restricted payment: %v", err)
	}
	n.mine(t, 1)
	ls, err := alice.Refresh(exS, n, p)
	must(t, err)
	if !wallet.Settled(ls, weight.Zero) {
		t.Fatalf("legs %+v", ls)
	}
}

// A 2-of-2 policy between Alice and Carol: Alice's payment from it waits
// for Carol's signature, then goes through.
func TestSharedPolicyCosigning(t *testing.T) {
	n := newNode(t)
	alice, carol := newWallet(t, "alice", wallet.Options{}), newWallet(t, "carol", wallet.Options{})
	ck := carol.Keys()[0]
	joint, err := alice.AddPolicy("joint", 2, []wallet.PolicyKey{{}, {Public: ck.Public}}, "")
	must(t, err)
	jp := joint.Params().Policy()
	if err := wallet.CheckPolicy(exS, jp); err != nil {
		t.Fatal(err)
	}
	ja := exS.Address(jp)
	_, err = alice.Claim(exS, n, 4)
	must(t, err)
	n.mine(t, 1)
	_, _, err = alice.Pay(exS, n, ja, 60, wallet.PayOptions{Policy: &jp})
	must(t, err)
	n.mine(t, 1)

	bob := newWallet(t, "bob", wallet.Options{})
	to, _ := bob.NewAddress(exS, "")
	// Alice's own change is 40, so 50 needs the joint portion.
	p, u, err := alice.Pay(exS, n, to.Address, 50, wallet.PayOptions{})
	must(t, err)
	if u == nil || u.Complete() || p.Legs[0].State != wallet.StateUnsigned {
		t.Fatalf("expected an unsigned payment, got %+v %+v", p, u)
	}
	if m := u.Missing(); m[0][0] != 1 {
		t.Fatalf("missing %v", m)
	}
	if _, err := alice.Submit(exS, n, u); err == nil {
		t.Fatal("submitted without Carol")
	}
	// Carol signs, through the file a payer would hand over.
	b, _ := json.Marshal(u)
	var cu wallet.Unsigned
	must(t, json.Unmarshal(b, &cu))
	added, err := carol.Cosign(exS, &cu)
	must(t, err)
	if added != 1 || !cu.Complete() {
		t.Fatalf("carol added %d", added)
	}
	if _, err := alice.Submit(exS, n, &cu); err != nil {
		t.Fatal(err)
	}
	n.mine(t, 1)
	ls, err := alice.Refresh(exS, n, p)
	must(t, err)
	if ls[0].State != wallet.StateSelected {
		t.Fatalf("shared payment %+v", ls)
	}
}

// A copy of a wallet on another device spends the same portion: the
// payment from the first copy goes void once the other is selected. A
// payment the node forgot is dropped and can be sent again.
func TestDroppedVoidAndRivals(t *testing.T) {
	n := newNode(t)
	seed := make([]byte, wallet.SeedSize)
	seed[1] = 9
	a := newWallet(t, "a", wallet.Options{Seed: seed})
	c, err := a.Claim(exS, n, 4)
	must(t, err)
	n.mine(t, 1)
	to := testutil.NewKeys(exS).Addr("shop")
	p, _, err := a.Pay(exS, n, to, 10, wallet.PayOptions{})
	must(t, err)

	// Dropped: the node no longer holds it; the input is still unspent.
	n.hide[p.Legs[0].TID] = true
	ls, err := a.Refresh(exS, n, p)
	must(t, err)
	if ls[0].State != wallet.StateDropped {
		t.Fatalf("hidden %+v", ls)
	}
	if bal, _ := a.Balance(exS, n); bal[4] != 100 {
		t.Fatalf("a dropped payment still reserves its input: %v", bal)
	}
	if k, err := a.Resend(exS, n, p); err != nil || k != 1 {
		t.Fatalf("resend %d %v", k, err)
	}

	// A rival: the copy spends the same portion in a sibling entry.
	b := newWallet(t, "b", wallet.Options{Seed: seed})
	must(t, b.WatchMints(exS, 4))
	other := testutil.NewKeys(exS).Addr("thief")
	n.mine(t, 1) // a's payment is selected
	// The copy knows nothing of it and spends the claim portion again, in
	// an entry on the claim's entry: a sibling of a's.
	claimEntry := mustClaimEntry(t, n, c)
	rival := &tx.Tx{Mint: exS.Mint(4), Inputs: p.Legs[0].Inputs, Outputs: []tx.Output{{Address: other, Amount: 100}}}
	signAsCopy(t, b, rival)
	en, h, err := entry.Build(exS, claimEntry, []*tx.Tx{rival}, 0)
	must(t, err)
	k := engine.PoolKey{Series: exS.ID(), Index: 1}
	_, err = n.e.HandleEntry(k, en, engine.Pushed)
	must(t, err)
	ls, err = a.Refresh(exS, n, p)
	must(t, err)
	if ls[0].Rivals != 1 || ls[0].Rival == nil || !ls[0].Ahead {
		t.Fatalf("rival not seen: %+v", ls[0])
	}
	// Enough work on the rival: it is selected, and the payment is void.
	for i := 0; i < 8; i++ {
		_, _, err := n.e.Work(k, h, uint64(i)<<32, 20000)
		must(t, err)
		if st, _ := n.e.Status(k); st.Tip == h {
			break
		}
	}
	if st, _ := n.e.Status(k); st.Tip != h {
		t.Fatal("the rival did not overtake in the work tried")
	}
	ls, err = a.Refresh(exS, n, p)
	must(t, err)
	if ls[0].State != wallet.StateVoid {
		t.Fatalf("after the rival won: %+v", ls)
	}
}

func mustClaimEntry(t *testing.T, n *engineNode, c *wallet.Sent) hash.Hash {
	t.Helper()
	st, err := n.e.Payment(engine.PoolKey{Series: exS.ID(), Index: 1}, c.Legs[0].TID)
	must(t, err)
	if !st.Selected {
		t.Fatal("claim not selected")
	}
	return st.Entry
}

// signAsCopy signs a spend of the claim portion with the copy's first
// key, which owns it.
func signAsCopy(t *testing.T, w *wallet.Wallet, rt *tx.Tx) {
	t.Helper()
	pol := policy.SingleKey(policy.AlgEd25519, w.Keys()[0].Public)
	rt.Witnesses = []policy.Witness{{Policy: pol}}
	u := &wallet.Unsigned{Series: exS.ID(), Legs: []wallet.UnsignedLeg{
		{Serial: 4, Tx: rt.Encode(), Inputs: []wallet.PartialWitness{{Policy: pol.Encode()}}},
	}}
	if _, err := w.Cosign(exS, u); err != nil {
		t.Fatal(err)
	}
	ft, err := u.Legs[0].Finish()
	must(t, err)
	rt.Witnesses = ft.Witnesses
}

func TestRequestChecks(t *testing.T) {
	w := newWallet(t, "w", wallet.Options{})
	r, err := w.Request(exS, 10, "")
	must(t, err)
	if _, err := r.Check(exS); err != nil {
		t.Fatal(err)
	}
	// The policy of another address.
	other, _ := w.NewAddress(exS, "")
	bad := r
	bad.Policy = other.Policy.Encode()
	if _, err := bad.Check(exS); err == nil {
		t.Fatal("a policy not behind the address passed")
	}
	// A type the wallet does not recognize.
	odd := policy.Policy{Type: 99, Params: []byte{1}}
	unk := wallet.Request{Series: exS.ID(), Address: exS.Address(odd), Amount: 1, Policy: odd.Encode()}
	if _, err := unk.Check(exS); err == nil {
		t.Fatal("an unknown policy type passed")
	}
	other2 := testutil.MustSeries(func() series.Params { p := testutil.ExampleParams(); p.Mints = 16; return p }())
	if _, err := r.Check(other2); err == nil {
		t.Fatal("a request for another series passed")
	}
	if _, err := w.Request(exS, 0, ""); err == nil {
		t.Fatal("a request for nothing")
	}
}
