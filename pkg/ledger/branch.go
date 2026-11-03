package ledger

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Building on a branch (D161). A node builds its entries on the selected
// tip, but any held entry is a valid parent: a user who wants a mint
// claimed otherwise than the selected ledger has it builds a rival branch
// from before that claim and adds weight to it. Nothing here is a rule:
// the entries are checked by AddEntry like any other, and weight decides.

// ErrNoClaim: the selected ledger holds no claim of that mint.
var ErrNoClaim = errors.New("ledger: the selected ledger holds no claim of this mint")

// ErrNotHeld: the parent is neither a held entry nor the pool_id.
var ErrNotHeld = errors.New("ledger: parent entry not held")

// ClaimEntry returns the entry of the selected ledger that holds mint
// serial's claim, and the claim's tid.
func (p *Pool) ClaimEntry(serial uint64) (entryHash, tid hash.Hash, err error) {
	for _, n := range p.chain {
		for _, it := range n.items {
			if it.Tx.IsClaim() && it.Tx.Mint.Serial == serial {
				return n.hash, it.TID, nil
			}
		}
	}
	return hash.Zero, hash.Zero, ErrNoClaim
}

// ParentOf returns the parent of a held entry: an entry hash, or the
// pool_id for the pool's first entry.
func (p *Pool) ParentOf(h hash.Hash) (hash.Hash, bool) {
	n, ok := p.nodes[h]
	if !ok {
		return hash.Zero, false
	}
	return n.entry.Parent, true
}

// Refused is a transaction CheckOn left out, and why.
type Refused struct {
	TID hash.Hash
	Err error
}

// CheckOn returns the transactions of txs that are valid together on the
// state after parent (a held entry or the pool_id), in canonical order,
// taking each in turn (one claim of a mint, no input spent twice) and
// leaving out the ones that fail with their error.
// It changes nothing. Its cost is that of AddEntry on the same parent: the
// selected chain reverted down to it and the branch applied.
func (p *Pool) CheckOn(parent hash.Hash, txs []*tx.Tx) ([]*tx.Tx, []Refused, error) {
	var n *node
	if parent != p.id {
		var ok bool
		if n, ok = p.nodes[parent]; !ok {
			return nil, nil, fmt.Errorf("%w: %s", ErrNotHeld, parent.Short())
		}
	}
	items := make([]entry.Item, len(txs))
	for i, t := range txs {
		items[i] = entry.Item{TID: t.TID(p.s.Hash()), Tx: t}
	}
	entry.SortItems(items)
	v := p.viewAfter(n).Overlay()
	var ok []*tx.Tx
	var refused []Refused
	var taken []entry.Item
	for _, it := range items {
		if err := tx.CheckStateless(p.s, it.Tx, it.TID); err != nil {
			refused = append(refused, Refused{it.TID, err})
			continue
		}
		// Checked against the state after parent alone, as AddEntry does:
		// a transaction may not spend an output made in the same entry.
		if err := entry.CheckAgainst(p.s, v, []entry.Item{it}); err != nil {
			refused = append(refused, Refused{it.TID, err})
			continue
		}
		if conflicts(taken, it) {
			refused = append(refused, Refused{it.TID, entry.ErrConflict})
			continue
		}
		taken = append(taken, it)
		ok = append(ok, it.Tx)
	}
	return ok, refused, nil
}

// conflicts reports whether it claims a mint or spends an input that one of
// taken already does.
func conflicts(taken []entry.Item, it entry.Item) bool {
	for _, t := range taken {
		if it.Tx.IsClaim() && t.Tx.IsClaim() && it.Tx.Mint.Serial == t.Tx.Mint.Serial {
			return true
		}
		for _, a := range t.Tx.Inputs {
			for _, b := range it.Tx.Inputs {
				if a == b {
					return true
				}
			}
		}
	}
	return false
}
