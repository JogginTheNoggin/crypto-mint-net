# ORANGE-039: Round 29 measured; cold entries, the recent window and the pending pool after a deep switch

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `d1a9128` |
| Area | `pkg/ledger` (D175 cold entries), `pkg/engine` (D174 serving), `pkg/mempool`; node policy `recent_window` |
| Effort | one small change and one test; three proposals |
| Decided by | Grey; Black in Part D |

## Summary

D174 reads served bodies outside the engine lock. D175 drops the items
and undo of entries more than `recent_window` below the tip (default
10,000), and recomputes the undo from the tids index when it is needed.
I measured both against the build before them (`f29420b`, D173 with
ORANGE-038). The histories have 30,000 entries (120,000 transactions).
Their owners use Ed25519 or ML-DSA-65.

**1. A cold entry costs about 40% less memory per transaction. Most of
what is left is the skeleton.**

These are the heaps after a restart, in bytes per transaction:

| owners | before | window = all | window 10,000 (2/3 cold) | window 1,000 (97% cold) |
| --- | --- | --- | --- | --- |
| Ed25519 | 989 | 1,001 | 767 | 615 |
| ML-DSA-65 | 920 | (not run) | 685 | 577 |

From these, a cold transaction costs about 600 bytes (565 with ML-DSA)
and a warm one about 1,000 (920). So the cold part grows by about
0.6 GB per million transactions, down from about 1 GB. The window itself
costs a fixed amount: 10,000 entries of four transactions take about
16 MiB more than 1,000 entries do.

**2. Serving a cold reply now holds the engine lock for 0.2 to 0.3 ms.**

A reply of 1,024 cold entries, with a 1 MiB cache:

| owners | lock held before | lock held now | whole reply now |
| --- | --- | --- | --- |
| Ed25519 | 6.4–6.6 ms | 0.14–0.24 ms | 6.1–6.4 ms |
| ML-DSA-65 | 24–27 ms | 0.18–0.33 ms | 24–30 ms |

The reply takes as long as before, but the engine is free while it reads.

**3. Start-up is as fast as before, with a lower peak.**

| owners | build | restart | peak heap |
| --- | --- | --- | --- |
| Ed25519 | before | 3.3 s | 184 MiB |
| Ed25519 | window 10,000 | 3.3–3.6 s | 128–130 MiB |
| Ed25519 | window 1,000 | 3.2–3.4 s | 115–121 MiB |
| ML-DSA-65 | before | 10.0 s | 239 MiB |
| ML-DSA-65 | window 10,000 | 10.1 s | 177 MiB |
| ML-DSA-65 | window 1,000 | 10.2 s | 153 MiB |

The time is still re-checking every witness.

**4. A switch past the window costs about 16 µs more for each cold entry
it reverts.**

These are the calls that switch, Ed25519, for a heavier branch from D
entries below the tip:

| D | before | window = all | window 10,000 | window 1,000 |
| --- | --- | --- | --- | --- |
| 1,000 | 14.2 ms | 13.8 ms | 15.7 ms | 13.4–14.4 ms |
| 5,000 | 60 ms | 62 ms | 67–73 ms | 126–128 ms |

At D = 5,000 with a window of 1,000, the switch warms 4,000 cold entries
and takes about 65 ms more. That is about 16 µs an entry with Ed25519,
reading bodies from the log through a 1 MiB cache. With ML-DSA-65 a cold
read is about 28 µs (section 2), so it should be about twice that. I
estimate a switch 100,000 deep at about 1.6 s more with Ed25519 and
about 3 s with ML-DSA-65. I could not measure it, for the reason in
section 5. Nothing is refused for depth; a deep switch is only slower.

With ML-DSA-65 the 1,000-deep switch took 33–36 ms in every build. The
5,000-deep branch did not overtake the main chain there, so it is left
out.

**5. After a deep switch, each new tip re-sorted the whole pending pool.
Fixed here.**

The 120,000-entry run did not finish in an hour. Building the rival
branch got slower the longer it was:

| D (window = all, 12,000 entries) | 500 | 1,000 | 2,000 | 4,000 |
| --- | --- | --- | --- | --- |
| whole branch | 0.28 s | 0.54 s | 0.76 s | 7.0 s |

At D = 6,000, 79% of the CPU was in `mempool.Revalidate`, through
`engine.afterSwitch`, and 43% of that was `sortBySeq`.

- A switch carries every reverted transaction back into the pending
  pool. `AddCarried` does not apply `max_pending`, so after a switch
  5,000 deep the pool holds about 20,000 transactions.
- Each later entry on the new tip is also a switch (an extension), and
  `afterSwitch` calls `Revalidate`. It collected the whole pool from its
  map and sorted it by arrival every time, all under the engine lock.

The fix keeps the pool in arrival order as it goes. Arrival numbers only
grow, so a new transaction is appended. A removed one is marked and
dropped at the next pass. `Revalidate` and `Ready` read that order
instead of sorting. The order they see is exactly the one the sort gave.

| Ed25519, 30,000 entries | before the fix | with it |
| --- | --- | --- |
| 1,000-deep branch, whole | 0.96–1.02 s | 0.51 s |
| 5,000-deep branch, whole | 24–25 s | 10–11 s |
| the switch call itself | unchanged | unchanged |

What is left is `Revalidate`'s walk over the pool for each new tip
(Proposal 2).

## Proposals

**Proposal 1: keep `recent_window` at 10,000.**

- The window costs a fixed amount (about 16 MiB for four-transaction
  entries). The memory that grows with the history is the cold part,
  which no window setting changes.
- A smaller window saves little and makes every switch deeper than it
  read the disk: at 1,000, a 5,000-deep switch took twice as long.
- 10,000 keeps every switch up to 10,000 deep in memory.

A node very short of memory can set 1,000 and lose only switch speed,
never a verdict.

**Proposal 2: re-check only what a new tip changed.** On an extension,
`Revalidate` still visits every pending transaction. It could visit
only:
- the ones the new entries include;
- the ones spending the same inputs or mints;
- the ones waiting on a parent the entries include.

The pool already indexes these (`spentBy`, `claims`). That makes a new
tip cost the size of the entry, not the pool. It is a larger change to
`pkg/mempool`, so I leave it to Grey. Separately, Grey may want carried
transactions to count against `max_pending` once the switch is done,
oldest kept, so that a deep switch cannot grow the pool without bound.
Dropping a carried transaction is only a lack of information.

**Proposal 3: the cold skeleton is the next saving.** A cold transaction
still costs about 600 bytes:
- the tids index;
- each entry's skeleton (hash, parent, weights, layers, the mints it
  claims);
- the state maps.

That is about 4.5 GiB a year at 15 transactions a minute. Moving the
tids index to disk (by height, through the pool log) would be the next
step. I can profile the skeleton by field next round if Grey wants it.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512) and runs Go
1.27.3. The harness was a test file outside the commit. It was built the
same way on this branch's base (`d1a9128`), on `f29420b` in a worktree,
and on the base with this fix.

- **History:** the example series with Ed25519 and ML-DSA-65 allowed,
  following pool 1. There are four claims, then 30,000 entries, each
  spending each of the four mints once. Each entry gets a layer of 16
  tries, so it is selected. The entry cache is 1 MiB in every run.
- **Heap:** `HeapInuse` after two GCs. "After a restart" is after
  closing and reopening on the same directory. The peak heap is sampled
  every 2 ms during `node.Open`. Bytes per transaction divide by
  120,000.
- **Replies:** 1,024 entries from height 1 and from height 15,001. The
  new build takes `Pool.Ref` for each hash inside `Engine.Ledger` and
  calls `Get` after it, as `replier` does since D174. The old build
  calls `Pool.Entry` inside the lock. "Lock held" is the `Ledger` call.
- **Deep switch:** a branch from D entries below the tip, of D+1 empty
  entries, each handed in and given a layer of 256 tries, so it becomes
  heavier. The slowest single call, the one that switches, is reported,
  and so is the time for the whole branch.
- **Profile:** CPU profile of the 6,000-deep branch on a 12,000-entry
  history, window = all.
- The one ML-DSA-65 run with a window of all was not made, to save time.
  The 100,000-deep switch was not made because of section 5.

## What changed

- `pkg/mempool`: the pool keeps its transactions in arrival order
  (`order`, `pending.gone`, `oldestFirst`). `Revalidate` and `Ready` read
  it instead of sorting (`sortBySeq` is gone).
- Test: `TestOrderKeptWithoutSorting`, 2,000 random adds and removes,
  with `Ready` checked against a list model after each and `Revalidate`
  every seventh step.

## Consensus impact

None.
- No verdict on an entry or a ledger changes.
- The pending pool sees its transactions in the same order as before,
  so first come, first served is unchanged.
- No depth limit is added. The `recent_window` proposal keeps the
  default.
