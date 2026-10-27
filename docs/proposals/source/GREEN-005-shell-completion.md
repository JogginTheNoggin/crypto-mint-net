# GREEN-005: Shell completion (`poo completion bash|zsh|fish`)

| Field | Value |
| --- | --- |
| Status | deferred |
| Based on | `claude/m10-watch` @ `06f3970` (first proposed on `4692b4e`) |
| Area | `pkg/cli` (new command) |
| Effort | small |
| Decided by | Grey: worth having, but it needs the per-command flag set refactor first; revisit after GREEN-003 |

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

On `claude/source-green`, based on `claude/m10-watch` @ `06f3970`:
`pkg/cli/completion.go`, `pkg/cli/completion_test.go`, and the
`completion` and `__complete` cases in `pkg/cli/main.go`. Install notes are
in `docs/running-a-node.md`, and `docs/modules/poo.md` has a row for it.

**No big refactor.** Grey deferred this until each command's flag set was
built by its own function. The prototype needs only a small change
instead. Every `flag.NewFlagSet(name, flag.ContinueOnError)` in `pkg/cli`
becomes `newFlagSet(name, out)` (24 call sites, mechanical). When the
invocation carries a probe, `newFlagSet` hands it the flag set and
silences it. `poo __complete` runs the command with `-h` under the probe,
reads the flags in `Usage`, and returns. Every command parses its flags
before doing anything else, so the probe only reads.
`TestProbeOnlyReads` runs every command and subcommand that way and checks
that nothing is written to `$POO_HOME`, `$HOME` or the working directory.

**A fix that comes with it.** `newFlagSet` also sends flag errors and `-h`
output to the invocation's `Stdio.Err` instead of the process's
`os.Stderr`. The headless tool behaves the same, since `cmd/poo` passes
`os.Stderr`. In the GUI terminal, `poo work -nope` used to show only the
error line, while the flag list went to the GUI process's stderr. Now it
shows the same as a shell. `TestFlagErrorsGoToErr` checks this.

Candidates come from three places:
- commands and subcommands, from the usage texts (`walletUsage` and
  `simUsage` for those two)
- flags, from the probe
- policy names after `-policy`, from the policy manager (archived ones
  left out)

Other flag values fall back to the shell's own completion of files.
Peer addresses are not completed, since that would need a running node.

**Tested.** The bash script is tested by hand: `poo wallet pa<TAB>` gives
`passwd pay`, and `poo ledger -l<TAB>` gives `-limit`. The zsh and fish
scripts are tested only by the unit test, which checks they call
`poo __complete`. Neither shell was installed in Green's container.
