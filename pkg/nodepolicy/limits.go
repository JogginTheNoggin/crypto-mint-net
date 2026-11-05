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

// BoundSeriesCeiling is series_ceiling (D170), settable since D171: the
// largest witness ceiling of a series a policy import may add. 0, the
// default, declines none. It only decides which series this node adds,
// which is a lack of information when it declines, never a verdict on a
// ledger.
var BoundSeriesCeiling = Bound{"series_ceiling", 1024, 1 << 30, 0}

// BoundEntryCacheMiB is entry_cache_mib (D172): the MiB of entry bodies a
// node keeps in memory, read back from its pool logs, across all pools.
// Every held entry's skeleton (what selection, switching and checking
// read) stays in memory whatever this is; only bodies, which no verdict
// reads from the cache, are bounded. A small cache makes serving peers and
// queries read the disk more, never decide differently. The ceiling is
// 1 TiB, enough to keep every body of a busy node in memory.
var BoundEntryCacheMiB = Bound{"entry_cache_mib", 1, 1 << 20, DefaultEntryCacheMiB}

// DefaultEntryCacheMiB is entry_cache_mib's default: 256 MiB counted as
// bodies really take, about what 512 took when bodies were counted at
// twice their size (ORANGE-038, approved by jerry, D173).
const DefaultEntryCacheMiB = 256

// BoundRecentWindow is recent_window (D175): how many entries below each
// pool's selected tip keep their items and undo records in memory. Deeper
// ones are read back from the pool log when a switch or a view reaches
// them, and their undo is recomputed, so a switch at any depth is still
// made, only slower: no depth limit, never a verdict. The floor keeps
// ordinary reorganisations in memory; the ceiling keeps everything.
var BoundRecentWindow = Bound{"recent_window", 16, 1 << 30, DefaultRecentWindow}

// DefaultRecentWindow is recent_window's default.
const DefaultRecentWindow = 10000

// BoundSnapshotEvery is snapshot_every (D178): how many records a pool's
// log takes between the snapshots a running node writes of the pool, so a
// start after a crash replays at most about this many records. A clean
// stop, and every compaction, also writes one. A snapshot is the node's
// own data and decides nothing: a node with none, or set to ignore them
// (no_snapshots, -reverify), replays its logs and ends with the same
// pools, only more slowly. The floor bounds how often a busy pool's
// skeleton is written out; the ceiling is effectively "at stops only".
var BoundSnapshotEvery = Bound{"snapshot_every", 1000, 1 << 30, DefaultSnapshotEvery}

// DefaultSnapshotEvery is snapshot_every's default.
const DefaultSnapshotEvery = 50000

// EntryCacheBytes returns the entry body cache bound this policy asks for.
func (p Policy) EntryCacheBytes() int64 {
	return int64(BoundEntryCacheMiB.Or(p.EntryCacheMiB)) << 20
}

// Bounds lists every bounded setting, for docs, the CLI and the GUI.
func Bounds() []Bound {
	return []Bound{BoundMaxBook, BoundBookPerRange, BoundVerifiedPerRange, BoundRetrySeconds, BoundRegistrySeconds,
		BoundCandidateSeconds, BoundMaxAddresses, BoundFlagHistory, BoundLogSeconds, BoundLogSize,
		BoundSeenSize, BoundMaxPending, BoundTies, BoundSeriesCeiling, BoundEntryCacheMiB, BoundRecentWindow, BoundSnapshotEvery}
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
	// InPlace says whether a running node applies a change of this limit
	// without a reload (InPlace, D168), so a front end need not keep its
	// own list (GUI-GREEN-053).
	InPlace bool `json:"in_place"`
}

// Limits lists every bounded setting of p with its value, in the order
// of Bounds. It is the one list checkBounds checks, so a new bound added
// here is both checked and shown by poo policy limits (GREEN-047).
func (p Policy) Limits() []Limit {
	var out []Limit
	for _, b := range Bounds() {
		v := *p.limitField(b.Name)
		out = append(out, Limit{Name: b.Name, Min: b.Min, Max: b.Max, Default: b.Default, Set: v, Used: b.Or(v), InPlace: InPlace[b.Name]})
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
	case BoundSeriesCeiling.Name:
		return &p.SeriesCeiling
	case BoundEntryCacheMiB.Name:
		return &p.EntryCacheMiB
	case BoundRecentWindow.Name:
		return &p.RecentWindow
	case BoundSnapshotEvery.Name:
		return &p.SnapshotEvery
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
	// Read only by a policy import, from node.json.
	BoundSeriesCeiling.Name: true,
	// The body cache takes its new bound and drops what no longer fits
	// (D172); a reload would start it empty, which costs reads, not
	// verdicts.
	BoundEntryCacheMiB.Name: true,
	// Read at each record a pool's log takes (D178).
	BoundSnapshotEvery.Name: true,
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
