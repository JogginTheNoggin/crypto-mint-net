# RED-012: A branch of entries with no layers costs time quadratic in its length, and the entries a chase fetches are not charged for it

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 9972137 (code 84d3e69, Source Round 6 Part B) |
| Component | pkg/ledger/pool.go (Pool.viewAfter, AddEntry), pkg/p2p/chase.go (fetchAll, takeItem), pkg/p2p/forward.go (handlePushed), pkg/engine/engine.go (EntryCost) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ 80ea33e (BLUE-007) |

## Summary

BLUE-006 (D111) prices the revert that `viewAfter` needs to check an entry
on an old parent, and charges it to the sender of a **pushed** entry. It does
not price the other half of `viewAfter`: for a parent that is **off** the
selected chain it re-applies the whole off-chain path from the fork point.

```go
// pkg/ledger/pool.go, viewAfter
for x != nil && !x.onChain { path = append(path, x); x = x.parent }
...
for i := len(path) - 1; i >= 0; i-- { v.Apply(path[i].items) }
```

So the n-th entry of one off-chain branch costs O(n), and a branch of L
entries costs O(L^2) in total. Entries stay off-chain as long as the branch
is not heavier than the selected chain, and an entry has weight only through
layers (or `EntryHashCredit`), so a branch of entries **without layers**
never gets selected, never switches, and is never pruned while it is the
newest leaf (`prune` keeps the newest entry). With the default
`MinEntryStrength` of 0 such entries need no work. They can be empty, and
they are valid, so nobody is flagged for them.

`viewCost` and `EntryCost` count that path (`cost++` per off-chain step), so
a **pushed** branch is charged and its length is limited by the 200,000 a
minute revert budget. The entries a node asks for itself are not:

- `takeItem` (the chase) calls `HandleEntryFrom(..., engine.Requested, ...)`.
  `handlePushed` is the only caller of `EntryCost`/`Spend`.
- `fetchAll` (a node that holds nothing of the pool, `height == 0`, or a
  priority peer with `FastCatchUp`) asks for the sender's **whole ledger**
  and takes it in as it arrives, with **no work check**. `fetchBranch` has
  `checkWork` first (the provisional layers must beat the node's weight), but
  `fetchAll` goes straight to the entries: `runChase` ends its first step
  with `return h.fetchAll(pi, p, before)` when the node holds nothing, and
  the only gate in front is the optional confirmation by one priority peer
  (`NoConfirm` false, `askPeerIn(..., CatPriority, p)`). A node without
  another priority peer, such as a new one that has only met its seeds,
  skips it.
- The sender's reason to be chased is its **stated** weight, which nothing
  verifies (RED-003, won't fix): any weight above the node's own (zero for a
  node that holds nothing) starts the chase.
- The reply is bounded only by `maxRounds` (4,096 requests) times
  `EntryLimit` (512 entries per request): about two million entries.

## Reproduction

Measured on `claude/m10-watch` @ 9972137, 4 vCPU, Go 1.27.3. A temporary
test in `pkg/ledger` (not committed): a pool with `K = DefaultK`, `Ties = 2`
and no selected chain; then 32,000 times `entry.Build(parent, nil)` (an empty
entry on the previous one) and `AddEntry`, no layers:

```
branch length   last AddEntry   total (all AddEntry calls)
   100             2.8 us          0.46 ms
 1,000            26 us           24 ms
 4,000           109 us          371 ms
16,000           2.5 ms           7.3 s
32,000           1.6 ms          32.5 s      (linear would be about 0.1 s)
```

Every call was accepted, none earned an offence, and the selected height
stayed 0. The total grows with the square of the length (a 4x longer branch
costs about 20x), so 100,000 entries cost about five minutes of CPU, almost
all inside `AddEntry` under the engine lock, which the chase takes and
releases once per entry; the rest of the node (RPC, other peers, the
chase queue) runs at the share of the lock it can win. The entries are also
appended to the store, so the log grows with them.

I did not drive the chase end to end; the path above is from the code. The
reach is: a stated weight above the node's own, then `runChase` with
`height == 0` (or FastCatchUp and a priority peer), then `fetchAll`.

## Impact

Medium for a node that holds nothing of a pool, which is a node that is new,
has just followed a pool, or lost its store, and for a node that fast-catches
up from a priority peer. A remote peer, as the first it meets, can keep that
node busy for as long as it keeps serving zero-weight entries, at no cost to
itself beyond the bytes, without being flagged (a chase that does not
switch is a `Nowhere` outcome, not an offence). For a node that already holds
a ledger the chase needs real work first, so the reach is much narrower, and
a pushed branch is bounded by BLUE-006's budget. Availability only.

A related note on BLUE-006: its unit is **entries**, but the real cost of a
revert depends on the transactions in them. In my measurement a revert of
entries with one transaction each cost about 1.5 us per entry and of empty
entries about 33 ns, a factor of 45, and an entry may hold many transactions.
The 200,000 a minute budget therefore allows about 0.3 s of lock a minute per
address on a chain of one-transaction entries but several seconds a minute
on a chain whose entries hold 50 or more. Counting the reverted and applied
items (`len(items)`), or weighting an entry by its transactions, would make
the budget mean the same on every chain.

## Recommendation

- **Remove the quadratic.** `AddEntry` has just built and applied the view
  after the new entry (`view.Apply(stItems)`). Keep that scratch state for
  the next call, keyed by the new node's hash, and drop it when the selected
  chain or the branch changes; the next entry on the same parent, which is
  every entry of a branch fetched in order, then starts from it and costs
  O(its own transactions). Mind the memory (one overlay), and discard it on
  any pool change.
- **Gate `fetchAll` on work.** Before taking in a whole ledger from a sender
  that is not a priority peer, run the provisional layer check
  (`checkWork`) on the first entries, or stop the fetch as `Nowhere` when a
  run of N entries brings no layer weight at all (an honest ledger has
  layers on all but its newest entries).
- **Charge requested entries too.** Draw `EntryCost` of an entry taken in
  by a chase from the sender's `revert` budget (the sender is `p.id`/`p.key`
  in `takeItem`), and end the fetch when it is out.
- **Price by items, not entries.** Let `viewCost` count the transactions
  reverted and applied (or entries plus transactions) so the budget is the
  same on a quiet and a busy chain.

Tests: in `pkg/ledger`, a benchmark of `AddEntry` along an off-chain branch
of 1,000 / 4,000 / 16,000 empty entries (the cost per entry must not grow with
the length); in `pkg/p2p`, a peer that serves an endless branch of
zero-weight entries to a node that holds nothing is cut off after a bounded
number of entries, with a count of the revert budget spent; in `pkg/ledger`,
`ViewCost` is larger for entries with more transactions.

## Resolution

Fixed by BLUE-007 (decision D114), taking the first, third and fourth
recommendations, and a weight-free form of the second. `AddEntry` keeps
the state after the entry it added (unless that entry switched the
ledger), so the next entry on it starts there. Measuring showed two more
walks of the branch on every `AddEntry`: `weightOf` walked up to the anchor
and `prune` scanned every off-chain node for leaves. Off-chain nodes now
keep their anchor height and the pool keeps a leaf set. A 64,000-entry
branch with no layers costs 1.5 us per entry, the same as at 16,000. View
cost now counts transactions as well as entries (a prefix sum along the
selected chain keeps it O(1)), and `MaxPreviewCost` is 256 units. Entries a
chase takes in spend the sender's `revert` budget like pushed ones; taken
in order they cost only the first revert, so an honest catch-up or switch
is unaffected. Both fetches end at more than `MaxBareRun` (4,096) entries
in a row without layers. Tests: `TestEntryOwnersRefusesDeepParent`,
`TestKeptViewIsTheParentsState`, `TestLongBranchIsLinear` and
`BenchmarkOffChainBranch` (`pkg/ledger`), and `TestBareRunEndsFetch`,
`TestChaseInOrderCostsLittleRevert` (passes at a budget of 20, fails at 5)
and `TestDeepForkSpendsRevertBudget` (`pkg/p2p`).

Open for Red: `AddLayer` on an off-chain entry still walks its subtree to
update relative weights, so a layer on the root of a long branch costs
O(branch). Layers have their own budget (6,000 a minute) and K bounds the
branches kept, but this is the same shape and may be worth a look.
