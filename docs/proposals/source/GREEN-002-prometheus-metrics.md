# GREEN-002: Prometheus metrics at `GET /v1/metrics`

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/rpc` (new `metrics.go`) |
| Effort | small |
| Decided by | Grey: merged as prototyped in 3955bb9; documented in docs/modules/rpc.md |

## Summary

The local RPC serves what `/v1/status` and `/v1/peers` already report in
Prometheus text format, so an operator can graph heights, pending
transactions and peer counts with a stock Prometheus and Grafana. No new
dependency, no new counters, same token. Prototype on the branch, with
tests.

## Problem

The only way to watch a node over time is to poll `poon status` and parse
JSON. Operators expect a scrape endpoint, and the design's "tests to run"
on real networks will need numbers over time, not snapshots.

## Idea

`GET /v1/metrics`, behind the same bearer token (Prometheus supports
`authorization: credentials_file: .../rpc.token`). Families:

| Metric | Labels | From |
| --- | --- | --- |
| `poon_pool_height`, `_weight`, `_entries`, `_branches`, `_pending`, `_waiting`, `_stored_records` | series, pool | engine status, stores |
| `poon_peers_connected` | direction | host peers |
| `poon_peer_categories` | category | peer standings |
| `poon_peer_sent_total`, `poon_peer_received_total` | | host peers |
| `poon_peer_book_records` | | peer book |

Peer families appear only when p2p runs. Weight is exported as a float, and
the exact value stays in `/v1/status`. Later, if wanted: counters the node
does not keep today (items accepted and rejected by reason, offences by
rule) and a `metrics.listen` node policy setting for a separate
listener.

## Cost and risk

About 130 lines, standard library only. Read-only, built from the same
calls as `/v1/status`, so the lock order (node, engine, host) is unchanged.
A series id is 64 hex characters, so label cardinality is the number of
followed pools, which is small. It exposes nothing a token holder cannot
already read.

## Prototype

`pkg/rpc/metrics.go` and `pkg/rpc/metrics_test.go` (with and without a
p2p host, and the 401 without a token); the route and the package comment
in `pkg/rpc/rpc.go`. Not added: a client method or a `poon metrics`
command, since a scraper is the user.
