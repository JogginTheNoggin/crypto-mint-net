# GREEN-003: Readable table output for view commands

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/cli` (`view.go`, `node.go`) |
| Effort | medium |
| Decided by | Grey: merged as prototyped (columns from the JSON keys) in the GREEN-003/007 merge |

## Summary

Add `-o table` (and `-o json`, the default) to `poon status`, `pools`,
`ledger`, `pending`, `unspent`, `address`, `peers` and `tracking`, so a
person reading a terminal gets aligned columns with shortened hashes, while
scripts keep the JSON they rely on. No prototype yet.

## Problem

Every read command prints indented JSON. `poon status` for two pools is
about 30 lines of 64-character hashes. It is good for scripts and the GUI,
but hard to scan by eye.

## Idea

- A shared `-o json|table` flag, JSON by default, so nothing that parses
  output today changes, and the GUI terminal behaves exactly like headless
  (jerry's rule).
- Tables use `text/tabwriter`, show hashes as the first 8 and last 4 hex
  digits, and `-wide` prints them in full.
- One small formatter per reply type, beside the reply, tested with a
  golden file per command.
- Optional later: `POON_OUTPUT=table` in the environment for people who
  always want tables.

## Cost and risk

About 300 lines plus golden files. The risk is two output forms drifting
from the JSON fields, which the golden tests catch. The default stays JSON,
so nothing breaks.

## Prototype

On `claude/source-green`, based on `claude/m10-watch` @ `5ea6828`: `pkg/cli/table.go` and
`pkg/cli/table_test.go`; `-o` and `-wide` on `status`, `pools`, `ledger`,
`pending`, `unspent`, `tx`, `address`, `holdings`, `peers`, `tracking` and
`chases`; a usage line and a paragraph in `docs/modules/poon.md`.

The prototype differs from the idea above in one way. It has no
per-reply formatters. The table is built from the reply's JSON, read with
its key order kept. A list of objects becomes columns named after the JSON
fields, and small flat objects become dotted columns (`input.tid`). An
object becomes one field per line, with a titled table for each list in
it. Lists of objects inside a row show their count (`[3]`). So a field
added to a reply shows up in the table with no change here, and the table
can never disagree with the JSON. Golden tests per command were not needed:
one exact-output test covers the renderer.

Not done: `POON_OUTPUT`, and table output for `watch` (a stream) and the
wallet commands.
