package wallet

import (
	"errors"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// LegStatus is where one leg of a payment stands, with the numbers a
// receiver reads (the design's section 7): from(e), and the margin
// against the heaviest known rival.
type LegStatus struct {
	Serial uint64        `json:"serial"`
	TID    hash.Hash     `json:"tid"`
	Amount uint64        `json:"amount"`
	State  string        `json:"state"`
	Entry  *hash.Hash    `json:"entry,omitempty"`
	From   weight.Weight `json:"from"`
	// Rival is the heaviest known rival's weight after its fork, and
	// Ahead whether From beats it; with no rival known, Ahead is true.
	Rival  *weight.Weight `json:"rival,omitempty"`
	Ahead  bool           `json:"ahead"`
	Rivals int            `json:"rivals,omitempty"`
}

// Settled reports whether every leg is selected, has from(e) of at least
// threshold, and is ahead of every known rival: the receiver's rule of
// the design's sections 7.4 and 7.6. A payment across pools settles only
// when its slowest leg does.
func Settled(legs []LegStatus, threshold weight.Weight) bool {
	if len(legs) == 0 {
		return false
	}
	for _, l := range legs {
		if l.State != StateSelected || threshold.Greater(l.From) || !l.Ahead {
			return false
		}
	}
	return true
}

// status reads one transaction's standing. For one the node does not
// hold, it tells dropped (every input still unspent) from void.
func status(s *series.Series, n Node, serial uint64, tid hash.Hash, inputs []tx.Input, claim bool) (LegStatus, Status, error) {
	ls := LegStatus{Serial: serial, TID: tid, Ahead: true}
	st, err := n.Payment(s.ID(), serial, tid)
	if err != nil {
		return ls, st, err
	}
	switch {
	case st.Selected:
		ls.State = StateSelected
		e := st.Entry
		ls.Entry, ls.From = &e, st.From
	case st.Pending:
		ls.State = StatePending
	default:
		ps, err := n.Holdings(s.ID(), serial)
		if err != nil {
			return ls, st, err
		}
		unspent := map[tx.Input]bool{}
		for _, p := range ps {
			unspent[p.Input] = true
		}
		ls.State = StateDropped
		if claim && len(ps) > 0 {
			ls.State = StateVoid
		}
		for _, in := range inputs {
			if !unspent[in] {
				ls.State = StateVoid
			}
		}
	}
	for _, r := range st.Rivals {
		if ls.Rival == nil || r.After.Greater(*ls.Rival) {
			a := r.After
			ls.Rival = &a
		}
	}
	ls.Rivals = len(st.Rivals)
	if ls.Rival != nil {
		ls.Ahead = ls.From.Greater(*ls.Rival)
	}
	return ls, st, nil
}

// Refresh reads where each leg of p stands on the node and records it.
// Unsigned legs are left alone. The caller saves the wallet.
func (w *Wallet) Refresh(s *series.Series, n Node, p *Sent) ([]LegStatus, error) {
	if p.Series != s.ID() {
		return nil, errors.New("wallet: the payment is of another series")
	}
	out := make([]LegStatus, 0, len(p.Legs))
	for i := range p.Legs {
		l := &p.Legs[i]
		if l.State == StateUnsigned {
			out = append(out, LegStatus{Serial: l.Serial, TID: l.TID, Amount: l.Amount, State: l.State, Ahead: true})
			continue
		}
		ls, _, err := status(s, n, l.Serial, l.TID, l.Inputs, p.Claim)
		if err != nil {
			return out, err
		}
		ls.Amount = l.Amount
		l.State = ls.State
		out = append(out, ls)
	}
	return out, nil
}

// Resend sends the dropped legs of p again: the node no longer held them
// and their inputs are still unspent. It returns how many it sent.
func (w *Wallet) Resend(s *series.Series, n Node, p *Sent) (int, error) {
	sent := 0
	for i := range p.Legs {
		l := &p.Legs[i]
		if l.State != StateDropped {
			continue
		}
		t, err := tx.Decode(l.Tx)
		if err != nil {
			return sent, err
		}
		if err := n.Submit(t); err != nil {
			return sent, err
		}
		l.State = StatePending
		sent++
	}
	return sent, nil
}

// Receipt is what has arrived for an expected payment.
type Receipt struct {
	Request Request     `json:"request"`
	Legs    []LegStatus `json:"legs"`
	// Received counts the units paid to the address by legs pending or
	// selected; Complete is set when it reaches the amount asked.
	Received uint64 `json:"received"`
	Complete bool   `json:"complete"`
}

// Receive reads what has arrived for e: portions at its address in the
// watched mints of the selected ledger, and the transactions the payer
// named. Each leg's amount is read from the transaction itself.
func (w *Wallet) Receive(s *series.Series, n Node, e *Expected) (Receipt, error) {
	r := Receipt{Request: e.Request}
	if e.Request.Series != s.ID() {
		return r, errors.New("wallet: the request is of another series")
	}
	refs := append([]TxRef(nil), e.TIDs...)
	seen := map[TxRef]bool{}
	for _, ref := range refs {
		seen[ref] = true
	}
	for _, serial := range w.Watched(s.ID()) {
		ps, err := n.Holdings(s.ID(), serial)
		if err != nil {
			return r, err
		}
		for _, p := range ps {
			ref := TxRef{Serial: serial, TID: p.Input.TID}
			if p.Address == e.Request.Address && !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	for _, ref := range refs {
		ls, st, err := status(s, n, ref.Serial, ref.TID, nil, false)
		if err != nil {
			return r, err
		}
		if st.Tx != nil {
			if st.Tx.TID(s.Hash()) != ref.TID {
				return r, errors.New("wallet: the node returned another transaction")
			}
			for _, o := range st.Tx.Outputs {
				if o.Address == e.Request.Address {
					ls.Amount += o.Amount
				}
			}
		}
		if ls.State == StateDropped || ls.State == StateVoid {
			// Not held: nothing is known to have arrived.
			ls.State = "unknown"
		}
		if ls.State == StatePending || ls.State == StateSelected {
			r.Received += ls.Amount
		}
		r.Legs = append(r.Legs, ls)
	}
	r.Complete = r.Received >= e.Request.Amount
	return r, nil
}

// Settled reports whether the whole amount arrived and every leg that
// pays it is settled at threshold.
func (r Receipt) Settled(threshold weight.Weight) bool {
	if !r.Complete {
		return false
	}
	var legs []LegStatus
	for _, l := range r.Legs {
		if l.Amount > 0 {
			legs = append(legs, l)
		}
	}
	return Settled(legs, threshold)
}
