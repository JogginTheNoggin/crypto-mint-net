# GREEN-036: Doctor fixes that change the checked policy, and a live layer floor

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `claude/m10-watch` @ `1ae02ca` |
| Area | `pkg/cli` (`doctor`), `pkg/grind` (`Manager.Schedule`), `docs/modules/grind.md` |
| Effort | small |
| Consensus | none: the CLI and the node's own grinding only |
| Decided by | Grey (Black may skip it) |

## Summary

Green tried Round 11's GREEN-035 changes as a person would. A full
schedule of 16 waiting jobs now leaves room for a user job (job 17
started), and removing one scheduled job stops only that job, with the
reason "the schedule changed". Two problems remained, and both are
prototyped with tests:

1. **A doctor fix could change the wrong node policy.** `poo doctor -policy second` warned about `second`'s lowered `layer_rate` and offered `fix_args` `grind schedule set -layer-rate 50`. Run as printed, by a person or by a tool that runs `fix_args` (GREEN-022's purpose), that command changes the *current* policy (`default`), not `second`. Doctor then still warns about `second`, while `default` is changed. The same applies to the `init` and `node` fixes.
2. **Changing the layer floor or rate restarted every scheduled job.** `schedule set -layer-rate 20` stopped all 15 running scheduled jobs and started them again, resetting their tries and rate. But `Manager.work` reads `m.def.Floor` and `m.def.Rate` at each search, so the running jobs would use the new values anyway.

## Idea

**1.** When `-root`, `-policy` or `-dir` chose the policy, `doctorCmd`
adds `-dir DIR` (the data directory checked) to every `fix_args`. All
three fix commands (`init`, `node`, `grind schedule set`) take `-dir`.
Without those flags nothing changes, so the fixes stay short in the usual
case, and the current policy is the one that was checked.

**2.** `Schedule` keeps running jobs when only the device and threads
are the same, rather than when every default is the same. The floor and
rate are still stored in `m.def`, so the next search uses them. A changed
device or threads still restarts every job, because the plans change.
`grind.md` is updated.

## Testing

- `TestDoctorLoweredHasFixArgs` now runs `poo doctor -dir DIR -o json`, checks that `fix_args` ends in `-dir DIR`, runs the fix exactly as printed, and checks that the warning is gone. `TestDoctorOutputAndGrind` expects `init -dir DIR` when `-dir` was given.
- `TestScheduleAppliesTheDifference` now checks that a new `layer_floor` or `layer_rate` keeps both job IDs and updates the defaults.
- By hand: `poo doctor -policy second`'s fix changed `second` and left `default` alone.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

A `fix_args` with `-dir` is longer. A tool that compared `fix_args` to a
fixed list would see the extra two items, but only when the person chose
a policy explicitly.

Small note, not prototyped: the schedule accepts 16 identical jobs
(`add pending` 16 times), which all grind the same target. A warning on
`add` when the same job is already scheduled would catch a double click
in the GUI.
