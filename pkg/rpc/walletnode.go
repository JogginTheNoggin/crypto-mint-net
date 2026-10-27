package rpc

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// WalletNode is a wallet.Node over this interface: a wallet reading from
// and sending to a running node.
type WalletNode struct {
	C     *Client
	tidOf func(*tx.Tx) hash.Hash
}

// NewWalletNode returns the node behind c for a wallet; tidOf computes a
// transaction's tid under its series.
func NewWalletNode(c *Client, tidOf func(*tx.Tx) hash.Hash) *WalletNode {
	return &WalletNode{C: c, tidOf: tidOf}
}

// Holdings implements wallet.Node.
func (n *WalletNode) Holdings(series hash.Hash, serial uint64) ([]wallet.Portion, error) {
	hs, err := n.C.Holdings(series, serial)
	if err != nil {
		return nil, err
	}
	out := make([]wallet.Portion, len(hs))
	for i, h := range hs {
		out[i] = wallet.Portion{Input: tx.Input{TID: h.TID, Index: h.Index}, Address: h.Address, Amount: h.Amount}
	}
	return out, nil
}

// Payment implements wallet.Node.
func (n *WalletNode) Payment(series hash.Hash, serial uint64, tid hash.Hash) (wallet.Status, error) {
	r, err := n.C.Payment(series, serial, tid)
	if err != nil {
		return wallet.Status{}, err
	}
	st := wallet.Status{Pending: r.Pending, Selected: r.Selected}
	if r.Entry != nil {
		st.Entry = *r.Entry
	}
	if err := st.From.UnmarshalText([]byte(r.From)); err != nil {
		return st, fmt.Errorf("rpc: from: %w", err)
	}
	if r.Tx != "" {
		b, err := hex.DecodeString(r.Tx)
		if err != nil {
			return st, err
		}
		t, err := tx.Decode(b)
		if err != nil {
			return st, err
		}
		if n.tidOf != nil && n.tidOf(t) != tid {
			return st, errors.New("rpc: the node returned another transaction")
		}
		st.Tx = t
	}
	for _, rv := range r.Rivals {
		var a weight.Weight
		if err := a.UnmarshalText([]byte(rv.After)); err != nil {
			return st, err
		}
		st.Rivals = append(st.Rivals, wallet.Rival{Conflict: rv.Conflict, Entry: rv.Entry, After: a})
	}
	return st, nil
}

// Submit implements wallet.Node. A transaction the node already holds
// counts as sent.
func (n *WalletNode) Submit(t *tx.Tx) error {
	_, err := n.C.SubmitTx(t)
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) && e.Code == CodeConflict && n.tidOf != nil {
		tid := n.tidOf(t)
		if p, perr := n.C.Payment(t.Mint.Series, t.Mint.Serial, tid); perr == nil && (p.Pending || p.Selected) {
			return nil
		}
	}
	return err
}

var _ wallet.Node = (*WalletNode)(nil)
