# RED-010: A failed fsync of the store is retried and a later waiter is told the record is on disk

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | `claude/m10-watch` @ 0ac8e4e (Source Round 4 Part A) |
| Component | pkg/store/store.go (Log.Durable, Log.AppendLater), pkg/node/node.go (record) |
| Surface | node files / disk fault |
| Fixed by | — |

## Summary

The group commit (ORANGE-005, D105) lets several waiters share one fsync. In
`Log.Durable`, a failed `f.Sync()` returns the error to the caller that ran
it and leaves `l.synced` unchanged. The next waiter, which finds
`l.synced < ticket`, runs `f.Sync()` again:

```go
// pkg/store/store.go, Durable
f, upto := l.f, l.written
...
err := f.Sync()
...
if err != nil { return err }     // synced is not advanced; nothing is latched
l.synced = upto                  // a later, successful Sync covers the same records
```

On Linux a failed `fsync` reports its error once and then marks the dirty
pages clean, so a second `fsync` of the same file normally **succeeds**
without the data having reached the disk (the "fsyncgate" behaviour that
PostgreSQL and others had to make fatal). Here that means: the waiter that
ran the failing sync is told the record was not stored, but a waiter queued
behind it on `syncMu` retries, succeeds, sets `synced = upto` (which includes
the records of the failed sync) and returns nil. Every later `Append` or
`Durable` on that log also succeeds, and `Close` ends with
`synced = written`. The node then runs on, serving, forwarding and building on
records it believes durable that are not on disk.

The old per-record `Append` returned the error too and then carried on in the
same way, so the underlying weakness is not new. The group commit makes it
concrete inside one batch (one caller sees the error, the next is told all is
well) and the node's own comment in `Durable` shows the retry is deliberate.

## Reproduction

A `pkg/store` test that wraps the file so that its first `Sync` fails and the
second succeeds (the `Log` holds a concrete `*os.File`, so this needs a small
`Sync() error` seam), then: `AppendLater` two records; run `Durable` for the
first in one goroutine and the second in another. Expect: one gets the error,
the other gets nil, and `l.synced` covers both. Today it does.

## Impact

Low. It needs a disk fault (I/O error on writeback, a full or failing device,
a network or thin-provisioned volume that reports the error at fsync), not an
action by a peer or by the owner, and what is lost is the tail of a log that
peers also hold, which the node fetches again (D105 says the same of events
before the fsync). It is still wrong for a node to carry on after the disk has
said it did not keep what it was asked to keep, and to say afterwards that it
did.

## Recommendation

Latch the first sync error on the `Log` (`l.err`): once set, every later
`AppendLater`, `Durable`, `Rewrite` and `Close` returns it, and `Durable` never
advances `synced` past it. Have the node treat that error on a pool's store as
fatal for the pool (stop following it and say so in the log, or stop the node),
so a restart replays what is really on disk.

Test: as above, plus that after a failed sync no later `Durable` returns nil.
