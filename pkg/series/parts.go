package series

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// Parts of a variable series (PoO section 16.8, D163).
//
// Part 1 is the series' N mints, released by the issuer witness on the
// series. Part k, from 2 on, holds the M serials from N + (k − 2) × M to
// N + (k − 1) × M − 1. Its id is
//
//	part_id(1) = H("PoO-part" ∥ series_id ∥ 1 ∥ series_id)
//	part_id(k) = H("PoO-part" ∥ series_id ∥ k ∥ part_id(k − 1))
//
// with k written as 8 bytes, and nothing else: the issuer has nothing to
// choose, so there is exactly one part k. The issuer releases part k with
// a witness over H("PoO-part-v1" ∥ part_id(k)), which sits beside the part
// and outside its id, the way a witness sits outside a tid. A release of
// part k proves every part before it, and nothing withdraws one.

var (
	// ErrFixed: a fixed series has no parts.
	ErrFixed = errors.New("series: a fixed series has no parts")
	// ErrNoPart: the series can never have a part with this number: 0,
	// past the largest number of parts, or past the serials 64 bits hold.
	ErrNoPart = errors.New("series: no such part")
	// ErrRelease marks a release that does not release a part of the
	// series: the wrong series, part 1 (released by the series itself), a
	// previous part id that is not the part before, or a witness that is
	// not the issuer's.
	ErrRelease = errors.New("series: invalid release")
)

// partChain caches part ids; it never changes a result. It keeps one id in
// every partStride (a mark) and the ids of the stride last asked for, so
// its memory is k/partStride ids rather than k, and listing parts in order
// works each id out once (ORANGE-034).
type partChain struct {
	mu    sync.Mutex
	marks []hash.Hash // marks[j] is part_id(j×partStride + 1)
	next  uint64      // parts 1 to next have their marks
	at    hash.Hash   // part_id(next), where the marks stop
	block uint64      // the stride held in ids, numbered from 1 (0: none)
	ids   []hash.Hash // ids[i] is part_id((block−1)×partStride + i + 1)
}

// partStride is how many parts lie between two marks.
const partStride = 1024

// IsVariable reports whether the series is variable.
func (s *Series) IsVariable() bool { return s.params.SupplyType == Variable }

// SupplyType returns the series' supply type.
func (s *Series) SupplyType() SupplyType { return s.params.SupplyType }

// PartSize returns M, 0 for a fixed series.
func (s *Series) PartSize() uint64 { return s.params.PartSize }

// MaxParts returns the largest number of parts, 0 for no limit or for a
// fixed series.
func (s *Series) MaxParts() uint64 { return s.params.MaxParts }

// mintsThrough returns N + (k − 1) × M, the serials parts 1 to k hold,
// and false when k is 0 or the count does not fit in 64 bits.
func mintsThrough(p Params, k uint64) (uint64, bool) {
	if k == 0 {
		return 0, false
	}
	hi, lo := bits.Mul64(k-1, p.PartSize)
	if hi != 0 {
		return 0, false
	}
	n, carry := bits.Add64(p.Mints, lo, 0)
	return n, carry == 0
}

// checkPart refuses a part number the series can never have.
func (s *Series) checkPart(k uint64) error {
	if !s.IsVariable() {
		return ErrFixed
	}
	if k == 0 {
		return fmt.Errorf("%w: parts are numbered from 1", ErrNoPart)
	}
	if m := s.params.MaxParts; m != 0 && k > m {
		return fmt.Errorf("%w: part %d, and the series has at most %d", ErrNoPart, k, m)
	}
	if _, ok := mintsThrough(s.params, k); !ok {
		return fmt.Errorf("%w: part %d holds serials past what 64 bits hold", ErrNoPart, k)
	}
	return nil
}

// MintsThrough returns N + (k − 1) × M, the number of serials parts 1 to
// k hold: once part k is released, serials 0 to MintsThrough(k) − 1 exist.
func (s *Series) MintsThrough(k uint64) (uint64, error) {
	if err := s.checkPart(k); err != nil {
		return 0, err
	}
	n, _ := mintsThrough(s.params, k)
	return n, nil
}

// PartRange returns the first and last serial of part k.
func (s *Series) PartRange(k uint64) (first, last uint64, err error) {
	end, err := s.MintsThrough(k)
	if err != nil {
		return 0, 0, err
	}
	if k == 1 {
		return 0, end - 1, nil
	}
	return end - s.params.PartSize, end - 1, nil
}

// PartOf returns the part whose range holds serial: the one part a mint of
// a variable series may name.
func (s *Series) PartOf(serial uint64) (uint64, error) {
	if !s.IsVariable() {
		return 0, ErrFixed
	}
	k := uint64(1)
	if serial >= s.params.Mints {
		k = 2 + (serial-s.params.Mints)/s.params.PartSize
	}
	if err := s.checkPart(k); err != nil {
		return 0, err
	}
	return k, nil
}

// partID returns H("PoO-part" ∥ series_id ∥ k ∥ previous).
func (s *Series) partID(k uint64, previous hash.Hash) hash.Hash {
	w := encoding.NewWriter(8 + hash.Size).U64(k).Raw(previous[:])
	return s.h(hash.TagPart, s.id[:], w.Bytes())
}

// PartID returns part_id(k). Part ids follow from the series alone, so
// anyone can work out every future part id in advance. Working out part k
// takes k hashes the first time; after that, any part up to k takes at
// most partStride.
func (s *Series) PartID(k uint64) (hash.Hash, error) {
	if err := s.checkPart(k); err != nil {
		return hash.Zero, err
	}
	c := &s.parts
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.next == 0 {
		c.at = s.partID(1, s.id)
		c.next = 1
		c.marks = append(c.marks, c.at)
	}
	for c.next < k {
		c.next++
		c.at = s.partID(c.next, c.at)
		if (c.next-1)%partStride == 0 {
			c.marks = append(c.marks, c.at)
		}
	}
	if k == c.next {
		return c.at, nil
	}
	b := (k-1)/partStride + 1
	i := k - (b-1)*partStride - 1
	// The stride is worked out again when another is asked for, or when
	// it was worked out before the marks reached k.
	if c.block != b || i >= uint64(len(c.ids)) {
		first := (b-1)*partStride + 1
		id := c.marks[b-1]
		c.ids = append(c.ids[:0], id)
		for n := first + 1; n < first+partStride && n <= c.next; n++ {
			id = s.partID(n, id)
			c.ids = append(c.ids, id)
		}
		c.block = b
	}
	return c.ids[i], nil
}

// ReleaseMessage returns H("PoO-part-v1" ∥ part_id), the value a release
// witness signs in place of a tid. Its tag keeps it from ever equal to a
// tid or to the series' issuer message, so a release is never valid as
// either (PoO section 16.8).
func (s *Series) ReleaseMessage(partID hash.Hash) hash.Hash {
	return s.h(hash.TagRelease, partID[:])
}

// Release is the record that releases part k, from 2 on, of a variable
// series: the series, the part number and the previous part's id, with
// the issuer's witness beside it. That is the whole record; the range,
// the size and the units follow from the series.
type Release struct {
	Series   hash.Hash
	Part     uint64
	Previous hash.Hash
	// Witness is the issuer's witness over ReleaseMessage(part_id). It is
	// not part of the part id: any valid witness releases the same part.
	Witness policy.Witness
}

// ID returns the id of the part the release names, worked out from its own
// fields: H("PoO-part" ∥ series ∥ part ∥ previous).
func (r Release) ID(h hash.Func) hash.Hash {
	w := encoding.NewWriter(8 + hash.Size).U64(r.Part).Raw(r.Previous[:])
	return h(hash.TagPart, r.Series[:], w.Bytes())
}

// Encode returns the release's byte form: series ∥ part ∥ previous ∥
// witness.
func (r Release) Encode() []byte {
	w := encoding.NewWriter(2*hash.Size + 8 + r.Witness.Size())
	w.Raw(r.Series[:]).U64(r.Part).Raw(r.Previous[:])
	r.Witness.Encode(w)
	return w.Bytes()
}

// DecodeRelease reads the byte form written by Encode.
func DecodeRelease(b []byte) (Release, error) {
	rd := encoding.NewReader(b)
	var r Release
	rd.Into(r.Series[:])
	r.Part = rd.U64()
	rd.Into(r.Previous[:])
	r.Witness = policy.DecodeWitness(rd)
	if err := rd.Finish(); err != nil {
		return Release{}, fmt.Errorf("%w: %v", ErrRelease, err)
	}
	return r, nil
}

// releaseJSON is a release as people and files read it: the witness in
// hex of its byte form, as node.json writes an issuer witness.
type releaseJSON struct {
	Series   hash.Hash `json:"series"`
	Part     uint64    `json:"part"`
	Previous hash.Hash `json:"previous"`
	Witness  string    `json:"witness"`
}

// MarshalJSON writes the release with its witness in hex.
func (r Release) MarshalJSON() ([]byte, error) {
	w := encoding.NewWriter(r.Witness.Size())
	r.Witness.Encode(w)
	return json.Marshal(releaseJSON{r.Series, r.Part, r.Previous, hex.EncodeToString(w.Bytes())})
}

// UnmarshalJSON reads what MarshalJSON writes.
func (r *Release) UnmarshalJSON(b []byte) error {
	var j releaseJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	wb, err := hex.DecodeString(j.Witness)
	if err != nil {
		return fmt.Errorf("%w: witness: %v", ErrRelease, err)
	}
	rd := encoding.NewReader(wb)
	w := policy.DecodeWitness(rd)
	if err := rd.Finish(); err != nil {
		return fmt.Errorf("%w: witness: %v", ErrRelease, err)
	}
	*r = Release{Series: j.Series, Part: j.Part, Previous: j.Previous, Witness: w}
	return nil
}

// Unsigned returns the release of part k without its witness, for the
// issuer to sign ReleaseMessage(part_id(k)).
func (s *Series) Unsigned(k uint64) (Release, error) {
	if err := s.checkPart(k); err != nil {
		return Release{}, err
	}
	if k < 2 {
		return Release{}, fmt.Errorf("%w: part 1 is released by the issuer witness on the series", ErrRelease)
	}
	prev, err := s.PartID(k - 1)
	if err != nil {
		return Release{}, err
	}
	return Release{Series: s.id, Part: k, Previous: prev}, nil
}

// CheckRelease checks that r releases part r.Part of this series: k ≥ 2
// and within the largest number of parts, the previous id is part_id(k −
// 1), and the witness is the issuer's over ReleaseMessage(part_id(k)),
// the same check as the issuer witness on the series. The witness is
// checked first, from the release's own fields, so only the issuer can
// make a node work out a long chain of part ids.
func (s *Series) CheckRelease(r Release) error {
	if !s.IsVariable() {
		return ErrFixed
	}
	if r.Series != s.id {
		return fmt.Errorf("%w: it names series %s", ErrRelease, r.Series.Short())
	}
	if r.Part < 2 {
		return fmt.Errorf("%w: part %d; part 1 is released by the issuer witness on the series", ErrRelease, r.Part)
	}
	if err := s.checkPart(r.Part); err != nil {
		return fmt.Errorf("%w: %v", ErrRelease, err)
	}
	if err := s.reg.Check(s.h, s, r.Witness, *s.params.Issuer, s.ReleaseMessage(r.ID(s.h))); err != nil {
		return fmt.Errorf("%w: witness: %v", ErrRelease, err)
	}
	prev, err := s.PartID(r.Part - 1)
	if err != nil {
		return err
	}
	if r.Previous != prev {
		return fmt.Errorf("%w: previous part id %s is not part %d's", ErrRelease, r.Previous.Short(), r.Part-1)
	}
	return nil
}
