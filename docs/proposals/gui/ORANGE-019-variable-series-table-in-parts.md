# ORANGE-019: Draw the Variable series table 500 rows at a time

| Field | Value |
| --- | --- |
| Status | adopted (Round 27 Part A) |
| Based on | main @ eb74797 |
| Area | gui/web/static/views/node.js (the Variable series panel), gui/e2e/qol.mjs |
| Effort | small |
| Decided by | |

## Summary

Round 26 added a Variable series panel to the Node view (D166). It runs
`poon series followed` and draws one row for each variable series the
node follows. It drew every row. A node can follow up to 4,096 series:
past that, pkg/p2p refuses to start with "following N separate pools;
follow whole series instead". At 4,096 series, the table was 32,800
elements, more than twice the rest of the Node view (14,400), and the
panel took 1.1 to 1.25 s to appear.

The proposal draws 500 rows at first and 500 more on each press of Show
more, the way import plans do (ORANGE-015) and the way `render.js` draws
any table. With that change the table is 4,000 elements and the panel
appears in 0.6 to 0.8 s. Every row is still one press away, and the full
output is still in "Output as printed".

## Problem

The panel built its rows with `el('tbody', {}, ...vs.map(...))`, so all
of them were drawn, whatever their number. That is the same pattern
ORANGE-015 replaced in the import plans, where 100,000 rows overflowed
the stack. Here the 4,096 cap keeps the row count below that, but every
row costs eight elements, and the Node view is the first view most
people open.

## Measurements

I ran a real `poon node` here and the real `poon-gui`, with Chromium
through Playwright. The node followed 4,096 signed variable series (8
mints in parts of 4, made with `poon series new -variable` and `poon
series sign`). `poon series followed` printed 2.9 MB in 164 to 167 ms.

The probe opened the Node view, waited for the Variable series table,
and sampled for 15 s. "Panel shows" is the time from the click until
the table was drawn. "Busy" is the time the page spent on tasks in
those 15 s. Each build ran three times, back to back. The Round 25 row
is the build before the panel was added (main @ 88bd534).

| Build | Panel shows | Busy | Long tasks, longest | Elements in the view | In the table |
| --- | --- | --- | --- | --- | --- |
| Round 25 (no panel) | | 1.0 to 1.2 s | 4, 87 to 196 ms | 14,378 | |
| Round 26 (eb74797), every row | 1.07 to 1.25 s | 2.2 to 2.8 s | 4 to 6, 161 to 181 ms | 89,736 | 32,777 (4,096 rows) |
| This branch, 500 rows | 0.59 to 0.79 s | 1.3 to 1.7 s | 5 to 6, 115 to 148 ms | 60,970 | 4,009 (500 rows) |

With 1,000 followed series, `poon series followed` printed 718 KB in
60 ms.

The other changes in Round 26 cost little:

- **Add a release:** `poon series add-release` took 209 ms, including the
  save to node.json, on the node with 4,096 series. The node then
  reloaded, and `poon series followed` answered again in 161 ms.
- **Waiting for a release:** the waiting status is one regular
  expression test on a finished run's error text, once per run.

### What remains: the hidden "Output as printed"

Of the 46,600 elements the view still has beyond Round 25, 32,000 are
"Output as printed", in a closed `details` element. `runJob` draws the
whole result through `render.js`: 500 rows, each with the series'
parameters drawn as a nested list. The panel then moves that result
into the closed `details`. It is built and laid out once, then hidden.

Drawing it only when it is opened would save those 32,000 elements.
But it would mean `runJob` not drawing a result that a panel replaces,
which touches every panel (the doctor panel does the same). So this is
noted and not built here. GUI Grey can decide whether to look at it on
its own.

## Idea

`variableTable(vs, cells)` in node.js draws the first 500 rows into one
fragment. When there are more, it adds a note, "Showing 500 of N
variable series.", with a Show more button that draws 500 more each
press, and the note goes away once every row is drawn. The cells are the
same as before. The GUI computes nothing new: the cells come from poon's
answer as they did before.

## Cost and risk

- Small: about 30 lines in node.js.
- The e2e checks that read the first row (variable.mjs) are unchanged.
  The table keeps its class, `variable-series`.

## Prototype

Built on this branch. `gui/e2e/qol.mjs` checks it with 1,200 rows:

- 500 rows are drawn, with "Showing 500 of 1200 variable series."
- One press of Show more gives 1,000 rows.
- A second press gives all 1,200 and removes the note.
- Three rows are drawn with no note.

## Modularity

Nothing to challenge.

## Consensus

None. This changes only how many rows the page draws at first.
