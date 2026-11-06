# ORANGE-034: Round 24 measured; parts as ranges, part ids, peer counts

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `08404df` |
| Area | `pkg/cli` (`series info`, `series release`), `pkg/series` (`PartID`), `pkg/peers` and `pkg/rpc` (`peers -counts`) |
| Effort | three changes and three tests |
| Decided by | Grey |

## Summary

Round 24 adds variable supply phase 1 (D163) and the D164 tooling fixes.
I checked GUI Orange's ask that parts print as ranges, and measured part
ids and the new `peers -counts`.

**1. Parts as ranges: two outputs did not print parts at all. Fixed
here.**

GUI ORANGE-018 measured the GUI's parts table at 2.9 s for 100 parts
printed as lists of serials, and 30 ms as ranges. The GUI plan
(`docs/gui-variable-supply-plan.md`) draws Series info's parts table and
the release preview from the headless output.

| Output | `08404df` | This change |
| --- | --- | --- |
| `poon series parts` | ranges: first and last serial and pool, part id | the same, plus `mints` and `units` |
| `poon series release -check` | first and last serial, part id | the same, plus first and last pool |
| `poon series info` on a variable series | **part 1's pools only; no supply type, part size, cap or parts** | `supply_type`, `part_size`, `max_parts`, and parts 1 to 10 as rows of ranges |
| `poon series release -part K` (the file the issuer publishes) | **the release's four fields only** | those four fields first, then the part id, its serials and pools as ranges, `mints` and `units` |
| `poon series info` on a fixed series | unchanged | unchanged, byte for byte (no new keys) |

Every part is one short row, whatever its size. `TestSeriesPartsAsRanges`
uses parts of a million mints and checks that `series info` stays under
8 KB. It also checks that `series info`, `series parts`, the release and
`-check` agree on the same part.

A release file keeps its own fields (`series`, `part`, `previous`,
`witness`). `Release.UnmarshalJSON` ignores the other keys, so `-check`
and a phase 2 node read the same release as before. They work the range
out again from the series and never trust the printed one.

**2. Part ids kept one id per part, so a far part could use up memory.
Fixed here.**

`PartID(k)` takes k hashes the first time, by design (each part id
chains on the one before). It kept every id it worked out, which costs
32 bytes per part plus the slice's growth, and the growth also made it
4 times slower. Any series file reaches it through `poon series parts
-from K` with a large K. A phase 2 node holding an issuer's release of a
far part would reach it too.

| Parts worked out | `08404df` | This change |
| --- | --- | --- |
| 1,000,000 | 0.97 s, 30 MiB kept | 0.24 s, under 1 MiB |
| 10,000,000 | 11.3 s, 358 MiB kept | 2.7 s, under 1 MiB |
| 50,000,000 | **55 s, 1.7 GiB kept** | **13 s, 1 MiB kept** |
| 10⁹ (estimated) | about 34 GiB: more than this 16 GiB machine | about 4.5 min, 31 MiB |

The fix keeps a mark every 1,024 parts and the ids of the 1,024-part
stride last asked for. Any part already passed takes at most 1,023
hashes (under a millisecond), and listing parts in order works each id
out once. The ids are the same. `TestPartIDsAnyOrder` checks 5,000 ids
asked for in order, far first, at random and growing, against the chain
worked out from scratch with `Release.ID`. Before the fix to stride
growth, it caught a stride cached before the marks reached it.

**For Black, phase 2 (not a change here).** The time stays linear,
because the definition chains each part id on the one before. A node
holding the issuer's release of part 10⁹ spends about 4.5 minutes of one
core working its id out, and part 10¹² about 3 days. `CheckRelease`
checks the issuer's witness first, so only the issuer can ask this of a
node. I propose nothing that refuses such a release. A refusal would be
a new rule, and D115 forbids node policy that does so. Phase 2 could work
the ids out off the engine lock, or in the background with progress
shown.

**3. `peers -counts` still copied and sorted the whole book. Fixed
here.**

D164's counts-only reply took `Book.Records()`, which sorts and clones
every record under the book's lock, then counted the copy.
`Book.Counts()` counts in place under the lock, with no copy or sort.

| Book (2 policies a record) | `08404df`: Records + CountBook | This change: Counts |
| --- | --- | --- |
| 16,384 (D164's example) | 17.6 ms + 0.8 ms | 1.75 ms |
| 65,536 (ceiling) | 68 ms + 2.9 ms | 10.7 ms |

The GUI refreshes this often, and every peer message takes the book's
lock. `TestBookCountsInPlace` runs 4,000 random changes and checks that
`Book.Counts` gives what `rpc.CountBook` gives from a copy. The reply's
JSON is unchanged.

**Live peer import (D164)** only passes the caller's file digest through.
It costs what ORANGE-031 measured for an import. No change.

No consensus change. Part ids are the same values, kept differently. The
other two changes are local tool output and a local count.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4,
16 GiB.

How they were taken:
- **Outputs:** `poon series new -variable`, `info`, `release -part`,
  `release -check` and `parts`, on `08404df` and this change.
- **Part ids:** a test-only harness in `pkg/series` called `PartID(k)`
  on a fresh variable series (example params, M = 4) for k = 10⁶, 10⁷ and
  5 × 10⁷, then `runtime.GC` and `HeapAlloc`, on both commits. The ids
  matched (`c3982ad6`, `65fe281e`, `682ad28c`). The 10⁹ row multiplies
  the measured rates.
- **Counts:** a test-only harness in `pkg/peers` filled a book of 16,384
  or 65,536 learned IPv4 addresses with two sections each. It timed
  `Records`, `rpc.CountBook` on its copy, and `Counts`, averaged over 20.

The harnesses are not committed. The tests are.

## Idea

- **`pkg/cli`:**
  - `partRow`, `partRowOf` and `partRows` are shared by `series parts`,
    `series info` and the release.
  - `releaseOut` prints the release's own JSON fields first.
  - `releaseInfo` gains `first_pool` and `last_pool`.
- **`series.partChain`:** `marks` every `partStride` (1,024) parts, plus
  one stride of ids.
- **`peers.Book.Counts`:** used by `GET /v1/peers?counts=1`.
- **Tests:**
  - `TestSeriesPartsAsRanges` (`pkg/cli`)
  - `TestPartIDsAnyOrder` (`pkg/series`)
  - `TestBookCountsInPlace` (`pkg/rpc`)

## Cost and risk

- About 150 lines and three tests.
- New JSON keys appear only for a variable series, and on the release
  file. A fixed series' `info` is unchanged, and so is the GUI's parity
  of it.
- `rpc.CountBook` is no longer used by the server. I kept it, because
  the test uses it as the reference.
- The full suite passes with `go test -race ./...`, and so do the
  `gui` module's tests (locally, since GitHub Actions is not running).
  staticcheck does not run with go1.27.3.

## Prototype

On `claude/source-orange-r24`; see the commit "ORANGE-034".
