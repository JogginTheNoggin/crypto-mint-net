# BLUE-013: Paid cuts at the disk reserve; a second outage halves the probe; one follow replace

| Field | Value |
| --- | --- |
| Fixes | RED-031 (medium), RED-032 (medium), RED-033 (low), RED-034 (medium), GUI-018 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `bc2e4bc` (Source Round 12 Part C), with ORANGE-022 (`claude/source-orange-r12` @ `4f0ab30`) merged |
| Decisions | D139 |
| Review | No consensus change. For Black (Part D): the disk-reserve redesign replaces the ranks of B4 (D132) and B5 (D137). |

## Summary

This fixes Round 12's four Source findings and GUI-018. It adds no size,
count or depth limit, and no heavier ledger is refused (D115). C-6 and
D133 still hold, and B4's and B5's cases still pass under the new rule.

- **RED-031, RED-032, RED-033.** The ranks at the disk reserve are gone.
  A fetch with no room takes room only for bytes that have arrived, cut
  off the end of a kept file that is departed, stalled, stale, or would
  finish later at measured rates (bytes a second). A claimed size never
  decides; a sender keeps room only by delivering faster than the fetch
  it holds back would finish, and a cut file resumes where it was cut.
- **ORANGE-022.** Kept: the fetch that would finish first keeps the room,
  and between equals the one nearer its end does, so two slow fetches no
  longer throw each other's files away.
- **RED-034.** Drops while probing hold the pace only with nearly as much
  sent as at the first drops (seven eighths). Otherwise the probe halves,
  not below where it began, and goes on rising.
- **GUI-018.** `poo wallet follow` sends one `replace` request, applied
  under `policyMu`, so the series is never off node.json in between.

## What changed

- `pkg/p2p/chase.go`:
  - `progress.partsRate` and `partsAt` (measured at each save from
    `partial.seen` and `partial.dur`) replace `partsSlow`, `partsLast` and
    `partsWait`.
  - `reclaimParts`, `partsRank`, `finishesFirst`, `partsTurn` and
    `partsMoving` are gone. New: `takeRoom`, `takeFrom`, `finishIn`,
    `partsStale`, `mayTake` and `partial.cut`.
  - `fetchParts` asks when `mayTake` allows, cuts room part by part as
    parts arrive, and removes an empty waiting file.
- `pkg/p2p/pace.go`: `reqWindow.peak` and `start`; `dropped` halves a
  probe on drops with less than seven eighths of `peak` sent.
- `pkg/nodepolicy/follow.go`: `ReplaceFollow`.
- `pkg/rpc/view.go`: `FollowRequest.Replace`; remove and replace together
  are refused.
- `pkg/cli/wallet.go`: `walletFollow` uses one replace request, and
  `ReplaceFollow` with no node.
- Docs: D139, `docs/modules/p2p.md`, `rpc.md`, `nodepolicy.md`,
  `wallet.md`.

## Tests added

- `pkg/p2p`:
  - `TestTakeFrom` replaces `TestPartsRankTurns`: departed, stalled and
    stale holders, RED-031 to RED-033, B4, and ORANGE-022 both ways.
  - `TestMovingTricklerJustUnderCannotHoldTheReserve` (RED-031's shape,
    end to end). It also passes on the old code, so it guards the shape
    without reproducing the finding; the unit cases do that.
  - `TestSecondOutageWhileProbingKeepsProbing` (RED-034).
  - `TestLargeEntryWaitsForDiskSpace` allows one repeated offset: a part
    that arrived with no room is asked for again.
- `pkg/nodepolicy`: `TestReplaceFollow`. `pkg/rpc`: replace cases in the
  view test (GUI-018).
- Ten runs in a row each, all passing: `TestBlackSlowHonestCannotReclaimFromTrickler`
  (B5), `TestTwoSlowFetchesBothFinish` (ORANGE-022, without `-race`), the
  three trickler tests, `TestSlowSenderIsReachedOverManyChases`,
  `TestChaseBelowSenderBudgetIsReached`, `TestLargeEntryWaitsForDiskSpace`,
  `TestDiskWaitTakesRoomFromDepartedSenders` and
  `TestLargeClaimIsPacedOnDisk`.

## What is left

- A sender that delivers as fast as the honest fetch can hold it back.
  That is a bandwidth contest and costs it the honest sender's bandwidth.
- Two outages at the same low load still hold the pace at about three
  quarters of that load, as D133 says of a real budget.
- staticcheck and govulncheck are not installed here; Grey runs them.
