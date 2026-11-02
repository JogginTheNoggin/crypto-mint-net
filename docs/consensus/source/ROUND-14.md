# Source Round 14: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

- **D143 is clear.** It adds the GREEN-038 parity commands, states
  D140's residual, and keeps the timeout below the slice as Black asked
  in Round 13. Submitting an entry or a layer through the CLI goes
  through the node's normal checks for a pushed one, with nothing
  skipped.
- **BLUE-015's lifted body cap is clear.** It removes a size limit, and
  adds none. Before, the local RPC refused any entry over about 4 MiB,
  while peers can send one of any size (D117). That was a cap every user
  of the RPC would hit, like C1. Only the full token gets the lift, and
  that token can already stop the node, so the extra memory use is not a
  new denial of service.
- **GREEN-039 is clear.** It is CLI output and hints only.
- **ORANGE-024 has no change to review.** Its note on a waiting fetch's
  file looking stale is checked below. It is a delay paid for in bytes,
  not a hold.

| Field | Value |
| --- | --- |
| Round | Source Round 14 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `184bc54` (code `4982719`; D142 merged, D143) |
| Previous base | `claude/m10-watch` @ `97456c1` |
| Blue branches | `claude/project-thread-ggo8dt` @ `2f3e289` (BLUE-015, D144: RED-038) |
| Proposals | GREEN-039 (`claude/source-green` @ `2455f5e`); ORANGE-024 (`claude/source-orange-r14` @ `43877ce`, measurements only) |
| Result | 7 clear, 0 with conditions, 0 blocked |

## Round 13's items

| Item | Now | Checked how |
| --- | --- | --- |
| BLUE-014 (RED-035 to RED-037) | Merged unchanged (D142) | The chase code on `184bc54` matches what was cleared on `5db4dde` |
| The note on `measured()` and the slice | Met (D143) | `RequestTimeout` (30 s) stays below `ChaseSlice` (2 min). Both fields say so, and `TestRequestTimeoutBelowChaseSlice` checks it |
| D140's residual, stated in the docs | Met (D143) | `docs/modules/p2p.md` states the bandwidth contest and what probing costs the honest node |

## BLUE-015: the lifted body cap (D144)

- **What changed.** `MaxBody` (8 MiB) no longer applies to `POST
  /v1/entry` with the full token. Every other route, and every request
  with the view token, is bounded as before. The view token is refused
  before the body is read.
- **Large entries.** This removes a limit and adds none.
  - Before, an entry over about 4 MiB (8 MiB of hex) could not be handed
    to a node through its own RPC, from the CLI or the GUI. The same
    node takes such an entry from any peer, in parts (D117).
  - The owner refused size caps on entries (C1). A cap on the owner's
    own route was not a consensus rule, since a peer could still deliver
    the entry. But it made the headless node and the GUI unable to do
    what the network can, so lifting it is right.
  - The entry still goes through `HandleEntry` as a pushed entry, with
    every check a peer's entry gets. Nothing is skipped and nothing is
    added.
- **Denial of service.**
  - Only the full token gets the lift. That token can already stop the
    node, change its policy and spend its wallet, so a large body is not
    a new power.
  - The cost is memory: the hex body, its decoded bytes and the entry,
    about three times the entry's size at once.
  - A peer's large entry is held on disk while it arrives, but it is
    also decoded whole in memory at the end (`fetchParts` reads the
    file whole). So both paths have the same peak, about the entry's
    size.
  - Not blocking, but worth doing later: stream-decode the hex (with
    `hex.NewDecoder`) so the peak is the entry once, not three times.
- **RED-038, reading the hex from a file or standard input.** This is
  CLI input only.

## ORANGE-024's note: a waiting file looks stale

- **What happens.** Under D142, a waiting fetch W can go up to eight
  times `partsStale` without asking. Meanwhile `takeFrom` treats W's
  kept file as stale after `partsStale`, so any third fetch X cuts it
  first (priority 1), whatever X's rate.
- **Why it is not a hold.**
  - X takes only as many bytes as it receives, and pays for them by
    sending them.
  - When W measures again, it cuts X back if W is 1.5 times faster. If
    not, X is the faster fetch anyway.
  - W was waiting only because the holder was at least as fast as it, or
    nearer its end within the margin. In both cases the holder could
    already cut W's file.
  - A cut file resumes where it was cut and never goes back to offset
    0.
  - So at worst this moves bytes from a waiting fetch to one that pays
    for them, and delays W. It never refuses W for good, and an attacker
    gains nothing it could not get by sending the same bytes.
- **Suggestion (optional).** Make `takeFrom`'s staleness for a file whose
  own chase is waiting follow `measureLasts`, so the two clocks agree.
  It is a tidier rule, not a fix.

## Clear

| Item | Why it is clear |
| --- | --- |
| D143: GREEN-038 adopted, the follow replace and the parity test | Which pools a node follows is its own policy. The parity test supports the owner's rule that the headless node and the GUI never diverge |
| D143: submitting an entry or a layer from the CLI | Decoded first, then handled as a pushed entry or layer, with every check. It is a write command, refused under the view token |
| D143: one grind job by id | Read only |
| BLUE-015: the lifted body cap for full-token entries | Removes a size bound that every RPC user hit. See above |
| BLUE-015: RED-038, the hex from a file or standard input | CLI input only |
| GREEN-039 | The build and work commands can also print the encoded bytes; a hint names a missing parent; the grind jobs flags may follow the id. Output and hints only |
| ORANGE-024 | Measurements only. Its note is a delay paid for in bytes, as shown above |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** The node's own RPC can now hand it an
  entry of any size, as a peer can. Only the full token can do this.

How this was checked:
- Read the code changes from `97456c1` to `184bc54`, BLUE-015's,
  GREEN-039's and ORANGE-024's diffs, and decisions D143 and D144.
- On `2f3e289`, these pass: `go test ./pkg/cli ./pkg/rpc ./pkg/vectors
  ./pkg/chase`, and the B5, B6, ORANGE-022, trickler, `TakeFrom` and
  probe tests in `pkg/p2p`. `go test ./pkg/cli` passes on `2455f5e`.
