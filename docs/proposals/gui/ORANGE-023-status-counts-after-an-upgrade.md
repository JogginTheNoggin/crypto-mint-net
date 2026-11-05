# ORANGE-023: Keep `poo status -counts` short after an upgrade

| Field | Value |
| --- | --- |
| Status | proposed (needs Source) |
| Based on | main @ 55c5efa |
| Area | the `replay.snapshots_unused` field of `poo status` (pkg/node, pkg/cli: Source); gui/web/static/views/node.js reads it |
| Effort | small |
| Decided by | |

## Summary

ORANGE-021 made the Node view's 5 s refresh short: `poo status -counts`
prints one row per series, 629 B on the reserve series. Round 31's pool
snapshots (D178) add a `replay` field to that answer. It includes
`snapshots_unused`, which lists, one string per pool, every snapshot the
start did not use and why.

A snapshot written by another build is never used, by design. So the
first start after every upgrade lists every pool that has a log, and the
list stays in every `poo status` answer until the node is started again:

| Pools with a log | `-counts` after a normal start | After an upgrade |
| --- | --- | --- |
| 1,024 (the reserve series) | 877 B | 167 KB |
| 8,192 | 878 B | 1.3 MB |

The Node view asks for this every 5 s. The page itself copes: it shows
five of the reasons, and "Output as printed" is drawn only when opened
(ORANGE-020). But poo-gui holds 69 MB instead of 31 MB at 8,192 pools,
and the answer is back to the size ORANGE-021 removed.

The proposal is for Source: print the number of unused snapshots, and
only the first few reasons, the way `poo doctor` already sums it up ("N
pool snapshots were not used at the last start…: first reason"). The GUI
already shows only five.

## Problem

`node.Replayed().Unused` gets one entry for each pool whose snapshot
failed to load. `poo status` prints it whole, with or without `-counts`.
Each entry is about 160 bytes:

```
06ff4e95#0: snapshot not used: written by "poo (devel)", this is "poo v0.0.0-20261105073815-55c5efa54e35 55c5efa54e350279569278b6c9005cb3f8d8bf01"
```

The build id changes with every release and every commit
(`buildID()` in pkg/node/snapshot.go), so this is not a fault case: it
happens to every node on its first start after an upgrade, for every
pool it has a log for. The same goes for a damaged disk or copied logs,
where many snapshots fail at once.

## Measurements

The probe homes were made with a throwaway Go test that follows 1,024
or 8,192 pools of a test series, puts one record in each pool's log,
and closes the node so that every pool gets a snapshot. To make an
upgrade, the same home was started first with a poo built with
`-buildvcs=false` (build id "poo (devel)"), then with poo at 55c5efa.
The GUI probe opened the Node view in Chromium through Playwright,
waited 8 s and sampled 60 s, twice for each upgrade start.

| Pools | Start | `poo status -counts` | Unused listed | Page busy | Long tasks | Longest frame gap | poo-gui RSS |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1,024 | after an upgrade | 167 KB, 23 to 30 ms | 1,024 | 1% | none | 33 to 50 ms | 26 MB |
| 1,024 | normal | 877 B, 18 ms | 0 | 1% | none | 33 ms | 19 MB |
| 8,192 | after an upgrade | 1.3 MB, 114 ms | 8,192 | 2% | none | 33 to 100 ms | 69 MB |
| 8,192 | normal | 878 B, 85 ms | 0 | 1% | none | 67 ms | 31 MB |

What this shows:

- The page stays smooth, because the Followed pools panel draws its own
  table and folds the rest unopened. The Round 29 cost (5% busy, 150 ms
  pauses at 1,024 pools) does not come back.
- The cost is in poo-gui and on the wire. Every 5 s, 1.3 MB goes from
  the node to poo-gui, through its job stream and into the page, and
  poo-gui holds 38 MB more.
- It lasts the whole run after an upgrade, which can be months.

## Idea

1. **Source:** in `poo status`, print `snapshots_unused` as the number
   of unused snapshots, and the reasons for at most a few of them (for
   example the first 5 or 20, names Source's to choose). The full list
   stays in the node log, which already has one warning per pool, and
   `poo doctor` already prints a count and the first reason.
2. **GUI:** `statusNotes` in node.js reads the count from poo's answer
   instead of the list's length, and shows the reasons poo printed. No
   other change.

This changes only what is printed. The GUI adds nothing up itself
(D84): the count comes from poo.

## Cost and risk

- **Source:** a few lines where `Replayed()` is turned into the status
  answer, and the matching field in the GUI note.
- **Risk:** none to consensus. It changes what `poo status` prints, not
  what a node accepts.

## Prototype

None: the change starts in Source, like ORANGE-017 and ORANGE-021. The
"normal start" rows above are what the panel would cost with a short
answer.

## Modularity

Nothing to challenge.

## Consensus

None.
