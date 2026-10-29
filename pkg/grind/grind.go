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

// grinder returns the grinder for kind k and hash id: the fast one, else
// the generic one (fallback true).
func (r *Registry) grinder(k Kind, id hash.ID) (g Grinder, fallback, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.grinders[k][id]; ok {
		return g, false, true
	}
	if g, ok := r.grinders[k][0]; ok {
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
			if g, fallback, ok := r.grinder(d.Kind, id); ok && !fallback && g.Hash == id {
				d.Hashes = append(d.Hashes, id.Name())
			}
		}
		d.Generic = r.hasGeneric(d.Kind)
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

func (r *Registry) hasGeneric(k Kind) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.grinders[k][0]
	return ok
}

// autoIndex returns the device auto picks for hash id: a device other
// than the CPU with a fast grinder for it (an accelerator is meant to be
// faster), else the CPU with a fast one, else any device with a generic
// one.
func (r *Registry) autoIndex(devs []Device, id hash.ID) (int, bool) {
	best, bestRank := -1, 0
	for i, d := range devs {
		_, fallback, ok := r.grinder(d.Kind, id)
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
}

// ErrNoDevice is returned for a device that is not there or cannot grind
// the hash.
var ErrNoDevice = errors.New("grind: no such device")

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
	g, fallback, ok := r.grinder(d.Kind, id)
	if !ok {
		return Plan{}, fmt.Errorf("%w: %s has no grinder for %s", ErrNoDevice, d.ID, id.Name())
	}
	nh, err := g.New(d, id)
	if err != nil {
		return Plan{}, err
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
	return Plan{Device: d.ID, Kind: d.Kind, Grinder: g.Name, Hash: id.Name(), Threads: threads, Fallback: fallback, Reason: why, newHasher: nh}, nil
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
	return engine.Grind{New: p.newHasher, Workers: p.Threads, Tried: tried}
}

// newDefault returns a registry with the CPU probe, the GPU probe, a fast
// CPU grinder for every known hash and the generic CPU grinder.
func newDefault() *Registry {
	r := &Registry{}
	r.RegisterProbe(probeCPU)
	r.RegisterProbe(probeGPUs)
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

// threadsOf is runtime.NumCPU, a variable for tests.
var threadsOf = runtime.NumCPU
