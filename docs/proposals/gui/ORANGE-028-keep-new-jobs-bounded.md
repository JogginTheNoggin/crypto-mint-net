# ORANGE-028: Keep new jobs only up to twice the limits

| Field | Value |
| --- | --- |
| Status | adopted (GUI Round 38 Part A, the prototype as is) |
| Based on | main @ 7eb16ab |
| Area | `trim` in gui/jobs/jobs.go, plus a test in gui/jobs/jobs_test.go |
| Effort | small |
| Decided by | |

## Summary

Round 37 fixed a race in gui/jobs. A refresh that ended at once could be dropped before the page asked for its events. poon-gui now runs with `KeepNew: 10 * time.Second`: `trim` drops no finished job started less than 10 s ago.

That exemption has no bound. In 10 s, poon-gui now keeps every finished job, whatever `MaxJobs` (64) and `MaxTotalOutput` (64 MiB) say. A burst of quick jobs that print a lot holds all of their output:

- With 4 MiB jobs started one after another for 3 s, the runner kept 188 jobs and a 756 MB heap.
- Before the fix, the same burst kept 16 jobs and a 68 MB heap.

The prototype keeps the exemption but bounds it. A new job stays only while the jobs kept are within twice either limit. Past that, the oldest finished jobs go, whatever their age. The same burst then keeps 32 jobs and a 132 MB heap. The race fix still holds: the newest job is never the one dropped.

## Problem

```go
if j := r.jobs[id]; j.Done() && (j.Refresh || !refreshOnly) && now.Sub(j.Started) >= r.keepNew {
```

`trim` runs as each job starts. Every job started in the last 10 s fails the age test, so neither loop drops it. The limits it was meant to keep are then:

- `MaxTotalOutput`: 64 MiB of finished output.
- `MaxJobs`: 64 jobs kept.

These hold only for jobs older than 10 s. `MaxRunning` (16) still bounds the jobs running at once. But a quick job finishes in milliseconds, so it does not bound how many finished jobs pile up.

Who can start jobs that fast: anything that holds the page's token, such as the page itself, a script, or a page reload that refreshes every panel. A person clicking does not get near this. The limits exist so that no client, including a buggy one, can make poon-gui hold unbounded output.

## Measurements

These come from a throwaway Go test, not committed. It runs the real `Runner` with a `Main` that writes 4 MiB and returns. It starts a job, waits for it, and repeats for 3 s. The heap is `runtime.MemStats.HeapAlloc` after a GC.

| Runner | Jobs started in 3 s | Jobs kept | Heap |
| --- | --- | --- | --- |
| `KeepNew: 0` (before Round 37) | 3,540 to 3,656 | 16 | 68 MB |
| `KeepNew: 10s` (main @ 7eb16ab) | 188 | 188 | 756 MB |
| `KeepNew: 10s`, this prototype | 3,260 | 32 | 132 MB |

On main, the runner also started 19 times fewer jobs in the 3 s, because the Go heap kept growing. Over the full 10 s window the heap would have kept growing at the same rate.

The page's own use stays well inside the bound: the measuring kit's `jobs` scenario keeps 17 jobs, and the 5-minute soak keeps 64.

## Idea

`trim` makes a third pass. The first two passes are unchanged: finished refreshes, then other finished jobs, each skipping jobs newer than `KeepNew`, while over the limits. The third pass ignores age, and runs only while the runner is over twice either limit. Under normal use only the first two passes do anything. The race fix is kept, because a burst would have to fill twice the limits before a new job could go, and then the oldest go first.

`TestTrimBoundsNewJobs` starts 20 quick jobs with `MaxJobs: 2` and `KeepNew: time.Hour`. It checks that at most 4 are kept, and that the newest is still there. It fails on main (20 kept) and passes with the prototype. `TestTrimKeepsNewJobs` (Round 37's test) still passes. All gui/jobs tests pass with `-race`.

## Cost and risk

- About 10 lines in `trim`, and one test.
- **Risk:** if a burst passes twice the limits, a just-started quick job could go before its page follows it. The page then shows what it does for any dropped job. That is the race Round 37 fixed, but now only past twice the limits.
- **Compatibility (White):** no API path, flag or stored key changes.

## Prototype

On this branch: gui/jobs/jobs.go and gui/jobs/jobs_test.go.

## Modularity

Nothing to challenge: the bound lives in `trim`, beside the limits it keeps.

## Consensus

None.
