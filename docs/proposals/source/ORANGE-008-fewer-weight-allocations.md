# ORANGE-008: Fewer big-number allocations, and hash a layer once

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `0ac8e4e` (base `deecc80`) |
| Area | `pkg/weight` (`Sum`), `pkg/ledger` (`fenwick`, `AddLayerHashed`), `pkg/engine` (`HandleLayer`) |
| Effort | small |
| Decided by | Grey, Source Round 5: as prototyped |

## Summary

With ORANGE-001 to 005 adopted, a 64-node simulation runs in 9.0 s.
Weight arithmetic and layer handling were what remained:
- Each Fenwick prefix sum allocated a new big number at every step.
- The selected ledger's weight was summed again on every call.
- Every layer was hashed twice.

Three small changes fix this:
- Prefix sums now add into one value (`weight.Sum`).
- The tree keeps the selected total.
- The engine hands its layer hash to the pool.

Results:
- Allocations in the simulation drop by 26% and allocated bytes by 15%.
- Extending a long ledger takes 76 µs instead of 89 µs, with 61% fewer
  allocations.
- The simulation report is identical.

## Problem

- `fenwick.prefix(i)` did `sum = sum.Add(t[j])` for each of up to log n
  steps. `Weight.Add` allocates a new `big.Int` each time, since weights
  are immutable.
- `Pool.Weight()` (the selected ledger's weight) is `prefix(len)`. It is
  called on every `AddEntry`, every off-chain `AddLayer` and every
  `prune`.
- `Engine.HandleLayer` hashes the layer for the seen list, then
  `Pool.AddLayer` hashes it again.

## Measurements

Same machine as ORANGE-006.

The 64-node simulation (see ORANGE-001), run twice:

| Run | Wall time | Allocated | Allocations |
| --- | --- | --- | --- |
| Base `deecc80` | 8.99 s | 3.62 GB | 58.6 M |
| ORANGE-008 | 8.77 s, 8.74 s | 3.08 GB | 43.2 M |

`go test -bench 'ExtendLong|Switch|LayerOnOld' -count 2 ./pkg/ledger`:

| Benchmark | Base | ORANGE-008 |
| --- | --- | --- |
| `ExtendLong/len=10000` | 89.3 µs, 193 allocs | 76.4 µs, 74 allocs |
| `ExtendLong/len=100` | 77.5 µs, 178 allocs | 69.5 µs, 72 allocs |
| `Switch/depth=100` | 212 µs, 1,612 allocs | 199 µs, 1,469 allocs |
| `Switch/depth=1` | 6.09 µs | 5.78 µs |
| `LayerOnOldEntry` | 1.14 µs, 11 allocs | 1.14 µs, 12 allocs |

## Idea

- `weight.Sum`: a value that grows in place. `Add(w)` adds to it, and
  `Weight()` hands the result over and empties the sum, so the result
  stays immutable.
- `fenwick` keeps `total`, updated by `push`, `add` and `truncate`.
  `prefix(len)` returns it, and `prefixPlus(i, extra)` sums with one
  allocation. `Pool.weightOf` uses `prefixPlus` for an off-chain entry.
- `Pool.AddLayerHashed(l, lh)` takes the hash the caller already has.
  `AddLayer` is `AddLayerHashed(l, l.Hash(...))`, and
  `Engine.HandleLayer` calls `AddLayerHashed`.

## Cost and risk

About 60 lines. The cached total is checked against a plain sum of the
chain after every step of the randomized tests (`CheckOff`, with and
without pruning). A first version that recomputed the total from the
cache itself was caught by those tests at once. `TestSum` checks that a
handed-over sum never changes.

## Prototype

On `claude/source-orange-r4`, commit `41a84d7`.

## Further leads (not prototyped)

In the same profile, the rest of the node's own time is:
- Hashing, 13%. `tx.TID` (8.5%) runs 2 or 3 times for each transaction
  of an entry: `Entry.Hash` in `HandleEntry` and `PreverifyEntry`, then
  `CheckStructure` in `AddEntry`. Passing the tids through once would
  remove most of it.
- `prune`, 8.5%. It still ranks every off-chain leaf on every off-chain
  layer.
- Seen-list lookups, 7%.
