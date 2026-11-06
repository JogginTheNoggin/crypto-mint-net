# ORANGE-050: Round 40 checked; the rename to `poon` costs nothing, and one benchmark runs again

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `46b638d` |
| Area | the whole tree (D204, `poo` renamed to `poon`), `pkg/node` (`BenchmarkHandleTxParallel`) |
| Effort | one benchmark fix; no proposals |
| Decided by | Grey |

## Summary

D204 renames the tool from `poo` to `poon`. BASE.md said there was
nothing new to measure, so I checked that the rename slowed nothing down.

**1. The rename changes names, messages and comments only. The
benchmarks are the same before and after.**

- Outside tests, D204's Go changes are help text, error messages,
  comments and the names in `pkg/brand`. The rest is GUI-GREEN-065: the
  peers asked in `poon bounty list -ask`, and a held offer counted as
  filled once its layers are accepted. Neither is on a hot path. No code in `pkg/hash`, `pkg/grind`, `pkg/ledger`,
  `pkg/engine`, `pkg/tx` or `pkg/wire` changed apart from comments.
- The only change on the wire is the hello's agent field, `poon` in
  place of `poo`: one byte more in each hello.
- I ran the 66 benchmarks in `pkg/node`, `pkg/engine`, `pkg/ledger`,
  `pkg/tx`, `pkg/entry`, `pkg/layer` and `pkg/hash`, three times each,
  at `fa8dad7` (before) and `46b638d` (after). The geometric mean of
  after over before is 1.001.
- Single benchmarks moved from −17% to +13%, in both directions. The
  largest (`ExtendLong` +11%, `Grind/sha256` +10%) went away when run
  again in turns: `ExtendLong` 60–67 µs both before and after, and
  `Grind/sha256` 134–162 ns after against 141–154 ns before. That is
  noise on this shared machine.
- `go build`, `go vet` and `go test ./...` pass at `46b638d`.

**2. `BenchmarkHandleTxParallel` had stopped running. Fixed here.**

Its sub-benchmarks failed before the rename too:

```
nodepolicy: invalid: max_pending is 2; it takes 16 to 1048576
```

The benchmark sets `max_pending` to `b.N + 1`, and the first round has
`b.N` = 1. Since D159 gave `max_pending` a floor of 16, that policy is
refused. It now sets `max(b.N+1, 16)`. With the fix, at `46b638d`:

| sub-benchmark | per claim | claims a second |
| --- | --- | --- |
| no sync, 1 goroutine per core | 41–43 µs | about 24,000 |
| sync, 1 goroutine per core | 210–211 µs | about 4,750 |
| sync, 16 goroutines per core | 51–53 µs | about 19,500 |

These match the 20.7k tx/s of the earlier default-sync node figures.

## Proposals

None.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3.

- "Before" is a worktree at `fa8dad7`, the main before Round 40 Part A,
  and "after" is `46b638d`.
- `go test -run '^$' -bench . -benchtime 1s -count 3` on the seven
  packages, after first, then before. The ratios are medians of three.
- The two largest movers were then run twice more each, in turns.
- `BenchmarkHandleTxParallel` with the fix ran twice. `go test -race
  ./pkg/node` passes.

## What changed

- `pkg/node/bench_test.go`: `BenchmarkHandleTxParallel` sets
  `max_pending` to at least 16, the floor.
- This document and its row in the proposals README.

## Consensus impact

None. The only code change is in a benchmark. The rename itself changes
no consensus bytes: series, ledger, entry and layer encodings are as
before, and the hello's agent is a free text field. No verdict changes
and no depth limit is added.
