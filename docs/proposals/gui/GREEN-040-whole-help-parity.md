# GREEN-040: Every command in poo help is in a view, or listed as terminal-only

| Field | Value |
| --- | --- |
| Status | adopted (Round 15), with the -bytes and submit -f follow-ups |
| Based on | claude/gui-4-watch @ 3a7a39c |
| Area | gui/web/static/views.js (Series and keys), gui/e2e/qol.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

GREEN-039 kept the Wallet view level with `poo wallet help`. This does the
same for the whole of `poo help`. I listed what every view and Getting
started step runs and compared it with the help. Two commands that make
sense in the GUI had no form:

- `poo series example`, the worked example's series as JSON;
- `poo sim load|partition|attack|sizing`, the deterministic network
  simulator. It is offline and needs no node or files.

Both now have a form on Series and keys. A new check in `qol.mjs` reads
`poo help` at test time. It fails when a command is neither run by some
view nor in a short `TERMINAL_ONLY` list that gives the reason.

## Problem

- The rule is that the GUI is the same `poo`. Without a check, a new
  command appears in the terminal and nobody notices the GUI lacks it.
  That happened to 7 wallet commands (GREEN-039), and to `sim` and
  `series example` here.
- Some commands should stay in the terminal. Without a list, nobody can
  tell "left out on purpose" from "forgotten".

## Idea

- **Simulate the network** (Series and keys):
  - Fields: a scenario select, Seed, Runs, Nodes and Duration, plus Pool
    sizes, which shows only for `sizing` and uses `only`.
  - Other flags are in the terminal (`poo sim load -h`), as "New series
    parameters" already says for its own command.
  - A partition run takes about 0.2 seconds.
- **The worked example's series**: one button running `poo series
  example`.
- **TERMINAL_ONLY**, kept in the e2e next to the check, each with its
  reason:
  - `help`: the terminal has its own help, and every form shows its line.
  - `completion`: it sets up a shell, not the GUI.
  - `holdings` and `payment`: older forms of `unspent` and `tx`, kept
    for scripts.
  - `claim` and `pay` with a raw key file: the Wallet view does both from
    the wallet.

  Grey can move any of these into a view; the check accepts either.

## Waiting on Source (not merged yet)

- **Source GREEN-039**, `-bytes` on `build` and `work`: Pools and
  payments' Build and Work forms would get a "Print the bytes" checkbox,
  and the hex they print could go straight into "Submit an entry or
  layer".
- **BLUE-015**, `submit -f FILE` and stdin: the Submit form would take a
  file name in place of pasted hex, as Co-sign does.
- `poo grind jobs ID` (merged) has no use in the GUI yet. Work's Jobs
  table already shows every job in full, so a one-job view isn't needed
  now.
- Numbering: Source Green and GUI Green both have a GREEN-038 and a
  GREEN-039. Prefixing new ones, such as GUI-GREEN-041, would stop the
  clashes. The coordinator or Grey can decide.

## Cost and risk

About 15 lines of form definitions. The e2e visits each view once more,
which adds about a second. No logic is added to the GUI.

## Prototype

On `claude/gui-green`. `gui/e2e/qol.mjs` checks two things:

- Every `poo help` command is run by a view or listed in TERMINAL_ONLY.
- "Simulate the network" with `partition` previews `poo sim partition`
  and exits 0.

All eight e2e scripts pass.
