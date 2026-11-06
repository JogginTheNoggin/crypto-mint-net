# RED-118: The done file's crash recovery depends on an offset and a line start, so a trimmed file or a torn end brings a paid offer back with its unpaid claim

| Field | Value |
| --- | --- |
| Severity | Low (it needs a crash in one window plus a change to the done file, or a torn write) |
| Status | fixed |
| Found | Source Round 38 Part B, on `main` @ `9c4dc92` (D200, W-31, ORANGE-047 P1) |
| Area | `pkg/bounty/book.go` (`doneIDs`, `archive`, `Open`, `file.DoneSize`) |
| Fixed by | `claude/project-thread-ggo8dt` @ 8ee7aa6 (BLUE-039) |
| For Black | Not a rule. |

## What is wrong

The done file is how a start finds an offer that was paid and archived, but
whose `bounties.json` (with the claim still unpaid) came back after a crash
between the two writes. D200 reads only the part of the done file past
`done_size` and takes each line's id from its first 72 bytes. The offer it
fails to find is loaded as live, with its unpaid claim, and `poon bounty pay`
will pay it again. Two ways it fails with no hostile act:

1. **The offset is trusted.** `from` must land on a line start. If the done
   file changes before it (the first record trimmed or rotated, an editor, a
   restore), `from` points into a line, or exactly at the end, and the
   record that holds the offer is skipped or cut. Nothing is reported: the
   offer is simply live. (A done file that is shorter than `from` is read in
   full, which is right.)
2. **`archive` appends to a torn file.** It opens the file with O_APPEND and
   writes the next lines without looking at whether the last line ended in a
   newline. After a crash in the middle of an append, the next record is glued to the cut
   line, the reader sees one line, and the second offer's id is never read, so the same
   crash window is open for it.

## How I checked it

`pkg/bounty` tests (throwaway, removed; copy kept with Red's notes). Offer A
is posted, leased, claimed, paid (done file: header, A). Offer X is claimed;
`bounties.json` is copied (X unpaid, `done_size` after A); X is paid (archived);
the old `bounties.json` is put back (the crash).

```
no change to the done file:  X left out; "offer 0: 2a70ac32 is in bounties-done.jsonl already, finished"
the first record trimmed:     X live again, 1 unpaid claim; dropped: []
   (header 40 bytes, A 819, X 819, so done_size lands exactly at the end of the trimmed file and nothing is read)
```

For (2): a done file whose last line was cut 40 bytes short, then one more offer
finished: the file has 2 lines, the reader reads 1 id, and the second
offer's id is not among them.

## Suggested fix (Blue's call)

- Do not trust `done_size` alone: check that the byte before `from` is a
  newline (else read all), or keep the id of the last line archived beside
  the offset and read all when it is not found there.
- Say, as with the other drops, when an offer in `bounties.json` has an
  id in no done line but its `done_size` is not a line start.
- In `archive`, if the file is not empty and does not end in a newline, write
  one first (and report the cut line).
- Keep the claim `Paid` record where a restart finds it even if the done file
  is lost (see RED-119).

## Not tested

Trimming that leaves `done_size` mid-line (the case above is the exact-end one,
which the equal record sizes made); negative or huge `done_size` values (from
the code, they read all).

## Resolution

Fixed in BLUE-039 (D201).

- `bounties.json` records, beside `done_size`, the id on the line that ends there (`done_last`). A start reads from `done_size` only when the byte before it is a newline and the line it ends begins with that id. Otherwise it reads the whole file and says so in the log ("changed since bounties.json was written"). That covers the trimmed first record (the exact-end case), an offset part way into a line, and a file shorter than the offset. A file from D200 with no `done_last` is read in full once.
- A start says when the done file's last line has no newline. `archive` writes a newline first when the file does not end in one, so the next record is a line of its own. A write that fails part way is truncated back, so no cut line names an offer still in `bounties.json`.
- A cut last line's id still counts as done: only an offer that needs nothing more is ever written there, and a payment is recorded only after the wallet sent it.
- The claim's payment where the done file is lost: `poon bounty pay` finds it in the wallet (RED-119).

Tests: `TestBlue039TrimmedDoneFile`, `TestBlue039TornDoneFile`.
