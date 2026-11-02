# GREEN-030: Long views list their panels at the top

| Field | Value |
| --- | --- |
| Status | adopted (Round 8) |
| Based on | claude/gui-4-watch @ b20bf8f |
| Area | gui/web/static/views.js (mountAll), style.css |
| Effort | small |
| Decided by | (Grey) |

## Summary

Work now has nine panels, and Wallet, Node, Pools and payments, and
Explore are nearly as long. To reach Schedule you scroll past six panels.
With this change, any view with five or more titled panels shows a
"Jump to:" row of its panel titles under the intro. Each title scrolls to
its panel and moves focus to the panel's heading.

## Problem

- Panels are stacked in one column. The panel you want is often several
  screens down, and nothing at the top says what is below.
- The Schedule panel that Round 7 added sits at the bottom of Work, below
  the forms. A new user may never find it.

## Idea

- `mountAll` collects each part's `h2`. If there are five or more, it
  adds a `nav.jump` of buttons, one per title, after the intro and the
  node banner.
- A button calls `scrollIntoView` on the panel and focuses its heading,
  so keyboard and screen-reader users land in the right place too.
- It leaves the hash alone, because the hash is the router: a `#panel`
  link would change views.
- It is generic: every view gets it without its own code. Short views
  (Peers, Watch) show nothing new.

## Cost and risk

- About 15 lines of JS and 2 lines of CSS. No poo command, and no state.
- Tests that pick a button by text could now match a jump button with the
  same title. All eight e2e scripts still pass.

## Prototype

On `claude/gui-green`. `gui/e2e/work.mjs` clicks "Schedule" in the jump
list. It checks that the Schedule panel is at the top of the window,
that its heading has focus, and that the hash did not change.
