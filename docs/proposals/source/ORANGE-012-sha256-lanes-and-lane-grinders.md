# ORANGE-012: SHA-256 lanes, and lane searches wired as grinders

| Field | Value |
| --- | --- |
| Status | adopted (Source Round 7, D119) |
| Based on | `claude/m10-watch` @ `9972137` |
| Area | `pkg/layer` (`Search`, `GrindSearch`), `pkg/engine` (`Grind.Search`), `pkg/grind` (`Grinder.Search`, `lanes.go`, `sha256_lanes.go`, `sha256_lanes8.go`) |
| Effort | medium |
| Decided by | Grey |

## Summary

SHA-256 is the hash most series use, and its layer try was the slowest
part left in M11: 108 ns per try on one core with SHA-NI. This proposal
does two things:

1. **SHA-256 lanes.** Like ORANGE-011 for BLAKE3, sixteen SHA-256 layer
   tries share one AVX-512 vector. A try drops to about 30 ns on one
   core, and from 37 to about 10 ns on the default 3 threads. That is
   **3.6 times** the layer rate, on the hash that matters most.
2. **Lane searches wired as grinders.** `layer.Search` lets a grinder
   search a range of nonces itself. With `GOEXPERIMENT=simd` on an
   AVX-512 CPU, two grinders register:
   - `cpu-sha256-avx512`;
   - `cpu-blake3-avx512` (ORANGE-011's lanes).

   Jobs, `poo work` and `poo grind bench` then use them through the
   normal plan. A default build is unchanged: the lanes are only in an
   opt-in build. That keeps D112 (no experimental package in the default
   build) and makes the opt-in build useful now.

No consensus change: the lanes find the same layer as `layer.Grind`, the
lowest nonce among the strongest. Every layer is checked again with
package `hash` before anyone counts it.

## Measurements

The machine:
- 4 vCPU Xeon at 2.10 GHz, with SHA-NI, AVX2 and AVX-512F.
- Go 1.27.3.

The numbers:
- Rates are the median of three 2-second `RunBench` runs per row, through
  `Default.Choose`, so they include the job's own path. The machine is a
  shared VM, so single runs vary by about ±15%.
- The "today" column is the default build at `9972137`.
- The "lanes" column is `GOEXPERIMENT=simd` with this branch.

| Hash | Threads | Today | Lanes | Speed-up |
| --- | --- | --- | --- | --- |
| SHA-256 | 1 | 108 ns (`cpu-midstate`, SHA-NI) | 30.5 ns (`cpu-sha256-avx512`) | 3.5× |
| SHA-256 | 3 (default) | 37.2 ns | 10.3 ns | 3.6× |
| SHA-256 | 4 | 27.5 ns | 7.5 ns | 3.7× |
| BLAKE3 | 1 | 116 ns (`cpu-blake3-cv`) | 23.7 ns (`cpu-blake3-avx512`) | 4.9× |
| BLAKE3 | 3 (default) | 40.6 ns | 8.2 ns | 4.9× |
| BLAKE3 | 4 | 30.5 ns | 6.6 ns | 4.6× |

A 12-second tip job in the opt-in build ran at its bench rate, 14.1 ns
against 14.4 ns on 2 threads, so the wiring costs nothing.

The other measurements:
- **Refactoring `layer.GrindWith`** onto the shared range loop is
  neutral. On 1 worker it was 113 ns against 117 ns at the base, and on 4
  workers 29.1 ns against 29.8 ns (`BenchmarkGrindHash/sha256`, 2 s × 3).
- **AVX2, eight lanes (`sha256LayerLanes8`)** runs at 95–110 ns per try
  on one core. That is no gain over SHA-NI's 108 ns. But Go's AVX2
  SHA-256 without SHA-NI (`GODEBUG=cpu.sha=off`) takes 218 ns, so on a
  CPU with AVX2 and no SHA-NI the eight lanes are about 2× faster. It is
  kept and tested, but not registered.

How the one-core rate came down:

| Step | ns/try |
| --- | --- |
| First version: rounds in a loop, the state copied every round | 30–32 |
| Rounds 0–7, which depend only on the entry, computed once per search; words 17, 19 and 21 constant, and 16, 18, 20 and 22 a constant plus one term | no change (the copies dominated) |
| Eight rounds unrolled by renaming, so no state is copied; `maj` as `b ^ ((a^b) & (b^c))`, reusing the last round's `a^b` | 30 |
| Lanes as `Int32x16`, not `Uint32x16`: Go 1.27 fuses only signed vectors' logic into three-input `VPTERNLOGD`. That gives 37 fused ops; `Σ0`, `Σ1`, `σ0`, `σ1`, `Ch` and `Maj` each become one | 26–29 (about 15% better) |

The remaining cost is about 2 vector ops per cycle, the AVX-512 port
limit. More would need fewer ops, not better scheduling.

## Idea

- **`sha256LayerLanes(entry, from, n)`** works because a layer hash is
  `SHA-256(entry ∥ nonce)`: 40 bytes and one padded block.
  - Words 0–7 (the entry) and 10–15 (the padding) are the same in every
    try. Only words 8 and 9 (the nonce) differ.
  - It returns the strongest nonce, the lowest among equals.
  - A vector compare skips the per-lane scan unless some lane's first
    word could beat the best.
- **`layer.Search`** is `func(entry, from, n) (nonce, strength)`.
  - `layer.GrindSearch` is `GrindWith` with a `Search`: the same workers,
    ranges, counter and cancel checks. Both now share one loop.
  - `engine.Grind.Search` and `grind.Grinder.Search` carry one, and
    `Plan.Engine` and `RunBench` pass it on.
  - Entries still use `New`, since an entry's try is a different message.
- **`lanes.go`** (`goexperiment.simd && amd64`) registers the two
  grinders in `init` when `archsimd.X86.AVX512()` holds.
  - Each gets `New` (for entries) and a `Search` that hands multiples of
    16 nonces to the lanes, then hashes any left over one at a time.
  - `poo grind devices` lists them as the CPU's fast grinders, and auto
    picks them.
- **Grey's decision is whether to ship an opt-in build.** For example, a
  release asset built with `GOEXPERIMENT=simd`, or a `make simd` target.
  Without one, the code waits behind the tag at no cost.

## Cost and risk

- `simd/archsimd` is experimental, and the build tag keeps it out of the
  default build. One compiler quirk is used: the signed-lanes trick for
  ternary logic. Unsigned vectors would only be about 15% slower if a
  later Go changes it.
- A bug in a lane search would only waste work, since every layer is
  verified again. The full test suite under `GOEXPERIMENT=simd` caught
  one during this round: a schedule word left at zero. It slipped past
  the first lane tests because their entries were mostly zero bytes. The
  lane tests now use full entries (`fullEntry`).
- About 400 lines, half of them tests.

## Next

- **Entries in lanes.** After ORANGE-009's saved chaining values, an
  entry try is one or two compressions, the same shape.
- **SHA-NI on two to four nonces at once** (`SHA256TwoRounds` on
  `Uint32x4`) for CPUs without AVX-512. Go's SHA-NI code does one stream,
  so it waits on the instruction's latency.
- Register the AVX2 lanes when a CPU has AVX2 and no SHA-NI.

## Prototype

On `claude/source-orange-r6`, commit `2a210ce`:
- `pkg/layer/layer.go`;
- `pkg/engine/engine.go`;
- `pkg/grind/{grind.go,bench.go,lanes.go,sha256_lanes.go,sha256_lanes8.go,sha256_lanes_test.go}`.

`TestLanesGrinders` checks that each grinder is the one chosen and that
`GrindSearch` matches `GrindHash` for 1, 15, 17, 10,007 and 65,536 tries
on 3 workers. The lane tests skip without AVX-512 or AVX2. The full
suite passes:
- `go test -race ./...` (default build);
- `GOEXPERIMENT=simd go test ./...`.
