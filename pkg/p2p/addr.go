package p2p

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// errNotPublic refuses to dial an address learned from another party that
// leads into a local or reserved network (decision D96).
var errNotPublic = errors.New("p2p: address is not public")

// notPublic are the ranges, beside loopback, private, link-local,
// multicast and unspecified addresses, that are not reachable on the
// public internet: shared address space, IETF protocol assignments,
// documentation and benchmarking ranges, the reserved 240/4 and
// broadcast, the IPv6 documentation and discard prefixes, and the IPv6
// ranges that carry an IPv4 address inside them: NAT64 (well-known and
// local-use), 6to4, Teredo, IPv4-compatible and IPv4-translated. A host
// may route one of those to the IPv4 address it carries, which can be
// loopback or private, so they are refused whole (RED-007, decision
// D102); a peer with one also has the IPv4 address itself to give.
var notPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::ffff:0:0:0/96"),
}

// publicIP reports whether ip is a unicast address on the public
// internet. An IPv4 address written in IPv6 form is judged as IPv4.
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, p := range notPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// learnable reports whether an address another node or a registry gave
// may go into the book. A literal IP must be public unless the policy
// allows local peers; a host name is checked when it is dialled, since
// it may resolve anywhere.
func (h *Host) learnable(addr string) bool {
	if h.cfg.AllowLocalPeers {
		return true
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return true // a host name
	}
	return ip.Zone() == "" && publicIP(ip)
}

// learn adds an address learned from another party to the book, unless
// it leads into a local network (decision D96).
func (h *Host) learn(addr string, sections []wire.Section, src peers.Source, now time.Time) error {
	if !h.learnable(addr) {
		return errNotPublic
	}
	return h.book.Add(addr, sections, src, now)
}

// candidateDialer returns the dialer for a book candidate. Unless the
// policy allows local peers or the owner listed addr, it refuses, after
// name resolution and before connecting, an address that is not public,
// so neither a planted literal nor a host name resolving to a local
// address makes the node connect into its own network.
func (h *Host) candidateDialer(addr string) *net.Dialer {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	if h.cfg.AllowLocalPeers || h.configured[addr] {
		return d
	}
	d.ControlContext = func(_ context.Context, _, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil || !publicIP(ap.Addr()) {
			return errNotPublic
		}
		return nil
	}
	return d
}
