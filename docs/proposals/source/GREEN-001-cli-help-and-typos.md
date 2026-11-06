# GREEN-001: Short errors, typo suggestions and `poon help COMMAND`

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/cli` (`main.go`, new `help.go`) |
| Effort | small |
| Decided by | Grey: merged as prototyped in 3955bb9 |

## Summary

A mistyped command prints one line with the nearest command instead of
the 68-line usage, and `poon help ledger` prints only that command's
usage. Both read the existing usage text, so nothing new has to be kept
in step. Prototype on the branch, with tests.

## Problem

`poon stauts` prints `unknown command "stauts"` followed by the whole
usage on stderr. The real error scrolls off screen, and the GUI's built-in
terminal shows the same wall of text. There is no way to see the usage of
one command, and nothing points at the likely intended command.

## Idea

- Unknown command: `poon: unknown command "stauts"; did you mean status?
  (poon help lists every command)`. Suggestions use edit distance with
  adjacent swaps (at most 2 edits, 1 for short names) and prefixes of
  three or more letters, at most three suggestions.
- `poon help COMMAND` prints that command's lines from the usage,
  including continuation lines (for example `watch`'s FILTER list), and
  points to `poon wallet help` and `poon sim help` for those two.
- The command list is parsed from the usage text, and a test checks
  every listed command is dispatched, so the two cannot drift.

## Cost and risk

About 150 lines plus tests, standard library only. The unknown-command
message changes, but it still starts with `poon: unknown command "X"`, so
`cmd/poon`'s test and any script matching that prefix keep working. The
GUI gets it for free, since it runs the same `pkg/cli`.

## Prototype

`pkg/cli/help.go` and `pkg/cli/help_test.go`; `pkg/cli/main.go` routes
`help` and the default case through them and adds `poon help [COMMAND]` to
the usage.
