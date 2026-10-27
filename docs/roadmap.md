# Roadmap

## Summary

The project moves toward an open release in eight milestones, each delivered
as one or more pull requests, each stacked on the one before. Milestone 1,
the consensus core with a transport-independent full-node engine,
milestone 2, storage and the node daemon with a local RPC, milestone 3,
the wire protocol with forwarding and ledger data requests, and milestone
4, peer discovery with relaying and address registries, and milestone 5,
peer categories, ranking, tracking and chasing, are done. The next
milestones add a wallet, then large-scale simulation, and finally
hardening and release. Making the repository public and choosing
a license are the owner's decisions and are part of milestone 8.

## Milestones

| # | Name | Status |
| --- | --- | --- |
| M1 | Consensus core and full-node engine | Done |
| M2 | Storage and node daemon | Done |
| M3 | Wire protocol and forwarding | Done |
| M4 | Peer discovery, relaying, address registries | Done |
| M5 | Peer categories, ranking, tracking, chasing | Done |
| M6 | Wallet and user tools | Done |
| M7 | Network simulator, fuzzing, benchmarks | Next |
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

### M4: Peer discovery, relaying, address registries (done)

- `pkg/wire`: follow requests (direct and broadcast), yes and partial yes,
  peer lists, pings and pongs, dropped policy updates, missing chunks.
- `pkg/peers`: the peer book with candidate, verified and archived states
  per mint policy, network-range limits and retry backoff, in `peers.json`.
- `pkg/registry`: a provisional address registry format.
- `pkg/p2p`: peers verified by follow requests, relaying of broadcasts with
  return paths, rate and hop limits, peer lists, pings, dropped policies,
  and a connection manager that keeps enough verified peers per policy.
- The node policy's `registries`, `advertise` and discovery limits, `poo
  init -advertise -registry`, and the peer book in `poo peers`.

Left open, as the design leaves them: registry signing and updates to the
default registry list (there is no project registry yet), and announcing
dropped policies while running.

### M5: Peer categories, ranking, tracking, chasing (done)

- `pkg/tracking`: flags per address with severities and expiries leading
  to loss of priority, timeouts and blocks (also of network ranges);
  measures per mint policy in windows; points; budgets and repeat limits;
  basic, standard, detailed and custom templates chosen per peer, group
  and default, with escalation; saved as `tracking.json`.
- `pkg/peers`: priority and backup states, slots with range and inbound
  limits, placement and reviews by points with fair comparisons and a
  margin; blocked addresses refused by the book.
- `pkg/chase`: the chase queue: stated weights grouped by branch, ordered
  by evidence and never by weight, with maximum delays, running limits,
  backoff and a known-bad list.
- `pkg/p2p`: categories for every verified peer, queues by category,
  budgets and offences in place of disconnects, reviews, unlisted
  connections for wallets, pings by category, weight requests as regular
  checks, and chases that confirm with a priority peer, find the common
  entry, check the work with provisional layers, and only then fetch the
  entries; served bytes and repeated requests count against the asker.
- The node policy's slots, chasing settings and `tracking` section; `poo
  peer`, `poo tracking`, `poo chases` and their RPC endpoints.

Left for later milestones: tuning the provisional values (M7). Sharing
what a node records with other nodes is not planned: the design keeps it
on the node that recorded it.

### M6: Wallet and user tools (done)

- `pkg/wallet`: one seed sealed with a passphrase (PBKDF2-SHA256 and
  AES-256-GCM), keys derived from it with a gap limit, a fresh address per
  payment, shared k-of-n policies, coin selection (one mint, else one
  pool, else fewest pools), one signed transaction per mint, co-signing
  through an unsigned payment file, payment requests that carry the
  policy, and following every leg: pending, selected, dropped (resend) or
  void, with from(e), known rivals and the receiver's settled rule.
- `Ledger.Rivals`, and the engine and `GET /v1/payment` returning the
  transaction and its rivals; `rpc.WalletNode` for a running node.
- `poo wallet` with every command, and light mode as a node policy that
  follows only the wallet's pools (`poo wallet follow`).

Left for later milestones: verifying only some mints of a pool (not
planned, decision D70), hardware signers (the unsigned file is the
hand-off), and a wallet over a remote node with TLS.

### M7: Network simulator, fuzzing, benchmarks

A deterministic many-node simulator to run the design's "tests to run"
(attack weight, latency, partitions), fuzz tests for every decoder, and
benchmarks to set the provisional defaults (K, limits, seen size) from data.

### M8: Hardening and open release

Security review, a stable byte form v1 declaration, release builds, a
license, and making the repository public. Both the license and the
visibility change need the owner's explicit approval.
