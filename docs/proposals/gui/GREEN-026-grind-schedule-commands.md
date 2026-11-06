# GREEN-026: The grind schedule: edit it through commands, not the file

| Field | Value |
| --- | --- |
| Status | done: commands by Source (D113), Schedule panel and forms in Work (Round 7) |
| Based on | claude/gui-4-watch @ 8b044ac |
| Area | pkg/cli (new poon grind schedule), then gui/web/static/work.js |
| Effort | medium |
| Decided by | (Grey) |

## Summary

Grey asked whether the node.json grind schedule needs a commands-based
editor in the GUI. Yes, and it should come in two steps. First, the
headless tool gets commands that read and change the `grind` section, as
`follow` and `unfollow` do for the follow list. Then the Work view gets a
"Schedule" panel and two forms on those commands. The GUI must not write
node.json itself. It may not read or write node files (docs/gui.md), and
jerry's rule is that the GUI uses exactly the headless code.

## Problem

jerry's M11 request says work is assigned "by hand, on a schedule, or
automatically". By hand works in the CLI and the GUI. Scheduled and
automatic jobs come only from node.json's `grind.jobs`, and today the
only way to change them is to edit the file by hand and then
`poon reload`. In the GUI, the only sign of them is `source: policy` rows
in the jobs list.

## Idea

Commands (Source/Grey), named to match `follow`:

| Command | Does |
| --- | --- |
| `poon grind schedule [-o json\|table]` | The section: `device`, `threads`, and `jobs` numbered from 1 |
| `poon grind schedule add TARGET [-series ID] [-pool K] [-serial N] [-device D] [-threads N] [-hours 22-06] [-until-from W] [-no-reload]` | Appends a job, checks it as node.json's loader does (`MaxGrindJobs`, hours, threads), writes, and reloads a running node |
| `poon grind schedule remove N\|-all [-no-reload]` | Removes jobs |
| `poon grind schedule set [-device D] [-threads N] [-no-reload]` | The section's defaults |

Two mismatches to settle while doing it:
- JOB FLAGS take `-for`, but a node.json job has no `for`: policy jobs
  run until the node stops. `schedule add -for` should be refused, or
  the field added.
- JOB FLAGS take `-serial`, but a node.json job has only `pools`. `add`
  can resolve the serial to its pool as `run` does, and say which.

The GUI then shows (Grey, in a later round):
- A **Schedule** panel on `poon grind schedule -o json`: one row per job,
  with the same columns as GREEN-023, plus Remove (runs
  `poon grind schedule remove N`).
- An **Add to schedule** form on the Run a job fields, minus "For",
  plus "Only in hours". Its preview shows `poon grind schedule add …`.
- **Defaults** with the device choices from GREEN-024.

## Cost and risk

Medium, mostly in `pkg/cli` and `pkg/nodepolicy`: writing the policy
safely, as `follow` already does, plus the reload. The GUI part is small
once the commands print JSON.

## Prototype

None. The commands come first, and they are Source/Grey's.
