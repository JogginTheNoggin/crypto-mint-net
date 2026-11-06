# RED-102: `poon ledger import` and its dry run keep every unrelated field of a file in memory (about six times its size), and a repeated or early `entries` key brings back reading the whole file

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 34 Part B, on `main` @ `f1ac406` (D189, ORANGE-043) |
| Area | `pkg/cli/ledgerport.go` (`readLedgerFile`), used by `importLedger` and `ledgerDryRun` (and so by the GUI, which runs the same code) |
| Fixed by | `claude/project-thread-ggo8dt` @ 1af56e9 (BLUE-035) |
| For Black | Not a rule. |

## What is wrong

`readLedgerFile` streams the items of `entries`, but keeps every other
top-level field in `fields := map[string]json.RawMessage{}` and, to read the
header, marshals that map and unmarshals the result: a value of *n* bytes
costs several times *n*. The ledger file format has ten known keys; an unknown
key is accepted and kept, not refused. And where `entries` comes before the
fields that name the file (`kind`, `format`, `series`, `pool`), or appears twice,
the code reads that list whole into `held`, "as before". A file from someone
else can use any of these, so the streaming that ORANGE-043 measured for the
file `poon ledger export` writes does not hold for a file made to be large.

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes):
`importLedger` on a streamed file with a header, one extra field `junk` and an
empty `entries`, and a stub node:

```
a 64 MiB value in an unrelated field:  err=<nil>, peak heap 383 MiB over baseline
a 256 MiB value in an unrelated field: err=<nil>, peak heap 1472 MiB over baseline
```

The held-entries path is read from the code, not run.

## Suggested fix (Blue's call)

- Refuse any key of the header that format 1 does not define (a file that
  holds more is not a format 1 file), and read each known header value with a
  small limit.
- Treat a second `entries` key, and `entries` before the naming fields, as
  an error ("not a ledger file written by poon ledger export"), or skip them
  without holding them.
- Give the reader an overall size limit that an owner can raise.

## Not tested

A file with `entries` first or twice (read from the code).

## Resolution

Fixed in BLUE-035 (D191). Only format 1's header keys are taken, each
under 4 KiB and each once, and `entries` once. Entries before the naming
fields are spooled to a temporary file one at a time, so a file with
sorted keys still imports and is never held whole. Test:
`TestBlue035LedgerHeaderBounded`.
