// Package daemon runs a full node: it opens the node in a data directory,
// runs its p2p host and its local interface, and stops them together. It
// is the one place that starts a node, so the command line (poo node) and
// any graphical front end run exactly the same thing.
//
// A running node applies a changed node policy without a restart of the
// process: on a reload (from /v1/reload, /v1/follow, Options.Reload or
// SIGHUP through the command line) it stops its host and interface, closes
// the node, and opens it again with the policy now on disk. Stores of pools
// no longer followed stay on disk, so following them again later replays
// them.
package daemon

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// Options are the settings of Run. Zero values are fine.
type Options struct {
	// Log receives the node's log lines. It is written from several
	// goroutines, one line at a time.
	Log io.Writer
	// Ready, if set, is called with the interface's address each time the
	// node is up: once at the start and again after every reload.
	Ready func(addr string)
	// Reload, if set, makes the node reload whenever a value arrives.
	Reload <-chan struct{}
	// LogLevel and LogFormat override the node policy's log section
	// (package logging); "" keeps the policy's.
	LogLevel, LogFormat string
}

// Run runs the node in dir until ctx is done, reloading it when asked.
// It returns when ctx is done or the node fails to start.
func Run(ctx context.Context, dir string, o Options) error {
	log := o.Log
	if log == nil {
		log = io.Discard
	}
	log = &lockedWriter{w: log}
	for {
		again, err := runOnce(ctx, dir, log, o)
		if err != nil || !again || ctx.Err() != nil {
			return err
		}
	}
}

// runOnce runs the node until ctx is done or a reload is asked for, and
// reports which.
func runOnce(ctx context.Context, dir string, log io.Writer, o Options) (reload bool, err error) {
	n, err := node.Open(dir, node.Options{Log: log, LogLevel: o.LogLevel, LogFormat: o.LogFormat})
	if err != nil {
		return false, err
	}
	nlog := n.Logger().With(logging.Component, "node")
	rs := n.Replayed()
	nlog.Info("started", "dir", dir, "pools", len(n.Engine().FollowedPools()),
		"records", rs.Records, "stores", rs.Pools, "kept", rs.Kept)
	cfg := p2p.FromPolicy(n.Policy())
	cfg.Logger = n.Logger()
	book, err := peers.Open(filepath.Join(dir, peers.FileName), peers.Limits{})
	if err != nil {
		return false, errors.Join(err, n.Close())
	}
	cfg.Book = book
	tr, err := tracking.New(n.Policy().TrackingConfig(), filepath.Join(dir, tracking.FileName))
	if err != nil {
		return false, errors.Join(err, n.Close())
	}
	cfg.Tracker = tr
	host, err := p2p.New(n, cfg)
	if err != nil {
		return false, errors.Join(err, n.Close())
	}
	hub := watch.NewHub(n)

	inner, cancel := context.WithCancel(ctx)
	defer cancel()
	var once sync.Once
	asked := make(chan struct{})
	ask := func() { once.Do(func() { close(asked) }) }
	go func() {
		for {
			select {
			case <-asked:
			case <-o.Reload:
				// A policy the new node could not open would stop the node
				// for good, so a reload is refused until the file is
				// right (GREEN-011). The local interface checks the same
				// before it asks.
				if _, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName)); err != nil {
					nlog.Error("reload refused; still running on the policy it started with", "err", err)
					continue
				}
				ask()
			case <-inner.Done():
				return
			}
			cancel()
			return
		}
	}()

	// The node runs while both the host and the interface do.
	p2pUp := make(chan struct{})
	herr := make(chan error, 1)
	go func() {
		err := host.Run(inner, func() { close(p2pUp) })
		cancel()
		herr <- err
	}()
	select {
	case <-p2pUp: // the host logs its address
	case err := <-herr:
		hub.Close()
		return false, errors.Join(err, n.Close())
	}
	serr := rpc.ServeWith(inner, n, rpc.ServeConfig{Host: host, Hub: hub, Reload: ask}, func(addr string) {
		n.Logger().With(logging.Component, "rpc").Info("listening", "addr", addr)
		if o.Ready != nil {
			o.Ready(addr)
		}
	})
	cancel()
	hub.Close()
	perr := <-herr
	cerr := n.Close()
	select {
	case <-asked:
		reload = ctx.Err() == nil
	default:
	}
	if reload {
		nlog.Info("reloading its node policy")
	} else {
		nlog.Info("stopped")
	}
	return reload, errors.Join(serr, perr, cerr)
}

// lockedWriter lets the node's goroutines share one output.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
