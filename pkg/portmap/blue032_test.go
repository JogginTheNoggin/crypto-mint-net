package portmap_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/portmap"
)

// BLUE-032 (RED-091, RED-092): a failed request leaves no mapping at the
// router, and a PCP lifetime of 0 is no mapping.

// fakeIGD is a UPnP gateway whose control answers come from ctl; it
// records the actions asked for.
type fakeIGD struct {
	ssdp    *udpGateway
	srv     *httptest.Server
	mu      sync.Mutex
	actions []string
}

func newFakeIGD(t *testing.T, ctl func(action string, body string, w http.ResponseWriter, r *http.Request)) *fakeIGD {
	g := &fakeIGD{}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/desc.xml":
			io.WriteString(w, goodDesc)
		case "/ctl":
			body, _ := io.ReadAll(r.Body)
			a := r.Header.Get("SOAPAction")
			a = a[strings.Index(a, "#")+1 : len(a)-1]
			g.mu.Lock()
			g.actions = append(g.actions, a)
			g.mu.Unlock()
			ctl(a, string(body), w, r)
		}
	}))
	t.Cleanup(g.srv.Close)
	g.ssdp = newUDPGateway(t, func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nLOCATION: " + g.srv.URL + "/desc.xml\r\n\r\n")
	})
	return g
}

func (g *fakeIGD) client() *portmap.Client {
	return client(1, fmt.Sprintf("127.0.0.1:%d", g.ssdp.port()))
}

func (g *fakeIGD) seen() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.actions...)
}

const fault725 = `<s:Envelope><s:Body><s:Fault><detail><UPnPError><errorCode>725</errorCode><errorDescription>OnlyPermanentLeasesSupported</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`

// Red's router: permanent leases only, and the external address fails.
// The address is asked for first, so no mapping is made at all.
func TestBlue032UPnPExternalFailsLeavesNoMapping(t *testing.T) {
	g := newFakeIGD(t, func(a, body string, w http.ResponseWriter, _ *http.Request) {
		switch a {
		case "AddPortMapping":
			if !strings.Contains(body, "<NewLeaseDuration>0<") {
				w.WriteHeader(500)
				io.WriteString(w, fault725)
			}
		case "GetExternalIPAddress":
			w.WriteHeader(500)
		}
	})
	if _, err := g.client().Map(context.Background(), portmap.MethodUPnP, 8646, time.Hour, nil); err == nil {
		t.Fatal("mapped")
	}
	for _, a := range g.seen() {
		if a == "AddPortMapping" {
			t.Fatalf("a mapping was made though the request failed: %v", g.seen())
		}
	}
}

// A reload or stop while AddPortMapping waits for its answer removes what
// the router may have made.
func TestBlue032UPnPCancelRemoves(t *testing.T) {
	g := newFakeIGD(t, func(a, _ string, w http.ResponseWriter, r *http.Request) {
		switch a {
		case "GetExternalIPAddress":
			io.WriteString(w, `<s:Envelope><s:Body><u:GetExternalIPAddressResponse><NewExternalIPAddress>203.0.113.9</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`)
		case "AddPortMapping":
			select { // made, but the answer is slow
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := g.client().Map(ctx, portmap.MethodUPnP, 8646, time.Hour, nil); err == nil {
		t.Fatal("mapped")
	}
	got := g.seen()
	if len(got) == 0 || got[len(got)-1] != "DeletePortMapping" {
		t.Fatalf("no removal after the cancel: %v", got)
	}
}

// pcpAnswer answers a MAP request as pcpServer does, then lets edit change
// the reply; a removal (lifetime 0) is answered plainly.
func pcpAnswer(edit func(out []byte)) func([]byte) []byte {
	base := pcpServer(41000, 0)
	return func(req []byte) []byte {
		out := base(req)
		if req[0] == 2 && binary.BigEndian.Uint32(req[4:]) != 0 {
			edit(out)
		}
		return out
	}
}

func removals(g *udpGateway, nonce []byte) int {
	n := 0
	for _, r := range g.requests() {
		if len(r) == 60 && r[0] == 2 && binary.BigEndian.Uint32(r[4:]) == 0 && string(r[24:36]) == string(nonce) {
			n++
		}
	}
	return n
}

// Red's PCP router: it makes the mapping and answers with an IPv6
// address. The node cannot use it, so it asks for it to be removed.
func TestBlue032PCPUnusableRemoved(t *testing.T) {
	g := newUDPGateway(t, pcpAnswer(func(out []byte) {
		copy(out[44:60], []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	}))
	_, err := client(g.port(), "127.0.0.1:9").Map(context.Background(), portmap.MethodPCP, 8646, time.Hour, nil)
	if err == nil || !strings.Contains(err.Error(), "no usable address") {
		t.Fatalf("err %v", err)
	}
	if removals(g, g.requests()[0][24:36]) == 0 {
		t.Fatalf("no removal sent: %d requests", len(g.requests()))
	}
}

// A PCP reply with lifetime 0 says the mapping is gone: refused.
func TestBlue032PCPLifetimeZeroRefused(t *testing.T) {
	g := newUDPGateway(t, pcpAnswer(func(out []byte) { binary.BigEndian.PutUint32(out[4:], 0) }))
	_, err := client(g.port(), "127.0.0.1:9").Map(context.Background(), portmap.MethodPCP, 8646, time.Hour, nil)
	if err == nil || !strings.Contains(err.Error(), "lifetime of 0") {
		t.Fatalf("err %v", err)
	}
}

// A cancel while a PCP or NAT-PMP request waits for its answer sends a
// short removal, and does not wait long for it.
func TestBlue032CancelSendsRemoval(t *testing.T) {
	for _, method := range []string{portmap.MethodPCP, portmap.MethodNATPMP} {
		t.Run(method, func(t *testing.T) {
			g := newUDPGateway(t, func(req []byte) []byte {
				if method == portmap.MethodNATPMP && req[0] == 0 && req[1] == 0 {
					return pmpServer(req) // the external address only
				}
				return nil
			})
			c := &portmap.Client{Gateway: lo, Timeout: time.Minute}
			c.SetPorts(g.port(), "127.0.0.1:9")
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			start := time.Now()
			if _, err := c.Map(ctx, method, 8646, time.Hour, nil); err == nil {
				t.Fatal("mapped")
			}
			if d := time.Since(start); d > 2500*time.Millisecond {
				t.Fatalf("waited %s", d)
			}
			n := 0
			for _, r := range g.requests() {
				switch {
				case method == portmap.MethodPCP && len(r) == 60 && binary.BigEndian.Uint32(r[4:]) == 0:
					n++
				case method == portmap.MethodNATPMP && len(r) == 12 && r[1] == 2 && binary.BigEndian.Uint32(r[8:]) == 0:
					n++
				}
			}
			if n == 0 {
				t.Fatal("no removal sent after the cancel")
			}
		})
	}
}
