# ORANGE-006: Grinding baseline, and an allocation-free grinder per hash

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `0ac8e4e` (base `deecc80`) |
| Area | `pkg/hash` (`Grinder`), `pkg/layer` (`GrindHash`), `pkg/entry` (`BuildWorkers`), `pkg/engine` (`GrindLayer`) |
| Effort | small |
| Decided by | |

## Summary

Grey asked for a baseline of today's CPU grinding per hash type, for
milestone M11 (hardware detection and a modular grinder per hash). It is
below. It finds two costs that are not in the hash itself:
- Every layer try goes through the general hash function, which
  allocates. This adds 12 to 61% over the raw hash.
- Every entry try hashes the whole prefix again: the tag, the parent and
  every tid. So a try on an entry of 256 transactions costs 39 to 67
  times a layer try.

`hash.Grinder` keeps the digest's state after the fixed prefix and hashes
only the nonce, with no allocation. It is the per-hash building block M11
can grow into:
- A SHA-256 layer try drops from 161 to 113 ns.
- A 256-transaction SHA-256 entry try drops from 10.6 µs to 113 ns.

Results are unchanged, and the engine still grinds on one core.
ORANGE-007 adds more cores.

## Problem

- `layer.Grind` calls the series' `hash.Func` for each nonce. The call
  allocates the variadic argument slice and the result, and goes through
  a digest pool.
- `entry.BuildContext` calls `HashOf` for each nonce. It builds a slice of
  every tid, then hashes `tag ∥ parent ∥ tid₁ … tidₙ ∥ nonce` from the
  start. Only the last 8 bytes change between tries.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (cloud container), Linux, Go 1.27.3.
The CPU has SHA-NI, AVX2 and AVX-512F. Go's SHA-256 uses SHA-NI and
BLAKE2b uses AVX2. BLAKE3 (`zeebo/blake3`) uses SIMD only on inputs of 8
chunks or more, so never when grinding. Means of 3 runs, ns per try, one
core.

**Baseline (today).** Layer: `go test -bench Grind -count 3 ./pkg/layer`.
Entry: the new `go test -bench BuildTry -count 3 ./pkg/entry`, with the
minimum entry strength set out of reach. "Floor" is a test-only loop on
one reused digest (Reset, write 40 bytes, Sum into a stack buffer): the
cost of the hash alone.

| Hash | Floor | Layer try | Entry try, 1 tx | 16 txs | 256 txs |
| --- | --- | --- | --- | --- | --- |
| SHA-256 | 100 | 161 | 301 | 944 | 10,742 |
| SHA-512/256 | 324 | 372 | 425 | 1,775 | 20,593 |
| SHA3-256 | 487 | 547 | 691 | 2,728 | 31,570 |
| BLAKE2b-256 | 199 | 270 | 339 | 1,298 | 14,388 |
| BLAKE3 | 115 | 181 | 353 | 1,189 | 7,002 |

Layer tries allocate 80 B in 2 allocations. Entry tries allocate 168 B
to 6.6 KB in 4 allocations.

**With the Grinder (one worker).** `go test -bench 'GrindHash|BuildTry' -count 3 ./pkg/layer ./pkg/entry`.
No allocations per try.

| Hash | Layer try | Entry try, 1 tx | 16 txs | 256 txs |
| --- | --- | --- | --- | --- |
| SHA-256 | 113 | 119 | 111 | 113 |
| SHA-512/256 | 324 | 357 | 340 | 332 |
| SHA3-256 | 461 | 462 | 448 | 465 |
| BLAKE2b-256 | 221 | 218 | 219 | 225 |
| BLAKE3 | 135 | 254 | 831 | 702 |

In hashes per second on one core, a SHA-256 layer grinder goes from
6.2 M to 8.8 M (the floor is 10 M). The BLAKE3 entry case is limited by
copying the hasher, which carries an 8 KB buffer: see Leads.

## Idea

- `hash.NewGrinder(id, prefix...)` returns a `Grinder` whose
  `Hash(suffix)` equals `Func(prefix..., suffix)`.
  - When the prefix fills at least one block, it saves the digest's state
    once (`MarshalBinary` for SHA-2, SHA-3 and BLAKE2b; a struct copy for
    BLAKE3) and restores it for each try.
  - A shorter prefix is simply written again.
  - It allocates nothing per try. It is not safe for concurrent use, so
    each worker gets its own.
- `layer.GrindHash(id, entry, start, tries, workers)` is `layer.Grind` with
  a Grinder. `engine.GrindLayer` uses it with one worker.
- `entry.BuildWorkers(ctx, s, parent, txs, maxTries, workers)` is
  `BuildContext` with a Grinder. `BuildContext` is `BuildWorkers` with one
  worker.

## Cost and risk

About 110 lines in `pkg/hash` and the callers. The result is the same
hash, checked by `TestGrinderMatchesFunc`:
- for every known hash;
- for prefixes of 0 to 9,000 bytes, given in two parts;
- on both sides of a block.

`TestBuildWorkersMatchesSearch` checks that the nonce found is the one a
plain `HashOf` search finds. The `layer.Grind` and `entry.HashOf` used
for checking are unchanged.

## Prototype

On `claude/source-orange-r4`, commit `c4bfa0d` (shared with ORANGE-007):
- `pkg/hash/grinder.go` and `grinder_test.go` (with `BenchmarkGrinder`);
- `GrindHash` in `pkg/layer/layer.go`, `grind_test.go`;
- `BuildWorkers` in `pkg/entry/entry.go`, `build_test.go`, `bench_test.go`;
- `engine.GrindLayer`.

## Leads for M11

- **Hardware detection.** `golang.org/x/sys/cpu` is already a dependency.
  It reports SHA-NI, AVX2, AVX-512 and ARM SHA2/SHA3, which is enough to
  pick a grinder per hash and to report it in `poo doctor`.
- **Multi-buffer SIMD.** A layer try is one block, so the next large step
  is hashing 8 (AVX2) or 16 (AVX-512) nonces at once in vector lanes. For
  BLAKE3 and BLAKE2b this is typically 4 to 8 times one-lane speed. It
  would be a new grinder behind the same `Grinder` shape.
- **BLAKE3 midstate.** `zeebo/blake3` keeps whole chunks in its buffer,
  so its saved state includes up to 8 KB. A grinder that keeps only the
  chaining value of complete 64-byte blocks would make BLAKE3 entry tries
  about as cheap as its layer tries.
