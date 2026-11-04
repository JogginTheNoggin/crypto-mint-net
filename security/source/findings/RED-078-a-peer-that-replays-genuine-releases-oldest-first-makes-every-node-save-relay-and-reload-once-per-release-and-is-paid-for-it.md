# RED-078: A peer that replays genuine releases oldest first makes every node save, relay and reload once per release, and is credited for it

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 26 Part B, on `main` @ `439b369` (D168) |
| Area | `pkg/p2p` (`handleRelease`, `Released`, `Announce`), `pkg/node` (`AddRelease`), `pkg/tracking` (`FirstDelivery`, `Repeat`) |
| For Black | No consensus rule is touched. Every release in the replay is genuine and valid; the issue is what a node does for each one. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-027) |

## What is wrong

`handleRelease` ignores a release only when `r.Part <= s.Released()`. A node
that is behind therefore treats every older genuine release as new. A peer
holding releases 2..N (they are public; anyone who followed the series has
them) sends them **oldest first**, and for each one the node:

1. checks it (cheap: the shared chain makes it one hash),
2. loads node.json under `PolicyMu`, replaces the kept release, **saves and
   fsyncs the file**,
3. indexes the new pools, or, for a series followed by a list of pools or a
   whole follow that reaches `engine.ListAll`, **asks for a reload**,
4. **relays the release to every other verified peer** (`Announce`),
5. **credits the sender** with a first delivery (`FirstDelivery`), which
   raises its points toward priority.

A release is only useful if it is the newest. Releases below one the sender
could have sent are redundant by construction, and the sender can send the
newest one first.

## How I checked it

`pkg/p2p` test harness (throwaway, removed), series with parts of 4 and pools
of 4, `a` is the replayer and sends releases 2..300 to `b` with
`Host.SendRelease`, oldest first:

```
b at 300 after 1.0 s        b: 299 "release kept" lines (299 node.json saves)
d (honest relay peer of b)  299 "release kept" lines, at 300
c (follows a list of pools) 299 reload requests (Config.Reload)
a's points at b             0.0 -> 7.6  (priority threshold is 5)
a's offences                0
```

The same with `a` connected to two honest nodes `b` and `d` that are also
connected to each other, N = 2000:

```
b, d each took about 1,070 releases from a mix of a and each other
b and d each hold repeat-flood flags (FlagTotal 1 and 2) against their two peers;
the test did not separate which flag was against `a` and which against the other honest node
```

Each honest node relays what it took to the other, and the other has
usually taken it from `a` already, so each side counts the relay as a repeat
(`Repeat`, limit 600 a minute), and honest relays can be flagged by each
other. One repeat flag is severity 2 and `TimeOut` is at 10, so a replay
repeated for five minutes could pause honest links (by the defaults; I did not run it that long).

## What could happen

- A node that follows a list of pools reloads once per cycle of the replay;
  a replay paced to one release per reload (a message per reload, from a
  peer that has thousands of releases for a series with a part a minute)
  keeps the node reloading. A reload restarts the node's parts (about a second
  on the small scratch node I tried) and waits for the data lock (`daemon.Run`).
- Disk: one fsynced node.json write per release, on every node that takes
  the stream, and the same again on each relay hop.
- Bandwidth: each release is relayed to every verified peer, so one
  attacker message becomes about as many as the node has peers, at every hop.
- Reputation: the replayer earns priority at each node it replays to (the
  credit exists so that the first peer to deliver is preferred), so the
  attack improves the attacker's standing while it runs.
- Honest relay links flag each other (above).

## Suggested fix (Blue's call)

- A release below the newest one a node has been told of (but not yet took)
  need not be taken; the simplest rule is to **coalesce**: when a release
  arrives, wait a short window (a second), take only the highest part that
  arrived and checks, and drop the rest as covered. The chain check still
  proves the highest one, so nothing true is lost.
- Do not credit a first delivery for a release that is not the sender's
  highest known, or credit at most one release per peer per window.
- Do not relay a release when this node already knows (or has queued) a
  higher part.
- Rate-limit in-place applications and reload requests that a release
  causes, per peer and per node.
- Tests: a peer sending parts 2..300 oldest first makes the node write
  node.json once or a few times, request at most one reload, relay at most a
  few messages, and earn at most one first-delivery credit.

## Not tested

A real daemon (not the harness) under the replay, for the time the reloads
take on a node with many pools.

## Resolution

Fixed in BLUE-027 (D169), building on ORANGE-036 (#178).

- A release from a peer has its witness checked on arrival (`Series.CheckReleaseSigned`, one signature, no chain), so a forged one is an offence at once and cannot hold a place.
- The first release of a series is taken at once. Releases that arrive in the next 2 seconds wait, and only the highest of them is taken when the window ends, crediting its sender. So a replay costs a node about one save, relay, reload and first-delivery credit per window, not one per release.
- A copy of the part the node already holds is not counted as a repeat (honest links cross). Older parts still are.
- Test: `TestReleaseReplayIsCoalesced`. With a 300 ms window, parts 2 to 200 sent oldest first leave the node at part 200 after at most 4 takes and 4 reloads, with no offence. With no window, the same replay takes 199.
