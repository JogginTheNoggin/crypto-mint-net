# Source Round 32: consensus review (Black, Part D)

## Summary

This round is about node files, status and release readiness. Part A
(D184) does three things:
- it caps the unused-snapshot reasons in status (GUI ORANGE-023);
- it keeps settings in `node.json` that this release doesn't know (W-1);
- it puts the release in status (W-10).

Blue's D185 fixes Red's RED-093 to RED-096 in the kept-settings code. It
carries GREEN-057 and ORANGE-042, and makes the variable series id set a
`sync.Map` (GUI ORANGE-024). White's charter added WHITE-001 (the first
public release plan) and WHITE-002 (nodes share their version and
features). The coordinator asked Black to confirm that WHITE-002 step 1
is not a consensus change.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - No rule, byte form or wire message changes.
  - `node.json`, status and doctor are node policy and display.
- **The variable series set holds the same ids, added at the same
  point.** It decides how a mint's bytes are read (`tx.go`), so Black
  checked it directly:
  - `addVariableID` is still called only from `series.NewWithRegistry`;
  - `IsVariableID` is still read only from `tx` decoding;
  - the add stores the id before it sets the flag, so a reader that sees
    the flag also sees the id;
  - it passes under `-race`, with Blue's concurrent add-and-read test.
- **WHITE-002 step 1 (`poon/VERSION` in the hello) is not a consensus
  change.** `Agent` is free text that nothing decides on: no check, no
  peer score, no chase and no ledger reads it. Step 2 is clear on the
  terms below.
- **Fixed series, transactions and entries are unchanged.** The Round 24
  and Round 25 cross-version tests are byte for byte the same on `main`
  and on PR #251.
- **For the owner, about WHITE-001.** A public alpha tag counts as a
  release for the purpose of Round 27's warning. After it, any loosening
  of a witness limit, or any other change to what is valid, needs new
  algorithm or type numbers. WHITE-001 already says the consensus byte
  form freezes at alpha, so the two agree.

| Field | Value |
| --- | --- |
| Round | Source Round 32 |
| Consensus | None (D184, D185) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `6260590` (PR #245: D184, with Round 31's PRs and White's charter #241 merged) |
| Previous base | `main` @ `4c0f708` |
| Blue branches | PR #251 @ `378d4cb` (BLUE-033, D185: RED-093 to RED-096; carries GREEN-057 #247 and ORANGE-042 #249) |
| Red | PR #246 @ `10ec223` (RED-093 to RED-096) |
| Proposals | GREEN-057 (PR #247 @ `20dcd22`); ORANGE-042 (PR #249 @ `e16c9cd`); WHITE-001 and WHITE-002 (in `docs/white/source/`, merged with #241) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## The variable series set (D185, GUI ORANGE-024)

`series.IsVariableID` tells `tx` decoding whether a mint names a variable
series, which changes how its bytes are read (D166). The set is now a
`sync.Map` with an atomic "any" flag, in place of a map copied whole on
each add.

- **Same members.** `addVariableID(s.id)` is called only in `series.NewWithRegistry`,
  for a series whose params say variable, as before. Nothing removes an
  id, before or after.
- **Same moment.** A series is built before any of its transactions is
  decoded, and that order is unchanged. `LoadOrStore` comes before
  `variableAny.Store(true)`. A reader that loads the flag as true then
  finds the id, by Go's memory model for `sync/atomic` and `sync.Map`.
- **The empty fast path.** A node with no variable series reads `false`
  without a lookup, as with the old nil map.
- **Tested.** Blue's `TestBlue033VariableSetGrowsInPlace` adds 4,096 ids
  from four goroutines while reading, then checks every id and one that
  was never added. It passes under `-race`. The cross-version tests
  decode the same fixed series, transactions and entries to the same
  bytes and verdicts.

## Kept settings in node.json (D184 W-1, D185)

A node now keeps the `node.json` settings it doesn't know, and writes
them back on save.

- **This release ignores them, as before.** A kept setting has no field,
  so nothing in this release reads it. Before D184 it was dropped on
  save; now it survives for the release that knows it.
- **What runs is what the decoder applies (RED-093).** A key that
  `encoding/json` would apply, exactly or by case folding, is treated as
  known and saved under its own name. So the node runs with what `Load`
  applied, and a save can't bring back a setting the owner turned off.
- **Limits and the status cache (RED-094, RED-095).** These cover:
  - how much is kept;
  - how it is written;
  - how often status reads the file;
  - how many names a warning lists.
- **Imports (RED-096).** `Merge` keeps the local kept settings. Exports
  and bundles carry only known settings, so a shared file can't carry a
  newer release's setting to another node.
- **Followed series.** Kept settings are never taken from inside list
  entries (a series, a follow, a grind job). The series a node follows,
  and their releases, are read exactly as before. Which series a node
  follows is information in any case.

## WHITE-002: nodes share their version and features

### Step 1: `poon/VERSION` in the hello's `Agent`

Black confirms this is **not a consensus change**.

- **Nothing decides on `Agent`.** On `6260590` it is read in only two
  places:
  - `PeerInfo` copies it for display;
  - `sawSelfProbe` reads it, only for a hello with this node's own id,
    which is refused anyway (Round 31).

  Nothing in `peers`, `tracking`, `chase`, `ledger`, `engine` or `node`
  reads it.
- **No wire change.** `Agent` is already up to 256 bytes of text, and
  every node accepts any text. `poon/` doesn't collide with the self-check
  prefix `poon-self-check/`.
- **Nothing hashed or signed carries it.** The hello isn't part of any
  entry, layer, transaction or id.
- **What it costs is privacy, not agreement.** It tells peers which
  release a node runs. The proposed machine-local `p2p.announce_version`
  setting covers that, and it is the owner's and White's call.

### Step 2: the `Features` message (later round)

Clear for consensus, on the terms the proposal already states. Black
asks that they be written into the decision when step 2 is built:
- **Features never touch a verdict.** They never change what is valid,
  how weight counts, or what is selected.
- **The baseline always stays.** A node never stops asking a peer for a
  heavier ledger, or stops sending one, because a feature is missing. It
  falls back to protocol 1 requests, which every node keeps.
- **No refusing or ranking by version.** No peer is refused, scored down
  or dropped from the chase for its `Agent`, its `Protocol` or its
  features. Doing that wouldn't be a hard fork under the owner's
  definition, since it only loses information, but it could keep a node
  from hearing about a heavier ledger. Node policy must never make that
  easy.
- **A lie only costs time.** A peer's claim is used only to pick between
  ways of asking for the same data. A false claim then means a request
  that is ignored, and the fallback follows.
- **A new type number.** `Features` takes an unused number (4 to 9), so
  older nodes ignore it, as dispatch already does for unknown types.

## WHITE-001: the first public release

This plan is about process and has no rule in it. One consensus point
for the owner:

- **After the alpha tag, the byte form is fixed for good.** Round 27 said
  that witness limits could be loosened only before a release, and that
  afterwards it needs new algorithm or type numbers. A public
  `v0.1.0-alpha.1` is a release for that purpose, because nodes running
  it will judge ledgers. WHITE-001 already freezes "the consensus byte
  form and wire version 1" at alpha. Black agrees, and asks that every
  later change to what is valid be checked against the alpha's
  cross-version vectors.
- **Choosing the alpha's series is not a rule (open question 4).** It
  decides what early users hold, not how nodes agree.

## Clear

| Item | Why it is clear |
| --- | --- |
| D184: unused-snapshot and not-compared reasons capped at 8, with counts | Output only. The start log and doctor still name every pool (Round 31's condition holds) |
| D184 (W-1): unknown node.json settings kept and written back | This release ignores them as before. Exports carry only known settings |
| D184 (W-10): the release in status | Display |
| D185 (RED-093): case-folded keys are the setting | The node runs with what the decoder applies. Save writes it under its own name |
| D185 (RED-094, RED-095): kept-setting limits, compact writing, status cache, shorter warnings | Node files and display |
| D185 (RED-096): `Merge` keeps kept settings | Node policy import |
| D185 (GUI ORANGE-024): the variable series set as a `sync.Map` | The same ids, added and read at the same points. Passes under `-race`. Fixed forms are identical |
| GREEN-057 (#247) and ORANGE-042 (#249), carried in #251 | Doctor checks; one-pass insert and measurements |
| WHITE-002 step 1 (`poon/VERSION` in the hello) | Not consensus. Nothing decides on `Agent`. No wire change. Step 2 is clear on the terms above |
| WHITE-001; Red (PR #246): RED-093 to RED-096 | A process plan; findings fixed in #251. Neither touches a rule |

## Items for the owner

- **Nothing needs your approval this round.**
- **Showing peers your version can't fork anything.** No part of the node
  decides anything from it. The only cost is privacy, and the proposed
  setting lets you hide it.
- **Once a public alpha is tagged, the consensus rules are fixed.** After
  that, any loosening of what counts as valid needs new algorithm or type
  numbers, so that old and new nodes never disagree. White's plan already
  says this.

How this was checked:
- Read D184 and D185, WHITE-001 and WHITE-002, `series/released.go`
  (`addVariableID`, `IsVariableID`) and its callers, `nodepolicy/unknown.go`,
  `nodepolicy/nodepolicy.go`, `node/node.go` (`Replayed`), `p2p/peer.go`
  (`handshake`), every reader of `Hello.Agent` and `Hello.Protocol`, and
  the diffs of PRs #246, #247, #249 and #251.
- The Round 24 and Round 25 cross-version tests: identical on `6260590`
  and `378d4cb`.
- `go test ./...` passes on `6260590`, #246, #247, #249 and #251. The
  series, tx, nodepolicy, rpc, node and ledger packages pass under
  `-race` on `378d4cb`.
