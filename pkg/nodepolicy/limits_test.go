package nodepolicy

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// Left out, every bounded setting is the package default (D159).
func TestLimitsDefaults(t *testing.T) {
	var p Policy
	if got, want := p.BookLimits(), peers.DefaultLimits(); got != want {
		t.Errorf("book limits %+v, want %+v", got, want)
	}
	if got, want := p.TrackingConfig().Retain, tracking.DefaultConfig().Retain; got != want {
		t.Errorf("retention %+v, want %+v", got, want)
	}
	for _, b := range Bounds() {
		if b.Default < b.Min || b.Default > b.Max {
			t.Errorf("%s: default %d outside %d..%d", b.Name, b.Default, b.Min, b.Max)
		}
	}
}

func TestLimitsSet(t *testing.T) {
	var p Policy
	p.P2P.MaxBook, p.P2P.BookPerRange, p.P2P.VerifiedPerRange, p.P2P.RetrySeconds = 20000, 64, 8, 30
	p.Tracking.MaxAddresses, p.Tracking.FlagHistorySeconds, p.Tracking.LogSeconds, p.Tracking.LogSize = 50000, 86400, 600, 1000
	if err := p.checkBounds(); err != nil {
		t.Fatal(err)
	}
	p.P2P.CandidateSeconds = 7200
	want := peers.Limits{MaxRecords: 20000, PerRange: 64, VerifiedPerRange: 8, Retry: 30 * time.Second, MaxUnlisted: 5000, CandidateAge: 2 * time.Hour}
	if got := p.BookLimits(); got != want {
		t.Errorf("book limits %+v, want %+v", got, want)
	}
	want2 := tracking.Retention{FlagHistory: 24 * time.Hour, Log: 10 * time.Minute, LogSize: 1000, MaxAddresses: 50000}
	if got := p.TrackingConfig().Retain; got != want2 {
		t.Errorf("retention %+v, want %+v", got, want2)
	}
}

// Every bound refuses one under its floor and one over its ceiling, and
// takes both ends; a range never holds more than the book.
func TestLimitsBounds(t *testing.T) {
	set := map[string]func(*Policy, int){
		"p2p.max_book":                  func(p *Policy, v int) { p.P2P.MaxBook = v },
		"p2p.book_per_range":            func(p *Policy, v int) { p.P2P.BookPerRange, p.P2P.MaxBook = v, 65536 },
		"p2p.verified_per_range":        func(p *Policy, v int) { p.P2P.VerifiedPerRange = v },
		"p2p.retry_seconds":             func(p *Policy, v int) { p.P2P.RetrySeconds = v },
		"p2p.registry_seconds":          func(p *Policy, v int) { p.P2P.RegistrySeconds = v },
		"tracking.max_addresses":        func(p *Policy, v int) { p.Tracking.MaxAddresses = v },
		"tracking.flag_history_seconds": func(p *Policy, v int) { p.Tracking.FlagHistorySeconds = v },
		"tracking.log_seconds":          func(p *Policy, v int) { p.Tracking.LogSeconds = v },
		"tracking.log_size":             func(p *Policy, v int) { p.Tracking.LogSize = v },
		"p2p.candidate_seconds":         func(p *Policy, v int) { p.P2P.CandidateSeconds = v },
		"seen_size":                     func(p *Policy, v int) { p.SeenSize = v },
		"max_pending":                   func(p *Policy, v int) { p.MaxPending = v },
		"ties":                          func(p *Policy, v int) { p.Ties = v },
	}
	if len(set) != len(Bounds()) {
		t.Fatalf("%d setters for %d bounds", len(set), len(Bounds()))
	}
	for _, b := range Bounds() {
		f := set[b.Name]
		if f == nil {
			t.Fatalf("no setter for %s", b.Name)
		}
		for _, c := range []struct {
			v  int
			ok bool
		}{{b.Min, true}, {b.Max, true}, {b.Min - 1, b.Min-1 == 0}, {b.Max + 1, false}, {-1, false}} {
			var p Policy
			f(&p, c.v)
			err := p.checkBounds()
			if (err == nil) != c.ok {
				t.Errorf("%s = %d: %v", b.Name, c.v, err)
			}
			if err != nil && (!errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), b.Name)) {
				t.Errorf("%s = %d: error %q does not name it", b.Name, c.v, err)
			}
		}
	}
	var p Policy
	p.P2P.MaxBook, p.P2P.BookPerRange = 256, 300
	if err := p.checkBounds(); err == nil || !strings.Contains(err.Error(), "more than p2p.max_book") {
		t.Errorf("a range larger than the book: %v", err)
	}
}

// Validate applies the bounds, so Load refuses a node.json out of range.
func TestValidateRefusesOutOfBounds(t *testing.T) {
	p := Policy{Format: 1}
	if err := p.Validate(); err != nil {
		t.Fatalf("an empty policy: %v", err)
	}
	p.P2P.MaxBook = 10
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "p2p.max_book") {
		t.Errorf("max_book 10: %v", err)
	}
}
