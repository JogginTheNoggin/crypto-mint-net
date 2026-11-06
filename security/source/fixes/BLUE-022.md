# BLUE-022: Imported peers take a share of the book and give way; one text check; host names in any case; the digest binds the plan applied

| Field | Value |
| --- | --- |
| Fixes | RED-058 (medium), RED-059 (low), RED-060 (low), RED-061 (low), RED-062 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `8662766` (Source Round 21 Part C) |
| Code | `282571b` |
| Decisions | D158 |
| With | ORANGE-031 (PR #126) for RED-059's trim, and GREEN-046 (PR #124) for RED-062 points 1 and 2 |
| Review | No consensus change. For Black: the book's share for imported addresses and the eviction (RED-058); the peer book is node policy. |

## Summary

- **RED-058.**
  - Records known only from an imported file take at most a quarter of
    the book (`MaxUnlisted`).
  - An address the node learns takes the place of the oldest of them when
    the book or its range is full.
  - `poon peer remove -imported` takes them out in one step.
- **RED-059.**
  - The plan totals the blocks and says how much of IPv4 the ranges
    cover.
  - ORANGE-031 fixes the slow trim.
- **RED-060.** A host name's range is the name in lower case without the
  trailing dot, so a listed name in capitals is protected. The plan says
  host names are protected by name only.
- **RED-061.**
  - One text check (`pkg/plain`) for labels, node policy text and the
    terminal.
  - Direction marks and zero-width characters are refused, and table
    cells are escaped.
- **RED-062.**
  - `ImportIf` applies only the plan whose digest was checked, in the
    same call, offline or on the node.
  - The preview says the addresses will be dialled.
  - GREEN-046 fixes offline `-keep -label`.

## Consensus

None.

- The peer book and the tracker are local node policy.
- The share limits what a file puts in the book, never what the node
  learns: every book limit gives way to a learned address.
- No follow, ledger or series is limited.

## Tests

- `pkg/peers`:
  - `TestImportedAddressesGiveWay`
  - `TestRangeOfAHostName`
  - `TestLabelRefusesInvisibleText`
- `pkg/peerport`:
  - `TestImportTakesAShareOfTheBook`
  - `TestListedHostNameInCapitalsIsProtected`
  - `TestPlanCountsRangeBlocks`
  - `TestImportIfAppliesOnlyThePreviewedPlan`
- `pkg/plain`:
  - `TestTextAndSafe`
- `pkg/cli`:
  - `TestPeersForgetImported`
  - `TestTableEscapesText`
