//go:build (linux || darwin || windows) && (amd64 || arm64)

package grind

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

// openCLForTest returns the first OpenCL device of any type, or skips: a
// machine with no OpenCL driver (most CI) has nothing to test. A CPU
// driver such as PoCL runs the same kernels.
func openCLForTest(t *testing.T) clDevice {
	t.Helper()
	if os.Getenv("POON_OPENCL_ALL") != "1" {
		t.Skip("set POON_OPENCL_ALL=1 to test the GPU grinder on any OpenCL device")
	}
	devs, err := openCLDevices()
	if err != nil {
		t.Skipf("no OpenCL device: %v", err)
	}
	return devs[0]
}

// D208: the GPU search returns what the one-goroutine CPU search returns
// for the same nonces, the lowest nonce among equals, for every hash it
// grinds, ranges of any length and nonces across a 32-bit word and up to
// the last one.
func TestOpenCLSearchMatchesCPU(t *testing.T) {
	d := openCLForTest(t)
	r := rand.New(rand.NewPCG(1, 2))
	for id := range kernelNames {
		k, err := openCLKernel(d, id)
		if err != nil {
			t.Fatal(err)
		}
		cpu := hash.GrinderFor(id)
		cases := []struct{ from, n uint64 }{
			{0, 1}, {0, 63}, {5, 4097}, {1<<32 - 3000, 7000}, {^uint64(0) - 5000, 5001}, {r.Uint64() >> 1, 200_000},
		}
		for i := 0; i < 6; i++ {
			cases = append(cases, struct{ from, n uint64 }{r.Uint64() >> 2, 1 + r.Uint64N(70_000)})
		}
		for _, c := range cases {
			var entry hash.Hash
			for j := range entry {
				entry[j] = byte(r.Uint32())
			}
			gn, gs, err := k.search(entry, c.from, c.n)
			if err != nil {
				t.Fatal(err)
			}
			cn, cs := cpuSearch(cpu, entry, c.from, c.n)
			if gn != cn || gs != cs {
				t.Fatalf("%s from %d n %d: GPU %d/%d, CPU %d/%d", id.Name(), c.from, c.n, gn, gs, cn, cs)
			}
		}
	}
}

// D208: a plan on the OpenCL device grinds layers through the chunked
// search with the result of the CPU's, and entries keep the CPU's
// threads.
func TestOpenCLPlanGrinds(t *testing.T) {
	openCLForTest(t)
	gpuProbe = probeOpenCLGPUs
	defer func() { gpuProbe = probeGPUs }()
	devs := Default.Detect()
	var gpu *Device
	for i := range devs {
		if devs[i].cl > 0 {
			gpu = &devs[i]
		}
	}
	if gpu == nil {
		t.Fatal("no device has the OpenCL grinder")
	}
	for id := range kernelNames {
		p, err := Default.Choose(devs, id, gpu.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if p.Grinder != "gpu-opencl" || p.chunk < minChunk || p.entryThreads < 1 {
			t.Fatalf("plan %+v chunk %d entry threads %d", p, p.chunk, p.entryThreads)
		}
		entry := hash.Hash{9, byte(id)}
		l, s, err := layer.GrindSearchContext(context.Background(), p.search, entry, 100, 300_000, p.Threads, nil, nil, p.chunk)
		if err != nil {
			t.Fatal(err)
		}
		cl, cs, _ := layer.GrindHash(id, entry, 100, 300_000, 1)
		if l != cl || s != cs {
			t.Fatalf("%s: plan %v/%d, CPU %v/%d", id.Name(), l, s, cl, cs)
		}
	}
}

// D208: OpenCL devices are matched to the system's GPUs by PCI address,
// else by vendor in order; a GPU no driver lists keeps a note, and an
// OpenCL GPU the system does not list is added.
func TestMatchOpenCL(t *testing.T) {
	sys := []sysGPU{
		{dev: Device{ID: "gpu:0", Kind: GPU, Name: "Intel (i915)"}, vendor: "0x8086", pci: "00:02.0"},
		{dev: Device{ID: "gpu:1", Kind: GPU, Name: "NVIDIA (nvidia)"}, vendor: "0x10de", pci: "01:00.0"},
		{dev: Device{ID: "gpu:2", Kind: GPU, Name: "AMD (amdgpu)"}, vendor: "0x1002", pci: "03:00.0"},
	}
	devs := []clDevice{
		{name: "NVIDIA GeForce RTX 2070 SUPER", vendorID: 0x10de, pci: "01:00.0"},
		{name: "AMD Radeon", vendorID: 0x1002},
		{name: "Other", vendorID: 0x1234},
	}
	got := matchOpenCL(sys, devs, nil)
	if len(got) != 4 || got[0].cl != 0 || got[0].Note == "" || got[1].cl != 1 || got[1].Name != devs[0].name || got[2].cl != 2 || got[3].cl != 3 || got[3].ID != "gpu:3" {
		t.Fatalf("matched %+v", got)
	}
	none := matchOpenCL(sys, nil, errors.New("no OpenCL library"))
	if len(none) != 3 || !strings.Contains(none[1].Note, "no OpenCL library") || none[1].cl != 0 {
		t.Fatalf("no driver: %+v", none)
	}
}
