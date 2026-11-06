# BLUE-040: A dropped bounty payment sent again instead of replaced; the bounty memo names the offer by 16 bytes

| Field | Value |
| --- | --- |
| Fixes | RED-120 (low), RED-121 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `21c34c8` (Source Round 39 Part C), with GREEN-064 (#343) merged in |
| Code | `a29af78` |
| Decisions | D203 |
| With | GREEN-064 (#343), carried: it changes the same `cli/bounty.go`. ORANGE-049 (#340) is docs only. |
| Review | No consensus change. For White: the memo's frozen start `bounty ` is unchanged; what follows is now 32 hex characters (16 bytes of the offer id) instead of 8. No leg state added, no new flag. |

## Summary

- **RED-120.** `poo bounty pay` refreshes and resends a dropped earlier
  payment of the claim, then records it; it never pays anew over one.
- **RED-121.** `BountyMemoID` (16 bytes) in the memo and in `sentFor`.

## Consensus

None.

## Tests

- `pkg/cli/blue040_test.go`: `TestBlue040MemoID`, `TestBlue040DroppedIsResent`
- `pkg/cli/white034_test.go`: rows updated (dropped found to resend; 4-byte memo not found)
- `pkg/cli/bounty_test.go`: `TestBountyMarket` (a dropped leg asked about and recorded, not paid again)
- `TestBountyMarket` fails on the code before the fix (pay tries a new payment); the new file does not build against it (`BountyMemoID`, `sentFor`'s resend result).
