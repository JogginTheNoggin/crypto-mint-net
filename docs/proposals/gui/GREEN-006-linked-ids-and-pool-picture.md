# GREEN-006: Linked ids and a live pool picture

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-5-release @ f93508e |
| Area | gui/web/static/render.js, the G4 watch views |
| Effort | medium |
| Decided by | (Grey) |

## Summary

Two visual ideas for once `poo watch` lands (G4). First, make the ids in
rendered output act as links: clicking a transaction id opens "Where a
payment stands" filled in, and a pool id opens that pool. Second, draw a
small live picture of each followed pool: its tip entries as a strip of
blocks, the selected one highlighted, sized by the weight `poo` prints.

## Problem

The views render JSON as tables. Following a payment means copying a
hash from one view and pasting it into another form. The ledger's most
important idea, heavier ledgers winning, is hard to see in a table.

## Idea

- `render.js` already marks hex values (`.hex`, click to copy). Give it a
  map from known field names (`tid`, `pool`, `entry`) to the form that
  takes them, and render those as links that prefill the form. The form
  still shows and runs the exact command.
- A pool picture draws only what `poo status` or `poo watch` prints:
  entries, their weights and which is selected. It computes nothing. When
  a heavier ledger takes over, the picture shows the switch as it happens.
- Inline SVG, no library, so the CSP stays as it is.

## Cost and risk

Medium. The links are small. The picture depends on the exact fields
`poo watch` prints, so it should wait for G4. The risk to guard against
is the picture implying something (an order, a total) that the command
did not print; it must label every number with the field it came from.

## Prototype

None yet. Green can prototype on top of G4 when it exists.
