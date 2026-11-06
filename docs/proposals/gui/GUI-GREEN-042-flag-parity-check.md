# GUI-GREEN-042: A check that every flag poon lists is sent by a form, or left out for a reason

| Field | Value |
| --- | --- |
| Status | adopted (Round 17 Part A) |
| Based on | main @ 6a1cdce |
| Area | gui/e2e/parity.mjs (new), .github/workflows/ci.yml, gui/web/static/watch.js (Explore) |
| Effort | small |
| Decided by | (Grey) |

## Summary

GREEN-040 fails when a `poon` command is missing from the GUI.
GUI-GREEN-041 closed the option gaps I found by hand. This adds the
automatic check for options. `gui/e2e/parity.mjs` opens every view and
fills every field of every form, trying each action of an action select.
It reads the command line each form previews and compares the flags with
the ones `poon CMD -h` lists. A flag no form can send fails the check,
unless it is in `EVERYWHERE` or `LEFT_OUT` with a one-line reason. It
never presses a button, so nothing runs.

It covers 57 commands. On the first run it found three real gaps, which
this proposal fixes in Explore:

- **An address** had no `-below` (history paging) and no `-tid`.
- **Pools** offered only series, pool and serial of the shared filter. It
  had no `-address`, `-address-policy` or `-tid`, although poon pools
  takes them.

## Idea

- **How it reads the forms.** The filling happens in the page, three
  passes per form, so fields that `only:` shows once another is filled
  get filled too. Other selects take their last choice. The Wallet view's
  file and series boxes are filled as well. The `poon -h` text is read
  from standard error, where poon prints it.
- **Families.** `peer`, `policy` and `sim` subcommands share one flag set,
  and each action shows only the flags poon takes for it (GREEN-028). For
  these, the family's forms together must cover the family's flags.
- **`EVERYWHERE`** lists flags set in one place or unused by the GUI:
  - `-root` and `-policy`: Settings sets them for every view.
  - `-wallet`: the Wallet view sets it once, above its forms.
  - `-dir`: the GUI names a policy by `-root` and `-policy` instead.
  - `-o` and `-wide`: the GUI reads JSON.
  - The passphrase and seed file flags: the GUI uses masked prompts
    instead (D98, D106).
- **`LEFT_OUT`** lists flags left to the terminal, each with its reason.
  Grey can move any of them into a form; the check accepts either.
  - `node -log-level` and `-log-format`.
  - `sim`'s tuning flags and the rest of `series new`'s flags: each form
    points to the terminal for these.
  - `watch -once`: the Watch view reconnects by itself.
  - `grind start` and `run -lease`: Start leaves the job to the node, and
    Run stops with the page.
  - `grind schedule add` and `set -if-version`: they are applied under the
    node's lock and keep changes made meanwhile (D125).
  - `tx`'s filter flags: In mint serial is the narrowing that helps for a
    single transaction.
  - `-kind` on the view commands: see below.
- **CI** runs it after `streams.mjs`.

## Noted for Source

`poon pools`, `pending`, `unspent`, `ledger`, `tx` and `address -h` all
list `-kind`. But poon refuses it on every one of them ("-kind selects
events and applies to poon watch only"). The help offers a flag the
command never accepts. Registering `-kind` only for `watch` would make
the help match, and the entry in `LEFT_OUT` could then go. This is CLI
help text only, not consensus.

## Cost and risk

- About 130 lines of test, one line of CI, and 5 lines in Explore. It
  takes about 10 seconds, starting no node.
- Changing a fill value could trip `commandOf`, which skips the `x` and
  `1` the check types in front of flags. That only affects how the check
  reads its own input.

## Prototype

On `claude/gui-green`. I made sure it can fail: with Build's
`grindFlags` removed, it reports `poon build -device`, `-for` and
`-threads`. With the Explore fixes in place, all nine e2e scripts pass,
including this one.
