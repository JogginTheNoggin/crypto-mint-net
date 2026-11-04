# BLUE-028: Far releases checked off the read loop and stoppable; ceiling-less series warned of; declined series shown

| Field | Value |
| --- | --- |
| Fixes | RED-082 (medium), RED-083 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `1a1c729` (Source Round 27 Part C) |
| Code | `60471a7` |
| Decisions | D171 |
| With | Not carried: Orange #188 (a doc; its 4 MiB default is left to Grey and jerry) and Green #189 (no overlap). |
| Review | No consensus change. For Black: (1) a release is checked by the same `CheckRelease`, only in a worker and with a context; a cancelled check leaves the part waiting. (2) The ceiling warnings change nothing a node accepts, and the `series_ceiling` default stays 0. |

## Summary

- **RED-082.**
  - The full check of a release from a peer runs in one worker per
    series, never in the peer's read loop. Coalescing (D169) is kept.
  - The walk uses the host's context, so stopping the node cancels it.
    A cancelled check is not an offence.
  - `POST /v1/series/release` and the RPC policy check use the request's
    context.
- **RED-083.**
  - An import plan lists ceiling-less series, and series above 4 MiB, in
    `unbounded`, and the text plan prints them.
  - The text plan prints declined series with the follow rules they took.
  - `series info`, `series followed`, `GET /v1/series` and `doctor` warn.
  - `series_ceiling` is settable with `poo policy limits -set`.

## Consensus

None.

## Tests

- `pkg/p2p/blue027_test.go`: `TestFarReleaseDoesNotHoldTheNode`
- `pkg/nodepolicy/blue028_test.go`: `TestImportWarnsOfUnboundedSeries`,
  `TestDeclinedSeriesCountsItsFollows`, `TestSeriesCeilingIsBounded`
- `pkg/cli/blue028_test.go`: `TestPlanTextNamesDeclinedAndUnbounded`
