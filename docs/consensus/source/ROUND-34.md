# Source Round 34: consensus review (Black, Part D)

## Summary

This round settles the wire limits that freeze at the first tag, and
bounds what a peer or a file can make a node hold.

Part A (D190) does five things:
- moves the default ports to 7732 and 7733;
- widens the hello's room for later fields (64 KiB extensions, 255
  features);
- keeps none of a peer's hello extensions;
- renames the ledger file kind to `poon-ledger`;
- meets Round 33's condition with `wire.ErrBadExtras`.

Blue's D191 fixes RED-100 to RED-103, and carries Green's GREEN-059. It:
- reads frames as their bytes arrive;
- caps the hello frame at 3 MiB;
- counts handshakes against `max_inbound`;
- bounds each inbox queue in bytes;
- bounds import and ledger-file reads.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - Byte form v1 is untouched. `TestByteFormV1Frozen` passes on `main`
    and on PR #280, and the Round 24 and Round 25 cross-version tests
    give the same bytes on both.
  - Every new limit is on wire frames, queues or tool input. None is on
    an entry, a layer or a ledger.
- **Round 33's condition is met.** A bad release, features list or
  extension list now ends only that connection, with no offence.
  `misbehave` checks `ErrBadExtras` before `ErrMalformed`, and
  `TestExtrasAtTheLimitsNoOffence` covers a hello and a ping at the frozen
  maximum, plus a ping whose tags are out of order.
- **No entry is refused by size, and a heavier ledger is still taken.**
  - Chase and request replies don't pass through the inbox. They go
    straight to the waiting call, so the byte cap never drops a page the
    node asked for.
  - An empty queue always takes one message, however large.
  - A pushed message dropped from a full queue is lack of information.
    The node still learns of a heavier ledger from stated weights and
    chases it.
- **The 3 MiB hello frame fits every valid hello.** The largest hello the
  frozen limits allow is about 2.0 MiB.
- **Blue's question, a separate discovery frame limit:** no consensus
  issue either way. Black's view is below. In short, a ping limit is safe
  only if it is at least the largest valid ping, and it is frozen with
  wire version 1.

| Field | Value |
| --- | --- |
| Round | Source Round 34 |
| Consensus | None (D190, D191) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `8ceb7db` (PR #272: D190; GUI #279 on top) |
| Previous base | `main` @ `3c4ae00` |
| Blue branches | PR #280 @ `e77a120` (BLUE-035, D191: RED-100 to RED-103; carries GREEN-059 #277) |
| Red | PR #275 @ `6b74d70` (RED-100 to RED-103) |
| Proposals | ORANGE-044 (PR #276 @ `2819e3e`, P1 for Black and White); GREEN-059 (PR #277 @ `fa10ad5`, carried in #280) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## Round 33's condition: bad extras cost no offence (D190)

- **The error.** `wire.ErrBadExtras` marks a failure inside the release,
  the features list or the extension list of a hello, ping or pong. It
  also wraps `ErrMalformed`, so every existing caller still treats it as
  malformed.
- **The handling.** `misbehave` checks `ErrBadExtras` first and only
  closes the connection. A malformed hello never reached `misbehave`, and
  still only closes in `handshake`.
- **What still earns a flag.** Any other malformation of a ping or pong
  still earns the usual `InvalidMessage` flag, as for every message: a
  short body, trailing bytes after the lists, or a bad contact. D190
  rewords term 4 to say so. Black accepts that. Those parts of the layout
  never change, so no compliant later build can trip them.
- **The test.** `TestExtrasAtTheLimitsNoOffence` covers a hello with the
  longest release, 255 unknown features and 32 unknown 64 KiB extensions,
  and a ping at the same limit. Both are answered with no offence. A ping
  with its tags out of order ends the connection with no offence.

## Wider limits frozen at the tag (D190, W-14, W-17)

- **Room for post-quantum fields.** With `MaxExtValue` at 64 KiB and
  `MaxFeatures` at 255, a later extension can carry an ML-DSA or SLH-DSA
  key or signature. These limits and the release's characters freeze with
  wire version 1 at the first tag.
- **Still not consensus.** These fields are still read only for display
  and for optional extras that both peers list (Round 33's terms 2 and 3,
  unchanged).
- **Extensions not kept (ORANGE-043 proposal 1).** No tag is registered,
  so `theirs.Ext = nil`. Nothing could read them anyway.
- **`poon-ledger`.** A file name. Imports still run every check.

## Bounds on frames, queues and files (D191)

### Frames and the hello (RED-100, ORANGE-044 P1)

- **Frames are read as they arrive.** `readGrowing` starts at 64 KiB and
  doubles, so a declared length costs nothing until the bytes come.
  `MaxFrame` (16 MiB) is unchanged for every message after the hello, so
  no entry or reply is refused that was taken before.
- **The 3 MiB hello frame.** It applies only to the first frame. The
  largest valid hello under the frozen limits is:
  - 2 + 32 + (agent ≤ 256) + (release ≤ 64);
  - 255 × (1 + 32 + 2) for the features;
  - 32 × (2 + 3 + 65,536) for the extensions.

  That is about 2.11 million bytes (2.0 MiB) with the length prefixes. So
  every hello a compliant build can send fits. Because the limits freeze,
  a later wire-1 build can't need more. A new wire version that raises
  them would have to raise this bound with them.
- **Handshakes count against `max_inbound`.** While `max_inbound`
  connections are still saying hello, a new inbound one is closed. That
  bounds memory. At worst it delays who can connect in, which is lack of
  information. Outbound dials, and the chase over existing peers, are
  untouched.

### Inbox queues bounded in bytes (RED-103, ORANGE-044 P2)

- **What the queues hold.** Pushed entries, layers, transactions and
  notices, releases, discovery messages and requests. `dispatch` sends
  replies (`IsReply`, `IsDiscoveryReply`) straight to the waiting call,
  so a chase page or a requested entry never meets the byte cap.
- **One always gets through.** An empty queue takes any one message, so
  an entry of any size that is pushed to an idle queue is still handled.
  This keeps D144's "no size bound on an entry".
- **A dropped message is missing information.** It is dropped as the
  1,024-message cap already did. The node learns of a heavier ledger
  again from stated weights in pings and checks, and chases it with
  requests, whose replies aren't queued.

### Tool input (RED-101, RED-102)

The import call stops reading past the batch bound once the first item is
in, so the first item still has no size bound. The ledger file's header
keys are bounded, and early entries are spooled to disk. Every imported
entry still goes through the peer path as `Requested` (Round 33).

## Blue's question: a separate discovery frame limit (RED-103)

Black's view, for Grey and White:

- **No consensus issue either way.** Pings and pongs carry no ledger
  data. Losing one is lack of information.
- **Any ping limit must be at least the largest valid ping.** Under the
  frozen limits, a ping is 16 bytes plus up to 32 × (2 + 3 + 65,536)
  bytes of extensions, about 2.0 MiB, plus the header's section and
  stated weight.
  - A limit below that refuses what the frozen layout allows. A later
    release that uses the extension room would then lose its pings to
    older nodes, and older nodes would end its peerings after
    `MaxMisses`. That is the exact case term 4 exists to prevent.
  - A limit at `MaxHelloFrame` (3 MiB) for ping would be compatible.
- **Pongs need their own check before any cap.** A pong or partial pong
  can carry peer and policy lists. Their largest valid size has to be
  worked out before any cap below `MaxFrame` is set.
- **If taken, it freezes too.** Like the hello limits, a discovery cap
  would be part of wire version 1 from the tag. A frame over it would
  close with no offence, as a bad extras list does.
- **Black's recommendation.** Leave it out, as Blue did. The queue byte
  cap and the per-peer request budget already bound what pings can cost.
  If a cap is wanted, use 3 MiB for ping only, and add a test that the
  largest valid ping passes.

## Clear

| Item | Why it is clear |
| --- | --- |
| D190: ports 7732 and 7733 | Node policy defaults. Nothing is released |
| D190 (W-14, W-17): `MaxExtValue` 64 KiB, `MaxFeatures` 255, frozen at the tag | Wire room for later optional fields. Nothing decides on them |
| D190: a peer's hello extensions not kept | Memory. Nothing read them |
| D190: `ErrBadExtras` ends only the connection (Round 33's condition) | Condition met and tested at the frozen maximum. Term 4 reworded as Black allowed |
| D190 (W-15): `poon-ledger` | A file name |
| D191 (RED-100): frames read as they arrive; 3 MiB hello frame; handshakes count against `max_inbound` | Every valid hello fits (about 2.0 MiB). `MaxFrame` unchanged after the hello. Delays are lack of information |
| D191 (RED-103): inbox queues bounded in bytes; one message always taken | Replies bypass the inbox. No entry refused by size. A dropped push is missing information |
| D191 (RED-101, RED-102): import read stops past the batch bound; ledger header bounded | The first item has no bound. Every imported entry is checked as from a peer |
| GREEN-059 (carried in #280); ORANGE-044 (P1 and P2 built, P3 and P4 left for Grey) | Tooling and measurements. P3 and P4 change memory and order of work, not verdicts |
| Red (PR #275): RED-100 to RED-103 | Findings, fixed in #280. None touches a rule |

## Items for the owner

- **Nothing needs your approval this round.**
- **The network messages now leave room for post-quantum signatures**
  in later releases. They are fixed at the first tag, and older nodes
  will skip what they don't know without penalising the sender.
- **A node is harder to flood, and still takes every heavier ledger.**
  The new limits apply to how much a peer can make it hold, never to
  which ledger is valid.

How this was checked:
- Read D190 and D191, `wire/hello.go` (`ErrBadExtras`, the limits),
  `wire/wire.go` (`ReadFrame`, `readGrowing`, `MaxHelloFrame`),
  `p2p/peer.go` (`misbehave`, `dispatch`, `handshake`, `runInbound`),
  `p2p/inbox.go` (`put`), `p2p/extras_test.go`, `rpc/ledgerport.go`
  (`decodeImport`), `cli/ledgerport.go` (`readLedgerFile`), and the diffs
  of PRs #275, #276, #277 and #280.
- Black worked out the largest valid hello and ping from the frozen
  limits.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `8ceb7db` and `e77a120`.
- `go test ./...` passes on `8ceb7db`, #275, #276, #277 and #280. The wire,
  p2p, node, rpc and cli packages pass under `-race` on `e77a120`.
