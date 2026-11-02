# RED-002: Missing-chunks request resends chunks without dedup or per-chunk cost

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Component | pkg/p2p/discovery.go (handleMissing), pkg/wire/discovery.go (DecodeMissingChunksReq) |
| Found on | `claude/m10-watch` @ 581476e |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ a733b18 (BLUE-002) |

## Summary

`handleMissing` resends a stored reply's chunks for every position listed
in a `MissingChunksReq`, with no deduplication and no per-chunk accounting.
The whole request costs the sender a single rate-limiter token, yet the
positions list may hold up to `maxItems` (65536) entries and may repeat the
same position:

```go
// pkg/p2p/discovery.go, handleMissing
if !p.limiter.take(time.Now()) {   // one token for the whole request
    return nil
}
h.mu.Lock()
s, ok := h.sent[m.RequestID]
h.mu.Unlock()
if !ok || time.Now().After(s.until) {
    return nil
}
for _, pos := range q.Positions {          // no dedup, no count cap vs chunks
    if int(pos) <= len(s.chunks) {
        p.send(s.chunks[pos-1])             // may send the same chunk many times
    }
}
```

`DecodeMissingChunksReq` already guards the obvious crash — it rejects
position 0, so `s.chunks[pos-1]` cannot underflow — but it allows a long
list with repeats (`q.Positions = [5,5,5,…]`).

## Reproduction

1. From an attacker peer, make a request that the node answers with a
   multi-chunk reply it keeps for `sentTTL` (a peer-list request that
   produces several chunks; the chunks are stored in `h.sent[requestID]`).
2. Send a single `MissingChunksReq` with the same `RequestID` whose
   `Positions` repeats a valid position tens of thousands of times
   (e.g. position 1 repeated 65536 times).
3. The node enqueues one `send` per listed position — up to 65536 copies of
   that chunk — for the price of one rate-limiter token.

A `pkg/p2p` test can assert that a `MissingChunksReq` with a repeated
position causes more than one `send` of the same chunk (today it does); the
fix makes each position resend at most once.

## Impact

Low. The sender queue is bounded by `QueueSize` (default 4096) and a peer
that overflows it is disconnected with `errQueueFull`, so the burst is
capped at a few thousand chunks before the attacker's own connection is
dropped. The cost is the node's CPU/allocation to build and enqueue those
frames and, briefly, the queue memory — a minor, self-terminating waste
rather than a sustained DoS. There is also no `tr.Cost`/`Repeat`
accounting on this path, so the behaviour is invisible to ranking.

## Recommendation

- Deduplicate `q.Positions` and cap the number of distinct positions served
  to `len(s.chunks)`; ignore out-of-range and repeated positions.
- Charge the resend against the peer's budget/cost (`tr.Cost` with the bytes
  resent, and/or `tr.Repeat`) so an abusive pattern costs standing.
- Consider taking a token per chunk resent rather than one per request.

Test: `pkg/p2p` — a repeated-position `MissingChunksReq` resends each
distinct valid chunk exactly once and records a cost against the peer.

## Resolution

Fixed by BLUE-002. `handleMissing` resends each distinct valid position
once per request, ignores repeats and out-of-range positions, records the
bytes resent as the peer's cost, and counts a repeated position as a
repeat (`tracking.Repeat`), which flags a peer past the repeat limit.
Test: `TestMissingChunksResentOnce` in `pkg/p2p` (5,000 repeats of one
position give one resend).
