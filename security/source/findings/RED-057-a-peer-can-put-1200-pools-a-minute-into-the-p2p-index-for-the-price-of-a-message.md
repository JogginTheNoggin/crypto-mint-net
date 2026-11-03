# RED-057: A peer can put 1200 pools a minute into the p2p index for the price of a message

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 20 Part B, on `main` @ `522e891` (BLUE-020, D154, a residue of RED-049) |
| Area | `pkg/p2p/chase.go` (`offer`), `pkg/p2p/forward.go` (`handlePushed`, `onKept`), `pkg/p2p/host.go` (`index`, `sortedPools`, `resolveBig`) |
| For Black | No consensus rule is touched. Dropping an unused index record limits no ledger and no follow. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-021) |

## What is wrong

D154 indexes a pool of a large series followed whole only "when it is in
use": when an item is kept there or a branch is chased there. The second
case is decided by a message, not by work:

1. `handlePushed` finds the pool of an entry with `resolveBig`, which
   only decodes the entry and reads its first transaction's mint
   serial.
2. `HandleEntryFrom` returns `ErrUnknownParent` for an entry whose
   parent the node lacks. That happens before any work is checked.
3. `offer(pi, p, m.Stated, &eh)` runs, and for a big series it calls
   `h.index(pi)` first: "a branch to chase puts the pool in use". The
   `Stated` weight is the sender's own claim. Only then does the chase
   budget (`StartChase`) look at the sender.

So an entry that builds on nothing, from any peer, with a stated weight
above the node's own, adds a permanent record (`byPoolID`, `byKey`, and a
new `sorted` list on the next use) for the pool it names. There is no
check that a claim is real before it is indexed, and nothing removes a
record whose pool has no state and no peer.

The sender's entry budget (1200 a minute, `KindEntry`) is the only
limit; it is per address and refills every minute.

## How I checked it

A node following a series of 2^40 pools whole (`PoolSize 1`), and a raw
peer on loopback that sends entries with a random parent, one claim in
a different pool each, and a stated weight of 2^200. The count of indexed
pools (`Host.IndexedPools`) every 30 seconds, for three minutes:

| after | messages sent | pools indexed | peer still connected |
| --- | --- | --- | --- |
| 1 s | 1,500 | 1,200 | yes |
| 31 s | 69,000 | 1,200 | yes |
| 1 min | 135,000 | 2,400 | yes |
| 2 min | 268,500 | 3,600 | yes |
| 3 min | 403,000 | 4,800 | yes |

That is 1200 new pools a minute for one address, with nothing flagged
that ended the connection. A million pools take 14 hours from one
address, and addresses add up. RED-049 measured 2.3 s and 376 MiB for a
million pools in `p2p.New`; the same size here costs about 3 KiB a pool in
my run (heap before and after, so it includes what the test itself
holds), and `sortedPools` sorts all of them again every time a new peer
is verified and every check tick after a pool is added.

The entry needs no work, no key and no valid parent. It needs a
decodable claim whose serial falls in the pool, which the sender
chooses.

PoC: `scratchpad/red_r20b_p2p_test.go.txt` (not committed).

## What could happen

- A node that follows a big series whole (what a "follow everything" user
  sets) grows by a record per pool an attacker names, forever, and each
  verified peer and check pays for sorting the list. It is the cost
  RED-049 removed from `p2p.New`, back by another door.
- The same message path reaches `offer` for a stated weight that no one
  checked: the chase queue, `tr.Stated` and the budget all work on a pool
  the attacker chose.

## Suggested fix (Blue's call)

- Index a pool when something in it is kept, or when a chase has
  fetched something the node has verified (a first entry whose work
  passes), not when a stated weight is offered. Until then keep the
  record on the chase job only.
- Or let an index record leave again when the pool has no state, no
  chase and no covering peer (a plain expiry, not a cap on what can
  be followed), and do not rebuild `sorted` for every addition.
- Charge the chase budget before the index is changed.

## Not tested

More than one address (the tracker counts loopback as one in this test);
what a real attacker with many addresses does is a multiple of the figure
above. Memory after the records stay for days.

## Resolution

Fixed (D156), as Red's first suggestion.

- `offer` no longer indexes a pool of a large whole series. The pool's
  record is held in `Host.chasing` (by pool id) for the chase alone.
  `runJob` finds it there through `poolByName`.
- The index gets the pool only when something is kept in it (`onKept`,
  which also clears it from `chasing`).
- `sweepChasing` runs on every check tick. It drops a record once the
  chase queue holds no group for its pool (`chase.Queue.Pools`, new).
  So the records never outnumber the queue's groups, and the queue's own
  limits bound them.
- This is no cap on what can be followed: a pool offered again gets its
  record again, and a chase that finds a heavier ledger indexes it.
  `sorted` is not rebuilt for a chase record.
- Test: `TestStatedWeightIndexesNoPool` (pkg/p2p). Twenty entries with no
  parent, each in another pool of a 2^40-pool series, with a stated
  weight of 2^62, index no pool. Their chase records go once the sender
  leaves.
- Not changed: `tr.Stated` and the chase queue still see the claim, as
  they did for every pool before D154. The chase budget (`StartChase`)
  and the queue's per-pool group limits are the same.
