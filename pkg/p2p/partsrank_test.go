package p2p

import "testing"

// Black's B5 (D137): a slow chase that has waited its turn ranks as
// moving against a moving file and above a slow one that took in no more
// than it did; without a turn, D132's and RED-029's order stands.
func TestPartsRankTurns(t *testing.T) {
	file := func(slow bool, last, size, off uint64) *progress {
		return &progress{partsSlow: slow, partsLast: last, part: &partial{size: size, off: off}}
	}
	const mib = 1 << 20
	trickler := file(true, 64<<10, 100*mib, 90*mib) // slow, little to come
	honest := func(last uint64) partsRank {
		return partsRank{slow: true, turn: true, shown: file(true, last, 100*mib, 0)}
	}
	cases := []struct {
		name   string
		holder partsRank
		me     partsRank
		below  bool
	}{
		{"a lost first request has its turn", partsRank{slow: true, shown: trickler}, honest(0), true},
		{"a turn that took in more", partsRank{slow: true, shown: trickler}, honest(8 * mib), true},
		{"a turn that took in less", partsRank{slow: true, shown: file(true, 8*mib, 100*mib, 90*mib)}, honest(64 << 10), false},
		{"no turn yet: D132 stands", partsRank{slow: true, shown: trickler}, partsRank{slow: true, shown: file(true, 0, 100*mib, 0)}, false},
		{"a turn against a moving file nearer its end", partsRank{shown: file(false, 4*mib, 100*mib, 90*mib)}, honest(0), false},
		{"a turn against a moving file further from it", partsRank{shown: file(false, 4*mib, 100*mib, 10*mib)}, partsRank{slow: true, turn: true, shown: file(true, 0, 100*mib, 50*mib)}, true},
		{"a departed sender's file goes first", partsRank{away: true, shown: file(false, 4*mib, 100*mib, 99*mib)}, honest(0), true},
	}
	for _, c := range cases {
		if got := c.holder.below(c.me); got != c.below {
			t.Errorf("%s: below %v, want %v", c.name, got, c.below)
		}
	}
}
