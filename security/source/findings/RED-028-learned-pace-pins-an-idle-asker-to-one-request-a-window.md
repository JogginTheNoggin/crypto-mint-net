# RED-028: Two unanswered requests at low load teach the asker a pace of one request a window for half an hour

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 1ae02ca (Source Round 11 Part B) |
| Component | pkg/p2p/pace.go (`reqWindow.dropped`, `paced`), pkg/p2p/peer.go (`ask`) |
| Surface | peers (a sender the node chases; any honest sender with a transient outage) |
| Fixed by | `claude/project-thread-ggo8dt` @ 75a71e7 (BLUE-012) |

## Summary

D133 teaches the asker a pace when two requests to a peer in a row go
unanswered: `limit = max(1, n*3/4)`, where `n` is the number of requests
sent to that peer in the current window, and it holds for `paceLearn`
(30) windows. The idea is that `n` is the budget the sender enforced when it
started to drop. That holds only if the asker was at the sender's budget
when the drops began. A chase that is going one request at a time (a
reply before the next request, which is how every step of the chase
works) sends a handful of requests a window, far below the sender's budget.
Two requests lost for any reason (the sender stalled for a minute, a network
blip, a restart, an honest node that was busy with a disk sync) then set `n`
to a handful or to 1, and the asker holds itself to `max(1, n*3/4)`, which is
1 request a window (one a minute at the defaults), with the sender's real
budget (`RequestPace` 480 a minute by default) unchanged. This lasts 30 windows,
30 minutes at the defaults, and each later pair of unanswered requests
sets `until` again. The pace never rises while it lasts (a lower limit
replaces a higher one, `answered` only resets the miss count). That is up to
480 times slower than the sender allows.

The chase does not stop (a request is still allowed each window), so nothing
is refused, but a long branch (the heavier ledger, hundreds of pages and
layer lists, each a request) then takes hours instead of seconds, which is
the same as not being able to take it for as long as the pin lasts, and the
other chase states (progress TTL one hour, `Slowed` backoff) are tuned for
minutes. The pin also applies to every ask to that peer, not only the
one that dropped (`paced(p)` is per peer), so a confirm or a weight query waits
for the same window.

## Reproduction

A temporary test in `pkg/p2p` (not committed): the node has `RequestTimeout`
1 s and `RequestPaceWindow` 2 s (the default 1 minute and 30 s scaled
down; the learned pace lasts 30 windows, 60 s here). A raw sender serves a
weightless hash listing in pages, answers every request at once, except
that it ignores the 6th and 7th requests. Then it answers everything again.
The requests the node sent, per window of 2 s:

```
outage ended at 1.202s; requests in all: 26
window 0 (0-2 s):     7 requests  (the outage: two lost)
windows 1-19 (2-40 s): 1 request each
```

After the sender is serving normally again the asker sends one request a
window for the rest of the test, 20 windows here; the 30 windows would have
held to 60 s. At the defaults that is one request a minute for 30 minutes.

## Impact

Medium. No consensus effect, and no ledger is refused. A sender that is
honest and busy for a minute (or a link that drops twice) leaves every
asker that was chasing it paced to a thirtieth or less of the rate it
could use, for half an hour, with the next outage (or the same sender's next
slow minute) extending it. A heavier ledger is reached hours later, which is
bad where the ledger is moving. It is a
cheap lever for a sender that wants to slow a node's chase of it without being
dropped (ignore two requests, serve normally after; the node then paces
itself to 1 a minute), but a sender can be slow anyway, so the
more serious case is the honest outage.

## Recommendation

Learn a limit only from drops that say something about the sender's budget,
and never let one drop pin the asker far below the configured pace:

- learn only when the asker was near its own pace when the drops began, for
  example `n >= RequestPace/2` (at low load, drops tell nothing about the
  sender's budget; leave the pace as it was);
- never go below a floor tied to the configured pace, for example
  `max(n*3/4, RequestPace/4)`, so a pin costs at most a factor of four;
- let the learned limit rise: on each answered request after the pin, raise
  it (for example by a quarter of the configured pace) until it reaches the
  configured pace, or end it after a fixed number of answered requests, so a
  sender that is serving again is served at the pace it allows;
- apply the learned pace to the requests of the chase's bulk steps only, not
  to a confirm or weight query.

No size, depth or count cap is added; the pace only spaces requests.

Test: as above; after the outage, the requests per window should return to
the rate seen before it within a few windows.

## Resolution

Fixed by BLUE-012 (decision D136). The first pace learned from a
sender is a probe. It doubles once in each `RequestPaceWindow` in which
the sender replies, and ends at `RequestPace`. After two lost requests,
the asker is back to the configured pace within about nine windows from
a pace of 1, and sooner from more.

The pace holds as D133 says when either of these happens:
- two more requests in a row are dropped while the pace is still
  probing; or
- after a pace lapses, a drop comes with as much sent as the sender's
  known budget.

A held pace is lowered only by drops that came with at least half of it
sent.

A sender with a smaller budget costs one more round of drops the first
time. D133's `TestSlowSenderIsReachedOverManyChases` and
`TestChaseBelowSenderBudgetIsReached` still pass: 20 of 20 runs in a row
under `-race`. A first version that doubled on every reply overshot a's
budget in that test, so the rise is once per window.

Not done: Red's floor of `RequestPace/4`. With a sender whose budget is
below it, every learning would overshoot and the flags would add up to a
time-out (D133). Also not done: sparing the confirm and weight queries,
since the pace is still per peer.

Test: `TestLearnedPaceRisesUnlessShown` (`pkg/p2p`).
