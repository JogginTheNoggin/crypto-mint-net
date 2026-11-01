# RED-024: A timeout in the middle of a large entry deletes the parts already fetched, so the next chase starts that entry again from zero

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ 1ae4d46 (Source Round 9 Part B) |
| Component | pkg/p2p/chase.go (`fetchParts`, `chaseErr`), pkg/chase (`Stalled`) |
| Surface | peers (any sender of a large entry) |
| Fixed by | |

## Summary

D124 (B2) says a request that times out ends the chase as `Stalled`, "the
chase's progress is kept, so the next chase with it goes on from where this
one stopped". `fetchParts` keeps its parts file only when it yields
(`keep = true` for the slice deadline and for the disk reserve). Any other
error, a timeout included, leaves `keep` false, and the deferred function
closes and removes the file and clears `st.part`. The progress is kept,
the parts are not, and the next chase asks for the entry from offset 0.

## Reproduction

A temporary test in `pkg/p2p` (not committed): `RequestTimeout` 1 s, a raw
sender as for RED-023. It answers the first nine parts requests and
does not answer the tenth once. The offsets the node asks for:

```
request 10: offset 9 MB     (no answer; the file holds 9 MB)
request 11: offset 0 MB     (after the stall: the file is gone)
request 12: offset 1 MB
```

The log shows `result=stalled why="request timed out"`.

## Impact

Medium. A real heavier ledger with a large entry of, say, 50 GB over a link
that drops one request an hour never completes the entry; every drop costs
all of it. The stall was meant to make a slow or throttled sender delay a
chase and no more. A hostile sender can do it deliberately (answer
everything but the last request), which keeps the node downloading the same
bytes for ever; that costs the sender the same bandwidth, so the loss is the
node's time and disk writes.

## Recommendation

Keep the parts on a timeout, as on a yield: in `fetchParts` set `keep` for
`errTimeout` (and for the revert and layer errors that keep progress), so
`st.part` survives and the next chase resumes at `pt.off`. A part file is
only dropped when the entry is complete, wrong (a false error), or the sender
is gone (RED-023).

Test: as above; after the stall the next parts request has the offset where
the last good one ended.

## Resolution

(Blue fills in.)
