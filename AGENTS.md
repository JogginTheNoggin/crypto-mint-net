# Notes for AI agents

## Summary

This file tells an AI coding agent how to pick up work in this repository
safely. First find your team in [Teams](#teams-read-first): Source or
GUI, and Grey, Red, Blue, Green, Orange or Black. Then read
[CONTRIBUTING.md](CONTRIBUTING.md), then
[docs/architecture.md](docs/architecture.md), then
[docs/roadmap.md](docs/roadmap.md) to find the next milestone. Keep the
consensus packages free of network and storage code, keep every test green,
record decisions in [docs/decisions.md](docs/decisions.md), and do not change
a byte form without updating the test vectors on purpose. No change may
add or alter a consensus rule without the owner's approval, or make a hard
fork possible through any setting; Black checks this at the end of every
round.

## Teams (read first)

The project's owner splits the work into two layers, each with six
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
| Grey | Builds features and general mitigation and hardening, owns the integration line, merges Blue's fixes, decides on Green's and Orange's proposals | The integration line (below) |
| Red | Breaks things: finds exploits from outside (peers, the wire, registries, anything a remote party sends) and inside (local RPC, files, wallet, CLI, resource limits), writes findings and suggested fixes. Never implements them | Reads the integration line; writes only findings, on its own branch |
| Blue | Implements fixes for Red's findings, with tests | Its own branch, started from the integration line |
| Orange | Performance and optimization: measures where time, memory, allocations, disk and network go, finds slow paths and waste, and proposes improvements backed by benchmarks or profiles, with an optional prototype. May challenge a feature's modular form where measurements show it really hurts performance (see Orange proposals). Grey decides whether to adopt each one | `claude/<layer>-orange` (or `claude/<layer>-orange-<topic>`), started from the integration line |
| Green | Explores: quality-of-life improvements, nice-to-haves and new ideas, as written proposals with an optional prototype. Grey decides whether to adopt each one | `claude/<layer>-green` (or `claude/<layer>-green-<topic>`), started from the integration line |
| Black | Guards consensus (added 2026-10-31): checks that no consensus rule is broken or changed, filters out any suggestion that adds or changes a consensus rule without the owner's approval, and checks every change extensively for a way to make a hard fork possible, including through node policy. Never implements fixes | Reads the integration line, Blue's branches and the proposals; writes only reviews under `docs/consensus/<layer>/`, on `claude/<layer>-black` |

### Rounds

Each layer (Source, GUI) runs its own rounds, independently of the
other. A round has four parts, in order, and no part starts before the
previous one is finished:

| Part | Who works | Does | Hands to |
| --- | --- | --- | --- |
| A | Grey | Changes the integration line: its own work (from the owner or itself), the adopted Green and Orange proposals and the Blue fixes from the last round. Updates `BASE.md` | Red, Green and Orange, by naming the new base |
| B | Red, Green and Orange, side by side | Start from Grey's new base. Red explores security and writes findings; Green explores creative ideas, loose ends, mismatches and inconsistencies and writes proposals; Orange measures performance and writes proposals with the measurements | Red's findings go to Blue; Green's and Orange's proposals go to Black in Part D, then to Grey for a later round's Part A |
| C | Blue | Fixes Red's findings on its branch | Black |
| D | Black | Reviews the round's changes: Grey's new base, Blue's fixes and the Green and Orange proposals. Flags any consensus change and any hard-fork risk, and filters out (marks *blocked*) every item that would add or change a consensus rule without the owner's approval | Grey, for the next round's Part A, and the owner for anything that needs approval |

The team working a part decides when that part is finished. When it is,
it stops making edits and reports it (for example "Source Round 2 Part
A done"). Teams outside the current part do not edit until the round
reaches them again. Within a round the parts follow on by themselves:
once a part is reported done, the coordinator starts the next one. Only
a new round waits for the owner, who is told when each round is complete
and decides when the next one starts, for Source and GUI separately. The
owner may also "stop at" a part, which means that part is not started
until the owner says so; only an explicit "interrupt" or "stop now"
interrupts work in progress.

### Black reviews (Part D)

Black is the last part of every round. It starts from the commits the
round produced: Grey's base in `BASE.md`, each Blue branch waiting to be
merged, and each Green and Orange proposal (with its prototype, if any).
It writes one review per round,
`docs/consensus/<layer>/ROUND-<N>.md`, following the checklist in
`docs/consensus/<layer>/README.md`, and puts each item in one of these
verdicts:

| Verdict | Meaning | What Grey does in Part A |
| --- | --- | --- |
| clear | Changes no validity, weight, selection or byte-form rule, and no setting can make a heavier valid ledger unacceptable | May adopt or merge as usual |
| clear with conditions | Clear only if the stated conditions hold (for example, "keep each entry's counted credit") | Adopts only with the conditions, and records them in the decision |
| blocked: consensus | Adds or changes a consensus rule that is not in the design documents or the owner's approvals | Must not adopt it. If Black pre-approved the suggestion it waits for the owner's decision; if Black filtered it out it is dropped |
| blocked: hard fork | Some rule, setting or input could make nodes that hold the same ledgers disagree on which is valid or heavier, so a node refuses for good a heavier valid ledger it has been given (a depth limit, a size limit every peer would hit, a permanent refusal). A lack of information or slowness is not a hard fork (definition below) | Must not adopt it as is. Black names the failing case and the rule it breaks |

**What a hard fork is** (the owner, 2026-11-03). A hard fork is when
nodes that have the same information cannot decide universally which
ledger is valid, or which is heavier. Any set of nodes, given any set of
ledgers, must pick the same one. A voluntary or involuntary lack of
information is not a hard fork, and neither is a node being slow. So a
setting that only leaves a node uninformed or slow (it never asks, it
waits, its peers stay quiet) is not *blocked: hard fork*, though it may
still be worth fixing as node policy. A rule, setting or input that makes
nodes holding the same ledgers disagree on validity or weight is.

**Consensus suggestions go to Black first** (the owner, 2026-10-31).
Any team that wants to add or change a consensus rule, in a finding, a
fix, a proposal or a decision, writes the suggestion and sends it to
Black, never straight to the owner. Black decides whether it is worth the
owner's time:

- **Pre-approved**: the change is needed (no node-policy way gets the
  same result), it keeps hard forks impossible, and it fits the design
  documents' core ideas. Black passes it to the owner in plain words,
  with the case for it and the alternatives it ruled out. Only the owner
  gives final approval.
- **Filtered out**: a node-policy shape would do, it risks a hard fork,
  it repeats something the owner already refused, or it is not worth it.
  Black records why in its review and tells the sending team; it does
  not reach the owner.

Black's pre-approval is never the owner's approval: nothing is built
until the owner says yes. Black runs this gate in Part D for the round's
suggestions; a suggestion made outside a round waits for the next Part D.

A blocked item stays blocked until the owner approves it or a later
change removes the problem and Black clears it. Black never edits code or
another team's files, and never decides for the owner: it only flags,
filters and explains. Its baseline is
[docs/consensus-audit.md](docs/consensus-audit.md) and the owner's
decisions recorded there. When a Blue fix itself would be blocked (a fix
for a Red finding that adds a limit, for example), Black says so and
suggests a node-policy shape that keeps the rule; Blue reworks it in the
next round's Part C, or Grey in Part A, as the owner directs.

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

### Orange proposals

Orange works in Part B like Green, and its proposals go to Grey the same
way, in the same format, as
`docs/proposals/<layer>/ORANGE-<NNN>-<short-slug>.md`, listed in the same
index. An Orange proposal adds a **Measurements** section after Problem:
what was measured, how (the `go test -bench` or `pprof` command, the
`poo sim` scenario, the machine), and the numbers before and, with a
prototype, after. A claim without a measurement is not a proposal yet.
Orange keeps every behaviour and every test result the same: a change
that would alter what the node accepts, selects or sends is a design
question for Grey, not an optimization. Orange benchmarks only against
nodes and simulations it runs itself, like Red.

Features are modular and extensible by default: each lives in its own
package behind a small API, with thin CLI, RPC and GUI layers over it
([docs/FEATURES.md](docs/FEATURES.md)). Where it applies, Orange may
challenge that for a specific piece or feature when measurements show it
really hurts performance (the owner's rule, 2026-11-02). Orange makes
the case in its Part B proposal, with the measurements, what the
modular form costs, and what a tighter form would save and give up.
Grey decides, as for any Orange proposal. This applies to both Orange
teams. It never covers consensus rules, nor the rule that the GUI runs
exactly the headless code.

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

- **No new consensus rules, and no hard forks through node policy**
  (the owner, 2026-10-31). Do not add a rule that decides whether a
  ledger, entry, layer or transaction is valid or selected unless it is in
  the design documents or the owner has approved it. Send any you
  propose to Black, who pre-approves it for the owner or filters it out
  ([Black reviews](#black-reviews-part-d)); do not ask the owner
  directly. Prefer node policy, but
  node policy must never make a heavier valid ledger unacceptable: if any
  reachable peer serves a valid ledger heavier than the selected one, the
  node takes it in and selects it in the end, whatever its settings. A
  setting may delay, pace, order, choose peers or ask for work first. It
  may never refuse such a ledger for good, and it may never be a depth
  limit or a size limit that every peer would hit. The `pkg/sim` scenario
  for this (consensus-audit P0) must keep passing. What counts as a hard
  fork is the owner's definition under
  [Black reviews](#black-reviews-part-d): nodes with the same information
  that cannot agree; a lack of information is not one.
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
| A consensus review, or whether a change is consensus | `docs/consensus/<layer>/` (Black's checklist and reviews), `docs/consensus-audit.md` (the baseline) |
| The command-line tool | `pkg/cli` (every command; the GUI runs it too), `cmd/poo` (the process around it) |
