package nodepolicy

import (
	"fmt"
	"time"

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
)

// Bounds lists every bounded setting, for docs, the CLI and the GUI.
func Bounds() []Bound {
	return []Bound{BoundMaxBook, BoundBookPerRange, BoundVerifiedPerRange, BoundRetrySeconds, BoundRegistrySeconds,
		BoundMaxAddresses, BoundFlagHistory, BoundLogSeconds, BoundLogSize}
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

// checkBounds checks every bounded setting of p, and that a range never
// holds more than the whole book.
func (p Policy) checkBounds() error {
	for _, c := range []struct {
		b Bound
		v int
	}{
		{BoundMaxBook, p.P2P.MaxBook}, {BoundBookPerRange, p.P2P.BookPerRange},
		{BoundVerifiedPerRange, p.P2P.VerifiedPerRange}, {BoundRetrySeconds, p.P2P.RetrySeconds},
		{BoundRegistrySeconds, p.P2P.RegistrySeconds},
		{BoundMaxAddresses, p.Tracking.MaxAddresses}, {BoundFlagHistory, p.Tracking.FlagHistorySeconds},
		{BoundLogSeconds, p.Tracking.LogSeconds}, {BoundLogSize, p.Tracking.LogSize},
	} {
		if err := c.b.Check(c.v); err != nil {
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
