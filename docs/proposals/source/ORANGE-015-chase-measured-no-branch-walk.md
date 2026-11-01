# ORANGE-015: The new chase measured; no branch walk on every event

| Field | Value |
| --- | --- |
| Status | adopted (Source Round 8, D122) |
| Based on | `claude/m10-watch` @ `0fd59fa` |
| Area | `pkg/engine` (`Engine.Tip`), `pkg/node` (`emit`), `pkg/p2p` (`ownWeight`) |
| Effort | small |
| Decided by | Grey (Black reviews in Part D) |

## Summary

**The chase itself scales well.** The D116 chase (halving, no depth
limit, yield and resume) resolves deep forks in a few dozen requests:
- a split 20,000 entries back with 25,000 new entries: 72 messages,
  64–104 ms;
- catching up 100,000 entries: 200 messages.

**The ledger around it did not scale.** Taking in an entry of a long
run without layers cost time that grows with the run. The 5,000th
entry took 19 µs, and the 30,000th took 293 µs.

So the total grows with the square of the run:
- catching up 40,000 entries took 9.3 s;
- building 50,000 locally took 33 s.

That is exactly the ledger shape D116 makes reachable: no bare-run limit,
and an entry is off-chain until a layer lands.

**The cause.** Every node event (`node.emit`) called `Engine.Status` only
to read the weight. `Status` lists every inactive branch, and
`Pool.Branches` walks each one back to the selected ledger to find its
fork point. The p2p host's `ownWeight` did the same.

**The fix.** `Engine.Tip` returns the tip, height and weight without the
branches, and both callers use it:
- an entry costs a flat 5 µs at any run length;
- catching up 40,000 entries takes 0.38 s instead of 9.3 s (24×).

No consensus change: what is read is the same weight.

## Measurements

Same machine as ORANGE-012. A test-only harness (two `startNode` nodes)
builds bare entries with `entry.Build` and a layer at the end.

### Taking in a bare run on one node

| Entries so far | Before (µs an entry, over the last 5,000) | After |
| --- | --- | --- |
| 5,000 | 18.6 | 5.0 |
| 10,000 | 49.8 | 4.8 |
| 15,000 | 117.8 | 6.7 |
| 20,000 | 182.0 | 5.0 |
| 25,000 | 253.3 | 4.0 |
| 30,000 | 293.1 | 7.9 |

The CPU profile before: 92% of the time is in `ledger.(*Pool).Branches`
under `node.(*Node).emit` → `engine.(*Engine).Status`.

### The chase, b reaching a's heavier ledger

| Case | Before | After | Messages b sent |
| --- | --- | --- | --- |
| Catch up 10,000 | 337 ms | 63 ms | 24 |
| Catch up 40,000 | 9.29 s | 376 ms | 83 |
| Catch up 100,000 | (not run before) | 1.09 s | 200 |
| Split 1,000 back; 200 own and 300 new entries | 21 ms | 21 ms | 7 |
| Split 10,000 back; 2,000 own and 3,000 new | 23 ms | 21 ms | 29 |
| Split 20,000 back; 10,000 own and 12,000 new | 42 ms | 22 ms | 40 |
| Split 50,000 back; 20,000 own and 25,000 new | 104 ms | 64 ms | 72 |
| Building the 50,000-split case locally (setup) | 33.4 s | 0.82 s | |

The message counts show the halving: about log₂ of the depth plus the
entries, in chunks.

A heavier ledger of 100,000 bare entries is reached in about a second.

## Idea

- `engine.(*Engine).Tip(k) (hash.Hash, int, weight.Weight, error)` is
  `Status` without `Branches`, `Pending` and `Waiting`.
- `node.(*Node).emit` and `p2p.(*Host).ownWeight` call it.
- Other `Status` callers run per request or per 2 s slice (RPC, watch
  queries, grind jobs), so they were left alone. `Pool.Branches` is still
  O(off-chain entries) per call (not measured here).

## Cost and risk

- 20 lines.
- No behaviour change: the same weight, read the same way.

## Prototype

On `claude/source-orange-r7`, commit `34e3eb7`, which also carries
ORANGE-016:
- `Engine.Tip` in `pkg/engine/engine.go`;
- `emit` in `pkg/node/node.go`;
- `ownWeight` in `pkg/p2p/peer.go`.
