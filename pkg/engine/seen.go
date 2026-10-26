package engine

import "github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"

// DefaultSeenSize is the provisional default size of the seen list.
const DefaultSeenSize = 1 << 16

// Seen is the seen list: the tids and hashes of valid items received
// recently, held as a fixed-size ring that overwrites its oldest item. An
// item that drops out and arrives again is checked and forwarded once more,
// which keeps the cost small and bounded. Only valid items are listed, so an
// invalid copy can never block a valid one with the same id.
type Seen struct {
	ring []hash.Hash
	next int
	full bool
	set  map[hash.Hash]int // value: how many ring slots hold the hash
}

// NewSeen returns a seen list holding up to size items.
func NewSeen(size int) *Seen {
	if size <= 0 {
		size = DefaultSeenSize
	}
	return &Seen{ring: make([]hash.Hash, size), set: make(map[hash.Hash]int, size)}
}

// Has reports whether h is on the list.
func (s *Seen) Has(h hash.Hash) bool { return s.set[h] > 0 }

// Add puts h on the list, dropping the oldest item when full.
func (s *Seen) Add(h hash.Hash) {
	if s.full {
		old := s.ring[s.next]
		if s.set[old]--; s.set[old] <= 0 {
			delete(s.set, old)
		}
	}
	s.ring[s.next] = h
	s.set[h]++
	s.next++
	if s.next == len(s.ring) {
		s.next, s.full = 0, true
	}
}

// Len returns how many items are on the list.
func (s *Seen) Len() int {
	if s.full {
		return len(s.ring)
	}
	return s.next
}
