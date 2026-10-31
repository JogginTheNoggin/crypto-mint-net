# GREEN-031: Schedule Remove can remove the wrong job

| Field | Value |
| --- | --- |
| Status | proposed (Round 7) |
| Based on | claude/gui-4-watch @ b20bf8f |
| Area | pkg/cli/grind.go (`schedule remove`); gui/web/static/work.js (scheduleTable) |
| Effort | small |
| Decided by | (Source/Grey, then GUI/Grey) |

## Summary

`poo grind schedule remove N` removes the job at position N in
node.json. The Schedule panel's Remove button sends the position the
row had when the table was drawn. If the list changed since then, Remove
drops a different job, with no warning. This proposal asks Source for a
small guard that the GUI then passes.

## Problem

- Positions shift whenever a job is added or removed elsewhere: from the
  built-in terminal, from another browser tab, from a shell, or by
  editing node.json by hand.
- Example: the panel shows jobs 1 (tip) and 2 (tx:ab…). Someone removes
  job 1 in the terminal. Remove on the tx:ab… row still sends
  `remove 2`, and poo fails with "no scheduled job 2". With three jobs it
  would remove the wrong one instead.
- The same is true for a person in a shell who reads the list and then
  types a number, but the GUI makes the gap between reading and acting
  longer.

## Idea

This is node policy and command-line shape only, not consensus.

1. Source: `schedule remove N -target T` (or a short job id printed by
   `poo grind schedule`). poo removes job N only if its target is still
   T, and otherwise fails and says what job N is now.
2. GUI: the Remove button sends `-target` with the row's target, and on
   failure refreshes the table, so the person sees the current list.

The GUI keeps no list of its own and decides nothing: poo still checks.

Also noted for Grey: "Until from reaches" shows in the job forms for
every target, but only applies to an entry hash or a tx: target. Its
placeholder says so, and poo rejects a wrong use, so this is cosmetic.

## Cost and risk

- Source: about 10 lines and a test. Old commands keep working, because
  `-target` is optional.
- GUI: one flag on the Remove button and a refresh on error.

## Prototype

None yet, because it needs the Source flag first.
