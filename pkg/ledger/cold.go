package ledger

import (
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Items and undo outside a recent window (D175). An entry of the selected
// ledger more than Config.RecentWindow entries below the tip, whose body
// the source can give back, is cold: its items (the transactions it
// applies) and its undo record are dropped from memory, leaving the
// skeleton (hash, parent, height, weights, layers, the tids index and the
// mints it claims). Both come back when something needs them:
//   - items are the body's transactions, read back through the source;
//   - the undo record is what the entry's inputs spent, and each spent
//     portion is an output of the transaction that made it, found through
//     the tids index on the selected ledger, so it is recomputed exactly
//     with no undo file.
//
// A switch at any depth warms every entry it reverts before it changes
// anything; a view at an old anchor reads them on an overlay. Nothing is
// refused for depth: a deep switch only reads the disk and is slower. A
// read that fails while the pool is being changed panics with a ReadFault,
// which fails the engine (D174): the node stops and rebuilds from its
// stores, so it never selects on partial data.

// ReadFault is the panic value of a read back from the source that failed
// while the pool needed it to change or check something (D175).
type ReadFault struct {
	Entry hash.Hash
	Err   error
}

func (f ReadFault) Error() string {
	return fmt.Sprintf("ledger: could not read back entry %s from the store: %v", f.Entry.Short(), f.Err)
}

func (p *Pool) fault(n *node, err error) {
	panic(ReadFault{Entry: n.hash, Err: err})
}

// loadItems reads n's items back from its body, without witnesses.
func (p *Pool) loadItems(n *node) ([]state.Item, error) {
	e, err := p.ref(n).Get()
	if err != nil {
		return nil, err
	}
	items := toStateItems(e.Items(p.s.Hash()))
	if len(items) != n.nitems {
		return nil, fmt.Errorf("%w: %d transactions read back, %d held", ErrUnreadable, len(items), n.nitems)
	}
	stripWitnesses(items)
	return items, nil
}

// itemsOf returns n's items, read back when n is cold.
func (p *Pool) itemsOf(n *node) ([]state.Item, error) {
	if !n.cold {
		return n.items, nil
	}
	return p.loadItems(n)
}

// undoOf recomputes the undo record of n, an entry of the selected
// ledger, from its items: for each input, in the order Apply spends them,
// the output of the transaction that made it. Every such transaction is
// in an entry of the selected ledger below n (an entry never spends what
// its own transactions make), found by the tids index.
func (p *Pool) undoOf(items []state.Item, memo map[*node][]state.Item) (state.Undo, error) {
	var u state.Undo
	for _, it := range items {
		for _, in := range it.Tx.Inputs {
			c := p.tids[in.TID]
			if c == nil {
				return u, fmt.Errorf("%w: input %s made by no entry of the selected ledger", ErrUnreadable, in)
			}
			cis, ok := memo[c]
			if !ok {
				var err error
				if cis, err = p.itemsOf(c); err != nil {
					return u, err
				}
				memo[c] = cis
			}
			var maker *tx.Tx
			for _, ci := range cis {
				if ci.TID == in.TID {
					maker = ci.Tx
					break
				}
			}
			if maker == nil || int(in.Index) >= len(maker.Outputs) {
				return u, fmt.Errorf("%w: input %s not found in entry %s", ErrUnreadable, in, c.hash.Short())
			}
			out := maker.Outputs[in.Index]
			u.Spent = append(u.Spent, state.Spent{Input: in, Portion: tx.Portion{Serial: maker.Mint.Serial, Address: out.Address, Amount: out.Amount}})
		}
	}
	return u, nil
}

// warmed returns n's items and undo record, recomputed when n is cold,
// without keeping them. A failure panics with a ReadFault.
func (p *Pool) warmed(n *node, memo map[*node][]state.Item) ([]state.Item, state.Undo) {
	if !n.cold {
		return n.items, n.undo
	}
	items, ok := memo[n]
	if !ok {
		var err error
		if items, err = p.loadItems(n); err != nil {
			p.fault(n, err)
		}
		memo[n] = items
	}
	u, err := p.undoOf(items, memo)
	if err != nil {
		p.fault(n, err)
	}
	return items, u
}

// warm brings back the items and undo records of the cold entries of ns,
// all before any of them is changed, and keeps them. A failure panics
// with a ReadFault.
func (p *Pool) warm(ns []*node) {
	memo := map[*node][]state.Item{}
	type back struct {
		items []state.Item
		undo  state.Undo
	}
	got := make([]back, len(ns))
	for i, n := range ns {
		if n.cold {
			got[i].items, got[i].undo = p.warmed(n, memo)
		}
	}
	for i, n := range ns {
		if n.cold {
			n.items, n.undo, n.cold = got[i].items, got[i].undo, false
		}
	}
}

// cool drops the items and undo records of the selected ledger's entries
// more than RecentWindow below the tip, oldest first, while the source
// can give back their bodies. With no source or no window it does
// nothing.
func (p *Pool) cool() {
	w := p.cfg.RecentWindow
	if w <= 0 || p.cfg.Source == nil {
		return
	}
	for p.cooled < len(p.chain)-w {
		n := p.chain[p.cooled]
		if !n.cold {
			if !p.cfg.Source.Has(n.hash) {
				return // not written yet; a later call cools it
			}
			n.items, n.undo, n.cold = nil, state.Undo{}, true
		}
		p.cooled++
	}
}

// Cold counts the held entries whose items and undo are not in memory
// (for tests and status).
func (p *Pool) Cold() int {
	c := 0
	for _, n := range p.chain[:p.cooled] {
		if n.cold {
			c++
		}
	}
	return c
}
