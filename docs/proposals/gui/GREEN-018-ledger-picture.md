# GREEN-018: A picture of the selected ledger in Explore

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static/watch.js (ledgerPicture, the ledger form's onResult), style.css |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 4 Part A) |

## Summary

The second half of GREEN-006. When Explore shows a page of
`poon ledger`, a small picture appears above the table. Each entry is a
bar, oldest on the left, linked to its parent, with its exact own weight
above it, its height and transaction count below, and the rest of its
fields in a tooltip. When the page starts at the tip, the newest bar is
outlined as the tip.

## Problem

The ledger's central idea, entries building on their parents and weight
deciding which ledger is selected, was only visible as a table of hashes
and numbers.

## Idea

- `ledgerPicture(page)` draws inline SVG from the JSON that
  `poon ledger` printed, using the `onResult` hook Grey added for paging.
  It does not re-render or replace the table.
- Every number shown is a printed field. The only arithmetic is scaling
  bar heights to the largest own weight on the page, and the caption says
  so.
- A link is drawn only when an entry's `parent` is the bar to its left.
  With a filter set, a page can skip entries, and the picture then shows
  the gap instead of inventing a chain.
- It uses SVG attributes and CSS classes, with no inline styles, so the
  CSP is unchanged. Colours come from the theme tokens, so light, dark
  and high contrast all work. The SVG has an `aria-label` that sums it up.

Later: a log scale switch, because one heavy entry flattens the others,
and the same picture for the Watch view's selected events as they
arrive.

## Cost and risk

About 50 lines. The risk is the picture implying more than poon printed.
Labelling exact values and showing only printed links keeps it honest.

## Prototype

On the branch. `gui/e2e/watch.mjs` loads pool 1's ledger after a claim
and build, and checks for one bar marked as the tip and the SVG's label.
It was also checked by eye with four entries, in light and dark.
