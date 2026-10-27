# GREEN-015: Loose ends found in Watch, Explore and Source

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-4-watch @ 93ed36a |
| Area | gui/web/static (watch.js), gui/e2e, pkg/cli flags (Source) |
| Effort | small |
| Decided by | (Grey) |

## Summary

Observations from a sweep of the G4 views against a live node, as ideas
for Grey, not prototyped:

1. Leaving the Watch view stops the watch, so checking a transaction in
   Explore loses the live stream.
2. Explore's "Older" paging finds `next` by scanning the rendered page
   for a `dt` element whose text is `next`.
3. `gui/e2e/watch.mjs` failed once in 8 runs on 93ed36a. It timed out
   waiting for "node stopped (exit 0)" after Stop node.
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
3. **Flaky stop.** It may be the 20-second wait racing a slow shutdown
   on a loaded runner, or the Node view's `showRunning` following a job
   that ended between polls. Worth a look before CI meets it.
4. **`-kind` on non-stream commands.** The shared filter flag set gives
   every view command `-kind`. Either ignore it with a warning, refuse
   it, or document what it does for `tx` and `unspent`. That is for
   Source/Grey; mentioned here because the GUI must not offer it there,
   and does not.

## Cost and risk

Item 1 is medium, about 60 lines, because the table and counters move
out of `mount`. Items 2 and 3 are small. Item 4 is a Source decision.

## Prototype

None.
