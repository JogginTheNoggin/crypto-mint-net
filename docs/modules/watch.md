# pkg/watch

## Summary

Lets a node's owner look at what the node holds and follow it live. The
**views** show the followed pools, a pool's selected ledger page by page,
pending transactions, unspent portions, any transaction and any address.
The **hub** turns every item the node keeps into events and fans them out
to subscribers, each with its own filter. Every view and every
subscription takes a [`filter.Filter`](filter.md), so the command line,
the RPC and the GUI select items the same way. Nothing here changes the
node. Decisions D90 (filter), D91 (stream) and D94 (bounded views).

## Views

| Call | Returns | Filter fields used |
| --- | --- | --- |
| `Pools(e, f)` | `[]Pool`: series, pool, first and last serial, tip, height, weight, entries, branches, pending | series, pool, serial |
| `LedgerPage(e, key, f, below, limit)` | `Ledger`: the pool's height and weight, entries newest first from height `below` (0: the tip), and `next`, the height to ask for next (0: done); `unreadable` counts entries whose bodies could not be read back from the log, each listed with `unreadable: true` and no transactions (RED-084, D173); `AddressPage` counts them the same way | all but kind; with item fields set, only entries holding a matching transaction, with only those transactions |
| `Pending(e, f)` | `[]PendingTx`: pending transactions, oldest first within each pool | all but kind |
| `Holdings(e, f)` | `[]Holding`: unspent portions (input, serial, address, amount) | all but kind; tid selects by the transaction that made the portion |
| `TxOfSeries(e, series, tid, serial)` | `TxStatus`: pool, pending, selected, entry, height, weight behind it (`from`), rivals, and the transaction with its `raw` bytes | none; `serial` (optional) limits the search to one pool. `ErrNotFound` when the node does not hold it |
| `AddressOf(e, series, addr, f, limit)`, `AddressPage(ctx, e, series, addr, f, below, limit)` | `Address`: balance, holdings, selected history newest first (what each transaction received and how many portions it spent), pending, and `Next` per unfinished pool; looks at most 50×limit entries in all (D103); a page keeps whole entries, and each pool's `Next` starts at its newest entry not on the page, so following `Next` reaches every item once (GREEN-014) | series, pool, serial |

`Export(e, key, from, to)` (D188) returns `ExportPage`: the pool's
height, tip and weight, and its selected ledger from height `from` to
`to` (0: the tip), oldest first, at most `ExportLimit` (256) entries and
no more once the items reach `ExportBytes` (8 MiB). Each entry has an
`ExportEntry` summary (height, hash, parent, transaction and layer
counts) and an item, `wire.EncodeItem` with its layer nonces, exactly as a
peer sends it. `next` is the height to ask for next (0: done). Bodies are
read outside the engine's lock; the page stops before an entry whose body
cannot be read back and counts it in `unreadable`.

A page holds at most `limit` items (default `DefaultLimit` 50, at most
`MaxLimit` 1000). A view walks only the pools the filter's scope covers
and, in a ledger, only as far as the page needs (D94).

A transaction appears as `Tx`: `tid`, `series`, `serial`, `claim`,
`inputs` (`tid`, `index`, and `address` when the witness shows it),
`outputs` (`index`, `address`, `amount`), `addresses` (every address it
touches), `amount` (sum of outputs) and `size`. The address of a spent
input is the address its witness's policy makes under the series hash.

## The event stream

`NewHub(node)` observes the node. `hub.Subscribe(f, buffer)` returns a
`Sub` whose channel `C` carries the events `f` selects, in the order the
node kept the items. A subscriber more than `buffer` events behind
(default `DefaultBuffer` 1024) loses events instead of slowing the node;
`Missed()` says how many and resets the count. `Close` ends one
subscriber, `hub.Close` all of them. While nobody is subscribed the hub
costs almost nothing.

### Event JSON

One object per event. Hashes are hex, weights are decimal strings, times
are RFC 3339 UTC.

```json
{
  "seq": 42,
  "time": "2026-10-27T21:00:00Z",
  "kind": "selected",
  "series": "<hex>",
  "pool": 1,
  "weight": "123456",
  "origin": "pushed",
  "from": "203.0.113.5:7733",
  "tx": { "tid": "<hex>", "serial": 5, "inputs": [], "outputs": [], "...": "as Tx above" },
  "tid": "<hex>",
  "entry": { "hash": "<hex>", "parent": "<hex>", "height": 7, "tids": ["<hex>"], "selected": true },
  "layer": { "entry": "<hex>", "hash": "<hex>", "nonce": 9, "strength": 21, "credit": "2097152" },
  "switch": { "from": "<hex>", "to": "<hex>", "fork": "<hex>", "fork_height": 5,
              "reverted": [], "applied": [], "carried": [], "void": [] },
  "void": false,
  "missed": 0
}
```

`seq`, `time`, `kind`, `series`, `pool` and `weight` (the pool's selected
weight after the item) are always set. `origin` is `pushed`, `requested`,
`local` or `stored`; `from` names the peer that sent the item. The other
fields depend on the kind:

| Kind | When | Fields |
| --- | --- | --- |
| `tx` | A transaction became pending | `tx`, `tid` |
| `entry` | The node kept an entry | `entry` (with height and whether it is selected) |
| `layer` | The node kept a layer | `layer` |
| `selected` | A transaction entered the selected ledger (one event per transaction, also on a plain extension) | `tx`, `tid`, `entry` (hash, height) |
| `reverted` | A transaction left the selected ledger on a switch | `tid`, `entry` (the entry it was in), `tx` when the node still has it, `void` (true: it can no longer apply; false: it is pending again) |
| `switch` | The selected ledger moved to another branch | `switch` |
| `dropped` | A pending transaction became invalid | `tid` |
| `grind` | A grind job started, chose a plan, made progress, found a layer or entry, waited or ended ([grind.md](grind.md)) | `grind`: `{job, kind (start, plan, progress, found, waiting, end), series, pool, state, reason, hint, spec, plan, tries, rate, layer, entry, strength, from}`; `weight` is not set |

A job-wide `grind` event (a job's start, waiting and end) has no series
or pool, here or in `grind`. It passes a filter on series or pools when
the job's spec can work on one of them (a spec with no series or pools
can work on any), and never a filter on serials, addresses, policies or
transactions (GREEN-017). `Hub.PublishGrind` sends
them; the daemon wires the grind manager to it.

A switch produces its `selected` and `reverted` events before the
`switch` event. An `entry` or `layer` event matches a filter by the
transactions in its entry, a `switch` event by every transaction it
applied or displaced.

### Over the RPC

`GET /v1/watch` with filter query parameters streams the events as
newline-delimited JSON (`application/x-ndjson`). The stream adds notices,
whose kinds are not event kinds:

| Notice | Meaning |
| --- | --- |
| `hello` | First line: the stream is open |
| `ping` | Nothing else for `rpc.PingInterval` (15 s): the node is alive |
| `missed` | `missed` events were dropped since the last line because the reader was slow; written before the next event |
| `end` | Last line: the node is stopping or reloading |

Events while the node reloads are not replayed (D91): a client that sees
`end` connects again, reads the views for where things stand, and goes
on. `rpc.Client.Watch(ctx, f, fn)` reads the stream; `poon watch` does
the reconnecting.

## Tests

`watch_test.go`: `TestWatchAndViews` runs claims, payments, a rival
branch and a switch through a node and checks every view and every event
kind under several filters; `TestSlowWatcherMisses` checks the missed
count. `pkg/rpc`'s `TestViews` and `pkg/cli`'s `TestViewFollowAndWatch`
cover the same through the interface and the commands.

## Extending

A new view is a function over `*engine.Engine` and a `Filter` that
returns JSON-ready values, plus an endpoint in `pkg/rpc/view.go`, a
client method and a command. Keep it bounded: skip pools with
`MatchScope` and stop at the page limit. A new event kind needs a
`filter.Kind`, a case in `Hub.observe` and a row in the table above.
