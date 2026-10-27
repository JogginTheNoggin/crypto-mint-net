# pkg/rpc

## Summary

The node's local control interface: JSON over HTTP on a loopback address,
with a bearer token, for the node's owner and their tools (decision D26).
It submits transactions, entries and layers, asks the node to build entries
and add layers, and reads status, payments and holdings. For the network
it reports on peers, the tracker and the chase queue, and gives the owner
controls over single peers. For watching it has views of pools, ledgers,
pending transactions, holdings, transactions and addresses, a live event
stream, and endpoints that change what the node follows and apply it at
once ([watch.md](watch.md), [filter.md](filter.md)). `WalletNode` is a `wallet.Node` over the
interface, so a wallet uses a running node. It is not the peer protocol
(that is `pkg/p2p`).

## Endpoints

All under `/v1`, all requiring `Authorization: Bearer <token>`.

Two tokens are accepted, both 32 random bytes in hex in files readable
only by the node's user and made on first start. `rpc.token` opens every
endpoint. `rpc.view-token` opens the `GET` endpoints only, and any other
method gets 403 with code `forbidden`. Give the view token to a monitor
(a Prometheus scrape of `/metrics`), a dashboard or any tool that should
only watch the node (GREEN-007).

| Method and path | Body or query | Reply |
| --- | --- | --- |
| `GET /status` | | every followed pool, and replay stats |
| `GET /series` | | followed series with their params |
| `POST /tx` | `{tx}` | `{series, pool, tid, forward}` |
| `POST /entry` | `{series, pool, entry}` | `{hash, switched, pruned, dropped}` |
| `POST /layer` | `{series, pool, layer}` | `{hash, strength, switched}` |
| `POST /build` | `{series, pool, max_tries}` | the entry reply plus `entry` bytes |
| `POST /work` | `{series, pool, entry, start, tries}` | the layer reply plus `layer` bytes |
| `GET /payment` | `series, serial, tid` | `{pending, selected, entry, from, tx, rivals: [{tip, fork, entry, conflict, after}]}`: `tx` is the transaction's bytes in hex when the node holds it; each rival is an entry on an inactive branch holding a transaction that spends one of its inputs, with the branch's weight after its fork |
| `GET /holdings` | `series, serial` | `[{tid, index, address, amount}]` |
| `GET /peers` | | `{node_id, listen, peers: [{id, addr, key, outbound, agent, listen, coverage, categories: {policy: {category, points, partial}}, since, sent, received}], book: [{addr, source, added, last_tried, failures, policies: [{section, state, last_heard}]}]}` |
| `GET /tracking` | `addr` (optional) | without: `{tracked: {key: status}, ranges: {range: until}}`; with: that address's record (`key, status, template, settings, flags, policies: {policy: measures and points}, log`) |
| `GET /chases` | | `{groups: [{id, pool, hashes, senders, evidence, added, confirmed, asked, denied}]}`, in queue order |
| `POST /peer` | `{addr, action, seconds, range, what, template}` | `{key, closed, held, status}`; actions `timeout`, `block` (`seconds: -1` until unblocked, `range` for the whole network range), `unblock`, `clear` (`what`: flags, measures, all), `template`, `delete` |

### Views, watching and following

These take the filter's query parameters (`series`, `pool`, `serial`,
`address`, `policy`, `tid`, `kind`, each comma-separated; see
[filter.md](filter.md)). Where a `series` is needed it may be left out
when the node follows exactly one series. Reply shapes are in
[watch.md](watch.md).

| Method and path | Query or body | Reply |
| --- | --- | --- |
| `GET /pools` | filter | `[Pool]`: each followed pool the filter covers |
| `GET /ledger` | `series`, `pool`, `below`, `limit`, filter | `Ledger`: a page of the selected ledger, newest first, and `next` |
| `GET /pending` | filter | `[PendingTx]` |
| `GET /unspent` | filter | `[Holding]` |
| `GET /tx` | `series`, `id`, `serial` (optional) | `TxStatus`; `not_found` when the node does not hold it |
| `GET /address` | `series`, `id`, `limit`, filter (pool, serial) | `Address`: balance, holdings, history, pending |
| `GET /watch` | filter | A stream of newline-delimited JSON events with `hello`, `ping`, `missed` and `end` notices; it ends on a reload |
| `GET /follow` | | `{follow, pools}`: the node policy's follow list and the pools followed now |
| `POST /follow` | `{remove, issuer, series, pools, mints, no_reload}` | `{follow, pools, reloading}`; saves the changed node policy and, unless `no_reload`, reloads the node after replying |
| `POST /reload` | `{}` | `{reloading: true}`; the node applies its node policy again |
| `GET /metrics` | | What `/status` and `/peers` report, in Prometheus text format 0.0.4 (`poo_pool_height`, `poo_pool_weight` as a float, `poo_pool_pending`, `poo_peers_connected`, …); a scraper sends the token as a bearer token (GREEN-002) |

A reload restarts the interface: `rpc.addr` is written again and the next
call should `Dial` afresh. `/watch` replies `not_found` without a hub,
and `/reload` and a reloading `/follow` without a reload function (an
interface served by `Serve` rather than `pkg/daemon`).

The p2p endpoints reply `not_found` when p2p is not running. A status is
`{flag_total, timed_out_until, blocked_until, timed_out, blocked,
priority}`.

Items are hex of their byte form; hashes are hex; weights are decimal
strings. Errors are `{"error", "code"}` with codes `bad_request`,
`unauthorized`, `not_found`, `conflict`, `invalid`, `full`,
`nothing_pending`, `internal`, and a matching HTTP status.

## API

- `NewHandler(node, token)` and `SetHost(host)`, `Serve(ctx, node, ready)`
  (writes `rpc.addr`, creates the token, shuts down cleanly),
  `ServeHost(ctx, node, host, ready)`, `LoadOrCreateToken(dir)`,
  `LoadOrCreateViewToken(dir)` and `SetViewToken(tok)` for the read-only token.
- `Client`: `NewClient(addr, token)`, `Dial(dir)`, `DialView(dir)` (the
  view token), and one method per
  endpoint (`Tracking`, `TrackingOf`, `Chases`, `Peer` for the new ones).
  Errors from the node are `*rpc.Error`.
- `ServeWith(ctx, node, ServeConfig{Host, Hub, Reload}, ready)` is what
  `pkg/daemon` uses; it closes the hub before shutting down so watch
  streams end cleanly. `SetHub` and `SetReload` do the same on a handler.
- Client view methods: `Pools`, `Ledger`, `Pending`, `Unspent`, `Tx`,
  `Address`, `Follows`, `Follow`, `Reload`, and `Watch(ctx, f, fn)`,
  which calls fn for each line until the stream ends.
- `manage.go` holds the tracking, chase and peer endpoints; `view.go`
  the views, watch, follow and reload.

## Notes

Submitted items are taken in as `Pushed` (a wallet is an unlisted node);
build and work are `Local`. A work request holds the node while it grinds,
so it is capped at 2^24 nonces.

## Wallets

`NewWalletNode(client, tidOf)` returns a `wallet.Node` over a client:
`Holdings` and `Payment` read the endpoints above (checking that the
transaction returned has the tid asked for), and `Submit` posts a
transaction. A transaction the node already holds is not an error for a
wallet: when `POST /tx` refuses one with a conflict (seen or spent), `Submit`
asks `GET /payment` and succeeds if the node holds it pending or
selected. See [wallet.md](wallet.md).
