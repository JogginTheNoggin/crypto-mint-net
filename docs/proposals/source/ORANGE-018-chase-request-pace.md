# ORANGE-018: The B1 chase measured; pace requests under the sender's budget

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `ae8a103` |
| Area | `pkg/p2p` (`Config.RequestPace`, `RequestPaceWindow`, new `pace.go`, `peer.reqs`, the chase loops) |
| Effort | small |
| Decided by | Grey, with Red (Black reviews in Part D) |

## Summary

**A D115 problem under default settings.** Two ledgers that are heavier
are never reached from a peer running the defaults:
- a heavier branch whose newest entries carry no layers but the last
  one, when there are about 65,000 or more of them (between 100,000 and
  200,000 when the node already holds them without layers);
- the same entries with more layers deeper than about 75,000 entries
  below the tip (the B1 top-up).

**Why.** Checking such a branch takes two requests per 256 hashes: a
hash page and a layers page. A peer answers 600 requests a minute from
one address by default (tracking budget `request`). Past that it drops
requests without a reply. The chase waits `RequestTimeout` (30 s), ends
as "nowhere", and drops everything it took in (`st.drop()`). The next
chase starts again from the common entry, uses up the same budget at
the same point, and fails again. No amount of waiting helps.

D120's pacing of weightless pages does not prevent it. It yields every
64 pages (16,384 hashes) for 5 s, which is still 128 requests per 5 s
against a budget of 50 per 5 s.

**The fix (node policy).** The chase counts the requests it sends each
peer over the last minute. At `RequestPace` (480 by default, 80% of the
default budget), it yields and keeps its progress, like any other yield.
It goes on when the window has room. Nothing is refused, and the sender
never sees more than its budget.

| Case, defaults | Before | With pacing |
| --- | --- | --- |
| 100,000 bare entries, b holds none | never (stuck at height 5) | 2 m 1 s, 985 requests |
| 200,000 bare entries, b holds them without layers (B1) | never | 2 m 17 s, 1,178 requests |
| Layers 100,000 entries deep (B1 top-up) | never | 60 s, 786 requests |

Below the budget nothing changes. A chase that needs fewer than 480
requests a minute is never paced.

The rest of the B1 path scales well:
- **Walking back over a held branch** costs no requests: 10,000 held
  entries in 63 ms, with 65 requests.
- **Topping up layers** costs two requests per 256 entries of depth:
  50,000 deep in 183 ms.
- **Many layers on one entry** (C-3): 100,000 layers in 0.5 s whole, or
  0.7 s paged, with 7 or 8 requests.

No consensus change.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- A test-only harness with two `startNode` nodes and the default
  `p2p.Config` and tracking budgets.
- The test default `RequestTimeout` is 5 s (production: 30 s).
- "Requests" is b's sent message count to a.
- "Never" means b had not moved after 3 minutes, with "request timed
  out" in its log after the requests went over a's budget.
- Without pacing means `RequestPace = -1` on b.

### Bare runs: a's heavier ledger ends in N entries with no layers but the last

| N | b holds the entries? | Without pacing | With pacing | Requests | Peak heap, both nodes |
| --- | --- | --- | --- | --- | --- |
| 1,000 | yes | 23 ms | same | 11 | 27 MB |
| 10,000 | yes | 63 ms | same | 65 | 53 MB |
| 10,000 | no | 110 ms | same | 107 | 44 MB |
| 50,000 | yes | 15.4 s | same | 299 | 160 MB |
| 50,000 | no | 15.6 s | same | 497 | 135 MB |
| 100,000 | yes | 31 s | 66 s | 592 | 300 MB |
| 100,000 | no | never | 2 m 1 s | 985 | 232 MB |
| 200,000 | yes | never | 2 m 17 s | 1,178 | 539 MB |

From 50,000 up, the time is D120's pacing: a 5 s wait every 16,384
weightless hashes. The CPU is mostly idle while it waits.

With a's request budget raised to 100,000, the 100,000 case with no
entries held took 31 s. So the failure is the budget, not the chase.

At 100,000 held, unpaced is faster (31 s against 66 s) because 592
requests fit in 600. That is a narrow margin: any other request to the
same peer that minute (a sync, a second pool) tips it over.

For comparison, ORANGE-015 measured catching up 100,000 bare entries in
1.09 s before D120 added the weightless pacing. With a layer on every
entry, D120 does not pace, and 100,000 entries are reached in 2.2 s with
205 requests, since the work counted beats b's ledger early.

### B1 top-up: same entries, heavier layers N entries below the tip

| Depth | Without pacing | With pacing | Requests |
| --- | --- | --- | --- |
| 1,000 | 7 ms | same | 12 |
| 10,000 | 27 ms | same | 84 |
| 50,000 | 183 ms | same | 396 |
| 100,000 | never | 60 s | 786 |

### C-3: many layers on one entry

| Layers | Paged? | Time | Requests | Peak heap |
| --- | --- | --- | --- | --- |
| 10,001 | no (`LayersWhole` default) | 45 ms | 7 | 36 MB |
| 10,001 | yes (`LayersWhole` 1) | 61 ms | 8 | 32 MB |
| 100,001 | no | 518 ms | 7 | 136 MB |
| 100,001 | yes | 725 ms | 8 | 156 MB |

## Idea

- **`pace.go`:** a `reqWindow` per peer counts requests sent in 60
  buckets over `RequestPaceWindow` (one minute by default, the budget's
  window). `Host.ask` adds one per request.
- **`Host.paced(p)`:** reports whether `RequestPace` requests went to p
  in the window.
- **The chase loops** check it before each request and yield with
  "requests to the sender paced": `checkWork`, `fetchBranch`, `fetchAll`
  and `topUpLayers`. A yield keeps its progress (`saveProgress`), so the
  next slice goes on from where it stopped.
- **`Config.RequestPace`:** default 480; negative turns pacing off.
- **Tests:**
  - `TestReqWindowCounts` covers the window.
  - `TestLongChaseIsPaced` gives a a budget of 30 requests per 3 s and b
    a pace of 20, and checks that b reaches a 600-entry bare run without
    a dropped request (12 s).
  - `TestUnpacedChaseHitsTheBudget` checks that, without pacing, the
    same chase has a request dropped.
  - `TestHeavierLedgerAlwaysReached/tightest` turns pacing off on b. There a
    serves without a budget and b's one-hash pages take thousands of
    requests, so pacing them would only test the pace.
- **Checks:** `go vet`, `go test -race ./...`, `TestHeavierLedgerHuge`,
  and `GOEXPERIMENT=simd` for the changed packages all pass.

## For Red, Grey and Black

These are findings, not part of this change:
- **A timeout still drops a chase's progress.** A peer with a budget
  set lower than 480, or a slow one, still ends a long chase as
  "nowhere" with nothing kept. Keeping progress after a timeout (a
  yield instead of "nowhere", a few times in a row) would close that.
  It changes how a stalling sender is treated, so it is for Red.
- **The pace is the asker's guess at the sender's budget.** 480 fits the
  default of 600. Raising the default request budget, or letting a peer
  say its budget in its hello, would let a chase go faster. Both are
  node policy.
- **Nothing here is consensus.** It is how fast a node asks, never what
  it accepts. It is a D115 matter, so Black should see it in Part D.

## Cost and risk

- About 90 lines with tests.
- One 60-bucket counter per peer, and a lock per request.
- A chase that needs more than 480 requests a minute from one peer now
  takes longer than before, but it finishes. Before, it never did.

## Prototype

On `claude/source-orange-r8`; see the commit "ORANGE-018".
