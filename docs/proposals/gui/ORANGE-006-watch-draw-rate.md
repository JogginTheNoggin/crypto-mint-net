# ORANGE-006: Draw a busy Watch table at most ten times a second

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 50a0edd |
| Area | gui/web/static/watch.js (Watch view) |
| Effort | small |
| Decided by | |

## Summary

After ORANGE-001 the Watch view runs at 60 frames a second, but at 200
events a second the main thread was still busy 61% of the time. The view
redrew the table on every frame that had a new event, which meant 60
table layouts a second. This change draws at most every 100 ms. The
first event after a quiet spell still shows on the next frame. At 200
events a second the main thread is now busy 18% of the time, at 1000 a
second 36% instead of 76%, and at 5000 a second the frame rate goes from
34 to 47. The counts, rows, order and bounds are the same. They just
change ten times a second instead of sixty.

## Problem

Each draw prepends rows to a 1000-row table, and the browser lays the
table out again. Script time was small (0.4 s in 20 s) and almost all
the cost was layout and style: 1203 layouts in 20 s at 200 events a
second.

## Idea

`schedule()` replaces the bare `requestAnimationFrame`. If the last draw
was at least `DRAW_EVERY` (100 ms) ago, it draws on the next frame as
before. Otherwise it sets a timer for the rest of the 100 ms and then
draws on a frame. Resume still draws what was held at once, and cancels
a waiting timer. Nothing else changes.

## Measurements

`gui/perf/measure.mjs watch` against `perfgui`, headless Chromium, on a
4-core container. Before is base 50a0edd (ORANGE-001 to 005 merged).

| Events/s (total) | fps | Main thread busy | Layouts | Longest frame gap |
| --- | --- | --- | --- | --- |
| 200 (4000) | 60 → 60 | 12.2 s → **3.7 s** of 20 s | 1203 → **194** | 33 → 67 ms |
| 1000 (10000) | 58 → 58 | 7.7 s → **3.65 s** of 10 s | 574 → **100** | 50 → 50 ms |
| 5000 (20000) | 34 → **47** | 4.0 s → **2.5 s** | 101 → **43** | 117 → **67** ms |
| as fast as possible (20000) | 22 → **29** | 0.71 s → **0.50 s** | 14 → 8 | 83 → 100 ms; done in 855 → **689 ms** |

The re-measure of ORANGE-001 on the merged base held up: 60 fps at 200
events a second, and 20,000 events in 0.86 s at full rate.

## Cost and risk

A row can appear up to 100 ms after it arrived. That is not visible in a
table that is scrolling past. `DRAW_EVERY` is one constant if Grey
prefers 50 ms. The e2e checks wait for rows and pass unchanged.

## Prototype

Branch `claude/project-thread-f0df7u`: `schedule()` in `gui/web/static/watch.js`.
