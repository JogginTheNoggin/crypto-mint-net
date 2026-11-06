# GREEN-033: Schedule defaults show what is set now

| Field | Value |
| --- | --- |
| Status | adopted (Round 10) |
| Based on | claude/gui-4-watch @ 6fae281 |
| Area | gui/web/static/views.js (form `hint`), work.js (Schedule defaults) |
| Effort | small |
| Decided by | (Grey) |

## Summary

The Schedule defaults form now has four fields: device, threads, layer
floor and layer rate. All four start empty, and the only place that shows
the current values is the one-line summary in the Schedule panel above.
With this change, each field's placeholder shows the value set now, for
example "now 1" or "now auto". An empty field still sends nothing, so it
keeps that value, just as `poon grind schedule set` does.

## Problem

- To change one default, you have to scroll up to read the others first,
  and you can't tell which fields matter.
- The summary line leaves out the layer rate when the floor is 0. A rate
  set earlier (say 5) stays in node.json but shows nowhere. Setting a
  floor again brings that old rate back without warning.
- `poon grind schedule set` with no flags succeeds and changes nothing. In
  the GUI, Set with every field empty looks like it did something.

## Idea

- `form()` returns `hint(name, text)`, which sets a field's placeholder.
  It changes nothing else, and no value is filled in or sent.
- Each time the Schedule panel draws `poon grind schedule`'s reply, the
  Work view sets the four placeholders from its `grind` section. A
  missing value shows poon's default, such as "0: every core but one".
  This is poon's printed output shown again; the GUI keeps no defaults of
  its own.
- Because the layer rate placeholder always shows "now 5", the hidden
  rate is visible again.

Left for Grey to judge: turning Set off while every field is empty. That
would match how poon treats it, but it is a small change in how the form
behaves, so it is not in the prototype.

## Cost and risk

About 15 lines. The placeholders update only when the Schedule panel
reloads, which happens after every Set or Add and on Refresh. A change
made elsewhere shows after the next reload, as the table already does.

## Prototype

On `claude/gui-green`. `gui/e2e/work.mjs` sets threads to 1 and checks
that the threads placeholder becomes "now 1" and the device placeholder
is "now auto". All eight e2e scripts pass.
