# ORANGE-029: Round 19 measured; ask weights within the peer's request budget

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `4240d32` |
| Area | `pkg/p2p` (`checkLoop`, a new peer's first weight requests, new `sweepWeights`) |
| Effort | one function and a test |
| Decided by | Grey, with Black (it changes when the weight check asks, D153) |

## Summary

I measured what Round 19 changed: D152's start-up and pool index, and
D153's weight check that can no longer be turned off. D152 does what it
says. While measuring the weight check, I found a problem that is older
than Round 19: a node that follows a large series whole gets timed out
by its own peers.

**D152: start-up of a series followed whole.** One series of n pools,
followed whole, with nothing stored:

| Pools | `node.Open` on `533dbcd` | on `4240d32` | `p2p.New` on `533dbcd` | on `4240d32` |
| --- | --- | --- | --- | --- |
| 65,536 | 89 ms | 4 ms | 134 ms, 39 MiB | 67 ms, 23 MiB |
| 1,048,576 | 1.46 s | 5.5 ms | 5.19 s, 629 MiB | 1.12 s, 368 MiB |

`node.Open` no longer depends on the number of pools. The p2p index
went from 629 to 368 bytes a pool. It is still one entry per pool,
which D152 leaves to a protocol change.

**The problem: weight requests over the peer's budget.** When a peer
connects, and at every check (`check_seconds`, a minute by default),
the node asks the peer for its weight in each pool. It asked for every
pool at once, one goroutine each. A peer takes 600 requests a minute
from one address (tracking budget "request"). Past that it drops
requests without a reply, and flags the sender as over budget, once a
minute. So with more than about 600 pools followed whole:
- Most of the weights asked for are never answered. Those that are
  are a random part of the pools.
- Every check adds a flag. Ten flags time the node out, so a check
  every minute has an honest node timed out by its peers within ten
  minutes. Three time-outs get it blocked for a day.

Two nodes that follow a series of n pools whole, connected, with the
windows shortened from a minute to 2 s (so 40 s is 20 minutes):

| Pools | Before (`533dbcd` and `4240d32`) | This change |
| --- | --- | --- |
| 256 | no flags, all answered | the same |
| 600 | 2 flags; 576 of 9,000 unanswered | no flags |
| 1,024 | 10 flags: timed out; 4,592 of 9,985 unanswered; 2,489 goroutines | no flags; none unanswered; 35 goroutines |
| 4,096 | 5 flags in 30 s; 16,833 of 19,283 unanswered; 14,001 goroutines | (not run; the same pace) |

D153 makes this matter more. `check_seconds: -1` was the way out, and
now the longest interval is an hour. An hour does keep the flags from
adding up, but a new peer still gets every request at once.

**The fix: sweep each peer at a pace.** `sweepWeights` asks one peer
for the pools in order, in one goroutine:
- At most 8 requests wait for a reply at a time.
- In each pace window it sends at most half of the pace that chases use
  (`RequestPace`, 480 a minute, or the pace learned from the peer). The
  other half stays free for chases.
- It waits for room. It never skips a pool, so every pool is still
  asked. A large series takes more windows: 1,024 pools take about
  4 minutes per peer.
- While a sweep to a peer runs, a new check does not start another one
  to it.

Nothing about what a node accepts changes. A heavier ledger is found as
before, only spread over the windows the peer allows. Before, it was
mostly not found at all, because the requests were dropped. Under the
owner's definition, a weight learned later is a lack of information,
not a hard fork. Since it changes when D153's check asks, I put it to
Black.

No consensus change.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **Start-up:** a test-only harness. It saved a node policy that
  follows a series of n pools (`PoolSize` 1) whole, then timed
  `node.Open` and `p2p.New`, with the heap growth from
  `runtime.MemStats`. It ran on `533dbcd` and on `4240d32`.
- **Weight requests:** a test-only harness with two such nodes on
  loopback, using the default trackers and pace, except that the budget
  and pace windows were 2 s in place of a minute. b connects to a, with
  a check every 2 s. After 30–40 s it read each tracker's flags,
  answered and timed-out requests, and the peak goroutines. It ran on
  `533dbcd`, `4240d32` and this change.

The harnesses are not committed. The test below is.

## Idea

- **`sweepWeights(p, pools)`** in `pkg/p2p/sweep.go`, as described
  above. `peer.sweeping` keeps it to one at a time per peer.
- **`checkLoop`** groups the pools by priority peer and starts one sweep
  per peer. A new peer gets one sweep of the pools it covers.
- **Test:** `TestWeightSweepWithinBudget`. Two nodes follow 700 pools
  whole, with 2 s windows. Every pool's weight is answered both ways,
  none is left unanswered, and neither node flags the other. On
  `4240d32` it fails: the pools are never all answered.

## Cost and risk

- About 80 lines and a test.
- A pool's weight from a peer can arrive later than before: up to the
  time a sweep takes, about n / 240 minutes for n pools. Before, past
  600 pools, most of them never arrived.
- The full suite passes with `go test -race ./...`.

## Prototype

On `claude/source-orange-r19`; see the commit "ORANGE-029".
