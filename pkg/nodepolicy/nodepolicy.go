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
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
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
	// Lowered says what Load brought down to a bound lowered since the
	// file was written, for the caller to warn of; it is not saved.
	Lowered []Lowered `json:"-"`
	// Series lists the series this node knows. Series information comes
	// from outside the network, never from peers.
	Series []SeriesInfo `json:"series"`
	// Issuers says which series are accepted.
	Issuers Issuers `json:"issuers"`
	// SeriesCeiling, when set, declines a series being added to this
	// policy (by a policy import) whose witness ceiling is not set or
	// above it (D170). Declining is a lack of information, never a
	// verdict on a ledger, and it never drops a series already listed.
	// 0 declines none. It is a bounded setting (BoundSeriesCeiling), so
	// poo policy limits shows and sets it; poo init writes CeilingLimit
	// in a new node.json (RED-083, D171).
	SeriesCeiling int `json:"series_ceiling,omitempty"`
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
	// EntryCacheMiB bounds the entry bodies kept in memory, read back
	// from the pool logs, in MiB (0: default; D172).
	EntryCacheMiB int `json:"entry_cache_mib,omitempty"`
	// RecentWindow is how many entries below each selected tip keep their
	// items and undo records in memory (0: default; D175).
	RecentWindow int `json:"recent_window,omitempty"`
	// NoSync skips the fsync after each write to the store. Faster; a power
	// loss can then lose the latest items, which peers can send again.
	NoSync bool `json:"no_sync,omitempty"`
	// RPC configures the local control interface.
	RPC RPC `json:"rpc"`
	// P2P configures connections to other full nodes.
	P2P P2P `json:"p2p"`
	// Tracking configures what the node records about its peers.
	Tracking Tracking `json:"tracking,omitempty"`
	// Log chooses how much the node logs and in which form (GREEN-006).
	Log *Log `json:"log,omitempty"`
	// Grind says how the node grinds and which jobs it runs by itself
	// (package grind, M11).
	Grind *Grind `json:"grind,omitempty"`
}

// Grind configures the node's grinding (package grind, decision D109).
type Grind struct {
	// Device is the device grinding uses unless a job names one: "auto"
	// (the default) or an id from poo grind devices ("cpu", "gpu:0").
	Device string `json:"device,omitempty"`
	// Threads is how many threads a search takes (0: the device's
	// default, every CPU core but one).
	Threads int `json:"threads,omitempty"`
	// LayerFloor, when above 0, keeps every layer the node's own grinding
	// finds at or above this strength, not only the strongest of each
	// search, so more of its work counts (ORANGE-010, decision D118). It
	// is raised to the series' minimum layer strength where that is
	// higher. 0, the default, keeps only the strongest. It changes what
	// this node sends, never what any node accepts.
	LayerFloor int `json:"layer_floor,omitempty"`
	// LayerRate bounds the layers a second that layer_floor adds, the
	// strongest of each search kept first, so peers' layer budgets (6,000
	// a minute by default) take every one; 0 means DefaultLayerRate
	// (ORANGE-016).
	LayerRate int `json:"layer_rate,omitempty"`
	// Jobs start when the node starts and run until it stops: automatic
	// work, or scheduled with hours.
	Jobs []GrindJob `json:"jobs,omitempty"`
}

// IsZero reports whether the section sets nothing, so a node policy can
// leave it out. Every field counts: a section of only layer_floor was
// dropped when it was saved (GREEN-032).
func (g Grind) IsZero() bool {
	return g.Device == "" && g.Threads == 0 && g.LayerFloor == 0 && g.LayerRate == 0 && len(g.Jobs) == 0
}

// GrindJob is a job the node runs by itself (grind.Spec).
type GrindJob struct {
	// Target is "tip" (layers on the selected tip), "pending" (entries
	// from pending transactions), "tx:" and a TID (one payment, until
	// UntilFrom), or an entry hash.
	Target string     `json:"target"`
	Series *hash.Hash `json:"series,omitempty"`
	Pools  []uint64   `json:"pools,omitempty"`
	Device string     `json:"device,omitempty"`
	// Threads 0 takes the section's.
	Threads int `json:"threads,omitempty"`
	// Hours is a window of the node's local hours, "22-06".
	Hours string `json:"hours,omitempty"`
	// UntilFrom ends a job on one entry or payment once from(e) reaches
	// it.
	UntilFrom string `json:"until_from,omitempty"`
}

// MaxLayerFloor is the strongest layer_floor a grind section may set.
const MaxLayerFloor = 256

// GrindLimits are the bounds a grind section is checked against, so a
// front end can offer exactly what validates (GUI/Blue's ask, D130).
type GrindLimits struct {
	MaxJobs          int `json:"max_jobs"`
	MaxThreads       int `json:"max_threads"`
	MaxLayerFloor    int `json:"max_layer_floor"`
	MaxLayerRate     int `json:"max_layer_rate"`
	DefaultLayerRate int `json:"default_layer_rate"`
}

// Limits returns the bounds Grind.Validate checks.
func Limits() GrindLimits {
	return GrindLimits{MaxJobs: MaxGrindJobs, MaxThreads: MaxGrindThreads, MaxLayerFloor: MaxLayerFloor,
		MaxLayerRate: MaxLayerRate, DefaultLayerRate: DefaultLayerRate}
}

// MaxGrindJobs bounds the jobs a node policy may list.
const MaxGrindJobs = 16

// MaxGrindThreads bounds the threads a grind setting may ask for.
const MaxGrindThreads = 1024

// The layers a second layer_floor adds by default, and at most. Peers
// take 6,000 layers a minute from one node by default (tracking), 100 a
// second, and drop the rest, the strongest too, while it sends more
// (ORANGE-016); a node that keeps sending more is flagged once a minute,
// timed out and then blocked. The most is half that budget, so the
// floor's layers leave room for the node's other layers and those it
// forwards (GUI-015, D127). It bounds only what this node sends.
const (
	DefaultLayerRate = 10
	MaxLayerRate     = 50
)

// ParseHours reads a window "FROM-TO" of whole local hours, 0 to 23; TO
// may be below FROM, for a window across midnight. "" is every hour.
func ParseHours(s string) (from, to int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	a, b, ok := strings.Cut(s, "-")
	from, e1 := strconv.Atoi(strings.TrimSpace(a))
	to, e2 := strconv.Atoi(strings.TrimSpace(b))
	if !ok || e1 != nil || e2 != nil || from < 0 || from > 23 || to < 0 || to > 23 || from == to {
		return 0, 0, fmt.Errorf("hours %q: give FROM-TO in whole hours 0 to 23, such as 22-06", s)
	}
	return from, to, nil
}

func (g *Grind) validate() error {
	if g == nil {
		return nil
	}
	if g.Threads < 0 || g.Threads > MaxGrindThreads {
		return fmt.Errorf("threads must be 0 to %d", MaxGrindThreads)
	}
	if g.LayerFloor < 0 || g.LayerFloor > MaxLayerFloor {
		return errors.New("layer_floor must be 0 (off) to 256")
	}
	if g.LayerRate < 0 || g.LayerRate > MaxLayerRate {
		return fmt.Errorf("layer_rate must be 0 (the default, %d) to %d", DefaultLayerRate, MaxLayerRate)
	}
	if len(g.Jobs) > MaxGrindJobs {
		return fmt.Errorf("at most %d jobs", MaxGrindJobs)
	}
	for i, j := range g.Jobs {
		entry := false
		switch {
		case j.Target == "tip", j.Target == "pending":
		case strings.HasPrefix(j.Target, "tx:"):
			if _, err := hash.Parse(strings.TrimPrefix(j.Target, "tx:")); err != nil {
				return fmt.Errorf("job %d: tx:TID: %v", i, err)
			}
			if j.UntilFrom == "" {
				return fmt.Errorf("job %d: a tx:TID target needs until_from", i)
			}
			entry = true
		default:
			if _, err := hash.Parse(j.Target); err != nil {
				return fmt.Errorf("job %d: target is tip, pending, tx:TID or an entry hash", i)
			}
			entry = true
		}
		if j.Threads < 0 || j.Threads > MaxGrindThreads {
			return fmt.Errorf("job %d: threads must be 0 to %d", i, MaxGrindThreads)
		}
		if _, _, err := ParseHours(j.Hours); err != nil {
			return fmt.Errorf("job %d: %v", i, err)
		}
		if j.UntilFrom != "" {
			if !entry {
				return fmt.Errorf("job %d: until_from needs an entry as the target", i)
			}
			var w weight.Weight
			if err := w.UnmarshalText([]byte(j.UntilFrom)); err != nil {
				return fmt.Errorf("job %d: until_from: %v", i, err)
			}
		}
	}
	return nil
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
	// MaxAddresses bounds the addresses the tracker keeps records for; the
	// oldest with no flag, block or timeout go first. FlagHistorySeconds is
	// how long an expired flag is remembered, so a repeat offence counts
	// more; LogSeconds and LogSize bound each address's detailed log. Zero
	// means the default; see Bounds for each one's range (D159).
	MaxAddresses       int `json:"max_addresses,omitempty"`
	FlagHistorySeconds int `json:"flag_history_seconds,omitempty"`
	LogSeconds         int `json:"log_seconds,omitempty"`
	LogSize            int `json:"log_size,omitempty"`
}

// SeriesInfo is one known series, with the issuer's witness when the series
// names an issuer.
type SeriesInfo struct {
	Params series.Params `json:"params"`
	// IssuerWitness is the hex of the witness's byte form, signing the
	// series' issuer message. Required for a series accepted by issuer,
	// and for a variable series, whose part 1 it releases.
	IssuerWitness string `json:"issuer_witness,omitempty"`
	// Release is the newest release of a variable series this node knows
	// (D166). It proves every part before it, so it is the only one kept.
	Release *series.Release `json:"release,omitempty"`
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
// with no Pools and no Mints means the whole series, but the pools in
// Except.
type Follow struct {
	Issuer *hash.Hash `json:"issuer,omitempty"`
	Series *hash.Hash `json:"series,omitempty"`
	Pools  []uint64   `json:"pools,omitempty"`
	Mints  []uint64   `json:"mints,omitempty"`
	// Except, with a whole series, lists the pools left out of it: all
	// but these (RED-050, D154).
	Except []uint64 `json:"except,omitempty"`
}

// Log configures the node's log (package logging).
type Log struct {
	// Level is debug, info (the default), warn or error.
	Level string `json:"level,omitempty"`
	// Format is plain (the default: one readable line per event), text
	// (key=value with time and level) or json (one object per line).
	Format string `json:"format,omitempty"`
}

// LogSettings returns the log level and format, empty for the defaults.
func (p Policy) LogSettings() (level, format string) {
	if p.Log == nil {
		return "", ""
	}
	return p.Log.Level, p.Log.Format
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
	// their stated weight; 0 means the default. The check cannot be turned
	// off: -1, or anything over MaxCheckSeconds, is read as
	// MaxCheckSeconds, so a node whose peers send no notices still finds a
	// heavier ledger they hold (Black B7, D153).
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
	// NoDialImported stops the node trying, when it is short of peers,
	// the unlisted addresses an imported peer file gave it; they are then
	// tried only by poo peer connect (D157).
	NoDialImported bool `json:"no_dial_imported,omitempty"`
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
	// from another sender.
	NoConfirm bool `json:"no_confirm,omitempty"`
	// FastCatchUp and DepthLimit are no longer used: a chase finds the
	// common entry at any depth by halving (audit item G3, decision D116).
	// They are kept so older policy files still load, and are ignored.
	FastCatchUp bool `json:"fast_catch_up,omitempty"`
	DepthLimit  int  `json:"depth_limit,omitempty"`

	// MaxBook bounds the addresses the peer book holds, and BookPerRange
	// those from one network range; a quarter of the book is for imported
	// addresses (D158). VerifiedPerRange bounds the priority and backup
	// peers from one range for one mint policy. RetrySeconds is the first
	// wait before trying a candidate again after a failure, doubled with
	// each one. RegistrySeconds is the least time between two loads of the
	// registries. CandidateSeconds is how long an address learned from
	// the network and never reached keeps its place against a newer one
	// when the book or its range is full (D160). Zero means the default;
	// see Bounds for each one's range (D159).
	MaxBook          int `json:"max_book,omitempty"`
	BookPerRange     int `json:"book_per_range,omitempty"`
	VerifiedPerRange int `json:"verified_per_range,omitempty"`
	RetrySeconds     int `json:"retry_seconds,omitempty"`
	RegistrySeconds  int `json:"registry_seconds,omitempty"`
	CandidateSeconds int `json:"candidate_seconds,omitempty"`
}

// MaxCheckSeconds is the longest a node waits between asking its
// priority peers for their stated weight (D153). The check is how a node
// learns of a heavier ledger its peers hold without telling it, so it is
// never off; an hour costs one small request per priority peer per pool.
const MaxCheckSeconds = 3600

// Default returns a node policy that knows no series and follows nothing.
// Issuers.Any is on, because there is no project issuer yet (decision D24).
func Default() Policy {
	return Policy{Format: Format, Issuers: Issuers{Any: true}}
}

// LedgerConfig returns the ledger settings this policy asks for.
func (p Policy) LedgerConfig() ledger.Config {
	c := ledger.Config{K: ledger.DefaultK, Ties: p.Ties, RecentWindow: BoundRecentWindow.Or(p.RecentWindow)}
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
	if err := checkText(p); err != nil {
		return err
	}
	if p.K != nil && *p.K < ledger.Unlimited {
		return fmt.Errorf("%w: k must be -1 (keep all) or more", ErrInvalid)
	}
	if err := logging.Check(p.LogSettings()); err != nil {
		return fmt.Errorf("%w: log: %v", ErrInvalid, err)
	}
	if err := p.Grind.validate(); err != nil {
		return fmt.Errorf("%w: grind: %v", ErrInvalid, err)
	}
	for i, f := range p.Follow {
		if (f.Issuer == nil) == (f.Series == nil) {
			return fmt.Errorf("%w: follow %d must name exactly one of issuer and series", ErrInvalid, i)
		}
		if f.Issuer != nil && (len(f.Pools) > 0 || len(f.Mints) > 0 || len(f.Except) > 0) {
			return fmt.Errorf("%w: follow %d: an issuer has no pools, mints or except", ErrInvalid, i)
		}
		if len(f.Except) > 0 && (len(f.Pools) > 0 || len(f.Mints) > 0) {
			return fmt.Errorf("%w: follow %d: except goes with a whole series, not with pools or mints", ErrInvalid, i)
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
	if err := p.checkBounds(); err != nil {
		return err
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
	c.Retain = p.retention()
	return c
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ErrNoPartOne refuses a variable supply series listed without its issuer
// witness: part 1 of a variable series is released by the issuer witness
// on the series (PoO section 16.8), so without it no mint of the series is
// released at this node. Missing information, never a series found
// invalid (D166).
var ErrNoPartOne = errors.New("a variable supply series' mints are released by its issuer witness, and node.json has none for it: add the issuer_witness its issuer published (poo series sign)")

// Accepted is a known series the node accepts.
type Accepted struct {
	Series *series.Series
	// Issuer is the verified issuer, if the series names one and its
	// witness checked.
	Issuer *hash.Hash
	// ReleaseErr says why the release node.json keeps for the series was
	// not used. The series is followed with the parts proven without it
	// (RED-073).
	ReleaseErr error
}

// Refused is a listed series the node does not accept, and why.
type Refused struct {
	Index  int
	Reason error
}

// Build builds the series si lists and adds the release it keeps. A kept
// release is information the node was given: one that does not check is
// left out, and said in release, and the series is built with the parts
// proven without it, part 1 at least, never refused for it (RED-073).
// Every place that reads a listed series builds it here, so each sees the
// same parts the running node does (RED-072).
func (si SeriesInfo) Build() (s *series.Series, release error, err error) {
	s, err = series.New(si.Params)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case si.Release == nil:
	case !s.IsVariable():
		release = fmt.Errorf("release: %w", series.ErrFixed)
	default:
		if _, rerr := s.AddRelease(*si.Release); rerr != nil {
			release = fmt.Errorf("release of part %d: %w", si.Release.Part, rerr)
		}
	}
	return s, release, nil
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
		s, relErr, err := si.Build()
		if err == nil && s.IsVariable() && si.IssuerWitness == "" {
			err = ErrNoPartOne
		}
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
		accepted = append(accepted, Accepted{Series: s, Issuer: issuer, ReleaseErr: relErr})
	}
	return accepted, refused
}

// EncodeWitness returns the hex form of a witness used in node.json.
func EncodeWitness(w policy.Witness) string {
	enc := encoding.NewWriter(w.Size())
	w.Encode(enc)
	return hex.EncodeToString(enc.Bytes())
}

// CeilingLimit is the largest witness ceiling poo series new and poo init
// take without being forced, and the line above which an import, poo
// series info and poo doctor warn (D170, D171): one witness up to 4 MiB
// checks in about 3 s of one core. series_ceiling is off (0) unless the
// node sets it; nothing writes it by default (jerry, 2026-11-04).
const CeilingLimit = 4 << 20

// CeilingWarning says why a series' witness ceiling is worth a warning,
// or "" when it is not: it sets none, or one above CeilingLimit, so one
// witness can take seconds and hundreds of MB to check (RED-083). It
// refuses nothing.
func CeilingWarning(sp series.Params) string {
	switch {
	case sp.MaxWitness == 0:
		return "the series sets no witness ceiling: one witness can take seconds and hundreds of MB to check"
	case sp.MaxWitness > CeilingLimit:
		return fmt.Sprintf("the series' witness ceiling is %d bytes, above %d: one witness can take seconds to check", sp.MaxWitness, CeilingLimit)
	}
	return ""
}

// DeclinesSeries reports why the policy declines to add a series with
// params sp (SeriesCeiling, D170), or nil when it takes it.
func (p Policy) DeclinesSeries(sp series.Params) error {
	switch c := p.SeriesCeiling; {
	case c == 0:
		return nil
	case sp.MaxWitness == 0:
		return fmt.Errorf("the series sets no witness ceiling, and series_ceiling is %d", c)
	case int64(sp.MaxWitness) > int64(c):
		return fmt.Errorf("the series' witness ceiling is %d, above series_ceiling %d", sp.MaxWitness, c)
	}
	return nil
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

// lower brings a value over a bound that was lowered since it was
// written down to the new bound, and says what it changed. A file that
// was valid when it was written stays one a node opens: a lowered bound
// is enforced on every write (Save, the RPC, the CLI, the GUI) and only
// clamped on a read (RED-025, D128).
func (p *Policy) lower() []Lowered {
	var notes []Lowered
	if c := p.P2P.CheckSeconds; c < 0 || c > MaxCheckSeconds {
		notes = append(notes, Lowered{Field: "p2p.check_seconds", Was: c, Now: MaxCheckSeconds})
		p.P2P.CheckSeconds = MaxCheckSeconds
	}
	if g := p.Grind; g != nil && g.LayerRate > MaxLayerRate {
		notes = append(notes, Lowered{Field: "grind.layer_rate", Was: g.LayerRate, Now: MaxLayerRate,
			FixArgs: []string{"grind", "schedule", "set", "-layer-rate", strconv.Itoa(MaxLayerRate)}})
		g.LayerRate = MaxLayerRate
	}
	return notes
}

// Lowered is one value Load brought down to a bound lowered since the
// file was written: the field, as named in node.json, the value written
// and the value used. FixArgs, when set, is the poo command line that
// writes the value used, for `poo doctor`'s fix_args (GREEN-035).
type Lowered struct {
	Field   string   `json:"field"`
	Was     int      `json:"was"`
	Now     int      `json:"now"`
	FixArgs []string `json:"fix_args,omitempty"`
}

// String says what was lowered, for a warning.
func (l Lowered) String() string {
	if l.Was < 0 {
		return fmt.Sprintf("%s %d would turn it off, which it no longer can: %d is used", l.Field, l.Was, l.Now)
	}
	return fmt.Sprintf("%s %d is over the most, %d: %d is used", l.Field, l.Was, l.Now, l.Now)
}

// SameApplied reports whether p and q differ only in what a running node
// takes in without a reload: the grind section, which the schedule
// commands apply at once (D125). Anything else that differs needs a
// reload to take effect (GREEN-037).
func (p Policy) SameApplied(q Policy) bool {
	p.Grind, q.Grind = nil, nil
	a, errA := json.Marshal(p)
	b, errB := json.Marshal(q)
	return errA == nil && errB == nil && string(a) == string(b)
}

// Load reads and validates a node policy file. A value over a bound
// lowered since the file was written is brought down to it, and noted in
// Lowered.
func Load(path string) (Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return Policy{}, fmt.Errorf("%w: %s: %v", ErrInvalid, path, err)
	}
	p.Lowered = p.lower()
	if err := p.Validate(); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	trustChecked(filepath.Dir(path), p)
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
	// Each writer has its own temporary file, synced before the rename,
	// so concurrent writers never tear the file, and a crash leaves the old
	// or the new one (RED-013, decision D114; now shared, D148).
	if err := fsutil.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	SaveChecked(filepath.Dir(path), p)
	return nil
}

// ReleaseAdded is what AddRelease did with a release.
type ReleaseAdded struct {
	Series hash.Hash `json:"series"`
	// Part is the release's part.
	Part uint64 `json:"part"`
	// Released is the newest part the policy now knows released.
	Released uint64 `json:"released"`
	// Mints is the number of serials that now exist, parts 1 to Released.
	Mints uint64 `json:"mints"`
	// Added is true when the release was newer than the one kept and
	// replaced it; false when the policy already knew that part released.
	Added bool `json:"added"`
	// Taken is true when node.AddRelease added the release to the series
	// the running node's engine runs; false when another call took it
	// first. It is not printed.
	Taken bool `json:"-"`
}

// ErrSeriesNotListed: a release names a series the policy does not list.
var ErrSeriesNotListed = errors.New("the node policy does not list that series (add it with poo init -series FILE)")

// CheckRelease checks r in full against the series p lists, changing
// nothing. It can take r.Part hashes the first time in a process, so a
// caller that holds a lock checks first, without it, and then adds: the
// part ids are shared in the process, so AddRelease after it costs no
// more hashing (RED-075).
func (p Policy) CheckRelease(r series.Release) error {
	return p.CheckReleaseContext(context.Background(), r)
}

// CheckReleaseContext is CheckRelease that stops when ctx is done
// (series.PartIDContext, GUI-028).
func (p Policy) CheckReleaseContext(ctx context.Context, r series.Release) error {
	for _, si := range p.Series {
		s, err := series.New(si.Params)
		if err != nil || s.ID() != r.Series {
			continue
		}
		return s.CheckReleaseContext(ctx, r)
	}
	return fmt.Errorf("%w: %s", ErrSeriesNotListed, r.Series.Short())
}

// AddRelease keeps r as the newest release of its variable series when it
// is valid and newer than the release kept (D166). A release is always
// checked in full here, also one of a part already known, so a forged
// release is never answered as fine (RED-074); only the peer path takes a
// known part without the hashing. A kept release that does not check is
// replaced by a valid one (RED-073). An invalid release is refused and
// nothing changes.
func (p *Policy) AddRelease(r series.Release) (ReleaseAdded, error) {
	for i, si := range p.Series {
		// A kept release that does not check is left out by Build, so a
		// valid r replaces it.
		s, _, err := si.Build()
		if err != nil || s.ID() != r.Series {
			continue
		}
		if err := s.CheckRelease(r); err != nil {
			return ReleaseAdded{}, err
		}
		added, err := s.AddRelease(r)
		if err != nil {
			return ReleaseAdded{}, err
		}
		if added {
			rel := r
			p.Series[i].Release = &rel
		}
		return ReleaseAdded{Series: s.ID(), Part: r.Part, Released: s.Released(), Mints: s.Mints(), Added: added}, nil
	}
	return ReleaseAdded{}, fmt.Errorf("%w: %s", ErrSeriesNotListed, r.Series.Short())
}
