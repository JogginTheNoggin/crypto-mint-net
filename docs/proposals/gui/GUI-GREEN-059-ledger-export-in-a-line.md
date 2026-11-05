# GUI-GREEN-059: Ledger export, a line and Save as file

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 3c4ae00 |
| Area | gui/web/static/views/pools.js (Export a pool's ledger, ledgerSummary), gui/e2e/views.mjs, gui/e2e/qol.mjs, CHANGELOG.md, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 33 Part A added **Export a pool's ledger** (D188). With To a file
left empty, poo prints the ledger file and the page offers Save as file.
The page also draws the whole file as a table, one row per entry, with
each entry's base64 `item` in full.

A ledger file is for another node, not for reading. I measured a real
one: a node on a series of one pool with 512 ML-DSA-87 claims, each
worked into its own entry (`poo ledger export` printed 5,163,235 bytes,
512 entries). I exported it in Chromium three times each way:

| | Export shown in | Page elements in the form | Text drawn |
| --- | --- | --- | --- |
| main @ 3c4ae00 | 2.5 to 3.4 s | 6,578 | 4.96 million characters |
| this PR | 0.25 to 0.28 s | 37 | 337 characters |

The file saved from the page was poo's exact output: it parsed, and
`poo ledger import` of it into a second node kept 512 entries and 512
layers and switched to them.

## Idea

- **One line, then Save as file.** When the output is a ledger file, the
  form says what it holds, for example "A ledger file of pool 0: 512
  entries, heights 1 to 512 of 512.", with Save as file below. poo's
  output goes under the usual closed **Output as printed**, drawn only
  when opened (ORANGE-020), with Copy output inside.
- **Save as file only for one whole ledger file.** `ledgerSummary` reads
  poo's own fields. It returns nothing when the text does not parse, is
  not `"kind": "poo ledger"`, or holds a different number of entries than
  its own `from` and `to` say. Then the form shows no Save as file, and
  says to fill in To a file so poo writes the file itself.

  poo-gui keeps the last 4 MiB of a job's output (`DefaultMaxOutput`),
  and a form's stream does not listen for the server's `dropped` event.
  So a page that falls far enough behind a long export could hold a ledger
  with its start or a middle stretch missing. In my runs the stream kept
  up (no `dropped` event, on 5.2 MB), because poo writes the export one
  RPC page at a time. The check costs nothing and means a broken file is
  never offered as a ledger.
- Nothing is decided by the GUI: the count is the length of poo's
  `entries` list, compared with poo's `from` and `to`. The node that
  imports the file still checks every entry in full (D188).

## Checked, nothing to change

- **New ports (D187):** nothing in `gui/` names 8645 or 8646. Write a node
  policy's RPC and P2P fields say "default", so poo's own help gives the
  ports and they cannot drift.
- **Node release (W-10):** against a node from the same build, the Node
  view reads "The node runs v0.1.0-alpha.1-dev." views.mjs already checks
  the "different releases" note.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)", and the Pools and payments row in FEATURES.md |
| 6. Only through pkg/cli | Page only; no Go change |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## For Grey (not built here)

- **Forms ignore `dropped`.** The terminal writes "[earlier output
  dropped]", but `runJob` has no `dropped` handler, so any form or panel
  whose output passes 4 MiB faster than the page reads it would draw a
  result with a gap and no word about it. A two-line handler could add a
  note to the result. I found no command that triggers it today: poo
  writes most large outputs in one write, which the job keeps whole, and
  the ledger export is slow enough for the stream to keep up. So I left it
  as a note.

## Cost and risk

About 20 lines in pools.js. A person who wants to read the entries opens
Output as printed. No rule, consensus, policy or `/api/` change.

## Prototype

This PR.
- views.mjs: the export says "A ledger file of pool 0 …" with Save as
  file, nothing is drawn under Output as printed until it is opened, and
  then poo's output is there.
- qol.mjs checks `ledgerSummary` on its own: a whole file with 3 entries,
  1 entry and none gives the line, and a file cut at its end or start,
  short of its range, of another kind, or poo's "wrote N entries" text
  gives nothing.
