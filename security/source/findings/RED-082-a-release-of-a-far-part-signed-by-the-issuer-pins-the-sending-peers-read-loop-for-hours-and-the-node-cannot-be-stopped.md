# RED-082: A release of a far part, signed by the issuer, pins the sending peer's read loop for hours, and the node cannot be stopped

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 27 Part B, on `main` @ `1a1c729` (D170, D168, D165) |
| Area | `pkg/p2p` (`handleRelease`, `takeRelease`), `pkg/node` (`AddRelease`), `pkg/series` (`CheckRelease`, `PartIDContext`) |
| Related | RED-070 (memory; fixed), RED-075 (reload time; fixed), GUI-028 (cancellable far part, D170) |
| For Black | No consensus rule is touched. D165's point is kept: no cap on parts. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-028) |

## What is wrong

D170 made the part-id chain walk cancellable (`PartIDContext`,
`CheckReleaseContext`), but only for the commands (`poo series release`,
`add-release` with the node stopped, and so on). The node's own paths still
call `s.CheckRelease(r)` with `context.Background()`: `node.AddRelease`
(p2p type 14 and `POST /v1/series/release`).

A release that passes `CheckReleaseSigned` (one signature check) is taken
at once and checked in full, in the receiving peer's read loop. For a part
k the check hashes k part ids. A release is signed by the issuer, but the
issuer's signature covers the release's `Previous`, not whether `Previous` is
right, so a signed release of part 2^36 with a junk `Previous` passes the
witness check and then costs 2^36 hashes (about 4.7 hours at the 4 million a
second here) before it fails. The same holds for a genuine far release.
One signed message, valid for ever, can be sent again after every restart.

## How I checked it

`pkg/p2p` test harness (throwaway, removed): `a` and `b` follow a variable
series; `a` sends one release of part 2^36 with `Previous` set to junk,
signed by the series' issuer key, with `Host.SendRelease`:

```
after 3 s: b released 1, peers 1               b's CPU about 1.2 cores
a then sends a genuine part 2:  b released 1   (a's read loop at b is stuck)
b.cancel() (stop the host):     b did NOT stop within 20 s
```

I killed the test process after about 70 s at 122% CPU. The host's `Run`
never returned, so `poo node` could not shut down on SIGINT either, except
by kill.

## What could happen

- The key that signs releases is trusted for supply. A leaked or hostile
  issuer key (or an issuer who signs by mistake) now also pegs a core on
  every follower and makes each stuck connection useless, and the nodes
  cannot be stopped or restarted cleanly while it runs. Restarting
  does not help: the peer sends it again.
- Every peer link that carries it is stuck in the same walk (they share the
  chain), so several of a node's peer slots are held by one message.
- A node under this cannot take a later, valid release from the same peer,
  as the test shows.

## Suggested fix (Blue's call)

- Give `AddRelease` and `takeRelease` the host's context (the one `Run`
  has), and use `CheckReleaseContext`, so stopping the node, and a peer
  closing, end the walk. Stopping says nothing about validity.
- Do not walk the chain in a peer's read loop. Run the full check in one
  worker per series (single flight), and let the loop go on reading. While
  it runs, further releases of that series are held as the window already
  does.
- Optionally bound the time of one full check on the p2p path and report
  "checking part k" in `poo status` / the log, so an owner can see it.
- Tests: a release of part 2^36 with a junk previous does not stop the host
  from stopping within a second; a peer that sent it can still send a
  transaction while it is checked.

## Not tested

The RPC `POST /v1/series/release` path (it takes the owner token, and uses
the request context for the wait only).

## Resolution

Fixed in BLUE-028 (D171). The full check of a release from a peer runs in
one worker per series (`pkg/p2p/releasequeue.go`), never in the read loop,
so the peer's later messages are still read. The walk uses the host's
context (`node.AddReleaseContext`), so stopping the node cancels it, and a
cancelled check is not counted against the peer. Bursts still coalesce
(D169), and there is still no cap on parts (D165). Test:
`TestFarReleaseDoesNotHoldTheNode` (part 2^36: a claim from the same peer
still arrives, and the node stops in under 3 s).
