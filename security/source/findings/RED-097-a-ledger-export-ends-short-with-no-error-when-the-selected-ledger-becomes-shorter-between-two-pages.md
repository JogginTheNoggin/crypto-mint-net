# RED-097: `poo ledger export` ends with no error, and a header that promises more, when the selected ledger becomes shorter between two pages

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 33 Part B, on `main` @ `941049d` (D188) |
| Area | `pkg/cli/ledgerport.go` (`writeLedgerFile`), `pkg/watch/export.go` (`Export`) |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ 8606137 (BLUE-034) |

## What is wrong

An export is made page by page. After the first page, `writeLedgerFile`
asks for the next from `page.Next`, and guards against a switch only when the
new page has entries: `len(page.Entries) > 0 && page.Entries[0].Parent !=
lastHash`. The selected ledger can switch to one that is **shorter and
heavier** (weight, not height, selects). Then `Export` is asked for a height
above the new tip, takes the early return (`from > end`), and gives an empty
page with `Next == 0` and no error. The loop sees `Next == 0` and ends:
the file is closed as valid JSON, `poo ledger export` prints
`wrote 2 entries to …` and exits 0, while the header, written from the
first page, says `to: 5, height: 5`.

The export is how an owner backs up or moves a ledger, so a quietly short
file is worse than a failed one. The entries it does hold are a prefix of a
ledger that is no longer selected.

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes): a test
server answers the first request with heights 1 and 2 and `Next: 3`
(`Height: 5`), and the second with an empty page (`Height: 2`):

```
n=2 err=<nil> json=<nil> header from=1 to=5 height=5 entries=2
```

I did not make a real node switch to a shorter ledger between two requests;
the page behaviour is the code's own and the empty-page case is read from
`Export`.

## Suggested fix (Blue's call)

- At the end, check that the file holds what its header says: the last
  entry's height equals `to` (or the tip it was told), and that its hash is
  the tip of the last page; otherwise fail with the same "run it again"
  message as for a switch.
- Treat an empty page after a page with `Next != 0` as a switch, not as the
  end. Also compare each page's `Height` and `Tip` with the first's.

## Not tested

A real reorganisation during an export; exporting the other way (a
longer ledger replaces the one being read, which the parent check does
catch).

## Resolution

Fixed in BLUE-034 (D189). Every page after the first is asked only up to
the header's `to`. Each must start at the next height and carry on from
the last entry, so an empty page after one that promised more is a
switch. The file must end at `to`, and at the header's tip when it runs
to the tip. Otherwise the export fails with "run it again", and `-out`
writes nothing. Test: `TestBlue034ExportEndsWhereItsHeaderSays`, which
includes Red's empty-page case.
