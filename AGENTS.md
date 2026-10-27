# Notes for AI agents

## Summary

This file tells an AI coding agent how to pick up work in this repository
safely. First find your team in [Teams](#teams-read-first): Source or
GUI, and Grey, Red or Blue. Then read [CONTRIBUTING.md](CONTRIBUTING.md), then
[docs/architecture.md](docs/architecture.md), then
[docs/roadmap.md](docs/roadmap.md) to find the next milestone. Keep the
consensus packages free of network and storage code, keep every test green,
record decisions in [docs/decisions.md](docs/decisions.md), and do not change
a byte form without updating the test vectors on purpose.

## Teams (read first)

The project's owner splits the work into two layers, each with four
teams. Every agent belongs to exactly one layer and one colour, given
when its thread is created (for example "Source/Red"). Stay in your lane.

| Layer | Covers |
| --- | --- |
| Source | The headless node and tools: everything outside `gui/` |
| GUI | The graphical layer: the `gui/` module, which only calls `pkg/cli` and `pkg/rpc` |

The layers stay separate: source teams do not change `gui/`, GUI teams do
not change anything outside `gui/`. A GUI finding about the source goes to
the source teams as a finding, not as a patch.

| Colour | Does | Works on |
| --- | --- | --- |
| Grey | Builds features and general mitigation and hardening, owns the integration line, merges Blue's fixes, decides on Green's proposals | The integration line (below) |
| Red | Breaks things: finds exploits from outside (peers, the wire, registries, anything a remote party sends) and inside (local RPC, files, wallet, CLI, resource limits), writes findings and suggested fixes. Never implements them | Reads the integration line; writes only findings, on its own branch |
| Blue | Implements fixes for Red's findings, with tests | Its own branch, started from the integration line |
| Green | Explores: quality-of-life improvements, nice-to-haves and new ideas, as written proposals with an optional prototype. Grey decides whether to adopt each one | `claude/<layer>-green` (or `claude/<layer>-green-<topic>`), started from the integration line |

### The integration line

The owner calls it `main`. While the milestone pull requests wait for the
owner to merge them, `main` holds only the first commit, so the
integration line is the newest Grey branch. `security/<layer>/BASE.md`
names it with its commit. Red attacks that commit, and Blue branches from
it. Once the stack is merged, `main` is the integration line again.

### The loop

1. **Red** attacks the base and writes each finding to
   `security/<layer>/findings/` (format below) on a branch
   `claude/<layer>-red-<topic>`, adds it to the index there, and tells Blue
   and Grey which findings are new. Red reports, it does not patch. Red
   tests only against nodes it runs itself (loopback, temporary
   directories), never against anyone else's machine or network.
2. **Blue** picks findings, fixes them on `claude/<layer>-blue-<topic>`
   branched from the base, adds a regression test for each, writes a note
   to `security/<layer>/fixes/`, and tells Grey the branch and commit.
3. **Grey** reviews Blue's branch, merges it into the newest Grey branch,
   runs every check (`gofmt -l .`, `go vet ./...`, `go test -race ./...`,
   `staticcheck ./...`, `govulncheck ./...`, the fuzz targets), and checks
   that existing behaviour still works. Grey then updates `BASE.md` and
   the findings' status, and tells Blue and Red the new base. That commit
   is Blue's next starting point.

### Green proposals

Green runs beside the loop, not inside it. Green writes each idea to
`docs/proposals/<layer>/GREEN-<NNN>-<short-slug>.md` on its own branch,
optionally with a prototype on the same branch, and tells Grey the branch,
commit and proposal ids. A proposal starts with this table, then the
sections Summary, Problem, Idea, Cost and risk, and Prototype (what the
branch holds, if anything).

| Field | Value |
| --- | --- |
| Status | proposed, adopted, adopted in part, deferred, or declined |
| Based on | branch @ commit |
| Area | package, command or screen |
| Effort | small, medium or large |
| Decided by | Grey, with the merge commit or the reason |

Grey reads each proposal and decides. Grey may merge Green's prototype,
rewrite it, take part of it, defer it, or decline it, and records the
outcome in the proposal's Status and Decided by fields. Nothing reaches
the integration line except through Grey, and Green never changes
consensus rules or another team's files. A Green idea that is really a
security problem goes to Red as a note, not into a proposal.

Threads talk through these files and through thread messages that name
a branch, a commit and finding ids. Decisions still go in
`docs/decisions.md`: take the next free number, and check the newest
branches first so two teams do not take the same one.

### Finding format

One file per finding: `security/<layer>/findings/RED-<NNN>-<short-slug>.md`,
numbered in order, never renumbered. It starts with this table, then the
sections.

```markdown
# RED-007: Short title

| Field | Value |
| --- | --- |
| Severity | critical, high, medium, low or info |
| Status | open, fixing, fixed, won't fix, or not a bug |
| Found on | branch @ commit |
| Component | package or file, e.g. pkg/p2p/discovery.go |
| Surface | peer, wire, registry, local RPC, files, wallet, CLI, or build |
| Fixed by | Blue branch @ commit, then Grey merge commit (once fixed) |

## Summary
## Reproduction   (exact steps, or a test that fails today)
## Impact         (what an attacker gains, and what it costs them)
## Recommendation (the suggested fix and how to test it)
```

Severity: **critical** means remote theft or forging value, or a remote
crash of every node. **High** is a remote crash, eclipse or denial of
service at low cost, or a local secret leaking. **Medium** costs more or
needs a position (a peer slot, local access). **Low** is a hardening gap.
**Info** is an observation. `security/<layer>/findings/README.md` keeps
the index (id, title, severity, status).

A fix note is `security/<layer>/fixes/BLUE-<NNN>.md`: the findings it
fixes, what changed, the tests added, and how Grey can check it.

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
  That changes every id in every series. Byte form v1 is frozen (D76): do
  not edit it. A new form is a new format version beside v1, with its own
  vectors; `TestByteFormV1Frozen` fails otherwise, whatever `-update` does.
- Prefer extending the randomized model test in `pkg/ledger` over adding only
  hand-written cases when you touch selection, switching or pruning.
- Keep node policy in configuration with documented, provisional defaults.
  The design leaves most default values open until tests show what works.
- Use the design's terms. Do not rename concepts.
- `pkg/p2p` must never hold its own lock while calling the node, and
  node observers must never block: the lock order is node, engine, host.
  Start host goroutines with `Host.spawn`, never `wg.Add` directly, so
  none starts while `Run` waits for them.
- `pkg/sim` must stay deterministic: every random choice comes from the
  run's seed, never the clock or map order. When it finds a bug, fix the
  package at fault, add a unit test there, and keep the seed's scenario
  passing.
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
| How a holder chooses coins, derives keys, pays or follows a payment | `pkg/wallet` (a decision for each habit), `poo wallet` in `pkg/cli/wallet.go` |
| A network experiment, or one of the design's "tests to run" | `pkg/sim` (a scenario returning a report), `poo sim`, and its numbers in `docs/simulation.md` |
| A new series setting | `pkg/series` (changes the byte form and so the vectors) |
| The command-line tool | `pkg/cli` (every command; the GUI runs it too), `cmd/poo` (the process around it) |
