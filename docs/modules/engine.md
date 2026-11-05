# pkg/engine

## Summary

The transport-independent full node. It holds the known series, what the
node follows, and for each followed pool a ledger and a pending pool. Every
item (transaction, entry, layer) goes through one `Handle*` call that
returns whether to forward it and what changed. Networking and storage in
later milestones are built around this API; the engine never reads a clock
or opens a socket. Calls are safe from multiple goroutines.

## API

- `ErrFailed`, `Failed()`, `Err()` (D174): a panic while `HandleTx`,
  `HandleEntry` or `HandleLayer` changes a pool fails the engine, which
  then takes nothing more; the daemon stops the node on `Failed()`.
  `Config.Source` gives each pool its body source (D172).
- `New(Config{Ledger, MaxPending, SeenSize})`, `DefaultConfig`.
- Series and following: `AddSeries`, `Series`, `FollowSeries`,
  `FollowSeriesExcept` (a whole series but some pools; a pool any rule
  follows stays followed, D154), `FollowPool`, `FollowMint` (follows the
  mint's pool, decision D20),
  `Follows`, `KeyOf(tx)`. `PoolKey{Series, Index}` names a pool.
- Items: `HandleTx(t, origin)`, `HandleEntry(key, e, origin)`,
  `HandleLayer(key, l, origin)`. `Origin` is `Pushed`, `Requested`,
  `Local` or `Stored` (read back from the node's store, decision D27).
  Results carry `Forward` (true only for `Pushed` and `Local`), and for
  entries and layers the `Switch` and pruned hashes.
- Building on a chosen parent (D161): `BuildEntryOn(ctx, key, maxTries, g,
  BuildOn{Parent, Txs})` builds on any held entry or the pool_id, from
  chosen transactions or the pending ones, leaving out (and returning as
  `ledger.Refused`) those that do not fit there. `ClaimParent(key,
  serial)` is the parent of the selected entry holding a mint's claim,
  where a rival claim goes; `PoolID(key)` is the pool_id.
- Building: `Build(key, maxTries)` bundles ready pending transactions on the
  tip (`ErrNothingPending`, `ErrAlreadyBuilt`); `Work(key, entryHash, start,
  tries)` grinds and adds a layer (`ErrNoStrongerLayer`). `BuildEntry(ctx,
  key, maxTries)` and `GrindLayer` do the search without taking the item
  in, and hold the engine's lock only to read the pool, never while
  searching (D99). `BuildEntryWith` and `GrindLayerWith(ctx, ...)` take a
  `Grind{New, Workers, Tried}`: the hasher (package grind picks one per
  device), the goroutines and a try counter; `GrindLayerWith` with tries
  0 grinds until ctx ends and keeps the strongest layer found (D109).
- Reading: `FollowedPools()` (every followed pool, for the p2p index);
  `EachFollowedPoolOf(id, fn)` (one series' pools, without the list);
  `FollowsWhole(id)`; `ListedPools()`
  (what status, metrics, queries and grind targets go through: every pool
  followed by index, every pool of a whole series under `ListAll`, 65,536
  pools, and of a larger one only the pools with state, RED-046, D152);
  `FollowedCount()`, `FollowedSeries()`; `Pending(key)` (ready pending transactions,
  oldest first), `Status(key)`, `Payment(key, tid)` (pending, in which entry,
  weight added since, selected, the transaction itself, and its rivals on
  inactive branches from `Ledger.Rivals`), `Holdings(series, serial)`,
  `Ledger(key, fn)` for read access under the lock.
- Before the lock: `PreverifyTx(t)` and `PreverifyEntry(key, e, origin)`
  verify witness signatures with the engine's lock held only to find the
  pool, so a node checks items from several peers on several cores, and
  the checks in `HandleTx`/`HandleEntry` find them remembered (ORANGE-004,
  D105). Under the lock they run every check `Handle*` runs before a
  signature (`mempool.Pool.Owners`, `ledger.Pool.EntryOwners`: seen or
  held, pending, conflicts, inputs unspent, amounts, an entry's shape and
  parent), and verify each witness only against the address it must
  satisfy, stopping at the first that fails (`tx.Preverify`; RED-009,
  D107). They change no result. A bad witness wraps `tx.ErrWitness`,
  which `pkg/p2p` charges as an offence. `PreverifyEntry` does nothing for
  a parent more than `ledger.MaxPreviewCost` units of revert away: that
  state is built once, in `HandleEntry` (RED-011, D111).
- `EntryCost(key, e)`: `ledger.Pool.ViewCost` of what `HandleEntry` would
  revert and apply to check e (at most about `MaxCountedCost`), which
  `pkg/p2p` charges to the sender's revert budget.
- `Seen`: a fixed-size ring of valid item hashes (decision D15: requested
  items bypass it).

## Behavior

After a switch the engine returns carried transactions to the pending pool
and revalidates it. Only valid items enter the seen list. A transaction's tid
leaves out its witnesses, so a copy with a bad witness is refused without
blocking the valid copy that arrives later.

## Tests

`engine_test.go` runs several engines on an in-process flood network:
payments spreading, a double spend resolved by weight and then reversed,
following rules, origins and the seen list.

## Pool state

A pool's state (its ledger and pending pool) is kept once an item leaves
something in it: an entry, a pending or waiting transaction. An item that
is refused leaves none, and a query or a check reads a fresh state that is
not kept. Every pool's state starts the same, so a state made later is the
state made at once, and nothing a node accepts changes (RED-049, D154).

