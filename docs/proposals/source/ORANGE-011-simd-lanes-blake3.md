# ORANGE-011: SIMD lanes, sixteen BLAKE3 layer tries at once

| Field | Value |
| --- | --- |
| Status | kept as a prototype |
| Based on | `claude/m10-watch` @ `29b08ce` (code `a92c2ca`) |
| Area | `pkg/grind` (`blake3_lanes.go`, build tag `goexperiment.simd && amd64`) |
| Effort | medium |
| Decided by | Grey, Source Round 6: kept behind `GOEXPERIMENT=simd`, not wired to a grinder until Go's `simd` package is stable (D112) |

## Summary

A layer try is one hash of one block, and tries differ only in the
nonce. So vector hardware can run many tries at once, one per lane. With
Go's experimental `simd/archsimd` package (`GOEXPERIMENT=simd`), sixteen
BLAKE3 layer tries share one AVX-512 vector:
- 24 ns a try on one core, against 113 ns today: 4.7 times faster.
- On 4 cores, 166 M tries a second against 26 M: 6.4 times.

It is a prototype outside the default build. The package is experimental
in Go 1.27, so shipping it needs a decision.

## Measurements

Same machine as ORANGE-009 (AVX-512F). `GOEXPERIMENT=simd go test -bench BLAKE3Lanes -count 3 ./pkg/grind`
for one core. A test-only harness ran 2²⁶ tries per thread for more
threads.

| Threads | BLAKE3 today (M11) | Lanes | Speed-up |
| --- | --- | --- | --- |
| 1 | 125 ns (113 in the bare grinder) | 23.4 ns | 4.8× (vs 113) |
| 3 (default) | 42.3 ns | 8.0 ns | 5.3× |
| 4 | 38.4 ns | 6.0 ns | 6.4× |

Two details mattered:
- `RotateAllRight` is emulated with two shifts and an or. AVX-512's
  per-lane rotate (`RotateRight` with a vector of counts, `VPRORVD`) took
  a try from 114 ns to 36 ns.
- Comparing the first byte of all sixteen hashes with zero in one vector
  operation, before looking at any lane, took it from 36 ns to 23 ns.

## Idea

- `blake3LayerLanes(entry, from, n)` hashes `entry ∥ nonce` for sixteen
  nonces per compression. It returns the strongest nonce, the lowest among
  equals, as `layer.Grind` does. `TestBLAKE3LanesMatchGrind` checks that
  over 70,000 nonces and several entries.
- To use it, M11 needs a grinder that can search a range itself and not
  only hash one suffix. One way is an optional interface beside
  `hash.Hasher`, such as `Search(ctx, from, n) (nonce, strength)`, that
  `layer.GrindWith` uses when the hasher has it. A `cpu-avx512-lanes`
  grinder for BLAKE3 layers would then be one registration, picked by
  auto when `archsimd.X86.AVX512()` holds.
- The same shape fits:
  - BLAKE2b and SHA-256 lanes (8 lanes with AVX2, 16 with AVX-512);
  - entries: the per-try work after the saved chaining values
    (ORANGE-009) is one or two compressions, so it lanes the same way;
  - a GPU grinder later.

## Cost and risk

- `simd/archsimd` exists only with `GOEXPERIMENT=simd` and is not covered
  by the Go 1 promise. The options are:
  - keep it in an opt-in build (a release build with the experiment on);
  - wait until the package is stable;
  - write the same loop in assembly, for example with `avo`.
- The build tag keeps the default build and its tests unchanged.
- About 130 lines. A mistake only wastes work, since every layer is
  checked again with package hash before anyone counts it.

## Prototype

On `claude/source-orange-r5`, commit `8b7e4d0`: `pkg/grind/blake3_lanes.go`
and `blake3_lanes_test.go`, both behind `goexperiment.simd && amd64`. The
test skips without AVX-512.
