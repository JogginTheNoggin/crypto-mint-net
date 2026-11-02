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
transport-independent full-node engine. A command-line tool runs four
in-process full nodes through a payment, a double spend and its resolution.
Networking, storage and peer management come in later milestones
([roadmap](docs/roadmap.md)). Every design decision and every place this code
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

You need Go 1.24 or later.

```sh
git clone https://github.com/JogginTheNoggin/crypto-mint-net
cd crypto-mint-net
go test ./...            # run every test
go run ./cmd/poo demo    # four in-process full nodes: payment, double spend, recovery
go run ./cmd/poo keygen  # a key and its 1-of-1 address
go run ./cmd/poo series example > series.json
go run ./cmd/poo series info series.json
```

## Repository layout

| Path | What it holds |
| --- | --- |
| `cmd/poo` | The command-line tool and the in-process demo |
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
| `pkg/vectors` | Published test vectors for independent implementations |
| `pkg/testutil` | Deterministic keys and builders for tests and the demo |
| `docs/` | Architecture, roadmap, decisions, protocol formats, module guides |

## Documentation

- [Architecture](docs/architecture.md): how the code is organized and why.
- [Roadmap](docs/roadmap.md): milestones from here to an open release.
- [Decisions](docs/decisions.md): design decisions and deviations from the design documents.
- [Byte formats](docs/protocol/encoding.md): exact encodings and test vectors.
- [Glossary](docs/glossary.md): the terms used throughout.
- [Module guides](docs/modules/README.md): one page per package.
- [Contributing](CONTRIBUTING.md) and [notes for AI agents](AGENTS.md).

## Status

Pre-release. The consensus core is implemented and tested; the node does not
yet talk to other nodes over a network or store data on disk. Do not use it
with anything of value. Keys made by the test helpers are derived from public
labels and must never hold real funds.

## License

Not yet chosen. Until a license is added, all rights are reserved by the
repository owner.
