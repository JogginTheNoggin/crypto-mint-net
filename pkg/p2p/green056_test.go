package p2p_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// Status says why a mapping is skipped, which router was asked, and when
// the node asks again (GREEN-056).
func TestPortMapStatusSaysWhyWhereWhen(t *testing.T) {
	t.Cleanup(p2p.SetPortMapWaits(time.Hour, time.Hour))
	addr := freeAddr(t)
	f := &fakeMapper{ext: func(int) (string, error) { return "", errors.New("none") }}
	h, _ := startMapped(t, addr, f, func(c *p2p.Config) { c.Advertise = addr })
	if st := h.PortMap(); st.State != p2p.PortMapSkipped || st.Reason != p2p.PortMapReasonAdvertise {
		t.Fatalf("skipped: %+v", st)
	}

	h, _ = startMapped(t, freeAddr(t), f, func(c *p2p.Config) { c.PortMapGateway = "192.0.2.1" })
	st := waitState(t, h, p2p.PortMapFailed)
	for st.NextTry.IsZero() {
		time.Sleep(5 * time.Millisecond)
		st = h.PortMap()
	}
	if st.Gateway != "192.0.2.1" || time.Until(st.NextTry) < 50*time.Minute {
		t.Fatalf("failed: %+v", st)
	}
}

// A mapping that worked and then lapsed is asked for again after the first
// wait, not the longest one earlier failures built up (GREEN-056).
func TestPortMapRetryResetsAfterAHeldMapping(t *testing.T) {
	t.Cleanup(p2p.SetPortMapBackoff(50*time.Millisecond, time.Hour, 10*time.Millisecond))
	addr := freeAddr(t)
	f := &fakeMapper{ext: func(call int) (string, error) {
		switch {
		case call < 4: // 50, 100, 200, 400 ms waits: the next would be 800
			return "", errors.New("router busy")
		case call == 4:
			return addr, nil
		}
		return "", errors.New("router rebooted")
	}, life: 40 * time.Millisecond}
	h, _ := startMapped(t, addr, f, nil)
	deadline := time.Now().Add(10 * time.Second)
	for {
		st := h.PortMap()
		if strings.HasPrefix(st.Error, "renewal:") && !st.NextTry.IsZero() {
			if wait := st.NextTry.Sub(st.Since); wait > 400*time.Millisecond {
				t.Fatalf("after a held mapping the next try waits %s", wait)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no lapsed renewal: %+v", st)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
