package rpc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// Serve runs the interface for n on the node policy's listen address until
// ctx is done. It creates the token if needed and writes the address it
// listens on to rpc.addr (removed again on return). ready, if not nil, is
// called with that address once the server accepts connections.
func Serve(ctx context.Context, n *node.Node, ready func(addr string)) error {
	return ServeHost(ctx, n, nil, ready)
}

// ServeHost is Serve for a node that also runs a p2p host, which the
// interface reports on.
func ServeHost(ctx context.Context, n *node.Node, host *p2p.Host, ready func(addr string)) error {
	return ServeWith(ctx, n, ServeConfig{Host: host}, ready)
}

// ServeConfig is what an interface reports on and controls beside the
// node.
type ServeConfig struct {
	Host *p2p.Host
	// Hub streams events on /v1/watch. It is closed when ctx is done, which
	// ends every stream so the server can shut down.
	Hub *watch.Hub
	// Reload makes the node apply its node policy again.
	Reload func()
	// Grind runs the node's grind jobs (/v1/grind).
	Grind *grind.Manager
}

// ShutdownTimeout is how long a stopping interface waits for requests
// in progress before it closes their connections.
var ShutdownTimeout = 5 * time.Second

// ServeWith is Serve with a host, an event hub and a reload hook.
func ServeWith(ctx context.Context, n *node.Node, cfg ServeConfig, ready func(addr string)) error {
	addr := n.Policy().ListenAddr()
	if addr == "" {
		<-ctx.Done()
		return nil
	}
	tok, err := LoadOrCreateToken(n.Dir())
	if err != nil {
		return err
	}
	view, err := LoadOrCreateViewToken(n.Dir())
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	addrPath := filepath.Join(n.Dir(), AddrFile)
	if err := os.WriteFile(addrPath, []byte(ln.Addr().String()+"\n"), 0o600); err != nil {
		ln.Close()
		return err
	}
	defer os.Remove(addrPath)
	handler := NewHandler(n, tok)
	handler.SetViewToken(view)
	handler.SetHost(cfg.Host)
	handler.SetHub(cfg.Hub)
	handler.SetReload(cfg.Reload)
	handler.SetGrind(cfg.Grind)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	if ready != nil {
		ready(ln.Addr().String())
	}
	select {
	case <-ctx.Done():
		if cfg.Hub != nil {
			cfg.Hub.Close()
		}
		shut, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		err := srv.Shutdown(shut)
		if errors.Is(err, context.DeadlineExceeded) {
			// A request still running after the grace period is cut
			// off: the node is stopping either way, and that is not a
			// failure of the stop.
			err = srv.Close()
		}
		<-errc
		return err
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
