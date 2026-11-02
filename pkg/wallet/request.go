package wallet

import (
	"errors"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// Request is a payment request: what a receiver sends a payer. It carries
// the policy behind the address so the payer can check, before paying,
// that the address is spendable under the series' rules (the design's
// section 17.6); only the payer learns it.
type Request struct {
	Series  hash.Hash `json:"series"`
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
	Policy  Hex       `json:"policy,omitempty"`
	Memo    string    `json:"memo,omitempty"`
}

func decodePolicy(b []byte) (policy.Policy, error) {
	r := encoding.NewReader(b)
	p := policy.DecodePolicy(r)
	if err := r.Finish(); err != nil {
		return p, err
	}
	return p, nil
}

// Check verifies the request against s: its series, and when it carries a
// policy, that the policy is behind the address and allowed. It returns
// the policy, or nil when the request has none.
func (r Request) Check(s *series.Series) (*policy.Policy, error) {
	if r.Series != s.ID() {
		return nil, errors.New("wallet: the request is for another series")
	}
	if r.Amount == 0 {
		return nil, errors.New("wallet: the request asks for nothing")
	}
	if len(r.Policy) == 0 {
		return nil, nil
	}
	p, err := decodePolicy(r.Policy)
	if err != nil {
		return nil, err
	}
	if s.Address(p) != r.Address {
		return nil, errors.New("wallet: the request's policy is not behind its address")
	}
	if err := CheckPolicy(s, p); err != nil {
		return nil, err
	}
	return &p, nil
}

// PayRequest checks r and pays it.
func (w *Wallet) PayRequest(s *series.Series, n Node, r Request) (*Sent, *Unsigned, error) {
	p, err := r.Check(s)
	if err != nil {
		return nil, nil, err
	}
	return w.Pay(s, n, r.Address, r.Amount, PayOptions{Policy: p, Memo: r.Memo})
}

// TxRef names a transaction by its mint and tid.
type TxRef struct {
	Serial uint64    `json:"serial"`
	TID    hash.Hash `json:"tid"`
}

// Expected is a payment the wallet asked for: a request it gave out, and
// the transactions the payer said pay it.
type Expected struct {
	Request Request   `json:"request"`
	Created time.Time `json:"created"`
	TIDs    []TxRef   `json:"tids,omitempty"`
}

// Expected returns the payments asked for, oldest first.
func (w *Wallet) Expected() []*Expected { return w.f.Expected }

// Request makes a payment request for amount units of s to a fresh
// address, and records it as expected.
func (w *Wallet) Request(s *series.Series, amount uint64, memo string) (Request, error) {
	if amount == 0 {
		return Request{}, errors.New("wallet: request at least 1 unit")
	}
	label := "request"
	if memo != "" {
		label = "request: " + memo
	}
	a, err := w.NewAddress(s, label)
	if err != nil {
		return Request{}, err
	}
	r := Request{Series: s.ID(), Address: a.Address, Amount: amount, Policy: a.Policy.Encode(), Memo: memo}
	w.f.Expected = append(w.f.Expected, &Expected{Request: r, Created: now()})
	return r, nil
}

// Find returns the expected payment to address.
func (w *Wallet) Find(address hash.Hash) (*Expected, bool) {
	for _, e := range w.f.Expected {
		if e.Request.Address == address {
			return e, true
		}
	}
	return nil, false
}

// Tell adds a transaction the payer says pays e, for a leg not yet in
// the selected ledger.
func (e *Expected) Tell(ref TxRef) {
	for _, r := range e.TIDs {
		if r == ref {
			return
		}
	}
	e.TIDs = append(e.TIDs, ref)
}
