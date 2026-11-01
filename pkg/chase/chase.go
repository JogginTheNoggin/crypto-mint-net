// Package chase keeps a full node's chase queue: the stated weights above
// the node's own that it may follow up, grouped by the branch they point
// at and ordered by evidence, never by their size (the design's "Chasing
// Heavier Ledgers", steps 0 to 2 and the limits on chasing). It also keeps
// each sender's backoff and the known-bad list of entry hashes shown to be
// false.
//
// The queue decides only what to chase next and from whom. Package p2p
// runs each chase (finding the common entry, checking the work with
// provisional layers, fetching the entries) and reports how it ended. A
// node switches only on weight it computed itself, so everything here is
// node policy. The package reads no clock.
package chase

import (
	"sort"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Evidence is how far a group has shown that its branch is real.
type Evidence int

const (
	// WordOnly: one sender, or several unlisted ones, and nothing else.
	WordOnly Evidence = iota
	// Corroborated: a priority peer, or enough independent listed senders,
	// back it.
	Corroborated
	// WorkChecked: provisional layers showed real work greater than the
	// weight to beat; only the entries are left.
	WorkChecked
)

func (e Evidence) String() string {
	return [...]string{"word-only", "corroborated", "work-checked"}[e]
}

// MarshalText writes the evidence's name.
func (e Evidence) MarshalText() ([]byte, error) { return []byte(e.String()), nil }

// Sender is a peer or unlisted node that sent a stated weight. Its stated
// weight is kept only to tell, when its turn comes, whether the node has
// caught up already; it never orders anything.
type Sender struct {
	ID     string         `json:"id"`
	Range  string         `json:"range"`
	Cat    peers.Category `json:"category"`
	Stated weight.Weight  `json:"stated"`
}

// Config holds the queue's limits, all node policy. Zero values mean the
// provisional defaults in DefaultConfig.
type Config struct {
	// MaxDelay is how long a group may wait for its next step, by the best
	// category among its senders. A group past it goes to the front of
	// its level.
	MaxDelay map[peers.Category]time.Duration
	// Corroborate is how many independent listed senders corroborate a
	// group; UnlistedCount lets unlisted senders count too.
	Corroborate   int
	UnlistedCount bool
	// GroupsPerPool caps the groups held for one pool.
	GroupsPerPool int
	// Chases caps the chases running at once, PerPool those for one pool,
	// and Unlisted those whose best sender is unlisted (their small part
	// of the chase share). One sender runs one chase at a time.
	Chases, PerPool, Unlisted int
	// Backoff is the first wait on a sender after a chase from it led
	// nowhere; it doubles with each failure in a row up to MaxBackoff and
	// resets after a chase from it succeeds.
	Backoff, MaxBackoff time.Duration
	// BadFor and BadSize bound the known-bad list.
	BadFor  time.Duration
	BadSize int
	// YieldWait is how long a sender whose chase yielded waits before it
	// goes on.
	YieldWait time.Duration
}

// DefaultConfig returns the provisional defaults.
func DefaultConfig() Config {
	return Config{
		MaxDelay: map[peers.Category]time.Duration{
			peers.CatPriority: 5 * time.Second, peers.CatBackup: 30 * time.Second,
			peers.CatCandidate: time.Minute, peers.CatUnlisted: 5 * time.Minute,
		},
		Corroborate:   2,
		GroupsPerPool: 64,
		Chases:        4, PerPool: 2, Unlisted: 1,
		Backoff: 30 * time.Second, MaxBackoff: time.Hour,
		BadFor: 24 * time.Hour, BadSize: 4096,
		YieldWait: 5 * time.Second,
	}
}

func (c *Config) fill() {
	d := DefaultConfig()
	if c.MaxDelay == nil {
		c.MaxDelay = map[peers.Category]time.Duration{}
	}
	for k, v := range d.MaxDelay {
		if c.MaxDelay[k] <= 0 {
			c.MaxDelay[k] = v
		}
	}
	if c.Corroborate <= 0 {
		c.Corroborate = d.Corroborate
	}
	if c.GroupsPerPool <= 0 {
		c.GroupsPerPool = d.GroupsPerPool
	}
	if c.Chases <= 0 {
		c.Chases = d.Chases
	}
	if c.PerPool <= 0 {
		c.PerPool = d.PerPool
	}
	if c.Unlisted <= 0 {
		c.Unlisted = d.Unlisted
	}
	if c.Backoff <= 0 {
		c.Backoff = d.Backoff
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = d.MaxBackoff
	}
	if c.BadFor <= 0 {
		c.BadFor = d.BadFor
	}
	if c.BadSize <= 0 {
		c.BadSize = d.BadSize
	}
	if c.YieldWait <= 0 {
		c.YieldWait = d.YieldWait
	}
}

// Group is every stated weight that points at one branch of one pool.
type Group struct {
	ID       uint64             `json:"id"`
	Pool     string             `json:"pool"`
	Hashes   map[hash.Hash]bool `json:"hashes"`
	Senders  map[string]Sender  `json:"senders"`
	Evidence Evidence           `json:"evidence"`
	Added    time.Time          `json:"added"`
	// Confirmed: a priority peer asked to confirm saw something heavier.
	Confirmed bool `json:"confirmed,omitempty"`
	// Asked: a priority peer was asked to confirm.
	Asked bool `json:"asked,omitempty"`
	// Denied lists priority peers that answered nothing heavier.
	Denied []string `json:"denied,omitempty"`

	running string // the sender a chase of it runs with, if any
}

func (g *Group) clone() Group {
	c := *g
	c.Hashes = make(map[hash.Hash]bool, len(g.Hashes))
	for h := range g.Hashes {
		c.Hashes[h] = true
	}
	c.Senders = make(map[string]Sender, len(g.Senders))
	for k, v := range g.Senders {
		c.Senders[k] = v
	}
	c.Denied = append([]string(nil), g.Denied...)
	return c
}

// Running reports whether a chase of the group is running.
func (g Group) Running() bool { return g.running != "" }

// Best returns the best category among the group's senders.
func (g Group) Best() peers.Category {
	best := peers.CatUnlisted
	for _, s := range g.Senders {
		if s.Cat < best {
			best = s.Cat
		}
	}
	return best
}

// Independent counts the group's senders from different network ranges,
// among those in cats (all when cats is empty).
func (g Group) Independent(cats ...peers.Category) int {
	rs := map[string]bool{}
	for _, s := range g.Senders {
		ok := len(cats) == 0
		for _, c := range cats {
			ok = ok || s.Cat == c
		}
		if ok {
			rs[s.Range] = true
		}
	}
	return len(rs)
}

type backoff struct {
	until time.Time
	fails int
}

// Queue is the chase queue. It is safe for concurrent use.
type Queue struct {
	mu      sync.Mutex
	cfg     Config
	seq     uint64
	groups  map[uint64]*Group
	open    map[string]map[string]uint64 // pool → sender → its open group
	backoff map[string]backoff
	bad     map[hash.Hash]time.Time
	senders map[string]bool // senders running a chase
}

// New returns an empty queue.
func New(cfg Config) *Queue {
	cfg.fill()
	return &Queue{
		cfg: cfg, groups: map[uint64]*Group{}, open: map[string]map[string]uint64{},
		backoff: map[string]backoff{}, bad: map[hash.Hash]time.Time{}, senders: map[string]bool{},
	}
}

// Config returns the limits in use, defaults filled in.
func (q *Queue) Config() Config { return q.cfg }

// IsBad reports whether h is on the known-bad list.
func (q *Queue) IsBad(h hash.Hash, now time.Time) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	u, ok := q.bad[h]
	return ok && now.Before(u)
}

// BackedOff returns when id's backoff ends, if it is backed off at now.
func (q *Queue) BackedOff(id string, now time.Time) (time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	b := q.backoff[id]
	return b.until, now.Before(b.until)
}

func (q *Queue) evidenceLocked(g *Group) Evidence {
	if g.Evidence == WorkChecked {
		return WorkChecked
	}
	if g.Confirmed || g.Best() == peers.CatPriority {
		return Corroborated
	}
	cats := []peers.Category{peers.CatBackup, peers.CatCandidate}
	if q.cfg.UnlistedCount {
		cats = append(cats, peers.CatUnlisted)
	}
	if g.Independent(cats...) >= q.cfg.Corroborate {
		return Corroborated
	}
	return WordOnly
}

// Offer puts a stated weight from s, for pool, in a group: the group
// holding h when one is given (the entry or layer it came with names it),
// else a group of the sender's own. A sender has one open stated weight
// per pool, so a newer one replaces its older one. It returns the group's
// id, and false when the pool's cap is reached and the group would be the
// lowest in the order. isNew reports whether the sender joined the group
// with this stated weight, which is what counts against its budget.
func (q *Queue) Offer(pool string, s Sender, h *hash.Hash, now time.Time) (id uint64, isNew, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if h != nil {
		if u, ok := q.bad[*h]; ok && now.Before(u) {
			return 0, false, false
		}
	}
	open := q.open[pool]
	if open == nil {
		open = map[string]uint64{}
		q.open[pool] = open
	}
	var target *Group
	for _, g := range q.groups {
		if g.Pool != pool {
			continue
		}
		if h != nil && g.Hashes[*h] {
			target = g
			break
		}
		if h == nil && len(g.Hashes) == 0 && len(g.Senders) == 1 && g.Senders[s.ID].ID == s.ID {
			target = g
			break
		}
	}
	if old, ok := open[s.ID]; ok && (target == nil || old != target.ID) {
		q.leaveLocked(old, s.ID)
	}
	if target == nil {
		if q.countLocked(pool) >= q.cfg.GroupsPerPool && !q.evictLocked(pool, s, now) {
			return 0, false, false
		}
		q.seq++
		target = &Group{ID: q.seq, Pool: pool, Hashes: map[hash.Hash]bool{}, Senders: map[string]Sender{}, Added: now}
		q.groups[target.ID] = target
	}
	if h != nil {
		target.Hashes[*h] = true
	}
	old, had := target.Senders[s.ID]
	if had && old.Cat < s.Cat {
		s.Cat = old.Cat
	}
	target.Senders[s.ID] = s
	open[s.ID] = target.ID
	target.Evidence = q.evidenceLocked(target)
	return target.ID, !had, true
}

// Leave takes sender out of group id, as when its stated weight is over
// its budget after all.
func (q *Queue) Leave(id uint64, sender string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if g := q.groups[id]; g != nil && g.running != sender {
		q.leaveLocked(id, sender)
	}
}

// leaveLocked takes sender out of group id, dropping the group when no
// sender is left and no chase of it runs.
func (q *Queue) leaveLocked(id uint64, sender string) {
	g := q.groups[id]
	if g == nil {
		return
	}
	delete(g.Senders, sender)
	if open := q.open[g.Pool]; open != nil && open[sender] == id {
		delete(open, sender)
	}
	if len(g.Senders) == 0 && g.running == "" {
		delete(q.groups, id)
		return
	}
	g.Evidence = q.evidenceLocked(g)
}

func (q *Queue) countLocked(pool string) int {
	n := 0
	for _, g := range q.groups {
		if g.Pool == pool {
			n++
		}
	}
	return n
}

// less orders groups: evidence first, then the best category among their
// senders, then independent senders, then age. A group past its max delay
// goes to the front of its level. Stated weight plays no part.
func (q *Queue) less(a, b *Group, now time.Time) bool {
	if a.Evidence != b.Evidence {
		return a.Evidence > b.Evidence
	}
	la, lb := q.lateLocked(a, now), q.lateLocked(b, now)
	if la != lb {
		return la
	}
	if ca, cb := a.Best(), b.Best(); ca != cb {
		return ca < cb
	}
	if ia, ib := a.Independent(), b.Independent(); ia != ib {
		return ia > ib
	}
	if !a.Added.Equal(b.Added) {
		return a.Added.Before(b.Added)
	}
	return a.ID < b.ID
}

func (q *Queue) lateLocked(g *Group, now time.Time) bool {
	return now.Sub(g.Added) > q.cfg.MaxDelay[g.Best()]
}

// evictLocked makes room in pool for a new group from s by dropping the
// lowest group, if the new one would rank above it.
func (q *Queue) evictLocked(pool string, s Sender, now time.Time) bool {
	var low *Group
	for _, g := range q.groups {
		if g.Pool == pool && g.running == "" && (low == nil || q.less(low, g, now)) {
			low = g
		}
	}
	if low == nil {
		return false
	}
	nw := &Group{Senders: map[string]Sender{s.ID: s}, Added: now, ID: q.seq + 1}
	nw.Evidence = q.evidenceLocked(nw)
	if !q.less(nw, low, now) {
		return false
	}
	for id := range low.Senders {
		q.leaveLocked(low.ID, id)
	}
	delete(q.groups, low.ID)
	return true
}

// Job is one step of a chase to run: a group and the sender to run it
// with.
type Job struct {
	Group  Group
	Sender Sender
}

// Next picks the next group to chase and its best sender that is not
// backed off and not running another chase, within the limits on chases
// at once. It marks the group running until Finish.
func (q *Queue) Next(now time.Time) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	total, unlisted := 0, 0
	perPool := map[string]int{}
	for _, g := range q.groups {
		if g.running != "" {
			total++
			perPool[g.Pool]++
			if g.Best() == peers.CatUnlisted {
				unlisted++
			}
		}
	}
	if total >= q.cfg.Chases {
		return Job{}, false
	}
	var cands []*Group
	for _, g := range q.groups {
		if g.running == "" && perPool[g.Pool] < q.cfg.PerPool {
			cands = append(cands, g)
		}
	}
	sort.Slice(cands, func(i, j int) bool { return q.less(cands[i], cands[j], now) })
	for _, g := range cands {
		if g.Best() == peers.CatUnlisted && unlisted >= q.cfg.Unlisted {
			continue
		}
		s, ok := q.senderLocked(g, now)
		if !ok {
			continue // every sender backed off or busy: it waits
		}
		g.running = s.ID
		q.senders[s.ID] = true
		return Job{Group: g.clone(), Sender: s}, true
	}
	return Job{}, false
}

// senderLocked returns g's best sender to run a chase with.
func (q *Queue) senderLocked(g *Group, now time.Time) (Sender, bool) {
	ids := make([]string, 0, len(g.Senders))
	for id := range g.Senders {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := g.Senders[ids[i]], g.Senders[ids[j]]
		if a.Cat != b.Cat {
			return a.Cat < b.Cat
		}
		return a.ID < b.ID
	})
	for _, id := range ids {
		if q.senders[id] || now.Before(q.backoff[id].until) {
			continue
		}
		return g.Senders[id], true
	}
	return Sender{}, false
}

// Wait returns how long until a backed-off group could run again, or 0
// when none waits on a backoff.
func (q *Queue) Wait(now time.Time) time.Duration {
	q.mu.Lock()
	defer q.mu.Unlock()
	var best time.Duration
	for _, g := range q.groups {
		if g.running != "" {
			continue
		}
		for id := range g.Senders {
			if d := q.backoff[id].until.Sub(now); d > 0 && (best == 0 || d < best) {
				best = d
			}
		}
	}
	return best
}

// Confirm records a priority peer's answer when asked to confirm group id:
// it saw something heavier (it joins as a sender and the group is
// corroborated) or nothing heavier (a denial, which never removes the
// group).
func (q *Queue) Confirm(id uint64, p Sender, heavier bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	g := q.groups[id]
	if g == nil {
		return
	}
	g.Asked = true
	if heavier {
		g.Confirmed = true
		p.Cat = peers.CatPriority
		g.Senders[p.ID] = p
		if open := q.open[g.Pool]; open != nil {
			if old, ok := open[p.ID]; ok && old != id {
				q.leaveLocked(old, p.ID)
			}
			open[p.ID] = id
		}
	} else {
		g.Denied = append(g.Denied, p.ID)
	}
	g.Evidence = q.evidenceLocked(g)
}

// AddHashes records hashes the running chase of group id learned from its
// sender, and merges in other groups of the pool that share any of them.
func (q *Queue) AddHashes(id uint64, hs []hash.Hash) {
	q.mu.Lock()
	defer q.mu.Unlock()
	g := q.groups[id]
	if g == nil {
		return
	}
	for _, h := range hs {
		g.Hashes[h] = true
	}
	for oid, o := range q.groups {
		if oid == id || o.Pool != g.Pool || o.running != "" {
			continue
		}
		shared := false
		for h := range o.Hashes {
			if g.Hashes[h] {
				shared = true
				break
			}
		}
		if !shared {
			continue
		}
		for h := range o.Hashes {
			g.Hashes[h] = true
		}
		for sid, s := range o.Senders {
			if old, ok := g.Senders[sid]; !ok || s.Cat < old.Cat {
				g.Senders[sid] = s
			}
			q.open[g.Pool][sid] = id
		}
		g.Confirmed = g.Confirmed || o.Confirmed
		g.Denied = append(g.Denied, o.Denied...)
		if o.Added.Before(g.Added) {
			g.Added = o.Added
		}
		delete(q.groups, oid)
	}
	g.Evidence = q.evidenceLocked(g)
}

// WorkChecked marks group id: its provisional layers beat the weight to
// beat.
func (q *Queue) WorkChecked(id uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if g := q.groups[id]; g != nil {
		g.Evidence = WorkChecked
	}
}

// Outcome is how one chase ended.
type Outcome int

const (
	// Switched: the node switched to the branch, on weight it computed.
	Switched Outcome = iota
	// Nowhere: an honest failure (refused, stalled, not held, not heavier
	// once checked). The sender is backed off and the
	// group carries on with its other senders.
	Nowhere
	// False: data that breaks the rules or contradicts itself. The hashes
	// go on the known-bad list, the sender is backed off, and the group is
	// dropped.
	False
	// Gone: the sender went away mid-chase. It leaves the group, with no
	// backoff.
	Gone
	// Caught: the node already holds as much as the sender stated. It
	// leaves the group, with no backoff.
	Caught
	// Requeue: a priority peer confirmed the group, which waits for its
	// next step with the priority peer as its best sender.
	Requeue
	// Yield: the chase stopped for a while, not for a fault: its time
	// slice ran out, or the sender's budget is spent for now. The group
	// keeps its senders and evidence, and the sender waits YieldWait with
	// no backoff, so a long chase goes on later instead of being dropped
	// (consensus audit P2, P4; D116).
	Yield
)

func (o Outcome) String() string {
	return [...]string{"switched", "nowhere", "false", "gone", "caught", "requeue", "yield"}[o]
}

// Finish ends the running chase of group id and returns the group as it
// was, so the caller can count denials after a switch.
func (q *Queue) Finish(id uint64, o Outcome, bad []hash.Hash, now time.Time) Group {
	q.mu.Lock()
	defer q.mu.Unlock()
	g := q.groups[id]
	if g == nil {
		return Group{}
	}
	sender := g.running
	g.running = ""
	delete(q.senders, sender)
	out := g.clone()
	switch o {
	case Switched:
		delete(q.backoff, sender)
		q.dropLocked(g)
	case Nowhere:
		q.backoffLocked(sender, now)
		q.leaveLocked(id, sender)
	case False:
		q.backoffLocked(sender, now)
		for _, h := range bad {
			q.addBadLocked(h, now)
		}
		q.dropLocked(g)
	case Gone, Caught:
		q.leaveLocked(id, sender)
	case Requeue:
		g.Evidence = q.evidenceLocked(g)
	case Yield:
		b := q.backoff[sender]
		b.until = now.Add(q.cfg.YieldWait)
		q.backoff[sender] = b
		g.Evidence = q.evidenceLocked(g)
	}
	return out
}

func (q *Queue) dropLocked(g *Group) {
	for id := range g.Senders {
		if open := q.open[g.Pool]; open != nil && open[id] == g.ID {
			delete(open, id)
		}
	}
	delete(q.groups, g.ID)
}

func (q *Queue) backoffLocked(id string, now time.Time) {
	b := q.backoff[id]
	wait := q.cfg.Backoff << b.fails
	if wait > q.cfg.MaxBackoff || wait <= 0 {
		wait = q.cfg.MaxBackoff
	}
	if b.fails < 32 {
		b.fails++
	}
	b.until = now.Add(wait)
	q.backoff[id] = b
}

func (q *Queue) addBadLocked(h hash.Hash, now time.Time) {
	if len(q.bad) >= q.cfg.BadSize {
		var oldest hash.Hash
		var at time.Time
		for k, u := range q.bad {
			if !now.Before(u) {
				delete(q.bad, k)
				continue
			}
			if at.IsZero() || u.Before(at) {
				oldest, at = k, u
			}
		}
		if len(q.bad) >= q.cfg.BadSize {
			delete(q.bad, oldest)
		}
	}
	q.bad[h] = now.Add(q.cfg.BadFor)
}

// Drop takes a sender that went away out of every group that is not
// running a chase with it.
func (q *Queue) Drop(sender string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for pool, open := range q.open {
		if id, ok := open[sender]; ok {
			if g := q.groups[id]; g != nil && g.running == sender {
				continue
			}
			q.leaveLocked(id, sender)
		}
		if len(open) == 0 {
			delete(q.open, pool)
		}
	}
}

// Groups returns a copy of the groups of pool ("" for all), in queue
// order.
func (q *Queue) Groups(pool string, now time.Time) []Group {
	q.mu.Lock()
	defer q.mu.Unlock()
	var gs []*Group
	for _, g := range q.groups {
		if pool == "" || g.Pool == pool {
			gs = append(gs, g)
		}
	}
	sort.Slice(gs, func(i, j int) bool { return q.less(gs[i], gs[j], now) })
	out := make([]Group, len(gs))
	for i, g := range gs {
		out[i] = g.clone()
	}
	return out
}
