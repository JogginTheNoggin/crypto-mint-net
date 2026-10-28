# ORANGE-004: Verify signatures before the node's lock

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `7e68fe0` (base `23e268f`) |
| Area | `pkg/engine` (`Preverify`), `pkg/node` (`HandleTxFrom`, `HandleEntryFrom`), `pkg/tx`, `pkg/series`, `pkg/policy` |
| Effort | small (needs ORANGE-002) |
| Decided by | |

## Summary

A node takes items from many peers at once, but `Node.HandleTxFrom` and
`HandleEntryFrom` hold the node's lock (and inside it the engine's) while
every signature is verified. So a node checked one transaction at a time,
about 13,000 a second, whatever its cores. With ORANGE-002 in place, the
node now verifies an item's signatures before taking its lock and
remembers the passes; the checks under the lock find them remembered. On
4 cores a node takes in 35,000 transactions a second (2.7 times as many),
and the engine alone 41,600. Results are unchanged.

## Problem

`p2p` calls `Node.HandleTxFrom` from each peer's goroutine. That takes
`Node.mu`, then `Engine.HandleTx` takes `Engine.mu`, and `mempool.Add`
runs `tx.Check`, about 70 µs of Ed25519 work per input. Every other peer
waits. Entries are worse: every transaction in the entry is verified under
both locks.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (cloud container), Linux, Go 1.27.3.

New benchmarks feed distinct, valid claims of one series from parallel
goroutines. `go test -run '^$' -bench HandleTxParallel -benchmem -cpu 1,2,4 -count 3 ./pkg/engine`
and the same with `-cpu 1,4 -count 2` for `./pkg/node` (the node with `no_sync`, so the disk does
not hide the effect; see ORANGE-005 for the default). Means; time per
transaction across all goroutines.

| Benchmark | Before | After |
| --- | --- | --- |
| engine, 1 core | 69.6 µs (14,400/s) | 67.2 µs (14,900/s) |
| engine, 2 cores | 76.8 µs (13,000/s) | 39.1 µs (25,600/s) |
| engine, 4 cores | 76.3 µs (13,100/s) | 24.0 µs (41,600/s) |
| node `no_sync`, 1 core | 66.8 µs (15,000/s) | 69.0 µs (14,500/s) |
| node `no_sync`, 4 cores | 77.5 µs (12,900/s) | 28.6 µs (35,000/s) |

## Idea

- `policy.Registry.Preverify(rules, w, tid, v)` runs the size and allowed
  checks and then `Satisfy`, remembering a pass in the series' list
  (ORANGE-002). It returns nothing: the full check under the lock decides
  and gives the error, in the same order as today.
- `series.Series.Preverify` and `tx.Preverify` wrap it per witness.
- `Engine.Preverify(k, txs...)` takes the engine lock only to find pool
  k, and returns at once when the pool is not followed, so nothing is
  verified that the engine would not have verified anyway.
- `Node.HandleTxFrom` and `HandleEntryFrom` call it before `n.mu.Lock()`.
  The sim, the demo and other direct engine callers are unchanged.

## Cost and risk

- A transaction that fails its signature is now verified twice: before
  the lock and again under it (failures are not remembered, so the error
  and its order stay exactly as today). The first one runs in parallel,
  so the time the lock is held for it is unchanged. **For Red:** this
  doubles the CPU a peer can make the node spend per bad signature, still
  bounded by peer scoring; a small list of recent failures would remove it
  if Red thinks it matters.
- Two peers sending the same new item at the same moment may both verify
  it. After that it is remembered.
- About 60 lines including benchmarks.

## Prototype

On `claude/source-orange`, commit `90e7b24`: the functions above,
`pkg/engine/bench_test.go` and `pkg/node/bench_test.go`. The race test
suite passes.
