# Roadmap

## Summary

The project moves toward an open release in nine milestones, each delivered
as one or more pull requests, each stacked on the one before. Milestone 1,
the consensus core with a transport-independent full-node engine,
milestone 2, storage and the node daemon with a local RPC, milestone 3,
the wire protocol with forwarding and ledger data requests, and milestone
4, peer discovery with relaying and address registries, and milestone 5,
peer categories, ranking, tracking and chasing, milestone 6, the wallet,
and milestone 7, the simulator with fuzzing and benchmarks, are done.
Milestone 8 has its security review, stable byte form, release builds and
changelog in place. What is left of it is the owner's: choosing a license,
tagging the first release, and making the repository public. Milestone 9
lets each mint choose its own hash, signature algorithms and address
policy types from a vetted list.

## Milestones

| # | Name | Status |
| --- | --- | --- |
| M1 | Consensus core and full-node engine | Done |
| M2 | Storage and node daemon | Done |
| M3 | Wire protocol and forwarding | Done |
| M4 | Peer discovery, relaying, address registries | Done |
| M5 | Peer categories, ranking, tracking, chasing | Done |
| M6 | Wallet and user tools | Done |
| M7 | Network simulator, fuzzing, benchmarks | Done |
| M8 | Hardening and open release | In progress (owner steps left) |
| M9 | Cryptographic choice for mints | Done |

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

### M7: Network simulator, fuzzing, benchmarks (done)

- `pkg/sim`: real engines over a simulated network in virtual time, with
  latency, partitions and heals, flooding, fetching, notices and stated
  weights; scenarios for load, partitions, attacks with a head start and
  the sizing sweep; `poo sim`. Results in
  [simulation.md](simulation.md).
- A bug it found, fixed: a pending payment waiting on a carried parent
  could stay waiting forever (decision D72).
- Fuzz targets for every decoder, run in CI (`scripts/fuzz.sh`).
- Benchmarks for transactions, layer hashing, the ledger and the store.
- The provisional defaults are kept (decision D74). The measurement that
  seemed to favour K = 8 turned out, at milestone 8, to be a pruning bug
  (decision D79).

Left for later: the remaining "tests to run" that need more workload
models (uneven activity, one stockpile and several spends, spam and
sibling flooding, light followers with invalid unchecked mints, proof
size, settlement time with services), simulating peer ranking and
chasing, and runs on real series parameters.

### M8: Hardening and open release

Done:

- A security review ([security.md](security.md)): threat model, checks,
  and three fixes. Inbound connections get their own cap (D75), the
  toolchain moves to a supported Go release (D77), and static-analysis
  findings are fixed.
- Byte form v1 and wire version 1 declared stable, with a test that the
  published vectors never change (D76).
- Reproducible release builds for seven platforms, a tag workflow that
  opens a draft release, and [releasing.md](releasing.md) (D78).
- [SECURITY.md](../SECURITY.md) and [CHANGELOG.md](../CHANGELOG.md).
- A full re-run on the release candidate: the race-detected test suite
  three times, every fuzz target for 2 minutes, and every simulation at
  16 to 128 nodes. It found a bug in pruning that could keep a node on a
  lighter ledger after a partition, now fixed (decision D79).

Left, each needing the owner's explicit approval:

- A license, then a `LICENSE` file, which release archives pick up.
- Enabling private vulnerability reporting, which SECURITY.md points to.
- Tagging `v0.1.0` and publishing the draft release.
- Making the repository public.

### M9: Cryptographic choice for mints (done)

A series chooses from five hashes (SHA-256, SHA-512/256, SHA3-256,
BLAKE2b-256, BLAKE3; D80), nine signature algorithms (Ed25519, ECDSA over
P-256, P-384, P-521 and secp256k1, BIP-340 Schnorr, ML-DSA-44, -65 and -87;
D81) and three address types (threshold, hashlock, nested k-of-policies;
D82). Keys of every algorithm derive from a seed, wallets pick their
algorithm at creation, and `poo crypto`, `poo keygen -alg -hash` and
`poo series new` expose the choice. Known-answer tests, end-to-end wallet
tests for every algorithm, and fuzz targets for every verifier and the new
types cover it. [cryptography.md](cryptography.md) lists every option and
those advised against (RSA, SLH-DSA, Ed448, Falcon, a lone hashlock).

Left for later: timelocks, which need a clock in consensus first, and
wallet support for spending from hashlock and k-of-policies addresses.
