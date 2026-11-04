# ORANGE-035: Round 25 measured; releases, the waiting path, and a design for in-place limits

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `e93eb29` |
| Area | `pkg/series` (part chain, variable ids); a design for `pkg/rpc`, `pkg/daemon`, `pkg/p2p`, `pkg/peers`, `pkg/tracking`, `pkg/engine` |
| Effort | two changes and one test; one design for Black |
| Decided by | Grey; Black for the design |

## Summary

D166 lets a node follow a variable series. I measured three things:
- the release path;
- the waiting path;
- what phase 2 added to every fixed-series transaction.

I also wrote the design for applying limits in place, which D164 left for
Black to see (ORANGE-033's proposal 3).

**1. A far release was checked from scratch every time the series was
built. Fixed here.**

`nodepolicy.Accepted` builds each series fresh and adds its kept release,
so `CheckRelease` works out `part_id(k − 1)` from part 1 again. That runs
in four places:
- on every start;
- on every reload;
- twice on `poo series add-release`: the handler checks the kept release
  and the new one, then the node reloads;
- in `poo doctor` and the wallet.

| Kept release at part 10,000,000 | `e93eb29` | This change |
| --- | --- | --- |
| `Accepted` (start, reload, doctor) | 2.68–2.77 s each time | first build in a process 2.7 s, then 0.09–0.4 ms |
| `poo series add-release` of the next part: reply | 2.91 s | 3.7 ms |
| Then the node answering again (reload) | 5.62 s | 5.0 ms |
| Part 1,000,000 | 0.24–0.26 s each time | the same, once |

The time per part is linear, about 270 ns (ORANGE-034). At part 10⁸,
each of those builds took about 27 s.

The fix shares one part chain per series id across the process. A part
id is a function of the series id alone, which hashes the params, so every
`Series` built from the same params shares the ids already worked out.
Like `variableIDs`, the set holds only facts and only grows. Each chain
stays within `maxPartMarks` ids (2 MiB, RED-070).

Every release is still checked in full: its witness first, then its
previous id against the chain. Only the work of finding `part_id(k − 1)`
is shared, and its value is the same whoever finds it.

`TestPartChainShared` checks that a series built again shares the chain,
a series with other params does not, and a fixed series has none. It
also checks the ids against the chain from scratch. Blue's
`TestPartChainBoundedAndExact` and `TestPartIDFarOutInBoundedMemory` now
give their series a chain of their own, so they still start from nothing.

Still linear, by design, and not changed here:
- **The first start of a node process.** It is 2.7 s at part 10⁷. A
  node would need to trust its own node.json to skip it, and Black should
  decide that.
- **Each `poo` command that builds a claim.** It runs in a new process
  and checks the node's release again (`cli/node.go`, "checked here
  too"), at the same 2.7 s for part 10⁷. The GUI runs `cli.Main` in one
  process, so it pays only once.

  Proposal for Grey: a command could take the released count from its
  own node, which it already trusts for everything else. The node checks
  the claim in full anyway.

**2. Reading a transaction got 10% slower for every series. Fixed
here.**

`tx.DecodeFrom` now asks `series.IsVariableID` for every transaction it
reads, so it knows whether the mint carries a part. That was a
`sync.Map` load with a 32-byte key.

| `BenchmarkDecode` (6 runs) | `08404df` | `e93eb29` | This change |
| --- | --- | --- | --- |
| time | 330.7 ns ± 5% | 363.4 ns ± 7% (+9.9%, p=0.009) | 323.6 ns ± 8% (no change from `08404df`, p=0.39) |
| bytes | 432 | 448 | 448 (the mint's new `Part` field) |

The set is now an immutable map behind an atomic pointer. Adding an id
replaces the map under a mutex. A read is one atomic load, and nil while
no variable series is built, which is every fixed-only node.

The rest of the fixed path costs nothing measurable:

| Benchmark (6 runs each) | `08404df` | `e93eb29` |
| --- | --- | --- |
| `tx` Check, first time | 59.3 µs ± 20% | 62.2 µs ± 20% (no change, p=0.82) |
| `tx` Check, again | 1.02 µs ± 24% | 1.02 µs ± 5% |
| `engine` HandleTxParallel | 29.7 µs ± 17% | 31.8 µs ± 16% (no change, p=0.49) |
| allocations | the same in every row | |

A fixed transaction keeps its bytes and tid, which D166 tests.

**3. The waiting path is cheap. No change.**

- A transaction for an unreleased part stops in `Engine.KeyOf` at
  `CheckMint`: one comparison with an atomic count, before any hash or
  signature. It is never put on the seen list, so it is checked again
  each time it is sent.
- A peer pushing items for an unreleased part names a pool the node does
  not have (`poolFor` returns nil). The message is charged to the peer's
  byte budget as any pushed message is, then dropped, with no penalty.
  So the waiting path costs a node what any message for an unfollowed
  pool costs.

**4. Design for Black: applying limits and releases in place.**

Today `poo policy limits -set` and `poo series add-release` save
node.json and then reload the whole node. A reload replays the store,
saves and loads the book and the tracker, and rebuilds the p2p host, so
every peer disconnects. ORANGE-033 measured the cost:

| Node data | Each change |
| --- | --- |
| 100,000 stored entries | 5.4–6.3 s offline, 930 MiB allocated |
| Tracker of 131,072 addresses | 0.5 s offline |

The design applies the change to the running parts and keeps the reload
for anything they cannot take.

**The rule Black can check.** After an in-place change, every part of the
node must hold what a reload onto the same node.json would give it. Any
step that cannot promise that falls back to the reload. In-place is an
optimisation of the reload, never a different outcome. So it adds no
rule, and it changes nothing a node accepts beyond what the same limit
already changes after a reload.

**The order.**
1. Under `policyMu`, as today: load node.json, check the change, save it.
2. Ask each part to apply its own values, under its own lock, one part at
   a time.
3. If any part reports it cannot, reload as today. The file is already
   saved, so the reload gives the end state the rule asks for.
4. The reply says how the change was applied: `in_place` or `reload`.

**Each limit:**

| Setting | Read by | In place | Same as a reload because |
| --- | --- | --- | --- |
| `p2p.max_book`, `book_per_range`, `verified_per_range` | `peers.Book.lim` under the book lock | `Book.SetLimits(l)` sets `lim` and runs `Fit(now)` | The reload opens the book with the new limits and runs the same `Fit` on start (D160) |
| `p2p.retry_seconds`, `candidate_seconds` | `Book.lim` (`Retry`, `CandidateAge`) | the same `SetLimits` | They are read at each use; nothing is cached from them |
| `p2p.registry_seconds` | `Host.cfg.RegistryEvery` in the discovery loop | an atomic duration the loop reads each pass | The next pass uses it, as the first pass after a reload would. A reload also loads the registry at once, so the in-place path should do the same |
| `tracking.max_addresses`, `flag_history_seconds`, `log_seconds`, `log_size` | `Tracker.cfg.Retain` under the tracker lock | `Tracker.SetRetention(r)` sets it and trims once | A reload saves and loads the tracker, and `get` trims at the bound (ORANGE-031). Trimming once at once gives the same records. A lower `log_size` cuts each log at its next event, as after a reload |
| `seen_size` | `Engine.seen` | `Seen.Resize(n)` drops the oldest above n | A reload starts with an empty seen list, so this keeps more than a reload would. The seen list only skips work, never changes a result, but Black should confirm, or the design empties it to match |
| `max_pending` | each pool's `mempool` limit, set when the pool is made | `Engine.SetMaxPending(n)` sets it on every pool, and each pool drops above n by its existing order | A reload refills pending from what peers send again. The kept set differs only in what was pending, which is never part of any ledger |
| `ties` | each pool's `ledger.Config` | `Engine.SetTies(n)` on every pool | Black's call. Ties decide which equal-weight branches a node keeps, and a reload rebuilds them from the store. If they can differ, `ties` stays on the reload |

**Releases.** `Series.AddRelease` already works on a running engine:
`FollowedPools` grows at once (`TestVariableSeriesWaitsForRelease`). The
p2p host indexes its pools when it starts (`host.go`,
`EachFollowedPoolOf`, then `indexLocked(makePool(...))`). The in-place
path would index the new part's pools the same way, under the host's
lock, through a new `Host.AddPools`, and add any new coverage. Waiting
items are kept nowhere (D166), so nothing else needs to change: peers
send them again, and they are taken. A series followed by its index
keeps the pools it lists, as a reload would.

**Tests the design should ship with.** For each setting and for a
release, two nodes on the same store and node.json. One applies the
change in place; the other reloads. Compare what each part holds:
- the book's records;
- the tracker's addresses and logs;
- each pool's ledger, tip and branches;
- the followed and listed pools;
- the limits each part reports.

Do this at the floor and the ceiling of each setting, lowered and
raised. The same test also checks the fallback: a part that refuses
must lead to a reload with the same end state.

**Cost.** About 150 lines across five packages, with one setter each
under the lock the part already has. The reload stays, unchanged, for
anything else that changes in node.json.

No consensus change in the code here. Parts 1 and 2 give the same values
with less work. Part 4 is a design for Black and is not built.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **`Accepted` and `AddRelease`:** a test-only harness in
  `pkg/nodepolicy`. It made a policy with `VariableParams(8, 4, 4)`, its
  issuer witness and a kept release of part 10⁶ or 10⁷. It called
  `Accepted` three times, then `AddRelease` of the next part, on both
  commits.
- **The node:** a test-only harness in `pkg/daemon`. It ran `daemon.Run`
  with that policy and p2p off. It timed the start, then
  `Client.AddRelease` of the next part: the reply, and the next `Ready`.
  - On `e93eb29` the start and both reload steps each paid the chain.
  - With the change, the harness had signed the releases in the same
    process, so the start found the chain warm. A node's first start in
    a new process still pays it once (2.7 s at 10⁷), as the table says.
- **Fixed path:** `go test ./pkg/tx -bench .` and
  `./pkg/engine -bench HandleTxParallel`, 6 runs each, compared with
  benchstat. The baseline was a worktree at `08404df`, the main before
  phase 2.

The harnesses are not committed. The test is.

## Idea

- **`series.chainFor(id)`:** a process-wide `sync.Map` from series id to
  `*partChain`. `Series.parts` is now a pointer, nil for a fixed series.
- **`series.variableIDs`:** an `atomic.Pointer` to an immutable map,
  replaced under `variableMu` by `addVariableID`.
- **Tests:**
  - `TestPartChainShared` (`pkg/series`).
  - `variableForTest` gives each Blue test its own chain.

## Cost and risk

- About 60 lines and one test.
- The shared chains live as long as the process, like the set of
  variable ids. Each is bounded at 2 MiB, and a chain only exists for a
  variable series the process has built.
- A test that changes the chain's sizes must give its series its own
  chain, as `variableForTest` now does.
- The full suite passes with `go test -race ./...`, and so do the `gui`
  module's tests. They were run locally, since GitHub Actions is not
  running.

## Prototype

On `claude/source-orange-r25`; see the commit "ORANGE-035".
