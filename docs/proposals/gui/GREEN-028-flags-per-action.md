# GREEN-028: Forms show only the flags the chosen action takes

| Field | Value |
| --- | --- |
| Status | adopted (Round 7) |
| Based on | claude/gui-4-watch @ e7df74b |
| Area | gui/web/static/views.js (form fields' only, the Peers and Node policies forms) |
| Effort | small |
| Decided by | (Grey) |

## Summary

A mismatch between the GUI and `poon`:
- "Act on a peer" showed Seconds, Whole range, Clear and Tracking
  template for every action.
- Its range box was labelled "(block, unblock)".
- "Unarchive fresh" showed for use, archive and delete too.

Since Source's GREEN-015, `poon` refuses a flag given to an action that
doesn't read it ("-range does nothing for unblock; it is for block").
So the GUI offered combinations that could only fail. Now a field can
say which actions it belongs to. It is shown, and sent, only while the
action select holds one of them.

## Problem

Using the Peers view: choose unblock, tick "Whole range (block,
unblock)" as the label suggests, and press Apply. `poon` refuses it. The
same happens with Seconds on delete, and with Fresh on use. The label was
wrong, and the other fields were noise for the action chosen.

## Idea

- `form()` fields take `only: { field, values }`. A field whose `only`
  doesn't match is hidden, and contributes nothing to the argv, so a
  value typed earlier can't leak into another action's command. The
  preview shows exactly what will run.
- The pairs are copied from the core's own `flagsFor` maps:

  | Command | Flag | Actions |
  | --- | --- | --- |
  | `poon peer` | seconds | timeout, block |
  | `poon peer` | range | block |
  | `poon peer` | what | clear |
  | `poon peer` | template | template |
  | `poon policy` | fresh | unarchive |

- The copy can drift. The lasting fix is for GREEN-004's `poon help -json`
  to print each flag's owners from those same maps. Then the GUI reads
  them and keeps no list of its own. Until then, a drift only means that
  `poon` refuses, which it does today.

## Cost and risk

About 20 lines in `form()`, plus the two field lists. Forms without
`only` are unchanged.

## Prototype

On the branch. `gui/e2e/qol.mjs` checks:
- block shows Seconds and Whole range, and previews
  `poon peer block -range ADDR`;
- switching to unblock hides both and previews `poon peer unblock ADDR`,
  with the ticked box no longer sent;
- clear shows only Clear.
