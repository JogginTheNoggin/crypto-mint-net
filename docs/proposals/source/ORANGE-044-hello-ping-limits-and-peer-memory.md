# ORANGE-044: Round 34 measured; the hello and ping at the new limits, and memory per connection

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `f1ac406` |
| Area | `pkg/wire` (hello and ping, D190), `pkg/p2p` (handshake, inbox, peer queues) |
| Effort | measurements only; four proposals |
| Decided by | Grey; Black and White in Part D for Proposal 1 |

## Summary

D190 raised an extension's value to 64 KiB and the features to 255, and
stopped keeping a peer's hello extensions. I measured the hello and the
ping at those limits, and the node's memory with many peers sending them.

**1. A hello or a ping at the limits is 2 MiB. Decoding one takes half a
millisecond, and a ping's request id 2.2 ms.**

| message | bytes | encode | decode |
| --- | --- | --- | --- |
| bare hello | 53 | 46 ns | 78 ns |
| typical hello (release, four features) | 114 | 68 ns | 365 ns |
| hello with 255 features, no extensions | 9,807 | 22 µs | 34 µs |
| largest hello (32 × 64 KiB extensions) | 2,107,398 | 3.7 ms | 0.56 ms |
| empty ping | 17 | 31 ns | 23 ns |
| largest ping | 2,097,361 | 4.7 ms | 0.49 ms |

- The largest hello was 35.5 KB in Round 33. It is now 60 times that.
- A ping's request id hashes its whole body: 2.2 ms for the largest.
  The node takes it, and decodes the ping, before the per-peer request
  rate is checked.
- So each largest ping costs the node about 2.7 ms of one core before it
  is answered or dropped.
- Encoding is slow because the writer grows from 128 bytes. Only the
  node's own small hello is encoded, so that does not matter.

**2. The node keeps nothing from a large hello. Each connection costs
about 1 MiB whatever its hello.**

| peers | heap per connection, largest hello | bare hello |
| --- | --- | --- |
| 32 | 1,043 KiB | 1,032 KiB |
| 128 | 1,035 KiB | 1,023 KiB |

D190 works as meant: the hello's 2 MiB is freed once it is read. The
1 MiB is there for any peer, before it sends anything:
- 800 KiB is the send queue, `make(chan wire.Message, 4096)`. A
  `wire.Message` is 200 bytes, and a buffered channel holds all its slots
  from the start.
- 200 KiB is the reply channel of the follow request the node sends each
  new peer, `make(chan wire.Message, 1024)`.

That is 32 MiB at the default 32 peers, and 1 GiB at 1,000.

**3. Peers sending the largest pings fill the inbox. The heap reached
3 to 5 GB.**

Each peer sent the largest pings as fast as loopback allowed for 5 s, the
same ping again and again.

| peers | pings taken | heap peak | heap 0.5 s after |
| --- | --- | --- | --- |
| 1 | 4,719 (944/s) | +279 MiB | +0 |
| 8 | 1,432 (281/s) | +3,005 MiB | +1,638 MiB |
| 32 | 3,651 (728/s) | +4,375 MiB | +1,650 MiB |
| 64 | 7,887 (1,565/s) | +4,799 MiB | +1,649 MiB |

- Each message that passes the cheap checks is queued whole in the
  inbox. The queue is capped at 1,024 messages for each kind of peer, not
  by bytes.
- One peer is held to 120 discovery messages a minute, so it can queue at
  most 240 MiB.
- Nine peers or more fill the queue for unlisted peers: 1,024 × 2 MiB is
  2 GiB, and the GC's headroom adds about as much again.
- A single worker takes the queue one message at a time, at about 2.7 ms
  a ping, so draining it takes about 3 s.
- The same holds before D190 for any message up to the 16 MiB frame
  limit, such as a large transaction or entry. Its size is checked only
  when it is taken off the queue. At that size, 1,024 messages are
  16 GiB. D190 only makes the ping one of those messages.
- Each round of the worker also copies the whole rest of the queue
  (`inbox.round`). With 1,024 queued, that is about 200 KB copied for
  each message taken.
- With 32 and 64 peers the node ended every flooding connection, with
  about one flag each. I did not trace which check did it.

**4. A connection that has not finished its hello can hold 16 MiB for 10
s, and such connections are not counted against the caps.**

Each connection sent a frame length of 16 MiB and then all but the last
byte, and never finished its hello.

| connections | heap | per connection | after the 10 s hello timeout |
| --- | --- | --- | --- |
| 32 | +528 MiB | 16.5 MiB | +0 |
| 128 | +2,065 MiB | 16.1 MiB | +0 |

- `handshake` reads the hello with the 16 MiB frame limit, and
  `ReadFrame` allocates the whole frame before reading it.
- `acceptLoop` checks `MaxPeers` and `MaxInbound` against registered
  peers only, so a connection still in its hello does not count. All 128
  were accepted with the default cap of 32 peers.
- The memory is freed after the 10 s timeout, and the sender can then
  start again.
- This is from before D190. A valid hello is now at most about 2.1 MB,
  so a 16 MiB hello frame is always invalid.

## Proposals

**Proposal 1: count connections in their hello against the inbound cap.**
`acceptLoop` would count connections still in `handshake` with the
registered inbound peers, or keep a small separate cap on them. That
bounds the memory in Section 4 by the cap instead of by how many
connections a sender opens. A second, smaller step is to read the hello
with a limit near the largest valid hello, for example 4 MiB, not 16 MiB.
White should say whether a later release might need a larger hello. A
hello's trailing bytes are refused today, so a new field would have to be
an extension or a new protocol version.

**Proposal 2: cap the inbox by bytes as well as by count.** For example,
64 MiB of message bodies for each kind of peer, with a message over the
cap dropped as when the queue is full. Then a flood of large messages
holds at most that much, whatever their size. Also, `inbox.round` could
keep a head index instead of copying the rest of the queue each round.

**Proposal 3: smaller per-peer channels.** Make the send queue a channel
of `*wire.Message`, or a slice that grows, and give the follow request's
reply channel a small buffer. That would take each connection from about
1 MiB to a few tens of KiB.

**Proposal 4: check the request rate before decoding a ping.** Take the
per-peer token before `DecodePing` and `RequestID`, giving it back if the
message turns out malformed. A peer over its rate would then cost nothing
to drop. It matters less once Proposal 2 caps the queue.

None of these changes what a node accepts from a peer that stays within
the limits.

Parked for a later round: why the node ended the flooding connections in
Section 3, and the pong rate with distinct pings.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harnesses were test files outside the
commit.

- **Hello and ping:** `Hello.Encode`, `DecodeHello`, `Ping.Encode`,
  `DecodePing` and `RequestID`. They ran 2,000 times each, or 200 times
  above 1 MiB. The largest hello has a 256-byte agent, a 64-byte release,
  255 features with 32-byte names, and 32 extensions of 64 KiB of random
  bytes.
- **Peers:** one node from the `pkg/p2p` tests, with `MaxPeers` and
  `MaxInbound` set to N + 8. N raw TCP clients each send their own
  largest hello (each with its own node id) and read everything back.
  - Heap is `HeapInuse` after two GCs, against the heap before the
    clients connect, with the clients' own hello frames freed. Peaks are
    sampled every 2 ms.
  - The flood: each client writes one largest ping frame in a loop for
    5 s.
  - Client and node share one process, so the client's few KiB per
    connection are included.
  - The per-peer sizes come from a heap profile at 128 peers.
- **Half hellos:** N clients each write a 16 MiB frame length and
  16 MiB − 1 bytes, then wait. The default caps apply.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code.
- Proposals 1 to 4 change how a node accepts connections and queues and
  answers messages from peers, not what any ledger, entry or layer
  verdict is.
- A message dropped from a full queue, or a connection refused, is a
  lack of information, not a disagreement. The sender, or another peer,
  can send it again.
- None adds a depth limit.
