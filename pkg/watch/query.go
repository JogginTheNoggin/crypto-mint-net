package watch

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Limits of one page of a view.
const (
	DefaultLimit = 50
	MaxLimit     = 1000
)

// ErrNotFound is returned for a transaction the node does not hold.
var ErrNotFound = errors.New("watch: not held by this node")

// Pool is one followed pool, as the pool list shows it.
type Pool struct {
	Series      hash.Hash     `json:"series"`
	Pool        uint64        `json:"pool"`
	FirstSerial uint64        `json:"first_serial"`
	LastSerial  uint64        `json:"last_serial"`
	Tip         hash.Hash     `json:"tip"`
	Height      int           `json:"height"`
	Weight      weight.Weight `json:"weight"`
	Entries     int           `json:"entries"`
	Branches    int           `json:"branches"`
	Pending     int           `json:"pending"`
	// Waiting are transactions waiting on an unknown input.
	Waiting int `json:"waiting"`
	// StoredRecords is how many records the pool's store holds; the
	// engine does not know, so the node's interface fills it in.
	StoredRecords int `json:"stored_records"`
}

// Pools lists the followed pools that f's series, pool and serial fields
// select, by series id and then index.
func Pools(e *engine.Engine, f filter.Filter) []Pool {
	var out []Pool
	for _, k := range e.ListedPools() {
		s, _ := e.Series(k.Series)
		first, last := s.PoolRange(k.Index)
		if !f.MatchScope(k.Series, k.Index, first, last) {
			continue
		}
		st, err := e.Status(k)
		if err != nil {
			continue
		}
		out = append(out, Pool{
			Series: k.Series, Pool: k.Index, FirstSerial: first, LastSerial: last,
			Tip: st.Tip, Height: st.Height, Weight: st.Weight, Entries: st.Entries,
			Branches: len(st.Branches), Pending: st.Pending, Waiting: st.Waiting,
		})
	}
	return out
}

// Entry is an entry of the selected ledger, with its transactions.
type Entry struct {
	Hash      hash.Hash     `json:"hash"`
	Parent    hash.Hash     `json:"parent"`
	Height    int           `json:"height"`
	Own       weight.Weight `json:"own"`
	Ledger    weight.Weight `json:"ledger"`
	From      weight.Weight `json:"from"`
	Layers    int           `json:"layers"`
	FirstSeen time.Time     `json:"first_seen"`
	TxCount   int           `json:"tx_count"`
	// Txs are the entry's transactions that the filter selects.
	Txs []Tx `json:"txs"`
}

// Ledger is one page of a pool's selected ledger, newest first.
type Ledger struct {
	Series  hash.Hash     `json:"series"`
	Pool    uint64        `json:"pool"`
	Height  int           `json:"height"`
	Weight  weight.Weight `json:"weight"`
	Entries []Entry       `json:"entries"`
	// Next is the height to ask for the next, older page; 0 when there is
	// none.
	Next int `json:"next"`
}

// LedgerPage returns entries of pool k's selected ledger from height
// below down, newest first (below 0: from the tip), at most limit of them.
// When f selects items (addresses, policies, tids, serials), only entries
// holding a selected transaction are listed, with only those
// transactions; every entry is still looked at, so Next moves past entries
// left out.
func LedgerPage(e *engine.Engine, k engine.PoolKey, f filter.Filter, below, limit int) (Ledger, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	s, ok := e.Series(k.Series)
	if !ok {
		return Ledger{}, engine.ErrUnknownSeries
	}
	out := Ledger{Series: k.Series, Pool: k.Index, Entries: []Entry{}}
	itemLevel := len(f.Addresses) > 0 || len(f.Policies) > 0 || len(f.TIDs) > 0 || len(f.Serials) > 0
	err := e.Ledger(k, func(p *ledger.Pool) {
		out.Height, out.Weight = p.Height(), p.Weight()
		h := out.Height
		if below > 0 && below <= h {
			h = below
		}
		// Look at a bounded number of entries per page, so a filter that
		// matches nothing cannot make one request walk the whole ledger.
		looked := 0
		for ; h >= 1 && len(out.Entries) < limit && looked < 50*limit; h-- {
			looked++
			eh := p.Chain(h, h)[0]
			en, _ := p.Entry(eh)
			info, _ := p.Info(eh)
			from, _ := p.From(eh)
			v := Entry{Hash: eh, Parent: info.Parent, Height: h, Own: info.Own, Ledger: info.Ledger, From: from,
				Layers: info.Layers, FirstSeen: info.FirstSeen, TxCount: len(en.Txs), Txs: []Tx{}}
			for _, t := range txsOf(s, en) {
				if f.Match(t.Subject(s, "")) {
					v.Txs = append(v.Txs, t)
				}
			}
			if itemLevel && len(v.Txs) == 0 {
				continue
			}
			out.Entries = append(out.Entries, v)
		}
		if h >= 1 {
			out.Next = h
		}
	})
	return out, err
}

// PendingTx is a pending transaction and its pool.
type PendingTx struct {
	Pool uint64 `json:"pool"`
	Tx
}

// Pending lists the pending transactions that f selects, in every followed
// pool f's scope covers, oldest first within each pool.
func Pending(e *engine.Engine, f filter.Filter) []PendingTx {
	out := []PendingTx{}
	for _, k := range e.ListedPools() {
		s, _ := e.Series(k.Series)
		first, last := s.PoolRange(k.Index)
		if !f.MatchScope(k.Series, k.Index, first, last) {
			continue
		}
		items, err := e.Pending(k)
		if err != nil {
			continue
		}
		for _, it := range items {
			v := TxOf(s, it.Tx)
			if f.Match(v.Subject(s, "")) {
				out = append(out, PendingTx{Pool: k.Index, Tx: v})
			}
		}
	}
	return out
}

// Rival is an inactive branch holding a transaction that spends one of the
// same portions.
type Rival struct {
	Tip      hash.Hash     `json:"tip"`
	Fork     hash.Hash     `json:"fork"`
	Entry    hash.Hash     `json:"entry"`
	Conflict hash.Hash     `json:"conflict"`
	After    weight.Weight `json:"after"`
}

// TxStatus is everything the node knows about one transaction.
type TxStatus struct {
	Pool     uint64        `json:"pool"`
	Pending  bool          `json:"pending"`
	Selected bool          `json:"selected"`
	Entry    *hash.Hash    `json:"entry,omitempty"`
	Height   int           `json:"height,omitempty"`
	From     weight.Weight `json:"from"`
	Rivals   []Rival       `json:"rivals"`
	Tx       *Tx           `json:"tx,omitempty"`
}

// TxOfSeries finds transaction tid in the followed pools of series id.
// With serial set (not nil), only that mint's pool is looked in.
func TxOfSeries(e *engine.Engine, id hash.Hash, tid hash.Hash, serial *uint64) (TxStatus, error) {
	s, ok := e.Series(id)
	if !ok {
		return TxStatus{}, engine.ErrUnknownSeries
	}
	for _, k := range e.ListedPools() {
		if k.Series != id || (serial != nil && k.Index != s.PoolOf(*serial)) {
			continue
		}
		st, ok, err := txIn(e, s, k, tid)
		if err != nil || ok {
			return st, err
		}
	}
	return TxStatus{}, ErrNotFound
}

func txIn(e *engine.Engine, s *series.Series, k engine.PoolKey, tid hash.Hash) (TxStatus, bool, error) {
	pay, err := e.Payment(k, tid)
	if err != nil || (pay.Tx == nil && !pay.Pending && !pay.Selected) {
		return TxStatus{}, false, err
	}
	st := TxStatus{Pool: k.Index, Pending: pay.Pending, Selected: pay.Selected, From: pay.From, Rivals: []Rival{}}
	if pay.Selected {
		eh := pay.Entry
		st.Entry = &eh
		e.Ledger(k, func(p *ledger.Pool) { st.Height, _ = p.HeightOf(eh) })
	}
	if pay.Tx != nil {
		v := TxOf(s, pay.Tx)
		v.Raw = hex.EncodeToString(pay.Tx.Encode())
		st.Tx = &v
	}
	for _, r := range pay.Rivals {
		st.Rivals = append(st.Rivals, Rival{Tip: r.Tip, Fork: r.Fork, Entry: r.Entry, Conflict: r.Conflict, After: r.After})
	}
	return st, true, nil
}

// Holding is an unspent portion.
type Holding struct {
	Input   tx.Input  `json:"input"`
	Serial  uint64    `json:"serial"`
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
}

// HistoryItem is a selected transaction that paid or spent from an
// address, with what it did to the address.
type HistoryItem struct {
	Pool     uint64        `json:"pool"`
	Entry    hash.Hash     `json:"entry"`
	Height   int           `json:"height"`
	From     weight.Weight `json:"from"`
	Received uint64        `json:"received"`
	// Spends counts the address's portions the transaction spent.
	Spends int `json:"spends"`
	Tx     Tx  `json:"tx"`
}

// Holdings lists the unspent portions that f selects, in every followed
// pool f's scope covers. Address and policy fields select by the
// portion's address; tids by the transaction that made the portion.
func Holdings(e *engine.Engine, f filter.Filter) []Holding {
	out := []Holding{}
	for _, k := range e.ListedPools() {
		s, _ := e.Series(k.Series)
		first, last := s.PoolRange(k.Index)
		if !f.MatchScope(k.Series, k.Index, first, last) {
			continue
		}
		e.Ledger(k, func(p *ledger.Pool) {
			p.State().ForEach(func(in tx.Input, po tx.Portion) {
				sub := filter.Subject{Series: k.Series, Hash: s.Hash(), Pool: k.Index, HasPool: true,
					Serials: []uint64{po.Serial}, Addresses: []hash.Hash{po.Address}, TIDs: []hash.Hash{in.TID}}
				if f.Match(sub) {
					out = append(out, Holding{Input: in, Serial: po.Serial, Address: po.Address, Amount: po.Amount})
				}
			})
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Serial != out[j].Serial {
			return out[i].Serial < out[j].Serial
		}
		if c := out[i].Input.TID.Compare(out[j].Input.TID); c != 0 {
			return c < 0
		}
		return out[i].Input.Index < out[j].Input.Index
	})
	return out
}

// Address is what the node knows about one address in one series.
type Address struct {
	Series   hash.Hash     `json:"series"`
	Address  hash.Hash     `json:"address"`
	Balance  uint64        `json:"balance"`
	Holdings []Holding     `json:"holdings"`
	History  []HistoryItem `json:"history"`
	Pending  []PendingTx   `json:"pending"`
	// Next lists the pools whose history was not walked to height 1, with
	// the height to pass as below for the rest (narrow the scope to that
	// pool).
	Next []PoolNext `json:"next,omitempty"`
}

// PoolNext is where the history of one pool continues.
type PoolNext struct {
	Pool  uint64 `json:"pool"`
	Below int    `json:"below"`
}

// AddressOf returns the holdings, selected history (newest first, at most
// limit items) and pending transactions of address a in the followed pools
// of series id that f's scope covers. It is AddressPage from the tips.
func AddressOf(e *engine.Engine, id, a hash.Hash, f filter.Filter, limit int) (Address, error) {
	return AddressPage(context.Background(), e, id, a, f, 0, limit)
}

// AddressPage is AddressOf for the history from height below down in
// every pool (0: from the tips), as LedgerPage reads below. Like LedgerPage it looks at no more than
// 50*limit entries in all, so a request cannot walk whole ledgers while
// holding the engine's lock; Next says where each unfinished pool goes
// on. It stops with ctx's error once ctx is done (RED-008, decision D103).
func AddressPage(ctx context.Context, e *engine.Engine, id, a hash.Hash, f filter.Filter, below, limit int) (Address, error) {
	s, ok := e.Series(id)
	if !ok {
		return Address{}, engine.ErrUnknownSeries
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	f.Series, f.Addresses, f.Policies, f.Kinds = []hash.Hash{id}, []hash.Hash{a}, nil, nil
	out := Address{Series: id, Address: a, Holdings: Holdings(e, f), History: []HistoryItem{}, Pending: Pending(e, f)}
	for _, h := range out.Holdings {
		out.Balance += h.Amount
	}
	budget := 50 * limit
	for _, k := range e.ListedPools() {
		first, last := s.PoolRange(k.Index)
		if k.Series != id || !f.MatchScope(id, k.Index, first, last) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Address{}, err
		}
		var err error
		e.Ledger(k, func(p *ledger.Pool) {
			h := p.Height()
			if below > 0 && below <= h {
				h = below
			}
			// A pool's history comes newest first, so more than limit
			// items from one pool are never needed.
			found := 0
			for ; h >= 1 && found < limit && budget > 0; h-- {
				budget--
				if budget%256 == 0 {
					if err = ctx.Err(); err != nil {
						return
					}
				}
				eh := p.Chain(h, h)[0]
				en, _ := p.Entry(eh)
				for _, t := range txsOf(s, en) {
					if !f.Match(t.Subject(s, "")) {
						continue
					}
					it := HistoryItem{Pool: k.Index, Entry: eh, Height: h, Tx: t}
					it.From, _ = p.From(eh)
					for _, o := range t.Outputs {
						if o.Address == a {
							it.Received += o.Amount
						}
					}
					for _, in := range t.Inputs {
						if in.Address != nil && *in.Address == a {
							it.Spends++
						}
					}
					out.History = append(out.History, it)
					found++
				}
			}
			if h >= 1 {
				out.Next = append(out.Next, PoolNext{Pool: k.Index, Below: h})
			}
		})
		if err != nil {
			return Address{}, err
		}
	}
	sort.SliceStable(out.History, func(i, j int) bool {
		return out.History[i].From.Cmp(out.History[j].From) < 0
	})
	if len(out.History) > limit {
		out.History, out.Next = truncateHistory(out.History, out.Next, limit)
	}
	return out, nil
}

// truncateHistory keeps the newest items of a page, whole entries at a
// time, up to limit (or the first entry, if it alone holds more), and
// moves each pool's Next up to the newest entry it did not keep, so that
// following Next reaches every dropped item exactly once (GREEN-014).
// Within one pool the items are newest first, so what a pool keeps is
// always a run of its newest entries.
func truncateHistory(hist []HistoryItem, next []PoolNext, limit int) ([]HistoryItem, []PoolNext) {
	kept := 0
	for i := 0; i < len(hist); {
		j := i + 1
		for j < len(hist) && hist[j].Pool == hist[i].Pool && hist[j].Height == hist[i].Height {
			j++
		}
		if kept > 0 && kept+(j-i) > limit {
			break
		}
		kept = j
		i = j
	}
	resume := map[uint64]int{}
	for _, h := range hist[kept:] {
		if h.Height > resume[h.Pool] {
			resume[h.Pool] = h.Height
		}
	}
	out := make([]PoolNext, 0, len(next)+len(resume))
	for _, n := range next {
		if _, ok := resume[n.Pool]; !ok {
			out = append(out, n)
		}
	}
	for pool, h := range resume {
		out = append(out, PoolNext{Pool: pool, Below: h})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pool < out[j].Pool })
	return hist[:kept], out
}
