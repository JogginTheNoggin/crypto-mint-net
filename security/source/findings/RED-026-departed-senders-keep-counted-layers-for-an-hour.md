# RED-026: A sender that stalls and leaves keeps its counted layers in the node's memory for an hour, at 20 times the bytes it sent

| Field | Value |
| --- | --- |
| Severity | high |
| Status | open |
| Found on | `claude/m10-watch` @ 0a6cc94 (Source Round 10 Part B) |
| Component | pkg/p2p/chase.go (`progress.counted`, `marks`, `branch`, `saveProgress`, `sweepLocked`), pkg/p2p/host.go (`ProgressTTL`) |
| Surface | peers (a sender the node chases) |
| Fixed by | |

## Summary

D129 moved the end of `ProgressGrace` from "forget the progress" to "drop the
parts file only", on the grounds that the rest of a departed sender's
progress "costs little memory". It does not. A progress that has counted
layers holds `st.counted`, a map from the hash of every layer counted, plus
`marks` and `branch`, and `checkWork` goes on counting until the work beats
the node's own weight after the common entry (`beat`). So a sender can make a
progress hold about as many layers as `beat` is in credits: for a series
with `min_layer_strength` 0 (the default), one credit per layer, and an
attacker picks the nonces whose hash has strength 0. It then stalls (or
yields); the chase ends as `Slowed` or `Stalled`, the progress is kept,
and the sender disconnects. Nothing drops the progress until
`ProgressTTL` (an hour, counted from the end of the sender's backoff),
and up to 256 progresses are kept, one for each node id, which costs
nothing to change. `ProgressHeld` and the metrics do not count `counted`,
so nothing shows it.

## Reproduction

A temporary test in `pkg/p2p` (not committed): node `a` with a ledger of
weight 23,330,976 (two entries and six strong layers). A raw sender states
a huge weight with a made-up tip, lists one made-up hash, answers the
layers request with a many-layers reply, and the layers-from request with
1,500,000 strength-0 nonces (12 MB), without the last chunk, so the
request times out (`RequestTimeout` 1 s, `ProgressGrace` 0.5 s). The log
shows `result=slowed`. Then the sender closes. Heap after a GC, with the
sender gone and the grace long past:

```
one sender:    heap +80 MB     (progress listed 1 held 3)
three senders: heap +240 MB    (three progresses)
```

That is about 53 bytes held for each 8 bytes sent, nothing is released
while the node runs for an hour, and each sender can bring `beat` worth of
layers: here up to 23 million (1.2 GB) for 187 MB of traffic. 256 such
senders hold 300 GB. A node of an old, heavy ledger has a big `beat`.

## Impact

High. A remote peer exhausts the memory of the node and keeps it exhausted
for an hour, at no cost beyond the traffic, with no work behind it (strength
0). It is RED-021 again: BLUE-009 stopped the paging at `beat`, and BLUE-010
and D129 now keep what was paged after the sender is gone.

## Recommendation

Do not keep per-layer state after a sender has left, and do not keep more
than a compact summary of a connected one. Nothing here may refuse a heavier
ledger; everything it forgets is rebuilt by asking the sender again.

- At the grace, drop `counted`, `marks` and `branch` along with the parts
  file, keeping `common`, the position and `complete`, and set
  `total` to zero and `worked` to false so the work is counted again if the
  sender returns (the layers are still on the sender).
- Make `counted` compact: the order check of D124 already makes the
  layers of one entry distinct, so a set is needed only for the layers
  of the entries still being paged, not for all of them; or keep 8 bytes a
  layer (the nonce, keyed by entry) instead of a hash and a map.
- Count the bytes progress holds (layers, hashes, parts) against one
  budget for the host, and when it is over, forget the progress with the
  least shown first (as C-2 does), connected or not. Show the bytes in
  `poo chases` and the metrics.

Test: as above; after the sender leaves and the grace passes, the heap is
within a few MB of its base and `poo chases` shows no progress for it.

## Resolution

(Blue fills in.)
