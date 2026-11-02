# ORANGE-022: Turn-taking measured; the fetch that would finish first keeps the room

| Field | Value |
| --- | --- |
| Status | superseded by D139 and D140; finding upheld (D141) |
| Based on | `claude/m10-watch` @ `bc2e4bc` |
| Area | `pkg/p2p` (`partsRank.below`, new `finishesFirst`), `.github/workflows/ci.yml` |
| Effort | small |
| Decided by | Grey, with Black (a D115 matter) and Red |

## Summary

I measured the cost of D137's turn-taking at the disk reserve.

**Two honest slow fetches never finish.** I ran two honest senders of
different large entries, both slow: each chase takes in one 64 KiB part,
then a request is lost. The room above the reserve fits about one entry.
- Under D137, each waiting fetch gets its turn after two backoff steps.
  It wins whenever its last chase took in at least as much as the
  holder's, and with the same pace that is every time.
- The turn deletes the holder's whole parts file. The two fetches take
  turns deleting each other's files and starting again from offset 0.

| Two slow fetches, room for about one | D137 (`bc2e4bc`) | This change |
| --- | --- | --- |
| 2 MiB entries, 2.5 MiB room, 150 s | neither reached its end; 6 restarts each; about 3.5 MiB taken in each | both reached their end, at 69 s and 125 s; no restarts |
| 2 MiB entries, 3 MiB room, 150 s | neither; 3 restarts each; about 3.8 MiB taken in each | both, at 69 s and 106 s; no restarts |
| 1 MiB entries, 1.5 MiB room (the new test) | neither in 120 s; 13 restarts each; 134 parts taken in each, 8× the entry | both, in 17–24 s |

That is a case D137's "What is left" says no longer exists: a fetch
that waits for good (D115), here two of them. It also wastes bandwidth
without limit: each turn fetches bytes the next turn throws away.

**The fix.** When a slow waiting chase has its turn against a slow
file, it takes the room only if it would finish first. Each fetch's
estimate is its bytes still to come over what its last chase took in.
- A chase that took in nothing yet still has its turn, as D137 requires
  (B5's lost first request).
- Two fetches at the same pace: the one nearer its end keeps the room
  until it finishes, then the other gets it. Neither starts over.
- A trickler keeps the room only while its own pace would finish first.
  It then has to finish, and the hash decides. If it slows down, its
  estimate grows and the next turn takes the room.
- Moving files and departed senders rank exactly as before.

No consensus change. It changes only which kept parts file gives way at
the disk reserve, as D137 did.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- Two `largeSender`s, the fake peers of the parts tests, connect to one
  node. Each names a different random large entry, and each leaves
  every second parts request unanswered.
- The node runs with `RequestTimeout` 300 ms, `Backoff` 2 s and
  `PartsReserve` 1.
- `SetDiskSpace` reports 100 MiB in all, so the reserve is 2 MiB, plus
  the given room minus the bytes in the parts directory.
- "Reached its end" means the sender was asked for its last part.

| Room above the reserve | Run | Sender 0 | Sender 1 |
| --- | --- | --- | --- |
| 2.5 MiB, D137 | 150 s | 132 parts asked, 6 restarts, got as far as part 12 of 32 | 110 asked, 6 restarts, part 10 |
| 3 MiB, D137 | 150 s | 122 asked, 3 restarts, part 19 | 120 asked, 3 restarts, part 19 |
| 2.5 MiB, this change | 150 s | 63 asked, 0 restarts, end at 69 s | 63 asked, 0 restarts, end at 125 s |
| 3 MiB, this change | 150 s | 63 asked, 0 restarts, end at 69 s | 63 asked, 0 restarts, end at 106 s |

Half the parts asked are left unanswered on purpose: 63 asked is the
32 parts plus 31 lost requests. A single
fetch at this pace needs about 69 s, so the first fetch is not slowed
at all and the second follows it.

## Idea

- **`finishesFirst(x, y)`:** reports whether x's bytes to come over x's
  last intake are less than y's. It compares the two products exactly
  in 128 bits. A fetch with no intake yet never finishes first.
- **`partsRank.below`:** for a turn against a slow file, the condition
  `b.partsLast >= a.partsLast` becomes
  `b.partsLast == 0 || finishesFirst(b, a)`. Everything else is
  unchanged.
- **Tests:**
  - `TestPartsRankTurns` gains three cases:
    - at the same pace, the holder nearer its end keeps the room;
    - a turn nearer its end takes it;
    - a holder that slowed down gives it up.
    The existing cases are unchanged and pass.
  - `TestTwoSlowFetchesBothFinish` is the measurement above, scaled
    down (1 MiB entries, `Backoff` 500 ms). It fails on `bc2e4bc` after
    2 minutes and passes in 17–24 s (3 runs). It is skipped under
    `-short` and `-race`, so it is added to CI's long-test step.
  - `TestBlackSlowHonestCannotReclaimFromTrickler`,
    `TestTricklerCannotHoldTheReserve` and
    `TestTricklerJustUnderCannotHoldTheReserve` still pass.

## For Black and Red

- **D115.** Under D137, two honest fetches at about the same slow pace,
  with room for one, never finish. This change removes that case.
- **The trickler.** D137 asked a trickler to send as much each chase as
  the honest sender does. Now it must keep a pace that would finish its
  claim first, and then finish it. A trickler that claims a huge size
  can never finish first against an honest entry (B4's argument). One
  that claims a small size has to deliver it, and then the hash decides.
  How long it can hold the room is bounded by its own claim at its own
  pace, and every turn re-measures it. Red may want to attack this with
  a trickler that keeps just ahead of an honest fetch's estimate, then
  slows down near its end.
- **What is still lost on a turn.** A turn still deletes the whole file
  it takes. Trimming only the bytes needed, from the end, would keep the
  loser's prefix. With this change turns are rare between honest
  fetches, so I did not build it.

## Cost and risk

- About 30 lines and the tests.
- `finishesFirst` is two 128-bit multiplications, run only when a slow
  waiting chase asks for room.
- The full suite passes: `go test -race ./...`,
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun`,
  `TestTwoSlowFetchesBothFinish`, and `GOEXPERIMENT=simd` for `pkg/p2p`.

## Prototype

On `claude/source-orange-r12`; see the commit "ORANGE-022".
