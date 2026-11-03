package rpc_test

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// dirFor writes rpc.addr, rpc.token and rpc.view-token for addr.
func dirFor(t *testing.T, addr string) (dir, full, view string) {
	t.Helper()
	dir = t.TempDir()
	full, view = strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	for name, v := range map[string]string{rpc.AddrFile: addr, rpc.TokenFile: full, rpc.ViewTokenFile: view} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, full, view
}

// RED-066, D162: a stranger listening at the address a crashed node left in
// rpc.addr never receives the token: Dial reports the node as not running.
func TestDialSendsNoTokenToAStranger(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	for _, answer := range []func(w http.ResponseWriter){
		func(w http.ResponseWriter) { http.Error(w, "\x1b[31mnot found", 404) },
		func(w http.ResponseWriter) { w.Write([]byte(`{"full":"00","view":"00"}`)) },
		func(w http.ResponseWriter) { w.Write([]byte(`{"full":` + strings.Repeat("9", 1<<16))) },
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen = append(seen, r.Header.Get("Authorization"))
			mu.Unlock()
			answer(w)
		}))
		dir, _, _ := dirFor(t, srv.Listener.Addr().String())
		for _, dial := range []func(string) (*rpc.Client, error){rpc.Dial, rpc.DialView} {
			_, err := dial(dir)
			if !errors.Is(err, rpc.ErrNotRunning) || !errors.Is(err, rpc.ErrNoProof) {
				t.Fatalf("a stranger was trusted: %v", err)
			}
			if strings.Contains(err.Error(), "\x1b") {
				t.Fatalf("the stranger's text reached the message: %q", err)
			}
		}
		srv.Close()
	}
	for _, h := range seen {
		if h != "" {
			t.Fatalf("a token was sent: %q", h)
		}
	}

	// Nothing listening: not running, as the dial error says.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	dir, _, _ := dirFor(t, addr)
	var oe *net.OpError
	if _, err := rpc.Dial(dir); !errors.Is(err, rpc.ErrNotRunning) || !errors.As(err, &oe) {
		t.Fatalf("nothing listening: %v", err)
	}
}

// The node's own interface proves itself for the address it wrote, with
// either token; the same server named by another address does not.
func TestDialProvesTheNode(t *testing.T) {
	full, view := strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	h := rpc.NewHandler(nil, full)
	h.SetViewToken(view)
	srv := httptest.NewUnstartedServer(h)
	srv.Start()
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	h.SetAddr(addr)
	dir, _, _ := dirFor(t, addr)
	if _, err := rpc.Dial(dir); err != nil {
		t.Fatalf("the node itself: %v", err)
	}
	if _, err := rpc.DialView(dir); err != nil {
		t.Fatalf("the node itself, view token: %v", err)
	}
	// The same node seen at another address (a relay) proves nothing.
	_, port, _ := net.SplitHostPort(addr)
	other, _, _ := dirFor(t, "localhost:"+port)
	if _, err := rpc.Dial(other); !errors.Is(err, rpc.ErrNoProof) {
		t.Fatalf("a proof for another address was taken: %v", err)
	}
	// A wrong token in the directory: the node's proof does not match it.
	wrong, _, _ := dirFor(t, addr)
	os.WriteFile(filepath.Join(wrong, rpc.TokenFile), []byte(strings.Repeat("ef", 32)), 0o600)
	if _, err := rpc.Dial(wrong); !errors.Is(err, rpc.ErrNoProof) {
		t.Fatalf("a wrong token: %v", err)
	}
	// A bad nonce is refused.
	resp, err := http.Get("http://" + addr + rpc.HelloPath + "?nonce=zz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a bad nonce: HTTP %d", resp.StatusCode)
	}
}
