# Source Round 39: consensus review (Black, Part D)

## Summary

This round lets a grinder say which offers it can finish (D202, jerry's
request). A grinder sends a required max strength byte in bounty
messages 71 (list request) and 73 (take). A payer lists, and leases, only
offers at or under it.

D202 also does three more things:
- it freezes the bounty memo's start and the wallet's leg state names
  (W-34);
- it always returns `done` (W-35);
- it writes the "hints are checked" rule for kept fields (W-33).

Blue's D203 fixes RED-120 (a dropped payment leg) and RED-121 (a memo that
named the offer by 4 bytes), and carries Green's GREEN-064.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - Since Round 38's base, no engine, ledger, tx, layer, entry, series,
    policy, node, wire or tracking code has changed.
  - `TestByteFormV1Frozen` passes on `main` and on PR #346, and the
    Round 24 and Round 25 cross-version tests give the same bytes on
    both.
- **Max strength decides what the market offers, never what the ledger
  keeps.**
  - It only filters the lists a payer sends and the leases it grants.
  - The claim path is unchanged: a claimed layer at or above the series'
    floor still goes to `HandleLayerFrom` as an ordinary pushed layer,
    whatever the offer or the grinder's setting.
- **A required byte in bounty version 1 is fine before the tag.** The
  byte is not a trailing extra, so a build from before D202 and one from
  after can't use messages 71 and 73 with each other. Their peers would
  close such a connection with an `InvalidMessage` flag.
  - No release has bounty version 1, so no node that will judge ledgers
    is cut off. This is the same reasoning Black accepted for the hello
    change (Round 33, term 5).
  - wire.md rows 71 and 73 now give the byte as required, with 0
    malformed. From the tag it is frozen with the rest of bounty version
    1, so Round 37's concern can't come back: a compliant later build
    always sends it.
- **The memo and leg names are wallet records.** A memo is a field of the
  wallet's sent list, not of any transaction. The leg state names are
  wallet file values. Treating an unknown state as sent (W-34) only
  stops a payer paying twice.

| Field | Value |
| --- | --- |
| Round | Source Round 39 |
| Consensus | None (D202, D203) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `21c34c8` (PR #339: D202; GUI #341 on top) |
| Previous base | `main` @ `1e71cf0` |
| Blue branches | PR #346 @ `cf7e940` (BLUE-040, D203: RED-120, RED-121; carries GREEN-064 #343) |
| Red | PR #342 @ `6b45a66` (RED-120, RED-121) |
| Proposals | GREEN-064 (PR #343 @ `38e12f4`, carried in #346); ORANGE-049 (PR #340 @ `ed9d3ed`, measurements, no proposals) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## The grinder's max strength (D202)

### What it changes

- **The setting.** `bounty.max_strength` (0 to 255, where 0 sends 255,
  meaning any), and `-max-strength` on `poon bounty list -ask` and `poon
  bounty work`.
- **The wire.** Message 71 is `Hash` series ∥ `U8` max strength, and
  message 73 is `Hash` offer ∥ `U64` rate ∥ `Hash` pay-to ∥ `U8` max
  strength. `DecodeListReq` and `DecodeTake` refuse a message without
  the byte, or with 0.
- **What a payer does.** `Book.Listable(series, max)` lists only offers at
  or under the asker's max strength. `TakeFrom` refuses a lease on a
  stronger offer with a grant of 0 seconds and a reason.
- **What a grinder does.** It refuses locally before it asks, drops
  received offers above its own setting, and (GREEN-064) won't ask for
  more than its own setting allows.

### Why it is clear

- **No verdict reads it.** The byte is read by `Book.Listable`,
  `Book.TakeFrom` and `Book.ReceiveFrom` in `pkg/bounty`, and by
  `AskBounties` and `TakeBounty` in `p2p`. No engine, ledger, chase, peer
  book or tracker code reads it.
- **Valid layers are still kept.** `handleBountyClaim` hasn't changed
  since D198. A claim's layer goes to the engine when it meets the
  series' floor and the pool doesn't hold it, whatever the offer's
  strength or the grinder's setting. A payer that refuses a lease is
  only declining to pay.
- **A false max strength costs only time.** A grinder that says too
  little sees fewer offers. One that says too much may get a lease it
  can't finish, and the lease runs out. Neither affects any ledger. That
  matches Round 32's term "a lie only costs time".
- **Nothing ranks or refuses a peer for it.** A refused lease is a
  market answer. It isn't an offence, and the connection stays.

### A required byte before the tag

- **Builds on either side of D202 can't share messages 71 and 73.** A
  32-byte list request or a 72-byte take is now malformed to a D202
  node, so `misbehave` flags it and ends the connection.
- **Why that is acceptable now.** Every build with bounty version 1 is a
  development build. None has been released, and the market is off by
  default. Round 33 (term 5) accepted the same thing for the hello
  layout before the tag. Changing the layout now, rather than adding a
  trailing field, keeps version 1 simple from the start.
- **From the tag it is frozen.** wire.md rows 71 and 73 say "required"
  and "0 is malformed". A compliant later build always sends the byte,
  so the `InvalidMessage` flag can only reach a build that breaks the
  frozen layout. That is the standard Round 34 set and Round 38
  confirmed for the offer bound.

## Wallet records and payments (D202 W-34, D203)

- **The memo (W-34, RED-121).** `bounty ` plus the first 16 bytes of the
  offer id in hex. The memo is a field of the wallet's sent list
  (`wallet.Sent.Memo`). It is never part of a transaction's bytes, its
  tid or any witness. A chance match needs about 2^64 offers, and could
  only stop a payer paying, never change a ledger.
- **Leg state names (W-34).** These are frozen values in the wallet
  file. `sentFor` treats a state it doesn't know as sent, so a later
  release's state can never make a paid claim look unpaid.
- **A dropped leg is resent, not replaced (RED-120).** `poon bounty pay`
  refreshes the earlier payment from the node and resends its dropped
  legs with the wallet's own `Refresh` and `Resend`. It records the
  payment only when every leg is sent. Resending gives nodes the same
  transaction again, with the same bytes and tid, and nodes check it as
  they check any transaction. No second payment is built for that claim.
- **Kept fields as hints (W-33).** A rule for a node's own files. A wrong
  hint costs time, never correctness.
- **`done` always returned (W-35), notes in status (GUI-GREEN-064),
  `minute_strength` in `grind bench` (GREEN-064).** These are output and
  display. `minute_strength` is log2(rate × 60), a guide for the
  setting, and nothing decides on it.

## Clear

| Item | Why it is clear |
| --- | --- |
| D202: `bounty.max_strength`, `-max-strength`; a required `U8` in bounty messages 71 and 73 | Market filtering only. The claim path and the engine are unchanged. A required field is acceptable before the tag (no release has bounty v1) and is frozen in wire.md from it |
| D202: payers list and lease only offers at or under the asker's max strength; grinders drop stronger offers | Market answers. Not an offence, no ranking. A false value costs time |
| D202 (W-34): memo start `bounty ` and leg state names frozen; an unknown state read as sent | Wallet records. No transaction carries a memo. It can only prevent a second payment |
| D202 (W-33, W-35): kept fields as checked hints; `done` always returned | A node's own files; output |
| D202 (GUI-GREEN-064): notes in `poon bounty list` and `poon status`; Min 0 on amount and switch limits; `=on` wording | Display and node policy |
| D203 (RED-120): a dropped bounty payment resent, not replaced | The wallet resends the same transaction. Nodes check it as any transaction |
| D203 (RED-121): memo names the offer by 16 bytes | Wallet record. Not in any transaction |
| GREEN-064 (carried in #346): `NotReceived` message, ask over own setting refused, `grind bench` `minute_strength` | CLI messages and a measurement |
| Red (PR #342): RED-120, RED-121 | Findings, fixed in #346. Neither touches a rule |
| ORANGE-049 (PR #340) | Measurements only. No code and no proposals |

## Items for the owner

- **Nothing needs your approval this round.**
- **Grinders saying which offers they can finish can't fork anything.**
  It only decides which offers a payer shows them and pays them for.
  Every valid layer a grinder hands in is still kept.
- **The new field in the bounty messages is safe to add now,** because no
  release has the market yet. From the first tag it is fixed, like the
  rest of the layout.

How this was checked:
- Read D202 and D203, the wire.md diff, `bounty.DecodeListReq`,
  `DecodeTake`, `Book.Listable`, `TakeFrom` and `ReceiveFrom`,
  `p2p/bounty.go` (`AskBounties`, `TakeBounty`, `handleBounty`,
  `handleBountyClaim`), `cli/bounty.go` (`sentFor`, pay), `wallet/pay.go`
  (`Sent.Memo`, the leg states), `grind/bench.go`, and the diffs of PRs
  #339, #340, #342, #343 and #346.
- Checked that no engine, ledger, tx, layer, entry, series, policy, node,
  wire or tracking code changed between `1e71cf0` and `cf7e940`, and
  that no transaction type has a memo field.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `21c34c8` and `cf7e940`.
- `go test ./...` passes on `21c34c8`, #340, #342, #343 and #346. The
  bounty, p2p, cli, grind, rpc and nodepolicy packages pass under `-race`
  on `cf7e940`.
