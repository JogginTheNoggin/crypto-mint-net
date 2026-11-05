package watch

import (
	"slices"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Ledger export (D188): a stretch of a pool's selected ledger, oldest
// first, as the raw entry items a peer would send (each entry's byte form
// and its layer nonces), with a readable summary of each. An import puts
// every item through the node's full checks, as for an entry a peer sent;
// nothing in the file is trusted.

// ExportLimit and ExportBytes bound one page: at most this many entries,
// and no more entries once the items reach this many bytes (one entry is
// always sent, whatever its size).
const (
	ExportLimit = 256
	ExportBytes = 8 << 20
)

// exportLimit is ExportLimit; tests lower it to page short ledgers.
var exportLimit = ExportLimit

// ExportEntry is the readable summary of one exported entry.
type ExportEntry struct {
	Height  int       `json:"height"`
	Hash    hash.Hash `json:"hash"`
	Parent  hash.Hash `json:"parent"`
	TxCount int       `json:"tx_count"`
	Layers  int       `json:"layers"`
}

// ExportPage is one page of an export.
type ExportPage struct {
	Series hash.Hash     `json:"series"`
	Pool   uint64        `json:"pool"`
	Height int           `json:"height"` // the selected ledger's height
	Tip    hash.Hash     `json:"tip"`
	Weight weight.Weight `json:"weight"`
	// Entries summarise Items, one for one, oldest first.
	Entries []ExportEntry `json:"entries"`
	// Items are the entry items' byte forms (wire.EncodeItem, with
	// layers), oldest first.
	Items [][]byte `json:"items"`
	// Next is the height to ask for next; 0 when the range is done.
	Next int `json:"next"`
	// Unreadable counts entries whose bodies could not be read back from
	// the log; the page stops before the first one, and Next is 0.
	Unreadable int `json:"unreadable,omitempty"`
}

// Export returns pool k's selected ledger from height from up to height to
// (0: the tip), oldest first, one page at a time.
func Export(e *engine.Engine, k engine.PoolKey, from, to int) (ExportPage, error) {
	if from < 1 {
		from = 1
	}
	out := ExportPage{Series: k.Series, Pool: k.Index, Entries: []ExportEntry{}, Items: [][]byte{}}
	type taken struct {
		h      int
		hash   hash.Hash
		parent hash.Hash
		layers int
		ref    ledger.BodyRef
		nonces []uint64
	}
	var take []taken
	err := e.Ledger(k, func(p *ledger.Pool) {
		out.Height, out.Weight = p.Height(), p.Weight()
		out.Tip, _, _ = p.Tip()
		end := out.Height
		if to > 0 && to < end {
			end = to
		}
		if from > end {
			return
		}
		last := min(end, from+exportLimit-1)
		for i, eh := range p.Chain(from, last) {
			info, _ := p.Info(eh)
			ref, _ := p.Ref(eh)
			take = append(take, taken{h: from + i, hash: eh, parent: info.Parent, layers: info.Layers, ref: ref, nonces: p.LayerNonces(eh)})
		}
		if last < end {
			out.Next = last + 1
		}
	})
	if err != nil {
		return out, err
	}
	// Bodies are read outside the engine's lock (ORANGE-038, D174).
	size := 0
	for i, t := range take {
		en, rerr := t.ref.Get()
		if rerr != nil {
			out.Unreadable++
			out.Next = 0
			break
		}
		if i > 0 && size >= ExportBytes {
			out.Next = t.h
			break
		}
		nonces := slices.Clone(t.nonces)
		slices.Sort(nonces)
		b := wire.EncodeItem(wire.EntryItem{Entry: en, HasLayers: true, Nonces: nonces})
		size += len(b)
		out.Items = append(out.Items, b)
		out.Entries = append(out.Entries, ExportEntry{Height: t.h, Hash: t.hash, Parent: t.parent, TxCount: len(en.Txs), Layers: t.layers})
	}
	return out, nil
}
