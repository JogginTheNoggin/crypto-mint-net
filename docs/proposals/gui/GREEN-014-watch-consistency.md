# GREEN-014: Watch matches the rest of the GUI

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 93ed36a |
| Area | gui/web/static/watch.js, style.css, gui/e2e/watch.mjs |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 3 Part A) |

## Summary

Three small mismatches in the Watch view, fixed:
1. Event kinds are tick boxes instead of a free-text field you had to
   type correctly.
2. Times show as local `HH:MM:SS.mmm`, like the Jobs view, instead of raw
   UTC with nanoseconds. The exact time is in the tooltip.
3. Hashes use the same shortening as every other view (10…6 instead of
   8…4).

## Problem

- The Kinds field expected exact words (`tx,entry,layer,selected,…`), and
  a typo was only caught by `poo watch` after pressing Start. All the
  other filters are open-ended values, but kinds are a fixed set.
- Watch printed `2026-10-27 22:34:01.706323438` (UTC, no zone shown),
  while Jobs printed `9:32:06 PM` (local). Side by side they disagreed by
  the time-zone offset, with nothing saying so.
- Two shortening styles meant the same hash looked different in Watch and
  in Explore.

## Idea

- The boxes come from the `KINDS` list watch.js already has. Ticked
  boxes become one `-kind a,b` flag, and none ticked sends no flag
  (every kind), as before. The preview updates as boxes change.
- The local time is computed from the event's own `time` field. Nothing
  is reordered or dropped.

`KINDS` is still a list kept in the GUI. GREEN-004 (`poo help -json`)
would let it come from `poo` instead.

## Cost and risk

About 30 lines. `gui/e2e/watch.mjs` changed from typing "tx, selected" to
ticking two boxes; it checks the same `poo watch -kind tx,selected`
preview.

## Prototype

On the branch, checked by `gui/e2e/watch.mjs`.
