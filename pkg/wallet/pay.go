package wallet

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Leg states.
const (
	// Unsigned: waiting for co-signers' signatures; not sent.
	StateUnsigned = "unsigned"
	// Pending: sent; the node holds it as a pending transaction.
	StatePending = "pending"
	// Selected: in an entry of the selected ledger.
	StateSelected = "selected"
	// Dropped: not held by the node, and its inputs are still unspent: it
	// can be sent again (Resend).
	StateDropped = "dropped"
	// Void: not held, and another transaction spent an input: the payment
	// did not happen (the design's section 6.6).
	StateVoid = "void"
)

// Leg is one transaction of a payment: the part paid from one mint.
type Leg struct {
	Serial uint64     `json:"serial"`
	TID    hash.Hash  `json:"tid"`
	Tx     Hex        `json:"tx"`
	Amount uint64     `json:"amount"`
	Change uint64     `json:"change,omitempty"`
	Inputs []tx.Input `json:"inputs,omitempty"`
	State  string     `json:"state"`
}

// Sent is a payment the wallet made, or a claim.
type Sent struct {
	Series  hash.Hash `json:"series"`
	To      hash.Hash `json:"to"`
	Amount  uint64    `json:"amount"`
	Memo    string    `json:"memo,omitempty"`
	Claim   bool      `json:"claim,omitempty"`
	Created time.Time `json:"created"`
	Legs    []Leg     `json:"legs"`
}

// Sent returns the payments made, oldest first.
func (w *Wallet) Sent() []*Sent { return w.f.Sent }

// open reports whether a leg may still be selected, so its inputs are not
// spent again.
func (l Leg) open() bool { return l.State == StateUnsigned || l.State == StatePending || l.State == "" }

// Spendable returns the portions of the watched mints of s that the wallet
// controls at the node's selected tip, less those an open payment of its
// own spends.
func (w *Wallet) Spendable(s *series.Series, n Node) ([]Spendable, error) {
	owners := w.Owners(s)
	reserved := map[tx.Input]bool{}
	for _, p := range w.f.Sent {
		if p.Series != s.ID() {
			continue
		}
		for _, l := range p.Legs {
			if l.open() {
				for _, in := range l.Inputs {
					reserved[in] = true
				}
			}
		}
	}
	var out []Spendable
	for _, serial := range w.Watched(s.ID()) {
		ps, err := n.Holdings(s.ID(), serial)
		if err != nil {
			return nil, fmt.Errorf("wallet: mint %d: %w", serial, err)
		}
		for _, p := range ps {
			if o, ok := owners[p.Address]; ok && !reserved[p.Input] {
				out = append(out, Spendable{Serial: serial, Portion: p, Owner: o})
			}
		}
	}
	return out, nil
}

// Balance sums the spendable units per watched mint of s.
func (w *Wallet) Balance(s *series.Series, n Node) (map[uint64]uint64, error) {
	sp, err := w.Spendable(s, n)
	if err != nil {
		return nil, err
	}
	out := map[uint64]uint64{}
	for _, serial := range w.Watched(s.ID()) {
		out[serial] = 0
	}
	for _, x := range sp {
		out[x.Serial] += x.Portion.Amount
	}
	return out, nil
}

// PayOptions refine a payment.
type PayOptions struct {
	// Policy is the receiver's policy, from their payment request: when
	// given, the wallet checks it is behind the address and that the
	// series allows it before paying.
	Policy *policy.Policy
	Memo   string
	// Mints limits the payment to these serials.
	Mints []uint64
}

// Pay pays amount units of series s to address to, from the portions the
// wallet controls: one transaction per mint used, change to a fresh
// address each. The wallet must be unlocked. When every input can be
// signed here, each leg is sent at once; otherwise nothing is sent and
// the payment waits, unsigned, for co-signers (see Unsigned). The wallet
// file is not saved; the caller saves it.
func (w *Wallet) Pay(s *series.Series, n Node, to hash.Hash, amount uint64, o PayOptions) (*Sent, *Unsigned, error) {
	if w.seed == nil {
		return nil, nil, ErrLocked
	}
	if amount == 0 {
		return nil, nil, errors.New("wallet: pay at least 1 unit")
	}
	if o.Policy != nil {
		if s.Address(*o.Policy) != to {
			return nil, nil, errors.New("wallet: the policy given is not behind the address")
		}
		if err := CheckPolicy(s, *o.Policy); err != nil {
			return nil, nil, err
		}
	}
	sp, err := w.Spendable(s, n)
	if err != nil {
		return nil, nil, err
	}
	if len(o.Mints) > 0 {
		keep := map[uint64]bool{}
		for _, m := range o.Mints {
			keep[m] = true
		}
		var f []Spendable
		for _, x := range sp {
			if keep[x.Serial] {
				f = append(f, x)
			}
		}
		sp = f
	}
	plans, ok := choose(s, sp, amount)
	if !ok {
		var have uint64
		for _, x := range sp {
			have += x.Portion.Amount
		}
		return nil, nil, fmt.Errorf("%w: %d held, %d asked", ErrNoFunds, have, amount)
	}
	pay := &Sent{Series: s.ID(), To: to, Amount: amount, Memo: o.Memo, Created: now()}
	u := &Unsigned{Series: s.ID()}
	for _, pl := range plans {
		t := &tx.Tx{Mint: s.Mint(pl.serial)}
		for _, in := range pl.inputs {
			t.Inputs = append(t.Inputs, in.Portion.Input)
		}
		t.Outputs = []tx.Output{{Address: to, Amount: pl.pay}}
		change := pl.total - pl.pay
		if change > 0 {
			c, err := w.NewAddress(s, "change")
			if err != nil {
				return nil, nil, err
			}
			t.Outputs = append(t.Outputs, tx.Output{Address: c.Address, Amount: change})
		}
		ul, err := w.sign(s, t, pl.inputs)
		if err != nil {
			return nil, nil, err
		}
		tid := t.TID(s.Hash())
		pay.Legs = append(pay.Legs, Leg{Serial: pl.serial, TID: tid, Amount: pl.pay, Change: change, Inputs: t.Inputs, State: StateUnsigned})
		u.Legs = append(u.Legs, ul)
	}
	w.f.Sent = append(w.f.Sent, pay)
	if !u.Complete() {
		for i := range pay.Legs {
			pay.Legs[i].Tx = u.Legs[i].Tx
		}
		return pay, u, nil
	}
	return pay, nil, w.send(s, n, pay, u)
}

// send finishes every leg of u, records it in pay and submits it.
func (w *Wallet) send(s *series.Series, n Node, pay *Sent, u *Unsigned) error {
	var txs []*tx.Tx
	for i := range u.Legs {
		t, err := u.Legs[i].Finish()
		if err != nil {
			return err
		}
		txs = append(txs, t)
	}
	for i, t := range txs {
		pay.Legs[i].Tx = t.Encode()
		pay.Legs[i].State = StatePending
		if err := n.Submit(t); err != nil {
			pay.Legs[i].State = StateDropped
			return fmt.Errorf("wallet: sending leg %d (mint %d): %w", i+1, pay.Legs[i].Serial, err)
		}
	}
	return nil
}

// sign makes the leg's witnesses: complete ones for the inputs this wallet
// can sign alone, and the signatures it can add for shared policies.
func (w *Wallet) sign(s *series.Series, t *tx.Tx, in []Spendable) (UnsignedLeg, error) {
	tid := t.TID(s.Hash())
	ul := UnsignedLeg{Serial: t.Mint.Serial}
	for _, x := range in {
		pw := PartialWitness{Policy: x.Owner.Policy.Encode()}
		ul.Inputs = append(ul.Inputs, pw)
	}
	t.Witnesses = make([]policy.Witness, len(t.Inputs))
	for i := range t.Witnesses {
		t.Witnesses[i] = policy.Witness{Policy: in[i].Owner.Policy}
	}
	ul.Tx = t.Encode()
	if _, err := w.addSigs(s, &ul, tid); err != nil {
		return ul, err
	}
	return ul, nil
}

// addSigs signs, for every input of ul, with each key of its policy the
// wallet holds. It returns how many signatures it added.
func (w *Wallet) addSigs(s *series.Series, ul *UnsignedLeg, tid hash.Hash) (int, error) {
	if w.seed == nil {
		return 0, ErrLocked
	}
	msg := policy.Message(tid)
	added := 0
	for i := range ul.Inputs {
		p, err := ul.Inputs[i].policy()
		if err != nil {
			return added, err
		}
		tp, err := policy.DecodeThresholdParams(policy.Default(), p.Params)
		if err != nil {
			return added, err
		}
		for ki, k := range tp.Keys {
			if ul.Inputs[i].has(uint16(ki)) {
				continue
			}
			idx, ok := w.keyIndex(k)
			if !ok {
				continue
			}
			sg, err := w.signer(idx)
			if err != nil {
				return added, err
			}
			ul.Inputs[i].Sigs = append(ul.Inputs[i].Sigs, SigHex{Index: uint16(ki), Sig: sg.Sign(msg)})
			added++
		}
	}
	return added, nil
}

// keyIndex finds the wallet key behind a policy key.
func (w *Wallet) keyIndex(k policy.Key) (uint32, bool) {
	if k.Alg != w.KeyAlg() {
		return 0, false
	}
	for _, ki := range w.f.Keys {
		if bytes.Equal(ki.Public, k.Public) {
			return ki.Index, true
		}
	}
	return 0, false
}

// Claim claims the whole supply of mint serial of s to a fresh address,
// sends it, and watches the mint. A claim is valid only while the mint is
// unclaimed in the ledger.
func (w *Wallet) Claim(s *series.Series, n Node, serial uint64) (*Sent, error) {
	if serial >= s.Mints() {
		return nil, fmt.Errorf("wallet: serial %d out of range", serial)
	}
	if w.seed == nil {
		return nil, ErrLocked
	}
	a, err := w.NewAddress(s, fmt.Sprintf("claim of mint %d", serial))
	if err != nil {
		return nil, err
	}
	sg, err := w.signer(*a.Key)
	if err != nil {
		return nil, err
	}
	t := &tx.Tx{Mint: s.Mint(serial), Outputs: []tx.Output{{Address: a.Address, Amount: s.Params().Supply}}}
	tid := t.TID(s.Hash())
	t.Witnesses = []policy.Witness{policy.SingleKeyWitness(sg, tid)}
	if err := w.WatchMints(s, serial); err != nil {
		return nil, err
	}
	pay := &Sent{Series: s.ID(), To: a.Address, Amount: s.Params().Supply, Claim: true, Created: now(),
		Legs: []Leg{{Serial: serial, TID: tid, Tx: t.Encode(), Amount: s.Params().Supply, State: StatePending}}}
	w.f.Sent = append(w.f.Sent, pay)
	if err := n.Submit(t); err != nil {
		pay.Legs[0].State = StateDropped
		return pay, err
	}
	return pay, nil
}

// Unsigned is a payment waiting for signatures: what a payer passes to
// co-signers and back. Each leg holds its transaction (witnesses without
// data) and, per input, the signatures gathered so far.
type Unsigned struct {
	Series hash.Hash     `json:"series"`
	Legs   []UnsignedLeg `json:"legs"`
}

// UnsignedLeg is one transaction waiting for signatures.
type UnsignedLeg struct {
	Serial uint64           `json:"serial"`
	Tx     Hex              `json:"tx"`
	Inputs []PartialWitness `json:"inputs"`
}

// PartialWitness is the policy of one input and the signatures gathered.
type PartialWitness struct {
	Policy Hex      `json:"policy"`
	Sigs   []SigHex `json:"sigs,omitempty"`
}

// SigHex is one signature by the key at Index of the policy.
type SigHex struct {
	Index uint16 `json:"index"`
	Sig   Hex    `json:"sig"`
}

func (p PartialWitness) policy() (policy.Policy, error) {
	return decodePolicy(p.Policy)
}

func (p PartialWitness) has(i uint16) bool {
	for _, s := range p.Sigs {
		if s.Index == i {
			return true
		}
	}
	return false
}

func (p PartialWitness) need() (int, error) {
	pol, err := p.policy()
	if err != nil {
		return 0, err
	}
	tp, err := policy.DecodeThresholdParams(policy.Default(), pol.Params)
	if err != nil {
		return 0, err
	}
	return int(tp.K) - len(p.Sigs), nil
}

// Complete reports whether every input of every leg has its signatures.
func (u *Unsigned) Complete() bool {
	for _, l := range u.Legs {
		for _, in := range l.Inputs {
			if n, err := in.need(); err != nil || n > 0 {
				return false
			}
		}
	}
	return true
}

// Missing returns, per leg and input, how many signatures are still
// needed.
func (u *Unsigned) Missing() [][]int {
	out := make([][]int, len(u.Legs))
	for i, l := range u.Legs {
		for _, in := range l.Inputs {
			n, _ := in.need()
			if n < 0 {
				n = 0
			}
			out[i] = append(out[i], n)
		}
	}
	return out
}

// Finish builds the signed transaction: each witness takes K of the
// gathered signatures in key order.
func (l UnsignedLeg) Finish() (*tx.Tx, error) {
	t, err := tx.Decode(l.Tx)
	if err != nil {
		return nil, err
	}
	if len(l.Inputs) != len(t.Inputs) {
		return nil, errors.New("wallet: unsigned leg has the wrong number of inputs")
	}
	for i, in := range l.Inputs {
		pol, err := in.policy()
		if err != nil {
			return nil, err
		}
		tp, err := policy.DecodeThresholdParams(policy.Default(), pol.Params)
		if err != nil {
			return nil, err
		}
		sigs := append([]SigHex(nil), in.Sigs...)
		sort.Slice(sigs, func(a, b int) bool { return sigs[a].Index < sigs[b].Index })
		if len(sigs) < int(tp.K) {
			return nil, fmt.Errorf("wallet: input %d needs %d more signatures", i, int(tp.K)-len(sigs))
		}
		var d policy.ThresholdData
		for _, sg := range sigs[:tp.K] {
			d.Sigs = append(d.Sigs, policy.Sig{Index: sg.Index, Signature: sg.Sig})
		}
		t.Witnesses[i] = policy.Witness{Policy: pol, Data: d.Encode()}
	}
	return t, nil
}

// Cosign adds this wallet's signatures to u, for every input whose policy
// holds one of its keys, and returns how many it added. The wallet must be
// unlocked. A co-signer should read the legs (what they pay, to whom)
// before signing.
func (w *Wallet) Cosign(s *series.Series, u *Unsigned) (int, error) {
	if u.Series != s.ID() {
		return 0, errors.New("wallet: the payment is of another series")
	}
	total := 0
	for i := range u.Legs {
		t, err := tx.Decode(u.Legs[i].Tx)
		if err != nil {
			return total, err
		}
		n, err := w.addSigs(s, &u.Legs[i], t.TID(s.Hash()))
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// Submit sends a payment whose signatures are complete. It must be one
// this wallet made (Pay recorded it), so its inputs stay reserved.
func (w *Wallet) Submit(s *series.Series, n Node, u *Unsigned) (*Sent, error) {
	if !u.Complete() {
		return nil, errors.New("wallet: signatures are missing")
	}
	h := s.Hash()
	for _, p := range w.f.Sent {
		if p.Series != s.ID() || len(p.Legs) != len(u.Legs) {
			continue
		}
		match := true
		for i, l := range u.Legs {
			t, err := tx.Decode(l.Tx)
			if err != nil {
				return nil, err
			}
			if t.TID(h) != p.Legs[i].TID {
				match = false
				break
			}
		}
		if match {
			return p, w.send(s, n, p, u)
		}
	}
	return nil, errors.New("wallet: not a payment this wallet made")
}
