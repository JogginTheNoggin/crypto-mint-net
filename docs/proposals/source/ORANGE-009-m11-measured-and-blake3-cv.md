# ORANGE-009: M11 measured, and a BLAKE3 grinder that keeps chaining values

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `29b08ce` (code `a92c2ca`) |
| Area | `pkg/grind` (`blake3.go`, the `cpu-blake3-cv` grinder) |
| Effort | small |
| Decided by | Grey, Source Round 6: `cpu-blake3-cv` is BLAKE3's CPU grinder (D112) |

## Summary

M11 delivers what ORANGE-006 and 007 measured:
- On one thread every hash grinds 15 to 32% faster than Round 4's
  baseline.
- On the default three threads (cores minus one, on this 4-core machine)
  it grinds 3.5 to 4.4 times faster than the baseline.
- Auto picks the CPU's fast grinder for every hash, since this machine
  has no GPU.
- A grind job runs at the raw benchmark's rate to within 3%, with or
  without fsync, so jobs add no measurable overhead.

One hash lags. BLAKE3 entry tries still cost 700 to 850 ns, because
restoring the general hasher's saved state copies its 8 KB buffer.

A new BLAKE3 grinder keeps only the 32-byte chaining values. It cuts an
entry try to 152 ns, 5.3 times faster for a 17-transaction entry. Hashes
are unchanged.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (SHA-NI, AVX2, AVX-512F), Linux,
Go 1.27.3. No GPU, so every device below is the CPU, as `Detect` reports
it.

**Per hash and threads.** These come from `grind.RunBench` for 3 s per
plan (a test-only harness: `Default.Choose(devs, id, "auto", threads)`
then `RunBench`). Values are ns per try across all threads. The Round 4
baseline is `layer.Grind` on one core (ORANGE-006).

| Hash | R4 baseline | M11, 1 thread | M11, default (3) | M11, 4 threads |
| --- | --- | --- | --- | --- |
| SHA-256 | 161 | 110 | 36.5 | 28.0 |
| SHA-512/256 | 372 | 309 | 106 | 80.2 |
| SHA3-256 | 547 | 453 | 157 | 117 |
| BLAKE2b-256 | 270 | 213 | 73.1 | 58.3 |
| BLAKE3 | 181 | 125 | 42.3 | 38.4 |

The default leaves one core to the node. That costs 23 to 26% of the
peak rate, except for BLAKE3 at 9%.

**Job overhead.** A tip job of 10 s on a real node runs default threads,
auto device, and its 2 s slices. Each slice ends with a stored layer and
a status check. Tries per second, ns per try:

| Hash | Bench, 3 threads | Job, `no_sync` | Job, fsync |
| --- | --- | --- | --- |
| SHA-256 | 36.5 | 36.2 | 37.0 |
| SHA-512/256 | 106 | 103 | 108 |
| SHA3-256 | 157 | 156 | 154 |
| BLAKE2b-256 | 73.1 | 73.0 | 72.1 |
| BLAKE3 | 42.3 | 41.8 | 41.7 |

Every job stored 5 layers, one per slice. ORANGE-010 is about that.

**BLAKE3 entries.** `go test -bench BLAKE3Grinder -count 3 ./pkg/grind`,
ns per try:

| Prefix | Example | General hasher, saved state | Chaining-value grinder |
| --- | --- | --- | --- |
| 32 bytes | a layer | 113 | 153 |
| 544 bytes | an entry of 16 txs | 820 | 156 |
| 8,224 bytes | an entry of 256 txs | 679 | 309 |

The pure-Go compression is slower than the general hasher's assembly for
one block. So the grinder keeps the general hasher for prefixes under one
block (layers) and uses chaining values from one block up (entries).

## Problem

`hash.Grinder` restores BLAKE3 by copying the `zeebo/blake3` hasher,
which holds a whole chunk in an 8 KB buffer. It never compresses a
chunk's blocks before the chunk ends, so for any entry the copy and then
the compression of every buffered block happen on every try.

## Idea

`grind/blake3.go` implements BLAKE3's compression and tree from the
specification (default 32-byte, unkeyed). A grinder for a prefix:
- compresses every whole chunk before the last one, merging parents as
  BLAKE3 does, and keeps the stack of chaining values;
- compresses the last chunk's whole blocks that cannot be its last block;
- keeps the bytes after them.

Each try compresses the tail and the suffix (one or two blocks, or the
rare chunk boundary), then the parents up to the root.

The `cpu-blake3-cv` grinder registers it for BLAKE3. Below one block it
hands over to `hash.NewGrinder`. `docs/modules/grind.md` lists the
grinders and would need the new row.

## Cost and risk

- About 250 lines.
- The risk is a mistake in a hand-written hash. `TestBLAKE3GrinderMatches`
  compares it with package hash's BLAKE3 for every prefix length from 0
  to 4,226 bytes, which crosses every block and chunk boundary up to four
  chunks, and suffixes of 0, 1, 8, 63, 64 and 65 bytes. It also runs
  1,000 nonces on one grinder, as a search does. The grind package's
  `TestEveryGrinderAgrees` passes with the new grinder registered.
- A wrong hash would only waste work: a peer recomputes every hash with
  package hash, so nothing invalid is ever accepted.

## Prototype

On `claude/source-orange-r5`, commit `3b96a32`: `pkg/grind/blake3.go`,
`blake3_test.go` (with `BenchmarkBLAKE3Grinder`), and the registration in
`grind.go`. The race suite passes.
