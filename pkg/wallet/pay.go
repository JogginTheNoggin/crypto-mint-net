package wallet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/keepjson"
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
	// Change, when set, receives the change of every leg. A watch-only
	// wallet, which cannot derive a fresh address, sends change back to
	// the leg's first input's address unless Change is set (D193).
	Change hash.Hash
}

// Pay pays amount units of series s to address to, from the portions the
// wallet controls: one transaction per mint used, change to a fresh
// address each. The wallet must be unlocked. When every input can be
// signed here, each leg is sent at once; otherwise nothing is sent and
// the payment waits, unsigned, for co-signers (see Unsigned). The wallet
// file is not saved; the caller saves it.
func (w *Wallet) Pay(s *series.Series, n Node, to hash.Hash, amount uint64, o PayOptions) (*Sent, *Unsigned, error) {
	if w.seed == nil && !w.f.WatchOnly {
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
	u := newUnsigned(s)
	for _, pl := range plans {
		t := &tx.Tx{Mint: s.Mint(pl.serial)}
		for _, in := range pl.inputs {
			t.Inputs = append(t.Inputs, in.Portion.Input)
		}
		t.Outputs = []tx.Output{{Address: to, Amount: pl.pay}}
		change := pl.total - pl.pay
		if change > 0 {
			to := o.Change
			switch {
			case !to.IsZero():
			case w.seed == nil: // watch-only
				to = pl.inputs[0].Owner.Address
			default:
				c, err := w.NewAddress(s, "change")
				if err != nil {
					return nil, nil, err
				}
				to = c.Address
			}
			t.Outputs = append(t.Outputs, tx.Output{Address: to, Amount: change})
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
	if w.seed == nil { // watch-only: every signature comes from elsewhere
		return ul, nil
	}
	if _, err := w.addSigs(s, &ul, tid); err != nil {
		return ul, err
	}
	return ul, nil
}

// addSigs signs, for every input of ul, with each key of its policy the
// wallet holds. It returns how many signatures it added.
func (w *Wallet) addSigs(s *series.Series, ul *UnsignedLeg, tid hash.Hash) (int, error) {
	if w.seed == nil {
		return 0, w.locked()
	}
	msg := policy.Message(tid)
	added := 0
	// Every input of a leg signs the same message, so a key that owns
	// several of them signs once and gives each the same signature
	// (ORANGE-045).
	sigs := map[uint32][]byte{}
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
			sig, ok := sigs[idx]
			if !ok {
				sg, err := w.signer(idx)
				if err != nil {
					return added, err
				}
				sig = sg.Sign(msg)
				sigs[idx] = sig
			}
			ul.Inputs[i].Sigs = append(ul.Inputs[i].Sigs, SigHex{Index: uint16(ki), Sig: sig})
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
	if err := tx.CheckMint(s, s.Mint(serial)); err != nil {
		return nil, fmt.Errorf("wallet: serial %d: %w", serial, err)
	}
	if w.seed == nil {
		return nil, w.locked()
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

// ClaimUnsigned prepares a claim of mint serial of s to the address of the
// wallet's policy named name, with no signature: a watch-only wallet's
// claim, for poo sign to sign where the key is (GREEN-060). The claim's
// one witness is for its one output, so the unsigned leg lists one
// policy, as a payment lists one per input. It records the claim, so
// Submit takes the signed file, and watches the mint.
func (w *Wallet) ClaimUnsigned(s *series.Series, serial uint64, name string) (*Sent, *Unsigned, error) {
	if err := tx.CheckMint(s, s.Mint(serial)); err != nil {
		return nil, nil, fmt.Errorf("wallet: serial %d: %w", serial, err)
	}
	var pi *PolicyInfo
	for i := range w.f.Policies {
		if w.f.Policies[i].Name == name {
			pi = &w.f.Policies[i]
		}
	}
	if pi == nil {
		return nil, nil, fmt.Errorf("wallet: no policy %q (poo wallet policy -name %s -k 1 -key ALG:PUBLIC)", name, name)
	}
	p := pi.Params().Policy()
	if err := CheckPolicy(s, p); err != nil {
		return nil, nil, err
	}
	addr := s.Address(p)
	t := &tx.Tx{Mint: s.Mint(serial), Outputs: []tx.Output{{Address: addr, Amount: s.Params().Supply}}}
	t.Witnesses = []policy.Witness{{Policy: p}}
	tid := t.TID(s.Hash())
	if err := w.WatchMints(s, serial); err != nil {
		return nil, nil, err
	}
	pay := &Sent{Series: s.ID(), To: addr, Amount: s.Params().Supply, Claim: true, Created: now(),
		Legs: []Leg{{Serial: serial, TID: tid, Tx: t.Encode(), Amount: s.Params().Supply, State: StateUnsigned}}}
	w.f.Sent = append(w.f.Sent, pay)
	u := newUnsigned(s)
	u.Legs = []UnsignedLeg{{Serial: serial, Tx: t.Encode(), Inputs: []PartialWitness{{Policy: p.Encode()}}}}
	return pay, u, nil
}

// witnessCount is how many witnesses t needs: one per input, or for a
// claim one per output (tx.CheckForm). An unsigned leg lists a partial
// witness for each.
func witnessCount(t *tx.Tx) int {
	if t.IsClaim() {
		return len(t.Outputs)
	}
	return len(t.Inputs)
}

// Unsigned is a payment waiting for signatures: what a payer passes to
// co-signers and back. Each leg holds its transaction (witnesses without
// data) and, per input, the signatures gathered so far.
type Unsigned struct {
	// Kind and Format name the file (W-23, D195). A file with neither is
	// format 1, as written before they were added.
	Kind   string    `json:"kind,omitempty"`
	Format int       `json:"format,omitempty"`
	Series hash.Hash `json:"series"`
	// Hash names the series' hash function, so a signer with no node and
	// no series file can work out each leg's transaction id (D193).
	Hash string `json:"hash,omitempty"`
	// Variable marks a variable-supply series, whose transactions carry a
	// part; a signer with no series needs it to read them (RED-105).
	Variable bool          `json:"variable,omitempty"`
	Legs     []UnsignedLeg `json:"legs"`
	// kept holds the fields a later release wrote that this one does not
	// know, written back unchanged by poo sign and cosign (W-23).
	kept keepjson.Kept
}

// PaymentFileKind and PaymentFormat name the file poo wallet pay -out
// writes and poo sign reads (W-23, D195). A field an older signer must
// not ignore raises PaymentFormat (W-25).
const (
	PaymentFileKind = brand.KindPrefix + "payment"
	PaymentFormat   = 1
)

// newUnsigned starts a payment file for series s.
func newUnsigned(s *series.Series) *Unsigned {
	return &Unsigned{Kind: PaymentFileKind, Format: PaymentFormat, Series: s.ID(), Hash: s.Params().HashID.Name(), Variable: s.IsVariable()}
}

// UnmarshalJSON reads a payment file: it refuses another kind of file and
// a format this release does not know, and keeps the fields it does not
// know.
func (u *Unsigned) UnmarshalJSON(b []byte) error {
	type plain Unsigned
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	switch {
	case p.Kind != "" && p.Kind != PaymentFileKind:
		return fmt.Errorf("a %q file, not a payment (%s)", p.Kind, PaymentFileKind)
	case p.Format > PaymentFormat:
		return fmt.Errorf("the payment file is format %d, from a later release; this release reads format %d, so sign it with the release that wrote it or a later one", p.Format, PaymentFormat)
	case p.Format < 0:
		return fmt.Errorf("the payment file has format %d", p.Format)
	}
	kept, err := keepjson.Split(b, p)
	if err != nil {
		return fmt.Errorf("the payment file: %w", err)
	}
	*u = Unsigned(p)
	u.kept = kept
	return nil
}

// MarshalJSON writes a payment file with the fields it kept.
func (u Unsigned) MarshalJSON() ([]byte, error) {
	type plain Unsigned
	b, err := json.Marshal(plain(u))
	if err != nil {
		return nil, err
	}
	return keepjson.Join(b, u.kept)
}

// UnsignedLeg is one transaction waiting for signatures.
type UnsignedLeg struct {
	Serial uint64           `json:"serial"`
	Tx     Hex              `json:"tx"`
	Inputs []PartialWitness `json:"inputs"`
	kept   keepjson.Kept
}

// UnmarshalJSON reads one leg, keeping the fields it does not know (W-23).
func (l *UnsignedLeg) UnmarshalJSON(b []byte) error {
	type plain UnsignedLeg
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	kept, err := keepjson.Split(b, p)
	if err != nil {
		return fmt.Errorf("a payment leg: %w", err)
	}
	*l = UnsignedLeg(p)
	l.kept = kept
	return nil
}

// MarshalJSON writes one leg with the fields it kept.
func (l UnsignedLeg) MarshalJSON() ([]byte, error) {
	type plain UnsignedLeg
	b, err := json.Marshal(plain(l))
	if err != nil {
		return nil, err
	}
	return keepjson.Join(b, l.kept)
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

// DecodeLeg reads leg i's transaction, with its part when the series is
// variable, and checks that it is of the payment's series and the leg's
// mint (RED-104, RED-105).
func (u *Unsigned) DecodeLeg(i int) (*tx.Tx, error) {
	l := u.Legs[i]
	var t *tx.Tx
	var err error
	if u.Variable || series.IsVariableID(u.Series) {
		t, err = tx.DecodeVariable(l.Tx)
	} else {
		t, err = tx.Decode(l.Tx)
	}
	if err != nil {
		if !u.Variable {
			return nil, fmt.Errorf("leg of mint %d does not decode (%v); if its series is variable supply, the file needs \"variable\": true, which poo wallet pay writes from this release", l.Serial, err)
		}
		return nil, fmt.Errorf("leg of mint %d: %w", l.Serial, err)
	}
	if t.Mint.Series != u.Series {
		return nil, fmt.Errorf("a leg of mint %d is in series %s, but the payment says series %s", l.Serial, t.Mint.Series, u.Series)
	}
	if t.Mint.Serial != l.Serial {
		return nil, fmt.Errorf("a leg says mint %d but its transaction spends mint %d", l.Serial, t.Mint.Serial)
	}
	return t, nil
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
	if len(l.Inputs) != witnessCount(t) {
		return nil, errors.New("wallet: unsigned leg has the wrong number of inputs")
	}
	if len(t.Witnesses) != len(l.Inputs) {
		t.Witnesses = make([]policy.Witness, len(l.Inputs))
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

// SignWith adds signer's signatures to u, with no wallet and no node: for
// every input whose policy lists the signer's key (alg and public key),
// it signs the leg's transaction id under the hash function h. It
// returns how many signatures it added. This is how a key kept off the
// node, on another machine, signs a payment a watch-only wallet made
// (poo sign, D193). The caller should show what the legs pay first.
func SignWith(u *Unsigned, h hash.Func, alg policy.AlgID, signer policy.Signer) (int, error) {
	pub := signer.Public()
	added := 0
	for li := range u.Legs {
		l := &u.Legs[li]
		t, err := u.DecodeLeg(li)
		if err != nil {
			return added, err
		}
		if len(l.Inputs) != witnessCount(t) {
			return added, errors.New("wallet: unsigned leg has the wrong number of inputs")
		}
		msg := policy.Message(t.TID(h))
		var sig []byte // one signature serves every input of the leg (ORANGE-045)
		for i := range l.Inputs {
			p, err := l.Inputs[i].policy()
			if err != nil {
				return added, err
			}
			tp, err := policy.DecodeThresholdParams(policy.Default(), p.Params)
			if err != nil {
				return added, err
			}
			for ki, k := range tp.Keys {
				if k.Alg != alg || !bytes.Equal(k.Public, pub) || l.Inputs[i].has(uint16(ki)) {
					continue
				}
				if sig == nil {
					sig = signer.Sign(msg)
					// A signature that does not verify never reaches the
					// file (RED-106).
					if a, ok := policy.Default().Alg(alg); !ok || !a.Verify(pub, msg, sig) {
						return added, errors.New("wallet: the key's signature does not verify under its public key")
					}
				}
				l.Inputs[i].Sigs = append(l.Inputs[i].Sigs, SigHex{Index: uint16(ki), Sig: sig})
				added++
			}
		}
	}
	return added, nil
}
