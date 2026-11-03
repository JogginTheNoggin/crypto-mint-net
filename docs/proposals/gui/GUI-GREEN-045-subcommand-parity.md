# GUI-GREEN-045: Every subcommand poo lists is run by some view

| Field | Value |
| --- | --- |
| Status | adopted (Round 20 Part A) |
| Based on | main @ e71e317 |
| Area | gui/e2e/parity.mjs, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Three checks already keep the GUI level with headless poo:

- GREEN-040 checks commands (`poo help`).
- GUI-GREEN-042 checks flags (`poo CMD -h`).
- GUI-GREEN-043 checks the defaults fields state.

One level was still unchecked: subcommands chosen by an action select.
The Peers form's Action list, Node policies' Action list, Simulate's
Scenario list and Submit's Kind list are copies of poo's own lists
("poo peer timeout|block|unblock|clear|template|delete").

Suppose Source added a seventh peer action. Its flags would already
belong to the `peer` family, so the flag check would pass. The command
check only looks at `peer`, so it would pass too. The new action would
never appear in the GUI, and nothing would say so.

`parity.mjs` now reads every subcommand poo lists from three places:

- the `poo help` lines `poo CMD a|b|c` and `poo CMD sub …`;
- `poo grind help`;
- `poo wallet help`.

Each one must be run by some view's form or panel, or be in
`LEFT_OUT_SUBS` with a reason. Only `completion bash|zsh|fish` is there,
because it sets up a shell.

It passes on main today: every subcommand is reachable. I made sure it
fails: with `delete` taken out of the Peers Action list, it reports
`poo peer delete`.

## Problem

- Action selects are the one place the GUI still keeps its own copy of
  poo's vocabulary, with no check behind it.
- Panels, such as Node policies' `poo policy list` and Work's `poo grind
  schedule`, were never counted, because the flag check fills forms only.

## Idea

- The form walk now also records each panel's line, marked so that only
  this check counts it. The flag check stays about what forms can send.
  Otherwise panels with no fields, such as `doctor -json` and `status`,
  would report flags no one can choose.
- Subcommands are compared on the first two words of each command line,
  for example `peer delete` and `grind schedule`.
- It requires that `peer block`, `grind schedule` and `wallet pay` were
  read, so a change in the help's layout can't make it pass by reading
  nothing.
- FEATURES.md's quality-check row now says the check covers subcommands
  too.

## Noted for Source

`poo help` lists `poo grind devices|bench|start|run|jobs|stop` without
`schedule`, though `poo grind help` documents `grind schedule` and its
add, remove and set. The top-level line could add `|schedule`. This is
help text only, not consensus. The check reads both helps, so it isn't
affected.

## Follow-up (not in this PR)

Flag values are a fourth copy. Peers' "Clear" select (`flags`,
`measures`, `all`) and Export's scope are lists poo also prints in its
`-h` text. A later check could read "(one of …)" from the help once Source
prints values in a fixed form. Until then the lists are reviewed by hand.

## Cost and risk

About 35 lines of test, and no change to the GUI. The check adds about a
second.

## Prototype

In the PR. All nine e2e scripts pass.
