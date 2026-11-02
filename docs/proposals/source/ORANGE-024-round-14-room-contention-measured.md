# ORANGE-024: Contention at the disk reserve measured after D142; no change proposed

| Field | Value |
| --- | --- |
| Status | measured; no change proposed |
| Based on | `claude/m10-watch` @ `184bc54` |
| Area | `pkg/p2p` (`fetchParts`, `takeRoom`, `mayTake`, `measureLasts`) |
| Effort | none |
| Decided by | Grey (for the record) |

## Summary

I measured D142 (BLUE-014: ORANGE-023's whole-reply probe and
RED-037's backoff for a waiting fetch) against Round 13's code
(`28f9931`, D140 with ORANGE-023 alone). I found no performance problem
to fix this round.

| Two fetches contending for room, replies of 4 parts | Round 13 (`28f9931`) | Round 14 (`184bc54`) |
| --- | --- | --- |
| Equal-rate senders, 100 s run: parts cut, parts thrown away | 76, 16 | 76, 12 |
| The holder's time to its end | 91–93 s | 87–89 s |
| Slow holder near its end, 60 s run: parts cut, thrown away | 20, 12 | 36, 16 |

The runs are within each other's noise. The cut and thrown-away counts
come from counters added to `takeRoom` and the probe in a test-only
build; they are not in the branch.

**What RED-037's backoff saves, at the defaults.** I calculated this
rather than ran it, because it takes hours. `partsStale` is 4 minutes,
and a probe lasts up to one `ChaseSlice` (2 minutes) or one reply
(64 MiB).
- Under D140, a fetch that waits re-probes about every 6 minutes. That
  is up to 10 replies an hour, about 640 MiB thrown away an hour for each
  waiting sender.
- Under D142, each measured chase that wrote nothing doubles the wait,
  to 4, 8, 16 and then 32 minutes. That is 4 probes in the first hour,
  then about 2 an hour: about 128 MiB an hour once settled, 5× less.

**What a waiting chase costs between probes.** When there is no room
and its measure still holds, a chase yields at once (`WaitDisk`) and
runs again after `YieldWait` (5 seconds). Each run sent the waiting
sender one ledger request and nothing else (counted: 6–7 in 40 s with
`YieldWait` at 5 s). That is cheap, so I propose nothing.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- The harness is ORANGE-023's: one node with `Backoff` 500 ms,
  `ChaseSlice` 2 s, `PartsReserve` 1 and `RequestPace` 1<<20.
- Two fake senders of random large entries, replying 4 parts of 64 KiB
  per request after a set delay.
- `SetDiskSpace` reports 1 TiB with room for the holder's entry above
  the reserve.
- The holder sends most of its entry at once, then slows.
- Both versions ran at the same time with the same settings.

| Case | Version | Holder | Fresh | Node |
| --- | --- | --- | --- | --- |
| Holder 1,000 parts (last 200 at 1.2 s a reply), fresh 300 at 1 s a reply, 100 s | R13 | 1,011 served, end at 92.5 s | 152 served | 76 cut, 16 thrown |
| same | R14 | 1,008 served, end at 89.3 s | 156 served | 76 cut, 12 thrown |
| Holder 600 (last 10 at 3 s a reply), fresh 64 at 2.5 s a reply, `YieldWait` 5 s, 60 s | R13 | 628 served, end at 11.9 s | 32 served | 20 cut, 12 thrown |
| same | R14 | 640 served, end at 11.9 s | 24 served | 36 cut, 16 thrown |

In the last case neither version's fresh fetch reached its end in 60 s,
even after the holder had finished. That is the harness, not the node:
one 2.5 s reply is longer than the 2 s slice, so each slice took one
reply and then waited `YieldWait` (5 s). At the defaults, a 2-minute
slice against a 5 s wait costs about 4%.

## Checked and left alone

- **A waiting fetch's own kept file looks stale to others while it
  backs off.** `takeFrom` still calls a file stale after `partsStale`,
  but under D142 a waiting chase now goes up to 8 × `partsStale` without
  asking. A third fetch therefore cuts the waiter's file first (priority
  1), before a slower holder (priority 2). That does not cost an honest
  fetch: a chase waits only when the holder is within the margin and
  nearer its end, or is 1.5× faster, and in both cases the holder could
  cut the waiter's file anyway. I am noting it for Black, since it is a
  new way that `partsStale` and `measureLasts` differ.
- **RED-036's pace check** adds one comparison in `reqWindow.dropped`.
  It is not on a hot path.

## Prototype

None. The harness and counters were test-only and are not committed.
