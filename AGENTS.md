# Notes for AI agents

## Summary

This file tells an AI coding agent how to pick up work in this repository
safely. Read [CONTRIBUTING.md](CONTRIBUTING.md), then
[docs/architecture.md](docs/architecture.md), then
[docs/roadmap.md](docs/roadmap.md) to find the next milestone. Keep the
consensus packages free of network and storage code, keep every test green,
record decisions in [docs/decisions.md](docs/decisions.md), and do not change
a byte form without updating the test vectors on purpose.

## Orientation

1. `go test ./...` must pass before you start. If it does not, fix that first
   or report it.
2. The roadmap lists milestones in order with their scope. Take the first one
   not marked done, and check open pull requests so you do not duplicate
   work.
3. The design documents are the source of truth for behavior. Where they are
   ambiguous or disagree with each other, `docs/decisions.md` records which
   reading the code follows. Add an entry for any new choice you make.

## Rules

- Do not import anything outside the Go standard library without recording
  the reason in `docs/decisions.md`.
- Do not add network, disk or clock access to `tx`, `entry`, `layer`,
  `state`, `ledger` or `engine`. Disk belongs to `store`, `node` and
  `peers` and `tracking` (their files); the network belongs to `wire`,
  `registry` and `p2p`. `tracking` and `chase` read no clock either.
  Consensus reads no clock; the ledger records first-seen times only
  through an injected clock that selection never reads.
- `go test ./pkg/vectors` failing means you changed an encoding or a hash.
  That changes every id in every series. Do it only on purpose, record it, and
  regenerate with `go test ./pkg/vectors -update`.
- Prefer extending the randomized model test in `pkg/ledger` over adding only
  hand-written cases when you touch selection, switching or pruning.
- Keep node policy in configuration with documented, provisional defaults.
  The design leaves most default values open until tests show what works.
- Use the design's terms. Do not rename concepts.
- `pkg/p2p` must never hold its own lock while calling the node, and
  node observers must never block: the lock order is node, engine, host.
  Start host goroutines with `Host.spawn`, never `wg.Add` directly, so
  none starts while `Run` waits for them.
- `pkg/wallet` is a holder's habits, never a rule: it decides nothing
  about validity, talks to a node only through `wallet.Node`, and never
  writes the seed unsealed. Do not import it from the node packages
  (`rpc` imports it only to implement `wallet.Node`).
- Never make the repository public or change its visibility; that is the
  owner's decision.

## Where things are

| Task | Start in |
| --- | --- |
| A validity rule for transactions | `pkg/tx` |
| A validity rule for entries | `pkg/entry` |
| Weight, selection, switching, top K | `pkg/ledger` |
| Pending transactions | `pkg/mempool` |
| What a full node does with an item | `pkg/engine` |
| How items are kept on disk | `pkg/store` (format), `pkg/node` (when and what) |
| A node policy setting | `pkg/nodepolicy` (an optional field with a documented default) |
| A local control endpoint | `pkg/rpc` (handler, client method, test) |
| A message between full nodes | `pkg/wire` (byte form, never reuse a type number), `docs/protocol/wire.md` |
| What a node sends, serves or asks peers | `pkg/p2p` (test it with nodes over loopback TCP) |
| What a node remembers about other nodes | `pkg/peers` (the book's rules and file, the ranking rules) |
| What a node records about a peer's behaviour, offences and points | `pkg/tracking` (a setting bit, a measure, an offence rule) |
| Which heavier ledger to check next | `pkg/chase` (the queue); the steps of a chase are in `pkg/p2p/chase.go` |
| A new policy type or key algorithm | `pkg/policy` (register it; never reuse a number) |
| How a holder chooses coins, derives keys, pays or follows a payment | `pkg/wallet` (a decision for each habit), `poo wallet` in `cmd/poo/wallet.go` |
| A new series setting | `pkg/series` (changes the byte form and so the vectors) |
| The command-line tool | `cmd/poo` |
