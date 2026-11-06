# ORANGE-001: Watch draws once per frame and lays out only the newest rows

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static/watch.js (Watch view), style.css |
| Effort | small |
| Decided by | |

## Summary

At 200 events a second the Watch view kept the browser's main thread
about 95% busy and dropped frames. At 5000 a second it ran at 5 to 6
frames a second with half-second freezes. Two changes fix most of this.
The view now draws arrived events once per animation frame, in one
fragment, with one click handler for the whole table. While live it lays
out only the newest 200 of the 1000 kept rows. The result is 60 frames a
second at 200 events a second, 58 at 1000, and almost four times the
throughput when events come as fast as possible. What poon watch prints,
the 1000-row bound (GUI-007) and the Pause and Clear behaviour are all
unchanged. Pause and Resume draw what was held at once.

## Problem

`add` built a row, attached two listeners to it, prepended it, trimmed
the table and rewrote the counts line, once for every event. Each prepend
changed a 1000-row auto-layout table, so the browser laid out the whole
table, and the page around it, about once a frame. Script time was small
(0.4 s in 20 s). Almost all the cost was style and layout.

## Idea

- Count every event as it arrives, but queue it and draw the queue in a
  `requestAnimationFrame`. A frame builds the rows of at most the newest
  1000 queued events in one `DocumentFragment` and prepends it once. If a
  hidden tab runs no frames, the queue keeps only the newest 1000.
- One `click` and one `keydown` listener on the `tbody` find the row's
  event through a `WeakMap`, so there are no longer two closures per row.
- CSS hides rows after the 200th while live
  (`table.events:not(.all) tbody tr:nth-child(n+201)`), and puts the
  table in a scroll box with `contain: content`. All 1000 rows are still
  in the table. A "Show all 1000 kept rows" box, or Pause, shows them.
  The e2e bounds check (1000 rows, newest first) passes unchanged.

## Measurements

Run with `gui/perf/measure.mjs watch` against `perfgui`, which streams
synthetic events in poon watch's shape through the real server and page
(ORANGE-005). Headless Chromium on a 4-core container. The Before figures
are base 9b4b19c. The After figures are this branch, which also includes
ORANGE-002's joined stream events.

| Events/s (total) | fps before → after | Longest frame gap (ms) | Long tasks (ms) | Main thread busy |
| --- | --- | --- | --- | --- |
| 200 (4000) | 37 → **60** | 100 → **17** | 831 → **0** | 19.4 s → 12.7 s of 20 s |
| 1000 (10000) | 30 → **58** | 100 → **50** | 435 → **0** | 9.9 s → 8.6 s of 10 s |
| 5000 (20000) | 6 → **25** | 450 → **83** | 3934 → **600** | 4.1 s → 3.9 s |
| as fast as possible (20000) | 5 → **17** | 483 → **100** | 3607 → **564** | took 3.8 s → **1.0 s** (5236 → 19627 events/s) |

Steps, on the 200 and 5000 events/s cases:

| Variant | fps at 200/s | fps at 5000/s |
| --- | --- | --- |
| base | 37 | 6 |
| draw once per frame, one handler | 39 | 6 |
| + scroll box with `contain: strict` | 41 | 10 |
| + `table-layout: fixed` | 44 | 11 |
| + lay out only the newest 200 rows | **60** | **25** |

Drawing once per frame on its own mostly helps when events come in
bursts, because it avoids up to 1000 layouts a frame. Most of the steady
gain comes from laying out fewer rows. `table-layout: fixed` added little
and needs fixed column widths, so it is left out.

## Cost and risk

The change is small and display-only. While live, the user scrolls 200
rows instead of 1000. The checkbox or Pause shows all 1000. Row order, the
detail pane and look-up links are unchanged.

## Prototype

Branch `claude/project-thread-f0df7u` (GUI/Orange): `gui/web/static/watch.js` and
`style.css`. The e2e checks, including `bounds.mjs`, pass.
