# RED-088: `-reverify` compares a full replay with the fingerprint written in a snapshot's header, never with the pool the snapshot restores, and says nothing when the snapshot is behind the log; a normal start never checks the restored pool against that fingerprint either

| Field | Value |
| --- | --- |
| Severity | Medium (the one tool meant to catch a snapshot that restores a different pool cannot see that case; a bug there is a node deciding differently from its own log) |
| Status | fixed |
| Found | Source Round 30 Part B, on `main` @ `cf8c859` (D178) |
| Area | `pkg/node/node.go` (`loadPool`, the `check` comparison), `pkg/node/snapshot.go` (`loadSnapshot`, `readSnapHeader`), `pkg/cli/doctor.go` (`snapshotsCheck`), `pkg/ledger/snapshot.go` (`Restore`, `Fingerprint`) |
| For Black | No consensus rule is touched. This is about whether a node can find out that its restored pool is not the pool its log gives, which is the same-information-different-ledger case. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-031) |

## What is wrong

A snapshot's header holds `print`, the pool's `Fingerprint` when the snapshot
was written. Under `-reverify` a start ignores the snapshot, replays the log
whole, and compares the replayed pool's fingerprint with `check.print`, the
header's value. Three consequences:

1. **It tests the writer, not the restore.** The comparison is replay against
   what the pool was at write time. It never loads the snapshot, so a
   `Snapshot`/`Restore` pair that does not round-trip (a field written and not
   read, a count off by one, a state record handled differently) passes: the
   header still says what the pool was. A normal start from that snapshot
   restores something else and nothing notices.
2. **A normal start never compares either.** After `eng.Restore` the node does
   not compute `Fingerprint` and compare it with `h.print`, though it has both
   and the cost is one pass over the skeleton and the unspent set, which is
   small beside the replay it saves.
3. **It silently checks nothing** when the snapshot is not at the end of the log
   (`check.off == l.Size() && check.records == l.Records()`), as after a crash,
   a kill, or any records taken since the last periodic snapshot. Then
   `reverified` is 0, `reverify_differed` is 0 and `poo doctor` says `ok` (its
   `snapshotsCheck` mentions `-reverify` only when `Differed > 0`). An operator
   who asked for a full check is told nothing went wrong.

## How I checked it

`pkg/node` tests (throwaway, removed), the existing `fixture` and `history`.

**Planted body.** After a clean stop I rewrote the snapshot's first state
record (one bit in the first unspent portion's amount) and recomputed the
file's SHA-256, so the file is whole and the header's `print` is untouched.

```
normal start:  snapshots=1  unused=[]   fingerprint equals the truth: false
-reverify:     reverified=1  differed=0
```

The node started from a pool that differs from its log, and `-reverify` reported
a clean match.

**Behind the log.** Snapshot A at a clean stop, 3 more records, then A put back
as after a crash:

```
-reverify, snapshot behind the log:  reverified=0  differed=0  snapshots=0  unused=[]
```

**How common is "valid file, different pool".** I flipped 1 to 3 bits in random
records of a real snapshot (the checksum recomputed each time), 600 times, and
started a node on each. No panic and no hang (good). 591 were refused. 9 were
used, and of those **3 restored a pool with a different fingerprint than the
log gives**, with no message. A check of the restored pool against `h.print`
refuses every one of them.

## What could happen

- A bug in `Restore` or `Snapshot` (the reason a snapshot differential
  exists) is only caught by the tests that were thought of. In the field the
  node would run on the wrong pool: it could accept or reject an entry its
  peers decide the other way, and the check meant to find out says `match`.
- The checksum guards against damage, not against a logic error: a file that
  is whole and wrong passes it.

## Suggested fix (Blue's call)

- After a restore, compute `Pool.Fingerprint()` and compare it with the header's
  `print`; on a difference treat the snapshot as unusable (`errSnapshot`,
  `Forget`, replay whole) and say so in `snapshots_unused`. It costs a pass over
  the skeleton and the unspent set, and it makes every start a differential of
  Restore against Snapshot. If that is thought too dear at 10^7 entries, do it
  under `-reverify` and `poo doctor`.
- Under `-reverify`, also load the snapshot (into a scratch engine, or before
  the replay) and compare its restored fingerprint with the replay's, not with
  the header's. That is the comparison the flag is described as making.
- When the snapshot does not reach the end of the log, `-reverify` can still
  compare at the snapshot's own offset: replay the log up to `header.off` and
  fingerprint there. If that is not wanted, report `reverified 0 of N pools`
  in `poo doctor` as a warning ("nothing was compared: the snapshot is behind
  the log"), never as `ok`.
- Tests: a snapshot with a planted body and a good header is refused at start
  and reported by `-reverify`; a snapshot behind the log is reported as not
  compared.

## Not tested

A Restore/Snapshot bug that is not planted (there is none I know of; the
ledger-level twin restored every 37 steps passes); pools larger than the test
history, where the fingerprint pass would be measured by Orange.

## Resolution

Fixed in BLUE-031 (D181).

- After `Restore`, every start compares the restored pool's fingerprint
  with the header's `print`. On a difference the snapshot is forgotten,
  the reason goes in `snapshots_unused`, and the log is replayed whole.
- Under `-reverify`, a snapshot behind the log is listed in
  `reverify_not_compared`, and `poo doctor` warns.
- Replaying up to the snapshot's offset is not built.
- Since every normal start now checks the restore against the same
  fingerprint that `-reverify` checks the replay against, the two
  together compare a replay with what the snapshot restores.

Tests: `TestSnapshotRestoreIsChecked` (a whole file with a wrong
`print`: it fails without the check) and
`TestReverifyBehindTheLogIsReported`.
