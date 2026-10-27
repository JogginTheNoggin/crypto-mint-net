package wallet

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Node is the full node a wallet reads from and sends to. The wallet
// trusts it for what the selected ledger holds, so a holder runs their own
// or picks one they trust; the node checks every transaction itself.
type Node interface {
	// Holdings returns the unspent portions of a mint at the selected tip.
	Holdings(series hash.Hash, serial uint64) ([]Portion, error)
	// Payment reports where a transaction of a mint stands.
	Payment(series hash.Hash, serial uint64, tid hash.Hash) (Status, error)
	// Submit sends a signed transaction.
	Submit(t *tx.Tx) error
}

// Portion is one unspent portion.
type Portion struct {
	Input   tx.Input  `json:"input"`
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
}

// Status is where a transaction stands on the node.
type Status struct {
	Pending  bool
	Selected bool
	// Entry holds it in the selected ledger, and From is from(Entry): the
	// weight a rival must beat after the entry's parent.
	Entry hash.Hash
	From  weight.Weight
	// Tx is the transaction, when the node holds it.
	Tx *tx.Tx
	// Rivals are the inactive branches the node knows that spend one of
	// the same portions.
	Rivals []Rival
}

// Rival is one known rival of a payment.
type Rival struct {
	Conflict hash.Hash     `json:"conflict"`
	Entry    hash.Hash     `json:"entry"`
	After    weight.Weight `json:"after"`
}
