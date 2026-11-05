# ORANGE-038: Round 28 measured; entry bodies were still held through their items

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `705639a` |
| Area | `pkg/ledger` (items, switching), `pkg/p2p` (chase walk); proposals for `pkg/p2p` serving and `entry_cache_mib` |
| Effort | two changes and two tests; three proposals |
| Decided by | Grey; Black in Part D |

## Summary

D172 drops each entry's body from memory once its log holds it, and reads
it back through a byte-bounded cache. I measured it against the build
before it (`adcf216`), with histories of 20,000 entries (80,000
transactions). The owners use Ed25519 or ML-DSA-65.

**1. D172 freed almost no memory, because each entry's items kept every
transaction, witnesses included. Fixed here.**

A pool's skeleton keeps each entry's `items`, the list of
`state.Item{TID, Tx}` it applies and reverts. Each `Tx` there is the same
pointer as in the body, so dropping the body freed only the entry's
header. The transactions and their witnesses, which are almost all of a
body's bytes, stayed in memory.

These are the heaps after a restart:

| owners | before D172 | D172, 1 MiB cache |
| --- | --- | --- |
| ML-DSA-65 | 508 MiB | 502 MiB |
| Ed25519 | 94 MiB | 91 MiB |

**The fix.** When a pool releases a body, it replaces each item's
transaction with a copy that has no witnesses (`stripWitnesses`). Nothing
that reads items after the entry was checked needs a witness:
- apply and revert;
- `ClaimEntry`;
- sorting a switch's displaced transactions.

**Switching.** `switchTo` sorts the transactions of a stripped entry with
`tx.Owners`, the same rules short of the witnesses. The answer is the
same as `CheckAgainst`'s. The witnesses satisfied these owners when the
entry was checked, and an input's owner is fixed by the transaction that
made it, so only the witness step can differ, and it cannot.

**Reading the body back.** A switch reads a stripped entry's body back
once, from the cache or the log, and reports every displaced transaction
whole, as before. Two things need the witnesses:
- a carried transaction goes back to the pending transactions;
- the watch hub finds a transaction's spenders by its witnesses, so
  `TestWatchAndViews` caught a first version that reported void
  transactions without them.

If the body cannot be read, the transactions are reported without
witnesses and none is carried. A transaction is never carried without
its witnesses, because `AddCarried` can insert one that waits on a pending
parent without checking it. That only leaves the transaction for a peer
to send again, which is a lack of information, not a verdict.

The fix brings the heap after a restart, with a 1 MiB cache, down to:
- ML-DSA-65: 502 → 67 MiB (6,582 → 881 B per transaction);
- Ed25519: 91 → 75 MiB (1,195 → 980 B per transaction).

**2. The chase walk read bodies to find parents. Fixed here.**

When a peer's hash chain meets a held branch off the selected ledger, the
chase walks back through held entries under the engine lock, and read each
body only for its parent. It now uses `ParentOf`, from the skeleton.

**3. A cold read is 7 to 29 µs, and serving reads under the engine lock.
Proposal 1.**

| | in memory or a hit | cold, Ed25519 | cold, ML-DSA-65 |
| --- | --- | --- | --- |
| one entry of 4 transactions | 0.4–0.5 µs | 6.8 µs | 27–29 µs |
| one reply of 1,024 entries, all cold | 0.5 ms | 7 ms | 29 ms |

Peers' entry requests (`replier.entries`, `byHash`, `parts`) read their
bodies inside `Engine.Ledger`, so the engine lock is held for the whole
reply. Before D172 that was half a millisecond. Now it is up to 29 ms per
reply with post-quantum owners, while several peers syncing from the
start can ask in turn.

**4. The cache's byte count is about twice the memory it uses. Proposal
2.**

`bodySize` counts twice an entry's byte form plus 256 bytes. Comparing
heaps at each cache size against 1 MiB, the memory a cached body actually
takes is its byte form plus about 1 KiB:

| owners | counted as | real |
| --- | --- | --- |
| Ed25519 | 2.7 KiB an entry | 2.1 KiB |
| ML-DSA-65 | 43.5 KiB an entry | 22.6 KiB |

So `entry_cache_mib` 512 holds about 280 MiB of ML-DSA bodies.

**5. Start-up memory no longer grows with the log; the time does not
change.**

| owners | build | restart | peak heap |
| --- | --- | --- | --- |
| ML-DSA-65 | before D172 | 7.4 s | 1,606 MiB |
| ML-DSA-65 | D172 | 7.3–8.3 s | 772–847 MiB |
| ML-DSA-65 | this fix, 1 MiB cache | 7.5 s | 160 MiB |
| Ed25519 | before D172 | 2.2 s | 167 MiB |
| Ed25519 | this fix, 1 MiB cache | 2.6 s | 115 MiB |

The time is re-checking every witness, as before.

**6. A deep switch costs the same or less.**

A switch to a heavier branch 1,001 entries long, made with ML-DSA owners,
took:
- 40.6 ms before D172;
- 41.9–43.4 ms with D172;
- 25–31 ms with this fix, including reading back the 1,000 displaced bodies.

Sorting by owners skips verifies that were cache hits anyway. With
Ed25519, a 101-entry switch took 0.8–1.3 ms in every build.

## Proposals

**Proposal 1: read bodies outside the engine lock when serving.** Under
the lock, take the hashes and any bodies held in memory. After it, read
the rest through the node's source and cache, which have their own locks.
A body that changed under the read is already refused by
`poolBodies.load`'s hash check. That brings a reply's lock hold back to
under a millisecond.

**Proposal 2: count bodies as they are, and set the default in real
memory.** Use `bodySize = Size() + 1 KiB`, so `entry_cache_mib` means MiB.
Set the default so the memory stays where it is now: **256 MiB** holds
about 11,600 entries of four ML-DSA-65 transactions, or about 120,000
entries of four Ed25519 ones. Keep the floor of 1 and the ceiling as they
are (jerry's settable-limits principle).

The hit rates below show what a cache buys. They are 20,000 reads, 80% of
them among the newest 10% of entries:

| owners | 1 MiB | 16 MiB | 64 MiB | 512 MiB (as counted now) |
| --- | --- | --- | --- | --- |
| Ed25519 | 14% (6.7 µs) | 87% (1.6 µs) | 100% (0.4 µs) | 100% (0.4 µs) |
| ML-DSA-65 | 0.7% (30 µs) | 13% (24 µs) | 49% (15 µs) | 92% (2.9 µs) |

A node short of memory loses only serving speed, never a verdict, so a
1 MiB cache is a fair choice there.

**Proposal 3: the skeleton is now the cost that grows.** After this fix, a
transaction costs 880 to 980 bytes of memory: items, undo, the tid index
and state maps. That is about 0.9 GiB per million transactions, or about 7 GiB a year at
15 transactions a minute.
D172's R29 step (items and undo outside a recent window) is the next
saving, and is worth more than any cache setting.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), and runs Go
1.27.3. The harness was a test file outside the commit. It was built
identically on main (`705639a`, with and without this fix) and on
`adcf216` in a worktree.

- **History:** the example series with Ed25519 and ML-DSA-65 allowed,
  following pool 1. There are four claims, then 20,000 entries, each
  spending each of the four mints once. Each entry gets a layer of 16
  tries so it is selected.
- **Heap:** `HeapInuse` after two GCs, before and after; "after a restart"
  is after closing and reopening on the same directory. Peak heap is
  sampled every 2 ms during `node.Open`.
- **Reads:** `Engine().Ledger(key, func(p) { p.Entry(h) })`, timed around
  the call, so the time includes taking the engine lock. Sequential is
  every chain entry in order after a restart. Random is 20,000 reads, 80%
  among the newest 10%. Hits and loads are from `node.Bodies()`.
- **Deep switch:** a branch from 1,000 entries below the tip, of 1,001
  empty entries, each with a layer of 64 tries, so it is heavier. The
  slowest call, the one that switches, is reported. With Ed25519 the
  1,000-deep branch did not overtake the main chain in any build, so only
  the 101-entry switch is compared there.

These are the heaps after a restart, in MiB, with bytes per transaction in
brackets:

| owners | before D172 | D172, 512 MiB | D172, 1 MiB | fix, 1 MiB | fix, 16 MiB | fix, 64 MiB | fix, 512 MiB |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Ed25519 | 93.7 (1,228) | 98.0 | 91.1 (1,195) | 74.8 (980) | 87.6 | 106.9 | 105.9 |
| ML-DSA-65 | 507.9 (6,658) | 506.2 | 502.2 (6,582) | 67.2 (881) | 76.1 | 101.8 | 346.6 |

## What changed

- `pkg/ledger`:
  - `releaseBodies` strips the witnesses from a released entry's items
    (`stripWitnesses`, `stripped`).
  - `switchTo` sorts a stripped entry's displaced transactions by
    `tx.Owners`, and reads its body back once to report them whole. If
    the body cannot be read, none of that entry's transactions is
    carried.
- `pkg/p2p`: the chase's walk back takes parents from `ParentOf`.
- Tests:
  - `TestSwitchCarriesFromReleasedBodies`, which covers carrying with the
    body readable and with every read failing.
  - `runRandom`'s twin pool now also compares every switch's unchanged,
    carried and void transactions with the pool's, byte for byte,
    witnesses included. I checked both checks
    fail when the body read or the owners rule is taken out.

## Consensus impact

None.
- No verdict on an entry or a ledger changes. Entries are checked with
  their witnesses as before, while the body is in memory.
- Switching applies and reverts the same items.
- A displaced transaction is sorted as before, by rules whose witness step
  cannot change the answer.
- A body that cannot be read only leaves its transactions out of the
  pending transactions, which is a lack of information.
- No depth limit is added.
