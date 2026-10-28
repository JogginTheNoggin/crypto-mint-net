package grind

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
)

// Spec says what a job does. The user names a target, never a hash.
type Spec struct {
	// Target is TargetTip, TargetPending, or an entry hash in hex: layers
	// on that one entry.
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
	// UntilFrom stops a job on one entry once from(e) reaches this
	// weight: "until my payment is safe".
	UntilFrom string `json:"until_from,omitempty"`
	// Hours limits the work to a window of the node's local hours, "22-06"
	// (from 22:00 to 06:00); empty works at any hour.
	Hours string `json:"hours,omitempty"`
	// Source says who made the job: "user" (the default) or "policy".
	Source string `json:"source,omitempty"`
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
	// From is the target entry's from(e), for a job on one entry.
	From string `json:"from,omitempty"`
}

// Done reports whether the job has ended.
func (j Job) Done() bool {
	return j.State == StateDone || j.State == StateStopped || j.State == StateFailed
}

// Event reports what a job did. Kind is start, progress, found, waiting
// or end.
type Event struct {
	Job    int       `json:"job"`
	Kind   string    `json:"kind"`
	Series hash.Hash `json:"series,omitzero"`
	Pool   uint64    `json:"pool,omitempty"`
	State  string    `json:"state,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Plan   *Plan     `json:"plan,omitempty"`
	Tries  uint64    `json:"tries,omitempty"`
	Rate   float64   `json:"rate,omitempty"`
	// Found: the layer or entry added, and the layer's strength.
	Layer    *hash.Hash `json:"layer,omitempty"`
	Entry    *hash.Hash `json:"entry,omitempty"`
	Strength int        `json:"strength,omitempty"`
	From     string     `json:"from,omitempty"`
}

// Limits on jobs.
const (
	// MaxJobs bounds the jobs a node runs at once.
	MaxJobs = nodepolicy.MaxGrindJobs
	// MaxDone is how many ended jobs a node remembers.
	MaxDone = 32
	// Slice is how long one search runs before the job looks again at its
	// target, its hours and its goal.
	Slice = 2 * time.Second
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

	mu        sync.Mutex
	jobs      map[int]*job
	done      []int // ended jobs, oldest first
	next      int
	listeners []func(Event)
	tries     map[[2]string]uint64 // by device and hash
}

type job struct {
	Job
	cancel  context.CancelFunc
	stopped bool
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
	if device == "" {
		device = m.def.Device
	}
	if threads == 0 {
		threads = m.def.Threads
	}
	return m.reg.Choose(m.devs, id, device, threads)
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
	switch s.Target {
	case TargetTip, TargetPending:
		if s.UntilFrom != "" {
			return errors.New("grind: until_from needs an entry as the target")
		}
	case "":
		return errors.New("grind: a target is needed: tip, pending or an entry hash")
	default:
		if _, err := hash.Parse(s.Target); err != nil {
			return fmt.Errorf("grind: target is tip, pending or an entry hash: %v", err)
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
	if running >= MaxJobs {
		m.mu.Unlock()
		return Job{}, ErrTooManyJobs
	}
	ctx, cancel := context.WithCancel(m.ctx)
	if s.For != "" {
		d, _ := time.ParseDuration(s.For)
		ctx, cancel = context.WithTimeout(m.ctx, d)
	}
	j := &job{Job: Job{ID: m.next, Spec: s, State: StateRunning, Started: time.Now().UTC(), Plans: []Plan{}}, cancel: cancel}
	m.next++
	m.jobs[j.ID] = j
	snap := j.Job
	m.mu.Unlock()
	m.log.Info("grind job started", "job", j.ID, "target", s.Target, "source", s.Source)
	m.emit(Event{Job: j.ID, Kind: "start", State: StateRunning})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(ctx, j)
	}()
	return snap, nil
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
	if ok && !j.Done() {
		j.stopped = true
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
	for i := 0; i < 200; i++ {
		if j, _ := m.Get(id); j.Done() {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := m.Get(id)
	return j
}

// StopAll stops every running job made by source ("" for all).
func (m *Manager) StopAll(source string) []Job {
	m.mu.Lock()
	var ids []int
	for id, j := range m.jobs {
		if !j.Done() && (source == "" || j.Spec.Source == source) {
			j.stopped = true
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
		j.stopped = true
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
	if j.stopped && state != StateFailed {
		state, reason = StateStopped, "stopped"
	}
	j.State, j.Reason, j.Ended = state, reason, time.Now().UTC()
	m.done = append(m.done, j.ID)
	for len(m.done) > MaxDone {
		delete(m.jobs, m.done[0])
		m.done = m.done[1:]
	}
	snap := j.snapshot()
	m.mu.Unlock()
	m.log.Info("grind job ended", "job", j.ID, "state", state, "reason", reason, "tries", snap.Tries, "layers", snap.Layers, "entries", snap.Entries)
	m.emit(Event{Job: j.ID, Kind: "end", State: state, Reason: reason, Tries: snap.Tries})
}

// run does job j's work until ctx ends or its goal is reached.
func (m *Manager) run(ctx context.Context, j *job) {
	s := j.Spec
	var entry hash.Hash
	var until *weight.Weight
	if s.Target != TargetTip && s.Target != TargetPending {
		entry, _ = hash.Parse(s.Target)
		if s.UntilFrom != "" {
			var w weight.Weight
			_ = w.UnmarshalText([]byte(s.UntilFrom))
			until = &w
		}
	}
	from, to, _ := ParseHours(s.Hours)
	plans := map[hash.ID]Plan{}
	waiting := ""
	for ctx.Err() == nil {
		if s.Hours != "" && !inHours(time.Now(), from, to) {
			waiting = m.wait1(ctx, j, waiting, "outside its hours "+s.Hours, time.Minute)
			continue
		}
		keys := m.targets(s, entry)
		if len(keys) == 0 {
			what := "no followed pool matches"
			if entry != (hash.Hash{}) {
				what = "no followed pool holds the entry yet"
			}
			waiting = m.wait1(ctx, j, waiting, what, time.Second)
			continue
		}
		worked := false
		for _, k := range keys {
			if ctx.Err() != nil {
				break
			}
			sr, ok := m.n.Engine().Series(k.Series)
			if !ok {
				continue
			}
			id := sr.Params().HashID
			p, ok := plans[id]
			if !ok {
				var err error
				if p, err = m.Choose(id, s.Device, s.Threads); err != nil {
					m.end(j, StateFailed, err.Error())
					return
				}
				plans[id] = p
				m.update(j, func(jj *Job) { jj.Plans = append(jj.Plans, p) })
				pp := p
				m.emit(Event{Job: j.ID, Kind: "start", Series: k.Series, Pool: k.Index, State: StateRunning, Reason: p.Reason, Plan: &pp})
			}
			did, err := m.slice(ctx, j, k, p, entry)
			if err != nil {
				m.end(j, StateFailed, err.Error())
				return
			}
			worked = worked || did
		}
		if until != nil {
			if f, ok := m.fromOf(entry); ok {
				m.update(j, func(jj *Job) { jj.From = f.String() })
				if f.Cmp(*until) >= 0 {
					m.end(j, StateDone, "from(e) reached "+s.UntilFrom)
					return
				}
			}
		}
		if !worked {
			waiting = m.wait1(ctx, j, waiting, "nothing to work on yet", time.Second)
			continue
		}
		if waiting != "" {
			waiting = ""
			m.update(j, func(jj *Job) { jj.State, jj.Reason = StateRunning, "" })
		}
	}
	if s.For != "" && !j.stopped && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		m.end(j, StateDone, "ran for "+s.For)
		return
	}
	m.end(j, StateStopped, "stopped")
}

// wait1 marks j waiting for why (emitting an event when that changes) and
// sleeps for d or until ctx ends.
func (m *Manager) wait1(ctx context.Context, j *job, was, why string, d time.Duration) string {
	if was != why {
		m.update(j, func(jj *Job) { jj.State, jj.Reason, jj.Rate = StateWaiting, why, 0 })
		m.emit(Event{Job: j.ID, Kind: "waiting", State: StateWaiting, Reason: why})
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
	return why
}

// targets returns the followed pools job s works on now.
func (m *Manager) targets(s Spec, entry hash.Hash) []engine.PoolKey {
	var out []engine.PoolKey
	for _, k := range m.n.Engine().FollowedPools() {
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

// slice runs one search of at most Slice on pool k and reports whether
// there was anything to work on. An error is returned only for a failure
// that ends the job.
func (m *Manager) slice(ctx context.Context, j *job, k engine.PoolKey, p Plan, entry hash.Hash) (bool, error) {
	var tried atomic.Uint64
	sctx, cancel := context.WithTimeout(ctx, Slice)
	defer cancel()
	t0 := time.Now()
	ev := Event{Job: j.ID, Series: k.Series, Pool: k.Index}
	var did bool
	switch j.Spec.Target {
	case TargetPending:
		st, err := m.n.Engine().Status(k)
		if err != nil || st.Pending == 0 {
			return false, nil
		}
		_, res, err := m.n.BuildWith(sctx, k, 0, p.Engine(&tried))
		switch {
		case err == nil:
			did = true
			h := res.Hash
			ev.Kind, ev.Entry = "found", &h
			m.update(j, func(jj *Job) { jj.Entries++ })
		case errors.Is(err, engine.ErrNothingPending), errors.Is(err, engine.ErrAlreadyBuilt):
			// Nothing new to build: an entry with these transactions is
			// held already, waiting for weight.
			return false, nil
		case ctx.Err() == nil && errors.Is(err, sctx.Err()):
			did = true // the slice ran out: go on searching
		case ctx.Err() != nil:
		default:
			if errors.Is(err, node.ErrClosed) {
				return did, err
			}
			m.log.Warn("grind build failed", "job", j.ID, "pool", k.String(), "err", err)
		}
	default:
		target := entry
		if j.Spec.Target == TargetTip {
			st, err := m.n.Engine().Status(k)
			switch {
			case err != nil:
				return false, nil
			case st.Height > 0:
				target = st.Tip
			case len(st.Branches) > 0:
				// No entry is selected yet: work on the best branch, such
				// as an entry just built, which has no weight until a
				// layer lands on it.
				target = st.Branches[0].Tip
			default:
				return false, nil
			}
		}
		did = true
		l, res, err := m.n.WorkWith(sctx, k, target, randomNonce(), 0, p.Engine(&tried))
		switch {
		case err == nil:
			lh, eh := res.Hash, l.Entry
			ev.Kind, ev.Layer, ev.Entry, ev.Strength = "found", &lh, &eh, lh.Strength()
			m.update(j, func(jj *Job) { jj.Layers++ })
		case errors.Is(err, node.ErrClosed):
			return did, err
		case ctx.Err() != nil, errors.Is(err, engine.ErrSeen), errors.Is(err, engine.ErrNoStrongerLayer):
		default:
			m.log.Warn("grind work failed", "job", j.ID, "pool", k.String(), "err", err)
		}
	}
	n := tried.Load()
	rate := float64(n) / max(time.Since(t0).Seconds(), 1e-9)
	m.mu.Lock()
	j.Tries += n
	j.Rate = rate
	m.tries[[2]string{p.Device, p.Hash}] += n
	total := j.Tries
	m.mu.Unlock()
	if ev.Kind == "found" {
		ev.Tries, ev.Rate = total, rate
		m.emit(ev)
	}
	m.emit(Event{Job: j.ID, Kind: "progress", Series: k.Series, Pool: k.Index, State: StateRunning, Tries: total, Rate: rate})
	return did, nil
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
