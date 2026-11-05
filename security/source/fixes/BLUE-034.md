# BLUE-034: Exports end where their header says; import calls bounded, dry run, partial totals on a stop; pre-D186 hellos named

| Field | Value |
| --- | --- |
| Fixes | RED-097 (low), RED-098 (low), RED-099 (low), GUI-031 (low, Source part) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `3c4ae00` (Source Round 33 Part C), with ORANGE-043 (#265) and GREEN-058 (#259) merged in |
| Code | `8606137` |
| Decisions | D189 |
| With | ORANGE-043 (PR #265), carried: the streaming import that RED-098 and GUI ORANGE-027 ask for. GREEN-058 (PR #259), carried: it changes the same `ledgerport.go`. GUI-031's GUI part (drawing the partial summary) is GUI Blue's. |
| Review | No consensus change. For Black: tools and RPC limits only; the first item of an import call has no size bound, as an entry has none (D144). For White: D186 stands; pre-D186 builds still cannot connect, now with a named error and a release note. |

## Summary

- **RED-097.**
  - Later pages are asked only up to the header's `to`.
  - An empty or non-continuing page is a switch.
  - The file must end at `to`, and at the header's tip.
- **RED-098.**
  - The streaming read comes from ORANGE-043.
  - `decodeImport` bounds a call to 256 items, and 8 MiB past the first.
  - `-dry-run` checks a file without a node.
- **RED-099.** `wire.ErrOldHello` names the old layout, and the
  CHANGELOG notes it.
- **GUI-031.**
  - `importLedger` checks the context between batches.
  - On a failure or a stop, the reply carries the totals so far, `sent`
    and `stopped`, with a non-zero exit.

## Consensus

None.

## Tests

- `pkg/cli/blue034_test.go`: `TestBlue034ExportEndsWhereItsHeaderSays`, `TestBlue034ImportReportsWhatWasKept`, `TestBlue034ImportPrintsPartialTotals`, `TestBlue034DryRun`
- `pkg/rpc/blue034_internal_test.go`: `TestBlue034ImportCallBounded`
- `pkg/wire/blue034_test.go`: `TestBlue034OldHelloNamed`
- The export, import-stop and call-bound tests fail on the code before the fix.
