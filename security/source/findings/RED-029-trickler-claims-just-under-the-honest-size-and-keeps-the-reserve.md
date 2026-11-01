# RED-029: A sender that claims a large entry a little smaller than the honest one, and trickles it, still holds the disk reserve

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 1ae02ca (Source Round 11 Part B) |
| Component | pkg/p2p/chase.go (`partsRank.below`, `partsToCome`, `reclaimParts`, `fetchParts`) |
| Surface | peers (a sender the node chases) |
| Fixed by | `claude/project-thread-ggo8dt` @ 75a71e7 (BLUE-012) |

## Summary

D132 (Black's B4) ranks "more bytes still to come" lower, so a sender that
names a huge size and trickles cannot hold the reserve against an honest
entry. That is confirmed closed against a claim far larger than the
honest entry (`huge-claim` below: the honest fetch gets its room at once).

The rank is a comparison of two claims, and the attacker chooses its claim.
The large entries of a ledger are public, so a sender that has copied the
heavier ledger knows the size of the entry an honest sender will serve, and
names a size a little smaller (`S_honest - 1`, or a few percent less). Its
`partsToCome` is then smaller than the honest entry's, so the trickler
ranks *above* the honest fetch: `reclaimParts` can drop the honest one but
never the trickler's. The trickler sends real bytes up to the point where
the disk is at its reserve (at most `S_honest - 1`, so only a disk with less
than about twice the entry's size above its reserve is affected), and then
sends one part (64 KiB) per chase and lets the next request time out, so
every chase is `Slowed` (took something) and never idle. The honest fetch
finds `partsRoom` false, reclaim fails, and it waits for disk space again
and again. BLUE-011's note on RED-027 listed this as still open ("a sender
that trickles a part per turn is not idle").

## Reproduction

A temporary test in `pkg/p2p` (not committed), the harness of
`TestTricklerCannotHoldTheReserve` (the disk reports its reserve while two
parts files exist): the trickler shows the same work as the honest sender,
claims N parts, answers one part per chase (answers a request, ignores the
next, `RequestTimeout` 300 ms), and the honest sender is started after
the trickler has 8 requests in; its entry is 2001 parts (of 64 KiB). Two cases:

```
huge-claim  (trickler claims 1<<20 parts): honest got room: true  (honest asked 4 parts)
just-under  (trickler claims 2000 parts):  honest got room: false (trickler asked 82 parts, honest 0 in 15 s)
log: result=slowed why="request timed out" (trickler), then
     "chase waits for disk space" every few hundred milliseconds (honest)
```

At the defaults (30 s timeout, backoff 30 s, step 64 KiB) the trickler
costs the attacker 64 KiB per minute to keep the room; filling the disk to
the reserve first costs real bytes, but no more than the honest entry.

## Impact

Medium. A heavier ledger with a large entry cannot be taken while the
trickler stays connected, as before D132 but with a condition: the node's
free space above its reserve must be less than about twice the entry. No
consensus effect, and the reserve still guards the store. The attacker needs
the heavier ledger's work shown (a copy of it is enough, as in Black's B4)
and the entry's size, which is on the ledger.

## Recommendation

The rank must not depend on a number the sender states alone. Options:

- rank by rate: a parts file that took fewer than a slice's worth of bytes
  over its last chase (or its last few) ranks below a fetch that is waiting,
  whatever it claims. A sender that moves bytes keeps its room, a trickler
  does not; and a file nearer its end is protected because it is moving;
- or by age: a file that has held the reserve past a number of chases
  without finishing yields to a waiter that has not held it, then goes
  back to the end of the line (turns, not ranks);
- or one parts file per entry hash (Black's option), so a sender cannot
  hold the room for an entry that another sender is also serving.

No size or count is refused; the sender pays for the bytes it claims, and a
sender that is moving them keeps its room.

Test: as above; the honest fetch is asked for parts within a few seconds in
the `just-under` case too.

## Resolution

Fixed by BLUE-012 (decision D136), by the rate option. At the disk
reserve a kept parts file ranks as slow, just above a departed sender's,
when the last chase that asked it for parts did either of these:
- took in less than `partsMoving` (1 MiB, a sixty-fourth of a parts
  reply);
- had a parts request time out.

This is kept on the progress (`partsSlow`), so a waiting chase is ranked
by its own last chase too, and a trickler whose file was taken cannot take
the honest one back. A sender that sends a part a chase is slow whatever
size it claims. Among moving fetches, D132's order stands.

`partsMoving` only ranks; it refuses nothing.

Still open: an honest sender on a link so poor that every chase takes in
under 1 MiB, or times out, ranks with tricklers.

Test: `TestTricklerJustUnderCannotHoldTheReserve` (`pkg/p2p`). The
trickler claims 2,000 parts against an honest 2,001, sends one part and
lets the next request time out. The honest fetch is asked for 16 parts.
The test fails with only "took nothing" counting as slow.
