# RED-003: An unverified inbound peer can start chases and pull in honest priority peers

| Field | Value |
| --- | --- |
| Severity | info |
| Component | pkg/p2p/forward.go (handlePushed/checkStated), pkg/p2p/chase.go (offer/runChase confirm step) |
| Found on | `claude/m10-watch` @ 581476e |
| Status | won't fix |
| Surface | peer |
| Fixed by | — |

## Summary

Any connected peer, including an unverified, unlisted inbound connection,
can assert an arbitrary `Stated` weight for a followed pool with no proof —
for example in a `TypeNotice` (whose `Tip` hash can be fabricated), or in
the `Stated` header of a ping or any pushed item. `handlePushed` feeds that
to `checkStated` → `offer`, and if the stated weight is above the node's
own it starts a chase group:

```go
// pkg/p2p/forward.go, TypeNotice
n, err := wire.DecodeNotice(m.Body)
...
at = &n.Tip
...
h.checkStated(pi, p, m.Stated, at)   // m.Stated is unproven, Tip can be fake
```

In the first step of a chase, a group whose best sender is not a priority
peer is put to one of the node's **honest priority peers** for confirmation:

```go
// pkg/p2p/chase.go, runChase
if !h.cfg.NoConfirm && j.Sender.Cat != peers.CatPriority && !j.Group.Asked {
    if q := h.askPeerIn(pi.section, peers.CatPriority, p); q != nil {
        heavier, stated := h.confirm(pi, q)   // request sent to an honest peer
        ...
```

So an attacker with only an inbound socket can cause the node to emit
weight requests to the peers it trusts most, and to spend chase worker
effort talking to the attacker (findCommon / checkWork rounds).

## Impact

Informational. The design already bounds this well, which is why it is not
rated higher:

- `tracking.StartChase` charges each new branch against the sender's chase
  budget (`ChaseBudget` = 30 per 10-minute window, per address), and going
  over adds an over-budget flag.
- `chase.Config` caps concurrent chases: `Chases` = 4 overall, `PerPool` =
  2, and `Unlisted` = 1 for groups whose best sender is unlisted. One sender
  runs one chase at a time, and `GroupsPerPool` = 64.
- A branch the attacker cannot back with real work ends as `Nowhere` or, if
  it serves a provably false branch, `FalseBranch` (a severity-5 flag).

The residual concern is amplification across many Sybil source addresses:
each distinct address gets its own chase budget and can nudge the node to
query its priority peers, so a botnet of inbound connections could apply a
low, steady query load to the honest priority set and keep the (small)
chase worker pool busy. The per-address caps keep this modest.

## Reproduction

1. Victim follows pool `P` and has at least one priority peer for `P`.
2. Attacker opens an inbound connection (hello only, no verification) and
   sends a `TypeNotice` for `P` with `Stated` set above the victim's own
   weight and `Tip` set to a random hash.
3. Observe (via `/v1/chases` and the victim's p2p log) a new chase group and
   a confirmation weight request sent to a priority peer, none of which the
   attacker had to back with real work.

## Recommendation

Hardening options for Blue/Grey to weigh (not a correctness fix):

- Do not let an `Unlisted`/unverified sender's stated weight trigger the
  priority-peer confirmation step on its own; require the group to reach
  `Corroborated` from listed peers before step 1 touches other peers, or
  gate the confirm step behind a minimum sender category.
- Consider ignoring the `Stated` header on `TypeNotice`/ping from
  connections that are not yet verified for the pool, treating it only as a
  hint once the sender has standing.
- Keep an eye on the interaction with RED-001 (an attacker who can also
  seed inbound Sybils benefits from both).

## Resolution

Blue's assessment: won't fix for now. The confirmation step is itself
the defence: it asks a trusted peer before the node acts on an unlisted
sender's weight. Refusing to chase on an unlisted sender's word would
let a node miss a heavier ledger that only a new peer has seen, which is
how a node catches up after a partition. The cost is bounded by the
per-address chase budget and the `Unlisted` chase cap, as the finding
notes. Revisit if a measurement shows the confirm load matters; Grey or
the owner may overrule.
