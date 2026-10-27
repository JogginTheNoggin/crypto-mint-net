# GREEN-009: A view menu for narrow windows

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-5-release @ 7209952 |
| Area | gui/web/static (app.js, style.css) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

Below 640 pixels wide, the row of view tabs is replaced by a single
drop-down menu of the same views. The terminal's long help line is hidden
there to give the output room, and pane padding shrinks.

## Problem

In a narrow window, such as a GUI snapped to half of a laptop screen,
the ten views became a horizontal tab strip. Most tabs were off screen
with nothing to show they exist. The terminal's help line wrapped onto
three lines above the output.

## Idea

- `app.js` builds a `<select>` from the same list as the nav, and keeps it
  in step with the current view. Choosing an entry sets the hash, exactly
  as a tab click does. CSS shows the menu or the tabs, never both.
- A native select is keyboard and screen-reader friendly, and needs no
  script for opening or closing.

Not done, and worth a look by Grey: some field placeholders are cut off
at every width (for example "default: wallet.json in the roo"). Moving
the default into the field's help text would fix that.

## Cost and risk

About 15 lines. Wide windows are unchanged.

## Prototype

On the branch, with checks in `gui/e2e/qol.mjs` (the menu shows and the
tabs hide when narrow, choosing Wallet opens it, and the menu hides when
wide again).

## Decision

Adopted. Cut-off placeholders are noted for a later pass.
