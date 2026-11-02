// Package peers is a full node's peer book: every node it knows of, for
// each mint policy, as a candidate, a priority or backup peer, or an
// archived one (the design's "Peer Discovery and Management", Peer states,
// and "Peer Categories and Ranking"). The book holds contact details and
// states only; package p2p decides when a state changes, with the ranking
// rules of this package (rank.go). Every change of state is node policy.
//
// Candidates come from address registries, peer lists, follow replies, the
// node policy and the archive. No source makes the node connect anywhere at
// once: the connection logic picks candidates later, spaced over time, and
// the book limits how many come from one network range, so one source
// cannot fill it or aim it at a victim.
//
// The book is saved as JSON in the node's data directory.
package peers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// FileName is the book's file in a data directory.
const FileName = "peers.json"

// State is what a node is to this node for one mint policy.
type State int

const (
	// Candidate: known from somewhere, not yet checked by this node, or
	// checked with no slot free for it.
	Candidate State = iota
	// Priority: verified, and relied on: asked, forwarded to, pinged
	// regularly.
	Priority
	// Backup: verified and kept ready: forwarded to, pinged less often,
	// asked only as a fallback.
	Backup
	// Archived: not in use; details kept as a starting point.
	Archived
)

// Listed reports whether the state is priority or backup: verified, with a
// slot.
func (s State) Listed() bool { return s == Priority || s == Backup }

func (s State) String() string {
	switch s {
	case Candidate:
		return "candidate"
	case Priority:
		return "priority"
	case Backup:
		return "backup"
	case Archived:
		return "archived"
	}
	return fmt.Sprintf("state(%d)", int(s))
}

// MarshalText writes the state's name.
func (s State) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText reads a state's name.
func (s *State) UnmarshalText(b []byte) error {
	for _, v := range []State{Candidate, Priority, Backup, Archived} {
		if v.String() == string(b) {
			*s = v
			return nil
		}
	}
	if string(b) == "verified" { // written before categories existed
		*s = Backup
		return nil
	}
	return fmt.Errorf("peers: unknown state %q", b)
}

// Source says where a node was first heard of.
type Source string

const (
	FromConfig   Source = "config"
	FromRegistry Source = "registry"
	FromPeerList Source = "peer-list"
	FromReply    Source = "reply"
	FromInbound  Source = "inbound"
)

// Policy is a node's state for one mint policy.
type Policy struct {
	Section   wire.Section `json:"-"`
	Text      string       `json:"section"`
	State     State        `json:"state"`
	LastHeard time.Time    `json:"last_heard,omitempty"`
}

// Record is one known node, by contact address.
type Record struct {
	Addr      string    `json:"addr"`
	Source    Source    `json:"source"`
	Added     time.Time `json:"added"`
	LastTried time.Time `json:"last_tried,omitempty"`
	Failures  int       `json:"failures,omitempty"`
	Policies  []Policy  `json:"policies"`
}

func (r *Record) policy(s wire.Section) *Policy {
	for i := range r.Policies {
		if r.Policies[i].Section == s {
			return &r.Policies[i]
		}
	}
	return nil
}

func (r *Record) clone() Record {
	c := *r
	c.Policies = append([]Policy(nil), r.Policies...)
	return c
}

// overlaps reports whether s and t have anything in common.
func overlaps(s, t wire.Section) bool { return s.Covers(t) != wire.None }

// Limits are the book's node policy bounds.
type Limits struct {
	// MaxRecords bounds the whole book.
	MaxRecords int
	// PerRange bounds the records from one network range.
	PerRange int
	// VerifiedPerRange bounds the priority and backup peers for one mint
	// policy from one network range.
	VerifiedPerRange int
	// Retry is the first wait before trying a candidate again after a
	// failed attempt; it doubles with each failure.
	Retry time.Duration
}

// DefaultLimits returns the provisional defaults.
func DefaultLimits() Limits {
	return Limits{MaxRecords: 4096, PerRange: 16, VerifiedPerRange: 4, Retry: time.Minute}
}

// Book is the peer book. It is safe for concurrent use.
type Book struct {
	mu     sync.Mutex
	path   string
	lim    Limits
	recs   map[string]*Record
	refuse func(addr string) bool
	dirty  bool
}

type file struct {
	Format  int       `json:"format"`
	Records []*Record `json:"records"`
}

// ErrRefused is returned for an address the refuse filter turns away, such
// as a blocked one.
var ErrRefused = errors.New("peers: address refused")

// ErrRangeFull is returned when a network range has reached its limit.
var ErrRangeFull = errors.New("peers: too many from this network range")

// ErrFull is returned when the book holds MaxRecords.
var ErrFull = errors.New("peers: book full")

// Open loads the book at path, or starts an empty one. An empty path keeps
// the book in memory only.
func Open(path string, lim Limits) (*Book, error) {
	d := DefaultLimits()
	if lim.MaxRecords == 0 {
		lim.MaxRecords = d.MaxRecords
	}
	if lim.PerRange == 0 {
		lim.PerRange = d.PerRange
	}
	if lim.VerifiedPerRange == 0 {
		lim.VerifiedPerRange = d.VerifiedPerRange
	}
	if lim.Retry == 0 {
		lim.Retry = d.Retry
	}
	b := &Book{path: path, lim: lim, recs: map[string]*Record{}}
	if path == "" {
		return b, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("peers: %s: %w", path, err)
	}
	for _, r := range f.Records {
		if r == nil || wire.CheckAddr(r.Addr) != nil {
			continue
		}
		var ps []Policy
		for _, p := range r.Policies {
			s, err := wire.ParseSection(p.Text)
			if err != nil {
				continue
			}
			p.Section = s
			// Nothing is verified across a restart: verification is this
			// run's own check.
			if p.State.Listed() {
				p.State = Candidate
			}
			ps = append(ps, p)
		}
		r.Policies = ps
		b.recs[r.Addr] = r
	}
	return b, nil
}

// Save writes the book if it changed.
func (b *Book) Save() error {
	b.mu.Lock()
	if !b.dirty || b.path == "" {
		b.mu.Unlock()
		return nil
	}
	f := file{Format: 1}
	for _, r := range b.sortedLocked() {
		c := r.clone()
		f.Records = append(f.Records, &c)
	}
	b.dirty = false
	b.mu.Unlock()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFile(b.path, append(data, '\n'), 0o600) // RED-041, D148
}

func (b *Book) sortedLocked() []*Record {
	out := make([]*Record, 0, len(b.recs))
	for _, r := range b.recs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Addr < out[j].Addr })
	return out
}

// Range names the network range of an address: the /16 of an IPv4
// address, the /32 of an IPv6 address, or the host name itself. Each
// loopback address is its own range, so nodes on one machine (tests, a
// local network of nodes) are not limited.
func Range(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return "host:" + host
	case ip.IsLoopback():
		return "loopback:" + addr
	case ip.To4() != nil:
		v4 := ip.To4()
		return fmt.Sprintf("v4:%d.%d", v4[0], v4[1])
	default:
		return fmt.Sprintf("v6:%x", []byte(ip.To16()[:4]))
	}
}

func (b *Book) inRangeLocked(rng string) int {
	n := 0
	for a := range b.recs {
		if Range(a) == rng {
			n++
		}
	}
	return n
}

// SetRefuse sets a filter for addresses the book must not take in, such
// as blocked ones. It is called without the book's lock held.
func (b *Book) SetRefuse(f func(addr string) bool) {
	b.mu.Lock()
	b.refuse = f
	b.mu.Unlock()
}

// Add records addr as a candidate for the sections, unless it is known
// already (then the sections it lacks are added as candidates). It refuses
// addresses the refuse filter turns away, a full book and a full network
// range.
func (b *Book) Add(addr string, sections []wire.Section, src Source, now time.Time) error {
	if err := wire.CheckAddr(addr); err != nil {
		return err
	}
	b.mu.Lock()
	refuse := b.refuse
	b.mu.Unlock()
	if refuse != nil && refuse(addr) {
		return ErrRefused
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	if r == nil {
		if len(b.recs) >= b.lim.MaxRecords {
			return ErrFull
		}
		if b.inRangeLocked(Range(addr)) >= b.lim.PerRange {
			return ErrRangeFull
		}
		r = &Record{Addr: addr, Source: src, Added: now}
		b.recs[addr] = r
		b.dirty = true
	}
	for _, s := range sections {
		if r.policy(s) == nil {
			r.Policies = append(r.Policies, Policy{Section: s, Text: s.Text(), State: Candidate})
			b.dirty = true
		}
	}
	return nil
}

func (b *Book) verifiedInRangeLocked(s wire.Section, rng, except string) int {
	n := 0
	for a, r := range b.recs {
		if a == except || Range(a) != rng {
			continue
		}
		if p := r.policy(s); p != nil && p.State.Listed() {
			n++
		}
	}
	return n
}

// Verify places addr in category st (priority or backup) for s: it
// answered this node's own follow request or ping, and has a slot. It
// refuses a range that already has its share of listed peers for s.
func (b *Book) Verify(addr string, s wire.Section, st State, now time.Time) error {
	if !st.Listed() {
		return fmt.Errorf("peers: verify as %s", st)
	}
	if err := b.Add(addr, []wire.Section{s}, FromReply, now); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	p := r.policy(s)
	if !p.State.Listed() && b.verifiedInRangeLocked(s, Range(addr), addr) >= b.lim.VerifiedPerRange {
		return ErrRangeFull
	}
	p.State, p.LastHeard = st, now
	r.Failures = 0
	b.dirty = true
	return nil
}

// Heard notes that addr answered for s.
func (b *Book) Heard(addr string, s wire.Section, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r := b.recs[addr]; r != nil {
		if p := r.policy(s); p != nil {
			p.LastHeard = now
			b.dirty = true
		}
	}
}

// Archive moves addr's s to the archive; with no sections, every policy
// of addr.
func (b *Book) Archive(addr string, ss ...wire.Section) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	if r == nil {
		return
	}
	for i := range r.Policies {
		p := &r.Policies[i]
		if len(ss) == 0 {
			p.State = Archived
			continue
		}
		for _, s := range ss {
			// A dropped mint inside a followed series archives the part
			// named; a dropped series archives what is inside it.
			if s.Contains(p.Section) || p.Section == s {
				p.State = Archived
			}
		}
	}
	b.dirty = true
}

// Unverify makes addr a candidate again for every policy it was listed
// for, as when its connection closes; with sections, only for those.
func (b *Book) Unverify(addr string, ss ...wire.Section) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	if r == nil {
		return
	}
	for i := range r.Policies {
		p := &r.Policies[i]
		if !p.State.Listed() {
			continue
		}
		if len(ss) > 0 && !containsSection(ss, p.Section) {
			continue
		}
		p.State = Candidate
		b.dirty = true
	}
}

func containsSection(ss []wire.Section, s wire.Section) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// Delete removes addr's details for good, by hand or to free memory. A
// deleted node can return as a new candidate: deleting alone does not keep
// it out; a block does.
func (b *Book) Delete(addr string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.recs[addr]; !ok {
		return false
	}
	delete(b.recs, addr)
	b.dirty = true
	return true
}

// Tried notes a connection attempt and whether it failed.
func (b *Book) Tried(addr string, ok bool, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	if r == nil {
		return
	}
	r.LastTried = now
	if ok {
		r.Failures = 0
	} else if r.Failures < 16 {
		r.Failures++
	}
	b.dirty = true
}

// Pick returns up to n candidates for s worth trying now, oldest attempt
// first, spread over network ranges, skipping those in skip. An empty
// result with archived nodes for s left means the archive can be searched
// with Unarchive.
func (b *Book) Pick(s wire.Section, n int, skip map[string]bool, now time.Time) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var cands []*Record
	for a, r := range b.recs {
		if skip[a] {
			continue
		}
		if r.Failures > 0 {
			wait := b.lim.Retry << (r.Failures - 1)
			if now.Sub(r.LastTried) < wait {
				continue
			}
		}
		for _, p := range r.Policies {
			if p.State == Candidate && overlaps(p.Section, s) {
				cands = append(cands, r)
				break
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if !cands[i].LastTried.Equal(cands[j].LastTried) {
			return cands[i].LastTried.Before(cands[j].LastTried)
		}
		return cands[i].Addr < cands[j].Addr
	})
	used := map[string]bool{}
	var out []string
	// First one per range, then the rest.
	for pass := 0; pass < 2 && len(out) < n; pass++ {
		for _, r := range cands {
			if len(out) >= n {
				break
			}
			rng := Range(r.Addr)
			if (pass == 0 && used[rng]) || contains(out, r.Addr) {
				continue
			}
			used[rng] = true
			out = append(out, r.Addr)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Unarchive makes the archived nodes for s candidates again, so they can
// be checked afresh. It returns how many.
func (b *Book) Unarchive(s wire.Section) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, r := range b.recs {
		for i := range r.Policies {
			p := &r.Policies[i]
			if p.State == Archived && overlaps(p.Section, s) {
				p.State = Candidate
				n++
			}
		}
	}
	if n > 0 {
		b.dirty = true
	}
	return n
}

// VerifiedFor lists the nodes listed (priority or backup) for a mint
// policy overlapping s,
// each with its verified policies that overlap s, sorted by address. With
// full, only nodes that follow all of s, or all of the part named by
// within, are listed.
func (b *Book) VerifiedFor(s wire.Section, full bool, within []wire.Section) []wire.PeerInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []wire.PeerInfo
	for _, r := range b.sortedLocked() {
		var ps []wire.Section
		fully := false
		for _, p := range r.Policies {
			if !p.State.Listed() || !overlaps(p.Section, s) {
				continue
			}
			ps = append(ps, p.Section)
			if p.Section.Contains(s) {
				fully = true
			}
			for _, w := range within {
				if p.Section.Contains(w) {
					fully = true
				}
			}
		}
		if len(ps) == 0 || (full && !fully) {
			continue
		}
		wire.SortSections(ps)
		out = append(out, wire.PeerInfo{Contact: wire.Contact{Addr: r.Addr}, Policies: ps})
	}
	return out
}

// Count returns how many nodes are in each state for policies overlapping
// s (all of them for wire.All).
func (b *Book) Count(s wire.Section) map[State]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[State]int{}
	for _, r := range b.recs {
		seen := map[State]bool{}
		for _, p := range r.Policies {
			if overlaps(p.Section, s) && !seen[p.State] {
				seen[p.State] = true
				out[p.State]++
			}
		}
	}
	return out
}

// Records returns a copy of every record, sorted by address.
func (b *Book) Records() []Record {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Record
	for _, r := range b.sortedLocked() {
		out = append(out, r.clone())
	}
	return out
}

// Record returns a copy of addr's record.
func (b *Book) Record(addr string) (Record, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	if r == nil {
		return Record{}, false
	}
	return r.clone(), true
}

// State returns addr's state for s, and whether it has one.
func (b *Book) State(addr string, s wire.Section) (State, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.recs[addr]
	if r == nil {
		return Candidate, false
	}
	p := r.policy(s)
	if p == nil {
		return Candidate, false
	}
	return p.State, true
}
