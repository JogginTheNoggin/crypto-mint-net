package p2p_test

import (
	"bufio"
	"io"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// BLUE-032 (RED-090, RED-092): a dial-back counts only when this node's
// own listener received it; the renewal does not follow a router's long
// lifetime, a too-short one is refused, and a stop does not wait on a
// silent dial-back.

// fakeAt runs a TCP server that hands each connection to serve, and
// returns its address and a channel that receives each accept.
func fakeAt(t *testing.T, serve func(net.Conn)) (string, <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan struct{}, 16)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case accepted <- struct{}{}:
			default:
			}
			go func() {
				defer c.Close()
				serve(c)
			}()
		}
	}()
	return ln.Addr().String(), accepted
}

// An address that is not this node's is never confirmed, whether it
// echoes the bytes back or answers with a copy of the node's own hello.
func TestBlue032DialBackNeedsOwnListener(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(time.Hour, time.Hour))
	t.Cleanup(p2p.SetPortMapConfirmWait(time.Second))
	servers := map[string]func(net.Conn){
		"echo": func(c net.Conn) { io.Copy(c, c) },
		"copied hello": func(c net.Conn) {
			m, _, err := wire.ReadFrame(bufio.NewReader(c), wire.MaxFrame)
			if err != nil {
				return
			}
			hello, err := wire.DecodeHello(m.Body)
			if err != nil {
				return
			}
			wire.WriteFrame(c, wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: hello.Encode()})
			io.Copy(io.Discard, c)
		},
	}
	for name, serve := range servers {
		t.Run(name, func(t *testing.T) {
			fake, _ := fakeAt(t, serve)
			f := &fakeMapper{ext: func(int) (string, error) { return fake, nil }, life: time.Hour}
			h, stop := startMapped(t, freeAddr(t), f, nil)
			st := waitState(t, h, p2p.PortMapUnconfirmed)
			if st.Advertised != "" || slices.Contains(h.Listed(), fake) {
				t.Fatalf("advertised an address that is not this node's: %+v", st)
			}
			stop()
			if _, unmaps := f.calls(); !slices.Equal(unmaps, []string{fake}) {
				t.Fatalf("unmapped %v", unmaps)
			}
		})
	}
}

// A stop while the dial-back waits on an address that accepts and says
// nothing returns at once, and still removes the mapping.
func TestBlue032StopDoesNotWaitOnSilentDialBack(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(time.Hour, time.Hour))
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	silent, accepted := fakeAt(t, func(net.Conn) { <-block })
	f := &fakeMapper{ext: func(int) (string, error) { return silent, nil }, life: time.Hour}
	_, stop := startMapped(t, freeAddr(t), f, nil)
	select {
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("the dial-back never came")
	}
	start := time.Now()
	stop()
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("stop took %s", d)
	}
	if _, unmaps := f.calls(); !slices.Equal(unmaps, []string{silent}) {
		t.Fatalf("unmapped %v", unmaps)
	}
}

// A lifetime too short to renew in time is refused and removed, never
// advertised.
func TestBlue032ShortLifetimeRefused(t *testing.T) {
	addr := freeAddr(t)
	f := &fakeMapper{ext: func(int) (string, error) { return addr, nil }, life: 2 * time.Second}
	h, stop := startMapped(t, addr, f, nil)
	st := waitState(t, h, p2p.PortMapFailed)
	if st.Advertised != "" || slices.Contains(h.Listed(), addr) {
		t.Fatalf("advertised a mapping that lapses before renewal: %+v", st)
	}
	stop()
	if _, unmaps := f.calls(); !slices.Equal(unmaps, []string{addr}) {
		t.Fatalf("unmapped %v", unmaps)
	}
}

// The renewal comes at half of the asked lease when the router states a
// longer lifetime, so a changed external address is found in time.
func TestBlue032RenewalCappedByLease(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(time.Hour, 10*time.Millisecond))
	addr := freeAddr(t)
	f := &fakeMapper{ext: func(int) (string, error) { return addr, nil }, life: 100 * 365 * 24 * time.Hour}
	h, _ := startMapped(t, addr, f, func(c *p2p.Config) { c.PortMapLease = 200 * time.Millisecond })
	waitState(t, h, p2p.PortMapAdvertised)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if maps, _ := f.calls(); len(maps) >= 2 && maps[1] == "renew" {
			break
		}
		if time.Now().After(deadline) {
			maps, _ := f.calls()
			t.Fatalf("not renewed at half the lease: %v", maps)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := h.RenewAfter(0); got != 100*time.Millisecond {
		t.Fatalf("a lease until removed renews after %s", got)
	}
	if got := h.RenewAfter(time.Millisecond); got != 10*time.Millisecond {
		t.Fatalf("renewal sooner than the floor: %s", got)
	}
}
