package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// Grinding (milestone M11, decision D109):
//
//	GET    /v1/grind/devices  the devices and what auto picks
//	GET    /v1/grind          every job
//	POST   /v1/grind          grind.Spec: start a job
//	GET    /v1/grind/{id}     one job
//	DELETE /v1/grind/{id}     stop a job
//	DELETE /v1/grind          stop every job (?source=user: only those)
//	POST   /v1/grind/{id}/renew  renew a job's lease
//	GET    /v1/grind/schedule    node.json's grind section
//	PUT    /v1/grind/schedule    replace it: saved, and applied at once;
//	                             with If-Match, only if it is still that version
//	POST   /v1/grind/schedule/edit  add, remove or set, under the node's lock
//
// build and work take device, threads and for, and reply with the plan
// and rate. Starting and stopping are writes: the view token cannot.

// MaxGrindFor bounds how long one build or work request may grind, below
// the client's 5-minute timeout; longer work is a job.
const MaxGrindFor = 4 * time.Minute

// GrindOptions choose how a build or work request grinds. Empty fields
// take the node policy's grind section, then the defaults.
type GrindOptions struct {
	Device  string `json:"device,omitempty"`
	Threads int    `json:"threads,omitempty"`
	// For grinds for this long ("30s") instead of a number of tries.
	For string `json:"for,omitempty"`
}

// GrindStats is how a build or work request ground.
type GrindStats struct {
	grind.Plan
	Tries   uint64  `json:"tries"`
	Seconds float64 `json:"seconds"`
	Rate    float64 `json:"rate"`
}

// DevicesReply is the reply of /v1/grind/devices.
type DevicesReply struct {
	Devices []grind.Device `json:"devices"`
}

// SetGrind lets the interface run grind jobs on m. Without one, build and
// work still choose devices, and the job endpoints answer not found.
func (h *Handler) SetGrind(m *grind.Manager) { h.grind = m }

func (h *Handler) grindRoutes() {
	h.mux.HandleFunc("GET /v1/grind/devices", h.grindDevices)
	h.mux.HandleFunc("GET /v1/grind", h.grindJobs)
	h.mux.HandleFunc("POST /v1/grind", h.grindStart)
	h.mux.HandleFunc("GET /v1/grind/{id}", h.grindJob)
	h.mux.HandleFunc("DELETE /v1/grind/{id}", h.grindStop)
	h.mux.HandleFunc("DELETE /v1/grind", h.grindStopAll)
	h.mux.HandleFunc("POST /v1/grind/{id}/renew", h.grindRenew)
	h.mux.HandleFunc("GET /v1/grind/schedule", h.grindSchedule)
	h.mux.HandleFunc("PUT /v1/grind/schedule", h.grindSetSchedule)
	h.mux.HandleFunc("POST /v1/grind/schedule/edit", h.grindEditSchedule)
}

// ScheduleReply is the grind section of node.json and, after a change,
// the jobs it started and why any did not start.
type ScheduleReply struct {
	Grind nodepolicy.Grind `json:"grind"`
	// Version names Grind's content, for If-Match on a replacement and
	// for GrindEdit.Version (GUI-014, D125).
	Version string `json:"version"`
	// Limits are the bounds the section is checked against (D130).
	Limits  nodepolicy.GrindLimits `json:"limits"`
	Started []grind.Job            `json:"started,omitempty"`
	// Failed maps a job's index in Grind.Jobs to why it did not start.
	Failed map[int]string `json:"failed,omitempty"`
}

func (h *Handler) grindRenew(w http.ResponseWriter, r *http.Request) {
	if !h.needGrind(w) {
		return
	}
	id, ok := h.jobID(w, r)
	if !ok {
		return
	}
	j, err := h.grind.Renew(id)
	if err != nil {
		writeError(w, &Error{Message: fmt.Sprintf("no job %d", id), Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	writeJSON(w, j)
}

func (h *Handler) grindSchedule(w http.ResponseWriter, r *http.Request) {
	p, err := nodepolicy.Load(h.policyPath())
	if err != nil {
		writeError(w, classify(err))
		return
	}
	var g nodepolicy.Grind
	if p.Grind != nil {
		g = *p.Grind
	}
	writeJSON(w, ScheduleReply{Grind: g, Version: g.Version(), Limits: nodepolicy.Limits()})
}

// grindSetSchedule saves a new grind section and applies it without a
// reload, so running user jobs go on. With an If-Match header it replaces
// only the version named, so a client that read the section, changed it
// and writes it back whole cannot undo a change made meanwhile (GUI-014,
// D124).
func (h *Handler) grindSetSchedule(w http.ResponseWriter, r *http.Request) {
	var g nodepolicy.Grind
	if e := decodeBody(r, &g); e != nil {
		writeError(w, e)
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	h.changeSchedule(w, func(cur nodepolicy.Grind) (nodepolicy.Grind, error) {
		if match != "" && match != cur.Version() {
			return cur, nodepolicy.ErrGrindChanged
		}
		return g, nil
	})
}

// grindEditSchedule applies one GrindEdit to the grind section where it
// is kept, under the lock, so two edits made at once both take effect,
// and removes a job only if it is still the job named (GUI-014, D125).
func (h *Handler) grindEditSchedule(w http.ResponseWriter, r *http.Request) {
	var e nodepolicy.GrindEdit
	if err := decodeBody(r, &e); err != nil {
		writeError(w, err)
		return
	}
	h.changeSchedule(w, e.Apply)
}

// changeSchedule loads node.json's grind section, changes it with f,
// validates, saves and applies the result. The file and the running
// policy jobs change together under policyMu, so two changes leave the
// same section in both (RED-013).
func (h *Handler) changeSchedule(w http.ResponseWriter, f func(nodepolicy.Grind) (nodepolicy.Grind, error)) {
	path := h.policyPath()
	h.policyMu.Lock()
	defer h.policyMu.Unlock()
	p, err := nodepolicy.Load(path)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	var cur nodepolicy.Grind
	if p.Grind != nil {
		cur = *p.Grind
	}
	g, err := f(cur)
	if errors.Is(err, nodepolicy.ErrGrindChanged) {
		writeError(w, &Error{Message: err.Error(), Code: CodeConflict, Status: http.StatusConflict})
		return
	}
	if err != nil {
		writeError(w, badRequest("%v", err))
		return
	}
	p.Grind = &g
	if g.IsZero() {
		p.Grind = nil
	}
	if err := p.Validate(); err != nil {
		writeError(w, &Error{Message: err.Error(), Code: CodeInvalid, Status: http.StatusUnprocessableEntity})
		return
	}
	if err := nodepolicy.Save(path, p); err != nil {
		writeError(w, classify(err))
		return
	}
	reply := ScheduleReply{Grind: g, Version: g.Version(), Limits: nodepolicy.Limits()}
	if h.grind != nil {
		started, failed := h.grind.Schedule(p.Grind)
		// The grind manager runs this schedule now (RED-080).
		h.n.UpdateRunning(func(run *nodepolicy.Policy) { run.Grind = p.Grind })
		reply.Started = started
		for i, err := range failed {
			if reply.Failed == nil {
				reply.Failed = map[int]string{}
			}
			reply.Failed[i] = err.Error()
		}
	}
	writeJSON(w, reply)
}

var (
	localOnce sync.Once
	localDevs []grind.Device
)

// devices returns the manager's devices, or detects them once when the
// interface has no manager.
func (h *Handler) devices() []grind.Device {
	if h.grind != nil {
		return h.grind.Devices()
	}
	localOnce.Do(func() { localDevs = grind.Default.Detect() })
	return localDevs
}

// plan chooses how a request on pool k grinds.
func (h *Handler) plan(k engine.PoolKey, o GrindOptions) (grind.Plan, time.Duration, *Error) {
	var d time.Duration
	if o.For != "" {
		var err error
		if d, err = time.ParseDuration(o.For); err != nil || d <= 0 || d > MaxGrindFor {
			return grind.Plan{}, 0, badRequest("for must be a duration up to %s; use a grind job for longer", MaxGrindFor)
		}
	}
	s, ok := h.n.Engine().Series(k.Series)
	if !ok {
		return grind.Plan{}, 0, classify(engine.ErrUnknownSeries)
	}
	id := s.Params().HashID
	var p grind.Plan
	var err error
	if h.grind != nil {
		p, err = h.grind.Choose(id, o.Device, o.Threads)
	} else {
		def := h.n.Policy().Grind
		if def != nil && o.Device == "" {
			o.Device = def.Device
		}
		if def != nil && o.Threads == 0 {
			o.Threads = def.Threads
		}
		p, err = grind.Default.Choose(h.devices(), id, o.Device, o.Threads)
	}
	if err != nil {
		return grind.Plan{}, 0, &Error{Message: err.Error(), Code: CodeBadRequest, Status: http.StatusBadRequest}
	}
	return p, d, nil
}

// grindCtx returns the context and engine.Grind for a request: bounded by
// d when set, counting tries.
func grindCtx(r *http.Request, p grind.Plan, d time.Duration, tried *atomic.Uint64) (context.Context, context.CancelFunc, engine.Grind) {
	ctx, cancel := r.Context(), context.CancelFunc(func() {})
	if d > 0 {
		ctx, cancel = context.WithTimeout(ctx, d)
	}
	return ctx, cancel, p.Engine(tried)
}

func stats(p grind.Plan, tried uint64, t0 time.Time) *GrindStats {
	secs := time.Since(t0).Seconds()
	return &GrindStats{Plan: p, Tries: tried, Seconds: secs, Rate: float64(tried) / max(secs, 1e-9)}
}

func (h *Handler) grindDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, DevicesReply{Devices: h.devices()})
}

func (h *Handler) needGrind(w http.ResponseWriter) bool {
	if h.grind == nil {
		writeError(w, &Error{Message: "this node runs no grind jobs", Code: CodeNotFound, Status: http.StatusNotFound})
		return false
	}
	return true
}

func (h *Handler) grindJobs(w http.ResponseWriter, r *http.Request) {
	if !h.needGrind(w) {
		return
	}
	writeJSON(w, h.grind.Jobs())
}

func (h *Handler) grindStart(w http.ResponseWriter, r *http.Request) {
	if !h.needGrind(w) {
		return
	}
	var s grind.Spec
	if e := decodeBody(r, &s); e != nil {
		writeError(w, e)
		return
	}
	// A job made here is the user's: any other label would escape
	// DELETE /v1/grind?source=user (RED-011 notes, decision D111).
	if s.Source != "" && s.Source != "user" {
		writeError(w, badRequest("source %q: jobs made here are the user's (policy is for jobs from node.json)", s.Source))
		return
	}
	j, err := h.grind.Start(s)
	switch {
	case errors.Is(err, grind.ErrTooManyJobs):
		writeError(w, &Error{Message: err.Error(), Code: CodeFull, Status: http.StatusServiceUnavailable})
	case err != nil:
		writeError(w, badRequest("%v", err))
	default:
		writeJSON(w, j)
	}
}

func (h *Handler) jobID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, badRequest("job id: not a number"))
		return 0, false
	}
	return id, true
}

func (h *Handler) grindJob(w http.ResponseWriter, r *http.Request) {
	if !h.needGrind(w) {
		return
	}
	id, ok := h.jobID(w, r)
	if !ok {
		return
	}
	j, ok := h.grind.Get(id)
	if !ok {
		writeError(w, &Error{Message: fmt.Sprintf("no job %d", id), Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	writeJSON(w, j)
}

func (h *Handler) grindStop(w http.ResponseWriter, r *http.Request) {
	if !h.needGrind(w) {
		return
	}
	id, ok := h.jobID(w, r)
	if !ok {
		return
	}
	j, err := h.grind.Stop(id)
	if err != nil {
		writeError(w, &Error{Message: fmt.Sprintf("no job %d", id), Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	writeJSON(w, j)
}

func (h *Handler) grindStopAll(w http.ResponseWriter, r *http.Request) {
	if !h.needGrind(w) {
		return
	}
	writeJSON(w, h.grind.StopAll(r.URL.Query().Get("source")))
}

// grindMetrics returns the grind metric families.
func (h *Handler) grindMetrics() []*metric {
	if h.grind == nil {
		return nil
	}
	tries, states := h.grind.Totals()
	t := &metric{name: "poo_grind_tries_total", help: "Nonces tried by grind jobs, by device and hash.", kind: "counter"}
	keys := make([][2]string, 0, len(tries))
	for k := range tries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
	for _, k := range keys {
		t.add(fmt.Sprintf(`{device="%s",hash="%s"}`, labelValue(k[0]), labelValue(k[1])), float64(tries[k]))
	}
	j := &metric{name: "poo_grind_jobs", help: "Grind jobs the node remembers, by state.", kind: "gauge"}
	names := make([]string, 0, len(states))
	for s := range states {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		j.add(fmt.Sprintf(`{state="%s"}`, s), float64(states[s]))
	}
	return []*metric{t, j}
}
