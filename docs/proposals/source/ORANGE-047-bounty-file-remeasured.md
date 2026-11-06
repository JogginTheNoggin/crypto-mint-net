# ORANGE-047: Round 37 measured; the bounty book no longer grows with its history, and keepjson's remaining cost

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `bf90ffd` |
| Area | `pkg/bounty` (ORANGE-046 P1 to P3, D198), `pkg/keepjson` and `pkg/wallet` (P4) |
| Effort | measurements only; three proposals |
| Decided by | Grey; White in Part D for Proposal 3 |

## Summary

D198 built ORANGE-046's four proposals:
- P1 moves finished offers to `bounties-done.jsonl`;
- P2 saves a lease on a timer, outside the inbox worker;
- P3 counts held offers per peer and checks their time once a second;
- P4 makes `keepjson` scan the top level instead of decoding into a map.

I measured them the same way as Round 36.

**1. A lease or a claim now costs the same at any history. Only the start
still grows.**

Each finished offer had 16 layers handed in and paid, as in ORANGE-046.

| finished offers | Round 36: lease, claim | Round 37: lease | claim | pay (moves the offer out) | start (`Open`) |
| --- | --- | --- | --- | --- | --- |
| 0 | 0.97 ms, 0.97 ms | 3 µs | 1.2–1.6 ms | 1.4–1.5 ms | 0 |
| 256 | 18 ms, 19 ms | 5 µs | 1.5–1.6 ms | 1.5–1.6 ms | 7 ms |
| 2,560 | 148 ms, 172 ms | 3 µs | 1.4–1.7 ms | 0.9–1.7 ms | 44–65 ms |
| 25,600 | 2.0 s, 1.9 s | 4 µs | 1.2–1.3 ms | 1.8–1.9 ms | 460–475 ms |

- `bounties.json` holds only what is still open or owed: 62 bytes once
  the 16 test offers were paid. The done file grows by about 4.5 KB per
  finished offer, so 114 MB at 25,600.
- A lease answers in microseconds. Its save follows within 200 ms, off
  the worker.
- An accepted claim and a payment are each one small synced write, about
  1.5 ms. That cost is the disk sync, not the history.
- The start reads the whole done file (`doneIDs`) to find any offer left
  in both files by a crash: 0.47 s at 25,600 finished offers. That is
  once per start, but it is the one cost still growing with history.

**2. `Receive` at 4,096 held offers is 15 times cheaper.**

| held offers | Round 36: a list of 64 | Round 37 |
| --- | --- | --- |
| 256 | 0.38 ms | 0.11 ms |
| 4,096 | 8.4 ms | 0.54 ms |

What is left at 4,096 is `dropHeld(0)`: with the book full, each new
offer drops the oldest by deleting the first element of `heldOrd`, which
moves the whole slice. That is 64 × 4,096 ids moved per list. At 32
peers × 120 lists a minute, it is about 3.5% of the worker, against 54%
before. Nothing more needed.

**3. `keepjson` still doubles a large wallet's read time, and writing is
1.3 to 5 times slower.**

| wallet (keys, sent payments) | file | read, plain → with keepjson | write, plain → with keepjson |
| --- | --- | --- | --- |
| 100, 100 | 0.3 MB | 1 → 2 ms | 1 → 2 ms |
| 10,000, 10,000 | 26.7 MB | 101 → 207 ms | 170 → 332 ms |
| 10,000, 10,000, 10 unknown keys | 26.7 MB | 109 → 211 ms | 129 → 419 ms |
| 100,000, 100,000 | 267 MB | 1.17 → 2.02 s | 1.11 → 5.45 s |

These are close to Round 36's figures. P4 made `Split` itself cheap, but
`Split` was never most of the extra cost. Here is the read of the
26.7 MB wallet, part by part:

| step | time |
| --- | --- |
| `json.Unmarshal` into the fields (what a plain read costs) | 106 ms |
| `encoding/json` checking the whole input before it calls `File.UnmarshalJSON` | about 23 ms |
| the same check again in `UnmarshalJSON`'s own `json.Unmarshal` | about 23 ms |
| `keepjson.Split`: `json.Valid` | 23 ms |
| `keepjson.Split`: the top-level scan | 31 ms |

So the bytes are checked three times and scanned once more. Writing has
the same shape:
- `File.MarshalJSON` builds the compact form (87 ms).
- `MarshalIndent` then checks and copies that output again before
  indenting it (173 ms in all, against 130 ms for the plain fields).
- With kept keys, `Join` scans the output once more.
- At 267 MB the extra copies also cost GC time, hence 5×.

A wallet with 10,000 payments still spends about 0.1 s more on each
read, and 0.15 to 0.3 s more on each write.

## Proposals

**Proposal 1: read only the end of the done file at start.** An offer can
be in both files only if a crash came between the append and the save
that follows it. Only the last append's offers can be duplicates, and
those are at the end of the done file. `doneIDs` could read the last
256 KB or so, or check only the ids that `bounties.json` holds against
the file read backwards, and stop once all are found or ruled out. The
start would then cost the same at any history. The done file stays as it
is.

**Proposal 2: skip the checks the caller already made.** In `keepjson`,
`eachTop` runs `json.Valid` first. Every caller of `Split` has just
decoded the same bytes with `json.Unmarshal`, and `Join` gets bytes
`json.Marshal` wrote. A `splitValid` or `joinValid` used by those callers
saves one of the three checks: about 23 ms at 26.7 MB, the same bytes in
and out. `Split` stays as it is for any other caller.

**Proposal 3: let the wallet read and write its file without the JSON
methods.** `Wallet.Open` would decode into the plain fields and call
`keepjson.Split` itself. `Wallet.Save` would `MarshalIndent` the plain
fields and add the kept keys. That removes `encoding/json`'s second check
on reading, and its check-copy-indent of the compact form on writing.
The read would cost about plain + 55 ms, and the write about plain. The
bytes written stay the same when no key is kept. With kept keys, their
values would need indenting to match today's bytes (`json.Indent` on
each kept value, which is small). White should confirm the file is
unchanged. `File.UnmarshalJSON` and `MarshalJSON` stay for any other
caller.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harnesses were internal test files
outside the commit.

- **The book:**
  - A done file of N finished offers, written and synced as `archive`
    writes it, each with 16 paid claims.
  - Then `Open`, 16 posted offers of one layer, and 16 each of `TakeFrom`,
    an accepted `Claim`, and `Paid`. `Paid` moves each offer to the done
    file.
  - Then `Open` again.
  - Each figure is a mean of 16, from two runs.
  - A first run without syncing the history file first gave a slow first
    sync, of up to 32 ms. That is the history's own dirty pages, not the
    book.
- **`Receive`:** a book with `take` on, filled to `held_offers` from
  peers of 64 offers each, then given 200 lists of 64 new offers from
  200 new peers.
- **`keepjson`:** the wallet harness of ORANGE-046:
  - a `File` with K keys and S sent payments of two legs;
  - optionally 10 unknown top-level keys;
  - `json.Unmarshal` and `json.MarshalIndent` on `File` and on a copy of
    its fields without the methods.

  The parts were timed one by one, 20 times each, on the 26.7 MB file.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code.
- Proposal 1 changes how a node reads its own market files at start.
- Proposals 2 and 3 change how files are read and written, with the same
  bytes.
- No verdict changes and no depth limit is added.
