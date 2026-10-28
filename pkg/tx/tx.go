// Package tx implements transactions: the only authorized object in the
// protocol.
//
// A transaction belongs to exactly one mint. It spends whole portions of
// that mint and creates new portions whose amounts add up to the amount
// spent, or, with no inputs, it claims the whole supply of an unclaimed mint.
//
//	Tx {
//	    mint      : (series_id, serial)
//	    inputs    : [ (tid, index) ]       // empty only for a claim
//	    outputs   : [ (address, amount) ]  // distinct addresses, amounts >= 1
//	    witnesses : [ Witness ]            // outside the tid
//	}
//	tid = H("PoO-tx" ∥ encode(mint, inputs, outputs))
//
// Validation is split in two. CheckStateless checks everything that does not
// depend on a ledger, including claim witnesses, since a claim's witnesses
// are checked against its own outputs. CheckAgainst checks a transaction
// against the state after a given entry: whether its inputs are unspent,
// whether the mint is claimed, conservation, and spend witnesses.
package tx

import (
	"errors"
	"fmt"
	"math/bits"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// Decoding bounds. They protect a node from huge allocations; they are not
// consensus limits on what a valid transaction may hold.
const (
	MaxInputs  = 1 << 16
	MaxOutputs = 1 << 16
)

// Errors returned by transaction checks. If any check fails the whole
// transaction is invalid; there is no partial credit.
var (
	ErrWrongSeries     = errors.New("tx: mint is not in this series")
	ErrSerialRange     = errors.New("tx: serial out of range")
	ErrNoOutputs       = errors.New("tx: no outputs")
	ErrZeroAmount      = errors.New("tx: output amount below 1 unit")
	ErrRepeatedAddress = errors.New("tx: two outputs go to the same address")
	ErrRepeatedInput   = errors.New("tx: an input is listed twice")
	ErrOverflow        = errors.New("tx: amounts overflow")
	ErrClaimSupply     = errors.New("tx: claim outputs do not add up to the whole supply")
	ErrWitnessCount    = errors.New("tx: wrong number of witnesses")
	ErrWitness         = errors.New("tx: invalid witness")
	ErrAlreadyClaimed  = errors.New("tx: mint already claimed in this ledger")
	ErrNotClaimed      = errors.New("tx: mint not claimed in this ledger")
	ErrMissingInput    = errors.New("tx: an input is not an unspent portion in this ledger")
	ErrWrongMint       = errors.New("tx: an input is a portion of another mint")
	ErrConservation    = errors.New("tx: outputs do not add up to the inputs")
)

// Input names a portion: the tid of the transaction that created it and its
// position among that transaction's outputs.
type Input struct {
	TID   hash.Hash `json:"tid"`
	Index uint32    `json:"index"`
}

// String returns "<tid short>:<index>", the form used in the design.
func (in Input) String() string { return fmt.Sprintf("%s:%d", in.TID.Short(), in.Index) }

// Output is an address and an amount. Output i of transaction t becomes
// the portion (t, i).
type Output struct {
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
}

// Tx is a transaction.
type Tx struct {
	Mint      series.MintID    `json:"mint"`
	Inputs    []Input          `json:"inputs"`
	Outputs   []Output         `json:"outputs"`
	Witnesses []policy.Witness `json:"witnesses"`
}

// IsClaim reports whether t has no inputs.
func (t *Tx) IsClaim() bool { return len(t.Inputs) == 0 }

// Body returns encode(mint, inputs, outputs), the bytes the tid covers.
func (t *Tx) Body() []byte {
	w := encoding.NewWriter(48 + 36*len(t.Inputs) + 40*len(t.Outputs))
	t.writeBody(w)
	return w.Bytes()
}

func (t *Tx) writeBody(w *encoding.Writer) {
	w.Raw(t.Mint.Series[:]).U64(t.Mint.Serial)
	w.Count(len(t.Inputs))
	for _, in := range t.Inputs {
		w.Raw(in.TID[:]).U32(in.Index)
	}
	w.Count(len(t.Outputs))
	for _, out := range t.Outputs {
		w.Raw(out.Address[:]).U64(out.Amount)
	}
}

// TID returns H("PoO-tx" ∥ body). Witnesses are left out, so adding or
// replacing a witness never changes the tid.
func (t *Tx) TID(h hash.Func) hash.Hash { return h(hash.TagTx, t.Body()) }

// Encode returns the full transaction: the body followed by the witnesses.
func (t *Tx) Encode() []byte {
	w := encoding.NewWriter(256)
	t.EncodeTo(w)
	return w.Bytes()
}

// EncodeTo writes the full transaction to w.
func (t *Tx) EncodeTo(w *encoding.Writer) {
	t.writeBody(w)
	w.Count(len(t.Witnesses))
	for _, wit := range t.Witnesses {
		wit.Encode(w)
	}
}

// Decode reads a full transaction written by Encode.
func Decode(b []byte) (*Tx, error) {
	r := encoding.NewReader(b)
	t := DecodeFrom(r)
	if err := r.Finish(); err != nil {
		return nil, fmt.Errorf("tx: decode: %w", err)
	}
	return t, nil
}

// DecodeFrom reads one full transaction from r. Check r.Err afterwards.
func DecodeFrom(r *encoding.Reader) *Tx {
	t := &Tx{}
	r.Into(t.Mint.Series[:])
	t.Mint.Serial = r.U64()
	n := r.Count(MaxInputs, 36)
	if n > 0 {
		t.Inputs = make([]Input, n)
		for i := range t.Inputs {
			r.Into(t.Inputs[i].TID[:])
			t.Inputs[i].Index = r.U32()
		}
	}
	n = r.Count(MaxOutputs, 40)
	t.Outputs = make([]Output, n)
	for i := range t.Outputs {
		r.Into(t.Outputs[i].Address[:])
		t.Outputs[i].Amount = r.U64()
	}
	n = r.Count(MaxInputs+MaxOutputs, 20)
	t.Witnesses = make([]policy.Witness, 0, n)
	for i := 0; i < n && r.Err() == nil; i++ {
		t.Witnesses = append(t.Witnesses, policy.DecodeWitness(r))
	}
	return t
}

// OutputSum returns the sum of the output amounts.
func (t *Tx) OutputSum() (uint64, error) {
	var sum uint64
	for _, o := range t.Outputs {
		var carry uint64
		sum, carry = bits.Add64(sum, o.Amount, 0)
		if carry != 0 {
			return 0, ErrOverflow
		}
	}
	return sum, nil
}

// CheckStateless checks the rules that do not depend on any ledger. For a
// claim it also checks the witnesses, one per output, against each output's
// address. tid must be t.TID(s.Hash()).
func CheckStateless(s *series.Series, t *Tx, tid hash.Hash) error {
	if err := CheckForm(s, t); err != nil {
		return err
	}
	if t.IsClaim() {
		return checkWitnesses(s, t, tid, claimOwners(t), "output")
	}
	return nil
}

// CheckForm is CheckStateless without a claim's witnesses: only rules
// that cost no signature check.
func CheckForm(s *series.Series, t *Tx) error {
	if t.Mint.Series != s.ID() {
		return ErrWrongSeries
	}
	if t.Mint.Serial >= s.Mints() {
		return ErrSerialRange
	}
	if len(t.Outputs) == 0 {
		return ErrNoOutputs
	}
	addrs := make(map[hash.Hash]struct{}, len(t.Outputs))
	for _, o := range t.Outputs {
		if o.Amount == 0 {
			return ErrZeroAmount
		}
		if _, dup := addrs[o.Address]; dup {
			return ErrRepeatedAddress
		}
		addrs[o.Address] = struct{}{}
	}
	sum, err := t.OutputSum()
	if err != nil {
		return err
	}
	if t.IsClaim() {
		if sum != s.Supply() {
			return ErrClaimSupply
		}
		if len(t.Witnesses) != len(t.Outputs) {
			return ErrWitnessCount
		}
		return nil
	}
	seen := make(map[Input]struct{}, len(t.Inputs))
	for _, in := range t.Inputs {
		if _, dup := seen[in]; dup {
			return ErrRepeatedInput
		}
		seen[in] = struct{}{}
	}
	if len(t.Witnesses) != len(t.Inputs) {
		return ErrWitnessCount
	}
	return nil
}

// Portion is an unspent output as a ledger state holds it.
type Portion struct {
	Serial  uint64    `json:"serial"`
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
}

// View is the state a transaction is checked against: the claims made and
// the portions left unspent in the ledger that ends at some entry.
type View interface {
	// Unspent returns the portion named by in, if it is unspent.
	Unspent(in Input) (Portion, bool)
	// Claimed reports whether the mint with this serial has a claim.
	Claimed(serial uint64) bool
}

// CheckAgainst checks the rules that depend on the state v. It assumes
// CheckStateless has passed. Witness signatures are checked last, since they
// are the most expensive step.
func CheckAgainst(s *series.Series, v View, t *Tx, tid hash.Hash) error {
	owners, err := Owners(v, t)
	if err != nil || t.IsClaim() {
		return err // a claim's witnesses are CheckStateless's
	}
	return checkWitnesses(s, t, tid, owners, "input")
}

// Owners runs CheckAgainst's rules short of the witnesses and returns the
// address each witness must satisfy: each input's for a spend, each
// output's for a claim. It assumes CheckForm has passed and verifies no
// signature.
func Owners(v View, t *Tx) ([]hash.Hash, error) {
	if t.IsClaim() {
		if v.Claimed(t.Mint.Serial) {
			return nil, ErrAlreadyClaimed
		}
		return claimOwners(t), nil
	}
	if !v.Claimed(t.Mint.Serial) {
		return nil, ErrNotClaimed
	}
	owners := make([]hash.Hash, len(t.Inputs))
	var in uint64
	for i, input := range t.Inputs {
		p, ok := v.Unspent(input)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrMissingInput, input)
		}
		if p.Serial != t.Mint.Serial {
			return nil, ErrWrongMint
		}
		var carry uint64
		in, carry = bits.Add64(in, p.Amount, 0)
		if carry != 0 {
			return nil, ErrOverflow
		}
		owners[i] = p.Address
	}
	out, err := t.OutputSum()
	if err != nil {
		return nil, err
	}
	if in != out {
		return nil, ErrConservation
	}
	return owners, nil
}

func claimOwners(t *Tx) []hash.Hash {
	owners := make([]hash.Hash, len(t.Outputs))
	for i, o := range t.Outputs {
		owners[i] = o.Address
	}
	return owners
}

// checkWitnesses checks witness i against owners[i] in order and stops at
// the first that fails.
func checkWitnesses(s *series.Series, t *Tx, tid hash.Hash, owners []hash.Hash, what string) error {
	if len(owners) != len(t.Witnesses) {
		return ErrWitnessCount
	}
	for i, w := range t.Witnesses {
		if err := s.CheckWitness(w, owners[i], tid); err != nil {
			return fmt.Errorf("%w: %s %d: %v", ErrWitness, what, i, err)
		}
	}
	return nil
}

// Preverify checks t's witnesses against owners, the addresses Owners
// returned, ahead of Check, and has the series remember each that passes,
// so Check, which may run under a node's lock, does not verify them again.
// It stops at the first witness that fails and returns that error: a
// witness never satisfies an address it does not match, so only items
// that passed every cheaper rule cost signature checks, and a bad one
// costs one (RED-009, decision D107). It never changes what Check returns.
func Preverify(s *series.Series, t *Tx, tid hash.Hash, owners []hash.Hash) error {
	what := "input"
	if t.IsClaim() {
		what = "output"
	}
	return checkWitnesses(s, t, tid, owners, what)
}

// Check runs CheckStateless and then CheckAgainst.
func Check(s *series.Series, v View, t *Tx, tid hash.Hash) error {
	if err := CheckStateless(s, t, tid); err != nil {
		return err
	}
	return CheckAgainst(s, v, t, tid)
}
