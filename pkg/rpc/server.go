package rpc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
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
	addr := n.Policy().ListenAddr()
	if addr == "" {
		<-ctx.Done()
		return nil
	}
	tok, err := LoadOrCreateToken(n.Dir())
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
	handler.SetHost(host)
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
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.Shutdown(shut)
		<-errc
		return err
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
