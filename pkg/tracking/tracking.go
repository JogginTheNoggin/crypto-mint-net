// Package tracking records what a full node's peers do, so it can rank the
// useful ones and act against the ones that misbehave (the design's "Peer
// Tracking and Telemetry"). It keeps two scopes: offences node-wide, per
// address, as flags with a severity and an expiry; and activity per mint
// policy, as the measures behind a peer's points. A tracking template
// decides what is recorded: basic, standard, detailed, or a custom one.
//
// Everything here is node policy, and nothing it records is sent to other
// nodes. The package reads no clock: every call passes the time, so tests
// and the simulator can drive it.
package tracking

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// FileName is the tracker's file in a data directory.
const FileName = "tracking.json"

// Offence names an event with no honest explanation.
type Offence string

const (
	// InvalidItem: an entry that breaks a rule, or a layer below the
	// series' minimum strength.
	InvalidItem Offence = "invalid-item"
	// InvalidMessage: a message that does not decode, a request id that
	// does not match, a reply out of order.
	InvalidMessage Offence = "invalid-message"
	// OverBudget: more messages of one kind than the budget allows.
	OverBudget Offence = "over-budget"
	// RepeatFlood: far more repeats than an overflowing seen list explains.
	RepeatFlood Offence = "repeat-flood"
	// FalseBranch: a chase that ended in data that breaks the rules or
	// contradicts itself.
	FalseBranch Offence = "false-branch"
	// Contradiction: the peer contradicted what it said before.
	Contradiction Offence = "contradiction"
)

// Action is what an offence leads to at once.
type Action int

const (
	// None: a flag only.
	None Action = iota
	// TimeOut pauses the address.
	TimeOut
	// Block refuses the address.
	Block
)

func (a Action) String() string {
	switch a {
	case TimeOut:
		return "timeout"
	case Block:
		return "block"
	}
	return "none"
}

// MarshalText writes the action's name.
func (a Action) MarshalText() ([]byte, error) { return []byte(a.String()), nil }

// UnmarshalText reads an action's name.
func (a *Action) UnmarshalText(b []byte) error {
	for _, v := range []Action{None, TimeOut, Block} {
		if v.String() == string(b) {
			*a = v
			return nil
		}
	}
	return fmt.Errorf("tracking: unknown action %q", b)
}

// Rule is how one offence is treated.
type Rule struct {
	Severity float64       `json:"severity"`
	Expiry   time.Duration `json:"expiry"`
	Action   Action        `json:"action,omitempty"`
}

// Limits are flag totals at which something happens.
type Limits struct {
	// Priority: at or above it a peer may not be a priority peer.
	Priority float64
	// TimeOut: at or above it the address is timed out.
	TimeOut float64
	// Block: at or above it the address is blocked.
	Block float64
	// TimeOutFor and BlockFor are how long each lasts.
	TimeOutFor time.Duration
	BlockFor   time.Duration
	// BlockAfter timeouts within the flag history period turn the next
	// one into a block.
	BlockAfter int
}

// Windows are the window lengths of the windowed counters. They are
// node-wide, so changing a peer's template never reinterprets a counter.
type Windows struct {
	Budget      time.Duration
	Repeat      time.Duration
	Activity    time.Duration
	Chase       time.Duration
	Connections time.Duration
}

// Retention bounds how long records are kept.
type Retention struct {
	// FlagHistory: how long an expired flag is kept, so repeated offences
	// can be treated more harshly.
	FlagHistory time.Duration
	// Log and LogSize bound a detailed log, whichever comes first.
	Log     time.Duration
	LogSize int
	// MaxAddresses bounds the records kept; the oldest with no flag, block
	// or timeout are dropped first.
	MaxAddresses int
}

// Weights say how much each measure counts towards points.
type Weights struct {
	Pings    float64 `json:"pings"`
	RTT      float64 `json:"rtt"`
	First    float64 `json:"first_deliveries"`
	Requests float64 `json:"requests"`
	Reply    float64 `json:"reply_timing"`
	Chases   float64 `json:"chases"`
	Denials  float64 `json:"denials"`
	// Cost is per MiB served beyond what the peer gave back; Large per
	// large request.
	Cost  float64 `json:"cost"`
	Large float64 `json:"large"`
}

// Config is the tracking part of a node policy. Zero values mean the
// provisional defaults in DefaultConfig.
type Config struct {
	// Default is the template for every peer not chosen otherwise.
	Default string
	// Custom are the user's own templates.
	Custom map[string]Custom
	// Groups maps a peer group to a template. Groups: priority, backup,
	// candidate, unlisted, inbound, outbound.
	Groups map[string]string
	// Peers maps an address to its own template.
	Peers map[string]string
	// EscalateAt is a flag total at which a peer on a lighter template is
	// tracked with the standard one as well (0: never).
	EscalateAt float64
	// Offences are the rules per offence.
	Offences map[Offence]Rule
	Limits   Limits
	Windows  Windows
	Retain   Retention
	// Budgets are how many messages of each kind one address may send in
	// the budget window (0 or missing: no budget).
	Budgets map[string]int
	// RepeatLimit is how many repeats in the repeat window are normal.
	RepeatLimit int
	// ChaseBudget is how many chases one address may start in the chase
	// window.
	ChaseBudget int
	Weights     Weights
	// StartingPoints are what a newly met peer begins with.
	StartingPoints float64
	// NoTimeoutMisses stops pings scheduled during a timeout counting as
	// missed.
	NoTimeoutMisses bool
	// LoopbackAsOne tracks every loopback connection as one address.
	// Otherwise each loopback address and port is its own, like its own
	// network range.
	LoopbackAsOne bool
}

// Budget kinds.
const (
	KindTx        = "tx"
	KindEntry     = "entry"
	KindLayer     = "layer"
	KindRequest   = "request"
	KindDiscovery = "discovery"
	// KindRevert counts the selected entries a pushed entry makes the
	// node revert to check it against the state after its parent
	// (RED-011, decision D111).
	KindRevert = "revert"
)

// DefaultConfig returns the provisional defaults.
func DefaultConfig() Config {
	return Config{
		Default:    BasicName,
		EscalateAt: 5,
		Offences: map[Offence]Rule{
			InvalidItem:    {Severity: 5, Expiry: 24 * time.Hour},
			InvalidMessage: {Severity: 5, Expiry: 24 * time.Hour},
			OverBudget:     {Severity: 1, Expiry: time.Hour},
			RepeatFlood:    {Severity: 2, Expiry: time.Hour},
			FalseBranch:    {Severity: 5, Expiry: 24 * time.Hour},
			Contradiction:  {Severity: 3, Expiry: 24 * time.Hour},
		},
		Limits: Limits{Priority: 5, TimeOut: 10, Block: 30, TimeOutFor: 10 * time.Minute, BlockFor: 24 * time.Hour, BlockAfter: 3},
		Windows: Windows{
			Budget: time.Minute, Repeat: time.Minute, Activity: time.Hour,
			Chase: 10 * time.Minute, Connections: 10 * time.Minute,
		},
		Retain: Retention{FlagHistory: 7 * 24 * time.Hour, Log: time.Hour, LogSize: 256, MaxAddresses: 8192},
		Budgets: map[string]int{
			KindTx: 1200, KindEntry: 1200, KindLayer: 6000, KindRequest: 600, KindDiscovery: 120,
			KindRevert: 200_000,
		},
		RepeatLimit: 600,
		ChaseBudget: 30,
		Weights: Weights{
			Pings: 2, RTT: 1, First: 1, Requests: 2, Reply: 1, Chases: 2, Denials: 2,
			Cost: 0.1, Large: 0.5,
		},
	}
}

func (c *Config) fill() {
	d := DefaultConfig()
	if c.Default == "" {
		c.Default = d.Default
	}
	if c.Offences == nil {
		c.Offences = map[Offence]Rule{}
	}
	for o, r := range d.Offences {
		if _, ok := c.Offences[o]; !ok {
			c.Offences[o] = r
		}
	}
	if c.Limits == (Limits{}) {
		c.Limits = d.Limits
	}
	if c.Windows == (Windows{}) {
		c.Windows = d.Windows
	}
	fillDur := func(v *time.Duration, def time.Duration) {
		if *v <= 0 {
			*v = def
		}
	}
	fillDur(&c.Windows.Budget, d.Windows.Budget)
	fillDur(&c.Windows.Repeat, d.Windows.Repeat)
	fillDur(&c.Windows.Activity, d.Windows.Activity)
	fillDur(&c.Windows.Chase, d.Windows.Chase)
	fillDur(&c.Windows.Connections, d.Windows.Connections)
	fillDur(&c.Limits.TimeOutFor, d.Limits.TimeOutFor)
	fillDur(&c.Limits.BlockFor, d.Limits.BlockFor)
	fillDur(&c.Retain.FlagHistory, d.Retain.FlagHistory)
	fillDur(&c.Retain.Log, d.Retain.Log)
	if c.Retain.LogSize <= 0 {
		c.Retain.LogSize = d.Retain.LogSize
	}
	if c.Retain.MaxAddresses <= 0 {
		c.Retain.MaxAddresses = d.Retain.MaxAddresses
	}
	if c.Budgets == nil {
		c.Budgets = d.Budgets
	}
	if c.RepeatLimit == 0 {
		c.RepeatLimit = d.RepeatLimit
	}
	if c.ChaseBudget == 0 {
		c.ChaseBudget = d.ChaseBudget
	}
	if c.Weights == (Weights{}) {
		c.Weights = d.Weights
	}
}

// Flag is one offence against an address.
type Flag struct {
	Offence  Offence   `json:"offence"`
	Severity float64   `json:"severity"`
	At       time.Time `json:"at"`
	Expires  time.Time `json:"expires"`
	Detail   string    `json:"detail,omitempty"`
}

// Event is one line of a detailed log.
type Event struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Section string    `json:"section,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

// TrafficCount counts one message type.
type TrafficCount struct {
	Messages uint64 `json:"messages"`
	Bytes    uint64 `json:"bytes"`
}

// Measures are one peer's activity for one mint policy.
type Measures struct {
	LastHeard time.Time `json:"last_heard,omitempty"`

	PingsSent Counter `json:"pings_sent"`
	Pongs     Counter `json:"pongs"`
	Missed    Counter `json:"missed"`
	RTT       Average `json:"rtt"`

	First Counter `json:"first"`

	ReqSent     Counter `json:"req_sent"`
	ReqAnswered Counter `json:"req_answered"`
	ReqTimedOut Counter `json:"req_timed_out"`
	ReqRefused  Counter `json:"req_refused"`
	ReqNotHeld  Counter `json:"req_not_held"`
	ReplyTime   Average `json:"reply_time"`
	Throughput  Average `json:"throughput"`

	Items map[string]*Counter `json:"items,omitempty"`

	Highest string `json:"highest,omitempty"`

	ChaseStarted   Counter `json:"chase_started"`
	ChaseConfirmed Counter `json:"chase_confirmed"`
	ChaseNowhere   Counter `json:"chase_nowhere"`

	Denials Counter `json:"denials"`

	Served   Counter `json:"served"`
	Received Counter `json:"received"`
	Large    Counter `json:"large"`

	Start float64 `json:"start"`
}

type record struct {
	Flags    []Flag      `json:"flags,omitempty"`
	History  []Flag      `json:"history,omitempty"`
	Timeouts []time.Time `json:"timeouts,omitempty"`

	TimedOutUntil time.Time `json:"timed_out_until,omitempty"`
	BlockedUntil  time.Time `json:"blocked_until,omitempty"`
	// Held are the categories, by mint policy, the peer had when it was
	// timed out, so it can return to them.
	Held map[string]string `json:"held,omitempty"`

	Template string   `json:"template,omitempty"`
	Groups   []string `json:"-"`

	Budget      map[string]*Counter `json:"budget,omitempty"`
	OverFlagged map[string]int64    `json:"over_flagged,omitempty"`
	Repeats     Counter             `json:"repeats"`
	RepeatFlag  int64               `json:"repeat_flagged,omitempty"`
	Chases      Counter             `json:"chases"`
	ChaseFlag   int64               `json:"chase_flagged,omitempty"`
	Conns       map[string]*Counter `json:"conns,omitempty"`

	Log     []Event                  `json:"log,omitempty"`
	Traffic map[string]*TrafficCount `json:"traffic,omitempty"`

	Seen     time.Time            `json:"seen"`
	Policies map[string]*Measures `json:"policies,omitempty"`
}

// Tracker holds every record. It is safe for concurrent use.
type Tracker struct {
	mu     sync.Mutex
	cfg    Config
	tpl    Templates
	path   string
	recs   map[string]*record
	ranges map[string]time.Time // blocked network ranges
	dirty  bool
	// trimAt, when above MaxAddresses, is the record count the next trim
	// waits for: set when a trim could not get under the bound because
	// the records it must keep (flagged, blocked, timed out) are most of
	// them. Without it every new address scanned and sorted every record,
	// so taking in 65,536 blocks took 53 s (ORANGE-031).
	trimAt int
}

type file struct {
	Format    int                  `json:"format"`
	Addresses map[string]*record   `json:"addresses"`
	Ranges    map[string]time.Time `json:"ranges,omitempty"`
}

// ErrUnknownTemplate is returned for a template name that is not known.
var ErrUnknownTemplate = errors.New("tracking: unknown template")

// GroupNames are the peer groups a template can be chosen for.
var GroupNames = []string{"priority", "backup", "candidate", "unlisted", "inbound", "outbound"}

func isGroup(g string) bool {
	for _, n := range GroupNames {
		if n == g {
			return true
		}
	}
	return false
}

// New loads the tracker at path, or starts an empty one. An empty path
// keeps records in memory only.
func New(cfg Config, path string) (*Tracker, error) {
	cfg.fill()
	tpl, err := NewTemplates(cfg.Custom)
	if err != nil {
		return nil, err
	}
	for _, n := range append([]string{cfg.Default}, values(cfg.Groups, cfg.Peers)...) {
		if _, ok := tpl.Get(n); !ok {
			return nil, fmt.Errorf("%w %q", ErrUnknownTemplate, n)
		}
	}
	for g := range cfg.Groups {
		if !isGroup(g) {
			return nil, fmt.Errorf("tracking: unknown peer group %q", g)
		}
	}
	t := &Tracker{cfg: cfg, tpl: tpl, path: path, recs: map[string]*record{}, ranges: map[string]time.Time{}}
	if path == "" {
		return t, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("tracking: %s: %w", path, err)
	}
	for k, r := range f.Addresses {
		if r != nil {
			t.recs[k] = r
		}
	}
	for k, u := range f.Ranges {
		t.ranges[k] = u
	}
	return t, nil
}

func values(ms ...map[string]string) []string {
	var out []string
	for _, m := range ms {
		for _, v := range m {
			out = append(out, v)
		}
	}
	return out
}

// Config returns the settings in use, defaults filled in.
func (t *Tracker) Config() Config { return t.cfg }

// Templates lists the known template names.
func (t *Tracker) Templates() []string { return t.tpl.Names() }

// Save writes the records if they changed.
func (t *Tracker) Save() error {
	t.mu.Lock()
	if !t.dirty || t.path == "" {
		t.mu.Unlock()
		return nil
	}
	data, err := json.Marshal(file{Format: 1, Addresses: t.recs, Ranges: t.ranges})
	t.dirty = false
	t.mu.Unlock()
	if err != nil {
		return err
	}
	return fsutil.WriteFile(t.path, append(data, '\n'), 0o600) // RED-041, D148
}

// Key is the address a peer is tracked by: the IP of a connection or
// contact address, never its port, except for loopback (see
// Config.LoopbackAsOne). A host name is kept as it is.
func (t *Tracker) Key(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return strings.ToLower(host)
	case ip.IsLoopback() && !t.cfg.LoopbackAsOne && err == nil:
		return addr
	}
	return ip.String()
}

// get returns the record for key, making one. It trims expired flags.
func (t *Tracker) get(key string, now time.Time) *record {
	r := t.recs[key]
	if r == nil {
		if len(t.recs) >= max(t.cfg.Retain.MaxAddresses, t.trimAt) {
			t.trimLocked(now)
		}
		r = &record{Seen: now}
		t.recs[key] = r
		t.dirty = true
	}
	t.expireLocked(r, now)
	return r
}

func (t *Tracker) expireLocked(r *record, now time.Time) {
	keep := r.Flags[:0]
	for _, f := range r.Flags {
		if now.Before(f.Expires) {
			keep = append(keep, f)
		} else {
			r.History = append(r.History, f)
			t.dirty = true
		}
	}
	r.Flags = keep
	cut := 0
	for cut < len(r.History) && now.Sub(r.History[cut].Expires) > t.cfg.Retain.FlagHistory {
		cut++
	}
	if cut > 0 {
		r.History = append([]Flag(nil), r.History[cut:]...)
		t.dirty = true
	}
	cut = 0
	for cut < len(r.Timeouts) && now.Sub(r.Timeouts[cut]) > t.cfg.Retain.FlagHistory {
		cut++
	}
	if cut > 0 {
		r.Timeouts = append([]time.Time(nil), r.Timeouts[cut:]...)
	}
}

// trimLocked drops the oldest records that hold nothing that must stay.
func (t *Tracker) trimLocked(now time.Time) {
	type kv struct {
		k    string
		seen time.Time
	}
	var drop []kv
	for k, r := range t.recs {
		if len(r.Flags) == 0 && !now.Before(r.BlockedUntil) && !now.Before(r.TimedOutUntil) {
			drop = append(drop, kv{k, r.Seen})
		}
	}
	sort.Slice(drop, func(i, j int) bool { return drop[i].seen.Before(drop[j].seen) })
	n := len(t.recs) - t.cfg.Retain.MaxAddresses + 1 + t.cfg.Retain.MaxAddresses/16
	for i := 0; i < n && i < len(drop); i++ {
		delete(t.recs, drop[i].k)
	}
	t.trimAt = 0
	if len(t.recs) >= t.cfg.Retain.MaxAddresses {
		// What must stay fills the bound: trim again only after as many
		// new records as a trim leaves room for.
		t.trimAt = len(t.recs) + 1 + t.cfg.Retain.MaxAddresses/16
	}
	t.dirty = true
}

// settingsLocked resolves what to record for r: its own template, else
// every setting of the groups it is in, else the default; with the
// standard template added once its flag total passes EscalateAt.
func (t *Tracker) settingsLocked(key string, r *record) Setting {
	var s Setting
	name := r.Template
	if name == "" {
		name = t.cfg.Peers[key]
	}
	if name != "" {
		s, _ = t.tpl.Get(name)
	} else {
		for _, g := range r.Groups {
			if n, ok := t.cfg.Groups[g]; ok {
				gs, _ := t.tpl.Get(n)
				s |= gs
			}
		}
		if s == 0 {
			s, _ = t.tpl.Get(t.cfg.Default)
		}
	}
	if t.cfg.EscalateAt > 0 && total(r.Flags) >= t.cfg.EscalateAt {
		s |= Standard
	}
	return s
}

func total(fs []Flag) float64 {
	n := 0.0
	for _, f := range fs {
		n += f.Severity
	}
	return n
}

func (t *Tracker) logLocked(r *record, s Setting, now time.Time, kind, section, detail string) {
	if !s.Has(Log) {
		return
	}
	r.Log = append(r.Log, Event{At: now, Kind: kind, Section: section, Detail: detail})
	cut := 0
	for cut < len(r.Log) && (len(r.Log)-cut > t.cfg.Retain.LogSize || now.Sub(r.Log[cut].At) > t.cfg.Retain.Log) {
		cut++
	}
	if cut > 0 {
		r.Log = append([]Event(nil), r.Log[cut:]...)
	}
}

// Status is the node-wide standing of an address.
type Status struct {
	FlagTotal     float64   `json:"flag_total"`
	TimedOutUntil time.Time `json:"timed_out_until,omitempty"`
	BlockedUntil  time.Time `json:"blocked_until,omitempty"`
	// TimedOut and Blocked hold at the time asked.
	TimedOut bool `json:"timed_out,omitempty"`
	Blocked  bool `json:"blocked,omitempty"`
	// Priority reports whether the flag total allows a priority category.
	Priority bool `json:"priority"`
}

// Allowed reports whether the address may be connected to.
func (s Status) Allowed() bool { return !s.TimedOut && !s.Blocked }

// Status returns the standing of addr, judged also by its network range.
func (t *Tracker) Status(addr string, now time.Time) Status {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	var st Status
	if r := t.recs[key]; r != nil {
		t.expireLocked(r, now)
		st.FlagTotal = total(r.Flags)
		st.TimedOutUntil, st.BlockedUntil = r.TimedOutUntil, r.BlockedUntil
	}
	if u, ok := t.ranges[peers.Range(addr)]; ok && u.After(st.BlockedUntil) {
		st.BlockedUntil = u
	}
	st.TimedOut = now.Before(st.TimedOutUntil)
	st.Blocked = now.Before(st.BlockedUntil)
	st.Priority = st.FlagTotal < t.cfg.Limits.Priority
	return st
}

// Offend records an offence of addr and returns the action it led to:
// the offence's own action, or a timeout or block when the flag total
// passes a limit. A peer whose template does not record offences gets no
// flag, and nothing happens.
func (t *Tracker) Offend(addr string, o Offence, detail string, now time.Time) Action {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.offendLocked(key, o, detail, now)
}

func (t *Tracker) offendLocked(key string, o Offence, detail string, now time.Time) Action {
	r := t.get(key, now)
	s := t.settingsLocked(key, r)
	if !s.Has(Offences) {
		return None
	}
	rule := t.cfg.Offences[o]
	r.Flags = append(r.Flags, Flag{Offence: o, Severity: rule.Severity, At: now, Expires: now.Add(rule.Expiry), Detail: detail})
	r.Seen = now
	t.dirty = true
	t.logLocked(r, s, now, "flag", "", string(o)+": "+detail)
	act := rule.Action
	tot := total(r.Flags)
	switch {
	case tot >= t.cfg.Limits.Block:
		act = Block
	case tot >= t.cfg.Limits.TimeOut && act == None:
		act = TimeOut
	}
	if act == TimeOut && len(r.Timeouts) >= t.cfg.Limits.BlockAfter && t.cfg.Limits.BlockAfter > 0 {
		act = Block // it keeps being timed out
	}
	switch act {
	case TimeOut:
		if now.Before(r.TimedOutUntil) {
			return TimeOut // already paused; the pause is not stretched
		}
		r.TimedOutUntil = now.Add(t.cfg.Limits.TimeOutFor)
		r.Timeouts = append(r.Timeouts, now)
	case Block:
		r.BlockedUntil = now.Add(t.cfg.Limits.BlockFor)
		r.Held = nil
	}
	return act
}

// TimeOutFor pauses addr by hand for d (0: the configured length).
func (t *Tracker) TimeOutFor(addr string, d time.Duration, now time.Time) {
	if d <= 0 {
		d = t.cfg.Limits.TimeOutFor
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(t.Key(addr), now)
	r.TimedOutUntil = now.Add(d)
	r.Timeouts = append(r.Timeouts, now)
	t.dirty = true
}

// BlockFor refuses addr for d (0: the configured length; negative: until
// lifted by hand, in practice a hundred years).
func (t *Tracker) BlockFor(addr string, d time.Duration, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(t.Key(addr), now)
	r.BlockedUntil = now.Add(t.blockLen(d))
	r.Held = nil
	t.dirty = true
}

func (t *Tracker) blockLen(d time.Duration) time.Duration {
	switch {
	case d == 0:
		return t.cfg.Limits.BlockFor
	case d < 0:
		return 100 * 365 * 24 * time.Hour
	}
	return d
}

// BlockRange refuses the whole network range of addr (see peers.Range).
func (t *Tracker) BlockRange(addr string, d time.Duration, now time.Time) string {
	rng := peers.Range(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ranges[rng] = now.Add(t.blockLen(d))
	t.dirty = true
	return rng
}

// Lift ends any timeout or block of addr and of its range. A blocked
// address returns only as a candidate; that is up to the caller.
func (t *Tracker) Lift(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r := t.recs[t.Key(addr)]; r != nil {
		r.TimedOutUntil, r.BlockedUntil = time.Time{}, time.Time{}
	}
	delete(t.ranges, peers.Range(addr))
	t.dirty = true
}

// Hold remembers the categories addr had, by mint policy text, when it was
// timed out.
func (t *Tracker) Hold(addr string, held map[string]string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(t.Key(addr), now)
	r.Held = held
	t.dirty = true
}

// Held returns and forgets the categories remembered for addr, and counts
// the pings missed while it was timed out, one per ping period, unless
// NoTimeoutMisses is set.
func (t *Tracker) Held(addr string, pingEvery time.Duration, now time.Time) map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.recs[t.Key(addr)]
	if r == nil || r.Held == nil {
		return nil
	}
	held := r.Held
	r.Held = nil
	t.dirty = true
	if !t.cfg.NoTimeoutMisses && pingEvery > 0 && len(r.Timeouts) > 0 {
		paused := r.TimedOutUntil.Sub(r.Timeouts[len(r.Timeouts)-1])
		n := uint64(paused / pingEvery)
		for s := range held {
			if m := r.Policies[s]; m != nil && n > 0 {
				m.PingsSent.Add(now, t.cfg.Windows.Activity, n)
				m.Missed.Add(now, t.cfg.Windows.Activity, n)
			}
		}
	}
	return held
}

// Clear forgets records of addr by hand: "flags", "measures", or "all".
// A block or timeout stays unless all is cleared; even then a block on its
// network range stays.
func (t *Tracker) Clear(addr, what string) error {
	switch what {
	case "flags", "measures", "all":
	default:
		return fmt.Errorf("tracking: clear %q: want flags, measures or all", what)
	}
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.recs[key]
	if r == nil {
		return nil
	}
	switch what {
	case "flags":
		r.Flags, r.History, r.Timeouts = nil, nil, nil
	case "measures":
		r.Policies = nil
	case "all":
		delete(t.recs, key)
	}
	t.dirty = true
	return nil
}

// Forget drops what is kept about addr for one mint policy, as when the
// peer is deleted. Node-wide records stay.
func (t *Tracker) Forget(addr, section string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r := t.recs[t.Key(addr)]; r != nil && r.Policies != nil {
		delete(r.Policies, section)
		t.dirty = true
	}
}

// SetGroups sets the peer groups addr belongs to, for choosing its template.
func (t *Tracker) SetGroups(addr string, groups []string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(t.Key(addr), now)
	r.Groups = append([]string(nil), groups...)
}

// SetTemplate chooses addr's own template; "" goes back to its groups and
// the default.
func (t *Tracker) SetTemplate(addr, name string, now time.Time) error {
	if name != "" {
		if _, ok := t.tpl.Get(name); !ok {
			return fmt.Errorf("%w %q", ErrUnknownTemplate, name)
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(t.Key(addr), now)
	r.Template = name
	t.dirty = true
	return nil
}

// Settings returns what is recorded for addr.
func (t *Tracker) Settings(addr string, now time.Time) Setting {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.settingsLocked(key, t.get(key, now))
}

// Budget counts one message of kind from addr and reports whether it is
// within the budget. Past it, the message should be dropped; the first
// one past it in each window adds an over-budget flag, whose action is
// returned.
func (t *Tracker) Budget(addr, kind string, now time.Time) (bool, Action) {
	return t.Spend(addr, kind, 1, now)
}

// Spend is Budget for a message that costs n units of kind.
func (t *Tracker) Spend(addr, kind string, n uint64, now time.Time) (bool, Action) {
	limit := t.cfg.Budgets[kind]
	if limit <= 0 || n == 0 {
		return true, None
	}
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	if !t.settingsLocked(key, r).Has(Budgets) {
		return true, None
	}
	if r.Budget == nil {
		r.Budget = map[string]*Counter{}
	}
	c := r.Budget[kind]
	if c == nil {
		c = &Counter{}
		r.Budget[kind] = c
	}
	w := t.cfg.Windows.Budget
	// One spend larger than the whole budget passes when nothing else was
	// spent in the window, so no single item is too costly to take from
	// any peer at all (consensus audit P2, D116).
	// Only what is taken is counted: a message dropped over the budget
	// adds nothing, so a steady sender gets through at the budget's rate
	// and is never starved for good (ORANGE-016, Black's B1 note; D121).
	if sum := c.Sum(now, w); sum == 0 || sum+n <= uint64(limit) {
		c.Add(now, w, n)
		return true, None
	}
	e := epoch(now, w) / slots
	if r.OverFlagged == nil {
		r.OverFlagged = map[string]int64{}
	}
	if r.OverFlagged[kind] == e+1 {
		return false, None
	}
	r.OverFlagged[kind] = e + 1
	return false, t.offendLocked(key, OverBudget, kind, now)
}

// Repeat counts one repeated request or item from addr. Past the repeat
// limit in a window it adds a flag, once per window, and returns its
// action.
func (t *Tracker) Repeat(addr string, now time.Time) Action {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	if !t.settingsLocked(key, r).Has(Repeats) {
		return None
	}
	w := t.cfg.Windows.Repeat
	r.Repeats.Add(now, w, 1)
	if r.Repeats.Sum(now, w) <= uint64(t.cfg.RepeatLimit) {
		return None
	}
	e := epoch(now, w)/slots + 1
	if r.RepeatFlag == e {
		return None
	}
	r.RepeatFlag = e
	return t.offendLocked(key, RepeatFlood, "", now)
}

// StartChase counts a chase started by addr's stated weight against its
// budget. Past it the stated weight should be dropped; the first one past
// it in each window adds an over-budget flag.
func (t *Tracker) StartChase(addr string, now time.Time) (bool, Action) {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	if !t.settingsLocked(key, r).Has(Chases) {
		return true, None
	}
	w := t.cfg.Windows.Chase
	r.Chases.Add(now, w, 1)
	if r.Chases.Sum(now, w) <= uint64(t.cfg.ChaseBudget) {
		return true, None
	}
	e := epoch(now, w)/slots + 1
	if r.ChaseFlag == e {
		return false, None
	}
	r.ChaseFlag = e
	return false, t.offendLocked(key, OverBudget, "stated weights", now)
}

// Connection counts a connection event: "opened", "closed" or "refused".
func (t *Tracker) Connection(addr, what string, now time.Time) {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	s := t.settingsLocked(key, r)
	t.logLocked(r, s, now, "connection", "", what)
	if !s.Has(Connections) {
		return
	}
	if r.Conns == nil {
		r.Conns = map[string]*Counter{}
	}
	c := r.Conns[what]
	if c == nil {
		c = &Counter{}
		r.Conns[what] = c
	}
	c.Add(now, t.cfg.Windows.Connections, 1)
	t.dirty = true
}

// Message records traffic of one message type from or to addr, for the
// detailed template.
func (t *Tracker) Message(addr, typ string, bytes int, now time.Time) {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	if !t.settingsLocked(key, r).Has(Traffic) {
		return
	}
	if r.Traffic == nil {
		r.Traffic = map[string]*TrafficCount{}
	}
	tr := r.Traffic[typ]
	if tr == nil {
		tr = &TrafficCount{}
		r.Traffic[typ] = tr
	}
	tr.Messages++
	tr.Bytes += uint64(bytes)
}

// measuresLocked returns addr's measures for a mint policy, making them.
func (t *Tracker) measuresLocked(r *record, section string) *Measures {
	if r.Policies == nil {
		r.Policies = map[string]*Measures{}
	}
	m := r.Policies[section]
	if m == nil {
		m = &Measures{Start: t.cfg.StartingPoints}
		r.Policies[section] = m
	}
	return m
}

// activity runs f on addr's measures for section with its settings.
func (t *Tracker) activity(addr, section string, now time.Time, kind, detail string, f func(*Measures, Setting, time.Duration)) {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	r.Seen = now
	s := t.settingsLocked(key, r)
	m := t.measuresLocked(r, section)
	f(m, s, t.cfg.Windows.Activity)
	t.logLocked(r, s, now, kind, section, detail)
	t.dirty = true
}

// Heard notes that addr was heard from for section.
func (t *Tracker) Heard(addr, section string, now time.Time) {
	t.activity(addr, section, now, "heard", "", func(m *Measures, s Setting, w time.Duration) {
		if s.Has(LastHeard) {
			m.LastHeard = now
		}
	})
}

// Ping records a ping sent to addr for section, and whether it was
// answered and how fast.
func (t *Tracker) Ping(addr, section string, answered bool, rtt time.Duration, now time.Time) {
	t.activity(addr, section, now, "ping", fmt.Sprintf("answered %v in %v", answered, rtt), func(m *Measures, s Setting, w time.Duration) {
		if s.Has(Pings) {
			m.PingsSent.Add(now, w, 1)
			if answered {
				m.Pongs.Add(now, w, 1)
			} else {
				m.Missed.Add(now, w, 1)
			}
		}
		if answered && s.Has(RTT) {
			m.RTT.Add(rtt.Seconds())
		}
		if answered && s.Has(LastHeard) {
			m.LastHeard = now
		}
	})
}

// FirstDelivery credits addr with a new valid item of kind, sent before
// any other peer sent it.
func (t *Tracker) FirstDelivery(addr, section, kind string, now time.Time) {
	t.activity(addr, section, now, "first", kind, func(m *Measures, s Setting, w time.Duration) {
		if s.Has(FirstDeliveries) {
			m.First.Add(now, w, 1)
		}
		t.itemLocked(m, s, kind, now, w)
	})
}

// Item records a valid item of kind received from addr that was not new.
func (t *Tracker) Item(addr, section, kind string, now time.Time) {
	t.activity(addr, section, now, "item", kind, func(m *Measures, s Setting, w time.Duration) {
		t.itemLocked(m, s, kind, now, w)
	})
}

func (t *Tracker) itemLocked(m *Measures, s Setting, kind string, now time.Time, w time.Duration) {
	if s.Has(LastHeard) {
		m.LastHeard = now
	}
	if !s.Has(Items) {
		return
	}
	if m.Items == nil {
		m.Items = map[string]*Counter{}
	}
	c := m.Items[kind]
	if c == nil {
		c = &Counter{}
		m.Items[kind] = c
	}
	c.Add(now, w, 1)
}

// Outcome is how a request ended.
type Outcome int

const (
	Answered Outcome = iota
	Refused
	NotHeld
	TimedOut
)

func (o Outcome) String() string {
	return [...]string{"answered", "refused", "not-held", "timed-out"}[o]
}

// Request records a request sent to addr and how it ended: the time to the
// whole reply and its size.
func (t *Tracker) Request(addr, section string, o Outcome, took time.Duration, bytes int, now time.Time) {
	t.activity(addr, section, now, "request", o.String(), func(m *Measures, s Setting, w time.Duration) {
		if s.Has(Requests) {
			m.ReqSent.Add(now, w, 1)
			switch o {
			case Answered, Refused, NotHeld:
				m.ReqAnswered.Add(now, w, 1)
			case TimedOut:
				m.ReqTimedOut.Add(now, w, 1)
			}
		}
		if s.Has(RequestOutcomes) {
			switch o {
			case Refused:
				m.ReqRefused.Add(now, w, 1)
			case NotHeld:
				m.ReqNotHeld.Add(now, w, 1)
			}
		}
		if o == Answered && s.Has(ReplyTiming) {
			m.ReplyTime.Add(took.Seconds())
			if took > 0 {
				m.Throughput.Add(float64(bytes) / took.Seconds())
			}
		}
		if o != TimedOut && s.Has(LastHeard) {
			m.LastHeard = now
		}
	})
}

// Stated records a stated weight from addr for section, keeping the
// highest.
func (t *Tracker) Stated(addr, section string, w weight.Weight, now time.Time) {
	t.activity(addr, section, now, "stated", w.String(), func(m *Measures, s Setting, _ time.Duration) {
		if !s.Has(StatedWeights) {
			return
		}
		var old weight.Weight
		if m.Highest != "" {
			old.UnmarshalText([]byte(m.Highest))
		}
		if w.Greater(old) {
			m.Highest = w.String()
		}
	})
}

// ChaseOutcome records how a chase started by addr's stated weight ended:
// confirmed (a heavier ledger) or led nowhere.
func (t *Tracker) ChaseOutcome(addr, section string, confirmed bool, now time.Time) {
	t.activity(addr, section, now, "chase", fmt.Sprintf("confirmed %v", confirmed), func(m *Measures, s Setting, w time.Duration) {
		if !s.Has(ChaseOutcomes) {
			return
		}
		m.ChaseStarted.Add(now, w, 1)
		if confirmed {
			m.ChaseConfirmed.Add(now, w, 1)
		} else {
			m.ChaseNowhere.Add(now, w, 1)
		}
	})
}

// Denial records that addr, asked as a priority peer, said it saw nothing
// heavier when a heavier ledger existed.
func (t *Tracker) Denial(addr, section string, now time.Time) {
	t.activity(addr, section, now, "denial", "", func(m *Measures, s Setting, w time.Duration) {
		if s.Has(Denials) {
			m.Denials.Add(now, w, 1)
		}
	})
}

// Denials returns addr's denials for section within the activity window.
func (t *Tracker) Denials(addr, section string, now time.Time) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.recs[t.Key(addr)]
	if r == nil || r.Policies[section] == nil {
		return 0
	}
	return r.Policies[section].Denials.Sum(now, t.cfg.Windows.Activity)
}

// Cost records bytes served to addr and received from it for section, and
// whether its request was a large one, such as a whole ledger.
func (t *Tracker) Cost(addr, section string, served, received int, large bool, now time.Time) {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.get(key, now)
	s := t.settingsLocked(key, r)
	if !s.Has(Cost) {
		return
	}
	m := t.measuresLocked(r, section)
	w := t.cfg.Windows.Activity
	if served > 0 {
		m.Served.Add(now, w, uint64(served))
	}
	if received > 0 {
		m.Received.Add(now, w, uint64(received))
	}
	if large {
		m.Large.Add(now, w, 1)
	}
	t.dirty = true
}

// Points returns addr's points for section, counting only the measures in
// only (0: all) that its template records. Points come only from activity;
// flags never subtract them.
func (t *Tracker) Points(addr, section string, only Setting, now time.Time) float64 {
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.recs[key]
	if r == nil {
		return t.cfg.StartingPoints
	}
	m := r.Policies[section]
	if m == nil {
		return t.cfg.StartingPoints
	}
	s := t.settingsLocked(key, r)
	if only != 0 {
		s &= only
	}
	return t.points(m, s, now)
}

func ratio(a, b uint64) float64 {
	if a+b == 0 {
		return 0
	}
	return float64(a) / float64(a+b)
}

func (t *Tracker) points(m *Measures, s Setting, now time.Time) float64 {
	w := t.cfg.Windows.Activity
	k := t.cfg.Weights
	p := m.Start
	if s.Has(Pings) {
		p += k.Pings * ratio(m.Pongs.Sum(now, w), m.Missed.Sum(now, w))
	}
	if s.Has(RTT) && m.RTT.N > 0 {
		p += k.RTT / (1 + m.RTT.Value)
	}
	if s.Has(FirstDeliveries) {
		p += k.First * math.Log2(1+float64(m.First.Sum(now, w)))
	}
	if s.Has(Requests) {
		p += k.Requests * ratio(m.ReqAnswered.Sum(now, w), m.ReqTimedOut.Sum(now, w))
	}
	if s.Has(ReplyTiming) && m.ReplyTime.N > 0 {
		p += k.Reply / (1 + m.ReplyTime.Value)
	}
	if s.Has(ChaseOutcomes) {
		c, n := m.ChaseConfirmed.Sum(now, w), m.ChaseNowhere.Sum(now, w)
		if c+n > 0 {
			p += k.Chases * (float64(c) - float64(n)) / float64(c+n)
		}
	}
	if s.Has(Denials) {
		p -= k.Denials * float64(m.Denials.Sum(now, w))
	}
	if s.Has(Cost) {
		served, recv := m.Served.Sum(now, w), m.Received.Sum(now, w)
		if served > recv {
			p -= k.Cost * float64(served-recv) / (1 << 20)
		}
		p -= k.Large * float64(m.Large.Sum(now, w))
	}
	return p
}

// LastHeard returns when addr was last heard from for section.
func (t *Tracker) LastHeard(addr, section string) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.recs[t.Key(addr)]
	if r == nil || r.Policies[section] == nil {
		return time.Time{}
	}
	return r.Policies[section].LastHeard
}

// PolicyReport summarizes one peer's measures for one mint policy.
type PolicyReport struct {
	Points          float64   `json:"points"`
	LastHeard       time.Time `json:"last_heard,omitempty"`
	Pongs           uint64    `json:"pongs"`
	Missed          uint64    `json:"missed"`
	RTTMillis       float64   `json:"rtt_ms,omitempty"`
	FirstDeliveries uint64    `json:"first_deliveries"`
	Answered        uint64    `json:"answered"`
	TimedOut        uint64    `json:"timed_out"`
	ChasesConfirmed uint64    `json:"chases_confirmed"`
	ChasesNowhere   uint64    `json:"chases_nowhere"`
	Denials         uint64    `json:"denials"`
	Served          uint64    `json:"bytes_served"`
	Received        uint64    `json:"bytes_received"`
	Large           uint64    `json:"large_requests"`
	Highest         string    `json:"highest_stated,omitempty"`
}

// Report summarizes everything kept about one address.
type Report struct {
	Key      string                  `json:"key"`
	Status   Status                  `json:"status"`
	Template string                  `json:"template"`
	Settings []string                `json:"settings"`
	Flags    []Flag                  `json:"flags,omitempty"`
	History  int                     `json:"history,omitempty"`
	Policies map[string]PolicyReport `json:"policies,omitempty"`
	Log      []Event                 `json:"log,omitempty"`
}

// Report describes addr. Its template is "" when the default or a group
// applies.
func (t *Tracker) Report(addr string, now time.Time) (Report, bool) {
	st := t.Status(addr, now)
	key := t.Key(addr)
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.recs[key]
	if r == nil {
		return Report{Key: key, Status: st}, false
	}
	s := t.settingsLocked(key, r)
	rep := Report{
		Key: key, Status: st, Template: r.Template, Settings: s.Names(),
		Flags: append([]Flag(nil), r.Flags...), History: len(r.History),
		Log: append([]Event(nil), r.Log...),
	}
	if len(r.Policies) > 0 {
		rep.Policies = map[string]PolicyReport{}
	}
	w := t.cfg.Windows.Activity
	for sec, m := range r.Policies {
		pr := PolicyReport{
			Points: t.points(m, s, now), LastHeard: m.LastHeard,
			Pongs: m.Pongs.Sum(now, w), Missed: m.Missed.Sum(now, w),
			FirstDeliveries: m.First.Sum(now, w),
			Answered:        m.ReqAnswered.Sum(now, w), TimedOut: m.ReqTimedOut.Sum(now, w),
			ChasesConfirmed: m.ChaseConfirmed.Sum(now, w), ChasesNowhere: m.ChaseNowhere.Sum(now, w),
			Denials: m.Denials.Sum(now, w), Highest: m.Highest,
			Served: m.Served.Sum(now, w), Received: m.Received.Sum(now, w), Large: m.Large.Sum(now, w),
		}
		if m.RTT.N > 0 {
			pr.RTTMillis = m.RTT.Value * 1000
		}
		rep.Policies[sec] = pr
	}
	return rep, true
}

// Keys lists every tracked address, sorted.
func (t *Tracker) Keys() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.recs))
	for k := range t.recs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Ranges lists the blocked network ranges with when each block ends.
func (t *Tracker) Ranges() map[string]time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]time.Time, len(t.ranges))
	for k, v := range t.ranges {
		out[k] = v
	}
	return out
}
