# BLUE-039: The done file read from its offset only when the line there is the one named; bounty pay records the wallet's payment instead of paying twice

| Field | Value |
| --- | --- |
| Fixes | RED-118 (low), RED-119 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `1e71cf0` (Source Round 38 Part C), with GREEN-063 (#328) merged in |
| Decisions | D201 |
| With | GREEN-063 (#328), carried: it changes the same `book.go` and `cli/bounty.go`. ORANGE-048 (#329) is measurements only, nothing to fix. |
| Review | No consensus change: the market is outside the ledger (D196). For White: `bounties.json` gains `done_last` (an older release keeps it as a later field, W-28); `poo bounty pay` output gains `recorded`. No new flag or command. |

## Summary

- **RED-118.** `done_last` beside `done_size`: a start reads from the
  offset only when the line ending there carries that id, else reads all
  and logs it. A cut last line is reported; `archive` starts a new line
  after it and truncates a failed partial write.
- **RED-119.** A failed `Paid` save is rolled back in memory.
  `poo bounty pay` records a payment the wallet already sent for the
  claim instead of sending again; a payment sent only in part is refused.

## Consensus

None.

## Tests

- `pkg/bounty/blue039_test.go`: `TestBlue039TrimmedDoneFile`, `TestBlue039TornDoneFile`, `TestBlue039PaidSaveFails`
- `pkg/cli/bounty_test.go`: `TestBountyMarket` (a failed record, then pay again: recorded, one amount at the grinder)
- Each fails on the code before the fix with the change taken out (checked one by one); the bounty file does not build against it (`doneIDs` returns a scan).
