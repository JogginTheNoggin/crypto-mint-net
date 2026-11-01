# RED-027: A connected sender that stalls keeps its parts file, so the disk reserve stays taken and no other large entry can be fetched

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ 0a6cc94 (Source Round 10 Part B) |
| Component | pkg/p2p/chase.go (`dropAwayParts`, `sweepLocked`, `fetchParts`, `partsRoom`) |
| Surface | peers (a sender the node chases) |
| Fixed by | |

## Summary

BLUE-010 (RED-023, D128) lets a fetch that must wait for disk space first
remove the parts files of senders that are *not connected*, and removes a
departed sender's file at the grace. A sender that stays connected has none
of this. It can send parts until the disk is at its reserve, then stop
answering: its chase ends as `Stalled` or `Slowed` (the progress is kept
with the parts, D128/D129), its backoff doubles to an hour, and the
chase after the backoff yields at the reserve again, which saves the
progress and renews the time to live. The file is never reclaimed, and every
other large-entry fetch finds `partsRoom` false and waits, as long as the
sender stays connected. A heavier ledger with a large entry cannot be reached.

## Reproduction

A temporary test in `pkg/p2p` (not committed): `PartsReserve` set to the free
space of the test disk less 300 MB. A raw sender (the hog) states a huge
weight, shows one layer, claims a 1 TB large entry and sends parts of 1 MiB
until the node stops asking; then it stays connected and answers nothing.
A second raw sender (the honest one) does the same flow with another tip
and answers everything:

```
hog sent 236 parts (236 MB); files 1
honest sender was asked for 0 parts in 25 s; files 2
log: result=stalled why="request timed out" (the hog), then
     "chase waits for disk space" every few hundred milliseconds
```

## Impact

Medium. A single connection that costs the sender 236 MB of traffic here, and
whatever the disk's reserve leaves in general, blocks large-entry sync for
as long as it stays connected, not an hour: the sender renews it by being
chased. Several unlisted connections (8 for one address) hold it between
them. The reserve guards the store, as intended; what is lost is the
node's ability to fetch a large entry it should take.

## Recommendation

When a fetch waits for room, reclaim the parts of any other chase that is
not making progress, connected or not, instead of only departed ones:

- rank the parts files by how much their chases have shown (the work
  counted, the stated weight, the last time a part arrived) and drop the
  least first, like C-2 ranks progress, until the fetch fits; the dropped
  chase asks again from offset 0 when its turn comes (a stalled sender
  that is slow pays for that);
- drop a stalled chase's parts after a number of stalls with no byte taken
  in between (not `Slowed`, which took something), so an idle connection
  does not hold them.

No size is refused and no ledger rejected: the first large entry to fetch
still gets the room it needs, and a sender that is moving bytes keeps its file.

Test: as above; the honest sender is asked for parts within a few seconds
and the hog's file is gone.

## Resolution

(Blue fills in.)
