package node

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// ImportFrom names an import as the source of its items in the node's
// events and logs.
const ImportFrom = "import"

// MaxImportItems bounds the items of one import call.
const MaxImportItems = 256

// ImportResult says what an import of entry items did (D188).
type ImportResult struct {
	// Kept and Known count entries taken in and entries already held;
	// LayersKept and LayersKnown the same for their layers.
	Kept        int `json:"kept"`
	Known       int `json:"known"`
	LayersKept  int `json:"layers_kept"`
	LayersKnown int `json:"layers_known"`
	// Refused lists every item that was not taken, with why.
	Refused []ImportRefusal `json:"refused,omitempty"`
	// Switched is set when the selected ledger changed.
	Switched bool `json:"switched,omitempty"`
}

// ImportRefusal is one item an import did not take.
type ImportRefusal struct {
	Index  int       `json:"index"`
	Hash   hash.Hash `json:"hash,omitzero"`
	Reason string    `json:"reason"`
}

// ImportItems puts entry items (wire.EncodeItem forms, oldest first) into
// pool k. Every entry and layer goes through the same checks as one a peer
// sent when asked (engine.Requested): nothing in an import is trusted,
// there is no path that skips a check (jerry's rule, 2026-11-05), and what
// is kept is not forwarded, as for requested data. An entry whose parent
// the node does not hold is refused, so a range must start where the
// node's ledger already reaches.
func (n *Node) ImportItems(k engine.PoolKey, items [][]byte) (ImportResult, error) {
	if len(items) > MaxImportItems {
		return ImportResult{}, fmt.Errorf("node: %d items in one import, at most %d", len(items), MaxImportItems)
	}
	s, ok := n.eng.Series(k.Series)
	if !ok {
		return ImportResult{}, engine.ErrUnknownSeries
	}
	var out ImportResult
	for i, b := range items {
		it, err := wire.DecodeItem(b)
		if err != nil {
			out.Refused = append(out.Refused, ImportRefusal{Index: i, Reason: err.Error()})
			continue
		}
		eh := it.Entry.Hash(s)
		res, err := n.HandleEntryFrom(k, it.Entry, engine.Requested, ImportFrom)
		switch {
		case err == nil:
			out.Kept++
			out.Switched = out.Switched || res.Switch != nil
		case errors.Is(err, ledger.ErrKnownEntry), errors.Is(err, engine.ErrSeen):
			out.Known++
		case errors.Is(err, engine.ErrNotFollowed), errors.Is(err, engine.ErrUnknownSeries):
			return out, err
		default:
			out.Refused = append(out.Refused, ImportRefusal{Index: i, Hash: eh, Reason: err.Error()})
			continue
		}
		for _, nonce := range it.Nonces {
			lr, err := n.HandleLayerFrom(k, layer.Layer{Entry: eh, Nonce: nonce}, engine.Requested, ImportFrom)
			switch {
			case err == nil:
				out.LayersKept++
				out.Switched = out.Switched || lr.Switch != nil
			case errors.Is(err, ledger.ErrKnownLayer), errors.Is(err, engine.ErrSeen):
				out.LayersKnown++
			default:
				out.Refused = append(out.Refused, ImportRefusal{Index: i, Hash: eh, Reason: "layer: " + err.Error()})
			}
		}
	}
	return out, nil
}
