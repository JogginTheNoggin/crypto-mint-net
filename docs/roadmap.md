# Roadmap

## Summary

The project moves toward an open release in eight milestones, each delivered
as one or more pull requests, each stacked on the one before. Milestone 1,
the consensus core with a transport-independent full-node engine, and
milestone 2, storage and the node daemon with a local RPC, are done. The
next milestones add the wire protocol and forwarding, then peer discovery
and peer management, then a wallet, then large-scale simulation, and
finally hardening and release. Making the repository public and choosing
a license are the owner's decisions and are part of milestone 8.

## Milestones

| # | Name | Status |
| --- | --- | --- |
| M1 | Consensus core and full-node engine | Done |
| M2 | Storage and node daemon | Done |
| M3 | Wire protocol and forwarding | Next |
| M4 | Peer discovery, relaying, address registries | Planned |
| M5 | Peer categories, ranking, tracking, chasing | Planned |
| M6 | Wallet and user tools | Planned |
| M7 | Network simulator, fuzzing, benchmarks | Planned |
| M8 | Hardening and open release | Planned |

### M1: Consensus core and full-node engine (done)

Series, pools, addresses and witnesses (threshold policy, Ed25519),
transactions and their validity, entries and canonical order, layers and
credit, pool state with undo and overlays, the per-pool ledger tree with
selection, switching, displaced transactions and top K pruning, the pending
pool, the engine, published test vectors, and the `poo` CLI with a 4-node
demo. Tests reproduce the design's worked example (Appendix A) and compare
the ledger against a brute-force model on random histories.

### M2: Storage and node daemon (done)

- `pkg/store`: an append-only, checksummed log per pool of entries, layers
  and pending transactions, replayed through the engine on start-up, with
  crash-safe tail cutting and atomic compaction.
- `pkg/nodepolicy`: the node policy file (series, issuers, follows, K and
  limits, durability, RPC) and the design's save, archive and delete
  operations for node policies.
- `pkg/node`: the engine with a data directory, a lock and a store per pool.
- `pkg/rpc`: a local JSON-over-HTTP interface on loopback with a token.
- `poo init`, `poo node` and client commands (status, claim, pay, build,
  work, payment, holdings, policy). See
  [running-a-node.md](running-a-node.md).

Left open, as the design leaves them: expiry of entries and layers by age
(D29), and witness pruning.

### M3: Wire protocol and forwarding

- Message framing over TCP, versioned, using the byte forms of
  [protocol/encoding.md](protocol/encoding.md).
- Pushing new items and forwarding valid ones once, using the seen list.
- Ledger data requests: ask a peer for entries and layers by hash or range,
  in chunks, never forwarding what was requested.
- Notices of stated weight, and chasing a heavier ledger a peer claims.

### M4: Peer discovery, relaying, address registries

Bootstrapping, exchanging peer addresses, relaying for nodes that cannot
accept connections, and registries that map addresses to reachable nodes.

### M5: Peer categories, ranking, tracking, chasing

Peer categories and ranking from observed behavior, tracking what each peer
has, flags for misbehavior, and the chasing rules for following stated
weight without being misled.

### M6: Wallet and user tools

Key storage, address and policy creation, building and signing payments,
watching for confirmation weight, and a light-client mode derived from the
full node.

### M7: Network simulator, fuzzing, benchmarks

A deterministic many-node simulator to run the design's "tests to run"
(attack weight, latency, partitions), fuzz tests for every decoder, and
benchmarks to set the provisional defaults (K, limits, seen size) from data.

### M8: Hardening and open release

Security review, a stable byte form v1 declaration, release builds, a
license, and making the repository public. Both the license and the
visibility change need the owner's explicit approval.
