package p2p_test

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/portmap"
)

// fakeMapper grants what ext returns for each call (the call's number
// from 0), and records the calls.
type fakeMapper struct {
	mu     sync.Mutex
	ext    func(call int) (string, error)
	life   time.Duration
	maps   []string // method asked, "renew" for a renewal
	unmaps []string
}

func (f *fakeMapper) Map(_ context.Context, method string, internal uint16, _ time.Duration, prev *portmap.Mapping) (portmap.Mapping, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := len(f.maps)
	if prev != nil {
		f.maps = append(f.maps, "renew")
	} else {
		f.maps = append(f.maps, method)
	}
	s, err := f.ext(call)
	if err != nil {
		return portmap.Mapping{}, err
	}
	return portmap.Mapping{Method: portmap.MethodNATPMP, External: netip.MustParseAddrPort(s), Internal: internal, Lifetime: f.life}, nil
}

func (f *fakeMapper) Unmap(_ context.Context, m portmap.Mapping) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unmaps = append(f.unmaps, m.External.String())
	return nil
}

func (f *fakeMapper) calls() (maps, unmaps []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.maps), slices.Clone(f.unmaps)
}

// startMapped runs a host listening on addr with port mapping on through
// f; edit may change its settings first. It returns the host and a stop
// that waits for Run to return.
func startMapped(t *testing.T, addr string, f p2p.PortMapper, edit func(*p2p.Config)) (*p2p.Host, func()) {
	t.Helper()
	pol := nodepolicy.Default()
	pol.RPC.Listen, pol.NoSync = "off", true
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol))
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	cfg := p2p.FromPolicy(pol)
	cfg.Listen, cfg.AllowLocalPeers = addr, true
	cfg.CheckEvery, cfg.ManageEvery, cfg.PingEvery = -1, -1, -1
	cfg.PortMap, cfg.PortMapper = "auto", f
	if edit != nil {
		edit(&cfg)
	}
	h, err := p2p.New(n, cfg)
	must(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		defer close(done)
		if err := h.Run(ctx, func() { close(ready) }); err != nil {
			t.Error(err)
		}
	}()
	<-ready
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			cancel()
			<-done
			n.Close()
		}
	}
	t.Cleanup(stop)
	return h, stop
}

func waitState(t *testing.T, h *p2p.Host, state string) p2p.PortMapStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := h.PortMap(); st.State == state {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port map state %+v, want %s", h.PortMap(), state)
	return p2p.PortMapStatus{}
}

// The router's address is advertised once a dial to it reaches this node,
// renewed, and removed on a clean stop (D182).
func TestPortMapConfirmedAdvertisedRemoved(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(50*time.Millisecond, 50*time.Millisecond))
	addr := freeAddr(t)
	f := &fakeMapper{ext: func(int) (string, error) { return addr, nil }, life: 100 * time.Millisecond}
	h, stop := startMapped(t, addr, f, nil)
	st := waitState(t, h, p2p.PortMapAdvertised)
	if st.Advertised != addr || st.Method != "natpmp" || st.Mode != "auto" {
		t.Fatalf("status %+v", st)
	}
	if !slices.Contains(h.Listed(), addr) {
		t.Fatal("the advertised address is not among the node's own")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		maps, _ := f.calls()
		if len(maps) >= 3 {
			if maps[0] != "auto" || maps[1] != "renew" {
				t.Fatalf("calls %v", maps)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not renewed: %v", maps)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	if _, unmaps := f.calls(); !slices.Equal(unmaps, []string{addr}) {
		t.Fatalf("unmapped %v", unmaps)
	}
}

// A mapped address that leads to another node is never advertised, and
// the mapping is removed.
func TestPortMapOtherNodeUnconfirmed(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(time.Hour, time.Hour))
	other, _ := startMapped(t, freeAddr(t), &fakeMapper{ext: func(int) (string, error) { return "", errors.New("none") }}, nil)
	f := &fakeMapper{ext: func(int) (string, error) { return other.Addr(), nil }, life: time.Hour}
	h, _ := startMapped(t, freeAddr(t), f, nil)
	st := waitState(t, h, p2p.PortMapUnconfirmed)
	if st.Advertised != "" || slices.Contains(h.Listed(), other.Addr()) {
		t.Fatalf("advertised another node's address: %+v", st)
	}
	if _, unmaps := f.calls(); !slices.Equal(unmaps, []string{other.Addr()}) {
		t.Fatalf("unmapped %v", unmaps)
	}
}

// A router behind another NAT reports a private address: refused.
func TestPortMapNotPublic(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(time.Hour, time.Hour))
	f := &fakeMapper{ext: func(int) (string, error) { return "100.64.1.2:8646", nil }, life: time.Hour}
	h, _ := startMapped(t, freeAddr(t), f, func(c *p2p.Config) { c.AllowLocalPeers = false })
	waitState(t, h, p2p.PortMapNotPublic)
	if _, unmaps := f.calls(); len(unmaps) != 1 {
		t.Fatalf("unmapped %v", unmaps)
	}
}

// A failure is retried after the wait.
func TestPortMapRetries(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(20*time.Millisecond, time.Hour))
	addr := freeAddr(t)
	f := &fakeMapper{ext: func(call int) (string, error) {
		if call < 2 {
			return "", errors.New("router busy")
		}
		return addr, nil
	}, life: time.Hour}
	h, _ := startMapped(t, addr, f, nil)
	waitState(t, h, p2p.PortMapAdvertised)
}

// An address set by hand always wins: no mapping is asked for.
func TestPortMapManualAdvertiseWins(t *testing.T) {
	f := &fakeMapper{ext: func(int) (string, error) { return "203.0.113.5:1", nil }}
	addr := freeAddr(t)
	h, _ := startMapped(t, addr, f, func(c *p2p.Config) { c.Advertise = addr })
	time.Sleep(50 * time.Millisecond)
	if st := h.PortMap(); st.State != p2p.PortMapSkipped {
		t.Fatalf("state %+v", st)
	}
	if maps, _ := f.calls(); len(maps) != 0 {
		t.Fatalf("asked the router: %v", maps)
	}
}

// Off by default.
func TestPortMapOffByDefault(t *testing.T) {
	f := &fakeMapper{ext: func(int) (string, error) { return "203.0.113.5:1", nil }}
	h, _ := startMapped(t, freeAddr(t), f, func(c *p2p.Config) { c.PortMap = "" })
	time.Sleep(50 * time.Millisecond)
	if st := h.PortMap(); st.State != p2p.PortMapOff || st.Mode != "off" {
		t.Fatalf("state %+v", st)
	}
	if maps, _ := f.calls(); len(maps) != 0 {
		t.Fatalf("asked the router: %v", maps)
	}
	if p2p.FromPolicy(nodepolicy.Default()).PortMap != "" {
		t.Fatal("the default policy maps a port")
	}
}
