# RED-108: Header fields after `entries` are read and dropped, so `poon ledger import -dry-run` checks the range and tip against zero

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 35 Part B, on `main` @ `866a4ac` (D192) |
| Area | `pkg/cli/ledgerport.go` (`readLedgerFile`, `ledgerDryRun`) |
| Fixed by | `claude/project-thread-ggo8dt` @ f835882 (BLUE-036) |
| For Black | Not a rule: a real import does not use these fields. |

## What is wrong

`readLedgerFile` calls `head(h)` as soon as `entries` starts if `kind`,
`format`, `series` and `pool` have been read. Keys after the list (`from`,
`to`, `height`, `tip`, `weight`, `exported`) are then read into `h` but
`head` has already been given its copy, and `started` is true, so they are
never delivered. `ledgerDryRun` compares the entries with `f.From`, `f.To`,
`f.Height` and `f.Tip`, taken in `head`: all zero for such a file.

- A correct file with this key order whose entries start at height 100 gets
  a false problem ("entry 0 is at height 100, but the file says it starts
  at 0").
- A file that says `to: 999` after a single entry at height 0 gets none:
  the "last one is where the header says" check passes against zero.

A real import is not affected: it sends entries to the node, which verifies
every one, and uses only `series` and `pool` from the header. The harm is a
dry run that says less than it appears to say.

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes): the four
naming fields, then `entries` with one item, then `"from":5,"to":999,
"height":999,"tip":"abab…"`: `readLedgerFile` returned no error, `head` ran
once, with `from=0 to=0 height=0` and a zero tip.

## Suggested fix (Blue's call)

- Give `head` the whole header: call it after the last header key, spooling
  (see RED-107) or, better, refusing a file whose `entries` comes before
  any of the five fields the dry run uses; or
- keep two callbacks: `head` for what the import needs (kind, format, series,
  pool) and `trailer` for the rest, and have the dry run do its range and tip
  checks in `trailer`.

## Not tested

Whether any real tool writes keys in that order.

## Resolution

Fixed in BLUE-036 (D194). `readLedgerFile` returns the whole header, and
the dry run checks the first height, the last height and the tip against
it after the read. Test: `TestBlue036LedgerFileOrder`.
