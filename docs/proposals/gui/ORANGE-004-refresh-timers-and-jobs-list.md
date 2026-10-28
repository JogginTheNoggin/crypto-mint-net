# ORANGE-004: No panel refreshes in a hidden tab; Jobs redraws only on change

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static/views.js (panel, Node view, Jobs view) |
| Effort | small |
| Decided by | |

## Summary

The refresh timers are cheap while you look at them. Left open, they
run forever. The Node view makes 48 requests a minute and starts 12
`poo status` jobs a minute, each with its own event stream. The Peers
view makes 20 requests a minute. They keep doing this in a background
tab, where nobody sees the result. With this change a panel skips its
timed refresh while `document.hidden` is true, and refreshes once as
soon as the tab is visible again. The Jobs view redraws its list only
when `/api/jobs` returned something different. The Node view's "is a
node running" poll pauses while hidden once it is already following
the node. Nothing changes while the tab is visible.

## Measurements

`gui/perf/measure.mjs refresh jobs`, each view left open for 30 s with
no node answering:

| View | requests a minute | jobs started a minute | timers behind them |
| --- | --- | --- | --- |
| Node | 48 | 14 | `poo status` every 5 s, `GET /api/jobs` every 3 s |
| Peers | 20 | 10 | `poo peers` every 10 s |
| Pools | 4 | 2 | none (its panels run once on opening) |
| Start | 0 | 0 | none |

The counts are over 30 s, doubled, so they include each view's first
runs on opening.

- Jobs list after 2 minutes: 15 jobs kept, all refreshes (GREEN-011
  drops those first), 1.97 KB of JSON, one draw takes 0.5 ms. The list
  is not a cost today. Skipping unchanged redraws mainly keeps a user's
  text selection and focus in the list from being reset every 3 s.
- Per command, the GUI's own overhead is small. `POST /api/run` for
  `poo crypto` takes 2.0 ms median (p90 2.9 ms) from the page.
  Starting a job and streaming it to exit takes 3.4 ms (p90 4.4 ms), and
  `GET /api/jobs` takes 2.0 ms. In Go, `/api/run` costs 0.9–1.1 ms
  around `cli.Main`. Nothing here is worth changing.
- Not measured: the saving in a hidden tab. Headless Chromium does not
  hide pages. Chrome throttles hidden timers to about once a minute only
  after 5 minutes, so until then a hidden Node view still runs its 48
  requests a minute. With this change it runs none.

## Cost and risk

- A tab coming back shows stale numbers for at most one round trip,
  then refreshes. A panel the user refreshes by hand always runs.
- The Watch stream is deliberately not paused, because it is a running
  command the user started, as in a shell.

## Prototype

Branch `claude/project-thread-f0df7u`: `panel()`, `nodeView.showRunning` and
`jobsView.refresh` in `gui/web/static/views.js`.
