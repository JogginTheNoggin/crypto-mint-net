# RED-100: A connection that has not yet said hello makes the node allocate the frame size it declares (up to 16 MiB) for ten seconds, and nothing caps how many such connections there are

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | open |
| Found | Source Round 34 Part B, on `main` @ `f1ac406` (D190; the code is older, the wider hello limits make the right bound easy to name) |
| Area | `pkg/wire/wire.go` (`ReadFrame`), `pkg/p2p/peer.go` (`handshake`), `pkg/p2p/host.go` (`acceptLoop`, `fullLocked`) |
| For Black | Not a rule. |

## What is wrong

1. `handshake` reads the first frame with `wire.ReadFrame(r, wire.MaxFrame)`.
   `ReadFrame` reads the four-byte length and then does
   `b := make([]byte, size)` before reading any of the body, for up to
   `MaxFrame` (16 MiB). A hello is at most about 2.2 MiB (the longest
   release, 255 features, 32 extensions of 64 KiB), yet the first frame may
   declare 16 MiB.
2. `acceptLoop` refuses a connection only when the registered peers
   (`h.peers`) are at a cap or the address is blocked. A connection that
   is still handshaking is not in `h.peers`, so any number of them wait
   at once, each up to `helloTimeout` (10 s), and the attacker sends four
   bytes for each.

Linux gives untouched pages of a fresh large allocation no memory, so
the resident size stays small. The Go heap does count them: it believes
16 MiB per connection is live, and sets the next collection at twice
that. While they hang, the node's own garbage (decoded entries, replies,
status bodies) is not collected until it has doubled the inflated heap.
In a container with a memory limit that is the node killed, and on
systems that charge commit for an allocation (Windows) the allocations
themselves fail.

## How I checked it

`pkg/p2p` tests (throwaway, removed; copies kept with Red's notes), a node
started with the defaults, raw connections that write only the four-byte length of a
16 MiB frame (`01 00 00 00`) and nothing else:

```
300 connections, each declaring 16 MiB and sending nothing (4 bytes each)
heap in use: 0 MiB -> 4802 MiB; rss 12 -> 23 MiB
```

With the same 300 connections hanging, the test process (the node and the
test share it) allocated 6 GiB of short-lived 1 MiB buffers, touching each:

```
no hanging connections:   peak rss 18 MiB
300 hanging connections:  peak rss 3424 MiB
```

The garbage there is synthetic: it shows the collector waiting, not the
node's own rate of garbage. I did not run this inside a container with a
memory limit.

## Suggested fix (Blue's call)

- Read the hello frame under its own bound (`MaxHelloFrame`, a little over
  the largest hello, say 3 MiB) and refuse a longer one before allocating.
- Make `ReadFrame` read the body in pieces that grow as bytes arrive (for
  example `io.CopyN` into a `bytes.Buffer` with a small first size), so memory
  follows what the sender actually sent. That also covers the 16 MiB frame
  of a registered peer (bounded by the peer cap, but the same effect at
  `MaxInbound` times 16 MiB).
- Cap connections that have not finished the hello (for example
  `MaxInbound` again, and a few per address), and drop the rest at once.

## Not tested

A node inside a memory-limited container; Windows; connections from many
addresses (the test used one).
