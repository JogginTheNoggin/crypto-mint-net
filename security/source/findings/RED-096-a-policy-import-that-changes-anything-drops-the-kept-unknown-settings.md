# RED-096: A policy or settings import that changes node.json drops the kept unknown settings, so W-1's "nothing lost" does not hold for imports

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 32 Part B, on `main` @ `6260590` (D184, W-1) |
| Area | `pkg/nodepolicy/bundle.go` (`Merge`, `mergeInto`), `pkg/rpc/policyimport.go`, `pkg/cli/policyport.go` |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ 290702d (BLUE-033) |

## What is wrong

D184 keeps unknown settings through `Load` then `Save`. `clone` copies them
(`q.unknown = p.unknown`), but the policy `Merge` returns does not carry
them: for each of `Settings`, `Full` and `Settings+Full` the result of
`Merge(local, file, …)` had `UnknownSettings() == []`. `poon policy import`
and the settings import then `Save` that result, so the first import that
changes anything rewrites node.json without the settings a newer release
wrote, and without a word. (An import that changes nothing does not save,
so the settings stay.) That is the loss W-1 set out to prevent, by the
route an owner is most likely to take after going back a release.

## How I checked it

`pkg/nodepolicy` test (throwaway): `Load` a file with a top-level and a
`p2p` unknown setting; `Merge` with a file that sets `p2p.max_peers`:

```
{Settings:true}: unknown=[] empty=false maxpeers=7
{Full:true}:     unknown=[] empty=false maxpeers=7
```

I did not run the import commands end to end. Exports carry no unknown
settings (checked with a file holding top-level, `rpc` and misspelt keys:
`ExportPolicy`, full and not, contains none), which is the right way round.

## Suggested fix (Blue's call)

- Have `Merge` start from `clone(local)` for its result, or copy `unknown`
  across at its end; add a test that an import which saves keeps them.

## Not tested

The CLI import path.

## Resolution

Fixed in BLUE-033 (D185). `Merge` keeps the local node's kept settings,
and `clone` copies the dropped and folded lists. An import that saves
keeps them, for `Settings`, `Full` and both. Test:
`TestBlue033ImportKeepsUnknown`.
