// Package entry implements entries: a bundle of transactions on mints of one
// pool, in canonical order, together with the hash of the entry it builds on.
//
//	Entry {
//	    parent : hash      // the pool_id for the first entry of a pool
//	    bundle : [ Tx ]    // full transactions, canonical order
//	    nonce  : u64       // only where the series sets a minimum entry strength
//	}
//	entry hash = H("PoO-entry" ∥ parent ∥ tid_1 ∥ ... ∥ tid_n [∥ nonce])
//
// The canonical order sorts by mint serial, then by tid as a big-endian
// number. Because no transaction in an entry may depend on another in the
// same entry, this order is complete: two builders holding the same set
// produce the same bundle, and so, without a nonce, the same entry.
package entry

import (
	"errors"
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// MaxTxs bounds decoding only. The design sets no limit on bundle size.
const MaxTxs = 1 << 20

// Errors returned by entry checks. One failing check makes the whole entry
// invalid, and an invalid entry is thrown out together with its hash.
var (
	ErrWrongPool      = errors.New("entry: a transaction is not on a mint of the entry's pool")
	ErrOrder          = errors.New("entry: bundle is not in canonical order")
	ErrWeak           = errors.New("entry: hash below the series' minimum entry strength")
	ErrNonce          = errors.New("entry: nonce given where the series sets no minimum entry strength")
	ErrConflict       = errors.New("entry: two transactions in the bundle conflict")
	ErrNotFound       = errors.New("entry: no nonce found within the tries allowed")
	ErrHashMismatch   = errors.New("entry: stated hash does not match the entry")
	ErrInvalidTx      = errors.New("entry: invalid transaction")
	ErrSeriesMismatch = errors.New("entry: transaction of another series")
)

// Entry is one entry of a pool's ledger.
type Entry struct {
	Parent hash.Hash
	Nonce  uint64
	Txs    []*tx.Tx
}

// TIDs returns the tids of the bundle, in bundle order.
func (e *Entry) TIDs(h hash.Func) []hash.Hash {
	out := make([]hash.Hash, len(e.Txs))
	for i, t := range e.Txs {
		out[i] = t.TID(h)
	}
	return out
}

// HashOf computes an entry hash from its parts. The nonce is included only
// when the series' entries carry one.
func HashOf(s *series.Series, parent hash.Hash, tids []hash.Hash, nonce uint64) hash.Hash {
	parts := make([][]byte, 0, len(tids)+3)
	parts = append(parts, hash.TagEntry, parent[:])
	for i := range tids {
		parts = append(parts, tids[i][:])
	}
	if s.EntriesHaveNonce() {
		parts = append(parts, encoding.NewWriter(8).U64(nonce).Bytes())
	}
	return s.Hash()(parts...)
}

// Hash returns the entry hash.
func (e *Entry) Hash(s *series.Series) hash.Hash {
	return HashOf(s, e.Parent, e.TIDs(s.Hash()), e.Nonce)
}

// Encode returns the wire form: parent, nonce (always written; it must be 0
// in a series with no minimum entry strength), and the full transactions.
func (e *Entry) Encode() []byte {
	w := encoding.NewWriter(64 + 256*len(e.Txs))
	e.EncodeTo(w)
	return w.Bytes()
}

// EncodeTo writes the wire form to w.
func (e *Entry) EncodeTo(w *encoding.Writer) {
	w.Raw(e.Parent[:]).U64(e.Nonce).Count(len(e.Txs))
	for _, t := range e.Txs {
		t.EncodeTo(w)
	}
}

// Decode reads an entry written by Encode.
func Decode(b []byte) (*Entry, error) {
	r := encoding.NewReader(b)
	e := DecodeFrom(r)
	if err := r.Finish(); err != nil {
		return nil, fmt.Errorf("entry: decode: %w", err)
	}
	return e, nil
}

// DecodeFrom reads one entry from r. Check r.Err afterwards.
func DecodeFrom(r *encoding.Reader) *Entry {
	e := &Entry{}
	r.Into(e.Parent[:])
	e.Nonce = r.U64()
	n := r.Count(MaxTxs, 60)
	e.Txs = make([]*tx.Tx, 0, n)
	for i := 0; i < n && r.Err() == nil; i++ {
		e.Txs = append(e.Txs, tx.DecodeFrom(r))
	}
	return e
}

// Item pairs a transaction with its tid.
type Item struct {
	TID hash.Hash
	Tx  *tx.Tx
}

// Items returns the bundle as (tid, tx) pairs.
func (e *Entry) Items(h hash.Func) []Item {
	out := make([]Item, len(e.Txs))
	for i, t := range e.Txs {
		out[i] = Item{TID: t.TID(h), Tx: t}
	}
	return out
}

func less(a, b Item) bool {
	if a.Tx.Mint.Serial != b.Tx.Mint.Serial {
		return a.Tx.Mint.Serial < b.Tx.Mint.Serial
	}
	return a.TID.Less(b.TID)
}

// SortItems puts items in canonical order: by mint serial, then by tid.
func SortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool { return less(items[i], items[j]) })
}

// CheckStructure checks the rules that need no ledger state: every
// transaction belongs to a mint of pool, the bundle is in strictly increasing
// canonical order, every transaction passes its stateless checks, and the
// hash meets the series' minimum entry strength. It returns the entry hash
// and the bundle items so callers need not recompute them.
func CheckStructure(s *series.Series, pool uint64, e *Entry) (hash.Hash, []Item, error) {
	if !s.EntriesHaveNonce() && e.Nonce != 0 {
		return hash.Zero, nil, ErrNonce
	}
	items := e.Items(s.Hash())
	for i, it := range items {
		if it.Tx.Mint.Series != s.ID() {
			return hash.Zero, nil, ErrSeriesMismatch
		}
		if it.Tx.Mint.Serial >= s.Mints() || s.PoolOf(it.Tx.Mint.Serial) != pool {
			return hash.Zero, nil, ErrWrongPool
		}
		if i > 0 && !less(items[i-1], it) {
			return hash.Zero, nil, ErrOrder
		}
	}
	tids := make([]hash.Hash, len(items))
	for i, it := range items {
		tids[i] = it.TID
	}
	h := HashOf(s, e.Parent, tids, e.Nonce)
	if h.Strength() < s.MinEntryStrength() {
		return hash.Zero, nil, ErrWeak
	}
	for _, it := range items {
		if err := tx.CheckStateless(s, it.Tx, it.TID); err != nil {
			return hash.Zero, nil, fmt.Errorf("%w %s: %v", ErrInvalidTx, it.TID.Short(), err)
		}
	}
	return h, items, nil
}

// CheckAgainst checks the bundle against v, the pool state after the
// entry's parent. Every transaction is checked against v, never against
// another transaction of the bundle, so a transaction that spends a portion
// a bundle-mate creates is refused as a missing input. Two transactions that
// spend the same portion, or two claims of the same mint, are a conflict.
// CheckStructure must have passed.
func CheckAgainst(s *series.Series, v tx.View, items []Item) error {
	spent := map[tx.Input]struct{}{}
	claims := map[uint64]struct{}{}
	for _, it := range items {
		if it.Tx.IsClaim() {
			if _, dup := claims[it.Tx.Mint.Serial]; dup {
				return ErrConflict
			}
			claims[it.Tx.Mint.Serial] = struct{}{}
		}
		for _, in := range it.Tx.Inputs {
			if _, dup := spent[in]; dup {
				return ErrConflict
			}
			spent[in] = struct{}{}
		}
	}
	for _, it := range items {
		if err := tx.CheckAgainst(s, v, it.Tx, it.TID); err != nil {
			return fmt.Errorf("%w %s: %v", ErrInvalidTx, it.TID.Short(), err)
		}
	}
	return nil
}

// Build makes an entry from transactions that the caller has checked are
// valid against the state after parent, do not conflict and do not depend on
// one another. It sorts them into canonical order and, where the series sets
// a minimum entry strength, tries nonces from 0 until the hash reaches it or
// maxTries nonces have been tried (0 means no limit).
func Build(s *series.Series, parent hash.Hash, txs []*tx.Tx, maxTries uint64) (*Entry, hash.Hash, error) {
	items := make([]Item, len(txs))
	for i, t := range txs {
		items[i] = Item{TID: t.TID(s.Hash()), Tx: t}
	}
	SortItems(items)
	e := &Entry{Parent: parent, Txs: make([]*tx.Tx, len(items))}
	tids := make([]hash.Hash, len(items))
	for i, it := range items {
		e.Txs[i] = it.Tx
		tids[i] = it.TID
	}
	if !s.EntriesHaveNonce() {
		return e, HashOf(s, parent, tids, 0), nil
	}
	min := s.MinEntryStrength()
	for nonce := uint64(0); maxTries == 0 || nonce < maxTries; nonce++ {
		h := HashOf(s, parent, tids, nonce)
		if h.Strength() >= min {
			e.Nonce = nonce
			return e, h, nil
		}
		if nonce == ^uint64(0) {
			break
		}
	}
	return nil, hash.Zero, ErrNotFound
}
