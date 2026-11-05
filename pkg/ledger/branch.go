package ledger

import (
	"errors"
	"fmt"
	"slices"

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
		if !slices.Contains(n.claims, serial) {
			continue
		}
		items, err := p.itemsOf(n)
		if err != nil {
			return hash.Zero, hash.Zero, err
		}
		for _, it := range items {
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
	return p.parentHash(n), true
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
// selected chain reverted down to it and the branch applied. A transaction
// on a mint of another pool is left out with entry.ErrWrongPool, as
// AddEntry would refuse it, so a build never grinds an entry that cannot
// be used (RED-069).
func (p *Pool) CheckOn(parent hash.Hash, txs []*tx.Tx) ([]*tx.Tx, []Refused, error) {
	return p.checkOn(parent, txs, true)
}

// CheckPendingOn is CheckOn for transactions the pending pool already
// took (tx.Check, or carried back from a ledger): it checks their claims,
// inputs and amounts against the state after parent (tx.Owners) but not
// their shape or witnesses again. A witness satisfies an address, and an
// input's address is that of the output it spends, the same after any
// parent; so the result is CheckOn's, and a build on a chosen parent with
// a full pending pool costs time linear in the pool with no signature
// check under the engine lock (RED-068). AddEntry checks the built entry
// in full in any case.
func (p *Pool) CheckPendingOn(parent hash.Hash, txs []*tx.Tx) ([]*tx.Tx, []Refused, error) {
	return p.checkOn(parent, txs, false)
}

func (p *Pool) checkOn(parent hash.Hash, txs []*tx.Tx, stateless bool) ([]*tx.Tx, []Refused, error) {
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
	taken := newTaken()
	for _, it := range items {
		if m := it.Tx.Mint.Serial; m >= p.s.Mints() || p.s.PoolOf(m) != p.index {
			refused = append(refused, Refused{it.TID, entry.ErrWrongPool})
			continue
		}
		if stateless {
			if err := tx.CheckStateless(p.s, it.Tx, it.TID); err != nil {
				refused = append(refused, Refused{it.TID, err})
				continue
			}
		}
		// Checked against the state after parent alone, as AddEntry does:
		// a transaction may not spend an output made in the same entry.
		if stateless {
			if err := entry.CheckAgainst(p.s, v, []entry.Item{it}); err != nil {
				refused = append(refused, Refused{it.TID, err})
				continue
			}
		} else if _, err := tx.Owners(v, it.Tx); err != nil {
			// Its witnesses were checked when the pending pool took it,
			// against these same addresses: an input's owner is the
			// address of the output it spends, whichever state holds it.
			refused = append(refused, Refused{it.TID, fmt.Errorf("%w %s: %v", entry.ErrInvalidTx, it.TID.Short(), err)})
			continue
		}
		if taken.conflicts(it.Tx) {
			refused = append(refused, Refused{it.TID, entry.ErrConflict})
			continue
		}
		taken.add(it.Tx)
		ok = append(ok, it.Tx)
	}
	return ok, refused, nil
}

// taken is what the transactions CheckOn has kept so far claim and spend:
// a set of mints and one of inputs, so each new transaction is checked in
// time linear in its own inputs rather than in all those kept (ORANGE-033).
type taken struct {
	claims map[uint64]struct{}
	spent  map[tx.Input]struct{}
}

func newTaken() taken {
	return taken{claims: map[uint64]struct{}{}, spent: map[tx.Input]struct{}{}}
}

// conflicts reports whether t claims a mint or spends an input that one of
// the kept transactions already does.
func (k taken) conflicts(t *tx.Tx) bool {
	if t.IsClaim() {
		if _, ok := k.claims[t.Mint.Serial]; ok {
			return true
		}
	}
	for _, in := range t.Inputs {
		if _, ok := k.spent[in]; ok {
			return true
		}
	}
	return false
}

// add records t as kept.
func (k taken) add(t *tx.Tx) {
	if t.IsClaim() {
		k.claims[t.Mint.Serial] = struct{}{}
	}
	for _, in := range t.Inputs {
		k.spent[in] = struct{}{}
	}
}
