# GREEN-002: Tab completion in the terminal, read from poo's help

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-5-release @ f93508e |
| Area | gui/web/static/terminal.js, new complete.js |
| Effort | small |
| Decided by | (Grey) |

## Summary

Tab in the terminal completes command and subcommand names. The words
come from running `poo help` and `poo CMD help` through the normal job
API, so the list can only name commands the headless tool has. A unique
match completes; several are listed, like a shell's double Tab.

## Problem

The terminal is the escape hatch for everything the views do not cover,
but it has no discoverability: the user must remember `wallet receive`
or `peer unblock`. Shells give completion; the GUI's terminal should not
be worse than the shell it stands in for.

## Idea

- On Tab with the cursor at the end of the line, `complete.js` runs
  `help` once (and `CMD help` for a command that has its own), caches the
  result for the page's life, and parses the names out of the usage text.
  `poo policy list|use|...` style lines give inline subcommands.
- Nothing typed is changed except the word under the cursor, and nothing
  runs until the user presses Enter.
- No list of commands lives in the GUI. If `pkg/cli` gains a command, its
  help line makes it completable.

Limits: flags are not completed, and parsing usage text depends on the
help layout. GREEN-004 proposes a machine-readable help to remove that
dependency.

## Cost and risk

About 70 lines. Each help run shows up as a job in the Jobs view (two or
three per page load at most). If the help layout changes, completion
silently offers fewer words; it can never offer a wrong command that then
runs, because Enter still goes to `cli.Main`.

## Prototype

`gui/web/static/complete.js`, the Tab binding in `terminal.js`, and checks
in `gui/e2e/qol.mjs` (a command, a wallet subcommand, an inline policy
subcommand, and an ambiguous prefix).
