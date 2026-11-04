# ORANGE-033: Round 23 measured; rival-claim builds and live limit changes

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `215d428` |
| Area | `pkg/ledger` (`CheckOn`); proposals for `pkg/rpc`/`pkg/daemon` (limit changes) and `pkg/series` (verified list) |
| Effort | one change and one test; two proposals not built |
| Decided by | Grey |

## Summary

D161 lets an owner build on a chosen parent (`poo build -parent`) and
set limits on a running node (`poo policy limits -set`). I measured both.

**1. A build on a chosen parent checked pending payments pairwise,
under the engine lock. Fixed here.**

With `-parent` and no `-tx` list, `BuildEntryOn` hands every ready
pending transaction to `ledger.Pool.CheckOn`, holding the engine lock.
`CheckOn` checked each transaction for a conflict against every one it
had already kept, input by input. That is quadratic in the pending
count. `max_pending` defaults to 10,000 and can be set to 1,048,576
(D160).

| Independent pending spends | `215d428` | This change |
| --- | --- | --- |
| 4,096 (benchmark) | 154–178 ms | 13–15 ms |
| 10,000 (default `max_pending`), signatures already checked | **1.10 s** | **35 ms** |
| 16,000, signatures already checked | 3.39 s | 63 ms |
| 64,000, signatures not cached | **85 s** | **3.8 s** |

Every peer message, every interface call and every other build waits
on the engine lock for that time. At the `max_pending` ceiling the old
check would have held it for hours.

The fix keeps two sets while `CheckOn` runs: the mints claimed and the
inputs spent by the transactions kept so far. Each new transaction is
checked by looking up its own inputs. The result is the same: the same
transactions are kept and refused, in the same order, with the same
errors. `TestCheckOnConflicts` checks a double spend, a spend that
shares one of two inputs, a double claim and the independent ones, and
that what is kept builds as one entry.

**2. Building on a deep parent costs what `AddEntry` there costs. No
change.** `CheckOn` reverts the selected chain down to the parent, as
`AddEntry` on that parent would. `ClaimEntry` (behind `-parent before-claim`) scans
the chain for the claim.

| Selected chain | `CheckOn` at tip | `CheckOn` at the first entry | `CheckOn` at `pool_id` | `ClaimEntry`, claim at the tip |
| --- | --- | --- | --- | --- |
| 1,000 entries | 57 µs | 0.24 ms | 0.51 ms | 1.5 µs |
| 10,000 entries | 97 µs | 2.6 ms | 2.3 ms | 112 µs |

Both grow linearly and stay in milliseconds. No change is proposed.

**3. Each `limits -set` restarts the node. Proposal, not built.**

`POST /v1/policy/limits` saves `node.json` in a few milliseconds and
then asks for a reload. A reload closes the node and opens it again:
the store is replayed, the peer book and tracker are saved and loaded,
and the p2p host is rebuilt, so every peer connection drops and is
dialled again.

| Node data | Reply | Node answering again | Allocated per change |
| --- | --- | --- | --- |
| Empty | 2–6 ms | 4–16 ms | under 1 MiB |
| Peer book of 65,536 | 2–3 ms | 43–118 ms | 15 MiB |
| Tracker of 131,072 addresses | 3–4 ms | 0.49–0.56 s | 130 MiB |
| Store of 20,000 entries | 2–4 ms | 0.62–0.67 s | 200 MiB |
| Store of 100,000 entries | 2–7 ms | **5.4–6.3 s** | **930 MiB** |

The time grows with the store: about 60 µs per stored entry. A node
with a long ledger is off the network for seconds for each limit
changed, and peers see it disconnect. Settings import and policy
import reload the same way, but they are rare. `limits -set` is meant
for tuning a running node.

Proposal for Grey: apply a limit change in place when the running parts
can take it, and reload only for the rest.
- The book could take new `Limits` and run `Fit`, as the reload does
  now.
- The tracker could take a new `Config`; its trim already runs on the
  next add.
- The engine could take new `seen_size`, `max_pending` and `ties`
  bounds and trim to them.
- The p2p timings could be read from the host's config on each use.

Each part would need a setter under its own lock. That is a design
change across four packages, so I have not built it. Black should
confirm it changes no consensus rule; I expect it does not, since each
limit already applies after a reload.

**4. The verified-signature list is smaller than the pending ceiling.
Proposal, not built.** A node remembers 16,384 checked witnesses
(`policy.DefaultVerifiedSize`). With more ready pending transactions
than that, a build on a chosen parent verifies their signatures again,
under the engine lock: the 64,000 row above is 3.8 s either way. A
list sized to `max_pending` would avoid it, at about 100 bytes a
witness: 1 MiB at the default, about 100 MiB at the ceiling. Grey may
prefer to leave it, since only owners who raise `max_pending` past
16,384 and build with `-parent` reach it.

No consensus change. The fix only changes how `CheckOn` remembers what
it has kept.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **Conflicts:** a test-only harness in `pkg/ledger`.
  - A series with a supply of n paid one claim of n outputs to n owners.
  - It built n spends, one per output, and timed `CheckOn` on the tip
    twice. The first run verifies signatures; the second finds them in
    the verified list when n is at most 16,384.
  - n was 1,000 to 64,000, on `215d428` and this change. A CPU profile
    at 8,000 put `conflicts` and signature checks at the top of
    `CheckOn`.
- **Benchmark:** `BenchmarkCheckOnMany` (4,096 spends), three runs each
  side. A long harness ran on the same machine at the time, so the
  absolute numbers are a little high on both sides.
- **Depth:** the same harness built a selected chain of 1,000 or 10,000
  one-spend entries with a layer each, then a claim of mint 5 at the
  tip. It timed `CheckOn` of a rival claim of mint 4 on the tip, the
  first entry and `pool_id`, and `ClaimEntry(5)` averaged over 100.
- **Limits:** a test-only harness in `pkg/daemon` ran a node with
  `daemon.Run` and p2p off.
  - Before start it stored 20,000 or 100,000 entries with a layer each,
    or filled a peer book of 65,536, or a tracker of 131,072 addresses,
    with the limits raised to fit.
  - It then called `SetLimits` three times (`tracking.log_size`), timing
    the reply and the next `Ready`, then dialled the interface and
    called `Status`.
  - Allocation is the growth of `runtime.MemStats.TotalAlloc` across
    one change.
  - The bare `poo` binary gave the same 20–25 ms for an empty node.

The harnesses are not committed. The test and benchmark are.

## Idea

- **`ledger.Pool.CheckOn`:** a `taken` value with a set of claimed mints
  and a set of spent inputs replaces the list of kept items and the
  pairwise `conflicts`.
- **Tests:** `TestCheckOnConflicts` and `BenchmarkCheckOnMany`
  (`pkg/ledger/orange033_test.go`).

## Cost and risk

- About 40 lines and one test. Memory is two maps the size of the kept
  set, freed when `CheckOn` returns.
- A transaction that spends the same input twice is refused by
  `CheckStateless` before the sets are consulted, as before.
- The full suite passes with `go test -race ./...` (run locally, since
  GitHub Actions is not running).

## Prototype

On `claude/source-orange-r23`; see the commit "ORANGE-033".
