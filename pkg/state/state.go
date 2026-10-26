// Package state implements pool state: for each claimed mint of a pool, its
// unspent portions with their addresses and amounts, after a given entry.
//
// No entry contains the state. A node finds it by replaying a ledger from
// its first entry, entry by entry, and within each entry transaction by
// transaction in canonical order. Two nodes that hold the same ledger compute
// the same state.
//
// A State can sit on top of another State as an overlay. Reads fall through
// to the base; writes stay in the overlay. The ledger uses overlays to check
// an entry on a branch other than the selected one without disturbing the
// state at the selected tip, and to sort displaced transactions after a
// switch.
package state

import (
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// State is the unspent portions and claims of one pool.
type State struct {
	base    *State
	unspent map[tx.Input]tx.Portion
	removed map[tx.Input]struct{} // base portions spent in this overlay
	claimed map[uint64]bool       // overrides; false means unclaimed here
}

// New returns an empty state: nothing claimed, no portions.
func New() *State {
	return &State{unspent: map[tx.Input]tx.Portion{}, removed: map[tx.Input]struct{}{}, claimed: map[uint64]bool{}}
}

// Overlay returns a new state that reads through to s and keeps its own
// changes. s must not change while the overlay is in use.
func (s *State) Overlay() *State {
	o := New()
	o.base = s
	return o
}

// Unspent implements tx.View.
func (s *State) Unspent(in tx.Input) (tx.Portion, bool) {
	if p, ok := s.unspent[in]; ok {
		return p, true
	}
	if _, gone := s.removed[in]; gone {
		return tx.Portion{}, false
	}
	if s.base != nil {
		return s.base.Unspent(in)
	}
	return tx.Portion{}, false
}

// Claimed implements tx.View.
func (s *State) Claimed(serial uint64) bool {
	if v, ok := s.claimed[serial]; ok {
		return v
	}
	if s.base != nil {
		return s.base.Claimed(serial)
	}
	return false
}

func (s *State) baseHas(in tx.Input) bool {
	if s.base == nil {
		return false
	}
	_, ok := s.base.Unspent(in)
	return ok
}

func (s *State) spend(in tx.Input) (tx.Portion, bool) {
	p, ok := s.Unspent(in)
	if !ok {
		return p, false
	}
	if _, own := s.unspent[in]; own {
		delete(s.unspent, in)
	}
	if s.baseHas(in) {
		s.removed[in] = struct{}{}
	}
	return p, true
}

func (s *State) add(in tx.Input, p tx.Portion) {
	delete(s.removed, in)
	if s.baseHas(in) {
		return // the base already holds it and it is no longer removed
	}
	s.unspent[in] = p
}

// Spent records one portion spent by an entry, so the entry can be undone.
type Spent struct {
	Input   tx.Input
	Portion tx.Portion
}

// Undo holds what an entry consumed: the portions its transactions spent,
// in order. The portions it created and the claims it made follow from the
// entry itself.
type Undo struct {
	Spent []Spent
}

// Item is one transaction with its tid, as the state applies it.
type Item struct {
	TID hash.Hash
	Tx  *tx.Tx
}

// ApplyTx applies one transaction that is known to be valid against s and
// appends what it spent to undo. It panics if an input is missing, since
// that means the caller applied something it had not checked.
func (s *State) ApplyTx(it Item, undo *Undo) {
	for _, in := range it.Tx.Inputs {
		p, ok := s.spend(in)
		if !ok {
			panic("state: applying a transaction whose input is not unspent: " + in.String())
		}
		if undo != nil {
			undo.Spent = append(undo.Spent, Spent{Input: in, Portion: p})
		}
	}
	for i, out := range it.Tx.Outputs {
		s.add(tx.Input{TID: it.TID, Index: uint32(i)}, tx.Portion{Serial: it.Tx.Mint.Serial, Address: out.Address, Amount: out.Amount})
	}
	if it.Tx.IsClaim() {
		s.claimed[it.Tx.Mint.Serial] = true
	}
}

// Apply applies the transactions of one entry, in canonical order, and
// returns the record needed to undo it.
func (s *State) Apply(items []Item) Undo {
	var u Undo
	for _, it := range items {
		s.ApplyTx(it, &u)
	}
	return u
}

// Revert undoes an entry applied with Apply, given the same items and the
// undo record Apply returned.
func (s *State) Revert(items []Item, u Undo) {
	spent := len(u.Spent)
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if it.Tx.IsClaim() {
			if s.base != nil && s.base.Claimed(it.Tx.Mint.Serial) {
				s.claimed[it.Tx.Mint.Serial] = false
			} else {
				delete(s.claimed, it.Tx.Mint.Serial)
			}
		}
		for j := range it.Tx.Outputs {
			in := tx.Input{TID: it.TID, Index: uint32(j)}
			if _, ok := s.spend(in); !ok {
				panic("state: reverting an entry whose output is not unspent")
			}
		}
		n := len(it.Tx.Inputs)
		for _, sp := range u.Spent[spent-n : spent] {
			s.add(sp.Input, sp.Portion)
		}
		spent -= n
	}
}

// ForEach calls fn for every unspent portion, in no particular order.
func (s *State) ForEach(fn func(tx.Input, tx.Portion)) {
	if s.base != nil {
		s.base.ForEach(func(in tx.Input, p tx.Portion) {
			if _, gone := s.removed[in]; gone {
				return
			}
			if _, own := s.unspent[in]; own {
				return
			}
			fn(in, p)
		})
	}
	for in, p := range s.unspent {
		fn(in, p)
	}
}

// Holding is one unspent portion with its name.
type Holding struct {
	Input   tx.Input
	Portion tx.Portion
}

// Portions returns the unspent portions of one mint, sorted by tid then
// index, so output is stable.
func (s *State) Portions(serial uint64) []Holding {
	var out []Holding
	s.ForEach(func(in tx.Input, p tx.Portion) {
		if p.Serial == serial {
			out = append(out, Holding{in, p})
		}
	})
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Input.TID.Compare(out[j].Input.TID); c != 0 {
			return c < 0
		}
		return out[i].Input.Index < out[j].Input.Index
	})
	return out
}

// Totals returns, for each claimed mint, the sum of its unspent portions.
// For a correct ledger every value equals the series' supply.
func (s *State) Totals() map[uint64]uint64 {
	t := map[uint64]uint64{}
	s.ForEach(func(_ tx.Input, p tx.Portion) { t[p.Serial] += p.Amount })
	return t
}

// Len returns the number of unspent portions.
func (s *State) Len() int {
	n := 0
	s.ForEach(func(tx.Input, tx.Portion) { n++ })
	return n
}
