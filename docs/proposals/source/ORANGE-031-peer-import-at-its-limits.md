# ORANGE-031: Round 21 measured; peer imports at their limits

| Field | Value |
| --- | --- |
| Status | adopted (D159) |
| Based on | `main` @ `8662766` |
| Area | `pkg/tracking` (`get`, `trimLocked`), `pkg/peers` (range counting) |
| Effort | two small changes and two tests |
| Decided by | Grey; Red for the tracker's bound |

## Summary

I measured D157's peer book features at the sizes the code allows:
- export and import of a full book (4,096 addresses, `peers.DefaultLimits`);
- a file with the most blocks (65,536, `peerport.MaxBlocks`);
- the filters;
- dialling imported addresses (`PromoteImported` and the manager).

Most of it is fast. Two parts were not, and both are fixed here.

| 4,096 addresses | `8662766` | This change |
| --- | --- | --- |
| Write the file (1.2 MiB) | 18 ms | the same |
| Read it | 13 ms | the same |
| Plan | 8 ms | the same |
| **Apply into an empty book** | **1.90 s, 237 MiB** | **0.30 s, 23 MiB** |
| Import again (nothing new) | 11 ms | the same |
| Export a full book | 4 ms | the same |

| 4,096 addresses and 65,536 blocks (`-blocks`) | `8662766` | This change |
| --- | --- | --- |
| Read the file (6.2 MiB) | 100 ms | the same |
| Plan | 85–148 ms | the same |
| **Apply** | **53–54 s, 424 MiB** | **0.9–1.1 s, 210–220 MiB** |
| **Then 1,000 new addresses seen by the tracker** | **2.0 s** | **11 ms** |

**1. The tracker rescanned everything for each new address once
blocks filled it.** The tracker keeps at most `MaxAddresses` (8,192)
records. It drops the oldest ones that hold no flag, block or time-out.
When the records it must keep fill the bound, a trim cannot get under
it, and every new address ran the trim again: a scan and sort of every
record. Taking in 65,536 blocks did this 57,000 times, which took 53 s.
- Each `BlockFor` holds the tracker's lock, and every message the node
  handles also takes that lock. On a running node
  (`POST /v1/peers/import`), that is almost a minute of contention.
- After the import, every new address the node sees costs a full scan
  (about 2 ms) for as long as the blocks last. Blocks last an hour to
  forever.
- Only an import reaches this today. A node does not normally collect
  8,192 blocked addresses on its own.

The fix: when a trim cannot get under the bound, it records the count
it reached. The next trim waits until 1/16 of the bound more records
have arrived (`trimAt`), the same slack a trim already leaves. Records
that can be dropped then sit above the bound by at most that much
before they are trimmed: 512 records at the default. Nothing that must
stay is ever dropped, and with no blocks the tracker behaves as before.

**2. The peer book parsed every address for each address added.**
`Book.add` checks the per-range limit (`PerRange`) by counting the
book's addresses in the new address's range. It called `peers.Range` on
every record, which parses the IP and formats a string. Filling a book
of 4,096 that way is 16.7 million parses, which took 1.9 s, 85% of it
in `Range`. Verifying a peer counts its range the same way.

The fix: the book caches `Range` by address. `Range` depends on the
address alone, so an entry can never be wrong. The cache is dropped and
rebuilt when old addresses make up most of it. The count is still a
scan of the book, now of map lookups, so 0.3 s remains for a full
import. Keeping a count per range would remove that too, but it would
need updating everywhere a record is added or removed. I did not think
it was worth the risk for an import.

**No change needed:**
- **The filters** (`poo peers -category -mint-policy -label -addr`)
  filter `Book.Records()`: 3 ms for 4,096 records.
- **Dialling imported addresses:**
  - `PromoteImported` scans and sorts the book: 1.3–1.9 ms at 4,096
    records. It runs once per short mint policy per manage pass, every
    30 s.
  - The manager then dials at most the slots it is short, each in its
    own goroutine with the dial timeout, as for any candidate.
  - `poo peer connect -imported` promotes them all (1.5–2.1 ms), but
    the manager still dials only what it has room for.
  - A file of dead addresses costs a few failed dials per manage pass,
    each with its back-off (`Retry`).
- **Files:** a file of 65,536 addresses is 19.5 MiB, over the 16 MiB
  `MaxSize`, so it is refused before it is parsed. The book's 4,096 is
  the real bound.

No consensus change. Both fixes are node-local bookkeeping. Everything
blocked stays blocked, and every limit refuses what it refused before.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- A test-only harness in `pkg/peerport` built a peer file. The
  addresses were IPv4s, 16 to a /16, so `PerRange` refuses none, and
  each had one mint policy and a label. The blocks were addresses
  blocked for an hour.
- It wrote and read the file, planned the import and applied it into an
  empty book and tracker kept on disk, with `-blocks` when there were
  blocks. Then it planned again, read the records, promoted and picked,
  exported, and had the tracker hear 1,000 new addresses.
- Each step was timed, with the bytes allocated from
  `runtime.MemStats`. It ran on `8662766` and this change, with a CPU
  profile of the apply.

The harness is not committed. The tests below are.

## Idea

- **`tracking.Tracker.trimAt`**, set by `trimLocked` when what must
  stay fills the bound. `get` trims at `max(MaxAddresses, trimAt)`
  records.
- **`peers.Book.rng`**, a cache of `Range` by address used by the range
  counts (`rangeLocked`).
- **Tests:**
  - `TestTrimWhenBlocksFillTheBound` (`pkg/tracking`): 1,024 blocks
    with a bound of 256 are all kept, with the next trim deferred. Once
    they end, new addresses bring the records back under the bound.
  - `TestRangeCacheCounts` (`pkg/peers`): a book filled to its bound
    refuses by `ErrFull` and `ErrRangeFull` as before, through deletes.
    Every cached range equals `Range`.

## Cost and risk

- About 30 lines and two tests.
- The tracker can hold up to `MaxAddresses/16` droppable records over
  its bound between trims, but only after a trim found the bound full of
  records it must keep. Red may want to look at that.
- The full suite passes with `go test -race ./...`.

## Prototype

On `claude/source-orange-r21`; see the commit "ORANGE-031".
