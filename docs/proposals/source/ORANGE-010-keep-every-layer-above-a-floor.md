# ORANGE-010: Keep every layer above a floor, not only the best per slice

| Field | Value |
| --- | --- |
| Status | adopted as node policy `grind.layer_floor`, off by default (Source Round 7, D118) |
| Based on | `claude/m10-watch` @ `29b08ce` (code `a92c2ca`) |
| Area | `pkg/grind` (`Manager.slice`), `pkg/layer` (`GrindWith`), node policy |
| Effort | medium |
| Decided by | Grey, Source Round 6: left open, since it changes what a node sends (and so the layer rate peers see); jerry decides |

## Summary

A grind job keeps one layer per 2-second slice, the strongest it found,
and drops the rest. An entry's weight is the sum of the credit
(2^strength) of every distinct layer on it. Across a slice the layers
below the best add up to several times the best one. Measured, keeping
every layer of strength 20 or more gives 3.2 to 5.3 times the credit for
the same CPU, at about 40 layers per slice. A floor of 16 gives 4.3 to
7.2 times at about 500 layers.

This changes what the node sends, so it is a design question for Grey
(and Red, for the message rate), not an optimization. The numbers are
here to decide it.

## Problem

`layer.GrindWith` returns the strongest layer of its range, as
`layer.Grind` always has ("a worker may try nonces freely and keep the
best"). `Manager.slice` runs one such search per slice and hands that
one layer to the node.

For N tries, the expected number of layers of strength exactly s is
N/2^(s+1), and their credit is N/2 for every s up to about log₂N. So
everything below the best is worth about (log₂N − k)/2 times N for a
floor k, while the best alone is worth about N.

## Measurements

A test-only harness ran 2²⁵ SHA-256 layer tries (about one slice at the
default 3 threads), counted strengths, and summed the credit of the
layers at or above a floor k. It compared that with the credit of the
best layer alone. Five entries, same machine as ORANGE-009:

| Entry | Best strength | k = 20: layers, credit ÷ best | k = 16 | k = 12 |
| --- | --- | --- | --- | --- |
| 1 | 25 | 46, 5.25 | 531, 7.20 | 8,112, 9.17 |
| 2 | 25 | 38, 3.56 | 504, 5.48 | 8,072, 7.48 |
| 3 | 25 | 35, 3.88 | 503, 5.78 | 8,260, 7.81 |
| 4 | 25 | 37, 4.94 | 503, 7.00 | 8,164, 9.00 |
| 5 | 26 | 35, 3.20 | 556, 4.30 | 8,248, 5.29 |

## Idea

- `layer.GrindWith` gains an optional floor, and returns or streams every
  layer at or above it besides the best.
- `Manager.slice` hands each to the node: one `HandleLayer` each, and
  each is forwarded like any layer.
- The floor is node policy, defaulting to off, which is today's behaviour.
  It could also be set from a target rate: pick k so a job sends about R
  layers a second at its measured try rate, k ≈ log₂(rate/R). At 27 M
  tries a second and R = 10, that is k = 21.

## Cost and risk

- **More messages.** Each kept layer is a message every peer forwards. At
  k = 20, one job sends about 20 layers a second instead of 0.5. Peers
  already bound layer floods (seen list, scoring, min layer strength), so
  Red should look at whether a much higher honest layer rate changes any
  of those limits.
- **More storage.** Each layer is about 40 bytes in the pool log.
- No change to what the node accepts or selects. The rule is unchanged:
  weight was always the sum of all distinct layers. Only the node's own
  sending changes.

## Prototype

None beyond the measurement harness (not committed). It is a question for
Grey before any code.

## Round 7 check

ORANGE-016 measured `layer_floor` on two nodes and corrects two points
here:

- **The gain.** The ratios above are medians per slice. Credit is
  2^strength, so the long-run total is set by the rare strongest layers.
  Over 2,000 simulated slices, the total gain is:
  - 1.9× for every layer at or above 16;
  - 2.2× at 12;
  - 1.5× for the strongest 20 of each slice.

  The weight is steadier, but the long-run gain is smaller than 3 to 9×.
- **The message rate.** Peers take 100 layers a second from one node and
  drop the rest, the strongest too. At floor 16 a 30 M/s grinder sends
  450 a second, and over 90 s its peer saw 6× less weight than with no
  floor. ORANGE-016 bounds the floor with `grind.layer_rate`.
