# ORANGE-003: Reuse hash digests

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `7e68fe0` (base `23e268f`) |
| Area | `pkg/hash` (`Lookup`) |
| Effort | small |
| Decided by | Grey, Source Round 4 |

## Summary

Every hash the node computes made a new digest object, and a builder
computes one per nonce it tries. Reusing digests through a pool makes a
builder's try 27% faster with SHA-256, 28% with BLAKE2b, and 16 times
faster with BLAKE3, whose digest costs 11 KB to make. It also removes one
of the larger sources of garbage in the simulator. Hash values are
unchanged.

## Problem

The function `hash.Lookup` returns calls `e.new()` on every call:
`sha256.New`, `blake3.New` and so on. In the simulator that was 10% of all
bytes allocated. For BLAKE3 the digest is 11 KB, so a series that picks
BLAKE3 (decision D80) grinds at an eighth of the speed of SHA-256 for no
reason in the hash itself.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (cloud container), Linux, Go 1.27.3.

`go test -run '^$' -bench . -benchmem -count 3 ./pkg/layer`, means.
`BenchmarkGrind` is new: `layer.Grind`, the builder's loop, per try.
SHA-512/256 and SHA3-256 moved within run-to-run noise (a 6-run rerun of
SHA3-256 gave 701 ns before and 634 ns after).

| Benchmark | Before | After |
| --- | --- | --- |
| `Grind/sha256` | 231 ns, 208 B, 3 allocs | 168 ns, 80 B, 2 allocs |
| `Grind/sha512-256` | 505 ns, 304 B | 458 ns, 80 B |
| `Grind/sha3-256` | 685 ns, 320 B | 781 ns, 80 B (noise; see above) |
| `Grind/blake2b-256` | 438 ns, 464 B | 316 ns, 80 B |
| `Grind/blake3` | 3,243 ns, 10,960 B | 198 ns, 80 B |
| `Hash` (a layer hash) | 244 ns, 264 B, 5 allocs | 201 ns, 136 B, 4 allocs |

## Idea

`Lookup` keeps a `sync.Pool` of digests for the function it returns. Each
call takes a digest, `Reset`s it, writes the parts, sums, and puts it
back. `Reset` returns every digest to its initial state, so the output is
the same as with a new one.

## Cost and risk

Ten lines. `sync.Pool` is safe for concurrent use. The remaining two
allocations per try are the variadic slice and the nonce encoding; a
`Grind` that writes to one digest directly could remove them, but that
would touch every hash function's API and is left for later.

## Prototype

On `claude/source-orange`, commit `869321a`: `pkg/hash/hash.go` and
`BenchmarkGrind` in `pkg/layer/bench_test.go`. The hash package's known-
answer tests pass unchanged.
