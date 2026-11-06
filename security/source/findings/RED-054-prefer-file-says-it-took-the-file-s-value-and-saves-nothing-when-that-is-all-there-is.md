# RED-054: `-prefer-file` says it took the file's value, and saves nothing when that is all there is

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 20 Part B, on `main` @ `522e891` (`Plan.Empty` is from D149) |
| Area | `pkg/nodepolicy` (`Plan.Empty`), `pkg/cli` (`importBundleCmd`, `importOne`), `pkg/rpc` (`importPolicy`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ 63e37ed (BLUE-021) |

## What is wrong

With `-prefer-file`, a value where the file and the node differ goes into
`Plan.Conflicts` with `took: "file"`. `Plan.Empty()` looks at `Added`,
`Removed` and `Changed` only, so a plan whose only change is such a
conflict is "empty": the command prints `nothing to change`, prints no
digest and no "dry run" line, and `-apply` saves nothing.

## How I checked it

A node with `issuers.any: false` and a settings file with it `true`:

```
$ poon settings import s.json -prefer-file -apply
  read first: ... issuers.any
  ! issuers.any: took the file's true (was false)
  nothing to change
$ grep any node.json
    "any": false
```

`poon policy import pol.json -settings -prefer-file -apply` does the same.
When another setting also changes (I added `p2p.max_peers`), the same
command saves and `issuers.any` becomes `true`. So the result depends on
whether something else differs.

## What could happen

The user imports a hardening file with `-prefer-file` (lower limits, a
shorter timeout) and reads "took the file's". Nothing was applied, and
nothing says so: the node keeps running with the old, weaker value. It
fails in the safe direction for a hostile file, but not for a good one.

## Suggested fix (Blue's call)

Make `Empty` count conflicts that took the file's value, or have the
preview say "nothing saved" next to a conflict that was not taken. Add a
test with a conflict as the only difference.

## Not tested

The same through a running node (`POST /v1/policy/import`); it uses the
same `Plan`, so I expect the same.

## Resolution

Fixed (D156). `Plan.Empty` counts a conflict whose `Took` is `file` as a
change. A plan made of such conflicts alone prints its digest and "dry
run", and `-apply` saves it, both offline and through a running node
(`POST /v1/policy/import` uses the same `Empty`). A conflict that kept
local's value is still no change. Tests:
`TestPreferFileConflictAloneIsAChange` (pkg/nodepolicy) and
`TestPreferFileAppliesALoneConflict` (pkg/cli).
