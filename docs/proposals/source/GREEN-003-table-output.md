# GREEN-003: Readable table output for view commands

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/cli` (`view.go`, `node.go`) |
| Effort | medium |
| Decided by | |

## Summary

Add `-o table` (and `-o json`, the default) to `poo status`, `pools`,
`ledger`, `pending`, `unspent`, `address`, `peers` and `tracking`, so a
person reading a terminal gets aligned columns with shortened hashes, while
scripts keep the JSON they rely on. No prototype yet.

## Problem

Every read command prints indented JSON. `poo status` for two pools is
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
- Optional later: `POO_OUTPUT=table` in the environment for people who
  always want tables.

## Cost and risk

About 300 lines plus golden files. The risk is two output forms drifting
from the JSON fields, which the golden tests catch. The default stays JSON,
so nothing breaks.

## Prototype

None yet. Green can build it if Grey wants it.
