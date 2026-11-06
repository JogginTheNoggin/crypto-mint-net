//go:build (linux || darwin || windows) && (amd64 || arm64)

package grind

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

// BLUE-042 (RED-124): after a driver error the range is searched on every
// core with the same result, a stop does not wait for the GPU's chunk,
// the error is logged once and said, and the GPU is tried again later.
func TestBlue042DriverFailureFallsBack(t *testing.T) {
	var logged []string
	defer func(f func(string, ...any)) { failLog = f }(failLog)
	failLog = func(msg string, args ...any) { logged = append(logged, msg) }
	for id := range kernelNames {
		k := &clKernel{id: id, devName: "test gpu"}
		k.fail(errors.New("driver reset"))
		k.fail(errors.New("again"))
		search := k.layerSearch()
		entry := hash.Hash{7, byte(id)}
		const from, n = 1000, 300_000
		gn, gs := search(context.Background(), entry, from, n)
		l, s, _ := layer.GrindHash(id, entry, from, n, 1)
		if gn != l.Nonce || gs != s {
			t.Fatalf("%s: fallback %d/%d, CPU %d/%d", id.Name(), gn, gs, l.Nonce, s)
		}
		// A GPU-sized chunk, stopped after 50 ms, returns at once.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		t0 := time.Now()
		search(ctx, entry, 0, 1<<30)
		cancel()
		if el := time.Since(t0); el > 2*time.Second {
			t.Fatalf("%s: a stop waited %v for the chunk", id.Name(), el)
		}
		if f := k.failure(); !strings.Contains(f, "grinding on the CPU") || !strings.Contains(f, "driver reset") {
			t.Fatalf("failure %q", f)
		}
	}
	if len(logged) != len(kernelNames) {
		t.Fatalf("logged %d times for %d kernels: %v", len(logged), len(kernelNames), logged)
	}
	// After retryGPU the GPU is tried again: the failure is no longer said.
	defer func(d time.Duration) { retryGPU = d }(retryGPU)
	retryGPU = time.Millisecond
	k := &clKernel{id: hash.SHA256, devName: "test gpu"}
	k.fail(errors.New("driver reset"))
	time.Sleep(5 * time.Millisecond)
	if f := k.failure(); f != "" {
		t.Fatalf("still failed after the retry time: %q", f)
	}
}

// parallelSearch over any split gives the one-goroutine result.
func TestBlue042ParallelSearch(t *testing.T) {
	nh := hash.GrinderFor(hash.SHA256)
	one := func(e hash.Hash, from, n uint64) (uint64, int) { return cpuSearch(nh, e, from, n) }
	for _, c := range []struct {
		from, n uint64
		w       int
	}{{0, 1, 4}, {5, 70_000, 3}, {1 << 40, 400_001, 7}, {9, cpuStep * 3, 16}} {
		e := hash.Hash{byte(c.n)}
		wn, ws := one(e, c.from, c.n)
		if gn, gs := parallelSearch(context.Background(), one, e, c.from, c.n, c.w); gn != wn || gs != ws {
			t.Fatalf("%+v: parallel %d/%d, one %d/%d", c, gn, gs, wn, ws)
		}
	}
}

// RED-125: POON_OPENCL_ALL is on for 1, true, yes or on, in any case.
func TestBlue042OpenCLAllValues(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "TRUE": true, " yes ": true, "On": true, "": false, "0": false, "off": false, "maybe": false} {
		if openCLOn(v) != want {
			t.Errorf("POON_OPENCL_ALL=%q: %v", v, !want)
		}
	}
}
