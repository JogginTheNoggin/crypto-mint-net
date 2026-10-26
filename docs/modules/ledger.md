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
  `Info`, `Layers`, `InSelected`, `From(h)` (weight added since entry h),
  `Locate(tid)`, `Chain`, `HeightOf`, `Branches`, `SetK`.
- Errors: `ErrKnownEntry`, `ErrUnknownParent`, `ErrInvalidEntry`,
  `ErrUnknownEntry`, `ErrWeakLayer`, `ErrKnownLayer`, `ErrNotSelected`.

## How it works

- **Nodes** hold an entry, its credit, its layers and its arrival number.
  The selected chain is a slice; its prefix weights live in a Fenwick tree
  (`fenwick.go`), so a transfer layer deep in the chain costs O(log n).
- **Off-chain nodes** keep weight relative to where their branch leaves the
  chain, so layers on the chain never touch them (decision D17).
- **Checking an entry** off the tip builds an overlay of the selected state,
  reverts to the fork point and applies the branch up to the parent.
- **Switching** reverts the old segment, applies the new one, re-anchors the
  relative weights, and sorts displaced transactions into unchanged, carried
  and void by replaying them in order (decision D14).
- **Pruning** keeps up to `Ties` branches tying the selected weight and the
  top K others (decision D12). Unknown-parent entries are refused, not
  held; fetching the missing parent is the network layer's job.

## Tests

- `pool_test.go`: Appendix A.1 to A.4 with exact weights and the exact void
  and carried sets, sibling entries, ties, entry hash credit, transfer
  layers, pruning and every error.
- `property_test.go`: random trees, transactions and layers compared
  against a model that recomputes every ledger's weight and state from
  scratch. Extend it when changing selection, switching or pruning.
