# ORANGE-005: A measuring kit for the GUI, and what was found to be fine

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/perf (new), gui/jobs/bench_test.go, gui/server/bench_test.go |
| Effort | small |
| Decided by | |

## Summary

This proposal adds a repeatable way to measure the GUI, which every
other ORANGE proposal uses. `gui/perf/perfgui` serves the real page and
API, but its job runner simulates `poo watch` (a set rate of events in
poo watch's shape), `poo node` (a set rate of log lines) and `poo big`
(a large result). Every other command runs through the real `cli.Main`.
`gui/perf/measure.mjs` drives it in headless Chromium and prints one JSON
object of numbers. There are also Go benchmarks for the job runner and
the server. Nothing in `gui/perf` is released or used by `poo-gui`, and
it imports only `gui/...` and `pkg/cli`, so `TestImportsOnlyCLI` passes.

## How to run

```sh
go -C gui build -o /tmp/perfgui ./perf/perfgui
NODE_PATH=$(npm root -g) node gui/perf/measure.mjs /tmp/perfgui            # everything, about 6 minutes
NODE_PATH=$(npm root -g) node gui/perf/measure.mjs /tmp/perfgui watch      # or: load watch terminal node refresh jobs command
RATES='[[200,4000]]' CSS='…' node gui/perf/measure.mjs /tmp/perfgui watch  # one rate; try a style without a rebuild
LOG_RATE=200 node gui/perf/measure.mjs /tmp/perfgui node
go -C gui test -run XXX -bench . ./jobs ./server
```

The script measures frames per second, the longest gap between frames,
long tasks, and Chromium's own counters (layouts, style recalculations,
script and task time, JS heap, DOM nodes).

## Fine as it is (measured, no change proposed)

| What | Number |
| --- | --- |
| Page load (13 requests, 88 KB, no build step) | DOMContentLoaded 38 ms, first view drawn 95 ms (median of 5) |
| A short command from the page (`/api/run`, `poo crypto`) | 2.0 ms median, 2.9 ms p90 |
| Start a job and stream it to exit | 3.4 ms median, 4.4 ms p90 |
| `/api/run` in Go around `cli.Main` | 0.9–1.1 ms |
| Job output write | 370–655 ns (0–4 followers) |
| Start a job with 64 kept | 11–13 µs |
| Jobs list (`/api/jobs`, 15–64 jobs) | about 2 KB, draw 0.5 ms |
| JS heap over every scenario | under 7 MB; no growth over a 60 s node log |

`Cache-Control: no-store` on the static files means each load fetches
88 KB again. Over loopback that is a few milliseconds, so it is not worth
weakening.

## For Source (through the coordinator)

No core performance issue showed up from the GUI's side. `poo watch`
writes one `json.Encoder` line per event, which is right for a shell.
ORANGE-002 joins those lines in the GUI's stream, so nothing is needed
in `pkg/cli`. The known shutdown-timeout exit (GREEN-015 item 3) is
already with Source/Grey.

## Prototype

Branch `claude/project-thread-f0df7u`: `gui/perf/perfgui/main.go`,
`gui/perf/measure.mjs`, `gui/jobs/bench_test.go` and
`gui/server/bench_test.go`.
