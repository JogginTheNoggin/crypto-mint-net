package nodepolicy

import (
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// AddFollow adds what f names to the follow list, merging it with what is
// already there: an issuer once, a whole series in place of any parts of
// it, and pools and mints of a series into one entry for that series (none
// when the whole series is followed). The series must be listed in
// Series.
func (p *Policy) AddFollow(f Follow) error {
	if err := p.checkFollow(f); err != nil {
		return err
	}
	switch {
	case f.Issuer != nil:
		for _, g := range p.Follow {
			if g.Issuer != nil && *g.Issuer == *f.Issuer {
				return nil
			}
		}
		p.Follow = append(p.Follow, Follow{Issuer: f.Issuer})
	case len(f.Pools) == 0 && len(f.Mints) == 0:
		p.Follow = append(p.without(*f.Series), Follow{Series: f.Series})
	default:
		if p.followsWhole(*f.Series) {
			return nil
		}
		merged := Follow{Series: f.Series}
		for _, g := range p.Follow {
			if g.Series != nil && *g.Series == *f.Series {
				merged.Pools = append(merged.Pools, g.Pools...)
				merged.Mints = append(merged.Mints, g.Mints...)
			}
		}
		merged.Pools = union(merged.Pools, f.Pools)
		merged.Mints = union(merged.Mints, f.Mints)
		p.Follow = append(p.without(*f.Series), merged)
	}
	return nil
}

// RemoveFollow takes what f names off the follow list. An issuer or whole
// series comes off entirely. Pools come off a series, which is first
// written out pool by pool if it was followed whole. Mints come off the
// mints listed; a mint followed only through its pool cannot be taken off
// alone, since a mint is checked with its whole pool.
func (p *Policy) RemoveFollow(f Follow) error {
	if err := p.checkFollow(f); err != nil {
		return err
	}
	switch {
	case f.Issuer != nil:
		var keep []Follow
		for _, g := range p.Follow {
			if g.Issuer == nil || *g.Issuer != *f.Issuer {
				keep = append(keep, g)
			}
		}
		p.Follow = keep
		return nil
	case len(f.Pools) == 0 && len(f.Mints) == 0:
		p.Follow = p.without(*f.Series)
		return nil
	}
	s, err := p.series(*f.Series)
	if err != nil {
		return err
	}
	cur := Follow{Series: f.Series}
	if p.followsWhole(*f.Series) {
		for k := uint64(0); k < s.PoolCount(); k++ {
			cur.Pools = append(cur.Pools, k)
		}
	} else {
		for _, g := range p.Follow {
			if g.Series != nil && *g.Series == *f.Series {
				cur.Pools = union(cur.Pools, g.Pools)
				cur.Mints = union(cur.Mints, g.Mints)
			}
		}
	}
	cur.Pools = minus(cur.Pools, f.Pools)
	cur.Mints = minus(cur.Mints, f.Mints)
	for _, m := range f.Mints {
		k := s.PoolOf(m)
		for _, q := range cur.Pools {
			if q == k {
				return fmt.Errorf("%w: mint %d is followed through pool %d; unfollow the pool", ErrInvalid, m, k)
			}
		}
	}
	p.Follow = p.without(*f.Series)
	if len(cur.Pools) > 0 || len(cur.Mints) > 0 {
		p.Follow = append(p.Follow, cur)
	}
	return nil
}

func (p *Policy) checkFollow(f Follow) error {
	if (f.Issuer == nil) == (f.Series == nil) {
		return fmt.Errorf("%w: name exactly one of an issuer and a series", ErrInvalid)
	}
	if f.Issuer != nil {
		if len(f.Pools) > 0 || len(f.Mints) > 0 {
			return fmt.Errorf("%w: an issuer has no pools or mints", ErrInvalid)
		}
		return nil
	}
	s, err := p.series(*f.Series)
	if err != nil {
		return err
	}
	for _, k := range f.Pools {
		if k >= s.PoolCount() {
			return fmt.Errorf("%w: pool %d out of range", ErrInvalid, k)
		}
	}
	for _, m := range f.Mints {
		if m >= s.Mints() {
			return fmt.Errorf("%w: mint %d out of range", ErrInvalid, m)
		}
	}
	return nil
}

// series returns a listed series by id.
func (p *Policy) series(id hash.Hash) (*series.Series, error) {
	for _, si := range p.Series {
		s, err := series.New(si.Params)
		if err == nil && s.ID() == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("%w: series %s is not listed in the node policy", ErrInvalid, id.Short())
}

func (p *Policy) followsWhole(id hash.Hash) bool {
	for _, g := range p.Follow {
		if g.Series != nil && *g.Series == id && len(g.Pools) == 0 && len(g.Mints) == 0 {
			return true
		}
	}
	return false
}

func (p *Policy) without(id hash.Hash) []Follow {
	var keep []Follow
	for _, g := range p.Follow {
		if g.Series == nil || *g.Series != id {
			keep = append(keep, g)
		}
	}
	return keep
}

func union(a, b []uint64) []uint64 {
	set := map[uint64]bool{}
	for _, n := range append(append([]uint64(nil), a...), b...) {
		set[n] = true
	}
	out := make([]uint64, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	if len(out) == 0 {
		return nil
	}
	return out
}

func minus(a, b []uint64) []uint64 {
	drop := map[uint64]bool{}
	for _, n := range b {
		drop[n] = true
	}
	var out []uint64
	for _, n := range a {
		if !drop[n] {
			out = append(out, n)
		}
	}
	return out
}
