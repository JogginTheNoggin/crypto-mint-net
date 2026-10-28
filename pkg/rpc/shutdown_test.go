package rpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// TestStopCutsOffSlowRequests: a request still open when the grace period
// ends is cut off, and the stop is not reported as a failure (GUI
// GREEN-015 item 3).
func TestStopCutsOffSlowRequests(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "127.0.0.1:0")
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	old := rpc.ShutdownTimeout
	rpc.ShutdownTimeout = 50 * time.Millisecond
	defer func() { rpc.ShutdownTimeout = old }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addrc := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- rpc.ServeWith(ctx, n, rpc.ServeConfig{}, func(a string) { addrc <- a }) }()
	var addr string
	select {
	case addr = <-addrc:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not start")
	}
	// Half a request: the server waits for the rest past the grace period.
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("GET /v1/status HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop with a slow request: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not cut the slow request off")
	}
}
