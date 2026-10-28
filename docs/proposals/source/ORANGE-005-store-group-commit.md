# ORANGE-005: Group commit for the store

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `7e68fe0` (base `23e268f`) |
| Area | `pkg/store` (`Log`), `pkg/node` (`record`, `Handle*From`) |
| Effort | medium |
| Decided by | Grey, Source Round 4: the event fires after the write, before the fsync; the Handle call still returns after it (D105) |

## Summary

With the default node policy every kept transaction, entry and layer is
fsynced before the node moves on, and the node holds its lock through
that fsync. So a node stored about 3,000 items a second however many
cores or peers it had, and layers, the most frequent item, paid the same.
The node now writes the record under its lock and waits for the disk
after releasing it, and items that wait together share one fsync. With
many peers on 4 cores a node takes in 17,400 transactions a second, 6
times as many. Every Handle call still returns only once its item is on
disk. One thing changes and needs Grey's decision: the node's event (which
forwards the item and feeds `watch`) now fires after the write and before
the fsync.

## Problem

`Node.record` calls `store.Log.Append`, which writes, flushes and calls
`f.Sync()`, inside `n.mu`. An fsync costs about 185 µs on this machine
(`BenchmarkAppend/sync=true`) and far more on a spinning disk. Nothing
else can happen on the node meanwhile: no other peer's item, no RPC
write, no build.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (cloud container, overlay file
system), Linux, Go 1.27.3.

`go test -run '^$' -bench 'HandleTxParallel' -benchmem -cpu 1,4 -count 2 ./pkg/node`,
means. `peers=1x` runs one goroutine per core; `peers=16x` sixteen per
core, as a node with many peers does. "Before" is the base; "after" has
ORANGE-002, 004 and 005.

| Benchmark (default sync) | Before | After |
| --- | --- | --- |
| `peers=1x`, 1 core | 450 µs (2,200/s) | 395 µs (2,500/s) |
| `peers=1x`, 4 cores | 316 µs (3,200/s) | 209 µs (4,800/s) |
| `peers=16x`, 1 core | 417 µs (2,400/s) | 154 µs (6,500/s) |
| `peers=16x`, 4 cores | 343 µs (2,900/s) | 57.5 µs (17,400/s) |
| `store` `BenchmarkAppend/sync=true` (one writer) | 187 µs | 203 µs (within noise: 171 to 235) |

## Idea

- `store.Log.AppendLater(rec)` writes and flushes the record, as `Append`
  did, and returns a ticket (the count of appends so far) without syncing.
  The record already survives a crash of the process.
- `store.Log.Durable(ticket)` returns once that append is on disk. One
  caller at a time syncs; it notes how many appends the file held before
  syncing and marks them all done, so callers waiting behind it usually
  return without a sync of their own. `Rewrite` and `Close` sync
  everything and mark it done, so a wait that overlaps them still
  succeeds. `Durable` is the one `Log` method that may run beside the
  others; a small mutex guards the file swap in `Rewrite`.
- `Append` is `AppendLater` then `Durable`, so every other caller is
  unchanged.
- `Node.record` returns a function that waits for the ticket. The three
  `Handle*From` methods call it after releasing `n.mu`, and return its
  error.

## Decision for Grey

Events are emitted under `n.mu`, so observers never run concurrently, and
that has to stay. So the event for an item now fires once it is written,
while its fsync may still be running. On a power loss (not a process
crash) at that moment, the node may have forwarded an item it no longer
holds after restart. Its peers hold it and it can be fetched again, the
same as with `no_sync`. The alternative keeps the event after the fsync
by emitting outside the lock in a queue, which is a larger change.

## Cost and risk

- About 90 lines in `pkg/store` and 60 in `pkg/node`, plus tests.
- A new concurrency edge in the store. `TestDurableGroupCommit` appends
  from 8 goroutines that wait outside the lock while the log is rewritten
  mid-run and then closed, and checks that every record is read back and
  no wait fails. It passes under `-race -count 3`, with the whole race
  suite.
- One writer gains nothing: the single-writer append cost is unchanged.

## Prototype

On `claude/source-orange`, commit `e4ce9e6`: `pkg/store/store.go`,
`pkg/store/durable_test.go`, `pkg/node/node.go` and the `peers` cases of
`pkg/node/bench_test.go`.

## Round 4 check

Re-measured by Orange on `claude/m10-watch` @ `0ac8e4e` (base `deecc80`),
same machine and commands. Default sync, `peers=16x`, 4 cores: 48.3 µs per transaction (20,700/s,
against 2,900/s before ORANGE-005); `peers=1x`, 4 cores: 180 µs.
