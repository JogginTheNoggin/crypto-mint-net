# BLUE-023: Engine memory limits are bounded; learned candidates give way; a lowered book is applied; a refused add changes nothing

| Field | Value |
| --- | --- |
| Fixes | RED-063 (medium), RED-064 (medium), RED-065 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `b997eb0` (Source Round 22 Part C), with ORANGE-032 (#136) merged in |
| Code | `f307480` |
| Decisions | D160 |
| With | ORANGE-032 (PR #136): this builds on its per-range index and imported set, so the PR carries its commit |
| Review | No consensus change. For Black: the three engine bounds (`ties` above all) and the peer book's give-way order; both are node policy. |

## Summary

- **RED-063.**
  - `seen_size`, `max_pending` and `ties` are D159 bounds (Red's ranges),
    so `Validate`, `doctor`, the settings import and the GUI form refuse
    an out-of-range value.
  - The seen list grows with its items, so even the ceiling costs nothing
    at start.
  - A new test sets every integer setting to its type's maximum: each one
    is refused, or the node opens without a panic.
- **RED-064.**
  - When the book or a range is full, a learned candidate that the node
    has never reached gives way to a new address. One whose last try
    failed goes at once, and an untried one after `p2p.candidate_seconds`
    (new; one day by default).
  - The owner's own add takes any learned place.
  - Records the owner gave or labelled, and ones the node reached, stay.
  - `Book.Fit` applies a lowered `max_book` when the node starts.
- **RED-065.** `add` decides which record gives way before it deletes,
  so a refused add leaves the book as it was.

## Consensus

None.

- The peer book is node policy. Which record gives way decides only which
  peers a node knows; nothing limits a ledger it can reach.
- `seen_size` and `max_pending` bound memory. A dropped item can be sent
  again.
- `ties` was already a cap on the off-chain entries kept that tie with
  the selected weight. The ceiling stops a value that turned the cap off,
  and a forgotten tie can be sent again. It bounds no weight, depth or
  length of a ledger. Black's heavier-ledger test now runs with all three
  at their floors and at their ceilings.

## Tests

- `pkg/peers`:
  - `TestRefusedAddChangesNothing`
  - `TestLearnedCandidatesGiveWay`
  - `TestFitLoweredBook`
  - `TestGiveWayQueuesStayBounded`
  - `BenchmarkAddFullBookLearned` (about 3.5 µs an add at 65,536 records)
- `pkg/node`:
  - `TestEveryIntegerSettingAtItsMaximum`
- `pkg/engine`:
  - `TestSeenGrowsWithItems`
- `pkg/nodepolicy`:
  - `TestLimitsBounds`, `TestLimitsSet` (the new bounds)
- `pkg/p2p`:
  - `TestBlackBundleSettingsCannotRefuseHeavier` (the engine limits at
    their floors and ceilings)

## For the GUI

`p2p.candidate_seconds`, `seen_size`, `max_pending` and `ties` are new in
`nodepolicy.Bounds()`, for the GUI's settings form.
