# GUI-GREEN-044: One file per view, one list of views

| Field | Value |
| --- | --- |
| Status | adopted (Round 19 Part A) |
| Based on | main @ 3804c55 |
| Area | gui/web/static (views.js, views/, app.js), gui/e2e (parity.mjs, qol.mjs), docs/FEATURES.md, docs/gui.md |
| Effort | small (code moved, not changed) |
| Decided by | (Grey) |

## Summary

`views.js` held two things in 950 lines: the panel and form helpers every
view is built from, and 8 of the GUI's 11 views. Watch, Explore and Work
had their own files, and the policy export and import form had just added
another 120 lines. The list of views was hard-coded in three places:
`views.js` (the navigation) and two e2e scripts. A new view would show in
the navigation, but GUI-GREEN-042's flag check and GREEN-040's command
check would quietly skip it.

This makes the GUI modular the way FEATURES.md's "Adding a feature" asks:

- `views.js` keeps only the helpers: `el`, `runJob`, `panel`, `form`,
  `flag`, `gate`, `mountAll` and the rest. It drops from 950 to 380 lines.
- Each view is one file in `gui/web/static/views/`: `node`, `pools`,
  `peers`, `wallet`, `policies`, `crypto`, `settings` and `jobs`, plus
  `watch` and `work`, moved there. Each exports `{ id, title, mount }`.
- `views/index.js` is the one list of views, in navigation order. It
  starts with the view contract and how to add a view.
- `parity.mjs` and `qol.mjs` read the views from the navigation rather
  than a hand list, so a new view is checked as soon as it is listed.
  The parity check now covers 59 commands, including Jobs and Settings.
- FEATURES.md gets a "One file per view" row and two GUI steps under
  "Adding a feature": add the form or view, then run `parity.mjs`.
  gui.md says where views live.

## Problem

- Every change to any view touched the same 950-line file. Grey, Red,
  Blue and Green all edit it each round, so merges collide on it.
- The view list lived in three places. The e2e lists had already drifted:
  they named 9 of the 11 views and missed Jobs and Settings.
- A contributor had no written path from "a new poo command" to "a new
  form", beyond copying a neighbour.

## Idea

- The code moved without changes. A check of the diff confirms it: the
  lines removed from `views.js` are exactly the lines in the new files,
  apart from each file's imports, its first comment line, and `export` on
  each view object.
- Imports are explicit per file, so a view's dependencies show at its
  top. `watch.js` and `work.js` only had their paths changed (`../`).
- `views/index.js` documents the contract. `mount(root, ctx)` draws the
  view and returns a function that stops what it started, or nothing.
  Every action is a `poo` command line the helpers show and run.
- No behaviour change. The served files are still embedded with
  `//go:embed static`, which includes the new directory. The CSP, which
  allows scripts from self only, is unchanged.

## Cost and risk

- A large but mechanical diff. Branches with edits to a view in
  `views.js` will conflict once, and the fix is to move the edit into
  that view's file.
- One more directory to know about. Its index.js says what it is for.
- Nothing runs differently. All nine e2e scripts pass, and so do
  `go test ./...` and `go vet ./...` in `gui/`.

## Prototype

On `claude/gui-green`, in the PR to main.
