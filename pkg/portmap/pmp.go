package portmap

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

const (
	pmpPort      = 5351
	pmpVersion   = 0
	pcpVersion   = 2
	pcpOpMap     = 1
	protoTCP     = 6
	pcpReqLen    = 24 + 36
	pmpMapReqLen = 12
)

// errNotSpoken says the gateway does not speak the protocol asked.
var errNotSpoken = errors.New("not spoken by the gateway")

func (c *Client) port() uint16 {
	if c.pmpPort != 0 {
		return c.pmpPort
	}
	return pmpPort
}

// exchange sends req to the gateway and returns the first reply accept
// takes, resending at 250 ms, 500 ms, 1 s and so on until the timeout.
// The socket is connected, so only the gateway's own port can answer.
func (c *Client) exchange(ctx context.Context, gw netip.Addr, req []byte, accept func([]byte, netip.Addr) (bool, error)) error {
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(gw, c.port())))
	if err != nil {
		return err
	}
	defer conn.Close()
	local := conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap()
	if len(req) == pcpReqLen && req[0] == pcpVersion {
		// PCP names the client's own address, IPv4-mapped.
		ip := local.As16()
		copy(req[8:24], ip[:])
	}
	deadline := time.Now().Add(c.timeout())
	cut := false // the caller's deadline comes first
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline, cut = d, true
	}
	stop := context.AfterFunc(ctx, func() { conn.SetReadDeadline(time.Now()) })
	defer stop()
	buf := make([]byte, 1100) // PCP replies are at most 1100 bytes
	wait := 250 * time.Millisecond
	for time.Now().Before(deadline) {
		if _, err := conn.Write(req); err != nil {
			return err
		}
		next := time.Now().Add(wait)
		if next.After(deadline) {
			next = deadline
		}
		conn.SetReadDeadline(next)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					break
				}
				// A refused port (ICMP unreachable) means nothing listens.
				return fmt.Errorf("%w (%v)", errNotSpoken, err)
			}
			done, err := accept(buf[:n], local)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
		wait *= 2
	}
	if cut {
		// The caller's context ended the wait, though its timer may not
		// have fired yet: the request went out and its answer was not
		// waited for (RED-091).
		return context.DeadlineExceeded
	}
	return errors.New("no answer from the gateway")
}

func lifetimeSecs(d time.Duration) uint32 {
	s := d / time.Second
	if s < 0 {
		return 0
	}
	if s > 1<<31 {
		return 1 << 31
	}
	return uint32(s)
}

// pcpRequest builds a PCP MAP request for TCP.
func pcpRequest(nonce [12]byte, internal, suggest uint16, lifetime uint32) []byte {
	b := make([]byte, pcpReqLen)
	b[0], b[1] = pcpVersion, pcpOpMap
	binary.BigEndian.PutUint32(b[4:], lifetime)
	m := b[24:]
	copy(m[0:12], nonce[:])
	m[12] = protoTCP
	binary.BigEndian.PutUint16(m[16:], internal)
	binary.BigEndian.PutUint16(m[18:], suggest)
	// Suggested external address: any IPv4 (::ffff:0.0.0.0).
	m[30], m[31] = 0xff, 0xff
	return b
}

var pcpResults = map[byte]string{
	1: "unsupported version", 2: "not authorized", 3: "malformed request", 4: "unsupported opcode",
	5: "unsupported option", 6: "malformed option", 7: "network failure", 8: "no resources",
	9: "unsupported protocol", 10: "user exceeded quota", 11: "cannot provide external", 12: "address mismatch",
	13: "excessive remote peers",
}

// pcpReply reads a PCP MAP reply to nonce; ok is false for a datagram that
// is not one (a stray or late reply), which is skipped.
func pcpReply(b []byte, nonce [12]byte) (m Mapping, ok bool, err error) {
	if len(b) >= 4 && b[0] == pmpVersion && b[1] >= 128 {
		// A NAT-PMP gateway answers a PCP request with its own
		// "unsupported version".
		return Mapping{}, true, errNotSpoken
	}
	if len(b) < pcpReqLen || b[0] != pcpVersion || b[1] != 0x80|pcpOpMap {
		return Mapping{}, false, nil
	}
	mp := b[24:]
	if [12]byte(mp[0:12]) != nonce {
		return Mapping{}, false, nil
	}
	if r := b[3]; r != 0 {
		if r == 1 {
			return Mapping{}, true, errNotSpoken
		}
		return Mapping{}, true, fmt.Errorf("refused: %s (%d)", pcpResults[r], r)
	}
	if mp[12] != protoTCP {
		return Mapping{}, false, nil
	}
	ext := netip.AddrFrom16([16]byte(mp[20:36])).Unmap()
	m = Mapping{
		Method:   MethodPCP,
		External: netip.AddrPortFrom(ext, binary.BigEndian.Uint16(mp[18:])),
		Internal: binary.BigEndian.Uint16(mp[16:]),
		Lifetime: time.Duration(binary.BigEndian.Uint32(b[4:])) * time.Second,
		nonce:    nonce,
	}
	if m.Lifetime == 0 {
		// In PCP a lifetime of 0 means the mapping was removed, not that
		// it lasts until removed (RED-092).
		return Mapping{}, true, errors.New("granted a lifetime of 0: no mapping")
	}
	if !ext.Is4() || m.External.Port() == 0 {
		// The router made a mapping the node cannot use; the caller
		// removes it (RED-091).
		return m, true, &unusableError{m}
	}
	return m, true, nil
}

// unusableError is a mapping the router made that the node cannot use;
// it must be removed.
type unusableError struct{ m Mapping }

func (e *unusableError) Error() string {
	return fmt.Sprintf("granted no usable address (%s)", e.m.External)
}

// cleanup makes a short best-effort removal of a mapping a failed request
// may have left at the router: one granted but unusable, or one asked for
// when the caller's context ended before the answer came (RED-091). It
// uses its own context, as a clean stop does, and its error is ignored.
func (c *Client) cleanup(remove func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), min(c.timeout(), cleanupWait))
	defer cancel()
	remove(ctx)
}

// cutShort says a request's wait was ended by the caller's context.
func cutShort(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// cleanupWait bounds a cleanup's wait for the router.
var cleanupWait = time.Second

func (c *Client) pcpMap(ctx context.Context, gw netip.Addr, internal uint16, lifetime time.Duration, prev *Mapping) (Mapping, error) {
	var nonce [12]byte
	suggest := internal
	if prev != nil {
		nonce, suggest = prev.nonce, prev.External.Port()
	} else {
		rand.Read(nonce[:])
	}
	if lifetime <= 0 {
		return Mapping{}, errors.New("pcp: a mapping needs a lifetime")
	}
	var got Mapping
	err := c.exchange(ctx, gw, pcpRequest(nonce, internal, suggest, lifetimeSecs(lifetime)), func(b []byte, _ netip.Addr) (bool, error) {
		m, ok, err := pcpReply(b, nonce)
		if ok && err == nil {
			got = m
		}
		return ok, err
	})
	var ue *unusableError
	if prev == nil && (errors.As(err, &ue) || cutShort(ctx, err)) {
		// A renewal's mapping is the caller's to remove; a new one the
		// node will not hold is removed here, with the same nonce.
		c.cleanup(func(cctx context.Context) error {
			return c.pcpUnmap(cctx, gw, Mapping{Method: MethodPCP, Internal: internal, nonce: nonce})
		})
	}
	if err == nil && ctx.Err() != nil {
		return Mapping{}, ctx.Err()
	}
	return got, err
}

func (c *Client) pcpUnmap(ctx context.Context, gw netip.Addr, m Mapping) error {
	return c.exchange(ctx, gw, pcpRequest(m.nonce, m.Internal, 0, 0), func(b []byte, _ netip.Addr) (bool, error) {
		if len(b) < pcpReqLen || b[0] != pcpVersion || [12]byte(b[24:36]) != m.nonce {
			return false, nil
		}
		if r := b[3]; r != 0 {
			return true, fmt.Errorf("refused: %s (%d)", pcpResults[r], r)
		}
		return true, nil
	})
}

var pmpResults = map[uint16]string{1: "unsupported version", 2: "refused", 3: "network failure", 4: "out of resources", 5: "unsupported opcode"}

// pmpExternal asks a NAT-PMP gateway for its external address.
func (c *Client) pmpExternal(ctx context.Context, gw netip.Addr) (netip.Addr, error) {
	var ext netip.Addr
	err := c.exchange(ctx, gw, []byte{pmpVersion, 0}, func(b []byte, _ netip.Addr) (bool, error) {
		if len(b) < 12 || b[0] != pmpVersion || b[1] != 128 {
			return false, nil
		}
		if r := binary.BigEndian.Uint16(b[2:]); r != 0 {
			if r == 1 {
				return true, errNotSpoken
			}
			return true, fmt.Errorf("refused: %s (%d)", pmpResults[r], r)
		}
		ext = netip.AddrFrom4([4]byte(b[8:12]))
		return true, nil
	})
	return ext, err
}

func pmpRequest(internal, suggest uint16, lifetime uint32) []byte {
	b := make([]byte, pmpMapReqLen)
	b[0], b[1] = pmpVersion, 2 // map TCP
	binary.BigEndian.PutUint16(b[4:], internal)
	binary.BigEndian.PutUint16(b[6:], suggest)
	binary.BigEndian.PutUint32(b[8:], lifetime)
	return b
}

// pmpMapReply reads a NAT-PMP TCP mapping reply for internal.
func pmpMapReply(b []byte, internal uint16) (port uint16, life uint32, ok bool, err error) {
	if len(b) < 16 || b[0] != pmpVersion || b[1] != 130 || binary.BigEndian.Uint16(b[8:]) != internal {
		return 0, 0, false, nil
	}
	if r := binary.BigEndian.Uint16(b[2:]); r != 0 {
		return 0, 0, true, fmt.Errorf("refused: %s (%d)", pmpResults[r], r)
	}
	return binary.BigEndian.Uint16(b[10:]), binary.BigEndian.Uint32(b[12:]), true, nil
}

func (c *Client) pmpMap(ctx context.Context, gw netip.Addr, internal uint16, lifetime time.Duration, prev *Mapping) (Mapping, error) {
	if lifetime <= 0 {
		return Mapping{}, errors.New("natpmp: a mapping needs a lifetime")
	}
	ext, err := c.pmpExternal(ctx, gw)
	if err != nil {
		return Mapping{}, err
	}
	suggest := internal
	if prev != nil {
		suggest = prev.External.Port()
	}
	var m Mapping
	err = c.exchange(ctx, gw, pmpRequest(internal, suggest, lifetimeSecs(lifetime)), func(b []byte, _ netip.Addr) (bool, error) {
		port, life, ok, err := pmpMapReply(b, internal)
		if !ok || err != nil {
			return ok, err
		}
		if port == 0 || life == 0 {
			return true, errors.New("granted no mapping")
		}
		m = Mapping{Method: MethodNATPMP, External: netip.AddrPortFrom(ext, port), Internal: internal,
			Lifetime: time.Duration(life) * time.Second}
		return true, nil
	})
	if prev == nil && cutShort(ctx, err) {
		// The request went out and the answer was not waited for: the
		// router may hold a mapping nobody knows of (RED-091).
		c.cleanup(func(cctx context.Context) error { return c.pmpUnmap(cctx, gw, Mapping{Internal: internal}) })
		if ctx.Err() != nil {
			return Mapping{}, ctx.Err()
		}
		return Mapping{}, err
	}
	return m, err
}

func (c *Client) pmpUnmap(ctx context.Context, gw netip.Addr, m Mapping) error {
	return c.exchange(ctx, gw, pmpRequest(m.Internal, 0, 0), func(b []byte, _ netip.Addr) (bool, error) {
		_, _, ok, err := pmpMapReply(b, m.Internal)
		return ok, err
	})
}
