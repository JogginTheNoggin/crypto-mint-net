# pkg/ledger

## Summary

The heart of the full node: one pool's tree of entries, its weights, and the
selected ledger. It adds entries and layers, keeps the heaviest valid ledger
selected, switches only to a strictly heavier one (keeping the first seen on
ties), reports what a switch displaced, and prunes to the selected ledger
plus the top K other branches. It reproduces the design's worked example
(Appendix A) and is checked against a brute-force model on random histories.

## API

- `New(series, poolIndex, Config{K, Ties, Clock})`; `DefaultK`,
  `DefaultTies`, `Unlimited`.
- `AddEntry(e) (AddResult, error)` and `AddLayer(l) (AddResult, error)`.
  `AddResult` carries the entry hash, an optional `*Switch` and the pruned
  hashes.
- `Switch{From, To, Fork, ForkHeight, Reverted, Applied, Unchanged, Carried,
  Void}` and `Extension()` (true when the new tip simply extends the old).
- Queries: `Tip`, `Weight`, `Height`, `Len`, `State`, `Has`, `Entry`,
  `Info`, `Layers`, `LayerCount`, `LayerNonces` (unsorted, for sorting
  outside the engine lock, RED-020), `InSelected`, `From(h)` (weight added
  since entry h), `Locate(tid)`, `Chain`, `HeightOf`, `Branches`, `SetK`.
- Building on a branch (D161): `CheckOn(parent, txs)` returns the
  transactions valid together on the state after a held entry or the
  pool_id, as `AddEntry` would find them, and the others as `Refused{TID,
  Err}`; `ClaimEntry(serial)` the selected entry holding a mint's claim and
  its tid (`ErrNoClaim`); `ParentOf(h)`. A parent not held is `ErrNotHeld`.
- `Rivals(t, tid)`: for each inactive branch, the first entry from its tip
  down to its fork that holds a transaction other than `tid` spending one
  of `t`'s inputs, as `Rival{Branch, Conflict, Entry, After}`; `After` is
  the branch's weight after its fork, the margin a receiver compares
  from(e) against (design section 7.3). Wallets read it through the
  engine's `Payment`.
- `ReplayOrder()`: every held entry in an order that rebuilds the same tree
  and selected ledger in an empty pool (the selected ledger first, then the
  rest by first sighting). Stores use it to compact (decision D23).
- `EntryOwners(e)`: `AddEntry`'s checks short of the witness signatures,
  for the engine's preverify. It refuses with `ErrDeepParent` (not an
  offence; `AddEntry` still takes the entry) when building the state after
  e's parent would cost more than `MaxPreviewCost` (256 units), so that
  revert is paid once, in `AddEntry` (RED-011, D111). `ViewCost(e, limit)`
  is that cost: one unit per entry reverted or applied plus one per
  transaction in it, 0 on the tip or on the entry `AddEntry` kept the state
  after (RED-012, D114).
- Errors: `ErrKnownEntry`, `ErrUnknownParent`, `ErrInvalidEntry`,
  `ErrUnknownEntry`, `ErrWeakLayer`, `ErrKnownLayer`, `ErrNotSelected`,
  `ErrDeepParent`.

## How it works

- **Nodes** hold an entry, its credit, its layers and its arrival number.
  The selected chain is a slice; its prefix weights live in a Fenwick tree
  (`fenwick.go`), so a transfer layer deep in the chain costs O(log n).
- **Off-chain nodes** keep weight relative to where their branch leaves the
  chain, so layers on the chain never touch them (decision D17).
- **Checking an entry** off the tip builds an overlay of the selected state,
  reverts to the fork point and applies the branch up to the parent.
  `AddEntry` keeps the overlay after the entry it added, unless that entry
  switched the ledger, so the next entry on it (every entry of a branch
  taken in order) starts there; any switch or the prune of that entry drops
  it. Off-chain nodes keep their anchor's height, the pool keeps the set of
  off-chain leaves for ranking and pruning, and `units` is a prefix sum of
  view cost along the selected chain, so taking in a branch of L entries
  costs O(L), not O(L^2) (RED-012, D114).
- **Switching** reverts the old segment, applies the new one, re-anchors the
  relative weights, and sorts displaced transactions into unchanged, carried
  and void by replaying them in order (decision D14).
- **Pruning** keeps up to `Ties` branches tying the selected weight and the
  top K others (decision D12), and the newest entry while it is a leaf
  (decision D79), so a heavier branch fetched one entry at a time is not
  dropped before its layers arrive. Unknown-parent entries are refused, not
  held; fetching the missing parent is the network layer's job.

## Tests

- `pool_test.go`: Appendix A.1 to A.4 with exact weights and the exact void
  and carried sets, sibling entries, ties, entry hash credit, transfer
  layers, pruning and every error.
- `property_test.go`: random trees, transactions and layers compared
  against a model that recomputes every ledger's weight and state from
  scratch, then rebuilds the pool from `ReplayOrder` and checks it selects
  the same ledger. Extend it when changing selection, switching or
  pruning.
- `deep_test.go`: `ViewCost` on the tip, near it, at the pool id and past
  an off-chain path, and `EntryOwners` refusing a deep parent while
  `AddEntry` takes the entry (RED-011); the kept state is the parent's
  (siblings, older parents, a refused entry), and a 4,000-entry branch
  with no layers costs nothing to view entry by entry (RED-012).
  `CheckOff` also checks the leaf set and every anchor.
- `bench_test.go`: `BenchmarkOffChainBranch` (per-entry time must not grow
  with `-benchtime`; 1.5 us at 16,000 and 64,000 entries).
