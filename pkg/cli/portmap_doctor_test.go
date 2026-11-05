package cli

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// doctor shows the port mapping only when the policy asks for one, warns
// for every state that leaves the node unreachable, and names the address
// to set by hand when the router cannot loop back (D182).
func TestPortMapCheck(t *testing.T) {
	if _, ok := portMapCheck(rpc.StatusReply{}); ok {
		t.Fatal("a check with no peer host")
	}
	if _, ok := portMapCheck(rpc.StatusReply{PortMap: &p2p.PortMapStatus{Mode: "off", State: "off"}}); ok {
		t.Fatal("a check with port mapping off")
	}
	for _, c := range []struct {
		st     p2p.PortMapStatus
		result string
		want   string
	}{
		{p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapAdvertised, Method: "pcp", Advertised: "203.0.113.7:8646", LeaseSeconds: 3600}, checkOK, "203.0.113.7:8646 (pcp, leased for 1h0m0s"},
		{p2p.PortMapStatus{Mode: "upnp", State: p2p.PortMapAdvertised, Method: "upnp", Advertised: "203.0.113.7:8646"}, checkOK, "until the node stops"},
		{p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapSkipped}, checkOK, "p2p.advertise"},
		{p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapUnconfirmed, External: "203.0.113.7:8646"}, checkWarn, "did not reach"},
		{p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapNotPublic, External: "100.64.0.9:8646"}, checkWarn, "not public"},
		{p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapFailed, Error: "no gateway"}, checkWarn, "no gateway"},
	} {
		st := c.st
		got, ok := portMapCheck(rpc.StatusReply{PortMap: &st})
		if !ok || got.Result != c.result || !strings.Contains(got.Detail, c.want) || got.Name != "port-map" {
			t.Errorf("%s: %+v", c.st.State, got)
		}
		if got.Result == checkWarn && got.Fix == "" {
			t.Errorf("%s: a warning without a fix", c.st.State)
		}
	}
	got, _ := portMapCheck(rpc.StatusReply{PortMap: &p2p.PortMapStatus{Mode: "auto", State: p2p.PortMapUnconfirmed, External: "203.0.113.7:8646"}})
	if !strings.Contains(got.Fix, "set p2p.advertise to it") || !strings.Contains(got.Fix, "203.0.113.7:8646") {
		t.Errorf("fix %q", got.Fix)
	}
}
