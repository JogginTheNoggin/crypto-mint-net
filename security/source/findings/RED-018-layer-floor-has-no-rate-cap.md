# RED-018: `grind.layer_floor` sends a layer for every 4,096 tries, far above what peers accept

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 0fd59fa (code e99e3e4, Source Round 7 Part B) |
| Component | pkg/node/node.go (WorkWith), pkg/layer/layer.go (GrindWithKeep), pkg/nodepolicy/nodepolicy.go (Grind.LayerFloor) |
| Surface | node policy / peers |
| Fixed by | `claude/project-thread-ggo8dt` @ 58e4e42 (BLUE-008) |

## Summary

With a `layer_floor` (D118, ORANGE-010) every range of `checkEvery` (4,096)
nonces whose strongest layer reaches the floor is kept, and `WorkWith` then
calls `HandleLayer` for each kept layer one after the other, each of which is
stored, announced and forwarded to every peer. The only bounds are the
validation of the number (0 to 256) and the series' minimum layer strength.
Nothing limits how many layers one search sends, the rate over a job, or the
peers' own limit (the tracker allows an address 6,000 layers a minute, that is
100 a second, and flags the excess).

## Reproduction

A temporary test in `pkg/node` (not committed): `WorkWith` with `Floor` set,
4 workers, 4,194,304 tries:

```
floor=4   kept=1023 layers (1 per 4100 tries)    in 0.35 s
floor=8   kept=1023 layers (1 per 4100 tries)    in 0.30 s
floor=12  kept=648 layers  (1 per 6472 tries)    in 0.23 s
```

So the kept layers are about the tries divided by 4,096 for any floor up to
about the 12 that one range of 4,096 reaches, and the CPU generic hasher
here made about 3,000 a second. A grinder at 40 million tries a second sends
about 10,000 layers a second; a GPU grinder several times that. A peer
accepts 100 a second from an address and flags the rest as over budget, so
the node that does the owner's extra work is the one its peers throttle and
eventually block, while its store and the engine lock take every layer.

## Impact

Low. It needs the owner to set the option (default off), and what is lost is
the node's standing with its peers and some disk; no one else is hurt more
than by any chatty peer. It is a trap in a documented option, and one that
the faster grinders of M11 make worse.

## Recommendation

Cap the kept layers per search and per second (for example the strongest 16
of a search, or a token bucket of about 50 a second), keep the strongest first,
and let `layer_floor` raise the number of layers a node *can* send, not
the number it does. Validate the floor against the series: a floor at or
below what one range of 4,096 reaches (about 12) keeps a layer per range.

Test: with a floor of 4 and a fast fake grinder, a job sends no more than the
cap a second and the rest are dropped.

## Resolution

Fixed by BLUE-008 (decision D120). `WorkWith` sends at most
`node.MaxKeptLayers` (16) kept layers besides the strongest per search, the
strongest first, and at most `node.KeptPerSecond` (50) a second across
searches, using a token bucket that holds one second's worth. That is half
the 100 a second a peer accepts from one address. The rest are dropped,
which loses only credit the node chose not to send. Test:
`TestWorkWithFloorIsCapped` (`pkg/node`). With a floor of 1 over 256
ranges it sends exactly 16, all of strength 8 or more. Six more searches
together stay under the rate.
