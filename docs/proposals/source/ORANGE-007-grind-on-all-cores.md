# ORANGE-007: Grind on several cores, with the same result

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `0ac8e4e` (base `deecc80`) |
| Area | `pkg/layer` (`GrindHash`), `pkg/entry` (`BuildWorkers`); wiring in `pkg/engine`, node policy (M11) |
| Effort | small (the functions); M11 decides the wiring |
| Decided by | Grey, Source Round 5: as prototyped; M11 jobs and requests use it with `-threads` (D109) |

## Summary

Building an entry and grinding layers each use one core today, whatever
the machine. `layer.GrindHash` and `entry.BuildWorkers` split the nonces
between workers and return exactly what one worker returns:
- for a layer, the strongest layer, with the lowest nonce among equals;
- for an entry, the lowest nonce that reaches the minimum.

On 4 cores a SHA-256 layer try costs 28.7 ns, 5.6 times faster than
today's 161 ns. How many cores a node may use is a node policy question
for M11, so the engine still uses one.

## Problem

`Engine.GrindLayer` and `Engine.BuildEntry` run `layer.Grind` and
`entry.BuildContext` on the calling goroutine. A builder that wants all
its cores has to run several `poon work` commands on different nonce
ranges and keep the best by hand.

## Measurements

Same machine as ORANGE-006, 4 vCPU. ns per try, means of 3 runs:
`go test -bench 'GrindHash|BuildTry' -count 3 ./pkg/layer ./pkg/entry`.

| Hash | Layer, today | Layer, 1 worker | Layer, 4 workers | Entry 256 txs, today | 1 worker | 4 workers |
| --- | --- | --- | --- | --- | --- | --- |
| SHA-256 | 161 | 113 | 28.7 | 10,742 | 113 | 57.8 |
| SHA-512/256 | 372 | 324 | 86.5 | 20,593 | 332 | 140.7 |
| SHA3-256 | 547 | 461 | 132 | 31,570 | 465 | 198.7 |
| BLAKE2b-256 | 270 | 221 | 56.8 | 14,388 | 225 | 82.0 |
| BLAKE3 | 181 | 135 | 33.2 | 7,002 | 702 | 248.5 |

Layers scale almost linearly (3.5 to 4.1 times). Entries scale by 2 to 2.8
times. Their search runs in rounds of 4,096 nonces per worker, and every
round waits for its slowest worker so that the lowest nonce is still the
one returned. Larger rounds, or workers that keep going past a round,
would close the gap. An entry search usually ends in a few rounds anyway
(the minimum entry strength is low next to layers).

## Idea

- `GrindHash` gives each worker one contiguous range of the nonces, each
  with its own `hash.Grinder`. It keeps the strongest result, and the
  earlier range among equal strengths.
- `BuildWorkers` searches in rounds. In round r, worker w tries the w-th
  run of 4,096 nonces. After the round, the lowest find wins, since every
  lower nonce has been tried. It checks the context between rounds, as
  `BuildContext` did (RED-006).
- For M11: a node policy setting such as `grind_workers` (0 = all cores),
  passed through `Engine.GrindLayer`, `Engine.BuildEntry` and `poon work`.
  Defaulting to 1 keeps today's behaviour.

## Cost and risk

- About 120 lines. `TestGrindHashMatchesGrind` compares the result with
  `layer.Grind` for every hash, 0 to 64 workers, and ranges that run to
  the top of the nonce space. `TestBuildWorkersMatchesSearch` does the
  same for entries, with limits before, at and after the answer.
  `TestBuildWorkersCancel` checks cancelling. All pass under `-race`.
- `BuildWorkers` never tries nonce 2⁶⁴−1 when there is no limit. The old
  loop could, after 2⁶⁴ tries.
- More cores means more power drawn and more heat. That is the operator's
  choice, which is why it belongs in node policy.

## Prototype

On `claude/source-orange-r4`, commit `c4bfa0d`: `GrindHash` and
`BuildWorkers`, with their tests and benchmarks. Nothing calls them with
more than one worker yet.
