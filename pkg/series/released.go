package series

import (
	"errors"
	"fmt"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// What a node knows of a variable series' releases (PoO section 16.8,
// D166). A node keeps only the newest valid release it has seen, since a
// release of part k proves every part before it. Which serials exist at a
// node grows with what it knows and never shrinks: nothing withdraws a
// part. A mint of a part no known release reaches is not invalid, only
// waiting: the node lacks information, which is never a hard fork.

var (
	// ErrWrongPart: the mint names a part that does not hold its serial,
	// or a fixed series' mint names a part. Invalid at every node, released
	// or not: PartOf follows from the series alone, so each mint has one
	// name (PoO section 16.8).
	ErrWrongPart = errors.New("series: the mint names a part that does not hold its serial")
	// ErrNoSerial: no part of the series can ever hold the serial (a fixed
	// series' serial at or above N, or past the largest number of parts).
	ErrNoSerial = errors.New("series: no part of the series can hold this serial")
	// ErrWaiting: the mint's part is not released at this node yet. It is
	// never invalid: it is checked again once a release reaches its part,
	// and nothing about it is kept as refused.
	ErrWaiting = errors.New("series: waiting for a release: the mint's part is not released at this node yet")
)

// variableIDs holds the id of every variable series this process has
// built, so a transaction's bytes can be read: a variable series' mint
// carries its part (tx package). An id is a hash of the params, so the set
// holds only facts and only grows.
var variableIDs sync.Map

// IsVariableID reports whether id is the id of a variable series this
// process has built. A series it has not built reads as fixed; a node
// cannot check such a series' transactions in any case.
func IsVariableID(id hash.Hash) bool {
	_, ok := variableIDs.Load(id)
	return ok
}

// Released returns the newest part this node knows released: 1 for a
// variable series with no later release added, 0 for a fixed series.
func (s *Series) Released() uint64 { return s.released.Load() }

// AddRelease adds a release this node has received. A release of a part
// already reached is known and costs no hashing (D165's point 4): it
// returns false. A later one is checked in full (CheckRelease) and, when
// valid, becomes the newest release; it returns true. Validity never
// depends on when a release arrives.
func (s *Series) AddRelease(r Release) (bool, error) {
	if !s.IsVariable() {
		return false, ErrFixed
	}
	if r.Series != s.id {
		return false, fmt.Errorf("%w: it names series %s", ErrRelease, r.Series.Short())
	}
	if r.Part <= s.released.Load() {
		return false, nil
	}
	if err := s.CheckRelease(r); err != nil {
		return false, err
	}
	s.newestMu.Lock()
	defer s.newestMu.Unlock()
	if r.Part <= s.released.Load() {
		return false, nil
	}
	s.newest = &r
	s.released.Store(r.Part)
	return true, nil
}

// NewestRelease returns the newest release added, if any: the one record
// a node keeps and passes on, since it proves every part before it.
func (s *Series) NewestRelease() (Release, bool) {
	s.newestMu.Lock()
	defer s.newestMu.Unlock()
	if s.newest == nil {
		return Release{}, false
	}
	return *s.newest, true
}

// CheckMint checks that m names a mint of this series by its one name and
// that the mint exists at this node. It returns ErrWrongPart and ErrNoSerial,
// which are invalid at every node, or ErrWaiting, which only says this node
// has not seen the release yet. It works out no part id.
func (s *Series) CheckMint(m MintID) error {
	if !s.IsVariable() {
		if m.Part != 0 {
			return fmt.Errorf("%w: a fixed series' mint names no part", ErrWrongPart)
		}
		if m.Serial >= s.params.Mints {
			return ErrNoSerial
		}
		return nil
	}
	k, err := s.PartOf(m.Serial)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoSerial, err)
	}
	if m.Part != k {
		return fmt.Errorf("%w: serial %d is in part %d, not %d", ErrWrongPart, m.Serial, k, m.Part)
	}
	if k > s.released.Load() {
		return fmt.Errorf("%w (part %d; released through part %d)", ErrWaiting, k, s.released.Load())
	}
	return nil
}
