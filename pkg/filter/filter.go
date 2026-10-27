// Package filter says which items a viewer or watcher wants: by series,
// pool, mint, address, policy, transaction and kind of event. One Filter
// value is shared by the node's views, its live watch stream, the RPC and
// the command line, so every front end selects items the same way.
//
// An empty field matches everything. A filter matches an item when every
// field that is set matches it, and a field matches when any of its values
// does: fields are joined with "and", values within a field with "or".
package filter

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// Kind is a kind of event in the watch stream.
type Kind string

// The kinds of event.
const (
	// KindTx: a transaction became pending at the node.
	KindTx Kind = "tx"
	// KindEntry: the node kept an entry.
	KindEntry Kind = "entry"
	// KindLayer: the node kept a layer.
	KindLayer Kind = "layer"
	// KindSwitch: the selected ledger of a pool changed.
	KindSwitch Kind = "switch"
	// KindSelected: a transaction entered the selected ledger.
	KindSelected Kind = "selected"
	// KindReverted: a transaction left the selected ledger; it is carried
	// (pending again) or void (it can no longer apply).
	KindReverted Kind = "reverted"
	// KindDropped: a pending transaction was dropped as no longer valid.
	KindDropped Kind = "dropped"
)

// Kinds lists every kind, in a fixed order.
var Kinds = []Kind{KindTx, KindEntry, KindLayer, KindSwitch, KindSelected, KindReverted, KindDropped}

// ErrBad is returned for a filter that cannot be read.
var ErrBad = errors.New("filter: bad filter")

// Filter selects items. See the package comment for how fields combine.
type Filter struct {
	Series    []hash.Hash     `json:"series,omitempty"`
	Pools     []uint64        `json:"pools,omitempty"`
	Serials   []uint64        `json:"serials,omitempty"`
	Addresses []hash.Hash     `json:"addresses,omitempty"`
	Policies  []policy.Policy `json:"-"`
	TIDs      []hash.Hash     `json:"tids,omitempty"`
	Kinds     []Kind          `json:"kinds,omitempty"`
}

// Subject is what an item offers to be matched against. A field left
// empty in a subject matches no filter value of that field, except Kind,
// which is not checked when empty.
type Subject struct {
	Series    hash.Hash
	Hash      hash.Func // the series' hash, for policies; nil skips them
	Pool      uint64
	HasPool   bool
	Serials   []uint64
	Addresses []hash.Hash
	TIDs      []hash.Hash
	Kind      Kind
}

// Empty reports whether f matches everything.
func (f Filter) Empty() bool {
	return len(f.Series) == 0 && len(f.Pools) == 0 && len(f.Serials) == 0 &&
		len(f.Addresses) == 0 && len(f.Policies) == 0 && len(f.TIDs) == 0 && len(f.Kinds) == 0
}

// Match reports whether s is selected by f.
func (f Filter) Match(s Subject) bool {
	if len(f.Series) > 0 && !hasHash(f.Series, s.Series) {
		return false
	}
	if len(f.Pools) > 0 && (!s.HasPool || !hasU64(f.Pools, s.Pool)) {
		return false
	}
	if len(f.Serials) > 0 && !anyU64(f.Serials, s.Serials) {
		return false
	}
	if len(f.Addresses) > 0 || len(f.Policies) > 0 {
		if !f.matchAddress(s) {
			return false
		}
	}
	if len(f.TIDs) > 0 && !anyHash(f.TIDs, s.TIDs) {
		return false
	}
	if len(f.Kinds) > 0 && s.Kind != "" && !hasKind(f.Kinds, s.Kind) {
		return false
	}
	return true
}

// MatchScope reports whether any item of pool index pool of series, which
// holds serials first to last, can match f. Views use it to skip pools.
func (f Filter) MatchScope(series hash.Hash, pool, first, last uint64) bool {
	if len(f.Series) > 0 && !hasHash(f.Series, series) {
		return false
	}
	if len(f.Pools) > 0 && !hasU64(f.Pools, pool) {
		return false
	}
	if len(f.Serials) == 0 {
		return true
	}
	for _, n := range f.Serials {
		if n >= first && n <= last {
			return true
		}
	}
	return false
}

// WantsKind reports whether f lets events of kind k through.
func (f Filter) WantsKind(k Kind) bool { return len(f.Kinds) == 0 || hasKind(f.Kinds, k) }

func (f Filter) matchAddress(s Subject) bool {
	if anyHash(f.Addresses, s.Addresses) {
		return true
	}
	if s.Hash == nil {
		return false
	}
	for _, p := range f.Policies {
		if hasHash(s.Addresses, policy.Address(s.Hash, p)) {
			return true
		}
	}
	return false
}

// Values returns f as query parameters, the form the RPC takes. Each
// field is one comma-separated parameter; policies are hex of their byte
// form.
func (f Filter) Values() url.Values {
	v := url.Values{}
	join := func(name string, n int, at func(int) string) {
		if n == 0 {
			return
		}
		parts := make([]string, n)
		for i := range parts {
			parts[i] = at(i)
		}
		v.Set(name, strings.Join(parts, ","))
	}
	join("series", len(f.Series), func(i int) string { return f.Series[i].String() })
	join("pool", len(f.Pools), func(i int) string { return strconv.FormatUint(f.Pools[i], 10) })
	join("serial", len(f.Serials), func(i int) string { return strconv.FormatUint(f.Serials[i], 10) })
	join("address", len(f.Addresses), func(i int) string { return f.Addresses[i].String() })
	join("policy", len(f.Policies), func(i int) string { return fmt.Sprintf("%x", f.Policies[i].Encode()) })
	join("tid", len(f.TIDs), func(i int) string { return f.TIDs[i].String() })
	join("kind", len(f.Kinds), func(i int) string { return string(f.Kinds[i]) })
	return v
}

// FromValues reads a filter from query parameters written by Values.
// Unknown parameters are left for the caller.
func FromValues(v url.Values) (Filter, error) {
	var b Builder
	for _, name := range []string{"series", "pool", "serial", "address", "policy", "tid", "kind"} {
		for _, s := range v[name] {
			if err := b.Add(name, s); err != nil {
				return Filter{}, err
			}
		}
	}
	return b.Filter(), nil
}

// Builder collects filter values one field at a time; the command line
// and FromValues both use it.
type Builder struct{ f Filter }

// Add adds the comma-separated values s to the field name: series, pool,
// serial, address, policy, tid or kind.
func (b *Builder) Add(name, s string) error {
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		if err := b.add(name, part); err != nil {
			return fmt.Errorf("%w: %s %q: %v", ErrBad, name, part, err)
		}
	}
	return nil
}

func (b *Builder) add(name, s string) error {
	switch name {
	case "series", "address", "tid":
		h, err := hash.Parse(s)
		if err != nil {
			return err
		}
		switch name {
		case "series":
			b.f.Series = appendHash(b.f.Series, h)
		case "address":
			b.f.Addresses = appendHash(b.f.Addresses, h)
		default:
			b.f.TIDs = appendHash(b.f.TIDs, h)
		}
	case "pool", "serial":
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		if name == "pool" {
			b.f.Pools = appendU64(b.f.Pools, n)
		} else {
			b.f.Serials = appendU64(b.f.Serials, n)
		}
	case "policy":
		raw, err := hex.DecodeString(s)
		if err != nil {
			return err
		}
		r := encoding.NewReader(raw)
		p := policy.DecodePolicy(r)
		if err := r.Finish(); err != nil {
			return err
		}
		b.f.Policies = append(b.f.Policies, p)
	case "kind":
		k := Kind(s)
		if !hasKind(Kinds, k) {
			return fmt.Errorf("unknown kind (want one of %s)", kindList())
		}
		if !hasKind(b.f.Kinds, k) {
			b.f.Kinds = append(b.f.Kinds, k)
		}
	default:
		return errors.New("unknown field")
	}
	return nil
}

// Filter returns what was added, with numbers sorted.
func (b *Builder) Filter() Filter {
	f := b.f
	sort.Slice(f.Pools, func(i, j int) bool { return f.Pools[i] < f.Pools[j] })
	sort.Slice(f.Serials, func(i, j int) bool { return f.Serials[i] < f.Serials[j] })
	return f
}

// Flags registers the filter flags on fs: -series, -pool, -serial,
// -address, -policy, -tid and -kind, each repeatable and taking
// comma-separated values. Call the returned function after fs.Parse.
func Flags(fs *flag.FlagSet) func() (Filter, error) {
	b := &Builder{}
	var errs []error
	add := func(name, usage string) {
		fs.Func(name, usage+" (repeatable, comma-separated)", func(s string) error {
			err := b.Add(name, s)
			if err != nil {
				errs = append(errs, err)
			}
			return err
		})
	}
	add("series", "only these series (id hex)")
	add("pool", "only these pool indices")
	add("serial", "only items of these mints")
	add("address", "only items paying or spending these addresses")
	add("policy", "only items paying or spending these policies (hex), under each series' hash")
	add("tid", "only these transactions")
	add("kind", "only these kinds of event: "+kindList())
	return func() (Filter, error) { return b.Filter(), errors.Join(errs...) }
}

func kindList() string {
	parts := make([]string, len(Kinds))
	for i, k := range Kinds {
		parts[i] = string(k)
	}
	return strings.Join(parts, ", ")
}

func hasHash(l []hash.Hash, h hash.Hash) bool {
	for _, x := range l {
		if x == h {
			return true
		}
	}
	return false
}

func anyHash(want, have []hash.Hash) bool {
	for _, h := range have {
		if hasHash(want, h) {
			return true
		}
	}
	return false
}

func hasU64(l []uint64, n uint64) bool {
	for _, x := range l {
		if x == n {
			return true
		}
	}
	return false
}

func anyU64(want, have []uint64) bool {
	for _, n := range have {
		if hasU64(want, n) {
			return true
		}
	}
	return false
}

func hasKind(l []Kind, k Kind) bool {
	for _, x := range l {
		if x == k {
			return true
		}
	}
	return false
}

func appendHash(l []hash.Hash, h hash.Hash) []hash.Hash {
	if hasHash(l, h) {
		return l
	}
	return append(l, h)
}

func appendU64(l []uint64, n uint64) []uint64 {
	if hasU64(l, n) {
		return l
	}
	return append(l, n)
}
