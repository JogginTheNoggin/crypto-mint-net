package p2p

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
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
}

// PortMap returns the port mapping's status.
func (h *Host) PortMap() PortMapStatus {
	h.pmMu.Lock()
	defer h.pmMu.Unlock()
	st := h.pm
	if st.Mode == "" {
		st.Mode, st.State = PortMapOff, PortMapOff
		if h.portMapAsked() {
			st.Mode, st.State = h.cfg.PortMap, PortMapSkipped
		}
	}
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
// lifetime, never sooner than pmMinRenew.
var (
	pmRetry    = time.Minute
	pmMaxRetry = 30 * time.Minute
	pmMinRenew = 30 * time.Second
	pmUnmap    = 3 * time.Second
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
		ok := sleep(retry)
		retry = min(retry*2, pmMaxRetry)
		return ok
	}
	for ctx.Err() == nil {
		h.setPortMap(func(s *PortMapStatus) { s.State = PortMapTrying })
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
		if !h.holdMapping(ctx, m, got) {
			return
		}
		if !backoff() {
			return
		}
	}
}

// holdMapping confirms m, advertises it and renews it until it fails or
// ctx ends; it removes the mapping before returning. It returns false when
// ctx ended.
func (h *Host) holdMapping(ctx context.Context, mp PortMapper, m portmap.Mapping) bool {
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
		if m.External != confirmed {
			h.mapped.Store(nil)
			if !h.cfg.AllowLocalPeers && !publicIP(m.External.Addr()) {
				show(PortMapNotPublic, "the router's external address is not public: it is behind another NAT (such as carrier-grade NAT), so no mapping can reach this node")
				h.log.Info("port mapping gives no public address", "external", ext)
				return ctx.Err() == nil
			}
			if err := h.confirmSelf(ctx, ext); err != nil {
				if ctx.Err() != nil {
					return false
				}
				show(PortMapUnconfirmed, "a dial to the mapped address did not reach this node ("+err.Error()+"); if the router cannot loop back, set p2p.advertise by hand")
				h.log.Info("port mapping unconfirmed", "external", ext, "err", err)
				return true
			}
			confirmed = m.External
			h.mapped.Store(&ext)
			h.mu.Lock()
			h.selfAddrs[ext] = true
			h.mu.Unlock()
			h.log.Info("port mapped and advertised", "method", m.Method, "external", ext, "lease", m.Lifetime)
			show(PortMapAdvertised, "")
		}
		renew := m.Lifetime / 2
		if m.Lifetime == 0 {
			renew = h.cfg.PortMapLease / 2
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(max(renew, pmMinRenew)):
		}
		next, err := mp.Map(ctx, m.Method, m.Internal, h.cfg.PortMapLease, &m)
		if err != nil {
			if ctx.Err() != nil {
				return false
			}
			show(PortMapFailed, "renewal: "+err.Error())
			h.log.Info("port mapping renewal failed", "err", err)
			return true
		}
		m = next
	}
}

// confirmSelf dials addr and reports nil when the hello that answers is
// this node's own: the mapped address leads here. Only the router's report
// and this dial decide what is advertised, never what peers say.
func (h *Host) confirmSelf(ctx context.Context, addr string) error {
	d := net.Dialer{Timeout: 10 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(helloTimeout))
	mine := wire.Hello{Protocol: Protocol, NodeID: h.id, Agent: h.cfg.Agent}
	if err := wire.WriteFrame(c, wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: mine.Encode()}); err != nil {
		return err
	}
	m, _, err := wire.ReadFrame(bufio.NewReader(c), wire.MaxFrame)
	if err != nil {
		return err
	}
	if m.Type != wire.TypeHello {
		return errors.New("the answer was not a hello")
	}
	theirs, err := wire.DecodeHello(m.Body)
	if err != nil {
		return err
	}
	if theirs.NodeID != h.id {
		return errors.New("another node answered")
	}
	return nil
}
