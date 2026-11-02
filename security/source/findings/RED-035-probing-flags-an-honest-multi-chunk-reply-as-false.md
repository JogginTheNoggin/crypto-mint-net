# RED-035: A fetch that probes at the disk reserve throws the first chunk of a reply away, so the second chunk is "false" and the honest sender is flagged

| Field | Value |
| --- | --- |
| Severity | high |
| Status | fixed |
| Found on | `claude/m10-watch` @ 97456c1 (Source Round 13 Part B) |
| Component | pkg/p2p/chase.go (`fetchParts`: the `probing` path and the offset check in the part callback) |
| Surface | peers (every honest sender of a large entry, while the node is at its disk reserve) |
| Fixed by | `claude/project-thread-ggo8dt` @ 8a64066 (BLUE-014) |

## Summary

D140 lets a fetch that is waiting for room with no measure of its sender
"keep asking for the part at its offset and throw it away until it has
measured the sender". The callback does this with `probing = true; return nil`
on the part that found no room: it neither writes the part nor advances
`pt.off`. A real parts reply is not one part. `replier.parts` (serve.go) sends
the item from the asked offset in chunks of about one chunk each, up to
`partsReply` (64 MiB), and every chunk of the reply goes through the same
callback. The callback checks first that `part.Offset == pt.off`. The part
after the one thrown away starts at `pt.off + len(part)`, so the check fails
and the callback returns `falseErr`: "part X+N of T for entry E, want Y of
T". The chase ends `False`: the sender is backed off (doubling), the group
is dropped, and the sender is flagged. An honest sender of any large entry
(anything past one chunk) gets this at its first reply while the node is
probing.

The tests of D139 and D140 pass because the harness's raw senders answer a
parts request with a single chunk.

The tracker's limits are 5 for priority, 10 for a time-out and 30 for a block
(24 h). The flag this gives is 5 for each false chase, so a node probing at
the reserve times out an honest sender after two chases and blocks it after
six, and the address is blocked for all the pools it serves.

## Reproduction

A temporary test in `pkg/p2p` (not committed). The parts disk holds 1000 parts
of 64 KiB above its reserve. A raw sender (the holder) claims a 1000-part large
entry, sends 999 parts and ignores the rest, so its file is kept with a fresh
measure and one part of room is left. `Chase.Backoff` 20 s, `ChaseSlice` 2 s,
`RequestTimeout` 300 ms. 1.5 s later a second raw sender (honest) shows the same
work, claims the same entry and answers each parts request with 4 chunks:

```
p2p: chase ... peer=b2f12fca result=false why="part 131072+65536 of 65536000 for entry 651eaca1, want 65536 of 65536000"
the honest sender is asked for 1 parts request; its tracker flag total: 5
```

The first chunk fits the byte of room. The second finds none, the fetch has no
measure and starts probing, and the third arrives at 131072 when the offset
is still 65536.

## Impact

High. At the reserve with a kept file of any other sender (a legitimate partial
from a slow sender is enough, or an attacker's holder, which costs the
bytes up to the reserve), every honest sender the node chases next for a large
entry is flagged false and, after a few chases, timed out and blocked for a
day. The heavier ledger's large entry cannot be fetched from anyone (D115),
and the honest senders are blocked. No consensus effect.

## Recommendation

Keep a running expected offset for the reply (`next := pt.off`, advanced by
each part whether it is written or thrown away) and check each part against
`next`, not `pt.off`; when probing, count the bytes and ignore their content
beyond the first check. Add a test with a raw sender that answers with several
chunks, and a case with a probe that begins on the first, a middle and the last
chunk of a reply.

Test: as above; the honest sender is not flagged and its fetch takes the room
once measured.

## Resolution

Fixed (D142), with ORANGE-023's fix as Orange wrote it. Each parts
request checks every part against the offset it expects next (`want`),
which moves with every part received, kept or not; once a probe throws
one part of a reply away it throws the rest of that reply away too, and
none of it is written. Test: `TestProbeNeverFlagsAMultiChunkSender`
(replies of 4 chunks, the probe beginning on the first, a middle and the
last chunk) fails on 97456c1 with Red's message and passes; Orange's
`TestProbeKeepsAnHonestSender` passes too.
