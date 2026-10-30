# RED-014: MaxJobs is checked and then used without holding the slot, so concurrent starts exceed it

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | `claude/m10-watch` @ 9972137 (code 84d3e69, Source Round 6 Part B) |
| Component | pkg/grind/jobs.go (Manager.Start), pkg/rpc/grind.go (grindStart) |
| Surface | RPC |
| Fixed by | |

## Summary

`Manager.Start` counts the running jobs under `m.mu`, **unlocks**, runs
`checkPlans` (which asks the engine for the pools and chooses a device), and
only then locks again to insert the job. It does not count again:

```go
m.mu.Lock(); ...; if running >= MaxJobs { ...ErrTooManyJobs }; m.mu.Unlock()
if s.Hours == "" { if err := m.checkPlans(s); ... }   // slow, no lock
m.mu.Lock(); ...; m.jobs[j.ID] = j; m.mu.Unlock()      // no recount
```

Any number of requests that pass the first check before one of them inserts
all start a job. The wider the gap (`checkPlans` runs for every job without
`hours`), the more get through. R5B's review took the bound as enforced;
it is not under concurrency.

## Reproduction

A temporary test in `pkg/grind` (not committed): on the test fixture, 300
goroutines released together call `Start(Spec{Target: "tip"})`; five rounds,
`MaxJobs` is 16:

```
round 0: 300 concurrent starts, 49 accepted, 49 running
round 1: 36 / 36    round 2: 50 / 50    round 3: 27 / 27    round 4: 33 / 33
```

Each accepted job has its own goroutine and its own thread count (up to
`MaxThreads`, 1,024; the grinder clamps CPU threads to the core count per
job). Over HTTP the same happens with parallel `POST /v1/grind` requests.

## Impact

Low. The bound is a safety limit for the owner's own CPU, and only the
control-token holder can start a job; there is no privilege boundary
crossed. It matters because D113 adds `Schedule`, which calls `Start` from a
handler that runs concurrently with the user's own starts, and because the
limit is documented and tested (`TestJobLimits`) as if it were exact.

## Recommendation

Reserve the slot under the same lock that counts it: insert the job (state
`running`, with its cancel) before `checkPlans`, and remove it again if the
check fails; or count once more after the second `Lock` and return
`ErrTooManyJobs` there.

Test: the one above, asserting that no more than `MaxJobs` jobs are ever
not-done, and that `Schedule` and `Start` together respect it.
