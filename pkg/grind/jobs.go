package grind

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// The targets a job works on.
const (
	// TargetTip keeps adding layers to the selected tip of each pool (to
	// its best branch while none is selected).
	TargetTip = "tip"
	// TargetPending builds entries from each pool's pending transactions
	// as they arrive.
	TargetPending = "pending"
	// TargetTx, followed by a TID in hex, grinds for one payment: entries
	// while it is pending, then layers on the entry that holds it, until
	// from(e) reaches the spec's UntilFrom (GREEN-019).
	TargetTx = "tx:"
)

// Spec says what a job does. The user names a target, never a hash.
type Spec struct {
	// Target is TargetTip, TargetPending, "tx:" and a TID, or an entry
	// hash in hex: layers on that one entry.
	Target string `json:"target"`
	// Series and Pools narrow the pools worked on; empty means every
	// followed pool (for an entry: the pool that holds it).
	Series *hash.Hash `json:"series,omitempty"`
	Pools  []uint64   `json:"pools,omitempty"`
	// Device and Threads as for Choose; empty takes the node policy's.
	Device  string `json:"device,omitempty"`
	Threads int    `json:"threads,omitempty"`
	// For bounds the run time ("30s", "2h"); empty runs until stopped.
	For string `json:"for,omitempty"`
	// UntilFrom stops a job on one entry or payment once from(e) reaches
	// this weight: "until my payment is safe". A tx target needs it.
	UntilFrom string `json:"until_from,omitempty"`
	// Hours limits the work to a window of the node's local hours, "22-06"
	// (from 22:00 to 06:00); empty works at any hour.
	Hours string `json:"hours,omitempty"`
	// Source says who made the job: "user" (the default) or "policy".
	Source string `json:"source,omitempty"`
	// Lease, when set ("15s"), stops the job unless it is renewed
	// (Manager.Renew) within that long: a job its follower started, such
	// as poo grind run's, ends when the follower is gone (GUI-010).
	Lease string `json:"lease,omitempty"`
}

// The states of a job.
const (
	StateRunning = "running"
	StateWaiting = "waiting" // outside its hours, or nothing to work on
	StateDone    = "done"    // reached its goal or its time
	StateStopped = "stopped"
	StateFailed  = "failed"
)

// Job is a job's state, as replies show it.
type Job struct {
	ID      int       `json:"id"`
	Spec    Spec      `json:"spec"`
	State   string    `json:"state"`
	Reason  string    `json:"reason,omitempty"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitzero"`
	// Plans are the device and grinder picked per hash worked on.
	Plans   []Plan  `json:"plans"`
	Tries   uint64  `json:"tries"`
	Rate    float64 `json:"rate"` // tries a second over the last slice
	Layers  int     `json:"layers"`
	Entries int     `json:"entries"`
	// From is the target entry's from(e), for a job on one entry or
	// payment; Entry is the entry, for a payment once it is in one.
	From  string     `json:"from,omitempty"`
	Entry *hash.Hash `json:"entry,omitempty"`
	// Hint says what would give a waiting job something to do.
	Hint string `json:"hint,omitempty"`
}

// Done reports whether the job has ended.
func (j Job) Done() bool {
	return j.State == StateDone || j.State == StateStopped || j.State == StateFailed
}

// Event reports what a job did. Kind is one of:
//
//   - start: the job started (job-wide, with its spec);
//   - plan: the device and grinder chosen for a pool's hash (per pool);
//   - progress: tries and rate, at most once a second per job (per pool);
//   - found: a layer or entry added (per pool);
//   - waiting: nothing to do now, with the reason and a hint (job-wide);
//   - end: the job ended, with its state and reason (job-wide).
//
// A job-wide event has no series or pool. A watch filter on series or
// pools passes it when the job's spec can work on one of them.
type Event struct {
	Job    int       `json:"job"`
	Kind   string    `json:"kind"`
	Series hash.Hash `json:"series,omitzero"`
	Pool   uint64    `json:"pool,omitempty"`
	State  string    `json:"state,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Hint   string    `json:"hint,omitempty"`
	Spec   *Spec     `json:"spec,omitempty"`
	Plan   *Plan     `json:"plan,omitempty"`
	Tries  uint64    `json:"tries,omitempty"`
	Rate   float64   `json:"rate,omitempty"`
	// Found: the layer or entry added, and the layer's strength.
	Layer    *hash.Hash `json:"layer,omitempty"`
	Entry    *hash.Hash `json:"entry,omitempty"`
	Strength int        `json:"strength,omitempty"`
	From     string     `json:"from,omitempty"`
	// Scope is the spec of the job of a job-wide event, for filters.
	Scope *Spec `json:"-"`
}

// JobWide reports whether ev is about the whole job rather than one pool.
func (ev Event) JobWide() bool { return ev.Series == (hash.Hash{}) }

// Limits on jobs.
const (
	// UserJobs is how many jobs past a full schedule a node runs at once,
	// so a schedule of nodepolicy.MaxGrindJobs jobs, even all waiting,
	// never blocks a job a user or a follower starts (GREEN-035).
	UserJobs = 4
	// MaxJobs bounds the jobs a node runs at once.
	MaxJobs = nodepolicy.MaxGrindJobs + UserJobs
	// MaxDone is how many ended jobs a node remembers.
	MaxDone = 32
	// Slice is how long one search runs before the job looks again at its
	// target, its hours and its goal.
	Slice = 2 * time.Second
	// ProgressEvery is how often a job reports progress at most; its rate
	// is measured over that time.
	ProgressEvery = time.Second
	// MaxLease bounds a job's lease.
	MaxLease = 10 * time.Minute
)

// Errors of the manager.
var (
	ErrTooManyJobs = fmt.Errorf("grind: at most %d jobs at once", MaxJobs)
	ErrNoJob       = errors.New("grind: no such job")
	ErrClosed      = errors.New("grind: the manager is closed")
)

// Defaults are the node policy's device and threads, for jobs that name
// none.
type Defaults struct {
	Device  string
	Threads int
	// Floor is the node policy's layer_floor (ORANGE-010), and Rate its
	// layer_rate (ORANGE-016).
	Floor int
	Rate  int
}

// Manager runs the jobs of one node.
type Manager struct {
	n    *node.Node
	reg  *Registry
	devs []Device
	def  Defaults
	log  *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// sched serialises Schedule, so two policies never run together
	// (RED-013).
	sched sync.Mutex

	mu   sync.Mutex
	jobs map[int]*job
	// reserved counts starts past the MaxJobs check that have not yet
	// inserted their job (RED-014).
	reserved  int
	done      []int // ended jobs, oldest first
	next      int
	listeners []func(Event)
	tries     map[[2]string]uint64 // by device and hash
}

type job struct {
	Job
	cancel context.CancelFunc
	// stopped is why the job was stopped from outside, if it was.
	stopped string
	lease   time.Duration
	timer   *time.Timer
	// The last progress event: when, and the tries then.
	progAt    time.Time
	progTries uint64
	// ended is closed when the job ends (ORANGE-013).
	ended chan struct{}
}

// NewManager returns a manager for n, grinding with reg's grinders on the
// devices it detects. log may be nil.
func NewManager(n *node.Node, reg *Registry, def Defaults, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{n: n, reg: reg, devs: reg.Detect(), def: def, log: log, ctx: ctx, cancel: cancel,
		jobs: map[int]*job{}, next: 1, tries: map[[2]string]uint64{}}
}

// Devices returns the devices the manager grinds on.
func (m *Manager) Devices() []Device { return slices.Clone(m.devs) }

// Choose is Registry.Choose on the manager's devices and defaults.
func (m *Manager) Choose(id hash.ID, device string, threads int) (Plan, error) {
	m.mu.Lock()
	def := m.def
	m.mu.Unlock()
	if device == "" {
		device = def.Device
	}
	if threads == 0 {
		threads = def.Threads
	}
	return m.reg.Choose(m.devs, id, device, threads)
}

// Schedule applies a node policy's grind section while the node runs. When
// the section's defaults are unchanged it applies the difference: a
// running policy job that is still scheduled keeps running, with its ID,
// tries and rate, and only the jobs taken out are stopped and the jobs
// added started (GREEN-035). When a default changes (device, threads,
// layer_floor, layer_rate), every plan may change, so every policy job is
// stopped and started again. A stopped job's reason says the schedule
// changed. It returns the running job of each scheduled job, kept or new,
// in the schedule's order, and, by job, why any was not started
// (GREEN-026). g may be nil: no defaults and no jobs.
func (m *Manager) Schedule(g *nodepolicy.Grind) ([]Job, map[int]error) {
	m.sched.Lock()
	defer m.sched.Unlock()
	var def Defaults
	var jobs []nodepolicy.GrindJob
	if g != nil {
		def, jobs = Defaults{Device: g.Device, Threads: g.Threads, Floor: g.LayerFloor, Rate: g.LayerRate}, g.Jobs
	}
	m.mu.Lock()
	// Only the device and threads go into a job's plans; the layer floor
	// and rate are read at each search (work), so changing them keeps the
	// running jobs (GREEN-036).
	same := m.def.Device == def.Device && m.def.Threads == def.Threads
	m.def = def
	// Each scheduled job takes the first running policy job of the same
	// spec, when the defaults are the same; the rest are stopped.
	kept := make([]int, len(jobs))
	taken := map[int]bool{}
	if same {
		var ids []int
		for id, j := range m.jobs {
			if !j.Done() && j.Spec.Source == "policy" {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		for i, gj := range jobs {
			want := SpecOf(gj)
			for _, id := range ids {
				if !taken[id] && sameSpec(m.jobs[id].Spec, want) {
					kept[i], taken[id] = id, true
					break
				}
			}
		}
	}
	var stop []int
	for id, j := range m.jobs {
		if !j.Done() && j.Spec.Source == "policy" && !taken[id] {
			if j.stopped == "" {
				j.stopped = "the schedule changed"
			}
			j.cancel()
			stop = append(stop, id)
		}
	}
	m.mu.Unlock()
	for _, id := range stop {
		m.wait(id)
	}
	started := []Job{}
	failed := map[int]error{}
	for i, gj := range jobs {
		if kept[i] != 0 {
			if j, ok := m.Get(kept[i]); ok {
				started = append(started, j)
			}
			continue
		}
		j, err := m.Start(SpecOf(gj))
		if err != nil {
			failed[i] = err
			m.log.Warn("job from the policy not started", "job", i, "err", err)
			continue
		}
		started = append(started, j)
	}
	return started, failed
}

// sameSpec reports whether a and b are the same in every field.
func sameSpec(a, b Spec) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// OnEvent adds a function that receives every job event. It runs on the
// job's goroutine and must not block.
func (m *Manager) OnEvent(f func(Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, f)
}

func (m *Manager) emit(ev Event) {
	m.mu.Lock()
	ls := slices.Clone(m.listeners)
	m.mu.Unlock()
	for _, f := range ls {
		f(ev)
	}
}

// Check validates a spec without starting it.
func Check(s Spec) error {
	switch {
	case s.Target == TargetTip, s.Target == TargetPending:
		if s.UntilFrom != "" {
			return errors.New("grind: until_from needs an entry or tx:TID as the target")
		}
	case s.Target == "":
		return errors.New("grind: a target is needed: tip, pending, tx:TID or an entry hash")
	case strings.HasPrefix(s.Target, TargetTx):
		if _, err := hash.Parse(strings.TrimPrefix(s.Target, TargetTx)); err != nil {
			return fmt.Errorf("grind: tx:TID: %v", err)
		}
		if s.UntilFrom == "" {
			return errors.New("grind: a tx:TID target needs until_from, the weight at which the payment is safe")
		}
	default:
		if _, err := hash.Parse(s.Target); err != nil {
			return fmt.Errorf("grind: target is tip, pending, tx:TID or an entry hash: %v", err)
		}
	}
	if s.Threads < 0 || s.Threads > MaxThreads {
		return fmt.Errorf("grind: threads must be 0 to %d", MaxThreads)
	}
	if s.For != "" {
		d, err := time.ParseDuration(s.For)
		if err != nil || d <= 0 {
			return fmt.Errorf("grind: for: a duration such as 30s or 2h")
		}
	}
	if s.UntilFrom != "" {
		var w weight.Weight
		if err := w.UnmarshalText([]byte(s.UntilFrom)); err != nil {
			return fmt.Errorf("grind: until_from: %v", err)
		}
	}
	if _, _, err := ParseHours(s.Hours); err != nil {
		return err
	}
	if s.Lease != "" {
		if d, err := time.ParseDuration(s.Lease); err != nil || d < time.Second || d > MaxLease {
			return fmt.Errorf("grind: lease: a duration from 1s to %s", MaxLease)
		}
	}
	return nil
}

// ParseHours reads a window of local hours, "22-06"
// (nodepolicy.ParseHours).
func ParseHours(s string) (from, to int, err error) {
	from, to, err = nodepolicy.ParseHours(s)
	if err != nil {
		err = fmt.Errorf("grind: %w", err)
	}
	return from, to, err
}

// inHours reports whether t's hour is in the window [from, to).
func inHours(t time.Time, from, to int) bool {
	h := t.Hour()
	if from < to {
		return h >= from && h < to
	}
	return h >= from || h < to
}

// Start starts a job and returns its state.
func (m *Manager) Start(s Spec) (Job, error) {
	if err := Check(s); err != nil {
		return Job{}, err
	}
	if s.Source == "" {
		s.Source = "user"
	}
	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return Job{}, ErrClosed
	}
	running := 0
	for _, j := range m.jobs {
		if !j.Done() {
			running++
		}
	}
	// The slot is held from the count to the insert, so concurrent starts
	// cannot pass MaxJobs together (RED-014, decision D114).
	if running+m.reserved >= MaxJobs {
		m.mu.Unlock()
		return Job{}, ErrTooManyJobs
	}
	m.reserved++
	m.mu.Unlock()
	// A job that would work now and has no device for one of its hashes
	// is refused at once, as build and work are (GREEN-017). One within
	// hours may find its device by then, and fails then if not.
	var err error
	if s.Hours == "" {
		err = m.checkPlans(s)
	}
	m.mu.Lock()
	m.reserved--
	if err != nil {
		m.mu.Unlock()
		return Job{}, err
	}
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return Job{}, ErrClosed
	}
	ctx, cancel := context.WithCancel(m.ctx)
	if s.For != "" {
		d, _ := time.ParseDuration(s.For)
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, d)
		parent := cancel
		cancel = func() { stop(); parent() }
	}
	j := &job{Job: Job{ID: m.next, Spec: s, State: StateRunning, Started: time.Now().UTC(), Plans: []Plan{}}, cancel: cancel, ended: make(chan struct{})}
	// A job outside its hours says so in the reply that starts it, not only
	// once its runner first looks (GREEN-020).
	if reason, hint, ok := outsideHours(s.Hours, time.Now()); ok {
		j.State, j.Reason, j.Hint = StateWaiting, reason, hint
	}
	if s.Lease != "" {
		j.lease, _ = time.ParseDuration(s.Lease)
		id := m.next
		j.timer = time.AfterFunc(j.lease, func() { m.expire(id) })
	}
	m.next++
	m.jobs[j.ID] = j
	snap := j.Job
	m.mu.Unlock()
	m.log.Info("job started", "job", j.ID, "target", s.Target, "source", s.Source)
	sp := s
	m.emit(Event{Job: j.ID, Kind: "start", State: snap.State, Spec: &sp, Scope: &sp})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(ctx, j)
	}()
	return snap, nil
}

// checkPlans chooses a plan for every hash job s would grind now.
func (m *Manager) checkPlans(s Spec) error {
	seen := map[hash.ID]bool{}
	for _, k := range m.targets(s, hash.Hash{}) {
		sr, ok := m.n.Engine().Series(k.Series)
		if !ok || seen[sr.Params().HashID] {
			continue
		}
		seen[sr.Params().HashID] = true
		if _, err := m.Choose(sr.Params().HashID, s.Device, s.Threads); err != nil {
			return err
		}
	}
	return nil
}

// Renew renews job id's lease, and returns its state.
func (m *Manager) Renew(id int) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, ErrNoJob
	}
	if j.timer != nil && !j.Done() {
		j.timer.Reset(j.lease)
	}
	return j.snapshot(), nil
}

// expire stops job id, whose lease ran out.
func (m *Manager) expire(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok && !j.Done() && j.stopped == "" {
		j.stopped = "its lease ran out: whoever followed it is gone"
		j.cancel()
	}
}

// Jobs returns every job, running and recently ended, by id.
func (m *Manager) Jobs() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j.snapshot())
	}
	slices.SortFunc(out, func(a, b Job) int { return a.ID - b.ID })
	return out
}

// Get returns one job.
func (m *Manager) Get(id int) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return j.snapshot(), true
}

func (j *job) snapshot() Job {
	s := j.Job
	s.Plans = slices.Clone(j.Plans)
	return s
}

// Stop stops a job; it keeps the work found so far. Stopping an ended job
// does nothing.
func (m *Manager) Stop(id int) (Job, error) {
	m.mu.Lock()
	j, ok := m.jobs[id]
	if ok && !j.Done() && j.stopped == "" {
		j.stopped = "stopped"
		j.cancel()
	}
	m.mu.Unlock()
	if !ok {
		return Job{}, ErrNoJob
	}
	return m.wait(id), nil
}

// wait waits a moment for job id to end, and returns its state.
func (m *Manager) wait(id int) Job {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if ok {
		t := time.NewTimer(2 * time.Second)
		defer t.Stop()
		select {
		case <-j.ended:
		case <-t.C:
		}
	}
	j2, _ := m.Get(id)
	return j2
}

// StopAll stops every running job made by source ("" for all).
func (m *Manager) StopAll(source string) []Job {
	m.mu.Lock()
	var ids []int
	for id, j := range m.jobs {
		if !j.Done() && (source == "" || j.Spec.Source == source) {
			if j.stopped == "" {
				j.stopped = "stopped"
			}
			j.cancel()
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	slices.Sort(ids)
	out := make([]Job, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.wait(id))
	}
	return out
}

// Close stops every job and waits for them.
func (m *Manager) Close() {
	m.mu.Lock()
	for _, j := range m.jobs {
		if j.stopped == "" {
			j.stopped = "the node stopped"
		}
	}
	m.mu.Unlock()
	m.cancel()
	m.wg.Wait()
}

// Totals returns the tries ground so far, by device and hash, and the
// number of jobs in each state, for metrics.
func (m *Manager) Totals() (tries map[[2]string]uint64, states map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tries = make(map[[2]string]uint64, len(m.tries))
	for k, v := range m.tries {
		tries[k] = v
	}
	states = map[string]int{StateRunning: 0, StateWaiting: 0, StateDone: 0, StateStopped: 0, StateFailed: 0}
	for _, j := range m.jobs {
		states[j.State]++
	}
	return tries, states
}

func (m *Manager) update(j *job, f func(*Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f(&j.Job)
}

// end records how a job ended, and forgets the oldest ended jobs.
func (m *Manager) end(j *job, state, reason string) {
	m.mu.Lock()
	if j.stopped != "" && state != StateFailed {
		state, reason = StateStopped, j.stopped
	}
	j.State, j.Reason, j.Hint, j.Ended = state, reason, "", time.Now().UTC()
	j.cancel()
	select {
	case <-j.ended:
	default:
		close(j.ended)
	}
	if j.timer != nil {
		j.timer.Stop()
	}
	m.done = append(m.done, j.ID)
	for len(m.done) > MaxDone {
		delete(m.jobs, m.done[0])
		m.done = m.done[1:]
	}
	snap := j.snapshot()
	m.mu.Unlock()
	m.log.Info("job ended", "job", j.ID, "state", state, "reason", reason, "tries", snap.Tries, "layers", snap.Layers, "entries", snap.Entries)
	sp := j.Spec
	m.emit(Event{Job: j.ID, Kind: "end", State: state, Reason: reason, Tries: snap.Tries, From: snap.From, Scope: &sp})
}

// run does job j's work until ctx ends or its goal is reached.
func (m *Manager) run(ctx context.Context, j *job) {
	s := j.Spec
	var entry, tid hash.Hash
	var until *weight.Weight
	switch {
	case s.Target == TargetTip, s.Target == TargetPending:
	case strings.HasPrefix(s.Target, TargetTx):
		tid, _ = hash.Parse(strings.TrimPrefix(s.Target, TargetTx))
	default:
		entry, _ = hash.Parse(s.Target)
	}
	if s.UntilFrom != "" {
		var w weight.Weight
		_ = w.UnmarshalText([]byte(s.UntilFrom))
		until = &w
	}
	r := &runner{m: m, j: j, plans: map[hash.ID]Plan{}}
	for ctx.Err() == nil {
		if reason, hint, ok := outsideHours(s.Hours, time.Now()); ok {
			r.wait(ctx, reason, hint, time.Minute)
			continue
		}
		var worked bool
		var err error
		switch {
		case tid != (hash.Hash{}):
			var done bool
			done, worked, err = r.payment(ctx, tid, *until)
			if done {
				return
			}
		default:
			worked, err = r.pools(ctx, entry)
		}
		if err != nil {
			m.end(j, StateFailed, err.Error())
			return
		}
		if until != nil && entry != (hash.Hash{}) {
			if f, ok := m.fromOf(entry); ok {
				m.update(j, func(jj *Job) { jj.From = f.String() })
				if f.Cmp(*until) >= 0 {
					m.end(j, StateDone, "from(e) reached "+s.UntilFrom)
					return
				}
			}
		}
		if !worked {
			why, hint := r.idle(entry)
			r.wait(ctx, why, hint, time.Second)
			continue
		}
		r.running()
	}
	if s.For != "" && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		m.end(j, StateDone, "ran for "+s.For)
		return
	}
	m.end(j, StateStopped, "stopped")
}

// runner is one job's run: its plans and whether it is waiting.
type runner struct {
	m       *Manager
	j       *job
	plans   map[hash.ID]Plan
	waiting string
	// For a tx target: the entry the job built for the payment, and
	// whether the node has held the payment.
	built hash.Hash
	seen  bool
	// What the last pass had: no pools, or pools with nothing to do.
	noPools bool
}

// plan returns the plan for pool k's hash, choosing it the first time.
func (r *runner) plan(k engine.PoolKey) (Plan, bool, error) {
	sr, ok := r.m.n.Engine().Series(k.Series)
	if !ok {
		return Plan{}, false, nil
	}
	id := sr.Params().HashID
	if p, ok := r.plans[id]; ok {
		return p, true, nil
	}
	p, err := r.m.Choose(id, r.j.Spec.Device, r.j.Spec.Threads)
	if err != nil {
		return Plan{}, false, err
	}
	r.plans[id] = p
	r.m.update(r.j, func(jj *Job) { jj.Plans = append(jj.Plans, p) })
	pp := p
	r.m.emit(Event{Job: r.j.ID, Kind: "plan", Series: k.Series, Pool: k.Index, State: StateRunning, Reason: p.Reason, Plan: &pp})
	return p, true, nil
}

// pools does one slice on each pool of a tip, pending or entry job.
func (r *runner) pools(ctx context.Context, entry hash.Hash) (bool, error) {
	keys := r.m.targets(r.j.Spec, entry)
	r.noPools = len(keys) == 0
	worked := false
	for _, k := range keys {
		if ctx.Err() != nil {
			break
		}
		p, ok, err := r.plan(k)
		if err != nil || !ok {
			return worked, err
		}
		var did bool
		switch {
		case r.j.Spec.Target == TargetPending:
			did, _, err = r.m.build(ctx, r.j, k, p)
		case entry != (hash.Hash{}):
			did, err = r.m.work(ctx, r.j, k, p, entry)
		default:
			did, err = r.m.workTip(ctx, r.j, k, p)
		}
		if err != nil {
			return worked, err
		}
		worked = worked || did
	}
	return worked, nil
}

// payment does one slice for a tx target and reports whether the job
// ended. While the payment is pending it builds an entry for it (or adds
// layers to the entry it built, or to the best branch, which likely holds
// it); once selected it adds layers to its entry until from(e) reaches
// until.
func (r *runner) payment(ctx context.Context, tid hash.Hash, until weight.Weight) (done, worked bool, err error) {
	keys := r.m.targets(r.j.Spec, hash.Hash{})
	r.noPools = len(keys) == 0
	for _, k := range keys {
		pay, err := r.m.n.Engine().Payment(k, tid)
		if err != nil || (pay.Tx == nil && !(r.seen && r.built != (hash.Hash{}) && r.held(k, r.built))) {
			continue
		}
		r.seen = true
		p, ok, err := r.plan(k)
		if err != nil || !ok {
			return false, false, err
		}
		switch {
		case pay.Selected:
			eh := pay.Entry
			r.m.update(r.j, func(jj *Job) { jj.Entry, jj.From = &eh, pay.From.String() })
			if pay.From.Cmp(until) >= 0 {
				r.m.end(r.j, StateDone, "from(e) of the payment's entry reached "+r.j.Spec.UntilFrom)
				return true, false, nil
			}
			did, err := r.m.work(ctx, r.j, k, p, eh)
			return false, did, err
		case r.built != (hash.Hash{}) && r.held(k, r.built):
			did, err := r.m.work(ctx, r.j, k, p, r.built)
			return false, did, err
		case pay.Pending:
			did, eh, err := r.m.build(ctx, r.j, k, p)
			if err != nil || did {
				if eh != nil {
					r.built = *eh
				}
				return false, did, err
			}
			// Built already, by this node or a peer: weigh the best branch.
			did, err = r.m.workTip(ctx, r.j, k, p)
			return false, did, err
		}
	}
	if r.seen {
		r.m.end(r.j, StateFailed, "the payment is no longer pending or in any entry: dropped, or spent twice")
		return true, false, nil
	}
	return false, false, nil
}

// held reports whether pool k holds entry eh.
func (r *runner) held(k engine.PoolKey, eh hash.Hash) bool {
	held := false
	_ = r.m.n.Engine().Ledger(k, func(p *ledger.Pool) { held = p.Has(eh) })
	return held
}

// idle says why a pass found nothing to do, and what would help.
func (r *runner) idle(entry hash.Hash) (why, hint string) {
	s := r.j.Spec
	switch {
	case r.noPools && entry != (hash.Hash{}):
		return "no followed pool holds the entry yet", "wait for it from a peer, or check the hash with poo ledger"
	case r.noPools:
		return "no followed pool matches", "follow the pool (poo follow), or change -series and -pool"
	case strings.HasPrefix(s.Target, TargetTx) && !r.seen:
		return "the node does not hold the transaction yet", "send it (poo pay), or wait for it from a peer"
	case s.Target == TargetPending:
		return "no pending transactions to build an entry from", "claim or pay to make one, or wait for one from a peer"
	case s.Target == TargetTip:
		return "no entry to add layers to yet", "build one: target pending, or poo build"
	}
	return "nothing to work on yet", ""
}

// wait marks the job waiting for why (emitting an event when that
// changes) and sleeps for d or until ctx ends.
func (r *runner) wait(ctx context.Context, why, hint string, d time.Duration) {
	if r.waiting != why {
		r.waiting = why
		r.m.update(r.j, func(jj *Job) { jj.State, jj.Reason, jj.Hint, jj.Rate = StateWaiting, why, hint, 0 })
		r.m.mu.Lock()
		r.j.progAt, r.j.progTries = time.Time{}, r.j.Tries // the rate starts again
		r.m.mu.Unlock()
		sp := r.j.Spec
		r.m.emit(Event{Job: r.j.ID, Kind: "waiting", State: StateWaiting, Reason: why, Hint: hint, Scope: &sp})
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// running marks the job running again after waiting.
func (r *runner) running() {
	if r.waiting != "" {
		r.waiting = ""
		r.m.update(r.j, func(jj *Job) { jj.State, jj.Reason, jj.Hint = StateRunning, "", "" })
	}
}

// targets returns the followed pools job s works on now.
func (m *Manager) targets(s Spec, entry hash.Hash) []engine.PoolKey {
	var out []engine.PoolKey
	for _, k := range m.n.Engine().ListedPools() {
		if s.Series != nil && k.Series != *s.Series {
			continue
		}
		if len(s.Pools) > 0 && !slices.Contains(s.Pools, k.Index) {
			continue
		}
		if entry != (hash.Hash{}) {
			held := false
			_ = m.n.Engine().Ledger(k, func(p *ledger.Pool) { held = p.Has(entry) })
			if !held {
				continue
			}
		}
		out = append(out, k)
	}
	return out
}

func (m *Manager) fromOf(entry hash.Hash) (weight.Weight, bool) {
	for _, k := range m.targets(Spec{}, entry) {
		var w weight.Weight
		var err error
		_ = m.n.Engine().Ledger(k, func(p *ledger.Pool) { w, err = p.From(entry) })
		if err == nil {
			return w, true
		}
	}
	return weight.Weight{}, false
}

// build runs one search of at most Slice for an entry of pool k's pending
// transactions. It reports whether there was anything to build and the
// entry built. An error is returned only for a failure that ends the job.
func (m *Manager) build(ctx context.Context, j *job, k engine.PoolKey, p Plan) (bool, *hash.Hash, error) {
	st, err := m.n.Engine().Status(k)
	if err != nil || st.Pending == 0 {
		return false, nil, nil
	}
	var tried atomic.Uint64
	sctx, cancel := context.WithTimeout(ctx, Slice)
	defer cancel()
	t0 := time.Now()
	defer m.progress(j, k, t0, &tried)()
	_, res, err := m.n.BuildWith(sctx, k, 0, p.Engine(&tried))
	var found *hash.Hash
	did := false
	switch {
	case err == nil:
		did = true
		h := res.Hash
		found = &h
		m.update(j, func(jj *Job) { jj.Entries++ })
	case errors.Is(err, engine.ErrNothingPending), errors.Is(err, engine.ErrAlreadyBuilt):
		// Nothing new to build: an entry with these transactions is held
		// already, waiting for weight.
		return false, nil, nil
	case ctx.Err() == nil && errors.Is(err, sctx.Err()):
		did = true // the slice ran out: go on searching
	case ctx.Err() != nil:
	case errors.Is(err, node.ErrClosed):
		return did, nil, err
	default:
		m.log.Warn("build failed", "job", j.ID, "pool", k.String(), "err", err)
	}
	ev := Event{Job: j.ID, Series: k.Series, Pool: k.Index}
	if found != nil {
		ev.Kind, ev.Entry = "found", found
	}
	m.count(j, k, p, tried.Load(), t0, ev)
	return did, found, nil
}

// workTip works on pool k's selected tip, or its best branch while none
// is selected, such as an entry just built, which has no weight until a
// layer lands on it.
func (m *Manager) workTip(ctx context.Context, j *job, k engine.PoolKey, p Plan) (bool, error) {
	st, err := m.n.Engine().Status(k)
	switch {
	case err != nil:
		return false, nil
	case st.Height > 0:
		return m.work(ctx, j, k, p, st.Tip)
	case len(st.Branches) > 0:
		return m.work(ctx, j, k, p, st.Branches[0].Tip)
	}
	return false, nil
}

// work runs one search of at most Slice for a layer on entry target of
// pool k. An error is returned only for a failure that ends the job.
func (m *Manager) work(ctx context.Context, j *job, k engine.PoolKey, p Plan, target hash.Hash) (bool, error) {
	var tried atomic.Uint64
	sctx, cancel := context.WithTimeout(ctx, Slice)
	defer cancel()
	t0 := time.Now()
	ev := Event{Job: j.ID, Series: k.Series, Pool: k.Index}
	stop := m.progress(j, k, t0, &tried)
	g := p.Engine(&tried)
	var kept atomic.Uint64
	m.mu.Lock()
	g.Floor, g.KeptCount = m.def.Floor, &kept
	g.KeepMax = keepMax(m.def.Rate)
	m.mu.Unlock()
	l, res, err := m.n.WorkWith(sctx, k, target, randomNonce(), 0, g)
	if n := kept.Load(); n > 0 {
		m.update(j, func(jj *Job) { jj.Layers += int(n) })
	}
	switch {
	case err == nil:
		lh, eh := res.Hash, l.Entry
		ev.Kind, ev.Layer, ev.Entry, ev.Strength = "found", &lh, &eh, lh.Strength()
		m.update(j, func(jj *Job) { jj.Layers++ })
	case errors.Is(err, node.ErrClosed):
		return true, err
	case ctx.Err() != nil, errors.Is(err, engine.ErrSeen), errors.Is(err, engine.ErrNoStrongerLayer):
	default:
		m.log.Warn("work failed", "job", j.ID, "pool", k.String(), "err", err)
	}
	stop()
	m.count(j, k, p, tried.Load(), t0, ev)
	return true, nil
}

// count adds a search's tries to the job and the totals, emits ev when it
// found something, and a progress event at most every ProgressEvery, with
// the rate since the last one.
func (m *Manager) count(j *job, k engine.PoolKey, p Plan, n uint64, t0 time.Time, ev Event) {
	now := time.Now()
	m.mu.Lock()
	j.Tries += n
	m.tries[[2]string{p.Device, p.Hash}] += n
	total := j.Tries
	if j.progAt.IsZero() {
		j.progAt = t0
	}
	var prog *Event
	if el := now.Sub(j.progAt); el >= ProgressEvery {
		j.Rate = float64(total-j.progTries) / el.Seconds()
		j.progAt, j.progTries = now, total
		prog = &Event{Job: j.ID, Kind: "progress", Series: k.Series, Pool: k.Index, State: StateRunning, Tries: total, Rate: j.Rate}
	}
	rate := j.Rate
	m.mu.Unlock()
	if ev.Kind == "found" {
		ev.Tries, ev.Rate = total, rate
		m.emit(ev)
	}
	if prog != nil {
		m.emit(*prog)
	}
}

// progress reports job j's progress every ProgressEvery while a search on
// pool k, started at t0, runs, counting the tries the search has made so far, so that a
// job reports each second and not only at the end of each Slice
// (ORANGE-013). The function it returns stops it.
func (m *Manager) progress(j *job, k engine.PoolKey, t0 time.Time, tried *atomic.Uint64) func() {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(ProgressEvery)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				m.mu.Lock()
				total := j.Tries + tried.Load()
				if j.progAt.IsZero() {
					j.progAt, j.progTries = t0, j.Tries
				}
				var prog *Event
				if now.Sub(j.progAt) >= ProgressEvery {
					j.Rate = float64(total-j.progTries) / now.Sub(j.progAt).Seconds()
					j.progAt, j.progTries = now, total
					prog = &Event{Job: j.ID, Kind: "progress", Series: k.Series, Pool: k.Index, State: StateRunning, Tries: total, Rate: j.Rate}
				}
				m.mu.Unlock()
				if prog != nil {
					m.emit(*prog)
				}
			}
		}
	}()
	return func() { close(done) }
}

// keepMax is how many layers besides the strongest one search of a Slice
// keeps at rate layers a second (0: nodepolicy.DefaultLayerRate).
func keepMax(rate int) int {
	if rate == 0 {
		rate = nodepolicy.DefaultLayerRate
	}
	return max(1, int(float64(rate)*Slice.Seconds()))
}

// randomNonce is where a layer search starts, so slices and jobs on the
// same entry try different nonces.
func randomNonce() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return binary.BigEndian.Uint64(b[:]) >> 1 // room for the search ahead
}

// SpecOf is the spec of a job from the node policy.
func SpecOf(j nodepolicy.GrindJob) Spec {
	return Spec{Target: j.Target, Series: j.Series, Pools: j.Pools, Device: j.Device, Threads: j.Threads, Hours: j.Hours, UntilFrom: j.UntilFrom, Source: "policy"}
}

// outsideHours reports whether now is outside a job's hours, with the
// reason and hint a waiting job shows.
func outsideHours(hours string, now time.Time) (reason, hint string, ok bool) {
	if hours == "" {
		return "", "", false
	}
	from, to, err := ParseHours(hours)
	if err != nil || inHours(now, from, to) {
		return "", "", false
	}
	return "outside its hours " + hours, fmt.Sprintf("it works again at %02d:00", from), true
}
