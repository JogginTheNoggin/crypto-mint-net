package nodepolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/mempool"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// Bound is the range a node policy setting may take. Zero in node.json
// always means the default; any other value must lie in [Min, Max].
type Bound struct {
	// Name is the setting's place in node.json, such as "p2p.max_book".
	Name     string
	Min, Max int
	// Default is what zero means.
	Default int
}

// The bounds of the settable peer book, tracker and discovery limits
// (D159). The floors keep a node able to find and keep peers; the ceilings
// bound the memory and disk a book or tracker can take. None of them
// touches which ledger a node selects: a node with a small book or a short
// memory of offences only knows fewer peers, which is a lack of
// information, never a hard fork.
var (
	BoundMaxBook          = Bound{"p2p.max_book", 256, 65536, peers.DefaultLimits().MaxRecords}
	BoundBookPerRange     = Bound{"p2p.book_per_range", 1, 4096, peers.DefaultLimits().PerRange}
	BoundVerifiedPerRange = Bound{"p2p.verified_per_range", 1, 64, peers.DefaultLimits().VerifiedPerRange}
	BoundRetrySeconds     = Bound{"p2p.retry_seconds", 1, 86400, int(peers.DefaultLimits().Retry / time.Second)}
	BoundRegistrySeconds  = Bound{"p2p.registry_seconds", 60, 86400, 3600}
	BoundMaxAddresses     = Bound{"tracking.max_addresses", 1024, 131072, tracking.DefaultConfig().Retain.MaxAddresses}
	BoundFlagHistory      = Bound{"tracking.flag_history_seconds", 3600, 365 * 86400, int(tracking.DefaultConfig().Retain.FlagHistory / time.Second)}
	BoundLogSeconds       = Bound{"tracking.log_seconds", 60, 86400, int(tracking.DefaultConfig().Retain.Log / time.Second)}
	BoundLogSize          = Bound{"tracking.log_size", 16, 4096, tracking.DefaultConfig().Retain.LogSize}
	// BoundCandidateSeconds is how long an address learned from the
	// network that this node has never reached keeps its place against a
	// newer address when the book or its range is full (RED-064, D160).
	BoundCandidateSeconds = Bound{"p2p.candidate_seconds", 60, 365 * 86400, int(peers.DefaultLimits().CandidateAge / time.Second)}
)

// The bounds of the engine's and the pools' memory limits (RED-063, D160).
// The seen list only saves checking an item twice, the pending pool holds
// transactions not yet in an entry, and ties bounds the off-chain entries
// kept that tie with the selected ledger's weight: an entry that is
// dropped or forgotten can be sent again. None of them bounds the weight,
// depth or length of a ledger a node takes, so a smaller value is a lack of
// information, never a hard fork. The ceilings keep one setting from
// taking the machine's memory at start or under a flood.
var (
	BoundSeenSize   = Bound{"seen_size", 1024, 1 << 22, engine.DefaultSeenSize}
	BoundMaxPending = Bound{"max_pending", 16, 1 << 20, mempool.DefaultLimit}
	BoundTies       = Bound{"ties", 1, 4096, ledger.DefaultTies}
)

// Bounds lists every bounded setting, for docs, the CLI and the GUI.
func Bounds() []Bound {
	return []Bound{BoundMaxBook, BoundBookPerRange, BoundVerifiedPerRange, BoundRetrySeconds, BoundRegistrySeconds,
		BoundCandidateSeconds, BoundMaxAddresses, BoundFlagHistory, BoundLogSeconds, BoundLogSize,
		BoundSeenSize, BoundMaxPending, BoundTies}
}

// Check refuses v when it is neither zero (the default) nor in range.
func (b Bound) Check(v int) error {
	if v == 0 || (v >= b.Min && v <= b.Max) {
		return nil
	}
	return fmt.Errorf("%w: %s is %d; it takes %d to %d (0 or left out: %d)", ErrInvalid, b.Name, v, b.Min, b.Max, b.Default)
}

// Or returns v, or the default when v is zero.
func (b Bound) Or(v int) int {
	if v == 0 {
		return b.Default
	}
	return v
}

// Limit is one bounded setting of a node policy: its bound, the value
// node.json gives (0: left out) and the value the node uses.
type Limit struct {
	Name    string `json:"name"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
	Default int    `json:"default"`
	Set     int    `json:"set"`
	Used    int    `json:"used"`
}

// Limits lists every bounded setting of p with its value, in the order
// of Bounds. It is the one list checkBounds checks, so a new bound added
// here is both checked and shown by poo policy limits (GREEN-047).
func (p Policy) Limits() []Limit {
	var out []Limit
	for _, b := range Bounds() {
		v := *p.limitField(b.Name)
		out = append(out, Limit{Name: b.Name, Min: b.Min, Max: b.Max, Default: b.Default, Set: v, Used: b.Or(v)})
	}
	return out
}

// bound returns the Bound a Limit came from.
func (l Limit) bound() Bound { return Bound{l.Name, l.Min, l.Max, l.Default} }

// checkBounds checks every bounded setting of p, and that a range never
// holds more than the whole book.
func (p Policy) checkBounds() error {
	for _, l := range p.Limits() {
		if err := l.bound().Check(l.Set); err != nil {
			return err
		}
	}
	if book, per := BoundMaxBook.Or(p.P2P.MaxBook), BoundBookPerRange.Or(p.P2P.BookPerRange); per > book {
		return fmt.Errorf("%w: p2p.book_per_range (%d) is more than p2p.max_book (%d)", ErrInvalid, per, book)
	}
	return nil
}

// BookLimits returns the peer book limits this policy asks for. The places
// for imported addresses stay a quarter of the book (D158).
func (p Policy) BookLimits() peers.Limits {
	book := BoundMaxBook.Or(p.P2P.MaxBook)
	return peers.Limits{
		MaxRecords:       book,
		PerRange:         BoundBookPerRange.Or(p.P2P.BookPerRange),
		VerifiedPerRange: BoundVerifiedPerRange.Or(p.P2P.VerifiedPerRange),
		Retry:            time.Duration(BoundRetrySeconds.Or(p.P2P.RetrySeconds)) * time.Second,
		MaxUnlisted:      max(1, book/4),
		CandidateAge:     time.Duration(BoundCandidateSeconds.Or(p.P2P.CandidateSeconds)) * time.Second,
	}
}

// retention returns the tracker's retention this policy asks for.
func (p Policy) retention() tracking.Retention {
	t := p.Tracking
	return tracking.Retention{
		FlagHistory:  time.Duration(BoundFlagHistory.Or(t.FlagHistorySeconds)) * time.Second,
		Log:          time.Duration(BoundLogSeconds.Or(t.LogSeconds)) * time.Second,
		LogSize:      BoundLogSize.Or(t.LogSize),
		MaxAddresses: BoundMaxAddresses.Or(t.MaxAddresses),
	}
}

// limitField returns the field of p a bounded setting is kept in; every
// bound of Bounds has one (TestSetLimits).
func (p *Policy) limitField(name string) *int {
	switch name {
	case BoundMaxBook.Name:
		return &p.P2P.MaxBook
	case BoundBookPerRange.Name:
		return &p.P2P.BookPerRange
	case BoundVerifiedPerRange.Name:
		return &p.P2P.VerifiedPerRange
	case BoundRetrySeconds.Name:
		return &p.P2P.RetrySeconds
	case BoundRegistrySeconds.Name:
		return &p.P2P.RegistrySeconds
	case BoundCandidateSeconds.Name:
		return &p.P2P.CandidateSeconds
	case BoundSeenSize.Name:
		return &p.SeenSize
	case BoundMaxPending.Name:
		return &p.MaxPending
	case BoundTies.Name:
		return &p.Ties
	case BoundMaxAddresses.Name:
		return &p.Tracking.MaxAddresses
	case BoundFlagHistory.Name:
		return &p.Tracking.FlagHistorySeconds
	case BoundLogSeconds.Name:
		return &p.Tracking.LogSeconds
	case BoundLogSize.Name:
		return &p.Tracking.LogSize
	}
	return nil
}

// SetLimits sets bounded settings by name (D161; poo policy limits -set):
// 0 puts one back to its default. The result is checked whole, so a
// setting that goes with another (p2p.book_per_range and p2p.max_book)
// may be set in the same call; on an error p is unchanged. It reports
// whether anything changed.
func (p *Policy) SetLimits(set map[string]int) (bool, error) {
	q := *p
	changed := false
	for name, v := range set {
		f := q.limitField(name)
		if f == nil {
			return false, fmt.Errorf("%w: %q is not a settable limit (poo policy limits lists them)", ErrInvalid, name)
		}
		if *f != v {
			*f, changed = v, true
		}
	}
	if err := q.checkBounds(); err != nil {
		return false, err
	}
	*p = q
	return changed, nil
}

// Retention returns the tracker's retention bounds this policy asks for.
func (p Policy) Retention() tracking.Retention { return p.retention() }

// InPlace names the bounded settings a running node applies without a
// reload (ORANGE-035's design, accepted by Black in Round 25, D168): the
// peer book's limits and the tracker's retention, which are information,
// and the seen list, which only saves work. Each part ends as a reload
// onto the same node.json would leave it. The others (verified_per_range,
// registry_seconds, max_pending, ties) stay on the reload until each has
// its own in-place path and comparison test: a reload makes every verified
// peer a candidate again, so a lowered verified_per_range holds after a
// reload but not in place.
var InPlace = map[string]bool{
	BoundMaxBook.Name: true, BoundBookPerRange.Name: true,
	BoundRetrySeconds.Name: true, BoundCandidateSeconds.Name: true,
	BoundMaxAddresses.Name: true, BoundFlagHistory.Name: true, BoundLogSeconds.Name: true, BoundLogSize.Name: true,
	BoundSeenSize.Name: true,
}

// ChangedLimits lists the bounded settings whose used value differs
// between old and new, in the order of Bounds.
func ChangedLimits(old, new Policy) []string {
	var out []string
	o := old.Limits()
	for i, l := range new.Limits() {
		if l.Used != o[i].Used {
			out = append(out, l.Name)
		}
	}
	return out
}

// SameButInPlace reports whether a and b differ only in what a running
// node applies without a reload: the bounded settings InPlace names. An
// in-place change of limits is taken only when the node.json on disk and
// the policy the node runs with are the same in all else, so a hand edit
// waiting for a reload is never left unapplied (D168). The grind schedule
// and p2p.peers are compared too: the RPC edits that apply them at once
// write them into the running policy (node.UpdateRunning), so a
// difference there is a hand edit, which waits for the reload (RED-080).
func SameButInPlace(a, b Policy) bool {
	strip := func(p Policy) ([]byte, error) {
		for name := range InPlace {
			*p.limitField(name) = 0
		}
		p.Lowered = nil
		return json.Marshal(p)
	}
	x, err1 := strip(a)
	y, err2 := strip(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}
