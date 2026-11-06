# GREEN-042: Grind -series help, and import plans that show values

| Field | Value |
| --- | --- |
| Status | adopted (D151) |
| Based on | `main` @ `b17e892` |
| Area | `pkg/cli` (`addClientSeries`, `grind start`/`run`, `policy import` plan output), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none: help text and command-line output only |
| Decided by | Grey (Black may skip it) |

## Summary

`grind start` and `grind run` said `-series` defaults to "the only
followed series", but a job without `-series` grinds every followed
series. The help now says so. Trying the new `policy import` (D149) as
a user, Green also found three things in its plan: added and removed
lines showed only a field name (`+ k`), set items showed as
`grind.jobs.[{...}]`, and "dry run: ... add -apply" was printed even
when there was nothing to change. The plan now shows values, set items
read `p2p.peers: 10.0.0.1:8646`, and the dry-run line appears only when
there is something to apply.

## Idea

- `addClientSeries(fs, usage)` is `addClient` with its own `-series` help. `addClient` calls it with the old text, so every other command is unchanged. `addJob` passes "default: every followed series; -pool and -serial need one, and take the only followed series", which is what `grind` does.
- `planLine(field, value, short)` formats one added or removed line. A set item `a.b.[X]` becomes `a.b: X`; a field with a value becomes `field: value`; a field without one stays as it was. `printPlan` uses it for both `+` and `-` lines. Changed lines (`~`) already showed was and now.
- The "dry run" line is printed only if some plan is not empty. Importing a node's own export prints "nothing to change" and stops there.
- `poon.md`: the `grind start` row says `-series` defaults to every followed series.

This is one helper with a usage argument, not a new flag set, so a later
command whose `-series` default differs takes the same path.

## Testing

- `TestKindOnlyOnWatch` also checks that `grind start` and `grind run` help says "every followed series" and not "default: the only followed series".
- `TestPlanLinesShowValues` covers `planLine`, and checks that importing a node's own export prints "nothing to change" with no "dry run" line.
- `TestPolicyExportImport` now expects `+ series: ID`.
- Manual run of export, plan, `-prefer-file`, `-apply`, a second import, and `-json`. The JSON output is unchanged.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

A script that parsed the text plan for `series.[` would need to change.
The plan's JSON (`-json`), which the GUI reads, is unchanged.

## Also noticed

- `docs/FEATURES.md` covers every command in `poon help` except `holdings`, the older form of `unspent -serial`. Grey can decide whether to list it.
- Still open from GREEN-020: errors name `until_from` where the person typed `-until-from`, and no command renews a lease.
