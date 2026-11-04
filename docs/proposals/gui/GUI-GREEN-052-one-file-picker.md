# GUI-GREEN-052: One file picker for every form that reads a file

| Field | Value |
| --- | --- |
| Status | adopted (Round 27 Part A) |
| Based on | main @ eb74797 |
| Area | gui/web/static/views.js (pickFile), views/policies.js (importForm), views/node.js (Add a release), views/crypto.js (Check a release), gui/e2e/views.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 26 Part A added **Add a release** with an "Or pick a file" picker.
It is a copy, line for line, of the one in `importForm` (policy, settings
and peers import): the same file input, the same switch of the field to
`-`, the same drop of the file when a path is typed again, and the same
note. Two copies of how a picked file reaches poo can drift apart, and
the next form that reads a file (a third import, a series file) would
make a third.

This proposal makes the picker one helper, `pickFile(form, {field,
onPick})` in views.js, used by both. It also gives **Check a release**
a picker: `poo series release FILE -check -` reads the release from its
input (D166), so a release made elsewhere can now be checked from the
browser before it is given to a node with Add a release.

## Problem

1. **The same feature written twice.** jerry's standing rule is that
   features are modular. `importForm` and Add a release each had about 12
   lines that did the same thing, and each had its own `picked` variable
   and its own way of feeding the job's input.
2. **Check a release took only a path.** The two steps a node owner takes
   with a release they were sent are: check it against the series, then
   give it to the node. The second step offered a picker and the first
   did not, so the file had to be saved somewhere on disk first.

## Idea

- **views.js, `pickFile(f, { field = 'file', onPick })`:**
  - It adds the "Or pick a file" field under `f.inputs[field]`.
  - A picked file sets the field to `-`. Typing a path again drops the
    picked file.
  - It returns `get()`, the file picked now, and `feed(job, file)`, which
    sends the file to the job's input with `api.inputFile` (streamed, as
    in ORANGE-014), only while the field is `-`.
  - The input keeps the name `pick` for a field named `file`, so existing
    tests and selectors still work. Another field gets `pick-FIELD`.
- **importForm:** uses `pickFile`. It keeps its own rule that Apply sends
  the file previewed (GUI-023): it holds the File object from the preview
  and passes it to `feed`, never the one picked now.
- **Add a release:** uses `pickFile`, with no change in behaviour.
- **Check a release:** a picker on the Release file field, sent as
  `-check -`.

This changes nothing in `poo` and adds no consensus rule.

## For Source

**A waiting refusal is told apart only by its words.** D166 makes a mint
of a part this node has no release for "waiting", never invalid. poo
exits 1 for it, as for any error. To show "waiting for a release" rather
than a failure, runJob matches poo's message text
(`/waiting for a release/`). A script that runs `poo claim` or `poo pay`
has to do the same.

A distinct exit code for a waiting refusal would let both the GUI and
scripts tell it apart without reading prose, for example 3, documented in
`poo help`. Exit codes are not consensus, so this is a CLI choice for
Source to make.

## Note for Grey (no change made)

Release a part's preview says "with every part before it, serials 0 to
L: L + 1 mints". It works that total out in the page from part K's last
serial. That is true by the design, because serials run from 0 with no
gap, but it is a sum the GUI does itself (see GUI-GREEN-050). If
`poo series parts` printed the mints through part K (its `MintsThrough`),
the page could show poo's number instead.

## Cost and risk

- Small: one helper of about 25 lines replaces two copies. Check a
  release's form moves into a small function so it can hold its picker.
- importForm's GUI-023 guarantee is unchanged. views.mjs's import checks
  pass: a picked file previewed and applied, and picking another file
  taking Apply away.

## Prototype

Built on this branch.

- views.mjs: picking the release file in Check a release previews
  `poo series release … -check -`, runs, and poo says it "releases part
  2, serials 8 to 11", the same answer as for the path.
- variable.mjs: Add a release with a picked file passes unchanged.
- All ten e2e scripts and the gui Go tests pass locally.
