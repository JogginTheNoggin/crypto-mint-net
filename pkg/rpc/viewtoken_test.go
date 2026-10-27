package rpc_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// TestViewToken checks the view token reads every GET endpoint and
// changes nothing, while rpc.token keeps full control.
func TestViewToken(t *testing.T) {
	dir := t.TempDir()
	s := writePolicy(t, dir, "127.0.0.1:0")
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(ctx, n, func(a string) { ready <- a }) }()
	var addr string
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not start")
	}
	defer func() { cancel(); <-done }()

	st, err := os.Stat(filepath.Join(dir, rpc.ViewTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("view token mode %v", st.Mode().Perm())
	}
	full, _ := rpc.LoadOrCreateToken(dir)
	view, _ := rpc.LoadOrCreateViewToken(dir)
	if full == view {
		t.Fatal("the view token is the full token")
	}

	v, err := rpc.DialView(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Status(); err != nil {
		t.Fatalf("view token cannot read status: %v", err)
	}
	if code, _, _ := scrape(t, "http://"+addr, view); code != 200 {
		t.Fatalf("view token cannot read metrics: %d", code)
	}
	k := testutil.NewKeys(s)
	claim := k.Claim(7, P("z", 100))
	if _, err := v.SubmitTx(claim); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token submitted a transaction: %v", err)
	}
	if err := v.Reload(); code(err) != rpc.CodeForbidden {
		t.Fatalf("view token reloaded: %v", err)
	}
	c, err := rpc.Dial(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubmitTx(claim); err != nil {
		t.Fatalf("full token: %v", err)
	}
	if _, err := rpc.NewClient(addr, "x"+view[1:]).Status(); code(err) != rpc.CodeUnauthorized {
		t.Fatalf("wrong token: %v", err)
	}
}
