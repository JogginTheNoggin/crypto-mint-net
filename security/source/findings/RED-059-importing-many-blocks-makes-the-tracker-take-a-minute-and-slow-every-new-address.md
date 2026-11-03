# RED-059: Importing many blocks makes the tracker take a minute, and slows every new address after

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 21 Part B, on `main` @ `8662766` (D157; the tracker code is older) |
| Area | `pkg/tracking` (`get`, `trimLocked`), `pkg/peerport` (`Import`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ 282571b (BLUE-022) |

## What is wrong

`Tracker.get` makes a record for a new address. When the tracker holds
`MaxAddresses` records (8192) it calls `trimLocked` first, which walks
and sorts every record that holds nothing that must stay. A record with a
block, a flag or a timeout must stay, so when most records are blocks
nothing can be dropped and each new record costs a walk of all of them,
under the tracker's lock. A peer file may hold 65,536 blocks
(`peerport.MaxBlocks`) and `-blocks` makes each one a record: 65,536
blocks cost a growing walk each, in total quadratic.

## How I checked it

`peerport.Import` with 65,536 address blocks, `Options{Blocks: true}`, a
fresh tracker and an empty book:

| step | time |
| --- | --- |
| plan (dry run) | 98 ms |
| apply | 57 s (40,000 blocks alone: 19 s) |
| 20 offences from new addresses afterwards | 33 ms (1.6 ms each, where an ordinary record costs microseconds) |

The CLI offline holds the data directory for those 57 seconds. Through a
running node (`POST /v1/peers/import`) the node spends its CPU on it, and
every tracker call waits for the lock between records. The tracker
file then holds 65,536 records, saved whole each time.

Range blocks do not go through `get`, so they add only a map entry;
65,536 `v4:A.B` ranges (`-blocks -ranges`) block the whole of IPv4, and
the plan lists them as 65,536 lines with no total or warning.

## What could happen

A shared "block list" of a hundred thousand lines stops a node for a
minute and leaves it a little slower for every new address, and a list of
all the ranges cuts it off from the network. Both need `-blocks`, and
the second `-ranges` too, so the owner chose them, but nothing in the
preview says how large the effect is.

## Suggested fix (Blue's call)

- Make `trimLocked` cheap when nothing can be dropped: keep a count of
  protected records and return early, or keep the droppable records in
  their own list.
- Keep blocks from a file in a map apart from the per-address records
  (the tracker already has `ranges`), or take the lock once for an
  import.
- In the plan, print the number of blocks and how much of the address
  space the ranges cover ("blocks 12% of IPv4"), not only the lines.

## Not tested

A running node's behaviour under the import (it stays up; I did not
measure its latency).

## Resolution

Fixed (D158), with ORANGE-031.

- The quadratic trim is fixed by ORANGE-031 (PR #126, `trimAt` in the
  tracker), which Orange measured. It is not repeated here, so Grey
  merges both.
- The plan now says how large the effect is: a note totals the blocks
  and ranges and says how much of IPv4 the `v4:` ranges cover, for
  example "656 blocks, 656 of them network ranges; the IPv4 ranges cover
  1.00% of IPv4". All 65,536 /16 ranges read 100.00%.
- Test: `TestPlanCountsRangeBlocks` (pkg/peerport).
