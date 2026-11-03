package nodepolicy

import (
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// MaxSections is the number of separate sections a node may announce as
// what it follows (a whole series is one, each pool of a series followed
// by pools is one): the wire's limit on a follow request. A change to the
// follow list that would need more is refused when it is made, not at the
// next start (RED-050, D154). It limits how a follow list is written, not
// what a node can follow: a whole series is one section, and Except takes
// pools off it.
const MaxSections = 4096

// AddFollow adds what f names to the follow list, merging it with what is
// already there into one entry per series (joinFollow): an issuer once, a
// whole series in place of any parts of it, pools and mints of a series
// together. The series must be listed in Series.
func (p *Policy) AddFollow(f Follow) error {
	if err := p.checkFollow(f); err != nil {
		return err
	}
	if f.Issuer != nil {
		for _, g := range p.Follow {
			if g.Issuer != nil && *g.Issuer == *f.Issuer {
				return nil
			}
		}
		p.Follow = append(p.Follow, Follow{Issuer: f.Issuer})
		return nil
	}
	s, _ := p.series(*f.Series)
	merged := normalFollow(f)
	for _, g := range p.Follow {
		if g.Series != nil && *g.Series == *f.Series {
			merged = joinFollow(g, merged, s)
		}
	}
	return p.setFollow(append(p.without(*f.Series), merged))
}

// RemoveFollow takes what f names off the follow list. An issuer or whole
// series comes off entirely. Pools come off a series: off its list, or,
// when it is followed whole, onto its Except list, so all but a few pools
// of a large series stays one entry (RED-050, D154); when fewer pools are
// left than are taken off, the pools left are listed instead. Mints come off the
// mints listed; a mint followed only through its pool cannot be taken off
// alone, since a mint is checked with its whole pool.
func (p *Policy) RemoveFollow(f Follow) error {
	if err := p.checkFollow(f); err != nil {
		return err
	}
	if len(f.Except) > 0 {
		return fmt.Errorf("%w: a removal names pools or mints, not except", ErrInvalid)
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
	var cur Follow
	found := false
	for _, g := range p.Follow {
		if g.Series != nil && *g.Series == *f.Series {
			if !found {
				cur, found = normalFollow(g), true
			} else {
				cur = joinFollow(cur, g, s)
			}
		}
	}
	if !found {
		return nil
	}
	if wholeFollow(cur) {
		except := map[uint64]bool{}
		for _, k := range cur.Except {
			except[k] = true
		}
		for _, m := range f.Mints {
			if k := s.PoolOf(m); !except[k] {
				return fmt.Errorf("%w: mint %d is followed through pool %d; unfollow the pool", ErrInvalid, m, k)
			}
		}
		cur.Except = union(cur.Except, f.Pools)
		rest := p.without(*f.Series)
		left := s.PoolCount() - uint64(len(cur.Except))
		switch {
		case left == 0:
		case left <= uint64(len(cur.Except)):
			// The shorter form: the pools left, listed.
			out := Follow{Series: cur.Series}
			ex := map[uint64]bool{}
			for _, k := range cur.Except {
				ex[k] = true
			}
			for k := uint64(0); k < s.PoolCount(); k++ {
				if !ex[k] {
					out.Pools = append(out.Pools, k)
				}
			}
			rest = append(rest, out)
		default:
			rest = append(rest, cur)
		}
		return p.setFollow(rest)
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
	rest := p.without(*f.Series)
	if len(cur.Pools) > 0 || len(cur.Mints) > 0 {
		rest = append(rest, cur)
	}
	return p.setFollow(rest)
}

// ReplaceFollow puts f in place of whatever the follow list holds for
// f's series: its pools and mints, the whole series when f names none, or
// the whole series but Except. It is one change, so a node that applies
// it never has the series off its list in between (GUI-018, D139).
func (p *Policy) ReplaceFollow(f Follow) error {
	if f.Series == nil || f.Issuer != nil {
		return fmt.Errorf("%w: a replace names a series and no issuer", ErrInvalid)
	}
	if err := p.checkFollow(f); err != nil {
		return err
	}
	return p.setFollow(append(p.without(*f.Series), normalFollow(f)))
}

// addFollows adds every entry of fs as AddFollow does, in time linear in
// the entries: look finds a listed series by id without building every
// series again, and the follow list is indexed once (RED-044, D150). The
// result follows what AddFollow one by one would, except that an entry it
// changes keeps its place instead of moving to the end. On an error p is
// unchanged.
func (p *Policy) addFollows(fs []Follow, look func(hash.Hash) (*series.Series, error)) error {
	for _, f := range fs {
		if err := checkFollowIn(f, look); err != nil {
			return err
		}
	}
	out := make([]Follow, 0, len(p.Follow)+len(fs))
	issuers := map[hash.Hash]bool{}
	at := map[hash.Hash]int{}
	add := func(f Follow) {
		if f.Issuer != nil {
			if !issuers[*f.Issuer] {
				issuers[*f.Issuer] = true
				out = append(out, Follow{Issuer: f.Issuer})
			}
			return
		}
		i, ok := at[*f.Series]
		if !ok {
			at[*f.Series] = len(out)
			out = append(out, normalFollow(f))
			return
		}
		s, _ := look(*f.Series)
		out[i] = joinFollow(out[i], f, s)
	}
	for _, f := range p.Follow {
		add(f)
	}
	for _, f := range fs {
		add(f)
	}
	return p.setFollowWith(out, look)
}

// wholeFollow reports whether f follows its series whole, but for Except.
func wholeFollow(f Follow) bool { return f.Series != nil && len(f.Pools) == 0 && len(f.Mints) == 0 }

// normalFollow returns f with its lists sorted and without repeats.
func normalFollow(f Follow) Follow {
	if f.Issuer != nil {
		return Follow{Issuer: f.Issuer}
	}
	return Follow{Series: f.Series, Pools: union(nil, f.Pools), Mints: union(nil, f.Mints), Except: union(nil, f.Except)}
}

// joinFollow returns the follow of series s that follows every pool and
// mint that a or b follows. A mint of a pool taken off a whole series
// puts the pool back, as the engine follows a mint with its whole pool.
func joinFollow(a, b Follow, s *series.Series) Follow {
	a, b = normalFollow(a), normalFollow(b)
	out := Follow{Series: a.Series}
	switch {
	case wholeFollow(a) && wholeFollow(b):
		out.Except = intersect(a.Except, b.Except)
	case wholeFollow(a) || wholeFollow(b):
		w, parts := a, b
		if wholeFollow(b) {
			w, parts = b, a
		}
		back := append([]uint64(nil), parts.Pools...)
		for _, m := range parts.Mints {
			back = append(back, s.PoolOf(m))
		}
		out.Except = minus(w.Except, back)
	default:
		out.Pools, out.Mints = union(a.Pools, b.Pools), union(a.Mints, b.Mints)
	}
	return normalFollow(out)
}

// setFollow sets the follow list if it needs no more than MaxSections
// sections, and leaves p unchanged otherwise.
func (p *Policy) setFollow(fs []Follow) error { return p.setFollowWith(fs, p.series) }

func (p *Policy) setFollowWith(fs []Follow, look func(hash.Hash) (*series.Series, error)) error {
	n := 0
	for _, f := range fs {
		switch {
		case f.Issuer != nil:
			for _, si := range p.Series {
				if si.Params.Issuer != nil && *si.Params.Issuer == *f.Issuer {
					n++
				}
			}
		case wholeFollow(f):
			n++
		default:
			pools := map[uint64]bool{}
			for _, k := range f.Pools {
				pools[k] = true
			}
			if len(f.Mints) > 0 {
				if s, err := look(*f.Series); err == nil {
					for _, m := range f.Mints {
						pools[s.PoolOf(m)] = true
					}
				}
			}
			n += len(pools)
		}
	}
	if n > MaxSections {
		return fmt.Errorf("%w: the follow list would need %d separate sections, and a node announces at most %d: follow the whole series and take the pools you do not want off it (poo unfollow -pool)", ErrInvalid, n, MaxSections)
	}
	p.Follow = fs
	return nil
}

func (p *Policy) checkFollow(f Follow) error { return checkFollowIn(f, p.series) }

func checkFollowIn(f Follow, look func(hash.Hash) (*series.Series, error)) error {
	if (f.Issuer == nil) == (f.Series == nil) {
		return fmt.Errorf("%w: name exactly one of an issuer and a series", ErrInvalid)
	}
	if f.Issuer != nil {
		if len(f.Pools) > 0 || len(f.Mints) > 0 || len(f.Except) > 0 {
			return fmt.Errorf("%w: an issuer has no pools, mints or except", ErrInvalid)
		}
		return nil
	}
	if len(f.Except) > 0 && (len(f.Pools) > 0 || len(f.Mints) > 0) {
		return fmt.Errorf("%w: except goes with a whole series, not with pools or mints", ErrInvalid)
	}
	s, err := look(*f.Series)
	if err != nil {
		return err
	}
	for _, k := range append(append([]uint64(nil), f.Pools...), f.Except...) {
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

func intersect(a, b []uint64) []uint64 {
	in := map[uint64]bool{}
	for _, n := range b {
		in[n] = true
	}
	var out []uint64
	for _, n := range a {
		if in[n] {
			out = append(out, n)
		}
	}
	return union(nil, out)
}
