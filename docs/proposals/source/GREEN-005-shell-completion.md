# GREEN-005: Shell completion (`poo completion bash|zsh|fish`)

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/cli` (new command) |
| Effort | small |
| Decided by | |

## Summary

Print a completion script for bash, zsh or fish that completes command
names, subcommands (`policy`, `peer`, `wallet`, `sim`, `series`) and each
command's flags. It builds on GREEN-001's command list, so it cannot drift
from the usage. No prototype yet.

## Problem

There are over 30 commands, many with subcommands and flags that are easy to
mistype. Every comparable node tool ships completion.

## Idea

- `poo completion bash` prints a script that calls back into
  `poo __complete WORDS...`, a hidden command that answers from the usage
  text and each command's `flag.FlagSet`. The scripts stay tiny, and the
  answers come from the code, so they are never stale.
- Dynamic answers where cheap and local: policy names from `poo policy
  list`, peer addresses from the running node's book.
- Install notes go in `docs/running-a-node.md`.

## Cost and risk

About 200 lines. To list flags without running a command, each command's
flag set has to be built by its own function, which is a small refactor of
`pkg/cli`. That also helps GREEN-001 print flag help.

## Prototype

None yet; it depends on GREEN-001.
