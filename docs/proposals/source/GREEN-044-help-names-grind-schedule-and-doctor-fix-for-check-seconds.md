# GREEN-044: Help names grind schedule, and doctor's fix for check_seconds

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `4240d32` |
| Area | `pkg/cli` (`poo help`, completion, `poo doctor`) |
| Effort | small |
| Consensus | none: help text, completion and a doctor hint only |
| Decided by | Grey (Black may skip it) |

## Summary

GUI Green found that the grind line in `poo help` leaves out `schedule`.
Completion builds its list from that line, so `poo grind sch<TAB>`
offered nothing either. The line now names `schedule`. Completion now
reads grind's own usage, as it already does for `wallet` and `sim`, so a
new grind subcommand needs no second list. A test checks that the `poo
help` line names every subcommand.

While trying D153 as a user, Green also found that `poo doctor` gives a
wrong fix for `p2p.check_seconds: -1`. The fix said "lower it in PATH
(poo grind schedule set, or edit the file)". The value is not too high,
and no grind command writes it. The fix now says "set p2p.check_seconds
to 3600 in PATH".

## Idea

- `pkg/cli/main.go`: `poo grind devices|bench|start|run|schedule|jobs|stop`.
- `subcommands("grind")` reads `grindUsage`, like `wallet` and `sim`. Its list is now devices, bench, start, run, jobs, stop, schedule and help.
- `poo doctor`: a lowered value with no `FixArgs` gets the fix "set FIELD to NOW in PATH". A value that has a command, `grind.layer_rate`, keeps it (GREEN-035).

## Testing

- `TestHelpLineNamesEverySubcommand`: the `poo help` line for `grind` and for `sim` names every subcommand of its own usage, and `grind sch` completes to `schedule`. It fails on `main`.
- `TestDoctorLoweredWithoutCommandNamesTheValue`: `check_seconds: -1` gets "set p2p.check_seconds to 3600 in PATH" and no `fix_args`.
- `TestComplete`, `TestProbeOnlyReads` (which now also probes `grind schedule -h`) and `TestDoctorLoweredHasFixArgs` pass.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

None known. The GUI reads `fix_args`, which is unchanged. It shows the
fix text as given.

## Also noticed

- On a whole-series follow, the import plan prints "follows all 2 pools". "follows both pools" or "follows every pool (2)" would read better. Text only; Grey's call.
- Still open from GREEN-020: errors name `until_from` where the person typed `-until-from`, and no command renews a lease.
