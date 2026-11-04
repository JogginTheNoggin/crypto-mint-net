# ORANGE-020: Draw "Output as printed" only when it is opened

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ b9fed52 |
| Area | gui/web/static/views.js (runJob, showResult, panel, form, a new rawFold), the five panels that fold their result (node.js twice, peers.js, policies.js, work.js), gui/e2e/qol.mjs |
| Effort | small |
| Decided by | |

## Summary

Five panels draw poo's JSON their own way and keep the original in a
closed "Output as printed" `details`:

- the Node view's Health check and Variable series panels;
- the import plans (node policy and peers);
- the Work view's tables.

`runJob` drew the whole result through `render.js` before the panel's
`onResult` moved it into that `details`. So every run built and laid out
a second copy of the answer that is hidden unless someone opens it.

ORANGE-019 noted this for the Variable series panel, and GUI Grey asked
for a measured proposal. This branch draws the folded result only when
"Output as printed" is first opened. At 4,096 followed series:

- the Node view drops from 60,970 to 28,887 elements;
- the Variable series panel shows in 0.35 to 0.42 s instead of 0.45 to
  1.0 s;
- the longest task falls from 86–186 ms to 51–114 ms.

Opening "Output as printed" costs what it did before, because the hidden
copy was laid out on opening anyway.

## Problem

`showResult` renders JSON at once. A panel that folds the result does so
in `onResult`, after the result has been drawn and laid out, visible, for
one frame. For a large answer that hidden copy is most of the view:

- **Variable series:** `poo series followed` at 4,096 series is 2.9 MB.
  `render.js` draws 500 rows, each with the series' parameters as a
  nested list, which comes to 32,000 elements. The panel's own table, at
  500 rows (ORANGE-019), is 4,000.
- **Health check:** its folded copy is small, 75 elements.

Most people never open "Output as printed": it is there for a script or
a bug report, and Copy output copies poo's exact text either way.

## Measurements

The setup was a real `poo node` here, following 4,096 signed variable
series, and the real `poo-gui` with Chromium through Playwright. The
probe opened the Node view and waited for the Variable series table. It
sampled for 15 s, then (in a second set) opened that panel's "Output as
printed". Builds ran back to back: main @ b9fed52, then this branch.

| Node view, 4,096 series | main @ b9fed52 (6 runs) | this branch (6 runs) |
| --- | --- | --- |
| Variable series panel shows | 0.45 to 1.0 s | 0.35 to 0.42 s |
| Page busy, 15 s (3 runs) | 1.2 to 2.0 s | 1.0 to 1.3 s |
| Longest task | 86 to 186 ms | 51 to 114 ms |
| Elements in the view | 60,970 | 28,887 |
| Elements in the two folded outputs | 75 and 32,016 | 4 and 4 (undrawn) |
| Opening Variable series' "Output as printed" (3 runs) | 0.72 to 0.75 s | 0.63 to 0.85 s |

Import previews barely change. Their folded JSON is small next to the
plan, which ORANGE-015 already draws 500 lines at a time. Two runs of
each build gave the following:

| Preview | main @ b9fed52 | this branch |
| --- | --- | --- |
| 20,000 issuers | 0.93 to 1.03 s | 0.89 to 0.93 s |
| 50,000 issuers | 2.1 to 2.9 s | 1.9 to 2.4 s |

## Idea

- `showResult(out, err, { lazy })`: with `lazy`, JSON output gets an
  empty `.undrawn` placeholder and `box.draw()` to draw it. Text output,
  errors, hints and the Copy output button are built as before.
- `panel` and `form` run their jobs with `lazy`. Right after `onResult`,
  `drawn(area)` draws any lazy result that no `onResult` folded away. A
  panel or form that does not fold its result therefore looks exactly as
  before, in the same task.
- `rawFold(raw)` replaces the five copies of "make a `details`, append
  the result". It draws the result on the first `toggle` that opens it.
- Direct `runJob` calls, such as Release a part and the policy import's
  Apply, do not pass `lazy` and are unchanged.

The GUI still shows only what poo printed, and the same `render.js`
draws it. Only the moment it is drawn changes.

## Cost and risk

- About 25 lines in views.js. The five call sites get shorter.
- A risk is a panel that reads its own drawn result in `onResult`. None
  does: the five that fold all read `r.out`, and `drawn(area)` runs after
  `onResult` for the rest.
- A test that looks inside a folded result must open it first. Only
  qol.mjs looks there, and it now checks the folding.

## Prototype

This is built on this branch. `gui/e2e/qol.mjs` checks the Health check
panel:

- before opening, the folded result holds `.undrawn` and no table or
  list;
- opening it draws poo's output;
- closing and opening it again does not draw it a second time.

All ten e2e scripts pass.

## Modularity

`rawFold` turns five copies of the same five lines into one helper. That
is the only structural change.

## Consensus

None.
