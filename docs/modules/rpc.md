# pkg/rpc

## Summary

The node's local control interface: JSON over HTTP on a loopback address,
with a bearer token, for the node's owner and their tools (decision D26).
It submits transactions, entries and layers, asks the node to build entries
and add layers, and reads status, payments and holdings. For the network
it reports on peers, the tracker and the chase queue, and gives the owner
controls over single peers. `WalletNode` is a `wallet.Node` over the
interface, so a wallet uses a running node. It is not the peer protocol
(that is `pkg/p2p`).

## Endpoints

All under `/v1`, all requiring `Authorization: Bearer <token>`.

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
  `ServeHost(ctx, node, host, ready)`, `LoadOrCreateToken(dir)`.
- `Client`: `NewClient(addr, token)`, `Dial(dir)`, and one method per
  endpoint (`Tracking`, `TrackingOf`, `Chases`, `Peer` for the new ones).
  Errors from the node are `*rpc.Error`.
- `manage.go` holds the tracking, chase and peer endpoints.

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
