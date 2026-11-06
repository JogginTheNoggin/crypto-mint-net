// Package grind does a node's hash work: it finds nonces for entries and
// layers as fast as the machine allows (milestone M11, decision D109).
//
// It has three parts, each open to extension without touching its
// callers:
//
//   - Devices: what hardware there is (Detect). A probe per kind of
//     device finds it: the CPU always, GPUs where the system lists them.
//     RegisterProbe adds a kind.
//   - Grinders: one per hash and kind of device, in a Registry. The CPU has
//     a fast grinder for every hash package hash knows (a hash.Grinder,
//     which keeps the digest's state after the fixed prefix), and a
//     generic one that works for any hash, so a hash added to package
//     hash can be ground at once and gets a fast grinder by registering
//     one. Register adds a grinder.
//   - Plans: Choose picks a device and a grinder for a pool's hash, by
//     name or automatically, and says why, so a command can show what was
//     picked. A Plan turns into the engine.Grind that the node's build
//     and work take.
//
// Jobs (Manager) run that work inside a node, by hand, on a schedule or
// from the node policy, against targets: the selected tip of some pools,
// their pending transactions, or one entry. The user names a target, never
// a hash: the series of the target says which hash to grind.
//
// Grinding is outside consensus: it only finds nonces sooner. Every
// search returns what the one-goroutine CPU search returns for the same
// nonces.
package grind

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// Kind is a kind of device.
type Kind string

// The kinds of device this package knows of. A probe may report others.
const (
	CPU Kind = "cpu"
	GPU Kind = "gpu"
)

// Auto is the device name that lets Choose pick.
const Auto = "auto"

// Device is one piece of hardware that could grind.
type Device struct {
	// ID names the device in commands: "cpu", "gpu:0", ...
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
	// Threads is how many searches it runs at once (the CPU's logical
	// cores).
	Threads int `json:"threads,omitempty"`
	// Features are the instruction sets or capabilities that matter for
	// hashing (for the CPU: sha, avx2, avx512f, asimd, sha2, sha3).
	Features []string `json:"features,omitempty"`
	// Hashes names the hashes this device has a fast grinder for, and
	// Generic says whether it can grind any other hash more slowly.
	Hashes  []string `json:"hashes"`
	Generic bool     `json:"generic,omitempty"`
	// AutoFor names the hashes for which "auto" picks this device.
	AutoFor []string `json:"auto_for,omitempty"`
	// Note says anything a user should know, such as a device found but
	// without a grinder yet.
	Note string `json:"note,omitempty"`

	// cl is the index+1 of the OpenCL device that is this device, 0 for
	// none (D208).
	cl int
}

// Grinder grinds one hash, or any hash, on one kind of device.
type Grinder struct {
	// Name names it in replies: "cpu-midstate", "cpu-generic", ...
	Name string
	Kind Kind
	// Hash is the hash it grinds, or 0 for a generic grinder that grinds
	// any hash, more slowly.
	Hash hash.ID
	// New returns the hasher maker for device d and hash id.
	New func(d Device, id hash.ID) (hash.NewHasher, error)
	// Search, if set, returns a layer search for device d and hash id
	// that tries many nonces at once, such as with SIMD lanes; layers are
	// searched with it and entries still with New (ORANGE-012).
	Search func(d Device, id hash.ID) (layer.Search, error)
	// SearchContext, if set, is Search for a search that sees the grind's
	// context (RED-124); it is used instead of Search.
	SearchContext func(d Device, id hash.ID) (layer.SearchContext, error)
	// Has, if set, says whether this grinder runs on device d: a GPU
	// grinder runs only on a GPU its driver lists (D208).
	Has func(d Device) bool
	// Chunk, if set, is how many nonces Search takes at a time on device
	// d (0: the CPU's 4,096); a GPU takes millions (D208).
	Chunk func(d Device, id hash.ID) uint64
}

// Registry holds the grinders and device probes. The zero value is empty;
// Default holds the built-in ones.
type Registry struct {
	mu       sync.RWMutex
	grinders map[Kind]map[hash.ID]Grinder
	probes   []func() []Device
}

// Default is the registry the node uses.
var Default = newDefault()

// Register adds g, replacing any grinder for the same kind and hash.
func (r *Registry) Register(g Grinder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.grinders == nil {
		r.grinders = map[Kind]map[hash.ID]Grinder{}
	}
	if r.grinders[g.Kind] == nil {
		r.grinders[g.Kind] = map[hash.ID]Grinder{}
	}
	r.grinders[g.Kind][g.Hash] = g
}

// RegisterProbe adds a function that reports devices of some kind.
func (r *Registry) RegisterProbe(p func() []Device) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probes = append(r.probes, p)
}

// grinder returns the grinder for device d and hash id: the fast one,
// else the generic one (fallback true).
func (r *Registry) grinder(d Device, id hash.ID) (g Grinder, fallback, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	runs := func(g Grinder) bool { return g.Has == nil || g.Has(d) }
	if g, ok := r.grinders[d.Kind][id]; ok && runs(g) {
		return g, false, true
	}
	if g, ok := r.grinders[d.Kind][0]; ok && runs(g) {
		return g, true, true
	}
	return Grinder{}, false, false
}

// Detect returns every device the probes find, the CPU first, each with
// the hashes it can grind and those auto picks it for.
func (r *Registry) Detect() []Device {
	r.mu.RLock()
	probes := slices.Clone(r.probes)
	r.mu.RUnlock()
	// Two probes may find the same device (the system's list and a
	// grinder's own); an id names one device, so the first one found wins.
	var devs []Device
	seen := map[string]bool{}
	for _, p := range probes {
		for _, d := range p() {
			if !seen[d.ID] {
				seen[d.ID] = true
				devs = append(devs, d)
			}
		}
	}
	sort.SliceStable(devs, func(i, j int) bool { return devs[i].Kind == CPU && devs[j].Kind != CPU })
	for i := range devs {
		d := &devs[i]
		d.Hashes = []string{}
		for _, id := range hash.IDs() {
			if g, fallback, ok := r.grinder(*d, id); ok && !fallback && g.Hash == id {
				d.Hashes = append(d.Hashes, id.Name())
			}
		}
		d.Generic = r.hasGeneric(*d)
		if len(d.Hashes) == 0 && !d.Generic && d.Note == "" {
			d.Note = "found, but no grinder runs on it yet; auto uses another device"
		}
	}
	for _, id := range hash.IDs() {
		if i, ok := r.autoIndex(devs, id); ok {
			devs[i].AutoFor = append(devs[i].AutoFor, id.Name())
		}
	}
	return devs
}

func (r *Registry) hasGeneric(d Device) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.grinders[d.Kind][0]
	return ok && (g.Has == nil || g.Has(d))
}

// autoIndex returns the device auto picks for hash id: a device other
// than the CPU with a fast grinder for it (an accelerator is meant to be
// faster), else the CPU with a fast one, else any device with a generic
// one.
func (r *Registry) autoIndex(devs []Device, id hash.ID) (int, bool) {
	best, bestRank := -1, 0
	for i, d := range devs {
		_, fallback, ok := r.grinder(d, id)
		if !ok {
			continue
		}
		rank := 1 // generic
		switch {
		case !fallback && d.Kind != CPU:
			rank = 3
		case !fallback:
			rank = 2
		}
		if rank > bestRank {
			best, bestRank = i, rank
		}
	}
	return best, best >= 0
}

// Plan is how one search grinds: on which device, with which grinder and
// how many threads.
type Plan struct {
	Device  string `json:"device"`
	Kind    Kind   `json:"kind"`
	Grinder string `json:"grinder"`
	Hash    string `json:"hash"`
	Threads int    `json:"threads"`
	// Fallback is true when the device has no fast grinder for the hash
	// and the generic one runs; Reason says why this device and grinder.
	Fallback bool   `json:"fallback,omitempty"`
	Reason   string `json:"reason"`

	newHasher hash.NewHasher
	search    layer.SearchContext
	chunk     uint64
	// entryThreads grind entries when the device is not the CPU: a GPU
	// searches layers, and entries are ground on the CPU (D208).
	entryThreads int
}

// ErrNoDevice is returned for a device that is not there, and
// ErrNoGrinder for one that is there but has no grinder for the hash
// (jerry's finding 12: "no such device" misled).
var (
	ErrNoDevice  = errors.New("grind: no such device")
	ErrNoGrinder = errors.New("grind: no grinder")
)

// MaxThreads bounds the threads one search may ask for.
const MaxThreads = nodepolicy.MaxGrindThreads

// Choose picks the device and grinder for hash id among devs: the device
// named want, or the one auto picks when want is "" or "auto". threads 0
// takes the device's default: for the CPU every logical core but one,
// so the node stays responsive.
func (r *Registry) Choose(devs []Device, id hash.ID, want string, threads int) (Plan, error) {
	if id.Name() == "" {
		return Plan{}, fmt.Errorf("grind: %w: %d", hash.ErrUnknown, id)
	}
	if threads < 0 || threads > MaxThreads {
		return Plan{}, fmt.Errorf("grind: threads must be 0 to %d", MaxThreads)
	}
	var d Device
	var why string
	if want == "" || want == Auto {
		i, ok := r.autoIndex(devs, id)
		if !ok {
			return Plan{}, fmt.Errorf("%w: none can grind %s", ErrNoDevice, id.Name())
		}
		d = devs[i]
		why = "auto: "
	} else {
		i := slices.IndexFunc(devs, func(d Device) bool { return d.ID == want })
		if i < 0 {
			ids := make([]string, len(devs))
			for i, d := range devs {
				ids[i] = d.ID
			}
			return Plan{}, fmt.Errorf("%w %q (there are: %s, auto)", ErrNoDevice, want, strings.Join(ids, ", "))
		}
		d = devs[i]
		why = "asked for: "
	}
	g, fallback, ok := r.grinder(d, id)
	if !ok {
		why := ""
		if d.Note != "" {
			why = " (" + d.Note + ")"
		}
		return Plan{}, fmt.Errorf("%w: %s has no grinder for %s%s", ErrNoGrinder, d.ID, id.Name(), why)
	}
	nh, err := g.New(d, id)
	if err != nil {
		return Plan{}, err
	}
	var search layer.SearchContext
	var chunk uint64
	switch {
	case g.SearchContext != nil:
		if search, err = g.SearchContext(d, id); err != nil {
			return Plan{}, fmt.Errorf("grind: %s: %w", d.ID, err)
		}
	case g.Search != nil:
		s, err := g.Search(d, id)
		if err != nil {
			return Plan{}, fmt.Errorf("grind: %s: %w", d.ID, err)
		}
		search = func(_ context.Context, e hash.Hash, from, n uint64) (uint64, int) { return s(e, from, n) }
	}
	if search != nil && g.Chunk != nil {
		chunk = g.Chunk(d, id)
	}
	switch {
	case fallback:
		why += fmt.Sprintf("%s has no fast grinder for %s, so the generic one runs", d.ID, id.Name())
	case d.Kind == CPU:
		why += fmt.Sprintf("the %s grinder for %s", g.Name, id.Name())
	default:
		why += fmt.Sprintf("%s has a %s grinder for %s", d.ID, g.Name, id.Name())
	}
	if threads == 0 {
		threads = defaultThreads(d)
	}
	if d.Threads > 0 && threads > d.Threads {
		threads = d.Threads
	}
	p := Plan{Device: d.ID, Kind: d.Kind, Grinder: g.Name, Hash: id.Name(), Threads: threads, Fallback: fallback, Reason: why, newHasher: nh, search: search, chunk: chunk}
	if d.Kind != CPU {
		p.entryThreads = max(1, threadsOf()-1)
	}
	return p, nil
}

func defaultThreads(d Device) int {
	if d.Kind == CPU {
		return max(1, d.Threads-1)
	}
	return max(1, d.Threads)
}

// Engine returns the engine.Grind for this plan, counting tries in tried
// if not nil.
func (p Plan) Engine(tried *atomic.Uint64) engine.Grind {
	return engine.Grind{New: p.newHasher, Workers: p.Threads, Tried: tried, SearchContext: p.search, Chunk: p.chunk, EntryWorkers: p.entryThreads}
}

// newDefault returns a registry with the CPU probe, the GPU probe, a fast
// CPU grinder for every known hash and the generic CPU grinder.
func newDefault() *Registry {
	r := &Registry{}
	r.RegisterProbe(probeCPU)
	r.RegisterProbe(func() []Device { return gpuProbe() })
	for _, id := range hash.IDs() {
		r.Register(Grinder{Name: "cpu-midstate", Kind: CPU, Hash: id, New: func(_ Device, id hash.ID) (hash.NewHasher, error) {
			return hash.GrinderFor(id), nil
		}})
	}
	r.Register(Grinder{Name: "cpu-blake3-cv", Kind: CPU, Hash: hash.BLAKE3, New: func(Device, hash.ID) (hash.NewHasher, error) {
		return newBLAKE3Best, nil
	}})
	r.Register(Grinder{Name: "cpu-generic", Kind: CPU, New: genericCPU})
	return r
}

// genericCPU hashes through hash.Lookup, which any hash package hash
// implements, at the cost of writing the whole prefix for every try.
func genericCPU(_ Device, id hash.ID) (hash.NewHasher, error) {
	f, err := hash.Lookup(id)
	if err != nil {
		return nil, err
	}
	return func(prefix ...[]byte) (hash.Hasher, error) {
		var p []byte
		for _, b := range prefix {
			p = append(p, b...)
		}
		return funcHasher{f: f, prefix: p}, nil
	}, nil
}

type funcHasher struct {
	f      hash.Func
	prefix []byte
}

func (h funcHasher) Hash(suffix []byte) hash.Hash { return h.f(h.prefix, suffix) }

// gpuProbe lists the GPUs: probeGPUs, or the OpenCL probe where the GPU
// grinder is built in (D208).
var gpuProbe = probeGPUs

// openCLStatus says what the GPU grinder found; the OpenCL build sets it.
var openCLStatus = func() string { return "this build has no GPU grinder (OpenCL)" }

// OpenCLStatus says, in one line, what the GPU grinder found: how many
// OpenCL devices, or why none (turned off, no library, no driver). poon
// grind devices and doctor show it, so a machine whose GPU is not listed
// says why even when no GPU was found to hang a note on (GREEN-066).
func OpenCLStatus() string { return openCLStatus() }

// openCLStatusOf is OpenCLStatus from what the OpenCL probe found.
func openCLStatusOf(n int, err error) string {
	switch {
	case err != nil:
		return "no GPU grinder: " + err.Error()
	case n == 0:
		return "no GPU grinder: an OpenCL driver is installed but lists no GPU"
	case n == 1:
		return "OpenCL: 1 device"
	}
	return fmt.Sprintf("OpenCL: %d devices", n)
}

// openCLOff reports whether a $POON_OPENCL value turns the GPU grinder
// off: off, 0, false or no, in any case (GREEN-066).
func openCLOff(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "0", "false", "no":
		return true
	}
	return false
}

// openCLOn reports whether a $POON_OPENCL_ALL value turns it on: 1,
// true, yes or on, in any case (RED-125).
func openCLOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// threadsOf is runtime.NumCPU, a variable for tests.
var threadsOf = runtime.NumCPU
