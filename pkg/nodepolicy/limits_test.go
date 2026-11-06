package nodepolicy

import (
	"errors"
	"reflect"
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
		// series_ceiling defaults to 0, which is off (D171), as do the
		// bounty market's on-off settings and amounts (D198).
		offByDefault := b.Name == BoundSeriesCeiling.Name || strings.HasPrefix(b.Name, "bounty.")
		if (b.Default < b.Min || b.Default > b.Max) && !(offByDefault && b.Default == 0) {
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
		"series_ceiling":                func(p *Policy, v int) { p.SeriesCeiling = v },
		"entry_cache_mib":               func(p *Policy, v int) { p.EntryCacheMiB = v },
		"recent_window":                 func(p *Policy, v int) { p.RecentWindow = v },
		"snapshot_every":                func(p *Policy, v int) { p.SnapshotEvery = v },
		"p2p.port_map_seconds":          func(p *Policy, v int) { p.P2P.PortMapSeconds = v },
		"bounty.max_open":               func(p *Policy, v int) { p.Bounty = &Bounty{MaxOpen: v} },
		"bounty.max_amount":             func(p *Policy, v int) { p.Bounty = &Bounty{MaxAmount: uint64(v)} },
		"bounty.budget":                 func(p *Policy, v int) { p.Bounty = &Bounty{Budget: uint64(v)} },
		"bounty.max_leases_per_grinder": func(p *Policy, v int) { p.Bounty = &Bounty{MaxLeasesPerGrinder: v} },
		"bounty.lease_floor_seconds": func(p *Policy, v int) {
			p.Bounty = &Bounty{LeaseFloorSeconds: v, LeaseCeilingSeconds: MaxBountyLeaseSec}
		},
		"bounty.lease_ceiling_seconds": func(p *Policy, v int) { p.Bounty = &Bounty{LeaseCeilingSeconds: v} },
		"bounty.held_offers":           func(p *Policy, v int) { p.Bounty = &Bounty{HeldOffers: v} },
		"bounty.min_price":             func(p *Policy, v int) { p.Bounty = &Bounty{MinPrice: uint64(v)} },
	}
	// The on-off settings hold only 0 and 1, so SetLimits refuses
	// anything else before it is stored (D198).
	for _, name := range []string{"bounty.post", "bounty.take"} {
		var p Policy
		for _, v := range []int{-1, 2} {
			if _, err := p.SetLimits(map[string]int{name: v}); err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("%s = %d: %v", name, v, err)
			}
		}
		if ok, err := p.SetLimits(map[string]int{name: 1}); err != nil || !ok || p.Bounty == nil {
			t.Errorf("%s = 1: %v %v", name, ok, err)
		}
		if ok, err := p.SetLimits(map[string]int{name: 0}); err != nil || !ok || p.Bounty != nil {
			t.Errorf("%s = 0 leaves %+v: %v %v", name, p.Bounty, ok, err)
		}
	}
	if len(set)+len(bountyBools) != len(Bounds()) {
		t.Fatalf("%d setters for %d bounds", len(set), len(Bounds()))
	}
	for _, b := range Bounds() {
		if bountyBools[b.Name] {
			continue
		}
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

// Limits lists every bound of Bounds, in its order, with what node.json
// sets and what is used, and is the list Validate checks (GREEN-047).
func TestLimitsListsEveryBound(t *testing.T) {
	p := Default()
	p.P2P.MaxBook = 1000
	got := p.Limits()
	all := Bounds()
	if len(got) != len(all) {
		t.Fatalf("%d limits, %d bounds", len(got), len(all))
	}
	for i, b := range all {
		l := got[i]
		if l.Name != b.Name || l.Min != b.Min || l.Max != b.Max || l.Default != b.Default {
			t.Errorf("limit %d %+v, bound %+v", i, l, b)
		}
		if l.Switch != (l.Name == "bounty.post" || l.Name == "bounty.take") {
			t.Errorf("%s: switch %v", l.Name, l.Switch)
		}
		if l.Name == "p2p.max_book" && (l.Set != 1000 || l.Used != 1000) {
			t.Errorf("max_book: %+v", l)
		}
		if l.Name != "p2p.max_book" && (l.Set != 0 || l.Used != b.Default) {
			t.Errorf("%s: %+v", l.Name, l)
		}
	}
	p.Tracking.LogSize = 5
	if err := p.Validate(); err == nil {
		t.Error("Validate took a value Limits shows out of range")
	}
}

// Every bound has a field SetLimits reaches, and a bad name, a value out
// of range or a pair out of step changes nothing.
func TestSetLimits(t *testing.T) {
	var p Policy
	for _, l := range p.Limits() {
		q := p
		if ok, err := q.SetLimits(map[string]int{l.Name: l.Max}); err != nil || !ok {
			if !(l.Name == BoundBookPerRange.Name) {
				t.Errorf("%s = %d: %v %v", l.Name, l.Max, ok, err)
			}
		}
	}
	if ok, err := p.SetLimits(map[string]int{"p2p.max_book": 300, "p2p.book_per_range": 300}); err != nil || !ok {
		t.Fatalf("a pair: %v %v", ok, err)
	}
	before := p
	for _, bad := range []map[string]int{{"p2p.nope": 1}, {"p2p.max_book": 1}, {"p2p.max_book": 256}} {
		if _, err := p.SetLimits(bad); err == nil {
			t.Errorf("%v taken", bad)
		}
		if !reflect.DeepEqual(p, before) {
			t.Fatalf("%v changed the policy", bad)
		}
	}
	if ok, err := p.SetLimits(map[string]int{"p2p.max_book": 300}); err != nil || ok {
		t.Errorf("the same value: %v %v", ok, err)
	}
	if ok, err := p.SetLimits(map[string]int{"p2p.max_book": 0, "p2p.book_per_range": 0}); err != nil || !ok || p.P2P.MaxBook != 0 {
		t.Errorf("back to the defaults: %v %v %+v", ok, err, p.P2P)
	}
}
