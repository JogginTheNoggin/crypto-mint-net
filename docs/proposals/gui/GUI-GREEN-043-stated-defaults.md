# GUI-GREEN-043: A field's stated default is poon's: a check, and one wrong hint

| Field | Value |
| --- | --- |
| Status | adopted (Round 18 Part A) |
| Based on | main @ 052d032 |
| Area | gui/e2e/parity.mjs, gui/web/static/work.js (Schedule a job) |
| Effort | small |
| Decided by | (Grey) |

## Summary

Many fields state poon's default in their placeholder, such as "default
16", "default 60000", "default 1,2,4,8" or "default ed25519". Those values
are copies of poon's, and nothing checked them. A change in Source would
leave the GUI telling users something untrue.

This extends GUI-GREEN-042's `parity.mjs`. Every field whose placeholder
states a default value is filled alone, which shows which flag it is. Its
value is then compared with the `(default X)` that `poon CMD -h` prints
for that flag. Go leaves a zero default out, so it counts as 0.

To keep the comparison exact, only values are checked: numbers, lists,
and ed25519, sha256 or auto. Prose such as "default: the tip" is left
alone.

Doing the same comparison by hand for the prose found one wrong hint and
one Source help slip.

## Problem

- **Schedule a job** reuses the job fields of a one-off job. Its Threads
  says "default: every core but one", and its Device says "default:
  node.json, else auto". But `poon grind schedule add -h` says threads
  default to the section's. That is the Schedule defaults form just
  below, which poon applies to every scheduled job. A user who set
  Schedule defaults to 1 thread is told the job will use every core.
- **For Source:** `poon grind start -h` and `poon grind run -h` say
  `-series` defaults to "the only followed series". But a job with no
  `-series`, `-pool` or `-serial` leaves its spec's series empty and
  grinds every followed series. `grind help` and the GUI both say so.
  The flag's help comes from the shared client flags, and it is wrong
  only here.

## Idea

1. In `work.js`, `scheduled(fd)` rewords Device and Threads for Schedule
   a job: "default: the section's, see Schedule defaults", as poon
   documents it. The other job forms keep their wording.
2. In `parity.mjs`, the stated-default pass: each mismatch is reported as
   "poon sim load -nodes: the GUI says default 15, poon says 16". It needs
   at least 6 such fields, so a broken reader can't pass without
   checking anything.
3. For Source: give `grind start` and `run` their own `-series` help,
   "(default: every followed series)", as `schedule add` already has.

## Cost and risk

- About 10 lines in `work.js` and 45 in the check, which adds about 5
  seconds.
- The check reads only placeholders that state a value, so prose
  defaults still depend on review. That is how this round's two prose
  findings were found.

## Prototype

On `claude/gui-green`. I made sure it fails: with Simulate's Nodes
placeholder changed to "default 15", it reports all four `sim`
scenarios. On the real code it passes, and all nine e2e scripts pass.
