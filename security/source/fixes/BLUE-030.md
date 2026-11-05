# BLUE-030: A cold read fault never leaves the engine locked; a revert's cost counts the bodies it reads; poo status -counts

| Field | Value |
| --- | --- |
| Fixes | RED-085 (medium), RED-086 (medium) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `d1a9128` (Source Round 29 Part C) |
| Code | `255f269` |
| Decisions | D177 |
| With | GUI ORANGE-021: `poo status -counts`. Not carried: Orange #212 (mempool only) and Green #209 (no overlap). |
| Review | No consensus change. For Black: (1) failing the engine on a read path stops the node, as D174 does on a change path; it decides nothing. (2) The view cost decides only when an entry from a peer is checked (the revert budget paces, and never refuses for good), never whether it is valid. |

## Summary

- **RED-085.**
  - `Engine.under` takes the lock, always releases it, and fails the
    engine on a panic before the panic goes on.
  - `PreverifyTx`, `PreverifyEntry`, `PreverifyPage` and `BuildEntryOn`
    use it.
  - Once the engine has failed, they do nothing, or return `ErrFailed`.
- **RED-086.**
  - `unitsOf` also counts the distinct transactions an entry's inputs
    spend from.
  - A cold revert reads at most one body for each.
- **ORANGE-021 (GUI).**
  - `poo status -counts` prints one row of totals per followed series,
    with the same notes.

## Consensus

None.

## Tests

- `pkg/engine/blue030_test.go`: `TestColdReadFaultReleasesTheLock`
- `pkg/ledger/blue030_test.go`: `TestColdRevertCostCountsMakers`
- `pkg/cli/blue030_test.go`: `TestStatusBySeries`
- `pkg/p2p/serve_test.go`: `TestDeepForkSpendsRevertBudget` (recalibrated)
