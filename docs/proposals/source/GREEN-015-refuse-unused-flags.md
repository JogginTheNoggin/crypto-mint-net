# GREEN-015: Refuse flags that do nothing for the subcommand given

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `0ac8e4e` |
| Area | `pkg/cli` (`peer`, `policy`, `sim`) |
| Effort | small |
| Decided by | Grey, Source Round 5: as prototyped (`flagsFor`) |

## Summary

Three commands share one flag set across their subcommands and silently
ignore the flags that do not apply. `poon peer unblock -range ADDR` unblocks
only the address and leaves the range blocked. `poon sim load -share 0.4`
runs with no attacker. `poon policy list -fresh` does nothing. In each
case, the person believes the flag took effect. With this change, such a
flag is refused with a message naming the subcommands that use it.
Prototyped, with a test.

## Problem

A sweep with the completion probe compared each subcommand's flags with
what it reads. These flags are accepted but never read:

| Command | Ignored flags |
| --- | --- |
| `peer unblock`, `clear`, `template`, `delete`, `timeout` | `-range` (block only), `-seconds` (timeout and block), `-what` (clear), `-template` (template) |
| `policy list`, `use`, `archive`, `delete` | `-fresh` (unarchive only) |
| `sim load` | `-share`, `-threshold`, `-head-start`, `-give-up`, `-split`, `-sizes` |
| `sim partition` | `-duration`, `-pay-every`, the attack flags, `-sizes` |
| `sim attack` | `-duration`, `-pay-every`, `-split`, `-sizes` |
| `sim sizing` | `-split` |

`peer unblock -range` matters most, because unblocking is a security
action: the owner thinks a range is open again when it is not. The `sim`
cases produce results for an experiment that was never run.

## Idea

`flagsFor(fs, sub, owners)` runs after `Parse`. `owners` maps a flag to
the subcommands that read it. A flag that was set but is not for this
subcommand returns "-range does nothing for unblock; it is for block".
Flags not in the map apply everywhere, as before. The maps sit next to
each command's flags, so a new flag needs one line there.

## Cost and risk

About 30 lines. A command line that worked before still works, unless it
passed a flag that was being ignored. That is the point of the change, but
it can break a script that passed such a flag. No doc or script in the
repository does; I checked every `poon sim` and `poon peer` line.

## Prototype

On `claude/source-green`: `pkg/cli/flagscope.go` and
`flagscope_test.go` (six refusals, and two lines that still pass), wired
into `peerCmd`, `policyCmd` (`node.go`) and `simCmd` (`sim.go`).

The same sweep found one doc gap, fixed on the branch: `poon build
-max-tries` was missing from `docs/modules/poon.md`.
