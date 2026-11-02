# RED-036: The RED-034 fix compares with the first drops' load, so at low load a second outage still pins the pace at one request a window

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ 97456c1 (Source Round 13 Part B) |
| Component | pkg/p2p/pace.go (`reqWindow.dropped`: `peak`, `start`, the `n < w.peak-w.peak/8` case) |
| Surface | peers (a sender with two short outages; a sender that wants the asker slow) |
| Fixed by | |

## Summary

D139 stops a second pair of lost requests from holding the pace when it was
sent from a probe held low: the drops show the budget only if at least
seven eighths of `peak`, what was sent at the first drops, was sent again. The
test in `TestSecondOutageWhileProbingKeepsProbing` uses a first drop with 40
requests in the window. At low load, which is where RED-028 and RED-034 were
found, the first drops have 1 or 2 requests in the window, so `peak` is 2
and `start` is 1. A second pair of lost requests with 2 requests in its
window is not below `peak - peak/8` (2 is not less than 2). It takes the
"budget which holds" case: `limit = max(1, n*3/4) = 1`, probing ends, and
`known` is 1. The asker is again at one request a window for 30 windows, with
the configured pace of 480 unchanged. The comparison with `peak` is a
comparison with the asker's own low load, which says nothing about the sender's
budget.

## Reproduction

The RED-028/034 test (not committed), window 2 s, `RequestTimeout` 1 s,
requests 6 and 7 ignored, and 16 and 17 ignored again:

```
window  0 ( 0- 2s): 7 requests    first outage (peak 2 at the second drop)
windows 1- 4 ( 2-10s): 1, 2, 4, 2 requests    probing 1, 2, 4
window  4: the second outage, n = 2
windows 5-19 (10-40s): 1 request each    pinned again
```

This is the result of Round 12's RED-034 test, unchanged by BLUE-013.

## Impact

Medium, as RED-028/034: two short outages a few windows apart at an idle
chase pin the pace to 1/480 of the sender's budget for 30 minutes at the
defaults, and each further outage extends it. A sender that wants the asker
slow needs only to lose two requests twice. No consensus effect.

## Recommendation

A drop pair may set a held pace only when `n` is a real fraction of the
configured pace, say `n >= RequestPace/4`; with less sent, the drops say
nothing of the sender's budget, whatever the first drops sent, so the probe
steps back and goes on. That also bounds a held pace below by something near
the configured pace, so a mistaken hold costs a factor of four at most.

Test: as above, and the unit test at `peak` 2, `n` 2.

## Resolution

Open.
