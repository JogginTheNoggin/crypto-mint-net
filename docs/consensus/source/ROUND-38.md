# Source Round 38: consensus review (Black, Part D)

## Summary

This round settles the bounty market's files and wire bounds before the
tag. Part A (D200) does four things:
- freezes the 1,024-byte offer bound in wire.md, meeting Black's Round 37
  condition (W-32);
- freezes the done file's format (W-31);
- starts the done-file read from where `bounties.json` left off
  (ORANGE-047);
- has the wallet check its file only once.

Blue's D201 fixes RED-118 (torn or trimmed done files) and RED-119 (a
payment sent but not recorded), and carries Green's GREEN-063.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - Since Round 37's base, no engine, ledger, tx, layer, entry, series,
    policy, node, wire or tracking code has changed. The only p2p change
    is D199's, which Round 37 reviewed.
  - `TestByteFormV1Frozen` passes on `main` and on PR #332, and the
    Round 24 and Round 25 cross-version tests give the same bytes on
    both.
- **Round 37's condition is met.** wire.md now freezes the offer bound
  with bounty version 1:
  - in row 70 ("at most 1,024 bytes in all");
  - in row 72 ("each at most 1,024 bytes");
  - in the W-29 rule, which says trailing fields must fit and a larger
    offer is bounty version 2.

  A compliant later release therefore can't send a version 1 node an
  offer that node calls malformed. The `InvalidMessage` flag on an
  over-size offer now only reaches a build that breaks the frozen layout,
  as Round 34 accepted for the other fixed parts.
- **Phase 2's signatures are planned outside the ledger.** wire.md says a
  signature, key or proof travels beside the offer's bytes. It goes in a
  new message type or a version 2 list entry, sent only to peers that list
  `bounty` at 2 or more, so an offer's id never changes. A new type that
  older nodes don't know is ignored, as dispatch already does. Black will
  check phase 2 against these terms when it is built.
- **Every file change is a node's own file.** The done file, `done_size`,
  `done_last`, the wallet's single-pass read and write, and `poo bounty
  pay`'s check of the wallet's sent list never touch transaction bytes or
  tids.

| Field | Value |
| --- | --- |
| Round | Source Round 38 |
| Consensus | None (D200, D201) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `1e71cf0` (PR #325: D200; GUI #327 on top) |
| Previous base | `main` @ `7eb16ab` |
| Blue branches | PR #332 @ `43a4df4` (BLUE-039, D201: RED-118, RED-119; carries GREEN-063 #328) |
| Red | PR #326 @ `e0a2249` (RED-118, RED-119) |
| Proposals | GREEN-063 (PR #328 @ `19ced7f`, carried in #332); ORANGE-048 (PR #329 @ `5d2abf5`, measurements, no proposals) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## Round 37's condition: the offer bound frozen (D200, W-32)

Round 37 asked that the 1,024-byte offer bound be written into wire.md as
frozen with bounty version 1, or that an over-size offer cost no offence.
D200 took the first option.

- **What the text says.** Rows 70 and 72 give the bound "frozen with
  bounty version 1". W-29 now adds: "Trailing fields must keep an offer
  within 1,024 bytes: an offer over it is malformed to a bounty version 1
  node, so a larger offer is bounty version 2." Version 2 messages go only
  to peers listing `bounty` at 2 or more.
- **Why that meets it.** A later release that follows wire.md either
  fits its offers in 1,024 bytes or sends them only to version 2 peers. No
  compliant build can then earn an `InvalidMessage` flag, or lose its
  peerings, from an older node's offer bound. That is the standard
  Round 34 set for the frozen parts of the ping and pong layout.
- **The code matches.** `MaxOfferBytes` is still 1,024 in `DecodeOffer`
  and `DecodeList`, unchanged since Black's check on `39e7cb2`, and
  `TestBlue038OfferBytesBounded` covers it.
- **The optional budget idea.** D200 leaves the claim lane on the
  discovery budget, about 2% of the layer budget, with every layer
  checked in full. Black agrees. It was optional, and it is node policy.

## The done file and payments (D200, D201)

- **The format is frozen (W-31).** The first line names the file, and
  every later line begins `{"id":"<hex>"`. A newer format is refused with
  a message. This file belongs to the node and is never sent.
- **Reading from `done_size` and `done_last` (ORANGE-047 P1, RED-118).** A
  start reads from the recorded offset only when the line ending there is
  the one named. Otherwise it reads the whole file. A cut last line is
  reported, and `archive` ends it before it appends.
  - Either way, an offer found in both files is still left out, so a
    payment can't be owed twice.
  - This decides what the market owes, never what a ledger holds.
- **Payments are recorded, never sent twice (RED-119).** `Book.Paid` puts a
  claim back to unpaid when its save fails. `poo bounty pay` looks for a
  matching payment in the wallet's `Sent` list (memo, address, amount,
  series, legs pending or selected, no tid used by another claim) and
  records it rather than sending again.
  - The payment itself is still an ordinary wallet transaction.
  - A match only skips building a second one.
- **Wallet files checked once (ORANGE-047 P2, P3).** `keepjson.SplitValid`
  and `JoinValid` skip a validity check that `encoding/json` has just
  made. `wallet.Open` and `Save` decode and encode directly.
  `TestOrange047FileBytesUnchanged` compares both paths byte for byte. A
  payment file's transaction bytes go through the same `Unsigned`
  decoding, so `poo sign` still signs the tid every node computes.
- **Offer states (GUI-GREEN-063).** Filled stays filled, offers that
  finish by the clock move to the done file, and `switch` marks appear in
  the limits list. All of this is display and market state.
- **`poo wallet watch -tid` and `poo bounty list -done` (GREEN-063).**
  `-tid` asks the node for a transaction through the existing `Tx` call,
  and adds that transaction's mint to the wallet's watch list. `-done`
  reads the done file. Both are reads.

## Clear

| Item | Why it is clear |
| --- | --- |
| D200 (W-32): 1,024-byte offer bound frozen with bounty version 1 in wire.md; phase 2 proofs beside the offer's bytes | Meets Round 37's condition. A compliant later build can't trip it. The offer id never changes |
| D200 (W-31): done file format 1 frozen | A node's own file |
| D200 (ORANGE-047 P1): `done_size` | A start reads less of a node's own file. An offer in both files is still left out |
| D200 (ORANGE-047 P2, P3): `SplitValid`, `JoinValid`; wallet read and written in one pass | Tool files, the same bytes (tested). Transaction bytes and tids are unchanged |
| D200 (GUI-GREEN-063, GUI Orange R37B): filled stays filled, offers done by the clock archived, `switch` marks, `bounty` in `status -counts` | Market state and display |
| D201 (RED-118): `done_last`, torn and trimmed done files | A node's own file. A payment is never owed twice |
| D201 (RED-119): failed `Paid` save rolled back; `poo bounty pay` records the wallet's earlier payment | Market records. The payment is an ordinary transaction, and a match only avoids sending a second |
| GREEN-063 (carried in #332): `poo wallet watch -tid`, `poo bounty list -done` | Reads through existing calls |
| Red (PR #326): RED-118, RED-119 | Findings, fixed in #332. Neither touches a rule |
| ORANGE-048 (PR #329) | Measurements only. No code and no proposals |

## Items for the owner

- **Nothing needs your approval this round.**
- **Last round's one condition is met.** The size limit on bounty offers
  is now part of the protocol document, so a later release can't trip it
  and lose its peers.
- **The bounty market still can't fork anything.** This round only
  changes how a node keeps its own market files and avoids paying twice.

How this was checked:
- Read D200 and D201, the wire.md diff, `bounty.DecodeOffer` and
  `DecodeList`, `pkg/bounty/book.go` (done file, `Paid`), `cli/bounty.go`
  (pay's sent-list check), `cli/wallet.go` (`-tid`), `wallet/pay.go`,
  `keepjson`, and the diffs of PRs #325, #326, #328, #329 and #332.
- Checked that no engine, ledger, tx, layer, entry, series, policy, node,
  wire or tracking code changed between `7eb16ab` and `43a4df4`.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `1e71cf0` and `43a4df4`.
- `go test ./...` passes on `1e71cf0`, #326, #328, #329 and #332. The
  bounty, keepjson, cli, wallet and rpc packages pass under `-race` on
  `43a4df4`.
