# RED-032: Each fresh sender ranks as moving and drops a slow file, however much it holds, so a stream of them restarts a slow link's fetch for good

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ bc2e4bc (Source Round 12 Part B) |
| Component | pkg/p2p/chase.go (`partsRank.below`, `reclaimParts`, `progress.partsSlow`) |
| Surface | peers (many senders the node chases one after another) |
| Fixed by | |

## Summary

`partsSlow` is set only after a chase has asked for parts and taken in less
than 1 MiB. A sender that has not asked yet is not slow: it ranks as
moving. Waiting for room, it drops (`low.drop()`) every file that ranks
below it, and a slow file ranks below a not-slow waiter whatever it holds
and whatever either claims (`a.slow != (b.slow && !b.turn)`, ahead of
`lessShown` and `partsToCome`). An honest sender on a link under about 35 KB/s
(under 1 MiB a chase at the defaults) is always slow, so its file is the
first one any fresh sender takes, and it restarts from offset 0 each time,
and its progress on the entry (possibly hours of a slow link) is lost.

Node ids cost nothing, and a fresh sender needs only the heavier ledger's
work shown, which a relay can copy. The sender then takes the room, takes
less than 1 MiB and is slow, and the honest sender gets its turn back after
two backoff steps, with nothing, from offset 0, until the next fresh sender.

## Reproduction

A temporary test in `pkg/p2p` (not committed), the same harness (disk at its
reserve while two parts files exist): the honest sender claims 400 parts and
gives 3 parts a chase (slow). Every 2.5 s a new raw sender shows the same
work, claims the same entry and answers one request. After five of them:

```
alone for 15.5 s:                honest reached part 114 of 400
with five fresh senders (15.5 s): honest peaked at part 24; its offset went
                                  back down after each fresh sender (5 resets)
```

## Impact

Medium. A slow link (under about 35 KB/s) cannot finish a large entry while
fresh senders keep arriving, at one per minute or so, each costing the
attacker a connection and a short chase. It is reached only at the disk
reserve (`partsRoom` false), as RED-027, RED-029 and RED-031 are. No
consensus effect.

## Recommendation

A sender that has not shown it moves bytes must not rank as moving:

- a waiter that has never asked for parts ranks as unknown, the same as slow
  with a turn: it may take a slow file only after it has waited `partsTurn`,
  as a slow waiter must, and not at once;
- and never drop a file for a waiter that holds fewer bytes than the file
  unless the file's chase has been slow for several chases in a row (so one
  lost request does not make it the first to go);
- optionally, keep the dropped file's progress marker (offset) and ask the
  next sender of the same entry to go on from it with a check of the hash at
  the end only, so a slow link's hours are not lost to one takeover (this
  needs no change to what is valid).

Test: as above; the honest offset never goes down.

## Resolution

Open.
