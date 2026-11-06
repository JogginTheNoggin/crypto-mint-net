# GREEN-035: Leave room for user jobs, and change only what a schedule edit changes

| Field | Value |
| --- | --- |
| Status | adopted (D135; item 1 as `grind.UserJobs`, not a lower cap) |
| Based on | `claude/m10-watch` @ `0a6cc94` |
| Area | `pkg/grind` (`Manager.Schedule`, `MaxJobs`), `pkg/cli` (`grind bench`, `doctor`), `pkg/node` |
| Effort | small to medium |
| Consensus | none: the node's own grinding and the command line only |
| Decided by | Grey (Black may skip it) |

## Summary

Green used Round 10's grind limits and RED-025's lowered-value warning as
a person would. The limits validate everywhere, and an old `node.json`
with `layer_rate: 1000` now loads with a warning, as D128 says. Four rough
edges remain:

1. **A full schedule blocks every other job.** The schedule may hold `MaxGrindJobs` (16) jobs, which is also how many jobs a node runs at once. With 16 scheduled jobs, every `grind start`, every `grind run` and the GUI's grind button fail with "at most 16 jobs at once". This happens even while all 16 scheduled jobs are only *waiting* (outside their hours, or with nothing pending).
2. **Every schedule edit restarts every scheduled job.** `Manager.Schedule` stops all policy jobs and starts them all again. Adding one job restarts the other fifteen. Their tries and rate start from zero, and the jobs list fills with entries that say only `stopped` (32 of 48 after adding 16 jobs one at a time). GREEN-020 item 6 is the same point.
3. **`grind bench -threads 2000` printed `[]` and exited 0.** Every device was skipped as unable to grind, so nothing said why. (Prototyped.)
4. **The lowered-value warning had no component.** It was logged as `warning: node policy value lowered ...`, while every other line names its component (`node: started ...`). (Prototyped.)

## Idea

**1. Room for user jobs.** Either of these would fix it, and Grey can pick:

- (a) Count only *running* jobs against `MaxJobs`, and let waiting jobs not take a slot until they work. This is the most natural, but a waiting job that wakes needs a slot too, so it may itself have to wait for one ("waiting for a free slot").
- (b) Keep counting every job, and cap the schedule below `MaxJobs`: for example 12 scheduled, leaving 4 for user and follower jobs. This is simpler, and `nodepolicy.Limits()` already tells the GUI the cap.

Green leans to (b) for its simplicity.

**2. Apply the difference.** When only the jobs change, `Schedule` could
stop the policy jobs whose `GrindJob` was removed and start the ones that
were added, leaving the rest running. When the defaults change (`device`,
`threads`, `layer_floor`, `layer_rate`), a restart is still right,
because the plans change. In that case the stopped jobs' reason should
say "the schedule changed", so the jobs list explains itself.

**3. Bench (prototyped).** `grind bench` keeps the last reason a device
was passed over. If nothing was measured, it returns that reason:
`poon: grind: threads must be 0 to 1024`. Test:
`TestGrindBenchNothingMeasured`.

**4. Log line (prototyped).** `node.Open` logs the note with the `node`
component: `node: warning: policy value lowered file=node.json note=...`.

**5. A fix command in `doctor` (proposal).** The doctor's lowered-value
warning has a fix in words, but no `fix_args`. If `Policy.Lowered` held
the field and the value it was lowered to, not only a sentence, the
doctor could offer `poon grind schedule set -layer-rate 50` as `fix_args`
(GREEN-022). That changes `Lowered`'s type, so it is left to Grey.

## Testing

- Items 3 and 4: `TestGrindBenchNothingMeasured`, and by hand the log line on a node started with `layer_rate: 1000`.
- Items 1 and 2 would need a test that fills the schedule and still starts a user job, and one that adds a job and checks the others kept their IDs.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

Item 1(a) changes when a waiting job can start working. Item 2 changes
the job IDs that a schedule edit reports in `started`, which the GUI
reads. GUI/Grey should hear of either.
