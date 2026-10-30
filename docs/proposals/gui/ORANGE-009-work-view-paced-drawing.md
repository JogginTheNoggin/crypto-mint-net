# ORANGE-009: The Work view draws at most ten times a second and reuses its chart

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-4-watch @ 8b044ac |
| Area | gui/web/static/work.js (Work view, live run), scroll.js (`paced`) |
| Effort | small |
| Decided by | |

## Summary

The live run in the Work view did all of its drawing for every line
`poo grind run` printed. It built a new SVG chart for every progress
line, rewrote the counts, and prepended a table row for every found
line. When a grind finds layers quickly (an easy target, or several
pools), it prints hundreds to thousands of lines a second. At 200 lines
a second the main thread was busy 46% of the time. At full rate it ran
at 9 frames a second, with a 267 ms freeze, and took 1.35 s to catch up
on 20,000 lines.

The view now reads every line as it arrives but draws at most every
100 ms, with the same pacing as Watch (ORANGE-006). Each draw shows the
newest state and plan, moves one kept chart line, adds the new found
rows in one fragment, and writes the counts once. At 200 lines a second
the main thread is busy 12% of the time. At full rate the view runs at
46 frames a second and catches up in 0.39 s. A slow, long run (two
lines a second for a minute) was already fine and is unchanged.

The prototype also fixes a missing bound. Stdout lines that are not JSON
were appended to the error box with no limit (`errOut.textContent +=`).
Stderr already kept its last 64 KiB. Both now go through one `errAppend`
that keeps the last 64 KiB. This belongs with GUI-007 (bounds), so it has
been passed to GUI/Red as a note.

## Idea

- `scroll.js` gains `paced(draw, every = 100)`. It returns
  `{ schedule, now }`: `schedule` draws on a frame at most once every
  `every` ms (the first call after a quiet spell draws on the next
  frame), and `now` draws at once. Watch's own `schedule()` from
  ORANGE-006 could move to it too.
- `add(ev)` only records what arrived: the newest state text and plan,
  the rate (still the last 120 points), and the found events (at most
  1000 waiting, so a hidden tab stays bounded). Then it schedules a draw.
- `rateChart()` builds the SVG and polyline once. `set(points)` updates
  the `points` and `aria-label` attributes, and finds the highest point
  with a loop instead of `Math.max(...points)`.
- The exit handler draws at once before it writes "(exit N)", so the
  last line is never left waiting.

## Measurements

`gui/perf/measure.mjs work` against `perfgui`, whose `grind run` prints
a start line with a plan, then progress lines at a set rate with a found
line before every 4th (the shape of `pkg/grind.Event`). Headless
Chromium on a 4-core container. Before is base 8b044ac.

| Lines/s (total) | fps | Main thread busy | Layouts | Longest frame gap | Caught up in |
| --- | --- | --- | --- | --- | --- |
| 2 (120, a 60 s run) | 60 → 60 | 1.45 s → 1.18 s of 60 s | 133 → 135 | 33 → 17 ms | (paced by the run) |
| 200 (4000) | 60 → 60 | 9.26 s → **2.31 s** of 20 s | 1202 → **199** | 17 → 17 ms | 20.1 s (the run) |
| 2000 (20000) | 60 → 60 | 7.03 s → **1.99 s** of 10 s | 601 → **101** | 33 → **17** ms | 10.1 s (the run) |
| as fast as possible (20000) | 9 → **46** | 1.24 s → **0.22 s** | 8 → 5 | 267 → **67** ms | 1351 → **392 ms** |

Script time at 2000 lines a second fell from 1.66 s to 0.31 s. That is
mostly the SVG no longer being rebuilt for every progress line.

Re-check of the earlier gains on 8b044ac (unchanged by this proposal):
Watch runs at 60 fps at 200 events/s with the main thread 21% busy, and
takes 20,000 events in 0.56 s at full rate. `poo watch` in the terminal
finishes in 0.57 s. The node log at 50 lines/s keeps the main thread
0.41–0.52 s busy in each 10 s. A 3.9 MB text result shows in 0.21 s.

## For Source (through the coordinator)

`pkg/grind`'s `slice` ends a slice on each find and then emits a found
line and a progress line. With an easy target, finds come thousands of
times a second, so does `progress`, and each `rate` is measured over a
slice of a few milliseconds, which makes it noisy. Emitting `progress`
at most about once a second per job (found lines unchanged) would make
the stream and the rate steadier for every consumer, `poo grind run` in
a shell included. The GUI copes either way after this change.

## Cost and risk

The counts, chart and found rows change up to 100 ms after a line
arrives. What is shown is otherwise the same. The e2e check `work.mjs`
waits for the chart and counts and passes.

## Prototype

Branch `claude/project-thread-f0df7u`: `gui/web/static/work.js`, `paced` in
`gui/web/static/scroll.js`, the `grind run` simulation in
`gui/perf/perfgui`, and the `work` scenario in `gui/perf/measure.mjs`.
