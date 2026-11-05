package nodepolicy_test

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// p2p.port_map is off by default, takes only its named methods and a
// gateway IP, and stays on the machine it was set on (D182).
func TestPortMapSettings(t *testing.T) {
	p := nodepolicy.Default()
	if p.P2P.PortMap != "" || p2p.FromPolicy(p).PortMapLease != time.Hour {
		t.Fatalf("default %+v", p.P2P)
	}
	for _, m := range []string{"", "off", "auto", "pcp", "natpmp", "upnp"} {
		p.P2P.PortMap = m
		if err := p.Validate(); err != nil {
			t.Fatalf("%q: %v", m, err)
		}
	}
	for _, bad := range []string{"on", "UPNP", "socks"} {
		p.P2P.PortMap = bad
		if p.Validate() == nil {
			t.Fatalf("took %q", bad)
		}
	}
	p.P2P.PortMap = "auto"
	p.P2P.PortMapGateway = "router.lan"
	if p.Validate() == nil {
		t.Fatal("took a host name as the gateway")
	}
	p.P2P.PortMapGateway, p.P2P.PortMapSeconds = "192.168.1.1", 600
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := p2p.FromPolicy(p); got.PortMap != "auto" || got.PortMapGateway != "192.168.1.1" || got.PortMapLease != 10*time.Minute {
		t.Fatalf("host config %+v", got)
	}
	s := nodepolicy.ExportPolicy(p, false)
	if s.P2P.PortMap != "" || s.P2P.PortMapGateway != "" {
		t.Fatalf("export kept the port mapping: %+v", s.P2P)
	}
	if s.P2P.PortMapSeconds != 600 {
		t.Fatal("export lost the lease")
	}
	if full := nodepolicy.ExportPolicy(p, true); full.P2P.PortMap != "auto" || full.P2P.PortMapGateway != "192.168.1.1" {
		t.Fatalf("full export: %+v", full.P2P)
	}
}
