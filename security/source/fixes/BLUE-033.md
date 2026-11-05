# BLUE-033: Folded keys are the setting; kept settings limited and compacted; status caches node.json; imports keep them

| Field | Value |
| --- | --- |
| Fixes | RED-093 (medium), RED-094 (medium), RED-095 (low), RED-096 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `6260590` (Source Round 32 Part C), with ORANGE-042 (#249) and GREEN-057 (#247) merged in |
| Code | `290702d` |
| Decisions | D185 |
| With | ORANGE-042 (PR #249), carried: the one-pass insert that RED-095 asks for. GREEN-057 (PR #247), carried: doctor's unknown-settings checks, now with the folded keys. GUI ORANGE-024 (#248), taken: the variable series set grown in place. |
| Review | No consensus change. For Black: node.json is node policy. For White: settings over the keeping limits are lost on a save by this release, and the start warning names them (D185). |

## Summary

- **RED-093.**
  - A key the decoder matches by case folding is that setting (`fieldFor`).
  - It is saved under its own name, never kept as unknown.
  - It is named in `FoldedSettings` and warned of.
- **RED-094.**
  - Kept: at most 256 settings, 64 KiB each, 64 levels deep. One over a
    limit is named in `UnknownDropped`.
  - Kept values are written compacted.
  - Status caches node.json by file, size and time.
- **RED-095.**
  - The one-pass save is ORANGE-042.
  - Warnings name the first ten (`NameSome`).
- **RED-096.** `Merge` keeps the local node's kept settings.
- **GUI ORANGE-024.** `variableIDs` is a `sync.Map` with an atomic
  empty flag.

## Consensus

None.

## Tests

- `pkg/nodepolicy/blue033_test.go`: `TestBlue033FoldedKeyIsTheSetting`, `TestBlue033UnicodeFoldedKey`, `TestBlue033KeptCompacted`, `TestBlue033KeptLimits`, `TestBlue033ImportKeepsUnknown`. Each fails on the code before the fix.
- `pkg/rpc/blue033_internal_test.go`: `TestBlue033StatusCachesPolicy`
- `pkg/series/blue033_internal_test.go`: `TestBlue033VariableSetGrowsInPlace`, `BenchmarkBlue033AddVariableIDs`
