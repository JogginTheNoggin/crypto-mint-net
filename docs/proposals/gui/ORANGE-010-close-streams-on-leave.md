# ORANGE-010: Close a view's event streams when it is left, so the GUI cannot stall

| Field | Value |
| --- | --- |
| Status | adopted (Round 7) |
| Based on | claude/gui-4-watch @ e7df74b |
| Area | gui/web/static/views.js (runJob, panel, form, Node view), work.js |
| Effort | small |
| Decided by | |

## Summary

This is a long-session bug, not a slowdown. While a node runs, every
visit to the Node view opened a new event stream for the node's log and
never closed it. A browser allows six connections to one host, and each
open stream holds one. After about two minutes of moving between views,
every connection was taken. From then on, every request the page made
waited forever: commands, the jobs list, refreshes, Start and Stop. The
GUI looked frozen until the tab was reloaded.

The fix closes a view's streams when the view is left. That covers the
Node view's log, a panel's or form's command still running, and the
Work view's take-over stream when the view is not showing. The jobs
keep running on the server, as a detached command does, and are listed
in Jobs. With the fix, a 5-minute session of the same kind stays
responsive throughout, and its DOM and listener counts are flat.

## Problem

- `nodeView.showRunning` calls `api.follow(j.id, …)` and drops the
  returned close function. The view's `stop` clears only its interval.
  Each later visit sees `following === null` and opens another stream.
- `runJob` dropped its close function too. A panel or form left while
  its command still ran (a long command, or one waiting for input) kept
  its stream until the command ended.
- On a page reload, Work's take-over of a running `poon grind run` (from
  Round 6 Part A) followed the job even when the Work view was not
  showing.
- `api.follow` uses `EventSource`. Over plain HTTP/1.1 to loopback (no
  h2c in browsers) the limit of six connections per host is fixed, so
  the only real fix is to keep few streams open.

## Idea

- `runJob` keeps its stream's close function on the area
  (`area.unfollow`). A new `leave(area)` marks the area gone and closes
  the stream. `panel()` and `form()` call `leave(area)` from their
  `stop`. A job that starts after its view is gone is not followed.
- The Node view keeps `unfollowLog`, closes it in its `stop`, and
  ignores a `showRunning` that returns after the view is gone.
- Work follows a taken-over job only if the view is showing. Otherwise
  `attach` follows it when the view opens.
- The new e2e check `gui/e2e/streams.mjs` starts a node, visits the Node
  view ten times, and asserts that at most 3 streams stay open and that
  `/api/jobs` answers in under 2 s. Add it to the CI e2e list.

## Measurements

`gui/perf/measure.mjs soak` (new): `perfgui` with a node logging 20 lines
a second, a watch at 50 events a second and a grind run at 50 lines a
second. The page moves to the next view every 2 s, through all of them
in turn. Every 30 s it takes a sample after a forced garbage collection:
open event streams, `/api/jobs` with a 5 s limit, DOM nodes, JS event
listeners, and the server's RSS.

| at | open streams, before → after | `/api/jobs`, before → after | DOM nodes, before → after | listeners, before → after |
| --- | --- | --- | --- | --- |
| 30 s | 3 → 3 | answers → answers | 9,344 → 9,156 | 370 → 354 |
| 60 s | 4 | answers → answers | 27,441 → 27,001 | 1,047 → 1,015 |
| 90 s | 5 | answers → answers | 27,996 → 27,248 | 1,090 → 1,050 |
| 120 s | **7** (6 open, 1 waiting) | **no answer in 5 s** → answers | 28,628 → 27,408 | 1,134 → 1,069 |
| 180 s | 7 | **no answer** → answers | 29,939 → 27,768 | 1,191 → 1,170 |
| 240 s | 7 | **no answer** → answers | 31,504 → 27,140 | 1,264 → 1,046 |
| 300 s | (not run) | answers | 27,395 | 1,059 |

Before the fix, DOM nodes and listeners grew by about 700 and 30 every
30 s, because each leaked log kept writing into a detached `<pre>`.
After the fix both are flat once the Watch and Work tables have filled
(about 27,000 nodes, almost all of them Watch's 1000 kept rows).

The `streams.mjs` e2e check fails on the base ("visit 6: the node log did not show within 5 s; 9 event streams open") and passes with the fix (3 runs out of 3).

## Also measured (no change proposed)

- ORANGE-009 on the merged base: the Work view at 200 lines/s is 13%
  busy, 2000/s is 23% busy, and 20,000 lines at full rate take 0.45 s
  at 36 fps. Watch at 200/s is 24% busy and takes 20,000 events in
  0.51 s. `poon watch` in the terminal takes 0.9 s.
- Reloading the page during a long `poon grind run` (40,000 lines, of
  which the server keeps the last 4 MiB): the Work view takes it up and
  catches up in 0.25 s at 48 fps.
- Server RSS in the soak rose from 14 MB to 41 MB over 5 minutes as
  the running watch, grind and node jobs filled their 4 MiB output
  limits and finished refresh jobs filled the 64-job list. Both are
  bounded by the runner's limits (16 running × 4 MiB plus 64 MiB
  finished). A longer soak could confirm where it levels off.

## Cost and risk

A command started from a form and then left behind no longer shows its
result when the view is opened again. The view is rebuilt on every
visit anyway, so that was already true of the result area. The job is
in Jobs with its output and its Stop. Nothing is cancelled that was not
cancelled before.

## Prototype

Branch `claude/project-thread-f0df7u`: `gui/web/static/views.js` and `work.js`, the new
`gui/e2e/streams.mjs`, and the `soak` and `reload` scenarios in
`gui/perf/measure.mjs`.
