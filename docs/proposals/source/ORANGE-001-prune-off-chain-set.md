# ORANGE-001: Rank inactive branches without scanning every entry

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `7e68fe0` (base `23e268f`) |
| Area | `pkg/ledger` (`Pool.prune`, `offChainLeaves`, `Branches`) |
| Effort | small |
| Decided by | |

## Summary

Every new entry, and every layer on an entry outside the selected ledger,
made the pool walk every entry it holds to find the inactive branches, and
sort them with a comparison that recomputed two ledger weights each time.
Adding an entry therefore got slower as the ledger grew: about 450 µs on a
10,000-entry ledger, five times the cost of the entry itself. With a set
of the off-chain entries and each weight worked out once, it stays near
90 µs at any length, and a 64-node simulation runs 21% faster on this
change alone. What is pruned, kept and selected is unchanged.

## Problem

`prune` runs at the end of `AddEntry` and of every off-chain `AddLayer`.
It calls `offChainLeaves`, which ranges over `p.nodes`, the map of every
held entry, selected ones included, to pick the off-chain leaves. The
selected ledger only grows, so this is O(ledger length) per item. The
sort then calls `better`, which calls `weightOf` twice per comparison,
each a Fenwick prefix sum of `math/big` additions, so the sort costs
O(b log b · log n) big-number additions for b branches.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (cloud container), Linux, Go 1.27.3.

`go test -run '^$' -bench 'Extend|Switch|LayerOnOld' -benchmem -count 3 ./pkg/ledger`,
means of 3 runs. `BenchmarkExtendLong` is new: an entry on the tip of a
ledger already `len` entries long, then a layer that selects it. Its
ledger keeps growing by `b.N` (about 10,000) during the run, which is why
`len=100` was already slow before.

| Benchmark | Before | After |
| --- | --- | --- |
| `ExtendLong/len=100` | 218 µs | 83 µs |
| `ExtendLong/len=1000` | 247 µs | 83 µs |
| `ExtendLong/len=10000` | 449 µs, 202 allocs | 90 µs, 196 allocs |
| `Extend` | 242 µs | 86 µs |
| `LayerOnOldEntry` | 1.34 µs | 1.26 µs |
| `Switch/depth=1` | 8.9 µs | 9.6 µs |
| `Switch/depth=10` | 37.0 µs | 39.2 µs |
| `Switch/depth=100` | 253 µs | 257 µs |

A switch now also moves each reverted and applied entry in or out of the
set: 1 to 8% slower on a switch, which is rare next to extensions.

Simulator: a test-only benchmark running `sim.Load` with 64 nodes, 16
builders, a payment every 100 ms for 120 s (seed 1), the same as
`poo sim load -nodes 64 -builders 16 -pay-every 100 -duration 120000`.
CPU profile before: `offChainLeaves` 14.1% of all CPU (10.1% in the sort's
`better`), `prune` 14.6%.

| Run | Wall time | Allocated | Allocations |
| --- | --- | --- | --- |
| Before | 41.0 s | 6.65 GB | 135 M |
| ORANGE-001 alone | 32.5 s | 4.30 GB | 65 M |

The report (agreement, conservation, 1,104 of 1,134 settled, mean wait
686.87 ms, sibling share 0.01786, 2,917,460 messages) is identical to the
last digit.

## Idea

- `Pool.off` is the set of held entries outside the selected ledger. It
  changes in four places only: an entry is added (off), `switchTo` moves
  reverted entries in and applied ones out, and `prune` deletes what it
  drops.
- `offChainLeaves` ranges over `off` and returns each leaf with its
  weight, computed once, sorted by weight then first sighting, the order
  `better` gives. `prune` and `Branches` use those weights.

## Cost and risk

About 40 lines in `pool.go`. The ranking order is total (`seq` is unique),
so the result does not depend on map order, before or after. The risk is
the set going stale if a future change adds a fifth place that moves an
entry on or off the selected ledger. The new test guards it.

## Prototype

On `claude/source-orange`, commit `da34cfb`:
- `pkg/ledger/pool.go`: the `off` set, `leaf`, and the changes above.
- `pkg/ledger/export_test.go`: `CheckOff` compares the set with a full scan
  and `Branches` with a brute-force ranking.
- `pkg/ledger/property_test.go`: `TestRandomPruned` runs the randomized
  steps with K = 1 and Ties = 2 (411 prunes across 12 seeds) and checks
  `CheckOff` after every step; the model test checks it too.
- `pkg/ledger/bench_test.go`: `BenchmarkExtendLong`.

## Further leads (not prototyped)

After ORANGE-001 to 004, the same simulator profile is led by
`HandleLayer` (38%): `weight.Add` allocates a new `big.Int` per addition
(12% of CPU, 47% of allocated bytes before), the layer hash is computed
twice (in `Engine.HandleLayer` and again in `Pool.AddLayer`), and `prune`
still runs on every off-chain layer (11%). Each is a candidate for a
later round.
