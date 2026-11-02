# GREEN-013: "Look up" links from ids to Explore

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 93ed36a |
| Area | gui/web/static (render.js, app.js router, watch.js Explore, views.js form) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 3 Part A) |

## Summary

Wherever a view shows a transaction id (`tid`) or an address
(`address`, `to`), a small "look up" link appears beside it. The link
opens Explore with that value filled into "A transaction" or "An
address", and runs it. This is the first half of GREEN-006, which was
deferred until G4 existed.

## Problem

Following a payment meant copying a hash from Watch or from a result and
pasting it into another form. Watch showed tids in its own short form,
which could not be copied, so the user had to open the event detail
first.

## Idea

- `render.js` knows which key it is rendering. For keys in a short
  `LOOKUPS` table (`tid` → transaction, `address` and `to` → address),
  a hex value gets a link to `#/explore?tid=…` or `#/explore?address=…`.
- The router passes what follows `?` to the view as `ctx.params`. Explore
  reads `tid` or `address` and calls the form's new `run(values)`, which
  fills the fields and submits exactly as a user would. The command
  preview shows what ran, and Jobs lists it.
- Watch renders its tid cell through `render`, so it gets the same
  shortening, click-to-copy and link as every other view.
- Clicking the hex copies it, as before, and copying is now allowed to
  fail. Before, a refused clipboard write was an uncaught promise
  rejection that showed as a page error.

## Cost and risk

About 50 lines. The links are plain anchors, so Back returns to the
previous view. A link that points at a value Explore can't find just
shows `poo tx`'s own error.

## Prototype

On the branch. `gui/e2e/watch.mjs` clicks a tid link in Watch and checks
that Explore ran `poo tx TID`, and clicks an address link in an unspent
result and checks that the address lookup ran.
