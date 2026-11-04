package nodepolicy

import (
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// FileRoom is the open files a node needs besides its pools' stores: peer
// connections, the RPC, the peer book and the tracker (ORANGE-037).
const FileRoom = 256

// FileLimit compares the open file limit with what a node may need: one
// store per followed pool that holds an item, and FileRoom more
// (ORANGE-037, D172).
type FileLimit struct {
	// Known is false where the limit cannot be read.
	Known bool   `json:"known"`
	Soft  uint64 `json:"soft"`
	// Need is the followed pools plus FileRoom.
	Need uint64 `json:"need"`
	Low  bool   `json:"low"`
}

// CheckFileLimit compares this process's soft open file limit, as Go
// raised it at start, with followed pools plus FileRoom.
func CheckFileLimit(followed uint64) FileLimit {
	soft, ok := openFileLimit()
	if !ok {
		return FileLimit{}
	}
	need := followed + FileRoom
	return FileLimit{Known: true, Soft: soft, Need: need, Low: soft < need}
}

// String says what is wrong and how to fix it, for a low limit.
func (f FileLimit) String() string {
	return fmt.Sprintf("the open file limit is %d and following these pools may need %d (one store file per pool in use, and %d for peers and the node's own files); once it runs out, stores of more pools and new peer connections fail with \"too many open files\"", f.Soft, f.Need, FileRoom)
}

// FollowedPools counts, at most, the pools p's follow rules follow among
// the accepted series: a series followed whole counts all its pools, and
// pools and mints listed count one each, never more than the series has.
// It is what the open file check needs without starting a node.
func (p Policy) FollowedPools(accepted []Accepted) uint64 {
	byID := map[hash.Hash]uint64{}
	byIssuer := map[hash.Hash][]hash.Hash{}
	for _, a := range accepted {
		byID[a.Series.ID()] = a.Series.PoolCount()
		if a.Issuer != nil {
			byIssuer[*a.Issuer] = append(byIssuer[*a.Issuer], a.Series.ID())
		}
	}
	count := map[hash.Hash]uint64{}
	for _, f := range p.Follow {
		switch {
		case f.Issuer != nil:
			for _, id := range byIssuer[*f.Issuer] {
				count[id] = byID[id]
			}
		case f.Series == nil:
		case len(f.Pools) == 0 && len(f.Mints) == 0:
			n := byID[*f.Series]
			if e := uint64(len(f.Except)); e < n {
				n -= e
			}
			count[*f.Series] = max(count[*f.Series], n)
		default:
			count[*f.Series] += uint64(len(f.Pools) + len(f.Mints))
		}
	}
	var total uint64
	for id, n := range count {
		total += min(n, byID[id])
	}
	return total
}
