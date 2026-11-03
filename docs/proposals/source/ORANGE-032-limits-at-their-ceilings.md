# ORANGE-032: Round 22 measured; the settable limits at their ceilings

| Field | Value |
| --- | --- |
| Status | adopted (D161) |
| Based on | `main` @ `b997eb0` |
| Area | `pkg/peers` (`Book` counts), `pkg/tracking` (`logLocked`) |
| Effort | two changes and two tests |
| Decided by | Grey (for the record) |

## Summary

D159 lets the owner set the peer book's and tracker's limits between a
floor and a ceiling. I measured both at their defaults and at their
ceilings: `p2p.max_book` 65,536 and `tracking.max_addresses` 131,072,
with `tracking.log_size` 4,096. The floors cost less than the defaults.

Two costs grew with the ceilings in a way that does not scale. Both are
fixed here.

**1. Every address added scanned the whole book.**
- `Book.add` counted the new address's network range by scanning every
  record. An import also counted the imported records by scanning (D158).
- When the book or range was full, it looked for the oldest imported
  record to give up by scanning every record again.

| Peer book | `b997eb0` | This change |
| --- | --- | --- |
| **Ceiling, 65,536:** fill with 49,152 learned addresses | **81.5 s** | **0.12 s** |
| Then import 16,384 (the imported places) | **93.9 s** | **0.63 s** |
| Then 1,000 learned addresses into the full book | **9.1 s** (9 ms each) | **0.65 s** |
| **Default, 4,096:** fill with 3,072 learned | 186 ms | 6.5 ms |
| Import 1,024 | 255 ms | 61 ms |
| 1,000 learned into the full book | 270 ms | 21 ms |

Each add held the book's lock. Peers, discovery and the manager all
take that lock, and at the ceiling each add took 1.7 to 9 ms.

The fix: the book keeps a count per network range, and the set of
imported records, up to date as records are added, changed and
deleted.
- Range checks are now one map lookup.
- The imported count is the size of that set.
- Finding a record to give up scans only the imported records, at most
  a quarter of the book.

The places that change a record's state all update the set: add,
`Verify`, `Archive`, `PromoteImported`, `Delete`, eviction,
`ForgetImported` and loading the book. `TestBookCountsKept` runs 5,000
random changes and a save and load, and checks the kept counts against
a scan of the book.

**2. The tracker copied a peer's whole event log for each event.** Once
a peer's log is full, each new event cut the oldest one by copying the
whole log. This only applies to peers on the `detailed` template, which
logs events.

| One peer's log, per event | `b997eb0` | This change |
| --- | --- | --- |
| `log_size` 16 | 0.84 µs | 0.31 µs |
| `log_size` 256 (default) | 5.7 µs | 0.27 µs |
| `log_size` 4,096 (ceiling) | **655 µs** | **0.44 µs** |

At the ceiling, a peer on `detailed` sending a thousand items a second
kept the tracker's lock for about two thirds of every second. Every
message from every peer takes that lock. The fix keeps the cut log as a
slice of the same array. `append` moves the live events to a new array
when the old one fills, so the array stays within about twice
`log_size`. `TestLogKeepsLastEvents` checks the order, the count, the
time cut and the array's size.

**What the ceilings cost after this change.** These are for the owner
and the docs, not proposals:

| | Default | Ceiling |
| --- | --- | --- |
| Peer book file | 1.7 MiB (4,096) | 27.6 MiB (65,536) |
| Book save (each manage pass that changed it, every 30 s) | 40–44 ms | 0.7–0.9 s |
| Book load at start | 16–18 ms | 0.23 s |
| `Pick` / `PromoteImported` per short policy | 2 ms / 0.4 ms | 33 ms / 12 ms |
| Tracker memory, addresses with one policy | 26 MiB (8,192) | 418 MiB (131,072), 3.3 KB each |
| Tracker file | 11 MiB | 171 MiB |
| Tracker save (at shutdown, and an import with `-blocks`) | 0.44 s | 6.9 s |
| Tracker load at start | 0.2 s | 4.7 s |

- **The book's save at the ceiling** writes and syncs 28 MiB about
  every 30 s. That is about 2.5% of a core and 1 MiB/s of disk.
  Writing the file without indentation (`json.Marshal` instead of
  `MarshalIndent`) or saving less often would cut it. I have not built
  either, because the default costs 40 ms.
- **The tracker's save encodes while holding the tracker's lock.** At
  the ceiling, an import with `-blocks` on a running node holds every
  peer's messages for about 7 s. It would need a copy of each record to
  encode outside the lock. I have not built this, because only an
  import or shutdown reaches it.
- **The tracker's memory at the ceiling is about 420 MiB.** The docs
  for `tracking.max_addresses` should say each address costs about
  3 KB.

The defaults are sensible: a 4,096 book and an 8,192 tracker cost
milliseconds and tens of MiB. I propose no change to any default,
floor or ceiling.

No consensus change. Both fixes are bookkeeping inside one node, and
every limit refuses what it refused before.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **Book and import:** a test-only harness in `pkg/peerport`.
  - It opened a book on disk with `MaxRecords` n and `MaxUnlisted`
    n/4.
  - It added 3n/4 learned IPv4 addresses, 16 to a /16.
  - It imported n/4 more from a peer file (`Import`, apply), then added
    1,000 learned addresses into the full book.
  - Then it timed `Records`, `Save`, `Open`, `Pick`,
    `PromoteImported` and `Unlisted`.
  - n was 4,096 and 65,536, on `b997eb0` and this change, with a CPU
    profile at 16,384. `inRangeLocked` was 70% of it, and
    `Record.imported` 14%.
- **Tracker:** the same harness had a tracker with `MaxAddresses`
  8,192 or 131,072 hear that many addresses, then 10,000 more (which
  trim). Then it saved, loaded, and read each address's status. Heap
  growth comes from `runtime.MemStats`.
- **Log:** a test-only loop of 200,000 `Item` events from one address
  on the `detailed` template, with `LogSize` 16, 256 and 4,096, on both
  commits.

The harnesses are not committed. The tests below are.

## Idea

- **`peers.Book`:**
  - `perRange` and `imp` are kept by `putLocked`, `dropLocked` and
    `trackLocked`.
  - `inRangeLocked` and `unlistedLocked` read them, and
    `evictImportedLocked` scans `imp`.
- **`tracking.Tracker.logLocked`:** re-slices instead of copying.
- **Tests:**
  - `TestBookCountsKept` (`pkg/peers`)
  - `TestLogKeepsLastEvents` (`pkg/tracking`)

## Cost and risk

- About 60 lines and two tests.
- The book's counts must stay right through every change to a record.
  Today the places that change a record are few, and the test checks
  every kind of change. A new one that changes a record's state must
  call `trackLocked`, as the comment on `Book.imp` says.
- The full suite passes with `go test -race ./...`.

## Prototype

On `claude/source-orange-r22`; see the commit "ORANGE-032".
