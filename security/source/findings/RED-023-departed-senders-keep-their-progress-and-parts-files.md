# RED-023: A sender that leaves after a yield keeps its progress, and its parts file on disk, until the node restarts

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ 1ae4d46 (Source Round 9 Part B) |
| Component | pkg/p2p/chase.go (`saveProgress`, `takeProgress`, `runJob`, `fetchParts`, `partsRoom`) |
| Surface | peers (a sender the node chases) |
| Fixed by | |

## Summary

A chase that yields (the time slice, the request pace, the disk reserve,
or now a stall) keeps its `progress` in `h.prog` under (group, sender id),
including `progress.part`, the temporary file a large entry's parts go into.
Nothing releases it when the sender goes. `runJob` ends a chase whose
sender is gone as `Gone` before it takes the progress, so the progress is
never taken and never dropped. It leaves `h.prog` only when 256 other
progresses push it out (C-2 never forgets a worked one for an unworked one),
or when the host stops, and the file stays on disk until the node
restarts (GREEN-033 removes `poo-parts-*` at start). A sender needs no
identity: the id in `h.prog` is the node id of the hello, which costs
nothing to change.

With BLUE-009's RED-022 guard this gets worse. A fetch now waits when the
disk is down to its reserve (1 GiB or 2%). The room the orphans hold is
never given back, so once departed senders have used it up, every large
entry fetch of an honest sender waits for ever ("waiting for disk
space"), and the node cannot reach a heavier ledger that has one.

## Reproduction

A temporary test in `pkg/p2p` (not committed): a raw peer states a huge
weight with a made-up tip, lists one made-up hash, shows one layer, claims a
1 TB `Large` entry and answers parts requests with 1 MiB of data
(`ChaseSlice` 300 ms, `PartsDir` a test directory). When the node's log shows
the chase yielded, the peer closes its connection. Twelve peers, one after
another:

```
after 12 senders left: 0 peers, 12 part files, 543 MB on disk
```

Nothing removes them while the node runs (watched for seconds; the code
has no timer and no hook on disconnect). Each one could be as large as a
slice of sending (a fast link gives 180 MB a second for two minutes, 21 GB;
RED-022), up to the 256 progresses.

## Impact

Medium. A remote peer pins disk space for as long as the node runs, up to
the reserve, at the price of its own bandwidth, and so keeps every honest
large-entry fetch waiting: a heavier ledger with an entry over 16 MiB
cannot be fetched, which D115 says must not happen. Even without that, the
files and the progress structures (`counted`, `marks`, `branch`) of
departed senders are a slow leak.

## Recommendation

Release what a sender left when it can no longer use it, without limiting
what a heavier ledger can take:

- when a chase ends as `Gone`, or when a peer disconnects, take every
  progress for that sender id and `drop()` it (the part file included);
- give a progress a time to live (for example an hour of no chase
  with it) and sweep the expired in `saveProgress` or the manage loop;
- when a fetch must wait for disk space, first drop the part files of
  senders that are not connected, oldest first, before it waits.

Test: as above; after the peer closes, the parts directory is empty within
a moment and `ProgressHeld` is zero.

## Resolution

(Blue fills in.)
