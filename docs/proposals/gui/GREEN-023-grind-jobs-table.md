# GREEN-023: The grind jobs as a table, with Stop on each

| Field | Value |
| --- | --- |
| Status | adopted (Round 6) |
| Based on | claude/gui-4-watch @ 8b044ac |
| Area | gui/web/static/work.js (jobsTable, the Jobs panel's onResult), style.css |
| Effort | small |
| Decided by | (Grey) |

## Summary

The Work view's Jobs panel printed `poon grind jobs` as a generic tree, so
a list of jobs became a long column of nested `spec`, `plans` and raw
numbers. To stop one, the user read its id there and typed it into the
"Stop a job" form further down. Now the panel draws one row per job:

- id;
- what it works on (target, pools, hours, `for`, and who started it);
- a state pill with poon's reason under it;
- the device and grinder of each plan;
- tries, rate, and layers / entries found;
- a Stop button on each job that is still running or waiting.

## Problem

Seen while using the view: after "Start a job in the background", the
only sign of the job was a block of key/value pairs, and stopping it took
three steps. A job waiting with "nothing to work on yet" looked the same
as a busy one unless you read the `state` line.

## Idea

- `panel`'s `onResult` (GREEN-020) replaces the generic rendering with
  `jobsTable` and keeps the output as printed under a fold.
- Stop runs `poon grind stop ID` through `api.run`, which is the same
  command as the form's, and then refreshes the panel. The form stays for
  `-all` and for ids not shown.
- The pill colours follow doctor's: running is ok, waiting is warn,
  failed is fail, and ended is skip. The reason is printed under it, and
  left out when it only repeats the state.

A further idea, not prototyped: when a `tip` job waits with "nothing to
work on yet" on a fresh node, a next-step line could say "claim or pay
first, or use the pending target". That would be a GREEN-007 hint, keyed
to the reason.

## Cost and risk

About 40 lines. The table shows printed fields only. The one derived
item is the plan text, which joins each plan's printed device, grinder,
hash and threads.

## Prototype

On the branch. `gui/e2e/work.mjs` starts a background job, finds its row
with Stop, checks the rates read as H/s, presses Stop, and waits for the
row to lose its button.
