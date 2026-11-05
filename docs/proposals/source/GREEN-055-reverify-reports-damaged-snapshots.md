# GREEN-055: -reverify reports a damaged snapshot, and doctor reports -reverify

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `cf8c859` |
| Area | `pkg/node` (`loadPool`, the `-reverify` branch), `pkg/cli` (`snapshotsCheck` in doctor) |
| Effort | small |
| Consensus | none: what a start reports and what doctor says. Every pool is still replayed from its whole log under `-reverify`, and which ledger is valid does not change |
| Decided by | Grey (Black may skip it) |

## Summary

Green ran D178 as a node runner:

1. Started a node with snapshots on. The second start read the snapshot
   (`replay.snapshots` 1, `snapshot_records` 18).
2. Flipped one byte in a `<index>.log.snap`. A plain start reported it
   under `snapshots_unused` with "checksum differs", replayed the whole
   log, and the clean stop wrote a fresh snapshot.
3. Checked `status -counts` (snapshots listed) and `poo policy limits`
   (`snapshot_every` listed with `in_place` true).

All of that behaved as written. Three loose ends showed up around
`-reverify`, the flag meant to check everything:

1. **A damaged snapshot went unreported under `-reverify`.** `loadPool`
   read the snapshot header only to compare against it, and on any read
   error returned quietly. The pool was neither `reverified` nor
   `snapshots_unused`, the node logged nothing, and doctor said ok. A
   plain start of the same node reported the damage.
2. **Doctor said nothing about a good `-reverify`.** D178 says doctor
   reports it, but `snapshotsCheck` only spoke when a snapshot differed.
   After a clean `-reverify` the line looked like any other start.
3. **The "not used" line read "1 pool snapshots were"** and showed only
   the first reason, though each pool can fail for its own reason (an
   upgrade on one, a damaged file on another).

## Idea

- **`loadPool` under `-reverify`.** A header that is missing is still
  silent (a pool with no snapshot yet). Any other read error is appended
  to `ReplayStats.Unused`, as on a plain start, and logged as a warning:
  "pool snapshot not compared under -reverify; the whole log is replayed".
  The whole log is replayed either way, as before.
- **Doctor, ok case.** When `Reverified > 0` it adds "-reverify replayed
  N pool(s) whole at the last start and it matched / each matched its
  snapshot". A difference is still the warn case above it.
- **Doctor, unused case.** The count and the verbs agree ("1 pool
  snapshot was ... its log was", "2 pool snapshots were ... their logs
  were"), and every reason is listed, joined with "; ".

## How it was tested

- **`TestReverifyReportsADamagedSnapshot`** (pkg/node) writes a history,
  flips a byte in the snapshot, opens with `-reverify`, and expects
  `Reverified` 0 and one `Unused` reason containing "checksum". It fails
  on `main` with `Unused: []`.
- **`TestSnapshotsCheckReverifyAndReasons`** (pkg/cli) checks the ok line
  after one reverified pool, and the warn line with two reasons. It fails
  on `main`.
- **By hand:** a damaged snapshot under `poo node -reverify` now logs the
  warning, and `poo doctor` shows "warn snapshots 1 pool snapshot was not
  used at the last start ... checksum differs". After a good `-reverify`,
  doctor ends its ok line with "-reverify replayed 1 pool whole at the
  last start and it matched its snapshot".
- **Checks run locally:** gofmt, go vet, staticcheck, `go test -race ./...`,
  and the nine GUI e2e scripts.

## Ideas for later

- The GUI's doctor view shows the same line, so nothing is needed there.
  A small "last start" panel that shows `replay` (snapshots used,
  reverified, unused) could help a runner who never opens doctor.
