# Changelog

## Unreleased (0.1.0)

The first release: a full node for Proof of Ownership, with its wallet and
simulator. See [docs/roadmap.md](docs/roadmap.md) for what each milestone
built.

- **Consensus core:** series, pools, threshold policies with Ed25519,
  transactions, entries, layers and credit, pool state, the per-pool ledger
  tree with selection, switching and top-K pruning, and the pending pool.
- **Node:** an append-only, checksummed store replayed on start-up, node
  policies, and a local RPC on loopback with a token.
- **Network:** a framed binary wire protocol, forwarding, catch-up, peer
  discovery and relaying, address registries, peer categories, ranking,
  tracking and chasing.
- **Wallet:** keys from one sealed seed, claims, payments with coin
  selection, payment requests, co-signing and light mode.
- **Simulator:** many real engines over a modelled network, with load,
  partition, attack and sizing scenarios (`poo sim`).
- **Hardening:** fuzzing of every decoder, benchmarks, a security review
  ([docs/security.md](docs/security.md)), a separate cap on inbound
  connections, byte form v1 and wire version 1 declared stable, a
  supported Go toolchain, and reproducible release builds.
