# Source Round 13: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**.

**Nothing is blocked this round.**
- **B6 is fixed on main (D140).** The disk reserve now goes by rates
  measured over whole chases, with a 1.5x margin. Black's test, Red's
  three-colluder attempt and ORANGE-022's test all pass.
- **The holder at two thirds of the honest rate is a bandwidth contest,
  not a hold.** Black checked it by reading. To keep the room, an
  attacker must deliver at least two thirds of the honest sender's rate
  over whole chases, a slice at a time, for as long as it holds. Each
  time colluders swap the room they must deliver at the honest rate
  itself. No claim and no burst helps. That is the residual already
  accepted in Round 12.
- **BLUE-014 (RED-035, RED-036, RED-037) is clear, and so is
  GREEN-038.** RED-035 was a real D115 case. At the reserve, every honest
  sender whose parts reply held more than one chunk was flagged false,
  so no large entry could be fetched. It is fixed by throwing the whole
  reply away while probing, so BLUE-014 should be merged.

| Field | Value |
| --- | --- |
| Round | Source Round 13 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `97456c1` (code `f74cbaa`; D140, D141) |
| Previous base | `claude/m10-watch` @ `bc2e4bc` |
| Blue branches | `claude/project-thread-ggo8dt` @ `5db4dde` (BLUE-014, D142: RED-035 to RED-037, ORANGE-023) |
| Proposals | GREEN-038 (`claude/source-green` @ `1e3ee01`) |
| Result | 8 clear, 0 with conditions, 0 blocked |

## Round 12's items

| Item | Now | Checked how |
| --- | --- | --- |
| B6, colluders hold the reserve with paid cuts | Fixed (D140) | `TestBlackColludingTricklersHoldTheReserve` (`b6_test.go`) passes, three runs under `-race` on `5db4dde`. Only its teardown changed, which is fine. A cut needs the cutter to be 1.5 times faster by a whole-chase measure. Within that margin, the file with less still to come keeps the room. A burst no longer decides: a rate counts only after a lost request, a full 64 MiB reply, or the end of the slice |
| ORANGE-022, two slow fetches evict each other | Closed (D139, D140) | `TestTwoSlowFetchesBothFinish` passes, in CI's long step. Cuts by bytes never send a file back to offset 0, and the margin stops two fetches at about the same rate from trading bytes |

## The two-thirds holder (Red's reasoning, checked)

This is accepted as the bandwidth contest, and it is not blocked.

- **What a holder must do.** A waiting fetch W cuts a holder whose
  measured rate is below two thirds of W's. To keep the room, the holder
  needs a measured rate of at least two thirds of W's.
- **Why that rate is real.** The measure is bytes over the time the
  parts requests took, lost ones included. It is saved only after a lost
  request, a full 64 MiB reply, or the end of the slice.
  - A lost request adds the whole `RequestTimeout` to that time. So
    staying above two thirds after a loss means sending about 20 s of the
    honest rate first.
  - Without a loss, the chase keeps asking until the slice ends. The
    attacker must answer each request at that rate the whole time, or
    take the loss.
  - A non-part reply, a disconnect or a short reply drops the attacker's
    file.
- **Colluders swapping the room.** Within the margin, the file with less
  still to come wins. So a colluder that has just been cut can cut back
  only by delivering 1.5 times its partner's measured rate. That is at
  least the honest rate itself. Without that, one file keeps cutting the
  other until it completes, and the hash check decides.
- **The cost.** For every chase that keeps its measure fresh (at least
  every `partsStale`, 4 minutes), the attacker sends honest-scale
  bandwidth for about a slice. That is a contest of bandwidth against a
  chosen node, as with flooding its link. No setting refuses the ledger,
  and the hold ends when the attacker stops paying.
- **For the docs.** `docs/modules/p2p.md` should state this residual and
  its cost plainly, beside the reserve's description.

## Clear

| Item | Why it is clear |
| --- | --- |
| D140: rates over whole chases, the 1.5x margin, "one byte of room is not room" | Ranking and timing only. It closes B6 and keeps B4, B5 and ORANGE-022 closed (their tests pass) |
| D141: GREEN-037 adopted, ORANGE-022 superseded | Cleared in Round 12 |
| BLUE-014: RED-035 with ORANGE-023 | A probe throws the whole reply away. Every chunk is still checked against a running offset, so a lying sender is still caught and an honest one is never flagged. This fixes a real D115 case at the reserve |
| BLUE-014: RED-036 | Drops count toward holding a pace only if the sender served part of the window. It only paces requests to one peer, never below one a window, and the pace lapses |
| BLUE-014: RED-037, `measureLasts` | A waiting chase that keeps losing asks less often: up to 8 times `partsStale`, 32 minutes. A holder that slows, stalls or leaves is still cut at once by its own measure. A waiting sender that sped up waits at most 32 minutes to show it. That is a delay, and the measure always lapses |
| BLUE-014: the test-harness teardown race | Tests only |
| GREEN-038, `poo follow -replace` | The CLI uses `ReplaceFollow` (D139, GUI-018), which is cleared. It changes which pools this node follows, as before |
| GREEN-038, the CLI and GUI parity test | Tests only. It supports the owner's rule that the GUI and the headless node never diverge |

## A note for Grey (no verdict)

`measured()` in `fetchParts` also counts the end of the slice. If a
chase reached `fetchParts` with its slice already over, one part's rate
would be saved as the whole chase's measure, which is B6's burst. With
the shipped values that cannot happen: a resumed chase asks for the
large entry first, and the notice must arrive within `RequestTimeout`
(30 s), well inside the 2-minute slice. Neither value is an operator
setting. Black tried it with a 1 s slice and a 2 s time-out in a test,
and the honest fetch still took the room. Keep `RequestTimeout` below
`ChaseSlice` if either ever becomes a setting, or count the slice end
only once a parts request has run for a while.

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open on main with BLUE-014 merged.** What
  remains is a bandwidth contest: an attacker who fills a node's spare
  disk can hold back a large entry only while it keeps sending at about
  the honest sender's rate.

How this was checked:
- Read the code changes from `bc2e4bc` to `97456c1`, the diffs of
  BLUE-014 and GREEN-038, and decisions D140 to D142.
- On `5db4dde`, these pass: `go test ./pkg/p2p ./pkg/chase ./pkg/vectors
  ./pkg/cli ./pkg/rpc ./pkg/nodepolicy`. So does `go test -race
  -count=3` on the B5, B6, ORANGE-022, trickler, `TakeFrom`, probe,
  outage, `MeasureLasts` and slow-sender tests.
- GREEN-038's CLI tests pass on `1e3ee01`.
