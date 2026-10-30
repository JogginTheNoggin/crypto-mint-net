# GREEN-017: Four rough edges in `poo grind`

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `29b08ce` (code `a92c2ca`) |
| Area | `pkg/grind`, `pkg/watch` |
| Effort | small |
| Decided by | Grey |

## Summary

Green used the M11 grinder as a person would and compared it with GREEN-016
and jerry's request (detect hardware, manual, scheduled or auto work,
targeting, picking the hash from the series, one grinder per hash). The
request is met. Four rough edges remain:

1. `grind start -device gpu:0` accepts the job and fails it later, while `work -device gpu:0` refuses at once.
2. Job-wide watch events (start, end) carry a zero series and pool 0, so `watch -kind grind -series S` never sees a job end.
3. `grind run` emits two `start` events per job.
4. The log line reads `grind: grind job started`.

This is a proposal only. Item 1 goes against `TestJobLimits`, which says
"a missing device fails the job, not the start", so it is Grey's call.

## Problem and idea

**1. A device that cannot work is found only after the job starts.**
`poo grind start tip -device gpu:0` prints a job in state `running`, and
the job then ends `failed` a moment later. A person who starts a job and
walks away finds out hours later. `poo work -device gpu:0` refuses at once.
The idea is a check in `Manager.Start`: for each hash the job would grind
(the hashes of the followed pools, narrowed by `-series`), run `Choose`
with the given `-device` and `-threads`, and refuse the start if one
fails. Green prototyped this as `checkPlans` (it is about 20 lines) and
then removed it, because `TestJobLimits` encodes the opposite on purpose.
A scheduled job (`-hours`) should still be accepted when its device is
missing now, since the device may appear later. So the check would run
only for a job that starts now.

**2. Job-wide events look like pool 0 and are dropped by filters.**
`Hub.PublishGrind` sends `start` and `end` with a zero series and pool
`0`, and a filter passes them only when it is empty. A watcher of one
series, which is the GUI's usual view, sees `found` and `progress` and
never sees `end`, so it cannot tell when the job is over. The JSON also
shows `"pool": 0` and `"weight": "0"`, which read as a real pool. The idea
is to omit `series`, `pool` and `weight` on job-wide events (pointer or
`omitempty`), and to let a job-wide event pass a series or pool filter
when the job's spec includes that series or pool.

**3. Two `start` events per job.** `grind run` prints one job-wide
`start` and then one per pool, both of kind `start`. Calling the per-pool
one `pool` (or `plan`, since it carries the chosen plan) lets a reader
switch on the kind alone.

**4. The log message repeats its component.** The `grind` prefix is
already added by `pkg/logging`, so the message can be `job started`,
`job ended` and so on, as the other components do.

## Testing

Item 2 was checked with a test: a series-filtered watcher got the
per-pool `start`, `found` and `progress`, and no `end`. Each item needs
one test like it.

## Risks

Item 1 changes `TestJobLimits` and the GUI's error path, because the
start itself can now fail. Item 2 changes the watch JSON, which the GUI
reads, so GUI/Grey should hear of it.
