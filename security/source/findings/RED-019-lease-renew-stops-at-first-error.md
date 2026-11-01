# RED-019: `poo grind run` stops renewing its lease after one failed request

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 0fd59fa (code e99e3e4, Source Round 7 Part B; reported first by GUI Red) |
| Component | pkg/cli/grind.go (renew, grindStart) |
| Surface | CLI / RPC |
| Fixed by | `claude/project-thread-ggo8dt` @ 58e4e42 (BLUE-008) |

## Summary

`poo grind run` starts a job with a 15-second lease and renews it every 5
seconds:

```go
// pkg/cli/grind.go
func renew(ctx context.Context, cl *rpc.Client, id int) {
    ...
    case <-t.C:
        if j, err := cl.GrindRenew(id); err != nil || j.Done() {
            return                               // one error ends the renewals
        }
}
```

A single failed request (a timeout while the node is busy, a dropped
connection, a 5xx while the node reloads) ends the loop for good. The lease
then runs out within 15 seconds, the node stops the job ("its lease ran out:
whoever followed it is gone"), and the follower, which is alive and still
streaming events, sees the job end and exits as if it had finished. I called
this fail-safe in Round 6 (a lease only shortens a job); GUI Red is right
that it is also a false stop for a follower that is alive. Round 6 looked at
a stop of the *follower*, not at a follower that is still there.

## Reproduction

Read from the code. A test double for `rpc.Client` whose `GrindRenew` fails
once: the renew goroutine returns and no later call is made.

## Impact

Low. The job ends early and says why; a restart fixes it. It happens under
load, which is when the owner most wants the job to keep going.

## Recommendation

On a transport error or a 5xx, retry (the ticker is already there) until the
lease would have run out; end only on "no such job" (404) or a job that is
done. Log the first failure. Also have the follower print the end reason it
receives, so "its lease ran out" is not shown as a normal end.

Test: `GrindRenew` failing once then succeeding keeps the job alive.

## Resolution

Fixed by BLUE-008 (decision D120). `renew` stops only for a job that is
done or that the node no longer knows (`rpc.CodeNotFound`), or once the
lease would have run out since the last success. Any other failure is
tried again at the next tick. The first failure in a row is written to the
invocation's error output (`errOf`), so it shows in the GUI terminal as in
a shell. The end event already carries the job's reason, which
`poo grind run` prints. Test: `TestRenewSurvivesAFailure` (`pkg/cli`).
