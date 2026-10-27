# GREEN-008: Jobs view filters, times and grouped repeats

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-5-release @ 7209952 |
| Area | gui/web/static/views.js (jobsView), style.css |
| Effort | small |
| Decided by | (Grey) |

## Summary

The Jobs view gains a text filter, a "Failed only" switch, the time each
job started, and "Group repeats" (on by default). Grouping shows a run of
the same finished command with the same exit code once, as its newest
job, with a count such as `×12`.

## Problem

Panels refresh on a timer, so after a minute on the Node view the Jobs
list is mostly `poo status`. The command the user actually ran from a
form or the terminal is hard to find, and there is no way to see only
what failed or when something ran.

## Idea

- All of it is display only: the list still comes from `/api/jobs`, every
  job is kept, and "Output", "Stop" and "Run again in terminal" work as
  before on the row shown.
- The time comes from the `started` field the server already returns.
- Running jobs are never grouped, so a running `poo node` always has its
  own row and Stop button.

## Cost and risk

About 40 lines. The list is redrawn on each 3-second refresh, as before.
With grouping on, older runs of a repeated command are reachable only by
switching grouping off.

## Prototype

`jobsView` in `views.js`, the `.job-tools` CSS, and checks in
`gui/e2e/qol.mjs` (two `poo status` runs show as one row, the text filter,
and Failed only).
