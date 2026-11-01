# pkg/chase

## Summary

The chase queue decides which stated weights above this node's own to
follow up, in what order, and from whom. It follows the design's "Chasing
Heavier Ledgers", steps 0 to 2 and its limits. Stated weights that name
the same branch are **grouped**; groups are ordered by **evidence**, never
by how heavy they claim to be, so a node cannot jump the queue by lying
about its weight. The queue also keeps each sender's **backoff** and a
**known-bad list** of entry hashes shown to be false. It only decides;
package `p2p` runs each chase (steps 3 to 5) and reports how it ended. A
node switches only through the engine, on weight it computed itself, so
everything here is node policy. The package reads no clock.

## Groups and evidence

`Offer(pool, sender, hash, now)` puts a stated weight in a group: the one
holding the entry hash it came with (a pushed entry, layer or notice names
one), else a group of the sender's own. A sender has **one open stated
weight per pool**; a newer one replaces its older one. `Offer` reports
whether the sender joined the group with this stated weight (`isNew`),
which `p2p` counts against the sender's chase budget.

| Evidence | Means |
| --- | --- |
| `work-checked` | a chase has checked the branch's provisional layers |
| `corroborated` | at least 2 independent listed senders (different ranges), or a priority peer confirmed it |
| `word-only` | anything else |

Unlisted senders do not corroborate unless `UnlistedCount` is set. Within
a level, groups go by the best category among their senders, then the
number of independent senders, then age. A group waiting longer than its
best category's **maximum delay** (priority 5 s, backup 30 s, candidate
1 min, unlisted 5 min) goes to the front of its level, so nothing waits
forever. Each pool holds at most 64 groups; a new group displaces the
lowest only if it would rank above it.

## Running chases

`Next(now)` returns the next job: a group and the sender to chase it with
(its best one that is not backed off and not busy). At most 4 chases run
at once, at most 2 per pool and 1 with an unlisted sender. `Wait` says how
long until a backed-off sender is free. While a chase runs, `Confirm`
records a priority peer's answer, `AddHashes` adds the branch's hashes
(merging groups that turn out to name the same branch), and `WorkChecked`
raises the evidence.

`Finish(id, outcome, bad, now)` ends it:

| Outcome | Then |
| --- | --- |
| `switched` | the sender's backoff is reset; the group ends |
| `nowhere` | the sender is backed off (30 s, doubling to 1 h) and leaves the group; others may still try |
| `false` | the sender is backed off; the hashes shown false, if any, go on the known-bad list for 24 h; the group ends |
| `gone` | the sender disconnected; it leaves the group |
| `caught` | this node caught up meanwhile; the group ends |
| `requeue` | a priority peer confirmed it; the group waits for its next turn |
| `yield` | the chase ran out of its time slice or the sender's budget for now; the sender waits `YieldWait` (5 s) with no backoff, and the group keeps its senders and evidence, so the chase goes on later (D116) |

`IsBad` lets `p2p` ignore pushed items and offers naming a known-bad hash.
`Drop` takes a disconnected sender out of every waiting group.

## API

`New(Config)`, `DefaultConfig`, `Offer`, `Leave`, `IsBad`, `BackedOff`,
`Next`, `Wait`, `Confirm`, `AddHashes`, `WorkChecked`, `Finish`, `Drop`,
`Groups(pool, now)` (in queue order; "" for every pool).

## Tests

`chase_test.go`: grouping and one open stated weight per sender, order by
evidence rather than weight, maximum delays, corroboration by unlisted
senders only when allowed, the running limits, backoff growing and
resetting, false branches and the known-bad list, confirmation, merging
and dropping, and the cap per pool. `pkg/p2p`'s `chase_test.go` runs whole
chases against a hand-driven peer.

## Extending

A new kind of evidence is a new `Evidence` level and a rule in
`evidenceLocked`; a new outcome is a constant and a case in `Finish`.
