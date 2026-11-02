# GREEN-004: Machine-readable help, so the GUI never parses usage text

| Field | Value |
| --- | --- |
| Status | accepted, later |
| Based on | claude/gui-5-release @ f93508e |
| Area | pkg/cli (a new flag on help), then gui forms and completion |
| Effort | medium |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

Add `poo help -json` (and `poo CMD -h -json`) that prints every command,
subcommand and flag with its type, default and usage, from the same flag
sets `cli.Main` parses. The GUI could then complete flags (GREEN-002) and
even generate a basic form for any command the views do not cover, with
no hand-kept list that can drift from the headless tool.

## Problem

The views hand-build their forms, so a new flag in `pkg/cli` is invisible
in the GUI until someone adds a field. Completion has to parse the human
help layout. Both are small ways the GUI can fall behind the source.

## Idea

- In `pkg/cli`, each command's `flag.FlagSet` already knows its flags.
  `help -json` walks them and prints `{command, summary, subcommands,
  flags: [{name, type, default, usage}]}`.
- The headless tool gets it too, which helps shell completion scripts
  (bash, zsh, fish) as a side benefit.
- The GUI uses it for flag completion and for a generic "Any command"
  form: pick a command, fill its flags, see the command line, run it.

## Cost and risk

Medium: it touches `pkg/cli`, which Grey owns, and needs a test that every
registered command appears. It is a pure read of existing flag sets, so no
behaviour changes. Not prototyped, because it is a change to the headless
code and is Grey's call first.

## Prototype

None.

## Decision

Accepted. It changes pkg/cli, which the headless tool shares, so it waits until the Source/Grey watch and filter commands land in pkg/cli to avoid conflicting edits. It then replaces the help-text parsing of GREEN-002.
