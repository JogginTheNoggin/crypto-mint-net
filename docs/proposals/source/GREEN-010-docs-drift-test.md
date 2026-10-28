# GREEN-010: A test that the docs name real commands and flags

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `c71f774` |
| Area | `pkg/cli` (test only) |
| Effort | small |
| Decided by | |

## Summary

A test reads every Markdown file in the repository except the proposals.
For each `poo …` command line, it checks that the command exists and that
every flag on the line is defined by that command or subcommand. A
renamed flag or a dropped command then fails CI instead of leaving the
docs wrong. Prototyped. It passes today: the docs have no drift, which a
sweep this round confirmed.

## Problem

The docs show many `poo` command lines: `running-a-node.md`, the module
pages, `AGENTS.md`, the README. Commands and flags change every round. D98
just replaced `-seed` with `POO_SEED` and `-seed-file`, and GREEN-009
adds `-serial` to `build`. Nothing catches a doc that still shows an old
flag. The GUI's help panes copy these examples too.

## Idea

`TestDocsNameRealCommands` in `pkg/cli`:
- It finds `poo WORDS` in inline code, in `$ ` and `./` prompts, and in
  code blocks.
- It checks the command against `commandNames()`, then picks the
  subcommand, if any, from `subcommands()`.
- It checks each `-flag` word against the flags that the GREEN-005 probe
  reads from that command's own flag set.
- It fails if it matches fewer than 50 lines, so a broken pattern cannot
  pass silently.

Proposals are skipped, because they describe future commands and show
typos on purpose. The test was checked against a planted bad line, and it
reported both the bad flag and the bad command.

## Cost and risk

About 80 lines in one test file, with no change to the tool. A false match
in prose is possible. The fix is to rephrase the prose, or to narrow the
pattern if that keeps happening.

## Prototype

On `claude/source-green`: `pkg/cli/docs_test.go`.
