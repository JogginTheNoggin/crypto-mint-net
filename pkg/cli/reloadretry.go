package cli

import (
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// reloadWait is how long a command waits for a node that is reloading.
var reloadWait = 20 * time.Second

// dropped reports whether err is the connection to the node going away
// rather than an answer: what a call sees when it reaches a node while a
// reload restarts its RPC.
func dropped(err error) bool {
	var re *rpc.Error
	if err == nil || errors.As(err, &re) {
		return false
	}
	var ne net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		(errors.As(err, &ne) && !ne.Timeout())
}

// callThroughReload makes call to the running node, and when the
// connection drops because the node is reloading (an earlier poon policy
// limits -set or add-release, say), dials it again and makes the call
// again, for up to reloadWait. Only calls that may be made twice use it:
// setting values and adding a release both are. Before, the second of two
// quick -set calls could fail with a bare EOF (GUI/Blue, D166).
func callThroughReload[T any](out io.Writer, dir string, cl *rpc.Client, call func(*rpc.Client) (T, error)) (T, error) {
	v, err := call(cl)
	deadline := time.Now().Add(reloadWait)
	for dropped(err) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		c, derr := dialNode(out, dir)
		if derr != nil {
			continue
		}
		v, err = call(c)
	}
	return v, err
}

// dialThroughReload dials the running node. When no node answers but one
// holds the data directory, it is between stopping and starting its RPC
// in a reload, so this dials again for up to reloadWait. Before, a second
// add-release or -set made in that gap fell to the offline path and failed
// with "a node or another command holds ..." (GREEN-050). With no node at
// all it returns the dial error at once, for the offline path.
func dialThroughReload(out io.Writer, dir string) (*rpc.Client, error) {
	cl, err := dialNode(out, dir)
	deadline := time.Now().Add(reloadWait)
	for err != nil && time.Now().Before(deadline) {
		unlock, lerr := nodepolicy.Lock(dir)
		if lerr == nil {
			unlock()
			return nil, err
		}
		if !errors.Is(lerr, nodepolicy.ErrLocked) {
			return nil, err
		}
		time.Sleep(200 * time.Millisecond)
		cl, err = dialNode(out, dir)
	}
	return cl, err
}
