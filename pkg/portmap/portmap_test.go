package portmap_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/portmap"
)

var lo = netip.MustParseAddr("127.0.0.1")

// udpGateway answers each datagram with what reply returns (nil: no
// answer) and records the requests.
type udpGateway struct {
	conn *net.UDPConn
	mu   sync.Mutex
	reqs [][]byte
}

func newUDPGateway(t *testing.T, reply func([]byte) []byte) *udpGateway {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	g := &udpGateway{conn: conn}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			req := append([]byte(nil), buf[:n]...)
			g.mu.Lock()
			g.reqs = append(g.reqs, req)
			g.mu.Unlock()
			if out := reply(req); out != nil {
				conn.WriteToUDP(out, from)
			}
		}
	}()
	return g
}

func (g *udpGateway) port() uint16 { return uint16(g.conn.LocalAddr().(*net.UDPAddr).Port) }

func (g *udpGateway) requests() [][]byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([][]byte(nil), g.reqs...)
}

var extIP = [4]byte{203, 0, 113, 7}

// pcpServer grants every MAP request on external port ext.
func pcpServer(ext uint16, result byte) func([]byte) []byte {
	return func(req []byte) []byte {
		if req[0] != 2 {
			return []byte{0, 128 + req[1], 0, 1, 0, 0, 0, 0} // NAT-PMP: unsupported version
		}
		out := make([]byte, 60)
		out[0], out[1], out[3] = 2, 0x81, result
		copy(out[4:8], req[4:8])
		copy(out[24:36], req[24:36])
		out[36] = 6
		copy(out[40:42], req[40:42])
		binary.BigEndian.PutUint16(out[42:], ext)
		out[54], out[55] = 0xff, 0xff
		copy(out[56:60], extIP[:])
		return out
	}
}

// pmpServer is a NAT-PMP-only gateway.
func pmpServer(req []byte) []byte {
	switch {
	case req[0] != 0:
		return []byte{0, 128 + req[1], 0, 1, 0, 0, 0, 0} // unsupported version
	case req[1] == 0:
		out := make([]byte, 12)
		out[1] = 128
		copy(out[8:], extIP[:])
		return out
	case req[1] == 2:
		out := make([]byte, 16)
		out[1] = 130
		copy(out[8:10], req[4:6])
		ext := binary.BigEndian.Uint16(req[6:])
		if ext == 0 && binary.BigEndian.Uint32(req[8:]) != 0 {
			ext = 40000
		}
		binary.BigEndian.PutUint16(out[10:], ext)
		copy(out[12:16], req[8:12])
		return out
	}
	return nil
}

func client(pmp uint16, ssdp string) *portmap.Client {
	c := &portmap.Client{Gateway: lo, Timeout: 600 * time.Millisecond}
	c.SetPorts(pmp, ssdp)
	return c
}

func TestPCPMapRenewUnmap(t *testing.T) {
	g := newUDPGateway(t, pcpServer(41000, 0))
	c := client(g.port(), "127.0.0.1:9")
	m, err := c.Map(context.Background(), portmap.MethodAuto, 8646, time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != portmap.MethodPCP || m.External.String() != "203.0.113.7:41000" || m.Lifetime != time.Hour || m.Internal != 8646 {
		t.Fatalf("mapping %+v", m)
	}
	req := g.requests()[0]
	if req[0] != 2 || req[1] != 1 || binary.BigEndian.Uint16(req[40:]) != 8646 || binary.BigEndian.Uint16(req[42:]) != 8646 {
		t.Fatalf("request %x", req)
	}
	if got := netip.AddrFrom16([16]byte(req[8:24])).Unmap(); got != lo {
		t.Fatalf("client address %s", got)
	}
	r, err := c.Map(context.Background(), portmap.MethodAuto, 8646, time.Hour, &m)
	if err != nil {
		t.Fatal(err)
	}
	reqs := g.requests()
	if string(reqs[1][24:36]) != string(req[24:36]) || binary.BigEndian.Uint16(reqs[1][42:]) != 41000 || r.External != m.External {
		t.Fatal("renewal did not reuse the nonce and port")
	}
	if err := c.Unmap(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	last := g.requests()[2]
	if binary.BigEndian.Uint32(last[4:]) != 0 || string(last[24:36]) != string(req[24:36]) {
		t.Fatalf("unmap request %x", last)
	}
}

func TestPCPIgnoresForeignNonce(t *testing.T) {
	inner := pcpServer(41000, 0)
	g := newUDPGateway(t, func(req []byte) []byte {
		out := inner(req)
		out[24] ^= 1 // a reply to someone else's request
		return out
	})
	c := client(g.port(), "127.0.0.1:9")
	if _, err := c.Map(context.Background(), portmap.MethodPCP, 8646, time.Hour, nil); err == nil {
		t.Fatal("took a reply with another nonce")
	}
}

func TestPCPRefused(t *testing.T) {
	g := newUDPGateway(t, pcpServer(41000, 2))
	c := client(g.port(), "127.0.0.1:9")
	_, err := c.Map(context.Background(), portmap.MethodPCP, 8646, time.Hour, nil)
	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("err %v", err)
	}
}

func TestFallsBackToNATPMP(t *testing.T) {
	g := newUDPGateway(t, pmpServer)
	c := client(g.port(), "127.0.0.1:9")
	m, err := c.Map(context.Background(), portmap.MethodAuto, 8646, 2*time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != portmap.MethodNATPMP || m.External.String() != "203.0.113.7:8646" || m.Lifetime != 2*time.Hour {
		t.Fatalf("mapping %+v", m)
	}
	if err := c.Unmap(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	last := g.requests()[len(g.requests())-1]
	if last[1] != 2 || binary.BigEndian.Uint32(last[8:]) != 0 || binary.BigEndian.Uint16(last[6:]) != 0 {
		t.Fatalf("unmap request %x", last)
	}
}

func TestSilentGatewayTimesOut(t *testing.T) {
	g := newUDPGateway(t, func([]byte) []byte { return nil })
	c := client(g.port(), "127.0.0.1:9")
	start := time.Now()
	_, err := c.Map(context.Background(), portmap.MethodNATPMP, 8646, time.Hour, nil)
	if err == nil {
		t.Fatal("mapped through a silent gateway")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("waited %s", d)
	}
	// It resent rather than waiting once.
	if n := len(g.requests()); n < 2 {
		t.Fatalf("%d requests", n)
	}
}

func TestCancelStopsWait(t *testing.T) {
	g := newUDPGateway(t, func([]byte) []byte { return nil })
	c := &portmap.Client{Gateway: lo, Timeout: time.Minute}
	c.SetPorts(g.port(), "127.0.0.1:9")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Map(ctx, portmap.MethodPCP, 8646, time.Hour, nil); err == nil {
		t.Fatal("no error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("waited %s after cancel", d)
	}
}

func TestShortRepliesSkipped(t *testing.T) {
	g := newUDPGateway(t, func(req []byte) []byte { return []byte{2, 0x81} })
	c := client(g.port(), "127.0.0.1:9")
	if _, err := c.Map(context.Background(), portmap.MethodPCP, 8646, time.Hour, nil); err == nil {
		t.Fatal("took a short reply")
	}
}

// igd is a fake UPnP gateway: SSDP on UDP, description and control on
// HTTP, all on 127.0.0.1.
type igd struct {
	ssdp     *udpGateway
	srv      *httptest.Server
	location string // what SSDP answers; "" means the server's own
	locPath  string // the path on the server's own, when location is ""
	desc     string // description body; "" means a good one
	mu       sync.Mutex
	actions  []string
	mapped   map[string]string
	only725  bool
}

func newIGD(t *testing.T, set func(*igd)) *igd {
	g := &igd{mapped: map[string]string{}}
	if set != nil {
		set(g)
	}
	g.srv = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	g.ssdp = newUDPGateway(t, func(req []byte) []byte {
		if !strings.HasPrefix(string(req), "M-SEARCH") {
			return nil
		}
		loc := g.location
		if loc == "" {
			loc = g.srv.URL + "/desc.xml"
			if g.locPath != "" {
				loc = g.srv.URL + g.locPath
			}
		}
		return []byte("HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\nLOCATION: " + loc + "\r\n\r\n")
	})
	return g
}

const goodDesc = `<?xml version="1.0"?><root xmlns="urn:schemas-upnp-org:device-1-0"><device>
<deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
<deviceList><device><deviceList><device><serviceList><service>
<serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType><controlURL>/ctl</controlURL>
</service></serviceList></device></deviceList></device></deviceList></device></root>`

func (g *igd) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/desc.xml":
		if g.desc != "" {
			io.WriteString(w, g.desc)
			return
		}
		io.WriteString(w, goodDesc)
	case "/ctl":
		body, _ := io.ReadAll(r.Body)
		action := r.Header.Get("SOAPAction")
		g.mu.Lock()
		g.actions = append(g.actions, action)
		g.mu.Unlock()
		switch {
		case strings.HasSuffix(action, `#AddPortMapping"`):
			if g.only725 && !strings.Contains(string(body), "<NewLeaseDuration>0<") {
				w.WriteHeader(500)
				io.WriteString(w, `<s:Envelope><s:Body><s:Fault><detail><UPnPError><errorCode>725</errorCode><errorDescription>OnlyPermanentLeasesSupported</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
				return
			}
			g.mu.Lock()
			g.mapped["add"] = string(body)
			g.mu.Unlock()
			io.WriteString(w, `<s:Envelope><s:Body><u:AddPortMappingResponse/></s:Body></s:Envelope>`)
		case strings.HasSuffix(action, `#GetExternalIPAddress"`):
			io.WriteString(w, `<s:Envelope><s:Body><u:GetExternalIPAddressResponse><NewExternalIPAddress>203.0.113.9</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`)
		case strings.HasSuffix(action, `#DeletePortMapping"`):
			g.mu.Lock()
			g.mapped["delete"] = string(body)
			g.mu.Unlock()
			io.WriteString(w, `<s:Envelope><s:Body><u:DeletePortMappingResponse/></s:Body></s:Envelope>`)
		}
	case "/big":
		w.Write([]byte(strings.Repeat("x", portmap.MaxReply+10)))
	case "/redirect":
		http.Redirect(w, r, "http://203.0.113.1/desc.xml", http.StatusFound)
	}
}

func (g *igd) client() *portmap.Client {
	return client(1, fmt.Sprintf("127.0.0.1:%d", g.ssdp.port()))
}

func TestUPnPMapUnmap(t *testing.T) {
	g := newIGD(t, nil)
	c := g.client()
	m, err := c.Map(context.Background(), portmap.MethodUPnP, 8646, time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != portmap.MethodUPnP || m.External.String() != "203.0.113.9:8646" || m.Lifetime != time.Hour {
		t.Fatalf("mapping %+v", m)
	}
	add := g.mapped["add"]
	for _, want := range []string{"<NewInternalClient>127.0.0.1<", "<NewLeaseDuration>3600<", "<NewProtocol>TCP<", "<NewInternalPort>8646<"} {
		if !strings.Contains(add, want) {
			t.Fatalf("AddPortMapping lacks %s: %s", want, add)
		}
	}
	if _, err := c.Map(context.Background(), portmap.MethodAuto, 8646, time.Hour, &m); err != nil {
		t.Fatal("renew:", err)
	}
	if err := c.Unmap(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.mapped["delete"], "<NewExternalPort>8646<") {
		t.Fatalf("delete %s", g.mapped["delete"])
	}
}

func TestUPnPPermanentOnly(t *testing.T) {
	g := newIGD(t, func(g *igd) { g.only725 = true })
	m, err := g.client().Map(context.Background(), portmap.MethodUPnP, 8646, time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Lifetime != 0 {
		t.Fatalf("lifetime %s, want permanent", m.Lifetime)
	}
}

func TestUPnPRefusesOffGateway(t *testing.T) {
	for name, set := range map[string]func(*igd){
		"location elsewhere": func(g *igd) { g.location = "http://203.0.113.1:80/desc.xml" },
		"location by name":   func(g *igd) { g.location = "http://localhost:1/desc.xml" },
		"https location":     func(g *igd) { g.location = "https://127.0.0.1:1/desc.xml" },
		"control elsewhere": func(g *igd) {
			g.desc = strings.Replace(goodDesc, "<controlURL>/ctl</controlURL>", "<controlURL>http://203.0.113.1/ctl</controlURL>", 1)
		},
		"base elsewhere": func(g *igd) {
			g.desc = strings.Replace(goodDesc, "<device>", "<URLBase>http://203.0.113.1/</URLBase><device>", 1)
		},
		"odd service": func(g *igd) {
			g.desc = strings.Replace(goodDesc, "WANIPConnection:1", `WANIPConnection:1" x="`, 1)
		},
		"no service":   func(g *igd) { g.desc = `<root><device></device></root>` },
		"not xml":      func(g *igd) { g.desc = `<<<` },
		"oversized":    func(g *igd) { g.locPath = "/big" },
		"redirected":   func(g *igd) { g.locPath = "/redirect" },
		"control path": func(g *igd) { g.desc = strings.Replace(goodDesc, "/ctl", "//203.0.113.1/ctl", 1) },
	} {
		t.Run(name, func(t *testing.T) {
			g := newIGD(t, set)
			if _, err := g.client().Map(context.Background(), portmap.MethodUPnP, 8646, time.Hour, nil); err == nil {
				t.Fatal("mapped")
			}
			g.mu.Lock()
			defer g.mu.Unlock()
			if len(g.actions) != 0 {
				t.Fatalf("called %v", g.actions)
			}
		})
	}
}

func TestParseRoutes(t *testing.T) {
	table := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t0\t00000000\n"
	a, err := portmap.ParseRoutes(strings.NewReader(table))
	if err != nil {
		t.Fatal(err)
	}
	// The table holds the address in host order; the test machine is
	// little-endian, as every builder of this repository is.
	if a.String() != "192.168.1.1" && a.String() != "1.1.168.192" {
		t.Fatalf("gateway %s", a)
	}
	if _, err := portmap.ParseRoutes(strings.NewReader(table[:60])); err == nil {
		t.Fatal("found a gateway in a table without a default route")
	}
}

func TestValidMethod(t *testing.T) {
	for _, m := range []string{"", "off", "auto", "pcp", "natpmp", "upnp"} {
		if !portmap.ValidMethod(m) {
			t.Fatal(m)
		}
	}
	if portmap.ValidMethod("socks") {
		t.Fatal("socks")
	}
}
