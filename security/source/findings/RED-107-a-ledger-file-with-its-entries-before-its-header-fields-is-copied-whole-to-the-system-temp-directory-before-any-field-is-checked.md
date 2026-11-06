# RED-107: A ledger file with `entries` before its naming fields is copied whole to the system temp directory before any field is checked

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 35 Part B, on `main` @ `866a4ac` (D192, W-20 / RED-102 follow-up) |
| Area | `pkg/cli/ledgerport.go` (`readLedgerFile`) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-036) |
| For Black | Not a rule: nothing about what the node accepts changes. |

## What is wrong

Since D191/D192 an `entries` list that comes before `kind`, `format`,
`series` and `pool` is spooled to `os.CreateTemp("", "poo-ledger-*.spool")`,
one item at a time, and replayed after the header. Four things follow:

- **No bound.** The spool takes as many bytes as the file has: nothing is
  checked first, not even that the file is a ledger file. A wrong file
  (`kind` something else, `format: 2`, a different series) is refused only
  after all of it is written. Where the temp directory is a RAM filesystem
  (tmpfs `/tmp` is the default on several distributions) the spool is memory,
  which is what the streaming read was built to avoid (ORANGE-043).
- **Bigger than the input.** Items are decoded and encoded again; here the
  spool was 1.13 times the file.
- **A kill leaves it.** The cleanup is a `defer`; a process killed by SIGKILL,
  a power cut, or a signal not handled leaves the file in the temp directory
  (the same shape as RED-089).
- It is written with mode 0600 and removed on every normal return and
  error return I tried: those are fine.

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes) with
`TMPDIR` set to an empty directory: a 27.8 MB file of 20,000 entries (1,023
byte items) followed by the four fields. At the moment `head` ran, the
directory held `poo-ledger-<n>.spool`, mode `-rw-------`, 31,288,890 bytes;
after the call it was empty. I did not run a kill, or a tmpfs.

## Suggested fix (Blue's call)

- Read the header keys without spooling when the file is seekable (`os.File`):
  scan for the header first, or seek back to `entries`. Spool only a pipe.
- Put a limit on the spool (for example refuse over a
  few GiB) and name the temp directory in the error.
- Put the spool next to the file being imported, or in the wallet/node data
  directory, not in the system temp, and remove stale `poo-ledger-*.spool`
  at the start of the next import.

## Not tested

tmpfs behaviour; a kill; the same through the GUI's import form.

## Resolution

Fixed in BLUE-036 (D194). There is no spool now. When entries come first,
the file is read twice from where it started (any seekable input), and a
pipe is refused with a message to give the path. Test:
`TestBlue036LedgerFileOrder`.
