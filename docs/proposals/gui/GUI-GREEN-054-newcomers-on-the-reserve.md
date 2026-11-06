# GUI-GREEN-054: Newcomers on the reserve default

| Field | Value |
| --- | --- |
| Status | adopted (Round 29 Part A, merged as is) |
| Based on | main @ adcf216 |
| Area | gui/web/static/start.js (Getting started), views/node.js (Followed series panel, rowsTable), gui/e2e/qol.mjs, gui/e2e/views.mjs, docs/FEATURES.md, docs/gui.md |
| Effort | small |
| Decided by | GUI Grey, Round 29 Part A |

## Summary

Since D170, `poon init` with no series follows the **built-in reserve
series**, the one every node follows out of the box. I walked a newcomer's
first steps on it, headless and in the GUI, on a fresh home:

- init, node, status and doctor;
- a claim of a mint;
- `poon build` of the entry: about 1 M tries at the entry floor of 20, in
  0.2 s;
- `poon work`: 2^26 tries, 7 s, for a layer at the floor of 24.

All of it works and is quick on a CPU. Two things in the GUI still point a
newcomer the wrong way, and one hides what they are on:

1. **Getting started writes the worked example, not the default.** Its
   "Write a node policy" step is `poon init -example`. A newcomer who
   follows the guide ends up on the documents' small example series,
   unlike every node set up by plain `poon init`, and unlike the network
   they will want to join.
2. **The Node view never says "reserve".** A newcomer sees a 64-hex id in
   two tables of 1,024 pool rows each (Followed pools, and What the node
   follows). `poon series followed` already says `"builtin": "reserve"`,
   only when the full id matches (D170), but no panel showed it. The
   Variable series panel reads the same reply and says only "None of the
   1 followed series is variable."

## Idea

- **Getting started → Write a node policy** runs `poon init` (with the
  Settings `-root` and `-policy`, as before). Its text says the command
  follows the built-in reserve series, that poon prints the full id, and
  that `-example` tries the small worked example instead. The step links
  to the Node view, like the other steps that have one.
- **Node → Followed series**, a new panel before Followed pools, runs
  `poon series followed`. It draws one row per series:
  - the name (from `builtin`: "the built-in reserve series", or
    "built in: X" for a later one);
  - the id, as 16 hex with the full id on hover;
  - fixed or variable;
  - pools, mints (for a variable series, the mints released so far);
  - units per mint, and the entry and layer floors.

  Everything comes from poon's JSON. The GUI never compares ids itself,
  so a look-alike id is never named the reserve (Red's point, D170). The
  raw output stays one click away.
- **`rowsTable`.** The Variable series table's code that draws 500 rows
  at a time with Show more (ORANGE-019) becomes `rowsTable`, shared by
  both panels. `variableTable` keeps its signature, classes and text.

## Notes for others (not built here)

- **Orange:** on the reserve, a newcomer's Node view draws two pool
  tables of 1,024 rows each. Each is cut at 500, and Followed pools
  redraws its rows every 5 s. A summary first ("1,024 pools, 1 with
  entries") with the full table folded might be lighter. Worth a
  measurement before anyone changes it.
- **Grey:** Getting started still has no step from "Get an address" to
  owning a mint: Claim a mint (`wallet claim -serial N`), then
  `grind start pending` to put it in an entry. I left it out, because
  the guide would have to name a serial, and every newcomer claiming the
  same one is a launch-fairness question jerry put out of scope. A step
  that says "pick an unclaimed serial in Explore" may be enough.

## Cost and risk

About 30 lines in node.js and one line in start.js. There is one more
`poon series followed` call when the Node view opens, the same call the
Variable series panel makes. No rule, consensus or policy change: the
name is poon's `builtin` field.

## Prototype

This PR.
- qol.mjs checks the Getting started line (`poon init -root …`).
- qol.mjs draws `seriesTable` with a reserve row and a variable row, and
  checks the name, mints and floors.
- views.mjs checks the Followed series row of the worked example's
  series: no name, fixed.
