# ORANGE-015: Draw a long import plan 500 lines at a time

| Field | Value |
| --- | --- |
| Status | adopted (Round 21 Part A) |
| Based on | main @ 9628426 |
| Area | gui/web/static/views/policies.js (`showPlan`, new `planTable`), gui/e2e/views.mjs |
| Effort | small |
| Decided by | |

## Summary

The Import node policies form draws `poon policy import -json` as a table,
with one row for each line of the plan. A plan has a line for each
issuer, series, follow and peer the file adds. `poon` reads files of up to
16 MiB, so a valid file can make a plan of 100,000 lines or more. The
form drew every line at once:

- A file following 10,000 issuers (1.5 MB, 20,000 lines) took 2.4 s to
  show its plan.
- 20,000 issuers (3 MB, 40,000 lines) took 4.6 s, and held the page for
  up to 0.9 s at a time.
- 50,000 issuers (7.4 MB, 100,000 lines) never showed a plan, and so
  never offered Apply. The rows went into one call as arguments, and
  that many arguments overflow the browser's stack ("Maximum call stack
  size exceeded"). Only the raw output appeared.

With the change, the form draws the first 500 lines, the same limit the
other tables use (`render.js`). A note says how many lines there are,
that Apply makes every one, and that "Output as printed" has them all.
"Show more" draws 500 more each time it is pressed. The rows are built
in a loop, never passed as arguments, so no length can overflow the
stack. Defences ("Read first") and the other notes are listed in full as
before, since `poon` prints them separately.

| Policy file (lines in the plan) | Before: until the plan shows | After: until the plan shows | After: one "Show more" |
| --- | --- | --- | --- |
| 1,000 issuers (2,000) | 242 ms | 150 ms | 123 ms |
| 10,000 issuers (20,000) | 2.4 s, longest task 211 ms | 0.84 s, 57 ms | 120 ms |
| 20,000 issuers (40,000) | 4.6 s, longest task 858 ms | 0.90 s, 72 ms | 119 ms |
| 50,000 issuers (100,000) | never shows: page error, no Apply | 2.2 s, 172 ms | 113 ms |

`poon` takes most of what remains after the change: on its own, it makes
the 50,000-issuer plan in 2.0 s. The page drew 220,000 elements for the
20,000-line plan before, and about 6,000 after. Each "Show more" press
held the page for no more than one frame (17 ms).

## Measured how

Each file was an exported bundle whose policy accepts and follows N
random issuers, with no node policy saved yet, so every line is
"added". A Playwright script started the real `poon-gui` with `POON_HOME`
set to an empty directory, typed the file's path into Import node
policies, pressed Preview, and timed until the plan's table was drawn.
It recorded long tasks and page errors. The before and after builds ran
back to back.

## Tried and not kept

The first prototype drew the remaining lines in parts on one press, one
part per animation frame. That kept the first view fast, but drawing all
100,000 lines still took 44 s, with frames up to 3 s apart, because
layout grows with the table. Two other variants did not help:

- each part in its own `tbody` with `content-visibility: auto`: 44 s;
- `table-layout: fixed`: 35 s.

Drawing 500 more on each press costs about 0.1 s whatever the plan's
length. For every line at once there is "Output as printed", and the
terminal.

## Checks

- `gui/e2e/views.mjs` adds a plan of 2,000 lines. It requires 500 rows
  and the "Showing 500 of 2000 lines" note at first, then 1,000 after one
  press, and 2,000 with the note gone after the last.
- The views, qol, streams and parity checks pass.

## Modularity

Nothing to challenge. `planTable` is one small function beside
`showPlan`, and draws exactly the lines `poon` printed.

## Consensus

None. This only changes how many lines of a plan the page draws at once.
