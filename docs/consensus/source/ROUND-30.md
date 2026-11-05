# Source Round 30: consensus review (Black, Part D)

## Summary

This round takes the third step of disk-backed state. Each pool now has
a snapshot beside its log, holding the skeleton, the unspent portions,
the pending transactions and the sidecar index. A node starts from its
own snapshot and replays only the records after it (D178). `-reverify`
replays everything and compares.

D181 tightens when a snapshot is trusted:
- the build id names the release (RED-087);
- every start compares the restored pool with the snapshot's
  fingerprint (RED-088);
- leftover temporary files are removed (RED-089).

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - A snapshot is the node's own data. It is never sent to a peer or
    taken from one.
  - Every held entry is restored, at any depth.
  - When a snapshot isn't used, the only cost is a replay.
- **A start from a snapshot selects the same ledger as a full replay.**
  Black reran the random-history twin with the twin restored from its
  snapshot after **every** step. That was 150 seeds × 300 steps, with and
  without pruning, at windows of 1 and 2: 38,578 restores and 691
  reorganisations.
  - Each restored pool had the in-memory pool's fingerprint.
  - It then ran on with the same switches, tips, weights and state at
    every later step.

  Black also ran restores every 3 and 7 steps at windows of 0 and 1,000,
  with the same result.
- **A snapshot that may be wrong is never used.** That covers a damaged
  file, a file from another pool, a file from another release, a file
  from before a compaction or a cut log, and a restore that doesn't round
  trip. Each falls back to a full replay through the normal checks.
- **Trusting the node's own snapshot is the owner's decision**
  (2026-11-05, recorded in D178). A snapshot start doesn't re-check the
  witnesses it covers. Black agrees this is safe for the node's own file,
  for the reasons below.
- **Fixed series, transactions and entries are unchanged.** The Round 24
  and Round 25 cross-version tests are byte for byte the same on `main`
  and on PR #226.

| Field | Value |
| --- | --- |
| Round | Source Round 30 |
| Consensus | None (D178 to D181) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `cf8c859` (PR #222: D178 to D180, with Round 29's PRs merged) |
| Previous base | `main` @ `d1a9128` |
| Blue branches | PR #226 @ `3ef2e17` (BLUE-031, D181: RED-087 to RED-089; carries GREEN-055) |
| Red | PR #223 @ `72dcf52` (RED-087 to RED-089) |
| Proposals | ORANGE-040 (PR #225 @ `bf22d04`); GREEN-055 (PR #224 @ `7f397c0`, carried in #226) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## Same ledger from a snapshot or from a full replay (D178)

A snapshot holds what selection reads. **`Pool.Fingerprint`** covers each
held entry in first-sighting order, with:
- its hash and parent;
- whether it is selected;
- its own weight;
- its counts of transactions and makers;
- the hashes of its layers and the mints it claims.

It also covers the selected ledger's tids and every unspent portion at
the tip. Ledger weights follow from own weights and parents, and the
tie-break follows from the sighting order and from which entry is
selected. Both are therefore covered.

- **Restore is all or nothing.** `ledger.Restore` builds a new pool, and
  `Engine.Restore` installs it only on success. A snapshot that is cut
  short, has records after its end, belongs to another pool, or names an
  entry the store doesn't hold is refused whole (`TestRestoreRefuses`).
- **Recent entries come back from their bodies.** Their items are the
  bodies' transactions, checked against the transaction counts. Their
  undo is recomputed as in D175, which Round 29 checked. Older entries
  start cold.
- **The records after the snapshot are replayed normally.** They go
  through `AddEntry` and `AddLayer` with every check, as on any start.
- **Pending transactions are offered again** through the normal checks.
  The pending pool is the node's own and is not selection.

### Black's differential

Black changed the random-history twin to restore from its snapshot after
every step instead of every 37th. The in-memory pool never restarts. The
twin is restored and then has to agree with it from then on, at every
step.

| Restores | Window | Seeds × steps | Restores done | Reorganisations | Result |
| --- | --- | --- | --- | --- | --- |
| every step | 1 | 150 × 300, plus pruned | 38,578 | 691 | identical |
| every step | 2 | 150 × 300, plus pruned | 38,578 | 691 | identical |
| every 3 steps | 0 (nothing cold) | 150 × 300, plus pruned | | | identical |
| every 7 steps | 1,000 (nothing cold) | 150 × 300, plus pruned | | | identical |

The two runs that keep nothing cold trip the test's guard that every seed
must go cold. That is the guard, not a difference. No comparison failed.

The node tests cover the store side:
- `TestSnapshotStart` and `TestSnapshotAfterCompaction`: a start from the
  latest snapshot, and from an older snapshot plus the records after it,
  builds the pool a full replay builds, before and after a compaction;
- `TestSnapshotFaults`: torn, damaged, trailing bytes, another node's
  file, not a snapshot;
- `TestSnapshotRestoreIsChecked`.

### Compaction renumbers first sightings

After a compaction, the running pool takes the first-sighting order that
a replay of the new log would give (`Pool.Renumber`).

- **It changes no selection now.** The selected ledger stays as it is,
  and only later exact ties read the order.
- **It is local information.** First sighting is local by design: two
  nodes that saw entries in different orders may break an exact tie
  differently, and the documents accept that (section 10.3, D11). This
  change makes a running node and its own restart agree, so a restart
  never flips a tie the running node would have kept.
- **It adds no rule.** The tie rule is unchanged.

## When a snapshot is not used (D178, D181)

A node uses a snapshot only when every check passes:
- the file's SHA-256;
- the pool;
- the build id, which since RED-087 names the version, commit and
  `snapshotSemantics`, plus an executable hash for a dirty tree;
- the log's tag, so a compacted, cut or replaced log gives another tag;
- since RED-088, the restored pool's fingerprint equals the header's.

Otherwise the node replays the log whole, which is what every start did
before D178.

- **A newer release never trusts an older one's pool.** Every upgrade
  replays once (RED-087). That matters because a pool that the new code
  would not build from the log is the case the owner's hard-fork
  definition is about. The guard now holds for release builds too.
- **Damage means a full replay.** A body that fails to read marks its
  pool damaged, and its snapshot is removed. A failed engine (D174)
  removes every snapshot at Close. So the next start replays and cuts the
  log at its first bad record, and peers fill in the rest. A snapshot
  never hides damage inside its span.
- **The owner's trust decision.** A snapshot start checks no witness the
  snapshot covers. That is safe for the node's own file:
  - those witnesses were checked when the entries were taken;
  - the file is whole (checksum) and matches its log (tag);
  - it was written by this release (build id);
  - it restores to the pool it was written from (fingerprint).

  The checksum guards against damage, not against tampering. Someone who
  can rewrite a node's files can rewrite its log and its binary too, so
  this adds no new exposure. `-reverify` is the full check.
- **`-reverify`** replays whole and compares with the snapshot's
  fingerprint. A pool whose snapshot doesn't reach the end of the log is
  reported as not compared, never as "ok" (RED-088). Together with the
  start check, it compares a replay with what the snapshot restores.

## ORANGE-040, proposal 1: the fingerprint out of the gather

Orange proposes writing no fingerprint in the header. Under `-reverify`,
the node would restore the snapshot into a scratch pool, take its
fingerprint, and compare it with the full replay. Black was asked to
confirm the coverage.

- **Coverage is the same.** It is the same `Fingerprint` function, so it
  covers the same fields as above.
- **The `-reverify` check gets stronger.** It would compare what `Restore`
  actually builds with what a replay builds, which is the property that
  keeps a node's start honest. Today that is checked only indirectly,
  through the header.
- **What is lost.** The per-start round-trip check that RED-088 just
  added. A start would no longer compare a restore with the pool the file
  was written from, so a writer/restore mismatch would show only under
  `-reverify` or in the random-history twin.
- **Black's view.** Either design is clear for consensus. A snapshot is
  still only a cache of the log, and any doubt costs a replay. If Grey
  takes proposal 1:
  - keep the random-history twin's restore-and-compare, which is the
    guard that catches a writer/restore mismatch;
  - consider running the `-reverify` comparison once automatically after
    an upgrade's first replay. It costs one restore.

Proposals 2 (scale the interval with the log) and 3 (gather by parts)
change only when and how the node writes its own snapshot. Proposal 3
needs its own differential when it is built.

## Clear

| Item | Why it is clear |
| --- | --- |
| D178: pool snapshots, start from the node's own snapshot | Restores the pool the node had. Same selection as a full replay (Black's every-step restore runs; node tests). The node's own data only |
| D178: the sidecar index | Where a record lies in the node's own log. The tag ties it to that log |
| D178: `-reverify`, trust in the node's own snapshot | The owner's decision. The full check stays available |
| D178: damage drops snapshots; a failed engine removes them | The next start replays and cuts at the damage. Missing information, filled by peers |
| D178: `Pool.Renumber` after compaction | First sighting is local. Running and restart now agree. No rule changes |
| D178: `snapshot_every`, `no_snapshots`, status, doctor | Node policy and tooling |
| D179, D180 | Display and output only |
| D181 (RED-087, RED-088, RED-089): release in the build id; every start checks the restore; `.snap.tmp` removed | Uses a snapshot in fewer cases. Not using one costs a replay |
| GREEN-055 (carried): `-reverify` reports a damaged snapshot; doctor reports `-reverify` | Tooling |
| ORANGE-040 (measurements, three proposals); Red (PR #223) | Measurements and findings. Proposal 1 is clear either way (above) |

## Items for the owner

- **Nothing needs your approval this round.**
- **Starting from a snapshot picks the same ledger as replaying
  everything.** Black restored from a snapshot after every single step of
  random histories, 38,578 times. The restored node always matched one
  that never restarted.
- **Your decision to trust the node's own snapshot is built safely.** A
  file that is damaged, comes from another release, or doesn't match its
  log is never used. `-reverify` checks everything when you want it to.

How this was checked:
- Read D178 to D181, `ledger/snapshot.go` (`Fingerprint`, `Restore`),
  `engine/snapshot.go`, `node/snapshot.go` (`loadSnapshot`, the header
  and tag checks), and the diffs of PRs #223 to #226.
- Black's random-history runs on `3ef2e17`, restoring every 1, 3 or 7
  steps, at windows of 0, 1, 2 and 1,000.
- The Round 24 and Round 25 cross-version tests: identical on `cf8c859`
  and `3ef2e17`.
- `go test ./...` passes on `cf8c859`, #225 and #226.
  - On `cf8c859` the timing test `TestManySeriesMergeIsLinear` failed
    once, while Black's long differential was running on the same cores.
    Run alone, it passed 3 times out of 3.
  - The ledger, node, store and p2p packages pass under `-race` on
    `3ef2e17`.
