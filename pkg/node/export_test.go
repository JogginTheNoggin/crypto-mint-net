package node

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	phash "github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// SetBuildID replaces the build a snapshot is written by and checked
// against, for a test, and returns a function that puts it back.
func SetBuildID(id string) (undo func()) {
	old := buildID
	buildID = func() string { return id }
	return func() { buildID = old }
}

// WriteSnapshotWithPrint writes pool k's snapshot as a clean stop would,
// but with print in its header in place of the pool's fingerprint, so
// the file is whole and its checksum right while what it restores is not
// what it says it holds (RED-088).
func WriteSnapshotWithPrint(n *Node, k engine.PoolKey, print phash.Hash) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	c, err := n.collect(k)
	if err != nil {
		return err
	}
	c.header.print = print
	tmp, size, err := n.writeCollected(c)
	if err != nil {
		return err
	}
	return n.placeSnapshot(c, tmp, size)
}

// MakeBuildID is makeBuildID, for a test.
var MakeBuildID = makeBuildID
