# ORANGE-023: Probing measured; a probe throws the whole reply away

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `97456c1` |
| Area | `pkg/p2p` (`fetchParts`), tests (`largeSender` replies of several parts) |
| Effort | small |
| Decided by | Grey, with Red (a D115 matter: Black reviews in Part D) |

## Summary

I measured D140's probing: a fetch with no room and no measure yet asks
for the part at its offset and throws it away until it has measured its
sender.

**An honest sender is shown false while a fetch probes.** A real
sender's parts reply is not one part. It holds up to `partsReply`
(64 MiB) in chunks of the asker's chunk size, each following the last
(`replier.parts`). D140 throws away only the chunk that found no room,
and leaves the fetch's offset where it was. The next chunk of the same
reply starts one chunk later, so `fetchParts` sees a part that does not
start at its offset and returns `falseErr`. The honest sender is shown
false, and its entry is not fetched from it.

Every parts test so far uses a fake sender that answers each request
with one part, so none of them could see it.

| A fast honest fetch that finds the room held | D140 (`97456c1`) | This change |
| --- | --- | --- |
| Replies of 1 part (the tests' fake sender) | reaches its end at 5.5 s | reaches its end at 5.5 s |
| Replies of 4 parts | shown false after 28 parts; never reaches its end | reaches its end at 5.2–6.2 s |
| Replies of 16 parts | (not run) | reaches its end at 3.6 s |

That is a D115 case: an honest, faster fetch of a heavier ledger stops
at the disk reserve whenever another fetch holds the room.

**The fix.** A probe throws away the whole reply it is in, not one
chunk. Each request tracks the offset it expects next (`want`), which
moves with every part received, kept or not. Once one part of a reply is
thrown away, the rest of that reply is thrown away too and none of it is
written. The checks are the same checks, against `want` instead of the
file's offset. When no part is thrown away, `want` is the file's offset,
so nothing else changes.

No consensus change. It changes only which parts a waiting fetch writes
and how it checks them, in node policy.

## The cost of probing

With the fix, I counted what a probe throws away. "Parts served" is
every part the fresh sender sent; the entry is 64 parts.

| Case | Parts served | Thrown away | Reached its end |
| --- | --- | --- | --- |
| Fresh fetch 16× faster than the holder, replies of 4 parts | 76–84 | 12–20 | at 5.2–6.2 s; the holder is cut |
| Fresh about as fast as the holder (within the 1.5× margin), replies of 4 | 84 | 20 | at 33.7 s, after the holder finished at 17.3 s |
| The same, replies of 1 part | 82 | 18 | at 36.6 s, after the holder at 20.1 s |

What it costs, in general:
- **A fetch that cuts.** It throws away up to one measure's worth:
  `min(partsReply, the rest of the entry)` minus the room it found, or a
  `ChaseSlice` of its sender's output. For an entry under 64 MiB, with no
  room at all, that is the whole entry once, fetched again after.
- **A fetch that waits.** Its measure goes stale after `partsStale`
  (2 × max(`Backoff`, `ChaseSlice`), 4 minutes by default), and it probes
  again. In these runs that threw away a quarter to a third of the waiting
  sender's output for as long as it waited: each probe lasts up to a
  slice (2 s here, 2 minutes by default), and the next comes a
  `partsStale` later.

I am not proposing a change to that cost this round. It is what D140
pays for a measure no burst can game (Black's B6). The waiting case is
bounded by how long the holder holds, and an honest holder finishes.
Two ways to lower it, for Grey and Black to weigh later:
- back off a waiting fetch's re-probes while each one ends in the same
  wait, up to a cap. The waiter's own kept file must then not count as
  stale (`takeFrom`'s priority 1) while it waits by its measure;
- or shorten a re-probe to the part of a slice needed to show the
  sender is now 1.5× faster. That is a B6 trade-off, so Black's call.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- A test-only harness on one node with `Backoff` 500 ms, `ChaseSlice`
  2 s, `PartsReserve` 1 and `RequestPace` 1<<20.
- Two fake senders, each of a different random large entry. Each reply
  holds 1, 4 or 16 parts of 64 KiB, after a set delay.
- `SetDiskSpace` reports 1 TiB in all, so the reserve is 2%, plus room
  for the holder's entry minus the bytes in the parts directory.
- The holder (600 parts) sends its first 560 parts at once, then slows,
  so the room is nearly full when the fresh sender (64 parts) arrives
  1.5 to 3.5 s later.
- "Reached its end" means the fresh sender was asked for its last part.
- On `97456c1`, with replies of 4 parts, a debug line in `fetchParts`
  showed the fetch end with `part 1638400+65536 of 4194304 for entry …,
  want 1572864`: the second chunk of a probed reply.

## Idea

- **`fetchParts`:** before each request, `want` is the file's offset and
  `probing` is cleared. Each part is checked against `want` (offset,
  size, and the parent when `want` is 0); `want` moves by each part's
  size. Once a part is thrown away, `probing` is set and every later
  part of that reply is checked and thrown away. The test before asking
  still uses `probing` from the last reply, so a fetch that probed asks
  again only while `mayTake` says it could take room.
- **Tests:**
  - `largeSender` gains `chunks` (parts in one reply, as a real sender
    sends) and `pause` (a wait before each reply, by offset). Unset,
    it behaves as before.
  - `TestProbeKeepsAnHonestSender`: a holder fills the room and slows
    down; an honest fresh sender, much faster, replies four parts at a
    time. On `97456c1` the fresh fetch never reaches its end in 30 s. With
    the fix it does in about 5 s. It is skipped under `-short`.
  - `TestTwoSlowFetchesBothFinish` and the B5 and B6 tests pass.

## For Red and Black

- Nothing thrown away is ever written, so a probe cannot put bytes of
  one reply at another offset.
- A part that does not follow the last in its reply is still shown
  false, as is a first part that does not start with the parent.
- What a probe counts toward its measure is the same: every byte
  received over the request's time (D140).

## Cost and risk

- About 10 lines in `fetchParts`, and the tests.
- The full suite passes: `go test -race ./...`,
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun`,
  `TestTwoSlowFetchesBothFinish`, `TestProbeKeepsAnHonestSender`, and
  `GOEXPERIMENT=simd` for `pkg/p2p`.

## Prototype

On `claude/source-orange-r13`; see the commit "ORANGE-023".
