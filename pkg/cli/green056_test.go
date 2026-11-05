package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// Doctor's port-map line names the one reason a mapping is skipped, the
// router it asked, and when it asks again (GREEN-056).
func TestPortMapCheckSaysWhyWhereWhen(t *testing.T) {
	got, _ := portMapCheck(rpc.StatusReply{PortMap: &p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapSkipped, Reason: p2p.PortMapReasonNoListen}})
	if !strings.HasSuffix(got.Detail, "not used: "+p2p.PortMapReasonNoListen) {
		t.Errorf("skipped: %q", got.Detail)
	}
	got, _ = portMapCheck(rpc.StatusReply{PortMap: &p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapFailed, Error: "upnp: no gateway answered the search",
		Gateway: "192.0.2.1", NextTry: time.Now().Add(4*time.Minute + 30*time.Second)}})
	if !strings.Contains(got.Detail, "asked the router at 192.0.2.1") || !strings.Contains(got.Detail, "asks again in 4m") ||
		!strings.Contains(got.Fix, "poo reload tries again at once") {
		t.Errorf("failed: %+v", got)
	}
	got, _ = portMapCheck(rpc.StatusReply{PortMap: &p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapFailed, Error: portmapNoGateway}})
	if !strings.Contains(got.Detail, "no gateway was found") || strings.Contains(got.Detail, "asks again") {
		t.Errorf("no gateway: %+v", got)
	}
}

const portmapNoGateway = "portmap: no gateway found; set p2p.port_map_gateway"
