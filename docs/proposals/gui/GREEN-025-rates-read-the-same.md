# GREEN-025: Rates read the same everywhere

| Field | Value |
| --- | --- |
| Status | adopted (Round 6) |
| Based on | claude/gui-4-watch @ 8b044ac |
| Area | gui/web/static (render.js rateText; work.js found table) |
| Effort | small |
| Decided by | (Grey) |

## Summary

The live counter in "Run a job" showed `29.04 MH/s`, but the same rate in
the Jobs and Benchmark results showed `29039117.32639153`. Now every
printed field named `rate` is shown with the same `rateText`, and the
exact value is in the tooltip. The found table's hashes also go through
`render`, so they shorten, copy and look like hashes everywhere else
(GREEN-014's rule).

## Idea

- `rateText` moves from `work.js` to `render.js`, and `scalar()` uses it
  for a number under the key `rate`. The tooltip says
  "N tries a second, as printed".
- Nothing else changes. Other numbers are shown as printed.

## Cost and risk

About 10 lines. `rate` is only used by `poo grind` today. If another
command ever prints a `rate` that isn't tries a second, the key check
would need its context.

## Prototype

On the branch. `gui/e2e/work.mjs` checks for `H/s` in the jobs table.
