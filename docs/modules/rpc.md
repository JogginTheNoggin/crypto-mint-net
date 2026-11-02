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
only watch the node (GREEN-007). The view token is refused before a body
is read. A request body is at most `MaxBody` (8 MiB), except `POST /entry`
with `rpc.token`: an entry has no size bound, as the node takes one of any
size from a peer in parts, so its owner may hand it one of any size too
(RED-038, D144); and `POST /policy/import` with `rpc.token`, which takes up
to `MaxImportBody`, so any bundle `poo policy import` reads fits (RED-045,
D150).

| Method and path | Body or query | Reply |
| --- | --- | --- |
| `GET /status` | | every followed pool, and replay stats; `policy_changed` when node.json differs, outside the grind section, from the policy the node runs, so a change waits for a reload (GREEN-037) |
| `GET /series` | | followed series with their params |
| `POST /tx` | `{tx}` | `{series, pool, tid, forward}` |
| `POST /entry` | `{series, pool, entry}` | `{hash, switched, pruned, dropped}` |
| `POST /layer` | `{series, pool, layer}` | `{hash, strength, switched}` |
| `POST /build` | `{series, pool, max_tries, device, threads, for}` | the entry reply plus `entry` bytes and `grind` (the plan, tries, seconds, rate); `max_tries` at most `MaxBuildTries` (2^24), 0 meaning that unless `for` (up to `MaxGrindFor`, 4m) bounds the time; the search holds no lock and stops if the client goes away (D99) |
| `POST /work` | `{series, pool, entry, start, tries, device, threads, for}` | the layer reply plus `layer` bytes and `grind`; `tries` 1 to 2^24, or 0 with `for` |
| `GET /grind/devices` | | `{devices}`: what the node grinds on ([grind.md](grind.md)) |
| `GET /grind` | | every grind job, running and recently ended |
| `POST /grind` | a `grind.Spec`: `{target, series, pools, device, threads, for, until_from, hours, lease}` | the job; 503 `full` at 16 jobs, 400 for a bad spec, a device that cannot grind (unless `hours` is set), or a `source` other than `user` (D111) |
| `POST /grind/{id}/renew` | | renews the job's lease; the job |
| `GET /grind/schedule` | | `{grind, version, limits}`: `node.json`'s grind section, a name for its content, and the bounds it is checked against (`max_jobs`, `max_threads`, `max_layer_floor`, `max_layer_rate`, `default_layer_rate`; D130) |
| `PUT /grind/schedule` | a grind section `{device, threads, jobs}`; optional `If-Match: "VERSION"` | saves it and applies it without a reload: the policy's jobs taken out stop and the ones added start, the rest run on (all restart when a default changes), and user jobs run on; `{grind, version, started, failed}`: `started` is the running job of each scheduled job, kept or new (D135), with why a job did not start, by index; 422 `invalid` for a section that does not validate; 409 `conflict` when `If-Match` names an older version (D125) |
| `POST /grind/schedule/edit` | `{remove: [{n, job}], remove_all, add: [job], set: {device, threads, layer_floor, layer_rate}, version}` | applies the edit to the section the node holds, under its lock, so edits made at once all apply; a removal names the whole job at number `n`, so a stale number is refused with 409 `conflict` rather than removing another job; reply as for `PUT` (GUI-014, D125) |
| `GET /grind/{id}` | | one job |
| `DELETE /grind/{id}` | | stops it; the job as it ended |
| `DELETE /grind` | `?source=user` (optional) | stops every job (or only those made over the RPC) |
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
| `GET /address` | `series`, `id`, `below`, `limit`, filter (pool, serial) | `Address`: balance, holdings, history, pending, and `next` (per pool, the `below` for the rest); at most 50×`limit` entries looked at per request, stopped if the client goes away (D103) |
| `GET /watch` | filter | A stream of newline-delimited JSON events with `hello`, `ping`, `missed` and `end` notices; it ends on a reload |
| `GET /follow` | | `{follow, pools}`: the node policy's follow list and the pools followed now |
| `POST /follow` | `{remove, replace, issuer, series, pools, mints, no_reload}` | `{follow, pools, reloading}`; saves the changed node policy and, unless `no_reload`, reloads the node after replying. `replace` puts `pools` and `mints` in place of the series' rule in one change (GUI-018, D139) |
| `POST /policy/import` | `{policy, options: {prefer_file, replace, settings, full, series_only}, dry_run, no_reload}` | `{plan, saved, reloading}`: takes only what to follow unless `settings` or `full` (D150); the body may be up to `MaxImportBody` (any bundle the CLI reads); merges a node policy from a bundle into the node's (`nodepolicy.Merge`) under the same lock as `/follow`, saves it unless `dry_run` or nothing changes, and reloads unless `no_reload`. 422 `invalid` when the result would not validate. The `rpc` section is never taken, even with `full`: it changes only in node.json with the node stopped, so a token holder cannot open the interface to the network (D149) |
| `POST /reload` | `{}` | `{reloading: true}`; the node applies its node policy again. 422 `invalid` when `node.json` does not load, and the node keeps its current policy |
| `GET /metrics` | | What `/status` and `/peers` report, in Prometheus text format 0.0.4 (`poo_pool_height`, `poo_pool_weight` as a float, `poo_pool_pending`, `poo_peers_connected`, …); a scraper sends the token as a bearer token (GREEN-002) |

A reload restarts the interface: `rpc.addr` is written again and the next
call should `Dial` afresh. `/watch` replies `not_found` without a hub, the grind jobs without a
grind manager (build and work still grind, and `/grind/devices` answers),
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
- `ServeWith(ctx, node, ServeConfig{Host, Hub, Reload, Grind}, ready)` is what
  `pkg/daemon` uses; it closes the hub before shutting down so watch
  streams end cleanly. `SetHub` and `SetReload` do the same on a handler.
- Client view methods: `Pools`, `Ledger`, `Pending`, `Unspent`, `Tx`,
  `Address`, `Follows`, `Follow`, `Reload`, and `Watch(ctx, f, fn)`,
  which calls fn for each line until the stream ends.
- Grinding: `BuildWith`, `WorkWith` (with `GrindOptions{Device, Threads,
  For}`), `GrindDevices`, `GrindJobs`, `GrindJob`, `GrindStart`,
  `GrindStop`, `GrindStopAll`; `SetGrind(manager)` on a handler.
  `ErrNotRunning` wraps a `Dial` with no node.
- `manage.go` holds the tracking, chase and peer endpoints; `view.go`
  the views, watch, follow and reload; `grind.go` grinding.

## Notes

Submitted items are taken in as `Pushed` (a wallet is an unlisted node);
build and work are `Local`. A work request holds its connection while it
grinds, so it is capped at 2^24 nonces or 4 minutes; longer work is a
grind job. `/metrics` adds `poo_grind_tries_total{device,hash}` and
`poo_grind_jobs{state}` when the node runs jobs.

## Wallets

`NewWalletNode(client, tidOf)` returns a `wallet.Node` over a client:
`Holdings` and `Payment` read the endpoints above (checking that the
transaction returned has the tid asked for), and `Submit` posts a
transaction. A transaction the node already holds is not an error for a
wallet: when `POST /tx` refuses one with a conflict (seen or spent), `Submit`
asks `GET /payment` and succeeds if the node holds it pending or
selected. See [wallet.md](wallet.md).
