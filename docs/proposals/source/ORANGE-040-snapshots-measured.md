# ORANGE-040: Round 30 measured; pool snapshots start 10 to 30 times faster, but gathering one holds the node for seconds

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `cf8c859` |
| Area | `pkg/node` (snapshots, D178), `pkg/ledger` (`Snapshot`, `Fingerprint`); node policy `snapshot_every` |
| Effort | measurements only; three proposals |
| Decided by | Grey; Black in Part D for Proposal 1 |

## Summary

D178 writes a snapshot of each pool beside its log and starts from it,
replaying only the records after it. I measured histories of 10,000,
100,000 and 1,000,000 entries, each entry with four transactions and a
layer, so two records an entry. The owners use Ed25519 or ML-DSA-65.

**1. A start from a snapshot is 10 to 30 times faster than a full
replay.**

| owners | entries | full replay | from the snapshot | speed-up |
| --- | --- | --- | --- | --- |
| Ed25519 | 10,000 | 1.06 s | 0.20 s | 5× |
| Ed25519 | 100,000 | 12.6 s | 0.88–1.17 s | 11–14× |
| Ed25519 | 1,000,000 | 128 s | 9.8 s | 13× |
| ML-DSA-65 | 10,000 | 3.5 s | 0.42–0.46 s | 8× |
| ML-DSA-65 | 100,000 | 36.5 s | 1.17–1.34 s | 27–31× |

From the snapshot, the start no longer depends on the key algorithm,
because it checks no witness the snapshot covers. What is left grows with
the history, at about 10 µs an entry. In a profile at 100,000 entries:
- `ledger.Restore` is most of it (reading back the 10,000 recent bodies
  and recomputing their undo);
- `maybeCompact` building `liveRecords` is about a quarter (already
  noted as open).

**2. A snapshot start also uses less memory.**

| owners | entries | full replay: peak, after | snapshot: peak, after |
| --- | --- | --- | --- |
| Ed25519 | 100,000 | 396, 209 MiB | 281–286, 171 MiB |
| Ed25519 | 1,000,000 | 3,813, 1,941 MiB | 2,576–2,754, 1,753–1,761 MiB |
| ML-DSA-65 | 100,000 | 388, 181 MiB | 330–335, 163 MiB |

**3. A snapshot is 323 bytes per held entry, whatever the keys.**

It holds no body, so it is the same size for both algorithms:
- 81 bytes a transaction at four transactions an entry;
- 3.2 MB at 10,000 entries, 32 MB at 100,000 and 323 MB at 1,000,000;
- 28% of the log with Ed25519 and 1.5% with ML-DSA-65.

Writing it, outside the lock, takes 4 ms at 10,000 entries, 42–59 ms at
100,000 and 0.43–0.49 s at 1,000,000.

**4. Gathering a snapshot holds the node's lock for 4 to 8 µs per held
entry: 0.4 s at 100,000 entries and 6.4 to 7.8 s at 1,000,000.**

`collect` runs under `n.mu`, so no transaction, entry or layer is taken
while it runs. Its parts take the engine's lock too.

| entries | gather under the lock | `Pool.Snapshot` | `Pool.Fingerprint` |
| --- | --- | --- | --- |
| 10,000 | 23–33 ms | 10–17 ms | 10–16 ms |
| 100,000 | 375–439 ms | 153–250 ms | 159–204 ms |
| 1,000,000 | 6.4–7.8 s | 3.0–3.2 s | 2.9–3.0 s |

The rest is the body index, which copies a location for every held
entry. In the profile at 100,000 entries:
- `tidsByNode` is 42% of the gather. It walks every tid in the pool, and
  runs twice, once for the snapshot and once for the fingerprint.
- The fingerprint is 44% of the gather. It is only needed by
  `-reverify`.

Two other things scale the same way. A clean stop also gathers under the
lock: `Close` after building took 2.5 s at 100,000 entries with Ed25519,
and 20.6 s at 1,000,000. A compaction also writes a snapshot under the
lock. Nothing is refused and no verdict changes, but at a million entries
the node stops taking anything for about 7 seconds each time.

**5. `snapshot_every`'s default of 50,000 records fits the start, but
not the stall at large histories.**

50,000 records is 25,000 entries here. After a crash, a start replays at
most that many records:

| owners | per record replayed | worst replay after the snapshot |
| --- | --- | --- |
| Ed25519 | 63 µs | about 3.2 s |
| ML-DSA-65 | 182 µs | about 9.1 s |

That is a fair bound. A clean stop writes a snapshot, so this applies
only after a crash.

The stall, though, grows with the history, while the interval does not.
At 1,000,000 entries the node stalls about 7 s every 25,000 entries.
Raising the default would make the stall rarer, not shorter. Proposal 1
shortens it, and Proposal 2 keeps it a fixed share of the work.

## Proposals

**Proposal 1: take the fingerprint out of the gather.** Write no
fingerprint in the header. Under `-reverify`, which is rare and already
slow:
1. restore the snapshot into a pool and take its `Fingerprint`;
2. drop that pool;
3. replay the log whole and compare.

That cuts the lock hold by about 45%: about 3 s at 1,000,000 entries. It
also checks more than today, because it compares the pool `Restore`
actually builds, not the one the node held when it wrote the snapshot.
The restore costs about as much as a snapshot start, which is a small
part of `-reverify`'s full replay (9.8 s against 128 s at 1,000,000
entries). Black should confirm it covers what the fingerprint covers
today.

**Proposal 2: scale the interval with the log.** Write a running snapshot
when the records since the last reach the larger of `snapshot_every` and
a tenth of the log's records. The gather's cost per record then stays
about the same at any size: 20 to 40 µs a record at the measured rates.
A start after a crash replays at most a tenth of the log: about 13 s at
1,000,000 Ed25519 entries, against 128 s with no snapshot. Keep 50,000
as the floor and the default.

**Proposal 3: gather outside the node's lock, by parts.** The full fix
for Section 4 is to stop gathering the whole pool at one moment. Two
ways:
- Copy only what changed since the last snapshot. Entries far below the
  tip change only in their layers.
- Gather the cold part without the lock, and the recent window, pending
  transactions and state under it.

Either is a larger change to `pkg/ledger` and `pkg/node`, and needs its
own differential test, so I leave it to Grey for a later round.
`tidsByNode` could also be built once per gather, not twice. That saves
about 20% now, but is subsumed by Proposal 1.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harness was an internal test file in
`pkg/node`, outside the commit.

- **History:** the example series with Ed25519 and ML-DSA-65 allowed,
  following pool 1:
  - four claims, then N entries, each spending each of the four mints
    once;
  - each entry gets a layer of 16 tries, so it is selected;
  - `entry_cache_mib` 1, `no_sync`, everything else at its default
    (`recent_window` 10,000, `snapshot_every` 50,000).
  - Building 1,000,000 Ed25519 entries took 17 min.
- **Starts:** `node.Open` on the directory after a clean stop, timed with
  the heap sampled every 2 ms. In order:
  - from the snapshot;
  - with the snapshot file removed (a full replay; the replay stats
    confirm every record was replayed);
  - from the snapshot again (the full replay's `Close` wrote it).
  - Heaps are `HeapInuse` after two GCs, relative to before the open.
- **Gather:** `n.collect` timed with `n.mu` held, three times on the
  opened node; then `writeCollected` timed without it, with the temporary
  file removed. `Engine.Snapshot` and `Engine.Fingerprint` are timed
  alone after each.
- **Profile:** a CPU profile of five gathers on the 100,000-entry Ed25519
  node.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code.
- Proposal 1 changes only what `-reverify` compares against: the pool a
  restore builds, instead of a fingerprint written beside it.
- Proposal 2 changes only when a node writes its own snapshot.
- Proposal 3 changes how the same snapshot is gathered.
- None changes a verdict or adds a depth limit.
