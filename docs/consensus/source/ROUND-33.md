# Source Round 33: consensus review (Black, Part D)

## Summary

This round gets the node ready for the first alpha:
- the hello carries the release and features, and hello, ping and pong
  each end in an extension list (D186, WHITE-002 as revised, jerry's
  "hello only, both extensible");
- new default ports, which `poo init` now writes (D187);
- ledger export and import, where every imported entry is fully checked
  (D188, jerry's rule).

Blue's D189 fixes Red's RED-097 to RED-099 and GUI-031. It carries
Orange's streaming import with parallel witness preverify (ORANGE-043)
and Green's GREEN-058.

**Result: 9 clear, 1 clear with a condition, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - Byte form v1 is untouched. `TestByteFormV1Frozen` passes on `main`
    and on PR #266, and the Round 24 and Round 25 cross-version tests give
    the same bytes on both.
  - Only the wire half of D76 is amended, as Black's term 1 required.
- **Nothing decides on a peer's release, features or extensions.** They
  are read only for `PeerInfo`, `poo peers` and the counts.
  `TestHeavierWhateverTheHelloClaims` shows a heavier ledger is taken
  whatever the hello claims.
- **An import goes through exactly the path of an entry a peer sent.**
  `ImportItems` calls `HandleEntryFrom` and `HandleLayerFrom` with origin
  `Requested`. Origin decides only the seen list and forwarding.
  `AddEntry` runs every check. Nothing in a file is trusted.
- **Parallel preverify gives identical verification.** It calls the same
  `series.CheckWitnesses` that `tx.Check` calls, and it only fills the
  cache of witnesses that passed. Each entry is then taken in order
  through every check.
- **The condition is about the hello terms, not a rule.** A malformed
  ping or partial pong still earns a lasting offence. Term 4 says it
  shouldn't, and D186 says term 4 is kept. Either fix it or correct the
  wording, and add one test. Details are below. It isn't blocking.

| Field | Value |
| --- | --- |
| Round | Source Round 33 |
| Consensus | None (D186 to D189). D186 amends the wire half of D76 before any release, as Black's terms allowed |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `3c4ae00` (PR #255: D186 to D188; #256 pool 0 export; GUI #258 on top) |
| Previous base | `main` @ `6260590` |
| Blue branches | PR #266 @ `c569810` (BLUE-034, D189: RED-097 to RED-099, GUI-031; carries ORANGE-043 #265 and GREEN-058 #259) |
| Red | PR #257 @ `2573446` (RED-097 to RED-099) |
| Proposals | ORANGE-043 (PR #265 @ `7ef7929`); GREEN-058 (PR #259 @ `b45b27f`) |
| Result | 9 clear, 1 clear with a condition, 0 blocked |

## Release, features and extensions (D186)

Black checked each of the terms from Round 32 against the code.

| Term | Holds? | What Black checked |
| --- | --- | --- |
| 1. Only the wire half of D76 | Yes | `TestByteFormV1Frozen` and the cross-version vectors are unchanged on `3c4ae00` and `c569810`. None of hello, ping or pong is part of any entry, layer, transaction, id, hash or signature |
| 2. Display and optional extras only | Yes | `Release` and `Features` are read only for `PeerInfo`, which feeds `poo peers` and the counts. `Ext` is decoded and held, and nothing reads it. Nothing in `peers`, `tracking`, `chase`, `ledger`, `engine` or `node` reads any of them |
| 3. Baseline kept, nothing gated | Yes | No code checks a feature. `TestHeavierWhateverTheHelloClaims` covers a peer whose hello claims anything |
| 4. Unknown skipped; malformed closes only that connection, no lasting offence | **Hello yes; ping and partial pong no** | See the condition below |
| 5. Old and new builds can't talk | Yes | `TestHelloMalformed`. D189 names it with `wire.ErrOldHello` and the CHANGELOG says so |
| 6. Registered names and tags; fuzzed | Yes | Registered in `wire/hello.go`; seeded in `FuzzBodies` |

Black also checked that a future build can use the full room the layout
freezes:
- a hello with `Protocol` 9, an unknown release, an unknown feature and 32
  unknown extensions of 1 KiB each decodes;
- a ping with the same 32 extensions decodes (about 33 KB, far under the
  16 MiB frame);
- a 33rd extension is refused.

So a later release that follows the frozen limits is skipped, never
refused.

### The condition: term 4 for ping and pong

A malformed hello only closes the connection. `handshake` returns the
error and `runConnNotify` closes, with no offence.

A malformed ping, or a malformed `PartialPong` body, goes another way.
The error wraps `wire.ErrMalformed`, and `misbehave` turns it into
`tracking.InvalidMessage` (severity 5, kept 24 hours) before it closes.
The paths are:
- `discovery` → `DecodePing` → `dispatch` → `misbehave`;
- `ping` → `DecodePongReply` → `misbehave`.

That is a lasting offence, which term 4 rules out and D186 says is kept.

- **Why it matters little in practice.** The limits are frozen at the
  first tag, and `checkSend` stops a node from sending what its peers
  would refuse. So a build that follows the layout never trips this. The
  flag lands only on a buggy or hostile peer.
- **Why it still needs settling.** Term 4 exists so that a later release
  is never flagged by older nodes for using the extension room.
  Penalising a peer can keep a node from hearing about a heavier ledger.
  That is a lack of information and not a hard fork under the owner's
  definition, but node policy should never make it likely. The decision
  text and the code should say the same thing.
- **Condition, for the next Part A (Grey or Blue).** Do one of these:
  - Make a failure inside the release, features or extension list of a
    ping or pong close the connection without an offence, as the hello
    already does. One way is an error that doesn't wrap `ErrMalformed`.
  - Or correct D186's term 4 to say that a malformed ping or pong gets
    the usual invalid-message flag, because no compliant build can send
    one.

  Either way, add a p2p test showing that a ping, and a hello, at the
  frozen maximum with unknown tags and an unknown feature is answered,
  with no offence recorded.
- **A note on plain pongs.** A plain `Pong` body is never decoded today,
  so its extensions aren't read at all. That is consistent with "skip
  what you don't know". If a later release needs pong extensions,
  receivers will have to start decoding them.

ORANGE-043's proposal 1 (keep only registered extensions after decoding)
is clear. It bounds memory and changes nothing about what is skipped or
refused.

## Ledger export and import (D188, D189, ORANGE-043)

### Import is the peer path

- **Same calls.** Each item is decoded with `wire.DecodeItem`. Its entry
  goes to `HandleEntryFrom(k, e, engine.Requested, "import")`, then each
  layer nonce goes to `HandleLayerFrom` with the same origin.
- **Origin changes no check.** `Requested` skips the seen list, so an
  entry dropped earlier can be fetched again. It also means the entry is
  never forwarded. `AddEntry` and `AddLayer` run every rule, the same as
  for a peer's entry.
- **No shortcut.** A missing parent is refused, as it would be from a
  peer. A file's summary fields (height, counts, tip) are only compared
  by `-dry-run` and the export checks. They are never trusted on import.
- **Bounds without a size rule (RED-098).** A call holds at most 256
  items, and at most 8 MiB after the first item. The first item has no
  bound, so an entry of any size can still be imported alone (D144). The
  command counts the first item too, so its batches always fit the
  node's bound. The p2p path is untouched.

### Parallel preverify (ORANGE-043, carried in #266)

`ImportItems` now decodes every item first. It then hands the leading run
that decoded to `Engine.PreverifyPage`, the same function a chase uses
for a page.

- **Owners come from the real state.** `PageOwners` builds the state
  after the run's parent under the lock (`viewAfter(...).Overlay()`), and
  applies each entry in turn. It stops at the first entry that:
  - fails `CheckShape`;
  - doesn't chain to the one before;
  - is already held;
  - has owners that can't be found.
- **Same check, run early.** The workers call `tx.Preverify`, which calls
  the same `series.CheckWitnesses(witnesses, owners, tid)` that
  `tx.Check` calls. A pass is remembered by witness and tid
  (`verifiedKey`). A failure is not remembered, and it stops the
  workers.
- **Then the full path, in order.** Every entry still goes through
  `HandleEntryFrom`. `CheckCached` runs the ceiling, address and allowed
  checks every time, before it looks at the cache. A remembered pass
  only skips `Satisfy`, which depends on nothing but the witness and the
  tid. So a wrong or stale owner still fails the address check.
- **Already accepted.** This is the mechanism Black cleared for chase
  pages (D107, ORANGE-014 and ORANGE-020). Only the caller is new.

### Export (D188, RED-097)

The export reads the selected ledger page by page and checks itself:
- each page starts at the next height and chains to the last entry
  written;
- the last entry is at the header's `to`;
- when the export runs to the tip, the tip matches the header.

Otherwise it fails and writes nothing. That is a tool's honesty about its
own file, and a file is checked again in full when it is imported.

## Clear

| Item | Why it is clear |
| --- | --- |
| D186: release, features and extensions in hello, ping and pong; `p2p.announce_version`; `poo peers` release and claimed features | Wire layout before any release. Nothing decides on them. Byte form v1 frozen and identical across builds. Terms 1, 2, 3, 5 and 6 hold (term 4 is under the condition) |
| D187: ports 7336 and 7337 written by `poo init`; `0.1.0-alpha.1-dev`; pre-release tags | Node policy and release tooling |
| D188: `poo ledger export` | Reads the selected ledger. A file is only a carrier |
| D188: `poo ledger import`, every item through `HandleEntryFrom` and `HandleLayerFrom` as `Requested` | The same checks as an entry from a peer. Nothing in a file is trusted. Not forwarded |
| ORANGE-043 (carried in #266): streamed file; `PreverifyPage` on import batches | The same `CheckWitnesses`, which only fills the cache. Every entry is still fully checked in order |
| D189 (RED-097): the export ends where its header says, or fails | Tool output |
| D189 (RED-098): import call bounded at 256 items and 8 MiB after the first; `-dry-run` | No entry refused by size (the first item has no bound). The p2p path is untouched |
| D189 (RED-099, GUI-031): `wire.ErrOldHello`; partial totals on a stop or error | Terms 5 and 3 stand. Reporting only |
| GREEN-058 (carried in #266); ORANGE-043 proposals 1 and 2 | Tooling. Proposal 1 bounds memory; proposal 2 is the same check streamed |
| Red (PR #257): RED-097 to RED-099 | Findings, fixed in #266. None touches a rule |

## Clear with a condition

| Item | Condition |
| --- | --- |
| D186 term 4: a malformed ping or partial pong closes with a lasting `InvalidMessage` offence | Either close those without an offence, as the hello already does, or correct D186's term 4 to say so. Add a p2p test that a ping and a hello at the frozen maximum, with unknown tags and an unknown feature, get no offence. Not blocking |

## Items for the owner

- **Nothing needs your approval this round.**
- **Importing a ledger file is as safe as getting it from a peer.** Every
  entry in the file goes through the same checks, and nothing in the file
  is trusted. A changed file is refused item by item.
- **Telling peers your version can't fork anything.** No part of the node
  decides from it. A future release can add fields that this release
  quietly skips.
- **One small fix is asked for next round.** A garbled ping should end
  only that connection, as a garbled hello already does, or the decision
  should say otherwise. It only affects buggy or hostile peers today.

How this was checked:
- Read D186 to D189, `wire/hello.go`, `wire/messages.go` (`DecodeHello`,
  `finish`), `wire/discovery.go` (`DecodePing`, `DecodePongReply`),
  `p2p/peer.go` (`handshake`, `ownHello`, `runConnNotify`, `dispatch`,
  `misbehave`), `p2p/discovery.go` (`ping`, `askFollow`, `verifyPeer`),
  `node/import.go`, `engine.go` (`Origin`, `HandleEntry`, `PreverifyPage`),
  `ledger/pool.go` (`PageOwners`), `tx.Preverify` and `checkWitnesses`,
  and the diffs of PRs #257, #259, #265 and #266.
- Black's wire test: a hello and a ping at the frozen maximum, with
  unknown tags and an unknown feature, decode, and a 33rd extension is
  refused.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `3c4ae00` and `c569810`.
- `go test ./...` passes on `3c4ae00`, #257, #259, #265 and #266. The wire,
  p2p, node, rpc, watch, engine and cli packages pass under `-race` on
  `c569810`.
