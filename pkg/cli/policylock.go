package cli

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// lockPolicy takes the data directory's lock, without waiting, before a
// command changes node.json because no node answered. A running node
// holds that lock, so a change is never written behind a node that
// answered with an error or did not answer at all (its next write would
// undo it), nor while one starts or another command writes. A leftover
// rpc.addr from a crash holds no lock, so the change goes on (GUI-017,
// D134; RED-030, D136). nodeErr, if set, is why the node could not be
// used although it is running. On a platform with no lock the change
// goes on, as before.
func lockPolicy(dir, what string, nodeErr error) (func(), error) {
	unlock, err := nodepolicy.Lock(dir)
	if errors.Is(err, nodepolicy.ErrLocked) {
		if nodeErr != nil {
			return nil, fmt.Errorf("the node is running but its %s could not be read or changed, so nothing was changed: %w", what, nodeErr)
		}
		return nil, fmt.Errorf("a node or another command holds %s, and no node answered, so nothing was changed: try again once it has started or ended: %w", dir, err)
	}
	if err != nil {
		return nil, err
	}
	return unlock, nil
}
