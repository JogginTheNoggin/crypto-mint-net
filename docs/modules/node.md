# pkg/node

## Summary

A full node with a data directory. `Open` reads the node policy, makes the
accepted series known, applies the follow rules, and replays each followed
pool's store through the engine. It finds the stores by reading the
`pools` directory, not by asking about every followed pool, so a series of
very many pools followed whole costs only what is stored (RED-046, D152). After that, every item the engine keeps is
appended to its pool's store before the call returns. Logs are compacted on
start-up and on close when most records are dead (decision D23). This is
the layer the p2p host and the RPC build on. Observers hear of every item
the node keeps, which is how the host forwards.

## Data directory

```
node.json                    node policy
LOCK                         held while the node runs
pools/<series_id>/<k>.log    one store per followed pool
rpc.token, rpc.addr          written by package rpc
```

Stores of pools no longer followed are left untouched, so following a pool
again picks up its history.

## API

- `Open(dir, Options{Log, CompactMin})`, `Close`.
- `HandleTx`, `HandleEntry`, `HandleLayer`, `Build`, `Work`: the engine's
  calls, plus storing what is kept. Calls are serialized so each log is in
  the order the engine accepted items. `Build`, `BuildContext` and `Work`
  search without the node's lock and then take the result in like
  `HandleEntry` (D99), so a long search never stalls peers or the RPC.
  `BuildWith(ctx, k, maxTries, g)` and `WorkWith(ctx, k, entry, start,
  tries, g)` take an `engine.Grind`: the hasher and workers a
  [grind](grind.md) plan chose, and a counter of tries. `Work` with tries
  0 grinds until ctx ends and keeps the strongest layer (M11, D109).
- `HandleTxFrom`, `HandleEntryFrom`, `HandleLayerFrom`: the same, naming
  the peer that sent the item.
- `Observe(f)`: f receives an `Event{Key, Tx|Entry|Layer, Origin, From,
  Forward, Switch, Dropped, Weight}` for every item kept from then on, after it is
  written to the store, in order. The fsync may still be running; the
  `Handle` call returns only after it (D105). It runs under the node's lock: it must not block or
  call the node.
- `Engine()` for reads (changes made directly on the engine are not
  stored), `Policy`, `Dir`, `Replayed` (`ReplayStats`), `Stores`,
  `Compact(key)`.
- `BodyCache()`, `Bodies()` (`BodyStats`: the cache's bound, use, hits,
  loads and errors, and the entries held, in memory and located),
  `FileLimit()` (the open file check at `Open`, ORANGE-037).
- `ErrClosed`.

## Entry bodies on disk (D172)

Each pool's ledger gets a body source that reads the pool's log by
location. Locations are built while the log is replayed (`store.OpenFunc`,
one window of 1,024 records at a time) and as records are appended, and
rebuilt by each compaction, which copies entry records from the old file
by location. A body read back is used only if the record is an entry with
the hash asked for. The node's one `ledger.BodyCache` is bounded by
`entry_cache_mib`. Lock order: the engine's lock, then a pool's location
lock, then the log's; a compaction holds the location lock while it
rewrites, so reads wait for the new locations. No index file is kept yet;
start-up reads the whole log anyway (a sidecar index comes with snapshots,
R30).

`Open` logs one warning when the soft open file limit is below the
followed pools plus 256 (`nodepolicy.CheckFileLimit`).

## Tests

`node_test.go`: restart restores tip, weight, branches, holdings and
pending; compaction (explicit and automatic) keeps state; a torn write is
cut; a record edited on disk is refused on replay; a second node on the
same directory is refused; follow rules and unaccepted series.
`bodies_test.go`: with no cache, every body reads back from disk and the
node selects and holds the same, through a compaction, a restart and
entries taken in after each; `entry_cache_mib` sets the bound.
`bodies_internal_test.go`: a location holding another entry or another
kind of record is never given as the body. Events are
exercised by the p2p tests.
