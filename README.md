# crypto-mint-net

A full node for **Proof of Ownership (PoO)**: a consensus mechanism for
decentralized, high-throughput payments in which ownership of a minted unit is
held by the cumulative work attached to the history that records it, not by
votes, block rewards or elapsed time.

## Summary

This repository is the reference implementation of the PoO full node, written
in Go with no dependencies outside the standard library. Milestone 1, the
consensus core, is complete: series and pools, addresses and witnesses,
transactions, entries, layers, pool state, the per-pool ledger tree with
heaviest-ledger selection and switching, pending transactions, and a
transport-independent full-node engine. Milestone 2 adds the node daemon:
a crash-safe store per pool that is replayed and checked again on start-up,
node policy files with save, archive and delete, and a local RPC with
command-line clients. Milestone 3 connects nodes: a wire protocol over TCP
that forwards what each node keeps, answers ledger data requests, and lets
a node catch up with a heavier ledger. Milestone 4 lets nodes find each
other: follow requests checked against each peer, relayed discovery, peer
lists, address registries and a saved peer book. Milestone 5 manages
peers: priority, backup and candidate peers ranked by points from what
each one does, flags, timeouts and blocks for misbehaviour, and chases of
heavier ledgers that check the work behind a branch before fetching it.
Milestone 6 adds the wallet: keys from one sealed seed, a fresh address
per payment, shared k-of-n policies with co-signing, payment requests
that carry the receiver's policy, and following every payment's weight
against known rivals (`poo wallet`). Milestone 7 adds a deterministic
network simulator, fuzzing and benchmarks: see [docs/simulation.md](docs/simulation.md) for
what they measured. Milestone 8 hardens it for release: a security review
([docs/security.md](docs/security.md)), byte form v1 declared stable, and
reproducible release builds ([docs/releasing.md](docs/releasing.md)). The
license and the open release are next ([roadmap](docs/roadmap.md)). Every design decision and every place this code
departs from the design documents is recorded in
[docs/decisions.md](docs/decisions.md).

## The design in brief

- A **series** is a published set of rules: a number of mints, a fixed supply
  of units per mint, a pool size, a hash, strength minimums and the allowed
  address types.
- The mints of a series are grouped into **pools**. All mints of a pool share
  one **ledger**: a chain of **entries**, each holding a bundle of
  transactions and the hash of the entry before it.
- A **transaction** belongs to one mint, spends whole **portions** of it and
  creates new ones adding up to the same amount. Its owners authorize it with
  **witnesses**, which sit outside its id (the **tid**).
- Work is added separately, as **layers**: unsigned hashes of an entry hash
  and a nonce. A layer's **credit** is 2 to the power of its leading zero
  bits.
- A node **selects** the valid ledger of each pool with the greatest total
  credit. A rival must out-weigh everything built after the last entry it
  shares with the selected ledger.
- Which issuers, series, pools and mints a node follows, and how much work a
  receiver waits for, are each node's own choice (**node policy**).

The full design lives in the project's design documents; the
[architecture](docs/architecture.md) maps each part of it to code.

## Quick start

You need Go 1.27 or later (the `go` command fetches the toolchain `go.mod` names).

```sh
git clone https://github.com/JogginTheNoggin/crypto-mint-net
cd crypto-mint-net
go test ./...            # run every test
go run ./cmd/poo demo    # four in-process full nodes: payment, double spend, recovery
go run ./cmd/poo keygen  # a key and its 1-of-1 address
go run ./cmd/poo series example > series.json
go run ./cmd/poo series info series.json
```

To run a node with a store and a local RPC, and pay with it, see
[docs/running-a-node.md](docs/running-a-node.md):

```sh
go build -o poo ./cmd/poo
./poo init -example      # a node policy following the example series
./poo node               # in another terminal: ./poo status
```

## Repository layout

| Path | What it holds |
| --- | --- |
| `pkg/cli`, `cmd/poo` | The `poo` command (node daemon, client commands, in-process demo) as a library, and the few lines of process around it |
| `pkg/encoding` | The canonical byte form every hash is computed over |
| `pkg/hash` | 32-byte hashes, the hash number list, domain tags, strength |
| `pkg/weight` | Arbitrary-precision credit and weight |
| `pkg/series` | Series rules, series_id, pools and pool_ids, issuer witness |
| `pkg/policy` | Addresses, policies, witnesses, policy types and key algorithms |
| `pkg/tx` | Transactions, tids and the transaction validity rules |
| `pkg/entry` | Entries, canonical order, entry hashes, entry validity |
| `pkg/layer` | Layers and grinding |
| `pkg/state` | Pool state (unspent portions) with undo and overlays |
| `pkg/ledger` | One pool's tree of entries, selection, switching, top K |
| `pkg/mempool` | Pending transactions of a pool |
| `pkg/engine` | The transport-independent full-node core |
| `pkg/store` | The append-only, checksummed log of one pool |
| `pkg/nodepolicy` | The node policy file and saved/archived node policies |
| `pkg/node` | The engine with a data directory and a store per pool |
| `pkg/wire` | The byte form of messages between full nodes |
| `pkg/peers` | The peer book and ranking: known addresses, categories, slots, network ranges |
| `pkg/tracking` | What each peer does: flags, timeouts, blocks, measures, points, templates |
| `pkg/chase` | The chase queue: heavier stated weights by evidence, backoff, known-bad hashes |
| `pkg/registry` | Address registries: bootstrap lists of nodes |
| `pkg/p2p` | Peer connections: discovery, ranking, forwarding, ledger data requests, chasing |
| `pkg/rpc` | The local control interface (JSON over HTTP, loopback, token) |
| `pkg/wallet` | Keys from one sealed seed, payments, requests, co-signing, following weight |
| `pkg/sim` | A deterministic many-node simulator over real engines |
| `pkg/vectors` | Published test vectors for independent implementations |
| `pkg/testutil` | Deterministic keys and builders for tests and the demo |
| `docs/` | Architecture, roadmap, decisions, protocol formats, module guides |

## Documentation

- [Running a node](docs/running-a-node.md): start a node, pay, restart.
- [Architecture](docs/architecture.md): how the code is organized and why.
- [Roadmap](docs/roadmap.md): milestones from here to an open release.
- [Decisions](docs/decisions.md): design decisions and deviations from the design documents.
- [Byte formats](docs/protocol/encoding.md): exact encodings and test vectors.
- [Wire protocol](docs/protocol/wire.md): messages between full nodes.
- [Glossary](docs/glossary.md): the terms used throughout.
- [Cryptography](docs/cryptography.md): the hashes, signature schemes and address types a mint can choose from, the ones advised against, and where each is used.
- [Security review](docs/security.md) and [security policy](SECURITY.md).
- [Releasing](docs/releasing.md): how releases are built and checked.
- [Changelog](CHANGELOG.md).
- [Module guides](docs/modules/README.md): one page per package.
- [Contributing](CONTRIBUTING.md) and [notes for AI agents](AGENTS.md).

## Status

Pre-release. The consensus core, a node with storage and a local RPC, and
the wire protocol between full nodes (forwarding, ledger data requests and
catch-up), peer discovery, and peer ranking, tracking and chasing are
implemented and tested, with a wallet. Every decoder is fuzzed, the hot
paths are benchmarked, and a deterministic network simulator runs the
design's tests ([docs/simulation.md](docs/simulation.md)); its numbers
support the provisional defaults but come from a model, not a real
network. A security review has been done by the project itself
([docs/security.md](docs/security.md)) but not yet by anyone outside it.
Do not use it
with anything of value. Keys made by the test helpers are derived from public
labels and must never hold real funds.

## License

Not yet chosen. Until a license is added, all rights are reserved by the
repository owner.
