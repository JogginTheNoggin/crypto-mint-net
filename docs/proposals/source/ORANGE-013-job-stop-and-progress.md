# ORANGE-013: Round 6 jobs measured; stop without polling, progress each second

| Field | Value |
| --- | --- |
| Status | adopted (Source Round 7, D119) |
| Based on | `claude/m10-watch` @ `9972137` |
| Area | `pkg/grind/jobs.go` |
| Effort | small |
| Decided by | Grey |

## Summary

I measured Round 6's job changes: the progress cap, leases, the live
schedule, and the grinder rates again.

| Change | Finding |
| --- | --- |
| Leases | Cost nothing |
| Grinder rates | Unchanged |
| Job overhead | Still within noise |
| Stop | 10.3 ms, almost all of it a 10 ms polling sleep |
| Progress | Once per 2 s slice, not once a second as D112 says |

The two fixes are on the branch:
- **Stop without polling.** A job closes a channel when it ends, and
  `Stop`, `StopAll` and `Schedule` wait on it. A stop goes from 10.3 ms
  to 0.44 ms, and a live schedule change with 3 policy jobs from 10.7 ms
  to 0.73 ms.
- **Progress each second.** While a search runs, a ticker reports
  progress from the search's try counter. A 12-second job sends 11 or 12
  progress events, not 6. The rate is measured over the last second, as
  D112 promises. A GUI meter now moves each second.

No consensus change.

## Measurements

The machine:
- 4 vCPU Xeon at 2.10 GHz, with SHA-NI and AVX-512F.
- Go 1.27.3.

How they were taken:
- From test-only harnesses on a real node (the `newFixture` setup).
- With the default build, unless noted.

### Grinder rates against Round 5

The rates are unchanged, within the VM's noise.

| Hash | R5, 1 / 3 / 4 threads | R6, 1 / 3 / 4 threads |
| --- | --- | --- |
| SHA-256 | 110 / 36.5 / 28.0 ns | 114 / 34.8 / 26.6 ns |
| SHA-512/256 | 309 / 106 / 80.2 ns | 325 / 103 / 93.8 ns |
| SHA3-256 | 453 / 157 / 117 ns | 426 / 151 / 121 ns |
| BLAKE2b-256 | 213 / 73.1 / 58.3 ns | 200 / 68.8 / 51.2 ns |
| BLAKE3 | 125 / 42.3 / 38.4 ns (generic midstate) | 127 / 40.2 / 31.9 ns (`cpu-blake3-cv`) |

### Job overhead

A 12-second tip job on 2 threads ran at 52.3 ns per try, against a bench
rate of 51.0 ns: 2.5%, as in R5.

With this change it ran at 55.5 ns against 53.6 ns, which is within the
±4% run-to-run noise. The progress ticker costs nothing measurable.

### Leases

| Operation | Cost |
| --- | --- |
| `Manager.Renew`, in process | 107 ns, no allocations |
| `Manager.Renew`, two goroutines at once | 259 ns (the manager's mutex) |
| `POST /v1/grind/{id}/renew`, round trip through `rpc.Client` | 105 µs, 10.5 KB and 105 allocations |

`poo grind run` renews every 5 s, so a followed job spends 0.002% of a
core on its lease. No change is needed.

### Stop and live schedule changes

These were measured over 15 stops of a running tip job, and 8 schedule
changes per row.

| Operation | Before | After |
| --- | --- | --- |
| `Stop` of a running job, p50 | 10.28 ms | 0.44 ms |
| `Stop`, min to max | 10.21–10.69 ms | 0.21–0.69 ms |
| `Schedule` with 1 policy job, p50 | 10.35 ms | 0.50 ms |
| `Schedule` with 3 policy jobs, p50 | 10.68 ms | 0.73 ms |
| `Schedule` with 8 policy jobs, p50 | 10.58 ms | 1.57 ms |

The search stops quickly: it checks its context every 4,096 tries, which
is about 0.5 ms for SHA-256. The cost was `wait`, which polled the job's
state with 10 ms sleeps, so every stop paid at least one sleep.

A restarted policy job loses no work. A search ended by its context
returns its best layer so far (`Engine.GrindLayerWith`), which is submitted.

### Progress cadence

`count` runs only when a search ends, once per 2 s `Slice`. So
`ProgressEvery = 1s` never had the chance to fire more often.

| Test | Before | After |
| --- | --- | --- |
| Progress events in a 12-second tip job | 6 | 11–12 |
| Progress events in 4.5 s (`TestProgressEachSecond`) | 2 | 4 |

## Idea

**Stop without polling.**
- `job.ended` is closed in `end`, guarded against a second close.
- `wait(id)` waits on it, for 2 s at most as before, instead of 200 loops
  of `Get` and a 10 ms sleep.

**Progress each second.**
- `progress(j, k, t0, &tried)` runs beside each `build` and `work`
  search.
- Each `ProgressEvery`, under the manager's lock, it takes
  `j.Tries + tried.Load()`. If a second has passed since the last report,
  it sets the rate and emits a `progress` event.
- `count` at the end of the search still adds the tries and may report.
  Both share `progAt`, so reports stay at least a second apart.
  `TestJobEvents`, which checks at most one per second and 900 ms apart,
  still passes.

## Cost and risk

- About 50 lines.
- One goroutine and one ticker per 2 s slice, stopped when the search
  ends.
- `TestProgressEachSecond` checks that a stop takes under 200 ms and that
  progress arrives each second.
- The full suite passes with `-race`.

## Prototype

On `claude/source-orange-r6`, commit `1b0aa14`: `pkg/grind/jobs.go` and
`TestProgressEachSecond` in `pkg/grind/grind_test.go`.
