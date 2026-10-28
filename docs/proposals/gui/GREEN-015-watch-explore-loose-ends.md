# GREEN-015: Loose ends found in Watch, Explore and Source

| Field | Value |
| --- | --- |
| Status | adopted: 1 and 2; 3 and 4 with Source/Grey |
| Based on | claude/gui-4-watch @ 93ed36a |
| Area | gui/web/static (watch.js), gui/e2e, pkg/cli flags (Source) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 3 Part A) |

## Summary

Observations from a sweep of the G4 views against a live node, as ideas
for Grey, not prototyped:

1. Leaving the Watch view stops the watch, so checking a transaction in
   Explore loses the live stream.
2. Explore's "Older" paging finds `next` by scanning the rendered page
   for a `dt` element whose text is `next`.
3. `gui/e2e/watch.mjs` fails about 1 run in 6, on 93ed36a and on this
   branch alike. The cause is in the headless node, not the GUI: after
   Stop node, `poo node` prints "node stopped" and then
   `poo: context deadline exceeded`, and exits 1 instead of 0.
4. A Source loose end: `poo tx` and `poo unspent` accept `-kind`, which
   only means something for `poo watch`.

## Problem and idea

1. **Watch in the background.** The terminal keeps a job running across
   views, but Watch cancels its job on unmount. Keep the watch job and
   its table at module level, as the terminal does with its pane, and
   show a "watching" dot on the Watch nav entry while it runs. Stop
   stays explicit. Now that look-up links (GREEN-013) jump from Watch to
   Explore, this matters more.
2. **Paging without scraping.** `showResult` could return the parsed
   JSON, or `runJob` could resolve with it, so the ledger form reads
   `next` from the data instead of from the rendered page. That is a
   small change to `runJob`'s callers.
3. **Flaky stop (a Source issue).** `pkg/rpc/server.go` gives
   `srv.Shutdown` 5 seconds. When a request is still in a handler after
   that, `Shutdown` returns `context.DeadlineExceeded`, and `poo node`
   reports it as a failure even though the node itself stopped cleanly.
   The run that failed had `poo status` refreshes from the Node view in
   flight. Options for Source/Grey: treat a shutdown timeout as a
   warning, not an exit-1 error; or close the remaining connections
   (`srv.Close`) after the deadline, and exit 0. In the meantime,
   watch.mjs could accept "node stopped" with any exit code. Whether a
   client can hold shutdown open on purpose is a question for Red.
4. **`-kind` on non-stream commands.** The shared filter flag set gives
   every view command `-kind`. Either ignore it with a warning, refuse
   it, or document what it does for `tx` and `unspent`. That is for
   Source/Grey; mentioned here because the GUI must not offer it there,
   and does not.

## Cost and risk

Item 1 is medium, about 60 lines, because the table and counters move
out of `mount`. Item 2 is small. Items 3 and 4 are Source decisions.

## Prototype

None.
