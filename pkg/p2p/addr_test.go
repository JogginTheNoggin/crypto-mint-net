package p2p

import (
	"net/netip"
	"testing"
)

// RED-001: only public unicast addresses count as public (decision D96).
func TestPublicIP(t *testing.T) {
	for a, want := range map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700::1111": true, "::ffff:8.8.8.8": true,
		"127.0.0.1": false, "127.5.5.5": false, "::1": false, "0.0.0.0": false, "::": false,
		"10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false, "fd00::1": false,
		"169.254.169.254": false, "fe80::1": false, "100.64.0.1": false, "192.0.2.7": false,
		"198.18.0.1": false, "203.0.113.9": false, "240.0.0.1": false, "255.255.255.255": false,
		"224.0.0.1": false, "ff02::1": false, "2001:db8::1": false, "::ffff:127.0.0.1": false,
		"::ffff:10.0.0.1": false, "0.1.2.3": false,
	} {
		if got := publicIP(netip.MustParseAddr(a)); got != want {
			t.Errorf("publicIP(%s) = %v, want %v", a, got, want)
		}
	}
}

func TestLearnable(t *testing.T) {
	h := &Host{}
	for a, want := range map[string]bool{
		"8.8.8.8:8646": true, "[2606:4700::1111]:8646": true, "node.example.org:8646": true,
		"127.0.0.1:9": false, "[::1]:9": false, "10.0.0.5:22": false, "169.254.169.254:80": false,
		"[fe80::1%eth0]:9": false, "garbage": false,
	} {
		if got := h.learnable(a); got != want {
			t.Errorf("learnable(%s) = %v, want %v", a, got, want)
		}
	}
	h.cfg.AllowLocalPeers = true
	if !h.learnable("127.0.0.1:9") {
		t.Error("AllowLocalPeers refused loopback")
	}
}
