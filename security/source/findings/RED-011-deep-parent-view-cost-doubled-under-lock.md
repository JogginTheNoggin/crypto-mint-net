# RED-011: An entry on a deep parent costs a revert of the whole selected chain, now twice, under the engine lock

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 29b08ce (code a92c2ca, Source Round 5 Part B) |
| Component | pkg/ledger/pool.go (Pool.viewAfter, AddEntry, EntryOwners), pkg/engine/engine.go (PreverifyEntry, HandleEntry), pkg/p2p/forward.go (handlePushed) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ 38eb846 (BLUE-006) |

## Summary

To check an entry against "the state after its parent", `Pool.viewAfter`
starts from the selected tip and **reverts every selected entry above the
fork point**, then applies the off-chain path:

```go
// pkg/ledger/pool.go, viewAfter
for h := len(p.chain); h > anchor; h-- {
    c := p.chain[h-1]
    v.Revert(c.items, c.undo)
}
```

For an entry whose parent is the pool id, or any entry deep in the selected
chain, `anchor` is 0 or small, so the work is proportional to the length of
the **whole selected chain** (and to the number of transactions in it), not
to anything the sender paid for. `AddEntry` already did this once, under
`e.mu`, for every entry whose shape and parent pass. BLUE-005 (D107) added a
second run in `PreverifyEntry`: `ledger.EntryOwners` also calls
`viewAfter(parent)`, and it too runs under `e.mu`, before the signatures. So
one pushed entry now costs two reverts of the chain with the engine lock
held, and the lock serves every other peer, the RPC and the chase.

Nothing in front of the revert makes an attacker pay:

- `entry.CheckShape` needs `Strength() >= MinEntryStrength`, which is 0 by
  default (`poo series new`), so there is no work gate.
- The parent test is `parentOf`: it holds for the pool id and for any held
  entry. A fork at the pool id is the most expensive and is always held.
- The stateless transaction checks and `CheckShape` are cheap and need no
  signature (RED-009).
- Only after the revert does the state decide the entry. If it fails, the
  sender earns one `InvalidItem` offence (5 points; a timeout at 10, a block
  at 30), so about six such entries per address. If it **passes**, there is
  no offence and the sender is credited: a transaction that is valid at the
  fork point (for example a claim of a mint that is unclaimed there) makes a
  valid entry, kept as an off-chain branch (K bounds how many). Each
  variant (another output, another amount) is a new entry hash, so none is
  "known". The per-address budget is 1,200 entries a minute (20 a second).
  I measured the invalid case; the revert runs before the verdict, so the
  valid case costs the same up to the `Apply` of one entry.

This is not new in `AddEntry` ("no depth limit on switching" is a design
statement). What changed is the factor of two, and that the check moved in
front of an existing, deliberately honest case (a fork at an old parent).

## Reproduction

Measured on `claude/m10-watch` @ 29b08ce, 4 vCPU, Go 1.27.3. A temporary
test in `pkg/ledger` (not committed): a pool of 40,000 mints, one claim
transaction per entry and a strength-1 layer on each so the chain is
selected; then an entry on `pool.ID()` with a spend of an input that does not
exist, timing `EntryOwners` (the preverify) and `AddEntry` (the handler):

```
selected height   EntryOwners   AddEntry   (both reject; mint not claimed in this ledger)
 2,000             1.32 ms       1.07 ms
10,000             8.14 ms       6.72 ms
40,000            59.65 ms      63.25 ms
```

The cost is linear in the height. Chains of entries with no transactions
cost far less (500 / 2,000 / 8,000 entries: 44 / 66 / 265 us), so the
transaction count is what matters. At 40,000 selected entries one pushed
entry holds the engine lock for about 120 ms; at the 20 entries a second one
address is allowed, that is more than two seconds of lock demand per second,
so one inbound connection can keep the engine lock saturated and starve every
other peer, the control API, `/v1/watch` and the chase. At 10,000 it is
about 0.3 s/s per address, and several addresses add up (each has its own
budget). Before BLUE-005 the factor was one half of that.

## Impact

Medium for a node whose pool has grown deep, low while the chain is short:
the cost scales with the age of the pool and needs no work, no stake and no
valid signature, only the entry format. A remote, unauthenticated peer can
degrade the whole node (it holds the global lock, as in RED-006 and RED-008),
and a valid fork entry earns no offence. No value is forged and consensus is
untouched, so it is availability only. The doubling is BLUE-005's; the
underlying cost is older.

## Recommendation

Several fixes, in the order I would take them:

- **Do not pay for it twice.** The preverify is there to move signature
  verification out of the lock, but the revert it needs is the expensive
  part. In `PreverifyEntry`, when the number of selected entries to revert
  (`len(chain) - anchor`) is above a small bound, skip the preverify (the
  signatures are then verified in `AddEntry` under the lock, as before
  ORANGE-004; a fork that deep is rare for an honest peer). This removes the
  doubling at once.
- **Price the revert.** Count the entries and transactions reverted as a
  cost against the sender: draw it from a per-peer (per-address) budget in
  `tracking`, as for the other request kinds, and refuse or drop the entry
  when the budget is out. An honest fork near the tip costs a few units;
  a fork at the pool id costs the chain.
- **Make the view reusable.** Cache the last one or two scratch states
  `viewAfter` built, keyed by (parent hash, chain version), and drop them
  when the selected chain changes. A burst of sibling entries on the same old
  parent then pays one revert, not one each. Mind the memory: an overlay
  over a deep revert is as large as the chain.
- Optionally give the fork depth a work gate: an entry whose parent is more
  than N entries below the tip must carry a strength proportional to the
  depth, so a deep fork is only as cheap as the work behind it. That is a
  consensus decision for the owner (it touches "no depth limit on
  switching"); the three points above need none.

Tests: in `pkg/ledger`, a benchmark of `AddEntry` and `EntryOwners` for a
parent at the pool id against heights of 1,000 / 10,000 (the cost must not
grow with the height for the cached case, or the entry must be refused for
the over-budget case); in `pkg/engine`, `PreverifyEntry` must not revert a
chain deeper than the bound; an end-to-end `pkg/p2p` test that a peer
sending many entries on the pool id is held to its budget.

## Resolution

Fixed by BLUE-006 (decision D111), taking the first two recommendations.
`ledger.Pool.EntryOwners` refuses with `ErrDeepParent` when the state after
the parent would cost more than `MaxPreviewCost` (64) entries of revert and
apply, so `PreverifyEntry` does nothing for such an entry and the revert is
paid once, in `AddEntry` (whose signatures are then verified under the
lock, as before ORANGE-004). It is not an offence, and the entry is still
taken. Every pushed entry now also spends `Engine.EntryCost` (the same
count, `ledger.Pool.ViewCost`) from the sender's new `revert` budget
(`tracking.Spend`, 200,000 entries a minute per address); over it the entry
is dropped and the address flagged once per window. Requested entries (the
chase, an honest deep reorg) are not charged. The cached view and the work
gate on fork depth are left open (D111 says why). Red's grind note is also
fixed: `POST /v1/grind` takes only the source `user`. Tests:
`TestEntryOwnersRefusesDeepParent` (`pkg/ledger`),
`TestPreverifySkipsDeepParent` (`pkg/engine`), `TestSpend`
(`pkg/tracking`), `TestDeepForkSpendsRevertBudget` (`pkg/p2p`), and a
custom source in `pkg/rpc`'s grind test.
