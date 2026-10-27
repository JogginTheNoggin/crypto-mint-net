# pkg/rpc

## Summary

The node's local control interface: JSON over HTTP on a loopback address,
with a bearer token, for the node's owner and their tools (decision D26).
It submits transactions, entries and layers, asks the node to build entries
and add layers, and reads status, payments and holdings. It is not the peer
protocol (that is `pkg/p2p`); it only reports on it.

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
| `GET /payment` | `series, serial, tid` | `{pending, selected, entry, from}` |
| `GET /holdings` | `series, serial` | `[{tid, index, address, amount}]` |
| `GET /peers` | | `{node_id, listen, peers: [{id, addr, outbound, agent, listen, coverage, since, sent, received}]}`, or `not_found` when p2p is not running |

Items are hex of their byte form; hashes are hex; weights are decimal
strings. Errors are `{"error", "code"}` with codes `bad_request`,
`unauthorized`, `not_found`, `conflict`, `invalid`, `full`,
`nothing_pending`, `internal`, and a matching HTTP status.

## API

- `NewHandler(node, token)` and `SetHost(host)`, `Serve(ctx, node, ready)`
  (writes `rpc.addr`, creates the token, shuts down cleanly),
  `ServeHost(ctx, node, host, ready)`, `LoadOrCreateToken(dir)`.
- `Client`: `NewClient(addr, token)`, `Dial(dir)`, and one method per
  endpoint. Errors from the node are `*rpc.Error`.

## Notes

Submitted items are taken in as `Pushed` (a wallet is an unlisted node);
build and work are `Local`. A work request holds the node while it grinds,
so it is capped at 2^24 nonces.
