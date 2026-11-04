package series

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

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
// holds only facts and only grows. Every transaction decoded reads it, so
// it is a map replaced whole under variableMu when an id is added, read
// with one atomic load and no lock, and nil while there is none
// (ORANGE-035).
var (
	variableMu  sync.Mutex
	variableIDs atomic.Pointer[map[hash.Hash]struct{}]
)

// addVariableID adds id to the set.
func addVariableID(id hash.Hash) {
	variableMu.Lock()
	defer variableMu.Unlock()
	old := variableIDs.Load()
	if old != nil {
		if _, ok := (*old)[id]; ok {
			return
		}
	}
	m := make(map[hash.Hash]struct{}, 1)
	if old != nil {
		for k := range *old {
			m[k] = struct{}{}
		}
	}
	m[id] = struct{}{}
	variableIDs.Store(&m)
}

// IsVariableID reports whether id is the id of a variable series this
// process has built. A series it has not built reads as fixed; a node
// cannot check such a series' transactions in any case.
func IsVariableID(id hash.Hash) bool {
	m := variableIDs.Load()
	if m == nil {
		return false
	}
	_, ok := (*m)[id]
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

// checked holds, for each variable series id, the newest release this
// process checked in full (CheckRelease) or took from its own cache
// (TrustRelease). A node writes it beside node.json so its next process
// need not work the chain out again (RED-075). Like variableIDs it holds
// only facts: one release per series.
var (
	checkedMu sync.Mutex
	checked   = map[hash.Hash]Release{}
)

// noteChecked records that r was checked in full.
func noteChecked(r Release) {
	checkedMu.Lock()
	defer checkedMu.Unlock()
	if old, ok := checked[r.Series]; !ok || r.Part >= old.Part {
		checked[r.Series] = r
	}
}

// ReleaseChecked reports whether this process checked r in full, or took
// it from the node's own cache, so its chain need not be worked out again.
// Only the series, part and previous id are compared: the witness is
// checked again each time, which costs no hashing of the chain.
func ReleaseChecked(r Release) bool {
	checkedMu.Lock()
	defer checkedMu.Unlock()
	c, ok := checked[r.Series]
	return ok && c.Part == r.Part && c.Previous == r.Previous
}

// TrustRelease takes r's previous id as part_id(r.Part − 1) without
// working the chain out, because this node checked r in full in an
// earlier process and kept that in its own cache (RED-075). It is a
// cache: it is read only from the node's own directory, never from a
// policy file another node wrote, and CheckRelease still checks r's
// witness. A cache that does not match the release kept is not used, so
// a damaged one costs time, never a result.
func TrustRelease(r Release) {
	if r.Part < 2 {
		return
	}
	c := chainFor(r.Series)
	c.mu.Lock()
	if c.next < r.Part-1 {
		c.trustK, c.trust = r.Part-1, r.Previous
	}
	c.mu.Unlock()
	noteChecked(r)
}
