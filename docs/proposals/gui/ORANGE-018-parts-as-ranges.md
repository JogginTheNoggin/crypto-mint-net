# ORANGE-018: Show a variable series' parts as ranges, not lists

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 2aceb1a |
| Area | gui/web/static/render.js (a cut list says so), docs/gui-variable-supply-plan.md (Series info, Release a part, Import a release), and the JSON shape of Source's series info and release commands (not yet built) |
| Effort | small |
| Decided by | |

## Summary

The GUI plan for variable supply series (docs/gui-variable-supply-plan.md)
puts a table of parts in Series info, with the columns "part, serials,
pools, units". It also shows a part's serials and pools in the Release a
part and Import a release previews. The plan draws these from the
headless output, which Source has not built yet.

How Source prints a part's serials and pools decides what that table
costs. If each part lists every serial and pool, the GUI page needs
seconds and hundreds of thousands of elements to draw Series info. If
each part prints its first and last serial and its first and last pool,
the same table draws in about 30 ms. That holds for any number of parts,
because `render.js` draws at most 500 rows.

The design document already writes parts as ranges. Its example table
says "50,000 to 59,999" and "pools 50 to 59". The proposal is to keep
that shape in the JSON:

- **Source:** in the series info, release preview and release import
  JSON, print each part as `first_serial`, `last_serial`, `first_pool`
  and `last_pool` (names are Source's to choose), never as arrays of
  every serial or pool.
- **GUI:** the plan's table columns become "part, serials (first to
  last), pools (first to last), units". Series info stays a panel that
  runs when asked, with no timed refresh.

This is about the output format only. It adds no consensus rule, and
the GUI works nothing out itself: poo prints the ranges.

## Problem

A part is large by design. Part 1 holds the series' initial N mints,
and every later part holds M. The design document's own example has
N = 50,000 and M = 10,000. `max parts` is optional, so a series that
releases a part a day has thousands of parts after a few years.

`render.js` draws a JSON array of rows as a table of at most 500 rows,
and any list inside a row as a list of at most 500 items. A part that
lists its serials and pools therefore draws up to 500 × (500 + 500)
list items. The text is also large: every serial costs about 15 bytes
of indented JSON.

## Measurements

These are from Chromium through Playwright, in this container. The probe
served `gui/web/static` from main @ 2aceb1a. It made a Series info
style JSON result, `{series, supply, part_size, parts: [...]}`, and timed
`JSON.parse` followed by `render()` and a layout. "List" gives each part
`serials: [every serial]` and `pools: [every pool]`, with one pool per 10
serials. "Range" gives each part `first_serial`, `last_serial`,
`first_pool` and `last_pool`.

| Parts | M | Shape | JSON | Draw and layout | Elements | Page height |
| --- | --- | --- | --- | --- | --- | --- |
| 10 | 1,000 | list | 153 KB | 337 ms | 12,111 | 87,000 px |
| 100 | 1,000 | list | 1.6 MB | 2.9 s | 120,921 | 870,000 px |
| 1,000 | 100 | list | 1.7 MB | 2.6 to 2.8 s | 114,522 | 874,000 px |
| 500 | 1,000 | list | 8.7 MB | 13.7 s (first run) | 604,521 | 4,352,000 px |
| 2,000 | 1,000 | list | 36 MB | 4.9 to 5.6 s | 604,522 | 4,352,000 px |
| 500 | 1,000 | range | 83 KB | 30 ms | 6,523 | 13,300 px |
| 2,000 | 1,000 | range | 339 KB | 25 ms | 6,524 | 13,300 px |
| 100,000 | 1,000 | range | 17.6 MB | 44 ms | 6,524 | 13,300 px |

With the design document's own example, N = 50,000 and M = 10,000, and
six parts in all:

| Parts | Shape | JSON | Draw and layout | Elements | Page height |
| --- | --- | --- | --- | --- | --- |
| 6 | list | 973 KB | 98 to 102 ms | 12,075 | 52,000 px |
| 6 | range | 1 KB | 1 ms | 101 | 250 px |
| 1 (a release preview, M = 10,000) | list | 152 KB | 21 to 25 ms | 2,030 | 8,800 px |
| 1 (part 1, N = 50,000) | list | 808 KB | 17 to 19 ms | 2,030 | 8,800 px |

A single part drew quickly because `render.js` drew only its first 500
serials. But it said nothing about the other 9,500, so the list seemed
to end at the 500th serial. A table says "Showing 500 of N rows", but a
plain list did not. This matters beyond parts: any list poo prints that
is longer than 500 looked complete.

What the numbers show:

- With lists, the cost grows with the number of serials until the
  500-row and 500-item caps stop it, at about 600,000 elements and a
  page 4.3 million px tall. Even 100 parts of 1,000 mints, a small
  series, take about 3 s and draw a table nobody can read.
- With ranges, the table always draws in 25 to 45 ms, and the 500-row
  cap holds it at about 6,500 elements.
- At 100,000 parts the range shape still prints 17.6 MB. The page copes
  with that, but it is the same size as the peer book in ORANGE-017. So
  Series info should run when asked and never on a timer.

## Idea

1. Source prints each part as ranges in the series info, release preview
   and release import JSON. This repeats what the design document's
   table shows and costs Source nothing: poo already knows each part's
   first and last serial.
2. The GUI plan's tables use those fields as printed, with no GUI rule
   of its own (D84). `render.js` already caps rows at 500.
3. Series info is a panel without `refresh`, like the other panels that
   run only when opened or pressed.
4. **Built here:** `render.js` now ends a list cut at 500 with "Showing
   500 of N items; the terminal shows all.", as a table already does. A
   list of 500 or fewer is drawn exactly as before.

## Cost and risk

- Source: no extra code, only the output's shape. A part's full list of
  serials, if anyone needs it, is the range written out.
- GUI: none beyond the plan, and three lines in `render.js` for the
  cut list's note. The rest of this proposal is written before either
  side is built, so that the shape is chosen once.
- Risk: none to consensus. It changes no rule and no part id, only how
  poo prints what it already knows.

## Prototype

The note on a cut list is built on this branch, and `gui/e2e/qol.mjs`
checks it: a list of 10,000 draws 500 items and the note "Showing 500 of
10000 items", and a list of three draws three items with no note.

The rest needs Source's commands, which do not exist yet. The probe that
made the numbers above is described under Measurements and can be run
again once Source prints real output.
