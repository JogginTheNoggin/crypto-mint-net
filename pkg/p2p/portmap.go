package p2p

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/portmap"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// DefaultPortMapLease is the mapping lifetime asked for when the policy
// sets none.
const DefaultPortMapLease = time.Hour

// PortMapper makes and removes port mappings (portmap.Client is one).
type PortMapper interface {
	Map(ctx context.Context, method string, internal uint16, lifetime time.Duration, prev *portmap.Mapping) (portmap.Mapping, error)
	Unmap(ctx context.Context, m portmap.Mapping) error
}

// Port mapping states, as PortMapStatus.State shows them.
const (
	PortMapOff         = "off"         // not asked for
	PortMapSkipped     = "skipped"     // asked for, but p2p.advertise is set or nothing listens
	PortMapTrying      = "trying"      // asking the router
	PortMapAdvertised  = "advertised"  // mapped, confirmed and advertised
	PortMapUnconfirmed = "unconfirmed" // mapped, but a dial to it did not reach this node; removed
	PortMapNotPublic   = "not_public"  // the router's external address is not public; removed
	PortMapFailed      = "failed"      // the router gave no mapping
)

// PortMapStatus is what poo status shows of the port mapping (D182).
type PortMapStatus struct {
	// Mode is the p2p.port_map setting ("off" when not set).
	Mode  string `json:"mode"`
	State string `json:"state"`
	// Method is how the mapping was made: pcp, natpmp or upnp.
	Method string `json:"method,omitempty"`
	// External is the address the router reported; Advertised is set only
	// once a dial to it reached this node.
	External   string `json:"external,omitempty"`
	Advertised string `json:"advertised,omitempty"`
	// LeaseSeconds is the lifetime the router granted (0 with a mapping:
	// until removed).
	LeaseSeconds int       `json:"lease_seconds,omitempty"`
	Since        time.Time `json:"since,omitzero"`
	Error        string    `json:"error,omitempty"`
	// Gateway is the router asked: p2p.port_map_gateway, or else the
	// default route's gateway as last found; empty when none was found
	// (GREEN-056).
	Gateway string `json:"gateway,omitempty"`
	// Reason says why a skipped mapping is not asked for (GREEN-056).
	Reason string `json:"reason,omitempty"`
	// NextTry is when the node asks the router again after a mapping
	// failed or was removed; zero while one is held (GREEN-056).
	NextTry time.Time `json:"next_try,omitzero"`
}

// Why a mapping that was asked for is skipped (PortMapStatus.Reason).
const (
	PortMapReasonAdvertise = "p2p.advertise is set by hand, which always wins"
	PortMapReasonNoListen  = "the node accepts no peers (p2p.listen is off)"
)

// PortMap returns the port mapping's status.
func (h *Host) PortMap() PortMapStatus {
	h.pmMu.Lock()
	defer h.pmMu.Unlock()
	st := h.pm
	if st.Mode == "" {
		st.Mode, st.State = PortMapOff, PortMapOff
		if h.portMapAsked() {
			st.Mode, st.State = h.cfg.PortMap, PortMapSkipped
			st.Reason = PortMapReasonNoListen
			if h.cfg.Advertise != "" {
				st.Reason = PortMapReasonAdvertise
			}
		}
	}
	st.Gateway = h.pmGW
	return st
}

func (h *Host) setPortMap(f func(*PortMapStatus)) {
	h.pmMu.Lock()
	defer h.pmMu.Unlock()
	f(&h.pm)
	h.pm.Mode = h.cfg.PortMap
}

func (h *Host) portMapAsked() bool { return h.cfg.PortMap != "" && h.cfg.PortMap != "off" }

// portMapOn says whether Run maps a port: asked for, listening, and no
// address given by hand (which always wins).
func (h *Host) portMapOn() bool {
	return h.portMapAsked() && h.cfg.Advertise == "" && h.Addr() != ""
}

// advertised is the address this node offers peers: p2p.advertise, or
// else a confirmed port mapping's.
func (h *Host) advertised() string {
	if h.cfg.Advertise != "" {
		return h.cfg.Advertise
	}
	if p := h.mapped.Load(); p != nil {
		return *p
	}
	return ""
}

// gatewayShown is the router a try speaks to, as status shows it:
// p2p.port_map_gateway, or else the default route's gateway (found again
// on each try, as portmap.Client finds it). Empty when none is found, or
// when a test supplies its own mapper.
func (h *Host) gatewayShown() string {
	if g := h.cfg.PortMapGateway; g != "" {
		return g
	}
	if h.cfg.PortMapper != nil {
		return ""
	}
	if a, err := portmap.DefaultGateway(); err == nil {
		return a.String()
	}
	return ""
}

func (h *Host) mapper() (PortMapper, error) {
	if h.cfg.PortMapper != nil {
		return h.cfg.PortMapper, nil
	}
	c := &portmap.Client{}
	if g := h.cfg.PortMapGateway; g != "" {
		a, err := netip.ParseAddr(g)
		if err != nil {
			return nil, fmt.Errorf("p2p.port_map_gateway %q: %v", g, err)
		}
		c.Gateway = a
	}
	return c, nil
}

// Port mapping waits: after a failure the next try waits pmRetry, doubled
// each time up to pmMaxRetry; a renewal comes at half the granted
// lifetime or the asked lease, whichever is shorter, never sooner than
// pmMinRenew, and a lifetime no longer than pmMinLife is refused
// (renewAfter, RED-092).
var (
	pmRetry    = time.Minute
	pmMaxRetry = 30 * time.Minute
	pmMinRenew = 30 * time.Second
	// pmMinLife is the shortest lifetime kept: a shorter one would lapse
	// before the renewal (RED-092).
	pmMinLife = 30 * time.Second
	pmUnmap   = 3 * time.Second
)

// portMapLoop keeps a mapping of the listen port while ctx lasts, and
// removes it on a clean stop (D182).
func (h *Host) portMapLoop(ctx context.Context) {
	defer h.wg.Done()
	_, ps, _ := net.SplitHostPort(h.Addr())
	port, _ := strconv.Atoi(ps)
	m, err := h.mapper()
	if err != nil {
		h.setPortMap(func(s *PortMapStatus) { s.State, s.Error = PortMapFailed, err.Error() })
		h.log.Warn("port mapping off", "err", err)
		return
	}
	retry := pmRetry
	sleep := func(d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
	}
	backoff := func() bool {
		h.setPortMap(func(s *PortMapStatus) { s.NextTry = time.Now().Add(retry) })
		ok := sleep(retry)
		retry = min(retry*2, pmMaxRetry)
		return ok
	}
	for ctx.Err() == nil {
		gw := h.gatewayShown()
		h.setPortMap(func(s *PortMapStatus) { s.State, s.NextTry = PortMapTrying, time.Time{} })
		h.pmMu.Lock()
		h.pmGW = gw
		h.pmMu.Unlock()
		got, err := m.Map(ctx, h.cfg.PortMap, uint16(port), h.cfg.PortMapLease, nil)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			h.setPortMap(func(s *PortMapStatus) {
				*s = PortMapStatus{State: PortMapFailed, Error: err.Error(), Since: time.Now()}
			})
			h.log.Info("port mapping failed", "err", err)
			if !backoff() {
				return
			}
			continue
		}
		cont, held := h.holdMapping(ctx, m, got)
		if !cont {
			return
		}
		if held {
			// A mapping that worked, then lapsed, is asked for again
			// soon, not after the longest wait earlier failures built up.
			retry = pmRetry
		}
		if !backoff() {
			return
		}
	}
}

// holdMapping confirms m, advertises it and renews it until it fails or
// ctx ends; it removes the mapping before returning. cont is false when
// ctx ended; held says the mapping was confirmed and advertised.
func (h *Host) holdMapping(ctx context.Context, mp PortMapper, m portmap.Mapping) (cont, held bool) {
	defer func() {
		h.mapped.Store(nil)
		uctx, cancel := context.WithTimeout(context.Background(), pmUnmap)
		defer cancel()
		if err := mp.Unmap(uctx, m); err != nil {
			h.log.Info("port mapping not removed", "err", err)
		} else {
			h.log.Info("port mapping removed", "external", m.External.String())
		}
	}()
	confirmed := netip.AddrPort{}
	for {
		ext := m.External.String()
		show := func(state, errText string) {
			h.setPortMap(func(s *PortMapStatus) {
				*s = PortMapStatus{State: state, Method: m.Method, External: ext, LeaseSeconds: int(m.Lifetime / time.Second),
					Since: time.Now(), Error: errText}
				if state == PortMapAdvertised {
					s.Advertised = ext
				}
			})
		}
		if m.Lifetime > 0 && m.Lifetime <= pmMinLife {
			// The mapping would lapse before the node renews it, so it
			// would be missing most of the time while advertised
			// (RED-092).
			h.mapped.Store(nil)
			show(PortMapFailed, fmt.Sprintf("the router granted a lifetime of %s, too short to keep the mapping", m.Lifetime))
			h.log.Info("port mapping lifetime too short", "lease", m.Lifetime)
			return ctx.Err() == nil, held
		}
		if m.External != confirmed {
			h.mapped.Store(nil)
			if !h.cfg.AllowLocalPeers && !publicIP(m.External.Addr()) {
				show(PortMapNotPublic, "the router's external address is not public: it is behind another NAT (such as carrier-grade NAT), so no mapping can reach this node")
				h.log.Info("port mapping gives no public address", "external", ext)
				return ctx.Err() == nil, held
			}
			if err := h.confirmSelf(ctx, ext); err != nil {
				if ctx.Err() != nil {
					return false, held
				}
				show(PortMapUnconfirmed, "a dial to the mapped address did not reach this node ("+err.Error()+"); if the router cannot loop back, set p2p.advertise by hand")
				h.log.Info("port mapping unconfirmed", "external", ext, "err", err)
				return true, held
			}
			confirmed, held = m.External, true
			h.mapped.Store(&ext)
			h.mu.Lock()
			h.selfAddrs[ext] = true
			h.mu.Unlock()
			h.log.Info("port mapped and advertised", "method", m.Method, "external", ext, "lease", m.Lifetime)
			show(PortMapAdvertised, "")
		}
		select {
		case <-ctx.Done():
			return false, held
		case <-time.After(h.renewAfter(m.Lifetime)):
		}
		next, err := mp.Map(ctx, m.Method, m.Internal, h.cfg.PortMapLease, &m)
		if err != nil {
			if ctx.Err() != nil {
				return false, held
			}
			show(PortMapFailed, "renewal: "+err.Error())
			h.log.Info("port mapping renewal failed", "err", err)
			return true, held
		}
		m = next
	}
}

// renewAfter is the wait before a mapping of the granted lifetime is
// renewed: half of the lifetime or of p2p.port_map_lease, whichever is
// shorter, so a router that states a longer lifetime than was asked for
// does not put off the next look at its external address (RED-092); and
// never sooner than pmMinRenew. A lifetime of 0 (until removed) counts as
// the lease.
func (h *Host) renewAfter(life time.Duration) time.Duration {
	lease := h.cfg.PortMapLease
	if lease <= 0 {
		lease = DefaultPortMapLease
	}
	if life <= 0 || life > lease {
		life = lease
	}
	return max(life/2, pmMinRenew)
}

// pmConfirmWait bounds the wait for this node's listener to see a
// dial-back.
var pmConfirmWait = helloTimeout

// selfProbeAgent starts the Agent of a dial-back hello; a random token
// follows it (RED-090).
const selfProbeAgent = brand.Name + "-self-check/"

// confirmSelf dials addr and reports nil once this node's own listener
// has received the dial: the mapped address leads here. The hello it
// sends carries a fresh random token, and only the listener's record of
// that token counts (RED-090); what the far end answers is not looked
// at, since a host that echoes the hello back, or copies it, would pass
// any check of the answer. Only the router's report and this dial decide
// what is advertised, never what peers say. The dial, the write and the
// wait all end when ctx does (RED-092).
func (h *Host) confirmSelf(parent context.Context, addr string) error {
	var tok [16]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return err
	}
	key := hex.EncodeToString(tok[:])
	seen := make(chan struct{})
	h.probeMu.Lock()
	h.probes[key] = seen
	h.probeMu.Unlock()
	defer func() {
		h.probeMu.Lock()
		delete(h.probes, key)
		h.probeMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(parent, pmConfirmWait)
	defer cancel()
	d := net.Dialer{}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	c.SetDeadline(time.Now().Add(pmConfirmWait))
	mine := wire.Hello{Protocol: Protocol, NodeID: h.id, Agent: selfProbeAgent + key}
	if err := wire.WriteFrame(c, wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: mine.Encode()}); err != nil {
		select {
		case <-seen:
			return nil
		default:
		}
		if parent.Err() != nil {
			return parent.Err()
		}
		return err
	}
	// The answer can only end the wait early with a failure: a hello
	// from another node, or a connection that closed (RED-090).
	answer := make(chan error, 1)
	go func() {
		m, _, err := wire.ReadFrame(bufio.NewReader(c), wire.MaxHelloFrame)
		if err == nil {
			if theirs, derr := wire.DecodeHello(m.Body); m.Type == wire.TypeHello && derr == nil && theirs.NodeID != h.id {
				err = errors.New("another node answered")
			} else {
				_, err = io.Copy(io.Discard, io.LimitReader(c, wire.MaxFrame))
				if err == nil {
					err = errors.New("the connection closed")
				}
			}
		}
		answer <- err
	}()
	fail := func(err error) error {
		select {
		case <-seen:
			return nil
		default:
		}
		if parent.Err() != nil {
			return parent.Err()
		}
		return err
	}
	select {
	case <-seen:
		return nil
	case err := <-answer:
		return fail(err)
	case <-ctx.Done():
		return fail(errors.New("this node's listener never received the dial"))
	}
}

// sawSelfProbe records a hello this node's listener received with its own
// node id: when its Agent carries a token confirmSelf is waiting on, the
// dial-back reached this node (RED-090).
func (h *Host) sawSelfProbe(agent string) {
	key, ok := strings.CutPrefix(agent, selfProbeAgent)
	if !ok {
		return
	}
	h.probeMu.Lock()
	defer h.probeMu.Unlock()
	if ch, ok := h.probes[key]; ok {
		close(ch)
		delete(h.probes, key)
	}
}
