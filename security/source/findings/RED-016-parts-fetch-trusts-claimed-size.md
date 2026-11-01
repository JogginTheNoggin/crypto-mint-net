# RED-016: A part fetch buffers whatever the sender sends against a size it claimed, and a node holding nothing fetches it with no work check

| Field | Value |
| --- | --- |
| Severity | high |
| Status | fixed |
| Found on | `claude/m10-watch` @ 0fd59fa (code e99e3e4, Source Round 7 Part B) |
| Component | pkg/p2p/chase.go (fetchParts, fetchAll, fetch), pkg/wire/messages.go (Large, DecodePart) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ 58e4e42 (BLUE-008) |

## Summary

An entry larger than `PartsOver` (about 16 MiB, one frame) travels in parts
(D117). The asker learns the size from the sender: a `TypeLarge` reply says
"the next entry has this hash and this size", and `fetchParts` then asks for
parts from offset 0, appending each to one buffer until the buffer reaches
that size:

```go
// pkg/p2p/chase.go, fetchParts
var buf []byte
for {
    ... buf = append(buf, pt.Data...)       // grows with the bytes that arrive
    if uint64(len(buf)) == l.Size { break } // l.Size is the sender's claim
    ...
}
it, err := wire.DecodeItem(buf)             // only now is anything checked
```

Nothing bounds `l.Size` (a uint64; the comment says "what is held grows only
with the bytes that arrive", which is true and is the problem), the number
of requests, or the time, and nothing is checked until the buffer is
complete: the entry's hash is compared with the name the sender gave only
after the last byte. A sender that claims 1 TiB and streams parts keeps the
node buffering until it dies.

The path is reached with no work shown: a node that holds nothing of the pool
(`height == 0`: a new node, a fresh follow, a lost store) goes straight to
`fetchAll`, which has no `checkWork` (it only waits for revert budget), after
a stated weight above zero and the optional priority-peer confirmation (RED-015
lists when that is absent). `fetchBranch` reaches `fetchParts` too, but only
after real work was shown.

## Reproduction

A temporary test in `pkg/p2p` (not committed): a node that holds nothing; a
raw peer sends a notice with a stated weight of 2^62, answers the full
ledger or extension request with `Large{Entry: random, Size: 1 TiB}`, and
answers every parts request with sixteen 1 MiB parts of that total, "more":

```
after 10 s: parts requests=27, 453 MB sent, heap +1027 MB (claimed size 1024 GiB)
```

The heap is about 2.3 times the bytes sent (append doubling), grows linearly
and never stops: the claimed total is never reached.

## Impact

High for a node that is syncing from nothing, which is the first thing every
node does and happens against whichever peers it meets first. One peer, no
work, no valid data: memory exhaustion in about a minute on a fast link.
Nodes that already hold a ledger are not reachable this way, because a chase
there needs real work first.

## Recommendation

- **Cap the claimed size.** Refuse a `Large` whose size is above a limit set
  by policy (default, say, 1 GiB; an entry that large is already far outside
  honest use) and mark the sender's claim false. The entry must be decodable
  and must fit in memory twice (buffer and decoded form).
- **Bound the fetch in time and requests.** Check each part's `Total` against
  the first one (already done), and stop the fetch when it outlasts the chase
  slice or makes more requests than `l.Size / chunk` plus a small margin.
- **Decode as it arrives** where possible, so a bad entry fails at its first
  bad transaction, not after the last byte; at least check the first bytes
  (the parent, which must be the expected one) after the first part.
- **Give `fetchAll` a work gate** (RED-012 recommended it; D114 charged the
  revert budget instead, which does not bound memory): ask for the layers on
  the first page of hashes and require some provisional weight before taking in
  entries; and stop the fetch when a run of N entries brings no layer weight at
  all. As in RED-015 this should go to Black first because of G3.

Tests: a raw peer that claims a size above the cap is refused and flagged; a
raw peer that streams parts for ever against a legal claim is cut off by the
time or request bound, with the node's heap growth bounded; a real large entry
(in parts) is still taken in.

## Resolution

Fixed by BLUE-008 (decision D120), without a size cap (D115, D117):

- **Work gate on `fetchAll`.** A node that holds nothing runs the work
  check from the pool_id before a whole-ledger fetch. Any heavier ledger
  shows some work against this node's none.
- **Checks as it arrives.** Each part must hold at least a byte and stay
  within the stated size, and the first must start with the expected
  parent. Otherwise the reply is false at once, and the sender is flagged.
- **Disk, not memory.** Parts go to a temporary file
  (`p2p.Config.PartsDir`, by default the system's), so a claimed size
  costs the node disk only as the sender sends the bytes.
- **Time.** A fetch past its slice yields, keeping the file and its
  offset, and the next slice goes on from there. The file is removed when
  the fetch ends, when its progress is forgotten, or when the host stops.

Still open, as D117 notes: decoding the entry needs it whole in memory,
as the pool will hold it, so a sender that really sends a 1 TiB entry
still costs that memory at the end. It also costs the sender 1 TiB of
sending, and the hash is checked then. Tests:
`TestLargeClaimIsPacedOnDisk` and `TestLargeClaimWithWrongParentIsFalse`
(`pkg/p2p`).
