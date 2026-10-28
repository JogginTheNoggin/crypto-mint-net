# pkg/node

## Summary

A full node with a data directory. `Open` reads the node policy, makes the
accepted series known, applies the follow rules, and replays each followed
pool's store through the engine. After that, every item the engine keeps is
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
- `ErrClosed`.

## Tests

`node_test.go`: restart restores tip, weight, branches, holdings and
pending; compaction (explicit and automatic) keeps state; a torn write is
cut; a record edited on disk is refused on replay; a second node on the
same directory is refused; follow rules and unaccepted series. Events are
exercised by the p2p tests.
