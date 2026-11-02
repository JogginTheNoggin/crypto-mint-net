# RED-031: A trickler that takes 1 MiB a chase and claims a little less than the honest entry is "moving", and no turn ever reaches it

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ bc2e4bc (Source Round 12 Part B) |
| Component | pkg/p2p/chase.go (`partsRank.below`, `partsMoving`, `saveProgress`, `reclaimParts`) |
| Surface | peers (a sender the node chases) |
| Fixed by | `claude/project-thread-ggo8dt` @ 56633dc (BLUE-013) |

## Summary

D136 and D137 make a chase *slow* when it took in less than `partsMoving`
(1 MiB) in its last chase, and give a slow waiting chase its turn after two
backoff steps (B5). A moving file is not slow, and a turn is only given to a
slow waiter, against a slow file or (rarely) a moving one nearer its end.
Between two moving fetches the old order stands (`lessShown`, then more
bytes still to come ranks below).

A sender that shows the same work as the honest one, claims a large entry a
little smaller than the honest entry (the size is on the ledger, and no
check is made until the whole file is in), sends 16 parts (1 MiB) and lets
the next request time out, is *moving* every chase (D137: a timeout after
`partsMoving` has arrived counts as moving). Its `partsToCome` is smaller
than the honest entry's, so it ranks above any honest fetch, moving or
slow, for good. A moving honest waiter has no turn; a slow honest waiter's
turn fails against a moving file nearer its end ("a turn against a moving
file nearer its end", `TestPartsRankTurns`). RED-029's residual is therefore
open at 16 times the trickler's cost: 1 MiB a chase instead of 64 KiB, and
this one is independent of how fast the honest sender is.

## Reproduction

A temporary test in `pkg/p2p` (not committed), the harness of
`TestBlackSlowHonestCannotReclaimFromTrickler` (the disk reports its
reserve while two parts files exist; `RequestPace` raised so the test's
speed is not paced): the trickler claims 2000 parts of 64 KiB, answers 24
requests in a row (1.5 MiB) and ignores the 25th, over and over. After 1.5 s
the honest sender, claiming 2001 parts and answering everything at once,
connects:

```
honest asked 0 parts in 15 s; trickler asked 775 parts (48 MiB)
log: result=slowed why="request timed out" (trickler), then
     "chase waits for disk space" (honest) every few hundred milliseconds
```

At the defaults (30 s timeout, 30 s backoff) the trickler costs the attacker
about 1 MiB a minute, 17 KB/s.

## Impact

Medium. While the trickler stays connected and the node's free space above
its reserve is under about twice the entry, the heavier ledger's large entry
cannot be fetched from anyone. No consensus effect. It needs the same
position as RED-029 (the heavier ledger's work shown, and the entry's size).

## Recommendation

The ranking between two moving files must not depend on a size the sender
states alone, and a holder must not keep a waiter out for good:

- give every waiting chase a turn after `partsTurn`, moving or slow, against
  a file that has held the room for as many chases as the waiter has waited
  (age, not claim): after the turn the holder is dropped once and goes to
  the end of the line, so a trickler cannot keep the room by being a little
  smaller;
- or count the claim only once the file is whole: while two files claim the
  same entry hash, treat differing sizes as evidence one is wrong and give
  the room to the one with more bytes already held (what has been paid for,
  not what is claimed);
- or one parts file per entry hash (Black's option), so a second sender of the
  same entry never needs room.

No size is refused; a fetch that moves keeps its room.

Test: as above; the honest sender is asked for parts within a few seconds.

## Resolution

Fixed (D139). Room at the disk reserve no longer goes by rank. A waiting
fetch takes room only for the bytes it actually receives, cut off the end
of a file that would finish later at the rate its last chase measured. A
trickler that claims just under the honest size and takes 1 MiB a chase
has a low rate in bytes a second, so the honest fetch, receiving faster,
cuts its file a part at a time and finishes. Test:
`TestTakeFrom` ("RED-031: a trickler just under") covers the
rule. `TestMovingTricklerJustUnderCannotHoldTheReserve`
is an integration test of the same shape. To be honest about it: it
passes on the old code too, so it guards the shape but does not
reproduce the finding; the unit case does.
