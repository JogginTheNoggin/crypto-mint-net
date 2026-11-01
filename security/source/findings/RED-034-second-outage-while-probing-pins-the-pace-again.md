# RED-034: A second pair of lost requests while the pace is being probed pins the asker to one request a window for 30 windows again

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ bc2e4bc (Source Round 12 Part B) |
| Component | pkg/p2p/pace.go (`reqWindow.dropped`, `probing`, `known`) |
| Surface | peers (a sender the node chases; an honest sender with two short outages) |
| Fixed by | |

## Summary

BLUE-012 (D136) fixed RED-028's first case: one pair of lost requests now
starts a probe that doubles the pace each window the peer replies, back to
`RequestPace` (the same outage test: 480 requests a window again after 10
windows). But `dropped` treats a second pair in a row while probing
(`active && w.probing`) as "its budget, which holds": `w.limit` is set to
`max(1, n*3/4)` for the `n` requests sent *in the window of the second
drops* (small, since the probe is at a few a window), `probing` ends, and
`known` takes the number. The pace is again `1` for `paceLearn` windows (30),
and `known` makes any later drop with at least as much sent hold at once.
So two short outages a few windows apart, which an honest sender on a flaky
link or under load can have, or a sender that wants the asker slow, give
the RED-028 pin back: n is small because the asker was probing up from 1.

## Reproduction

The RED-028 test (not committed), with requests 6 and 7 and again 16 and 17
ignored; window 2 s, `RequestTimeout` 1 s:

```
window 0 (0-2 s):  7 requests   (first outage)
windows 1-4 (2-10 s): 1, 2, 4, 2 requests  (probing up; second outage at 8-10 s)
windows 5-19 (10-40 s): 1 request each, to the end of the test
```

Without the second outage the same test goes 1, 2, 4, ... 480 and is at the
configured pace after 10 windows.

## Impact

Medium, as RED-028 was, for a sender with two short outages in a few
minutes; a sender that wants the asker slow can cause it at will (it is
the only one slowed). No consensus effect.

## Recommendation

A drop while probing must not set the pace from `n` in a window where the
asker was itself held low by the probe. Hold the budget only when the drops
came with the asker at the pace it had *before* the first learning (for
example the highest `n` sent in a window since the first learning, or
`RequestPace/2`), as RED-028 recommended; otherwise halve the probe's
current limit (not below its starting value) and keep probing. A floor of
`RequestPace/4` for any held pace also bounds the cost of a wrong guess.

Test: as above; after the second outage the requests per window rise again.

## Resolution

Open.
