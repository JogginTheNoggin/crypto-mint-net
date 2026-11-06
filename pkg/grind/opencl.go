//go:build (linux || darwin || windows) && (amd64 || arm64)

package grind

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

// The GPU grinder (D208) searches layers on any GPU with an OpenCL driver:
// NVIDIA, AMD and Intel all ship one. The OpenCL library is opened when
// devices are first listed, with no cgo, so every release build has it;
// a machine without the library or a driver lists its GPUs with a note,
// as before. Entries are still ground on the CPU.
//
// $POON_OPENCL=off turns it off; $POON_OPENCL_ALL=1 also takes OpenCL
// devices that are not GPUs (for tests on a machine with only a CPU
// driver).

//go:embed opencl_kernel.cl
var openclSource string

// OpenCL constants this file uses (Khronos cl.h).
const (
	clSuccess              = 0
	clDeviceTypeGPU        = 1 << 2
	clDeviceTypeAll        = 0xFFFFFFFF
	clDeviceVendorID       = 0x1001
	clDeviceMaxComputeUnit = 0x1002
	clDeviceMaxWorkGroup   = 0x1004
	clDeviceName           = 0x102B
	clDeviceVendor         = 0x102C
	clDeviceType           = 0x1000
	clDevicePCIBusInfoKHR  = 0x410F
	clDevicePCIBusIDNV     = 0x4008
	clDevicePCISlotIDNV    = 0x4009
	clDeviceTopologyAMD    = 0x4037
	clProgramBuildLog      = 0x1183
	clMemReadOnly          = 1 << 2
	clMemWriteOnly         = 1 << 1
)

// cl holds the OpenCL entry points, loaded once.
var cl struct {
	once sync.Once
	err  error

	GetPlatformIDs       func(n uint32, platforms *uintptr, num *uint32) int32
	GetDeviceIDs         func(platform uintptr, typ uint64, n uint32, devices *uintptr, num *uint32) int32
	GetDeviceInfo        func(device uintptr, param uint32, size uintptr, value unsafe.Pointer, ret *uintptr) int32
	CreateContext        func(props *uintptr, n uint32, devices *uintptr, notify uintptr, user uintptr, errcode *int32) uintptr
	CreateCommandQueue   func(ctx uintptr, device uintptr, props uint64, errcode *int32) uintptr
	CreateProgramWithSrc func(ctx uintptr, n uint32, src **byte, lengths *uintptr, errcode *int32) uintptr
	BuildProgram         func(prog uintptr, n uint32, devices *uintptr, options *byte, notify uintptr, user uintptr) int32
	GetProgramBuildInfo  func(prog uintptr, device uintptr, param uint32, size uintptr, value unsafe.Pointer, ret *uintptr) int32
	CreateKernel         func(prog uintptr, name *byte, errcode *int32) uintptr
	CreateBuffer         func(ctx uintptr, flags uint64, size uintptr, host unsafe.Pointer, errcode *int32) uintptr
	SetKernelArg         func(kernel uintptr, index uint32, size uintptr, value unsafe.Pointer) int32
	EnqueueWriteBuffer   func(q uintptr, buf uintptr, blocking uint32, offset uintptr, size uintptr, ptr unsafe.Pointer, nwait uint32, wait *uintptr, event *uintptr) int32
	EnqueueReadBuffer    func(q uintptr, buf uintptr, blocking uint32, offset uintptr, size uintptr, ptr unsafe.Pointer, nwait uint32, wait *uintptr, event *uintptr) int32
	EnqueueNDRangeKernel func(q uintptr, kernel uintptr, dim uint32, offset *uintptr, global *uintptr, local *uintptr, nwait uint32, wait *uintptr, event *uintptr) int32
	Finish               func(q uintptr) int32
	ReleaseMemObject     func(m uintptr) int32
	ReleaseKernel        func(k uintptr) int32
	ReleaseProgram       func(p uintptr) int32
	ReleaseCommandQueue  func(q uintptr) int32
	ReleaseContext       func(c uintptr) int32
}

// loadOpenCL opens the OpenCL library and finds its entry points.
func loadOpenCL() error {
	cl.once.Do(func() {
		if v := os.Getenv("POON_OPENCL"); openCLOff(v) {
			cl.err = errors.New("turned off by POON_OPENCL")
			return
		}
		lib, err := openCLLibrary()
		if err != nil {
			cl.err = err
			return
		}
		defer func() {
			if r := recover(); r != nil { // a missing symbol
				cl.err = fmt.Errorf("the OpenCL library lacks a function: %v", r)
			}
		}()
		reg := func(fn any, name string) { purego.RegisterLibFunc(fn, lib, name) }
		reg(&cl.GetPlatformIDs, "clGetPlatformIDs")
		reg(&cl.GetDeviceIDs, "clGetDeviceIDs")
		reg(&cl.GetDeviceInfo, "clGetDeviceInfo")
		reg(&cl.CreateContext, "clCreateContext")
		reg(&cl.CreateCommandQueue, "clCreateCommandQueue")
		reg(&cl.CreateProgramWithSrc, "clCreateProgramWithSource")
		reg(&cl.BuildProgram, "clBuildProgram")
		reg(&cl.GetProgramBuildInfo, "clGetProgramBuildInfo")
		reg(&cl.CreateKernel, "clCreateKernel")
		reg(&cl.CreateBuffer, "clCreateBuffer")
		reg(&cl.SetKernelArg, "clSetKernelArg")
		reg(&cl.EnqueueWriteBuffer, "clEnqueueWriteBuffer")
		reg(&cl.EnqueueReadBuffer, "clEnqueueReadBuffer")
		reg(&cl.EnqueueNDRangeKernel, "clEnqueueNDRangeKernel")
		reg(&cl.Finish, "clFinish")
		reg(&cl.ReleaseMemObject, "clReleaseMemObject")
		reg(&cl.ReleaseKernel, "clReleaseKernel")
		reg(&cl.ReleaseProgram, "clReleaseProgram")
		reg(&cl.ReleaseCommandQueue, "clReleaseCommandQueue")
		reg(&cl.ReleaseContext, "clReleaseContext")
	})
	return cl.err
}

// clDevice is one OpenCL device found.
type clDevice struct {
	id       uintptr
	name     string
	vendorID uint32
	units    uint32
	maxGroup uintptr
	// pci is the device's PCI address, "bus:device.function" in hex, when
	// the driver says it; "" otherwise.
	pci string
	gpu bool
}

var clList struct {
	once sync.Once
	devs []clDevice
	err  error
}

// openCLDevices lists the OpenCL devices once: GPUs, or every device with
// $POON_OPENCL_ALL=1.
func openCLDevices() ([]clDevice, error) {
	clList.once.Do(func() {
		if err := loadOpenCL(); err != nil {
			clList.err = err
			return
		}
		typ := uint64(clDeviceTypeGPU)
		if openCLOn(os.Getenv("POON_OPENCL_ALL")) {
			typ = clDeviceTypeAll
		}
		var np uint32
		if r := cl.GetPlatformIDs(0, nil, &np); r != clSuccess || np == 0 {
			clList.err = fmt.Errorf("no OpenCL platform (error %d): install the GPU vendor's OpenCL driver", r)
			return
		}
		plats := make([]uintptr, np)
		cl.GetPlatformIDs(np, &plats[0], nil)
		for _, p := range plats {
			var nd uint32
			if cl.GetDeviceIDs(p, typ, 0, nil, &nd) != clSuccess || nd == 0 {
				continue
			}
			ids := make([]uintptr, nd)
			if cl.GetDeviceIDs(p, typ, nd, &ids[0], nil) != clSuccess {
				continue
			}
			for _, id := range ids {
				clList.devs = append(clList.devs, describeCL(id))
			}
		}
		if len(clList.devs) == 0 {
			clList.err = errors.New("the OpenCL driver lists no GPU")
		}
	})
	return clList.devs, clList.err
}

func describeCL(id uintptr) clDevice {
	d := clDevice{id: id}
	var buf [256]byte
	var n uintptr
	if cl.GetDeviceInfo(id, clDeviceName, uintptr(len(buf)), unsafe.Pointer(&buf[0]), &n) == clSuccess && n > 0 {
		d.name = strings.TrimRight(string(buf[:n]), "\x00 ")
	}
	cl.GetDeviceInfo(id, clDeviceVendorID, 4, unsafe.Pointer(&d.vendorID), nil)
	cl.GetDeviceInfo(id, clDeviceMaxComputeUnit, 4, unsafe.Pointer(&d.units), nil)
	cl.GetDeviceInfo(id, clDeviceMaxWorkGroup, unsafe.Sizeof(d.maxGroup), unsafe.Pointer(&d.maxGroup), nil)
	var typ uint64
	cl.GetDeviceInfo(id, clDeviceType, 8, unsafe.Pointer(&typ), nil)
	d.gpu = typ&clDeviceTypeGPU != 0
	// The PCI address, from the Khronos extension, NVIDIA's or AMD's.
	var khr [4]uint32 // domain, bus, device, function
	var nvBus, nvSlot uint32
	var amd [24]byte
	switch {
	case cl.GetDeviceInfo(id, clDevicePCIBusInfoKHR, 16, unsafe.Pointer(&khr[0]), nil) == clSuccess:
		d.pci = fmt.Sprintf("%02x:%02x.%x", khr[1], khr[2], khr[3])
	case cl.GetDeviceInfo(id, clDevicePCIBusIDNV, 4, unsafe.Pointer(&nvBus), nil) == clSuccess &&
		cl.GetDeviceInfo(id, clDevicePCISlotIDNV, 4, unsafe.Pointer(&nvSlot), nil) == clSuccess:
		// NVIDIA's slot id is device<<3 | function.
		d.pci = fmt.Sprintf("%02x:%02x.%x", nvBus, nvSlot>>3, nvSlot&7)
	case cl.GetDeviceInfo(id, clDeviceTopologyAMD, uintptr(len(amd)), unsafe.Pointer(&amd[0]), nil) == clSuccess && amd[0] == 1:
		// cl_device_topology_amd: type, then bus, device, function bytes.
		d.pci = fmt.Sprintf("%02x:%02x.%x", amd[21], amd[22], amd[23])
	}
	return d
}

// clKernel is one hash's compiled search on one device. Calls are made
// one at a time.
type clKernel struct {
	mu      sync.Mutex
	ctx     uintptr
	queue   uintptr
	prog    uintptr
	kernel  uintptr
	entry   uintptr // the entry's 32 bytes
	out     uintptr // a key per work group
	outLen  int
	local   uintptr
	per     uint32
	id      hash.ID
	chunk   uint64
	keys    []uint64
	devName string
	// failed is the error the driver gave, and failedAt when; the CPU
	// searches since, until the GPU is tried again after retryGPU
	// (RED-124).
	failed   error
	failedAt time.Time
}

// retryGPU is how long a kernel whose driver failed searches on the CPU
// before the GPU is tried again: a driver reset or a resume may bring it
// back (RED-124).
var retryGPU = time.Minute

// cpuStep is how many nonces the CPU fallback searches between looks at
// the context, so a stop waits a fraction of a second, not a GPU chunk
// (RED-124).
const cpuStep = 1 << 16

// failLog logs a driver error once, when the kernel moves to the CPU.
var failLog = func(msg string, args ...any) { slog.Warn(msg, args...) }

var (
	clKernelsMu sync.Mutex
	clKernels   = map[string]*clKernel{}
)

// kernelNames maps the hashes the GPU grinds to their kernels.
var kernelNames = map[hash.ID]string{hash.SHA256: "sha256_layers", hash.SHA512_256: "sha512_256_layers"}

// openCLKernel returns the compiled kernel for hash id on device d,
// building it on first use.
func openCLKernel(d clDevice, id hash.ID) (*clKernel, error) {
	key := fmt.Sprintf("%x/%d", d.id, id)
	clKernelsMu.Lock()
	defer clKernelsMu.Unlock()
	if k := clKernels[key]; k != nil {
		return k, nil
	}
	name, ok := kernelNames[id]
	if !ok {
		return nil, fmt.Errorf("no GPU kernel for %s", id.Name())
	}
	k := &clKernel{id: id, devName: d.name}
	var e int32
	dev := d.id
	if k.ctx = cl.CreateContext(nil, 1, &dev, 0, 0, &e); e != clSuccess {
		return nil, fmt.Errorf("OpenCL context on %s: error %d", d.name, e)
	}
	fail := func(what string, code int32) (*clKernel, error) {
		k.release()
		return nil, fmt.Errorf("OpenCL %s on %s: error %d", what, d.name, code)
	}
	if k.queue = cl.CreateCommandQueue(k.ctx, dev, 0, &e); e != clSuccess {
		return fail("queue", e)
	}
	src := append([]byte(openclSource), 0)
	srcPtr := &src[0]
	n := uintptr(len(openclSource))
	if k.prog = cl.CreateProgramWithSrc(k.ctx, 1, &srcPtr, &n, &e); e != clSuccess {
		return fail("program", e)
	}
	opts := []byte("\x00")
	if r := cl.BuildProgram(k.prog, 1, &dev, &opts[0], 0, 0); r != clSuccess {
		var log [8192]byte
		var ln uintptr
		cl.GetProgramBuildInfo(k.prog, dev, clProgramBuildLog, uintptr(len(log)), unsafe.Pointer(&log[0]), &ln)
		k.release()
		return nil, fmt.Errorf("OpenCL build on %s: error %d: %s", d.name, r, strings.TrimSpace(string(log[:min(ln, uintptr(len(log)))])))
	}
	cname := append([]byte(name), 0)
	if k.kernel = cl.CreateKernel(k.prog, &cname[0], &e); e != clSuccess {
		return fail("kernel", e)
	}
	if k.entry = cl.CreateBuffer(k.ctx, clMemReadOnly, 32, nil, &e); e != clSuccess {
		return fail("buffer", e)
	}
	k.local = 64
	if d.maxGroup > 0 && d.maxGroup < k.local {
		k.local = 1
		for k.local*2 <= d.maxGroup {
			k.local *= 2
		}
	}
	k.per = 64
	runtime.KeepAlive(src)
	k.calibrate()
	clKernels[key] = k
	return k, nil
}

func (k *clKernel) release() {
	for _, m := range []uintptr{k.entry, k.out} {
		if m != 0 {
			cl.ReleaseMemObject(m)
		}
	}
	if k.kernel != 0 {
		cl.ReleaseKernel(k.kernel)
	}
	if k.prog != 0 {
		cl.ReleaseProgram(k.prog)
	}
	if k.queue != 0 {
		cl.ReleaseCommandQueue(k.queue)
	}
	if k.ctx != 0 {
		cl.ReleaseContext(k.ctx)
	}
}

// Chunk bounds: a chunk takes about chunkTarget on the device, so a
// search looks at its context that often.
const (
	minChunk    = 1 << 16
	maxChunk    = 1 << 30
	chunkTarget = 50 * time.Millisecond
)

// calibrate times a search of 2^20 nonces and sets the chunk to about
// chunkTarget's worth. Called with the kernel new, before it is shared.
func (k *clKernel) calibrate() {
	k.chunk = 1 << 20
	if _, _, err := k.search(hash.Hash{}, 0, 1<<16); err != nil { // the first call compiles lazily on some drivers
		k.chunk = minChunk
		return
	}
	t0 := time.Now()
	if _, _, err := k.search(hash.Hash{}, 0, 1<<20); err != nil {
		k.chunk = minChunk
		return
	}
	el := time.Since(t0)
	c := uint64(float64(1<<20) * float64(chunkTarget) / float64(max(el, time.Microsecond)))
	k.chunk = minChunk
	for k.chunk*2 <= c && k.chunk < maxChunk {
		k.chunk *= 2
	}
}

// search tries nonces [from, from+n) of layers on entry and returns the
// strongest, the lowest nonce among equals, as layer.Grind does. n is at
// most 2^32.
func (k *clKernel) search(entry hash.Hash, from, n uint64) (uint64, int, error) {
	if n == 0 {
		return from, -1, nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failed != nil {
		if time.Since(k.failedAt) < retryGPU {
			return 0, -1, k.failed
		}
		k.failed = nil // try the GPU again
	}
	items := (n + uint64(k.per) - 1) / uint64(k.per)
	groups := (items + uint64(k.local) - 1) / uint64(k.local)
	global := uintptr(groups) * k.local
	var e int32
	if int(groups) > k.outLen {
		if k.out != 0 {
			cl.ReleaseMemObject(k.out)
		}
		if k.out = cl.CreateBuffer(k.ctx, clMemWriteOnly, uintptr(groups)*8, nil, &e); e != clSuccess {
			k.out, k.outLen = 0, 0
			return 0, -1, fmt.Errorf("OpenCL buffer on %s: error %d", k.devName, e)
		}
		k.outLen = int(groups)
		k.keys = make([]uint64, groups)
	}
	// The entry as the kernel reads it: big-endian words, 32-bit for
	// SHA-256 and 64-bit for SHA-512/256, in the device's (little-endian)
	// order.
	var words [32]byte
	if k.id == hash.SHA256 {
		for i := 0; i < 8; i++ {
			binary.LittleEndian.PutUint32(words[4*i:], binary.BigEndian.Uint32(entry[4*i:]))
		}
	} else {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint64(words[8*i:], binary.BigEndian.Uint64(entry[8*i:]))
		}
	}
	if r := cl.EnqueueWriteBuffer(k.queue, k.entry, 1, 0, 32, unsafe.Pointer(&words[0]), 0, nil, nil); r != clSuccess {
		return 0, -1, fmt.Errorf("OpenCL write on %s: error %d", k.devName, r)
	}
	n32 := uint32(n) // n is at most maxChunk
	args := []struct {
		size uintptr
		ptr  unsafe.Pointer
	}{
		{unsafe.Sizeof(k.entry), unsafe.Pointer(&k.entry)},
		{8, unsafe.Pointer(&from)},
		{4, unsafe.Pointer(&n32)},
		{4, unsafe.Pointer(&k.per)},
		{unsafe.Sizeof(k.out), unsafe.Pointer(&k.out)},
		{k.local * 8, nil},
	}
	for i, a := range args {
		if r := cl.SetKernelArg(k.kernel, uint32(i), a.size, a.ptr); r != clSuccess {
			return 0, -1, fmt.Errorf("OpenCL argument %d on %s: error %d", i, k.devName, r)
		}
	}
	local := k.local
	if r := cl.EnqueueNDRangeKernel(k.queue, k.kernel, 1, nil, &global, &local, 0, nil, nil); r != clSuccess {
		return 0, -1, fmt.Errorf("OpenCL run on %s: error %d", k.devName, r)
	}
	if r := cl.EnqueueReadBuffer(k.queue, k.out, 1, 0, uintptr(groups)*8, unsafe.Pointer(&k.keys[0]), 0, nil, nil); r != clSuccess {
		return 0, -1, fmt.Errorf("OpenCL read on %s: error %d", k.devName, r)
	}
	runtime.KeepAlive(words)
	var best uint64
	for _, key := range k.keys[:groups] {
		best = max(best, key)
	}
	if best == 0 {
		return from, -1, nil
	}
	return from + uint64(0xffffffff-uint32(best)), int(best>>32) - 1, nil
}

// layerSearch is k's layer search. Ranges over maxChunk are searched a
// maxChunk at a time. If the driver fails, the range is searched on the
// CPU instead, with the same result: on every core, with the CPU's own
// fast search where there is one, looking at ctx every cpuStep nonces so
// a stop does not wait for the GPU's chunk. Later ranges stay on the CPU
// until retryGPU has passed; the error is logged once and shown in
// OpenCLStatus (RED-124).
func (k *clKernel) layerSearch() layer.SearchContext {
	cpu := cpuLayerSearch(k.id)
	return func(ctx context.Context, entry hash.Hash, from, n uint64) (uint64, int) {
		bestNonce, best := from, -1
		for done := uint64(0); done < n && ctx.Err() == nil; {
			m := min(n-done, maxChunk)
			nonce, s, err := k.search(entry, from+done, m)
			if err != nil {
				k.fail(err)
				nonce, s = parallelSearch(ctx, cpu, entry, from+done, m, threadsOf())
			}
			if s > best {
				bestNonce, best = nonce, s
			}
			done += m
		}
		return bestNonce, best
	}
}

// fail records a driver error, logs the first of a run, and moves k to
// the CPU.
func (k *clKernel) fail(err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failed == nil {
		failLog("GPU grinder failed; searching layers on the CPU", "device", k.devName, "hash", k.id.Name(), "err", err, "retry", retryGPU)
		k.failed, k.failedAt = err, time.Now()
	}
}

// failure says why k searches on the CPU, or "".
func (k *clKernel) failure() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failed == nil || time.Since(k.failedAt) >= retryGPU {
		return ""
	}
	return fmt.Sprintf("%s (%s): grinding on the CPU since %s: %v", k.devName, k.id.Name(), k.failedAt.Format(time.TimeOnly), k.failed)
}

// cpuLayerSearch is the CPU's layer search for id: its fast search (SIMD
// lanes) where one is registered, else one nonce at a time.
func cpuLayerSearch(id hash.ID) layer.Search {
	Default.mu.RLock()
	g, ok := Default.grinders[CPU][id]
	Default.mu.RUnlock()
	if ok && g.Search != nil {
		if s, err := g.Search(Device{Kind: CPU}, id); err == nil {
			return s
		}
	}
	nh := hash.GrinderFor(id)
	return func(entry hash.Hash, from, n uint64) (uint64, int) { return cpuSearch(nh, entry, from, n) }
}

// parallelSearch searches [from, from+n) with search on workers
// goroutines, each looking at ctx every cpuStep nonces. With every nonce
// searched the result is search's over the whole range: the strongest,
// the lowest nonce among equals.
func parallelSearch(ctx context.Context, search layer.Search, entry hash.Hash, from, n uint64, workers int) (uint64, int) {
	workers = int(min(uint64(max(workers, 1)), max(n/cpuStep, 1)))
	type res struct {
		nonce uint64
		s     int
	}
	out := make([]res, workers)
	per := n / uint64(workers)
	var wg sync.WaitGroup
	for w := range workers {
		lo, m := from+uint64(w)*per, per
		if w == workers-1 {
			m = n - uint64(w)*per
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := res{lo, -1}
			for done := uint64(0); done < m && ctx.Err() == nil; done += cpuStep {
				nonce, s := search(entry, lo+done, min(cpuStep, m-done))
				if s > r.s {
					r = res{nonce, s}
				}
			}
			out[w] = r
		}()
	}
	wg.Wait()
	best := res{from, -1}
	for _, r := range out { // in nonce order: the first of equals is the lowest
		if r.s > best.s {
			best = r
		}
	}
	return best.nonce, best.s
}

// cpuSearch is layer.Grind's search on one goroutine.
func cpuSearch(nh hash.NewHasher, entry hash.Hash, from, n uint64) (uint64, int) {
	g, err := nh(entry[:])
	if err != nil {
		panic(err) // the grinders of package hash take any prefix
	}
	var buf [8]byte
	bestNonce, best := from, -1
	for i := uint64(0); i < n; i++ {
		nonce := from + i
		binary.BigEndian.PutUint64(buf[:], nonce)
		if s := g.Hash(buf[:]).Strength(); s > best {
			bestNonce, best = nonce, s
		}
	}
	return bestNonce, best
}

// probeOpenCLGPUs lists the GPUs as probeGPUs does and marks each one an
// OpenCL device matches, by PCI address, else by vendor in order. OpenCL
// GPUs the system does not list (other systems than Linux) are added.
func probeOpenCLGPUs() []Device {
	devs, err := openCLDevices()
	return matchOpenCL(sysGPUs(), devs, err)
}

// matchOpenCL is probeOpenCLGPUs on the system's GPUs and the OpenCL
// devices found (or why none were).
func matchOpenCL(sys []sysGPU, devs []clDevice, err error) []Device {
	sys = slices.Clone(sys)
	if err != nil {
		for i := range sys {
			sys[i].dev.Note = "found; no GPU grinder runs on it: " + err.Error()
		}
		return sysDevices(sys)
	}
	used := make([]bool, len(devs))
	match := func(ok func(clDevice) bool) int {
		for j, c := range devs {
			if !used[j] && ok(c) {
				used[j] = true
				return j
			}
		}
		return -1
	}
	for i := range sys {
		s := &sys[i]
		j := -1
		if s.pci != "" {
			j = match(func(c clDevice) bool { return c.pci != "" && c.pci == s.pci })
		}
		if j < 0 {
			j = match(func(c clDevice) bool { return c.pci == "" && fmt.Sprintf("0x%04x", c.vendorID) == s.vendor })
		}
		if j < 0 {
			s.dev.Note = "found, but no OpenCL driver lists it, so no grinder runs on it; install its vendor's OpenCL driver"
			continue
		}
		s.dev.cl = j + 1
		s.dev.Name = devs[j].name
	}
	out := sysDevices(sys)
	for j, c := range devs {
		if !used[j] {
			out = append(out, Device{ID: fmt.Sprintf("gpu:%d", len(out)), Kind: GPU, Name: c.name, cl: j + 1})
		}
	}
	return out
}

func init() {
	gpuProbe = probeOpenCLGPUs
	openCLStatus = func() string {
		devs, err := openCLDevices()
		st := openCLStatusOf(len(devs), err)
		clKernelsMu.Lock()
		var failed []string
		for _, k := range clKernels {
			if f := k.failure(); f != "" {
				failed = append(failed, f)
			}
		}
		clKernelsMu.Unlock()
		slices.Sort(failed)
		for _, f := range failed {
			st += "; " + f
		}
		return st
	}
	for id := range kernelNames {
		Default.Register(Grinder{
			Name: "gpu-opencl", Kind: GPU, Hash: id,
			Has: func(d Device) bool { return d.cl > 0 },
			// Entries are ground on the CPU (D208).
			New: func(_ Device, id hash.ID) (hash.NewHasher, error) { return hash.GrinderFor(id), nil },
			SearchContext: func(d Device, id hash.ID) (layer.SearchContext, error) {
				k, err := deviceKernel(d, id)
				if err != nil {
					return nil, err
				}
				return k.layerSearch(), nil
			},
			Chunk: func(d Device, id hash.ID) uint64 {
				if k, err := deviceKernel(d, id); err == nil {
					return k.chunk
				}
				return 0
			},
		})
	}
}

func deviceKernel(d Device, id hash.ID) (*clKernel, error) {
	devs, err := openCLDevices()
	if err != nil {
		return nil, err
	}
	if d.cl < 1 || d.cl > len(devs) {
		return nil, fmt.Errorf("%s has no OpenCL device", d.ID)
	}
	return openCLKernel(devs[d.cl-1], id)
}
