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
  `state`, `ledger` or `engine`. Disk belongs to `store` and `node`.
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
| A new policy type or key algorithm | `pkg/policy` (register it; never reuse a number) |
| A new series setting | `pkg/series` (changes the byte form and so the vectors) |
| The command-line tool | `cmd/poo` |
