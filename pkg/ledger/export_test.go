package ledger

import (
	"fmt"
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// CheckOff checks that the pool's set of off-chain nodes holds exactly the
// held entries outside the selected ledger, and that Branches agrees with a
// scan of every entry held.
func CheckOff(p *Pool) error {
	off := 0
	var leaves []*node
	for _, n := range p.nodes {
		_, in := p.off[n]
		if in == n.onChain {
			return fmt.Errorf("entry %s: on chain %v, in off set %v", n.hash.Short(), n.onChain, in)
		}
		if !n.onChain {
			off++
			if len(n.children) == 0 {
				leaves = append(leaves, n)
			}
		}
	}
	var sum weight.Weight
	for _, n := range p.chain {
		sum = sum.Add(n.own)
	}
	if sum.Cmp(p.bit.total) != 0 || sum.Cmp(p.Weight()) != 0 {
		return fmt.Errorf("selected weight %s, cached %s, want %s", p.Weight(), p.bit.total, sum)
	}
	if off != len(p.off) {
		return fmt.Errorf("off set holds %d nodes, want %d", len(p.off), off)
	}
	sort.Slice(leaves, func(i, j int) bool { return p.better(leaves[i], leaves[j]) })
	bs := p.Branches()
	if len(bs) != len(leaves) {
		return fmt.Errorf("%d branches, want %d", len(bs), len(leaves))
	}
	for i, b := range bs {
		if b.Tip != leaves[i].hash || b.Weight.Cmp(p.weightOf(leaves[i])) != 0 {
			return fmt.Errorf("branch %d is %s, want %s", i, b.Tip.Short(), leaves[i].hash.Short())
		}
	}
	return nil
}
