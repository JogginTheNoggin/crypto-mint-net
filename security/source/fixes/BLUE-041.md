# BLUE-041: View-only fails closed; data from earlier builds is a fresh start; held offers past 16 layers read filled

| Field | Value |
| --- | --- |
| Fixes | RED-122 (low), RED-123 (info, in part) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `b94d861` (Source Round 40 Part C, after the PoON label change D206), with GREEN-065 (#356) and ORANGE-050 (#360) merged in |
| Code | `3ab7875` |
| Decisions | D207 |
| With | GREEN-065 (#356, usage spacing and `_poon`), ORANGE-050 (#360, a test-only benchmark fix). |
| Review | No consensus change. For White: `POON_VIEW_ONLY` reads more values as on (none as off that were on); no old name is read, as jerry set. |

## Summary

- **RED-122.** `POON_VIEW_ONLY` is off only for empty, 0, false, no or off.
  The guide says data from earlier builds is a fresh start.
- **RED-123.** (1) held offers count accepts past `MaxAcks`; (2) and (3)
  won't fix: old-build data is not read.

## Consensus

None.

## Tests

- `pkg/cli/blue041_test.go`: `TestBlue041ViewOnlyFailsClosed`
- `pkg/bounty/blue041_test.go`: `TestBlue041HeldFilledPastMaxAcks`
- Both fail on the code before the fix.
