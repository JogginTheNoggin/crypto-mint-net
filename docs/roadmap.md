# Roadmap

## Summary

The project moves toward an open release in eight milestones, each delivered
as one or more pull requests, each stacked on the one before. Milestone 1,
the consensus core with a transport-independent full-node engine,
milestone 2, storage and the node daemon with a local RPC, and milestone 3,
the wire protocol with forwarding and ledger data requests, are done. The
next milestones add peer discovery and peer management, then a wallet, then large-scale simulation, and
finally hardening and release. Making the repository public and choosing
a license are the owner's decisions and are part of milestone 8.

## Milestones

| # | Name | Status |
| --- | --- | --- |
| M1 | Consensus core and full-node engine | Done |
| M2 | Storage and node daemon | Done |
| M3 | Wire protocol and forwarding | Done |
| M4 | Peer discovery, relaying, address registries | Next |
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

### M3: Wire protocol and forwarding (done)

- `pkg/wire`: frames, the header with the mint policy section and stated
  weight, request ids, and every message body. See
  [protocol/wire.md](protocol/wire.md).
- `pkg/p2p`: the hello with coverage, forwarding of kept items to covering
  peers (never back, never requested data) with notices after switches,
  serving extension, hash chain, entries, full ledger, layers and weight
  requests from the active ledger, and catching up: full ledger for a new
  pool, extensions on connect, on regular checks and on a higher stated
  weight, and hash chains to find a deep split.
- The node policy's `p2p` section, `poo init -p2p/-peer`, `poo peers` and
  `/v1/peers`. Tests run several nodes over loopback TCP.

Left for later milestones: peer discovery (M4), choosing which peer to ask,
checking a branch's work with provisional layers before fetching it, and
the optional full ledger with top K request (M5).

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
