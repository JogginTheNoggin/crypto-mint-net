// Package nodepolicy holds a full node's node policy: every choice a node
// makes for itself, as opposed to the rules every node must follow to agree
// on who owns what. That is which series it knows and which issuers it
// accepts, what it follows, how many inactive branches it keeps, how many
// pending transactions it holds, how it writes to disk and where its local
// RPC listens.
//
// A node policy is a JSON file, node.json, in the node's data directory.
// Package nodepolicy also manages several of them side by side, with the
// design's three operations: save (keep it and pick up where it left off),
// archive (keep it apart; using it again starts from scratch) and delete.
// See Manager.
//
// Nothing here changes what is valid. Two nodes with different node
// policies agree on ownership of everything they both follow.
package nodepolicy

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// Format is the version of the node.json layout this code reads.
const Format = 1

// FileName is the node policy's file name inside a data directory.
const FileName = "node.json"

// DefaultListen is the default address of the local RPC: loopback only.
const DefaultListen = "127.0.0.1:8645"

// DefaultP2PListen is the default address for connections from peers: every
// interface, since peers are on other machines.
const DefaultP2PListen = ":8646"

// ErrInvalid marks a node policy that cannot be used as written.
var ErrInvalid = errors.New("nodepolicy: invalid")

// Policy is one node policy.
type Policy struct {
	Format int `json:"format"`
	// Series lists the series this node knows. Series information comes
	// from outside the network, never from peers.
	Series []SeriesInfo `json:"series"`
	// Issuers says which series are accepted.
	Issuers Issuers `json:"issuers"`
	// Follow lists the mint policies this node follows.
	Follow []Follow `json:"follow"`
	// K is how many inactive branches to keep per pool; nil means the
	// default, -1 keeps every branch.
	K *int `json:"k,omitempty"`
	// Ties is how many branches tying the selected weight to keep beyond K
	// (0: default).
	Ties int `json:"ties,omitempty"`
	// MaxPending is the pending transaction limit per pool (0: default).
	MaxPending int `json:"max_pending,omitempty"`
	// SeenSize is the size of the seen list (0: default).
	SeenSize int `json:"seen_size,omitempty"`
	// NoSync skips the fsync after each write to the store. Faster; a power
	// loss can then lose the latest items, which peers can send again.
	NoSync bool `json:"no_sync,omitempty"`
	// RPC configures the local control interface.
	RPC RPC `json:"rpc"`
	// P2P configures connections to other full nodes.
	P2P P2P `json:"p2p"`
	// Tracking configures what the node records about its peers.
	Tracking Tracking `json:"tracking,omitempty"`
}

// Tracking is what a node records about its peers and how offences are
// treated. Zero values mean the defaults of package tracking. The records
// themselves are kept in tracking.json next to the policy, so a saved
// policy picks up where it left off.
type Tracking struct {
	// Template is the default tracking template: basic, standard,
	// detailed or a custom one.
	Template string `json:"template,omitempty"`
	// Custom are the user's own templates.
	Custom map[string]tracking.Custom `json:"custom,omitempty"`
	// Groups maps a peer group (priority, backup, candidate, unlisted,
	// inbound, outbound) to a template.
	Groups map[string]string `json:"groups,omitempty"`
	// Peers maps an address to its own template.
	Peers map[string]string `json:"peers,omitempty"`
	// EscalateAt is the flag total at which a peer is tracked with the
	// standard template as well; -1 never.
	EscalateAt float64 `json:"escalate_at,omitempty"`
	// The flag totals at which a peer loses priority, is timed out and is
	// blocked, and how long a timeout and a block last.
	PriorityLimit  float64 `json:"priority_limit,omitempty"`
	TimeoutLimit   float64 `json:"timeout_limit,omitempty"`
	BlockLimit     float64 `json:"block_limit,omitempty"`
	TimeoutSeconds int     `json:"timeout_seconds,omitempty"`
	BlockSeconds   int     `json:"block_seconds,omitempty"`
	// Weights say how much each measure counts towards points.
	Weights *tracking.Weights `json:"weights,omitempty"`
	// StartingPoints are what a newly met peer begins with.
	StartingPoints float64 `json:"starting_points,omitempty"`
	// NoTimeoutMisses stops pings missed during a timeout counting.
	NoTimeoutMisses bool `json:"no_timeout_misses,omitempty"`
}

// SeriesInfo is one known series, with the issuer's witness when the series
// names an issuer.
type SeriesInfo struct {
	Params series.Params `json:"params"`
	// IssuerWitness is the hex of the witness's byte form, signing the
	// series' issuer message. Required for a series accepted by issuer.
	IssuerWitness string `json:"issuer_witness,omitempty"`
}

// Issuers is the rule for which series a node accepts.
type Issuers struct {
	// Any accepts every listed series, including ones that name no issuer.
	// An issuer witness that is present must still be valid.
	Any bool `json:"any"`
	// Accept lists issuer addresses whose series are accepted, each with a
	// valid issuer witness.
	Accept []hash.Hash `json:"accept,omitempty"`
}

// Follow is one mint policy: an issuer, a series, some pools of a series or
// some mints of a series. Exactly one of Issuer and Series is set. A series
// with no Pools and no Mints means the whole series.
type Follow struct {
	Issuer *hash.Hash `json:"issuer,omitempty"`
	Series *hash.Hash `json:"series,omitempty"`
	Pools  []uint64   `json:"pools,omitempty"`
	Mints  []uint64   `json:"mints,omitempty"`
}

// RPC configures the local control interface.
type RPC struct {
	// Listen is the TCP address; empty means DefaultListen, "off" disables
	// the interface.
	Listen string `json:"listen,omitempty"`
	// AllowRemote permits a non-loopback Listen address. The interface
	// controls the node, so this is off by default.
	AllowRemote bool `json:"allow_remote,omitempty"`
}

// P2P configures connections to other full nodes. Zero values mean the
// defaults of package p2p.
type P2P struct {
	// Listen is the TCP address for peers; empty means DefaultP2PListen,
	// "off" accepts no connections (the node still dials Peers).
	Listen string `json:"listen,omitempty"`
	// Advertise is the address peers are told to connect to.
	Advertise string `json:"advertise,omitempty"`
	// Peers are host:port addresses to stay connected to.
	Peers []string `json:"peers,omitempty"`
	// MaxPeers caps open connections.
	MaxPeers int `json:"max_peers,omitempty"`
	// The reply limits this node sets on its requests.
	ChunkSize  uint32 `json:"chunk_size,omitempty"`
	EntryLimit uint32 `json:"entry_limit,omitempty"`
	HashLimit  uint32 `json:"hash_limit,omitempty"`
	LayerLimit uint32 `json:"layer_limit,omitempty"`
	// NoNotices stops the node telling peers when it switches to a ledger
	// it asked for.
	NoNotices bool `json:"no_notices,omitempty"`
	// CheckSeconds is how often to ask the priority peers of each pool for
	// their stated weight; 0 means the default, -1 never.
	CheckSeconds int `json:"check_seconds,omitempty"`
	// Registries are address registry locations (http(s) URLs or files)
	// to find first peers from. None by default: there is no project
	// registry yet.
	Registries []string `json:"registries,omitempty"`
	// MaxOutbound caps the connections the node dials itself.
	MaxOutbound int `json:"max_outbound,omitempty"`
	// AllowLocalPeers lets addresses learned from peers and registries be
	// loopback, private or otherwise not public, and dials them. Off by
	// default (decision D96); turn it on for a network on one machine or
	// one LAN. Addresses in Peers are always dialled.
	AllowLocalPeers bool `json:"allow_local_peers,omitempty"`
	// MaxInbound caps the connections other nodes open to this one; by
	// default MaxPeers less MaxOutbound, -1 for no separate cap.
	MaxInbound int `json:"max_inbound,omitempty"`
	// NoRelay stops the node relaying other nodes' broadcast follow
	// requests; HopLimit is the hop count at which it stops relaying.
	NoRelay  bool `json:"no_relay,omitempty"`
	HopLimit int  `json:"hop_limit,omitempty"`
	// RequestRate caps the discovery requests one peer may send per minute.
	RequestRate int `json:"request_rate,omitempty"`
	// PingSeconds is how often to ping each priority peer; 0 the default,
	// -1 never. Backup peers are pinged a third as often.
	PingSeconds int `json:"ping_seconds,omitempty"`

	// PrioritySlots and BackupSlots are how many priority and backup peers
	// each mint policy holds; the node dials candidates until they are
	// full.
	PrioritySlots int `json:"priority_slots,omitempty"`
	BackupSlots   int `json:"backup_slots,omitempty"`
	// ReviewSeconds is how often points are compared to move peers
	// between priority and backup; 0 the default, -1 never. Margin is how
	// far a backup must beat the lowest priority peer to replace it.
	ReviewSeconds int     `json:"review_seconds,omitempty"`
	Margin        float64 `json:"margin,omitempty"`
	// UnlistedConns caps inbound connections in no category, such as
	// wallets; -1 allows none.
	UnlistedConns int `json:"unlisted_conns,omitempty"`
	// StrictReturn returns a timed out peer as a backup.
	StrictReturn bool `json:"strict_return,omitempty"`
	// NoConfirm stops asking a priority peer to confirm a stated weight
	// from another sender; FastCatchUp asks a priority peer for its whole
	// ledger when a split is deeper than DepthLimit hash chain pages.
	NoConfirm   bool `json:"no_confirm,omitempty"`
	FastCatchUp bool `json:"fast_catch_up,omitempty"`
	DepthLimit  int  `json:"depth_limit,omitempty"`
}

// Default returns a node policy that knows no series and follows nothing.
// Issuers.Any is on, because there is no project issuer yet (decision D24).
func Default() Policy {
	return Policy{Format: Format, Issuers: Issuers{Any: true}}
}

// LedgerConfig returns the ledger settings this policy asks for.
func (p Policy) LedgerConfig() ledger.Config {
	c := ledger.Config{K: ledger.DefaultK, Ties: p.Ties}
	if p.K != nil {
		c.K = *p.K
	}
	return c
}

// ListenAddr returns the RPC address, or "" when the interface is off.
func (p Policy) ListenAddr() string {
	switch p.RPC.Listen {
	case "":
		return DefaultListen
	case "off":
		return ""
	}
	return p.RPC.Listen
}

// P2PListenAddr returns the address for peers, or "" when the node accepts
// none.
func (p Policy) P2PListenAddr() string {
	switch p.P2P.Listen {
	case "":
		return DefaultP2PListen
	case "off":
		return ""
	}
	return p.P2P.Listen
}

// Validate checks the policy for mistakes that would stop a node from
// starting. It does not check issuer witnesses; Accepted does.
func (p Policy) Validate() error {
	if p.Format != Format {
		return fmt.Errorf("%w: format %d, this node reads %d", ErrInvalid, p.Format, Format)
	}
	if p.K != nil && *p.K < ledger.Unlimited {
		return fmt.Errorf("%w: k must be -1 (keep all) or more", ErrInvalid)
	}
	if p.Ties < 0 || p.MaxPending < 0 || p.SeenSize < 0 {
		return fmt.Errorf("%w: limits must not be negative", ErrInvalid)
	}
	for i, f := range p.Follow {
		if (f.Issuer == nil) == (f.Series == nil) {
			return fmt.Errorf("%w: follow %d must name exactly one of issuer and series", ErrInvalid, i)
		}
		if f.Issuer != nil && (len(f.Pools) > 0 || len(f.Mints) > 0) {
			return fmt.Errorf("%w: follow %d: an issuer has no pools or mints", ErrInvalid, i)
		}
	}
	if addr := p.ListenAddr(); addr != "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("%w: rpc listen %q: %v", ErrInvalid, addr, err)
		}
		if !p.RPC.AllowRemote && !isLoopback(host) {
			return fmt.Errorf("%w: rpc listen %q is not a loopback address; set allow_remote to permit it", ErrInvalid, addr)
		}
	}
	if addr := p.P2PListenAddr(); addr != "" {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("%w: p2p listen %q: %v", ErrInvalid, addr, err)
		}
	}
	if a := p.P2P.Advertise; a != "" {
		if _, _, err := net.SplitHostPort(a); err != nil {
			return fmt.Errorf("%w: p2p advertise %q: %v", ErrInvalid, a, err)
		}
	}
	for _, a := range p.P2P.Peers {
		if _, _, err := net.SplitHostPort(a); err != nil {
			return fmt.Errorf("%w: p2p peer %q: %v", ErrInvalid, a, err)
		}
	}
	if p.P2P.MaxPeers < 0 || p.P2P.CheckSeconds < -1 || p.P2P.PingSeconds < -1 || p.P2P.ReviewSeconds < -1 ||
		p.P2P.MaxOutbound < 0 || p.P2P.MaxInbound < -1 || p.P2P.HopLimit < 0 || p.P2P.RequestRate < 0 || p.P2P.PrioritySlots < 0 ||
		p.P2P.BackupSlots < 0 || p.P2P.Margin < 0 || p.P2P.UnlistedConns < -1 || p.P2P.DepthLimit < 0 {
		return fmt.Errorf("%w: p2p limits must not be negative", ErrInvalid)
	}
	if _, err := tracking.New(p.TrackingConfig(), ""); err != nil {
		return fmt.Errorf("%w: tracking: %v", ErrInvalid, err)
	}
	t := p.Tracking
	if t.PriorityLimit < 0 || t.TimeoutLimit < 0 || t.BlockLimit < 0 || t.TimeoutSeconds < 0 || t.BlockSeconds < 0 || t.EscalateAt < -1 {
		return fmt.Errorf("%w: tracking limits must not be negative", ErrInvalid)
	}
	for _, r := range p.P2P.Registries {
		if strings.TrimSpace(r) == "" {
			return fmt.Errorf("%w: p2p registry location is empty", ErrInvalid)
		}
	}
	return nil
}

// TrackingConfig returns the tracking settings this policy asks for.
func (p Policy) TrackingConfig() tracking.Config {
	t := p.Tracking
	c := tracking.Config{
		Default: t.Template, Custom: t.Custom, Groups: t.Groups, Peers: t.Peers,
		StartingPoints: t.StartingPoints, NoTimeoutMisses: t.NoTimeoutMisses,
	}
	d := tracking.DefaultConfig()
	c.EscalateAt = d.EscalateAt
	switch {
	case t.EscalateAt < 0:
		c.EscalateAt = 0
	case t.EscalateAt > 0:
		c.EscalateAt = t.EscalateAt
	}
	c.Limits = d.Limits
	if t.PriorityLimit > 0 {
		c.Limits.Priority = t.PriorityLimit
	}
	if t.TimeoutLimit > 0 {
		c.Limits.TimeOut = t.TimeoutLimit
	}
	if t.BlockLimit > 0 {
		c.Limits.Block = t.BlockLimit
	}
	if t.TimeoutSeconds > 0 {
		c.Limits.TimeOutFor = time.Duration(t.TimeoutSeconds) * time.Second
	}
	if t.BlockSeconds > 0 {
		c.Limits.BlockFor = time.Duration(t.BlockSeconds) * time.Second
	}
	if t.Weights != nil {
		c.Weights = *t.Weights
	}
	return c
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Accepted is a known series the node accepts.
type Accepted struct {
	Series *series.Series
	// Issuer is the verified issuer, if the series names one and its
	// witness checked.
	Issuer *hash.Hash
}

// Refused is a listed series the node does not accept, and why.
type Refused struct {
	Index  int
	Reason error
}

// Accepted builds every listed series and applies the issuer rule. A series
// that cannot be built, is unreadable at this node, or is not accepted is
// listed in refused rather than failing the whole policy, so one bad series
// does not stop a node.
func (p Policy) Accepted() (accepted []Accepted, refused []Refused) {
	allow := map[hash.Hash]bool{}
	for _, a := range p.Issuers.Accept {
		allow[a] = true
	}
	for i, si := range p.Series {
		s, err := series.New(si.Params)
		if err != nil {
			refused = append(refused, Refused{i, err})
			continue
		}
		var issuer *hash.Hash
		if si.IssuerWitness != "" {
			w, err := DecodeWitness(si.IssuerWitness)
			if err == nil {
				err = s.CheckIssuer(w)
			}
			if err != nil {
				refused = append(refused, Refused{i, fmt.Errorf("issuer witness: %w", err)})
				continue
			}
			issuer = si.Params.Issuer
		}
		switch {
		case issuer != nil && allow[*issuer]:
		case p.Issuers.Any:
		case issuer == nil && si.Params.Issuer != nil && allow[*si.Params.Issuer]:
			refused = append(refused, Refused{i, errors.New("issuer accepted, but the series has no issuer witness")})
			continue
		default:
			refused = append(refused, Refused{i, errors.New("issuer not accepted")})
			continue
		}
		accepted = append(accepted, Accepted{Series: s, Issuer: issuer})
	}
	return accepted, refused
}

// EncodeWitness returns the hex form of a witness used in node.json.
func EncodeWitness(w policy.Witness) string {
	enc := encoding.NewWriter(w.Size())
	w.Encode(enc)
	return hex.EncodeToString(enc.Bytes())
}

// DecodeWitness reads the hex form written by EncodeWitness.
func DecodeWitness(s string) (policy.Witness, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return policy.Witness{}, err
	}
	r := encoding.NewReader(b)
	w := policy.DecodeWitness(r)
	if err := r.Finish(); err != nil {
		return policy.Witness{}, err
	}
	return w, nil
}

// Load reads and validates a node policy file.
func Load(path string) (Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return Policy{}, fmt.Errorf("%w: %s: %v", ErrInvalid, path, err)
	}
	if err := p.Validate(); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Save validates p and writes it to path atomically.
func Save(path string, p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
