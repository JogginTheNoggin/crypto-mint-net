// Package portmap asks the router a node sits behind to forward an
// inbound TCP port to it, for an owner who turned on p2p.port_map
// (decision D182). It speaks PCP (RFC 6887) first, NAT-PMP (RFC 6886)
// second and UPnP IGD third.
//
// The package is wary of the network it talks to. It speaks only to the
// gateway: PCP and NAT-PMP replies come over a socket connected to the
// gateway, and a UPnP description or control URL must name the gateway's
// own IP address. Every wait is short and every reply is capped in size;
// HTTP takes no proxy and follows no redirect.
//
// A mapping says what the router reports as its external address. It is a
// claim only: the caller confirms it reaches this node (package p2p dials
// it and waits for its own listener to receive the dial, D183) before
// telling anyone.
//
// A request that fails leaves no mapping behind: what the router made on
// the way is removed before the error is returned (D183).
package portmap

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// Methods, in the order MethodAuto tries them.
const (
	MethodAuto   = "auto"
	MethodPCP    = "pcp"
	MethodNATPMP = "natpmp"
	MethodUPnP   = "upnp"
)

// Methods lists the methods a setting may name, besides "off".
var Methods = []string{MethodAuto, MethodPCP, MethodNATPMP, MethodUPnP}

// ValidMethod reports whether m names a method (or "", "off" for none).
func ValidMethod(m string) bool {
	if m == "" || m == "off" {
		return true
	}
	for _, x := range Methods {
		if m == x {
			return true
		}
	}
	return false
}

// MaxReply caps every reply read from the router: a UDP datagram, an SSDP
// answer, a device description or a SOAP reply.
const MaxReply = 64 << 10

// DefaultTimeout bounds the wait for one method's answer.
const DefaultTimeout = 3 * time.Second

// ErrNoGateway says no gateway was found or given.
var ErrNoGateway = errors.New("portmap: no gateway found; set p2p.port_map_gateway")

// Mapping is one port mapping the router granted.
type Mapping struct {
	// Method is how it was made: MethodPCP, MethodNATPMP or MethodUPnP.
	Method string
	// External is the router's external address and the port it forwards.
	External netip.AddrPort
	// Internal is this machine's port the router forwards to.
	Internal uint16
	// Lifetime is how long the router keeps the mapping without renewal;
	// zero means until it is removed (a UPnP router that grants only
	// permanent leases).
	Lifetime time.Duration

	nonce   [12]byte // PCP
	control string   // UPnP control URL
	service string   // UPnP service type
}

// Client makes mappings through one gateway.
type Client struct {
	// Gateway is the router's address. Zero means the default route's
	// gateway (DefaultGateway).
	Gateway netip.Addr
	// Timeout bounds each method's wait; zero means DefaultTimeout.
	Timeout time.Duration

	// For tests: the ports the gateway listens on.
	pmpPort  uint16 // 5351
	ssdpAddr string // 239.255.255.250:1900
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

func (c *Client) gateway() (netip.Addr, error) {
	if c.Gateway.IsValid() {
		return c.Gateway, nil
	}
	return DefaultGateway()
}

// Map asks for internal to be forwarded for lifetime, by method (MethodAuto
// tries each in turn). prev, when set, is the mapping being renewed: the
// same method is used and the same external port asked for.
func (c *Client) Map(ctx context.Context, method string, internal uint16, lifetime time.Duration, prev *Mapping) (Mapping, error) {
	gw, err := c.gateway()
	if err != nil {
		return Mapping{}, err
	}
	if prev != nil {
		method = prev.Method
	}
	var try []string
	switch method {
	case MethodAuto, "":
		try = []string{MethodPCP, MethodNATPMP, MethodUPnP}
	case MethodPCP, MethodNATPMP, MethodUPnP:
		try = []string{method}
	default:
		return Mapping{}, fmt.Errorf("portmap: unknown method %q", method)
	}
	var errs []string
	for _, m := range try {
		var got Mapping
		var err error
		switch m {
		case MethodPCP:
			got, err = c.pcpMap(ctx, gw, internal, lifetime, prev)
		case MethodNATPMP:
			got, err = c.pmpMap(ctx, gw, internal, lifetime, prev)
		case MethodUPnP:
			got, err = c.upnpMap(ctx, gw, internal, lifetime, prev)
		}
		if err == nil {
			return got, nil
		}
		if ctx.Err() != nil {
			return Mapping{}, ctx.Err()
		}
		errs = append(errs, fmt.Sprintf("%s: %v", m, err))
	}
	return Mapping{}, errors.New("portmap: " + strings.Join(errs, "; "))
}

// Unmap asks the router to remove m.
func (c *Client) Unmap(ctx context.Context, m Mapping) error {
	gw, err := c.gateway()
	if err != nil {
		return err
	}
	switch m.Method {
	case MethodPCP:
		return c.pcpUnmap(ctx, gw, m)
	case MethodNATPMP:
		return c.pmpUnmap(ctx, gw, m)
	case MethodUPnP:
		return c.upnpUnmap(ctx, gw, m)
	}
	return fmt.Errorf("portmap: unknown method %q", m.Method)
}

// DefaultGateway returns the gateway of the IPv4 default route. It reads
// /proc/net/route, so it finds one on Linux only; elsewhere the owner sets
// p2p.port_map_gateway.
func DefaultGateway() (netip.Addr, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return netip.Addr{}, ErrNoGateway
	}
	defer f.Close()
	return parseRoutes(io.LimitReader(f, MaxReply))
}

// parseRoutes finds the default route's gateway in a /proc/net/route table.
func parseRoutes(r io.Reader) (netip.Addr, error) {
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < 4 || f[1] != "00000000" {
			continue
		}
		var flags uint64
		fmt.Sscanf(f[3], "%x", &flags)
		if flags&0x3 != 0x3 { // RTF_UP | RTF_GATEWAY
			continue
		}
		// The table prints the address as the kernel holds it: a number in
		// host order whose bytes in memory are the address.
		v, err := strconv.ParseUint(f[2], 16, 32)
		if err != nil {
			continue
		}
		var b [4]byte
		binary.NativeEndian.PutUint32(b[:], uint32(v))
		a := netip.AddrFrom4(b)
		if a.IsUnspecified() {
			continue
		}
		return a, nil
	}
	return netip.Addr{}, ErrNoGateway
}
