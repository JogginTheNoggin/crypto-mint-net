package peers

import (
	"fmt"
	"sort"
)

// Category is how this node treats another node for one mint policy. The
// order is the order its requests and data are handled in.
type Category int

const (
	// CatPriority: relied on.
	CatPriority Category = iota
	// CatBackup: kept ready.
	CatBackup
	// CatCandidate: not yet verified, or verified with no slot.
	CatCandidate
	// CatUnlisted: in none of this node's categories, such as a wallet or
	// an archived peer.
	CatUnlisted
)

// Categories lists every category in handling order.
var Categories = []Category{CatPriority, CatBackup, CatCandidate, CatUnlisted}

// MarshalText writes the category's name.
func (c Category) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

func (c Category) String() string {
	switch c {
	case CatPriority:
		return "priority"
	case CatBackup:
		return "backup"
	case CatCandidate:
		return "candidate"
	case CatUnlisted:
		return "unlisted"
	}
	return fmt.Sprintf("category(%d)", int(c))
}

// State is the book state for a category.
func (c Category) State() State {
	switch c {
	case CatPriority:
		return Priority
	case CatBackup:
		return Backup
	}
	return Candidate
}

// Slots are the ranking rules for one mint policy, all node policy.
type Slots struct {
	// Priority and Backup are how many peers each category holds.
	Priority, Backup int
	// PriorityPerRange bounds the priority peers from one network range,
	// and PerRange the priority and backup peers together.
	PriorityPerRange, PerRange int
	// InboundPriority bounds the priority peers that connected to this
	// node, since peers it dialled itself are harder to place.
	InboundPriority int
	// Margin is how far a backup's points must beat the lowest priority
	// peer's at a review before the two swap.
	Margin float64
	// PartialPenalty is taken from the points of a peer that follows only
	// part of the mint policy, so full coverage ranks first.
	PartialPenalty float64
}

// DefaultSlots returns the provisional defaults.
func DefaultSlots() Slots {
	return Slots{Priority: 4, Backup: 4, PriorityPerRange: 1, PerRange: 4, InboundPriority: 2, Margin: 1, PartialPenalty: 1}
}

// Ranked is one verified peer for one mint policy, as a review sees it.
type Ranked struct {
	ID       string
	Range    string
	Outbound bool
	Partial  bool
	// Category is the peer's current one: priority, backup, or candidate
	// for a verified peer with no slot.
	Category Category
	// Points count every measure recorded; Fair only the measures a backup
	// and a priority peer both have (pings and first deliveries).
	Points, Fair float64
	// MayPriority is false when the flag total, or repeated denials, bar
	// the peer from priority.
	MayPriority bool
}

func (s Slots) score(r Ranked, fair bool) float64 {
	p := r.Points
	if fair {
		p = r.Fair
	}
	if r.Partial {
		p -= s.PartialPenalty
	}
	return p
}

// fits reports whether r may join category c given the others there.
func (s Slots) fits(r Ranked, c Category, in []Ranked) bool {
	sameRange, listedRange, inbound, n := 0, 0, 0, 0
	for _, o := range in {
		if o.ID == r.ID {
			continue
		}
		if o.Category == c {
			n++
			if o.Range == r.Range {
				sameRange++
			}
			if !o.Outbound {
				inbound++
			}
		}
		if (o.Category == CatPriority || o.Category == CatBackup) && o.Range == r.Range {
			listedRange++
		}
	}
	if s.PerRange > 0 && listedRange >= s.PerRange && r.Category != CatPriority && r.Category != CatBackup {
		return false
	}
	switch c {
	case CatPriority:
		return r.MayPriority && n < s.Priority &&
			(s.PriorityPerRange <= 0 || sameRange < s.PriorityPerRange) &&
			(r.Outbound || s.InboundPriority <= 0 || inbound < s.InboundPriority)
	case CatBackup:
		return n < s.Backup
	}
	return true
}

// Place decides where a newly verified peer goes: priority if a slot is
// free and the flags and spread allow it, else backup if a slot is free,
// else it stays a candidate.
func (s Slots) Place(others []Ranked, r Ranked) Category {
	r.Category = CatCandidate
	if s.fits(r, CatPriority, others) {
		return CatPriority
	}
	if s.fits(r, CatBackup, others) {
		return CatBackup
	}
	return CatCandidate
}

// Review runs one review of a mint policy's peers and returns the ones
// whose category changes. In order: priority peers the flags bar move to
// backup; free priority slots are filled from the backups and candidates
// with the most points; a backup whose fair points beat the lowest
// priority peer's by the margin swaps with it; free backup slots are
// filled from candidates; and backups beyond the slots become candidates.
func (s Slots) Review(rs []Ranked) map[string]Category {
	cur := append([]Ranked(nil), rs...)
	sort.Slice(cur, func(i, j int) bool { return cur[i].ID < cur[j].ID })
	set := func(i int, c Category) { cur[i].Category = c }
	by := func(c Category, fair, high bool) []int {
		var out []int
		for i := range cur {
			if cur[i].Category == c {
				out = append(out, i)
			}
		}
		sort.SliceStable(out, func(a, b int) bool {
			x, y := s.score(cur[out[a]], fair), s.score(cur[out[b]], fair)
			if x != y {
				return (x > y) == high
			}
			// Prefer peers this node dialled itself.
			if cur[out[a]].Outbound != cur[out[b]].Outbound {
				return cur[out[a]].Outbound == high
			}
			return false
		})
		return out
	}
	// Move 4: flags bar priority.
	for _, i := range by(CatPriority, false, true) {
		if !cur[i].MayPriority {
			set(i, CatBackup)
		}
	}
	// Move 3 (and 1): fill free priority slots, best first.
	for _, c := range []Category{CatBackup, CatCandidate} {
		for _, i := range by(c, false, true) {
			if s.fits(cur[i], CatPriority, cur) {
				set(i, CatPriority)
			}
		}
	}
	// Move 3 and 4: swaps by the margin, on fair points.
	for round := 0; round < s.Priority+s.Backup; round++ {
		pri := by(CatPriority, true, false) // lowest first
		bak := by(CatBackup, true, true)    // best first
		swapped := false
		for _, b := range bak {
			if len(pri) == 0 || !cur[b].MayPriority {
				continue
			}
			low := pri[0]
			if s.score(cur[b], true) < s.score(cur[low], true)+s.Margin {
				break
			}
			set(low, CatBackup)
			if s.fits(cur[b], CatPriority, cur) {
				set(b, CatPriority)
				swapped = true
				break
			}
			set(low, CatPriority) // the spread does not allow it
		}
		if !swapped {
			break
		}
	}
	// Move 2: fill free backup slots from candidates.
	for _, i := range by(CatCandidate, false, true) {
		if s.fits(cur[i], CatBackup, cur) {
			set(i, CatBackup)
		}
	}
	// Backups beyond the slots, the lowest first, become candidates.
	if bak := by(CatBackup, false, false); len(bak) > s.Backup {
		for _, i := range bak[:len(bak)-s.Backup] {
			set(i, CatCandidate)
		}
	}
	out := map[string]Category{}
	old := map[string]Category{}
	for _, r := range rs {
		old[r.ID] = r.Category
	}
	for _, r := range cur {
		if old[r.ID] != r.Category {
			out[r.ID] = r.Category
		}
	}
	return out
}
