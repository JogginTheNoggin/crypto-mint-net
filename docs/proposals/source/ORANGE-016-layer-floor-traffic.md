# ORANGE-016: layer_floor traffic measured; bound it with layer_rate

| Field | Value |
| --- | --- |
| Status | adopted (Source Round 8, D122) |
| Based on | `claude/m10-watch` @ `0fd59fa` |
| Area | `pkg/nodepolicy` (`grind.layer_rate`), `pkg/grind` (`keepMax`), `pkg/engine` (`Grind.KeepMax`, `Keep` takes the strength), `pkg/node` (`WorkWith`) |
| Effort | small |
| Decided by | Grey, with Red for the budgets (Black reviews in Part D) |

## Summary

`grind.layer_floor` (D118) works on the sender. The trouble is at its
peers.

By default, a peer takes 6,000 layers a minute from one node (`tracking`
budget `layer`), which is 100 a second. It drops the rest. A dropped
message still counts against the window, so a node that keeps sending
above the budget stays over it and is starved. Its strongest layers are
dropped along with the rest.

At floor 16, a 30 M tries/s grinder sends 450 layers a second. Over
90 s:
- **the peer took 6,000 of 40,723;**
- **the peer saw 6× less of the sender's work than with no floor at
  all:** 2.4 G against 14.6 G of weight.

The fix is `grind.layer_rate`, at most 10 by default. Layers are kept
strongest first, up to the rate times the 2 s slice. With it:
- **the peer takes every layer;**
- **the sender is never flagged;**
- **the weight both sides see is the same.**

**A correction to ORANGE-010.** Its "3 to 9 times the credit" were
medians per slice. Credit is 2^strength, so the long-run total is set by
the rare strongest layers. Simulated over 2,000 slices, the total gain
is:

| Kept besides the best | Total gain |
| --- | --- |
| The strongest 20 of a slice (10 a second) | 1.5× |
| 100 (50 a second) | 1.7× |
| Every layer at floor 16 | 1.9× |
| Every layer at floor 12 | 2.2× |

The 90-second live run agrees: 1.5× on the sender at floor 16. The weight
is steadier, but the long-run gain is smaller than ORANGE-010 said.

No consensus change. A node only sends a subset of what floor alone
would send, the strongest first. Nothing counted is dropped, which keeps
Black's D118 condition.

## Measurements

Same machine as ORANGE-012.

How they were taken:
- Two `startNode` nodes; a runs a tip job on 3 threads.
- A pushed layer is 95 bytes with its frame header.
- "Received" is b's message count from a.

### 10-second jobs, before (no `layer_rate`)

| Floor | Layers a second, a | Layers b took | Messages b received | Bytes a second to each peer |
| --- | --- | --- | --- | --- |
| 0 (off) | 0.5 | 5 | 6 | 50 |
| 24 | 2.0 | 20 | 21 | 190 |
| 20 | 26.8 | 268 | 270 | 2.5 KB |
| 16 | 456 | 4,563 | 4,564 | 43 KB |
| 12 | 4,530 | 9,070 of 45,460 | 45,464 | 430 KB |

### Longer runs, the budget's window filled

| Run | Layers a sent | Layers b took | b's flags on a | Weight on a | Weight b sees |
| --- | --- | --- | --- | --- | --- |
| Floor 0, 90 s | 45 | 45 | 0 | 14.6 G | 14.6 G |
| Floor 16, 90 s | 40,722 | 6,000 | (not read) | 22.1 G | **2.4 G** |
| Floor 16, 150 s | 66,802 | 6,000 | 3 (`OverBudget`) | | |
| Floor 16, 90 s, `layer_rate` 10 | 945 (10.5 a second) | 945 | 0 | 13.1 G | 13.1 G |
| Floor 12, 90 s, `layer_rate` 10 | 945 | 945 | 0 | 18.6 G | 18.6 G |

Single runs are dominated by the luckiest layer, so compare the weights
within a row, not between rows. The simulation below gives the expected
gains.

The grinder's rate stayed at 29–31 M tries/s in every run. Taking in its
own kept layers costs the sender little.

### Simulation

Per slice: 2²⁵ tries in 8,192 ranges of 4,096, each keeping its
strongest layer, as `layer.grind` does. 2,000 slices.

| Kept | Total credit ÷ best-only | Median per slice |
| --- | --- | --- |
| Best only | 1.00 | 1.00 |
| Top 20 (`layer_rate` 10) | 1.49 | 3.22 |
| Top 100 (`layer_rate` 50) | 1.71 | 4.33 |
| Top 200 | 1.81 | 4.79 |
| Every layer ≥ 20 | 1.56 | 3.56 |
| Every layer ≥ 16 | 1.93 | 5.47 |
| Every layer ≥ 12 | 2.21 | 6.88 |

## Idea

- **Node policy:** `grind.layer_rate`, 0 meaning `DefaultLayerRate`
  (10). At most `MaxLayerRate` (1,000).
- **Grind manager:** `keepMax(rate)` is rate × `Slice` (20 by default),
  set on `engine.Grind.KeepMax` for each search.
- **Engine:** `engine.Grind.Keep` now gets the strength too.
- **`node.WorkWith`:** after a search, sorts the kept layers strongest
  first (lowest nonce among equals) and takes in at most `KeepMax`
  besides the strongest one.
- **Tests:** `TestWorkWithFloorKeepMax` checks that exactly `KeepMax` are
  kept and that they are the strongest. The `nodepolicy` tests cover the
  bounds.

## For Red and Grey: the layer budget

These are findings, not part of this change:
- **Dropped messages are counted** (`tracking.Spend` adds before it
  compares). A sender that stays above the budget never gets back under
  it, and everything it sends after the first minute is dropped, its
  best layers too. Counting only what is taken would let an honest
  heavy sender through at the budget rate. D115 asks that no setting
  stop a node from reaching a heavier ledger. Chases and notices still
  carry weight, but pushed layers from such a peer are lost until it
  slows down.
- **The budget is per sending peer, for all the layers it forwards.** A
  relay forwards the layers of every grinder behind it. At 0.5 a second
  each (no floor), 200 grinders fit in 100 a second. At `layer_rate` 10,
  only 10 do. Whether the default layer budget should grow with the
  number of grinders, or with weight, is a question for Grey and Red.
  It is node policy, not consensus.

## Cost and risk

- About 60 lines with tests.
- A node with `layer_floor` set now sends fewer layers than before, but
  every one is taken, and the weight its peers see goes up.

## Prototype

On `claude/source-orange-r7`, commit `34e3eb7`, which also carries
ORANGE-015:
- `pkg/nodepolicy/nodepolicy.go`;
- `pkg/grind/jobs.go`;
- `pkg/engine/engine.go`;
- `pkg/node/node.go`;
- the tests.
