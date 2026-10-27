// Package watch lets a node's owner look at what the node holds and
// follow it live: the selected ledger of each pool, pending transactions,
// any transaction or address, and a stream of events as items arrive. Every
// view and the stream take a filter.Filter, so the command line, the RPC and
// any graphical front end select items the same way. Views are JSON-ready
// values; nothing here changes the node.
package watch

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Input is a spent portion, with the address it was paid to when the
// witness shows it.
type Input struct {
	TID     hash.Hash  `json:"tid"`
	Index   uint32     `json:"index"`
	Address *hash.Hash `json:"address,omitempty"`
}

// Output is a created portion.
type Output struct {
	Index   uint32    `json:"index"`
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
}

// Tx is a transaction as the views show it.
type Tx struct {
	TID     hash.Hash `json:"tid"`
	Series  hash.Hash `json:"series"`
	Serial  uint64    `json:"serial"`
	Claim   bool      `json:"claim"`
	Inputs  []Input   `json:"inputs"`
	Outputs []Output  `json:"outputs"`
	// Addresses are every address the transaction pays or spends from.
	Addresses []hash.Hash `json:"addresses"`
	Amount    uint64      `json:"amount"`
	Size      int         `json:"size"`
	// Raw is the transaction's byte form in hex.
	Raw string `json:"raw,omitempty"`
}

// TxOf returns the view of t in series s. Witness policies give the
// addresses of the portions a spend uses: each witness proves control of
// the address of the input at its position.
func TxOf(s *series.Series, t *tx.Tx) Tx {
	h := s.Hash()
	v := Tx{TID: t.TID(h), Series: t.Mint.Series, Serial: t.Mint.Serial, Claim: t.IsClaim(), Inputs: []Input{}, Outputs: []Output{}}
	seen := map[hash.Hash]bool{}
	addAddr := func(a hash.Hash) {
		if !seen[a] {
			seen[a] = true
			v.Addresses = append(v.Addresses, a)
		}
	}
	for i, in := range t.Inputs {
		vi := Input{TID: in.TID, Index: in.Index}
		if !v.Claim && i < len(t.Witnesses) {
			a := policy.Address(h, t.Witnesses[i].Policy)
			vi.Address = &a
			addAddr(a)
		}
		v.Inputs = append(v.Inputs, vi)
	}
	for i, out := range t.Outputs {
		v.Outputs = append(v.Outputs, Output{Index: uint32(i), Address: out.Address, Amount: out.Amount})
		v.Amount += out.Amount
		addAddr(out.Address)
	}
	if v.Addresses == nil {
		v.Addresses = []hash.Hash{}
	}
	b := t.Encode()
	v.Size = len(b)
	return v
}

// Subject returns what a filter matches a transaction against.
func (v Tx) Subject(s *series.Series, k filter.Kind) filter.Subject {
	return filter.Subject{
		Series: v.Series, Hash: s.Hash(), Pool: s.PoolOf(v.Serial), HasPool: true,
		Serials: []uint64{v.Serial}, Addresses: v.Addresses, TIDs: []hash.Hash{v.TID}, Kind: k,
	}
}

// subjectOf joins the subjects of several transactions of one pool.
func subjectOf(s *series.Series, pool uint64, k filter.Kind, txs []Tx, tids []hash.Hash) filter.Subject {
	sub := filter.Subject{Series: s.ID(), Hash: s.Hash(), Pool: pool, HasPool: true, Kind: k, TIDs: append([]hash.Hash(nil), tids...)}
	serials := map[uint64]bool{}
	for _, t := range txs {
		if !serials[t.Serial] {
			serials[t.Serial] = true
			sub.Serials = append(sub.Serials, t.Serial)
		}
		sub.Addresses = append(sub.Addresses, t.Addresses...)
		sub.TIDs = append(sub.TIDs, t.TID)
	}
	return sub
}

// txsOf returns the views of an entry's transactions.
func txsOf(s *series.Series, e *entry.Entry) []Tx {
	out := make([]Tx, len(e.Txs))
	for i, t := range e.Txs {
		out[i] = TxOf(s, t)
	}
	return out
}
