# RED-033: A slow trickler that takes one part more than the honest link each chase wins every turn, because the honest sender's last measure is stale

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | `claude/m10-watch` @ bc2e4bc (Source Round 12 Part B) |
| Component | pkg/p2p/chase.go (`partsRank.below` with `turn`, `progress.partsLast`) |
| Surface | peers (a sender the node chases) |
| Fixed by | |

## Summary

D137 settles two slow fetches by what each took in on its last chase that
took anything (`partsLast`): the waiter with its turn takes the room when
its `partsLast` is at least the holder's. The honest sender that lost its
file keeps the `partsLast` it had when it lost it, and cannot update it
because it never asks while it waits. A trickler that takes one part more
a chase than the honest link did (and stays under 1 MiB, so it is slow too)
beats it at the trickler's own turn, and the honest turn then compares the
old number with the trickler's larger one for as long as the trickler
keeps it up. D137 lists the cost ("it must send as much each chase as the
honest sender does"); the cost is the honest link's own rate. For a link under
35 KB/s that is under 3 GB a day, and the bytes can be junk.

## Reproduction

A temporary test in `pkg/p2p` (not committed), the same harness: the
trickler claims 200 parts, takes 4 parts a chase and ignores the 5th. 1.5 s
later the honest sender (201 parts) connects and gives 3 parts a chase:

```
honest asked 44 parts (highest part 30 of 201), trickler asked 269;
honest finished: false (25 s)
```

The honest sender takes the room once (a chase that took none has its turn
against any slow file), takes 3 parts a chase, and loses its file to the
trickler's turn (4 parts a chase against 3); it cannot win it back.

## Impact

Low. The attacker needs the honest link's rate in bandwidth and the usual
position, and the node is at its disk reserve. The honest sender is starved
only while the attacker keeps this up.

## Recommendation

- Compare a measure the waiter can raise: when a chase loses its file while
  waiting, forget `partsLast` (set it to 0), so a waiter with its turn
  always wins against a slow file once, and the two take turns on equal
  terms; or compare the bytes each file holds in total, which only a
  sender that has delivered more can have.
- Or give the room to the file that has been waiting longest against the one
  that has held it longest, and keep the loser's offset (RED-032) so the
  turn is not a loss of the progress.

Test: as above.

## Resolution

Open.
