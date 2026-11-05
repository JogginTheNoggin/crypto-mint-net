# RED-103: A ping at the new extension limit is 2 MiB, 24 inbound peers sending them made the node hold about 1.1 GiB more than the senders alone, and the inbox counts messages, not bytes

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 34 Part B, on `main` @ `f1ac406` (D190, W-14) |
| Area | `pkg/wire/hello.go` (`MaxExtValue`, `MaxExt`), `pkg/p2p/inbox.go`, `pkg/p2p/peer.go` (`dispatch`) |
| For Black | Not a rule: no decision depends on it. |

## What is wrong

D190 lets every extension value be 64 KiB, 32 of them in a hello, ping or
pong: a ping may be 2 MiB, where before D186 it was 16 bytes. A key or a
signature may need 64 KiB in a hello; no ping or pong has a reason to carry
one. The ping then goes through `Budget` (120 discovery messages a minute per
address) into the inbox, which holds up to `QueueLimit` (1,024) **messages** in
each of four queues, with their whole bodies, until the one worker reaches
them. Nothing counts the bytes in the queue. With the worker held up for
a while by a costly item, the bound is the message count times 2 MiB per
queue: up to 8 GiB at the defaults, and 240 MiB a minute per attacking
address from the budget alone.

## How I checked it

`pkg/p2p` test (throwaway, removed; copy kept with Red's notes): 30 raw
connections (24 were accepted at the default inbound cap) each said hello and
then sent 100 pings of 2 MiB (32 extensions of 64 KiB), 6,000 MiB offered in
all, the node's worker running normally:

```
senders to a sink that only reads:  peak heap 289 MiB
senders to the node:                peak heap 1356 MiB   (about 1.1 GiB more)
```

The test process holds the node and the senders, so the difference is the
node's. I did not stall the worker to fill the queue; the 8 GiB is from the
counts, not a run.

## Suggested fix (Blue's call)

- Bound the inbox in bytes as well as messages (the body bytes waiting, for
  example 64 MiB in all and a share for each queue), and drop over it as it
  drops over the count.
- Give discovery messages their own frame limit (a ping or pong carries no
  key: a few KiB of extensions is enough for what they say today), or drop a
  discovery message over a size before it is queued. Extension limits for
  the hello can stay at 64 KiB.

## Not tested

A stalled worker; many addresses; the same flood against pongs.
