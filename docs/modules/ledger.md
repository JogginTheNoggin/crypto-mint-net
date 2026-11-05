# pkg/ledger

## Summary

The heart of the full node: one pool's tree of entries, its weights, and the
selected ledger. It adds entries and layers, keeps the heaviest valid ledger
selected, switches only to a strictly heavier one (keeping the first seen on
ties), reports what a switch displaced, and prunes to the selected ledger
plus the top K other branches. It reproduces the design's worked example
(Appendix A) and is checked against a brute-force model on random histories.

## API

- `New(series, poolIndex, Config{K, Ties, Clock, Source, Bodies})`;
  `DefaultK`, `DefaultTies`, `Unlimited`.
- Entry bodies on disk (D172): with `Config.Source`, a `BodySource`
  (`Has(h)`, `Load(h)`; the node's reads the pool log), the pool keeps
  each held entry's skeleton (hash, parent, height, weights, layers,
  items, undo) and drops the body once the source has it, on the next
  `AddEntry` or `ReleaseBodies()`. `Entry`, `Rivals` and `ReplayOrder`
  users read bodies back through `Config.Bodies`, a `BodyCache`
  (`NewBodyCache(maxBytes)`, `SetMax`, `Stats`) shared by every pool of a
  node and bounded in bytes. No check of an entry reads a body that is not
  in memory, so the cache changes cost, never verdicts. A failed read is
  counted (`BodyStats.Errors`). `Entry(h)` returns `(*entry.Entry,
  error)`: `ErrNoEntry` for an entry not held, `ErrUnreadable` for one
  whose body cannot be read back, and never a nil entry without an error,
  so a caller cannot use one by mistake (RED-084, D173). `Tx(h, tid)`
  gives a transaction from the skeleton, without witnesses once the body
  was released. `Rivals` searches the skeleton's items, so it reads no
  body and misses no rival. The cache counts a body as its byte form plus
  1 KiB, about what it takes in memory (ORANGE-038, D173).
  `InMemory()` counts bodies still held by the pool.
- Items and undo outside a window (D175): with `Config.RecentWindow` and a
  source, entries of the selected ledger more than the window below the
  tip go cold once the source has their bodies (`Cold()` counts them):
  their items and undo leave memory. Items are read back from the body,
  and undo is recomputed from the tids index (each spent portion is an
  output of the transaction that made it). A switch warms every entry it
  reverts before changing anything; a view on a deep anchor reverts cold
  entries on an overlay. A failed read panics with `ReadFault` before the
  pool changes (the engine then fails, D174). `Tx`, `ClaimEntry` and
  views read cold entries back.
- `Ref(h)` returns a `BodyRef`, taken under the engine's lock; its `Get`
  reads the body without the lock (D174).
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
  A transaction on a mint of another pool is refused with
  `entry.ErrWrongPool`, before any grind (RED-069). Conflicts are set
  lookups (ORANGE-033). `CheckPendingOn` is the same for the pending
  pool's transactions: it checks claims, inputs and amounts against the
  parent (`tx.Owners`) but not shape or witnesses, which the pending pool
  checked against the same addresses, so a build on a chosen parent costs
  time linear in the pool with no signature check under the engine lock
  (RED-068, D162: 20,000 pending in about 30 ms, was 1.3 s).
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
  is that cost: one unit per entry reverted or applied, plus one per
  transaction in it, plus one per distinct transaction its inputs spend
  from, since a cold entry's revert reads each such maker's body (RED-086,
  D177); 0 on the tip or on the entry `AddEntry` kept the state
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
