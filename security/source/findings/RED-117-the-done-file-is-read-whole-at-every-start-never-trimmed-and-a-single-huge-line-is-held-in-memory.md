# RED-117: The done file is read whole at every start, never trimmed, and one huge line is held in memory

| Field | Value |
| --- | --- |
| Severity | Low (it needs write access to the node's data directory, or years of use) |
| Status | fixed |
| Found | Source Round 37 Part B, on `main` @ `bf90ffd` (D198, ORANGE-046 P1) |
| Area | `pkg/bounty/book.go` (`doneIDs`, `archive`, `Open`) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-038) |
| For Black | Not a rule. |

## What is wrong

`archive` appends each finished offer to `bounties-done.jsonl` and nothing ever
removes one. `Open` reads the whole file at each start (`doneIDs`, a
`json.Decoder` over it) to learn which ids to leave out of `bounties.json`. Two
results:

- The cost of a start grows with the number of offers the node ever finished:
  about 0.4 s for 200,000 lines (23 MiB) here.
- A line is decoded as one value, so a line of 128 MiB (a long string in any
  field) is held whole: peak heap 448 MiB, 1.3 s.

And from the code: an offer in `bounties.json` whose id is in the done file is
left out with no message (`continue // moved to the done file before a crash`),
unlike the other drops, which are reported. So a done file that holds a live
offer's id removes it and what it still owes.

## How I checked it

`pkg/bounty` test (throwaway, removed; copy kept with Red's notes): a valid empty
`bounties.json` and a done file, `Open` timed:

```
done file: 23 MiB, 200000 lines    Open: 380 ms
one 128 MiB line                   Open: 1.3 s, peak heap 448 MiB
```

The silent drop of a live offer is from the code, not a run.

## Suggested fix (Blue's call)

- Read the done file's lines with a bound (a line over a few KiB is not one of
  ours: stop and report), or keep only ids in a small index file beside it.
- Say, as for the other drops, which offers were left out for being in the
  done file, and only leave out one that has no unpaid claim.
- Trim or rotate the done file (by age or size) with a note in the docs.

## Not tested

A done file of many millions of lines; a crash between the append and the save.

## Resolution

Fixed in part in BLUE-038 (D199). Each done line is read only as far as its id, and the rest is skipped as it streams. Lines with no id are counted. An offer found in both files is reported, and is still left out, because the done copy records its payment and keeping the other would owe it again. Trimming the file, or reading only its end (ORANGE-047 P1), is left for Grey. Test: `TestBlue038DoneFileBounded`.
