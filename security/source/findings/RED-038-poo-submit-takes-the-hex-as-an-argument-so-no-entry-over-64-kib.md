# RED-038: `poo submit entry` takes the entry as one command-line argument, so an entry over about 64 KiB cannot be submitted, which the GUI can

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | `claude/m10-watch` @ 184bc54 (Source Round 14 Part B) |
| Component | pkg/cli/submit.go (`submitCmd`) |
| Surface | local operator (CLI) |
| Fixed by | |

## Summary

`poo submit entry|layer ... HEX` (GREEN-038, D143) reads the entry as the
last argument. On Linux one argument may be at most 131072 bytes
(`MAX_ARG_STRLEN`), so a hex string longer than that is refused by `exec` with
"Argument list too long" before `poo` runs. The hex is twice the bytes: the
command cannot hand the node an entry of more than 65535 bytes. Entries
past that size are ordinary (the node takes entries too large for one chunk
in parts, D117), and the GUI sends the entry in a JSON body of any size, so
the command does not do "the same as the GUI" (jerry's headless-and-GUI
rule, D143's reason) for them. A tool that grinds or builds off the node and
hands the result back with `poo submit` fails for the largest entries, with
an error from the shell and not from `poo`.

## Reproduction

The in-process test (`TestSubmitEntryAndLayer`) calls `run`, which takes
arguments from a Go slice and has no limit; I called `submit entry` with 400000
hex characters that way and got `entry: decode: encoding: input too short`, as
expected for junk. The limit is the operating system's, checked with
`/bin/true` and an argument of n bytes: 131071 runs, 131072 gives `Argument
list too long`. Not run with `poo` on a real entry of that size.

Also checked and fine: bad hex (`0x` prefix, odd length, empty, 400000
characters), trailing bytes, uppercase and a leading space or trailing
newline (trimmed) all give a clear error or are accepted as hex; under
view-only (`-view-only`, `Stdio.ViewOnly`) `submit entry` and `submit layer` are
refused with "view-only: poo submit changes things", and `poo grind jobs ID`
works with the view token and gives "no job N" for an unknown or negative id.

## Impact

Low. No security effect. A parity gap for large entries, found by reading
the argument handling.

## Recommendation

Take the hex from a file or standard input as well: `poo submit entry -f FILE`,
and `-` for standard input (one hex string, trimmed). Keep the argument form for
short ones. Test: a hex of 300 KB through standard input.

## Resolution

Open.
