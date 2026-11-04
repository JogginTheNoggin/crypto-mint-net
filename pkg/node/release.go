package node

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// PolicyMu serialises the edits of the node's node.json made while it
// runs: each is a read, a change and a write (RED-013). The interface and
// the p2p host share it, so a release a peer passes on and a command's
// edit never lose each other's change (D168).
func (n *Node) PolicyMu() *sync.Mutex { return &n.policyMu }

// ErrNotKnown: a release names a series the running node does not know.
var ErrNotKnown = fmt.Errorf("%w (or the node has not taken it: poo reload)", nodepolicy.ErrSeriesNotListed)

// AddRelease gives the running node r, a release of a variable series it
// knows, in place (variable supply phase 3, D168): it checks r in full,
// witness first, keeps it in node.json under PolicyMu as poo series
// add-release does, and adds it to the series the engine runs, so the
// part's pools exist at once, as after a reload onto the same node.json.
// A release of a part already known is checked as well (RED-074); the
// peer path skips those before calling, with no hashing. The p2p host
// indexes the new pools after it (p2p.Host.AddPools).
func (n *Node) AddRelease(r series.Release) (nodepolicy.ReleaseAdded, error) {
	s, ok := n.eng.Series(r.Series)
	if !ok {
		return nodepolicy.ReleaseAdded{}, fmt.Errorf("%w: %s", ErrNotKnown, r.Series.Short())
	}
	// Working out a far part takes its part number in hashes the first
	// time, so it is done before the lock (RED-075); the adds below find
	// the part ids worked out.
	if err := s.CheckRelease(r); err != nil {
		return nodepolicy.ReleaseAdded{}, err
	}
	n.policyMu.Lock()
	defer n.policyMu.Unlock()
	path := filepath.Join(n.dir, nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		return nodepolicy.ReleaseAdded{}, err
	}
	res, err := p.AddRelease(r)
	if err != nil {
		return nodepolicy.ReleaseAdded{}, err
	}
	if res.Added {
		if err := nodepolicy.Save(path, p); err != nil {
			return nodepolicy.ReleaseAdded{}, err
		}
	}
	if _, err := s.AddRelease(r); err != nil {
		return nodepolicy.ReleaseAdded{}, err
	}
	// The policy the node runs with now holds the release too, whatever
	// else waits in node.json for a reload.
	n.mu.Lock()
	run := n.run
	run.Series = slices.Clone(run.Series)
	_, rerr := run.AddRelease(r)
	if rerr == nil {
		n.run = run
	}
	n.mu.Unlock()
	res.Released, res.Mints = s.Released(), s.Mints()
	return res, nil
}
